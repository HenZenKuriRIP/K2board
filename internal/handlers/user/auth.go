package user

import (
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"K2board/internal/database"
	"K2board/internal/middleware"
	"K2board/internal/models"
	"K2board/internal/services"
	"K2board/internal/utils"
)

type UserAuthHandler struct {
	userSvc *services.UserService
}

func NewUserAuthHandler() *UserAuthHandler {
	return &UserAuthHandler{userSvc: services.NewUserService()}
}

// ── Request / Response types ──────────────────────

type SendCodeRequest struct {
	Email string `json:"email" binding:"required,email"`
}

type RegisterRequest struct {
	Email      string `json:"email" binding:"required,email"`
	Password   string `json:"password" binding:"required,min=6"`
	Code       string `json:"code" binding:"required,len=6"`
	InviteCode string `json:"invite_code"` // optional referral code
}

type LoginRequest struct {
	// No binding tags here: we normalize then validate so trailing spaces / case
	// don't produce opaque 400s that the UI mislabels as wrong password.
	Email    string `json:"email"`
	Password string `json:"password"`
}

type AuthResponse struct {
	Token        string `json:"token"`
	UUID         string `json:"uuid"`
	Email        string `json:"email"`
	SubscribeURL string `json:"subscribe_url"`
}

// ── In-memory verification code store ─────────────
// Keys are purpose-scoped (register / reset) so codes cannot be reused across flows.

const (
	codePurposeRegister = "register"
	codePurposeReset    = "reset"
)

// maxCodeFails: wrong attempts before the code is invalidated (must re-send).
// Does not change happy-path register/login for real users.
const maxCodeFails = 5

type codeEntry struct {
	Code     string
	ExpireAt time.Time
	// sentAt used for resend rate limit (independent of expire window)
	SentAt    time.Time
	FailCount int
}

var (
	codeStore   = make(map[string]*codeEntry)
	codeStoreMu sync.RWMutex
)

func codeStoreKey(purpose, email string) string {
	return purpose + ":" + email
}

func init() {
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			codeStoreMu.Lock()
			now := time.Now()
			for k, v := range codeStore {
				if now.After(v.ExpireAt) {
					delete(codeStore, k)
				}
			}
			codeStoreMu.Unlock()
		}
	}()
}

func storeCode(purpose, email, code string) {
	codeStoreMu.Lock()
	codeStore[codeStoreKey(purpose, email)] = &codeEntry{
		Code:      code,
		ExpireAt:  time.Now().Add(10 * time.Minute),
		SentAt:    time.Now(),
		FailCount: 0,
	}
	codeStoreMu.Unlock()
}

func verifyCode(purpose, email, code string) bool {
	codeStoreMu.Lock()
	defer codeStoreMu.Unlock()
	key := codeStoreKey(purpose, email)
	entry, ok := codeStore[key]
	if !ok || time.Now().After(entry.ExpireAt) {
		if ok {
			delete(codeStore, key)
		}
		return false
	}
	if entry.Code != code {
		entry.FailCount++
		// Brute-force lockout: invalidate and force a new send-code.
		// Legitimate users just re-request the email (60s resend gate still applies once re-sent).
		if entry.FailCount >= maxCodeFails {
			delete(codeStore, key)
		}
		return false
	}
	// One-time use
	delete(codeStore, key)
	return true
}

func canResend(purpose, email string) (bool, time.Duration) {
	codeStoreMu.RLock()
	defer codeStoreMu.RUnlock()
	entry, ok := codeStore[codeStoreKey(purpose, email)]
	if !ok {
		return true, 0
	}
	wait := 60*time.Second - time.Since(entry.SentAt)
	if wait > 0 {
		return false, wait
	}
	return true, 0
}

// ── SendCode ──────────────────────────────────────
// POST /api/v1/user/send-code
func (h *UserAuthHandler) SendCode(c *gin.Context) {
	if !isRegisterAllowed() {
		utils.Forbidden(c, "当前未开放注册")
		return
	}

	if !middleware.CheckLoginRate(c.ClientIP()) {
		utils.Error(c, 429, "请求过于频繁，请稍后再试")
		return
	}

	var req SendCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "请输入有效的邮箱地址")
		return
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))

	existing, _ := h.userSvc.GetUserByEmail(email)
	if existing != nil {
		utils.BadRequest(c, "该邮箱已注册")
		return
	}

	if ok, wait := canResend(codePurposeRegister, email); !ok {
		utils.Error(c, 429, fmt.Sprintf("发送过于频繁，请 %.0f 秒后重试", wait.Seconds()))
		return
	}

	code, err := generateCode(6)
	if err != nil || code == "" {
		utils.InternalError(c, "生成验证码失败")
		return
	}

	cfg := loadSMTPConfigFromDB()
	if cfg == nil {
		utils.InternalError(c, "邮件服务未配置，请联系管理员在后台填写 SMTP")
		return
	}

	subject, body := renderMailTemplate("register", code)
	if err := cfg.SendMail(email, subject, body); err != nil {
		slog.Error("send verification email failed", "email", email, "error", err)
		utils.InternalError(c, "邮件发送失败: "+err.Error())
		return
	}

	// Only store after successful send
	storeCode(codePurposeRegister, email, code)

	masked := maskEmail(email)
	utils.SuccessMessage(c, "验证码已发送至 "+masked)
}

// ── Register ──────────────────────────────────────
// POST /api/v1/user/register
func (h *UserAuthHandler) Register(c *gin.Context) {
	if !isRegisterAllowed() {
		utils.Forbidden(c, "当前未开放注册")
		return
	}

	if !middleware.CheckLoginRate(c.ClientIP()) {
		utils.Error(c, 429, "请求过于频繁，请稍后再试")
		return
	}

	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "请求参数无效")
		return
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))

	if !verifyCode(codePurposeRegister, email, strings.TrimSpace(req.Code)) {
		utils.BadRequest(c, "验证码错误或已过期")
		return
	}

	existing, _ := h.userSvc.GetUserByEmail(email)
	if existing != nil {
		utils.BadRequest(c, "该邮箱已注册")
		return
	}

	// Optional invite: invalid code rejects registration so users notice typos.
	// Inviter binding is permanent (not rebindable after register).
	var inviterID uint
	if strings.TrimSpace(req.InviteCode) != "" {
		id, err := services.ResolveInviterID(req.InviteCode, email)
		if err != nil {
			if errors.Is(err, services.ErrInviteSelf) {
				utils.BadRequest(c, "不能使用自己的邀请码")
				return
			}
			utils.BadRequest(c, "邀请码无效")
			return
		}
		inviterID = id
	}

	user := &models.User{
		Email:     email,
		Password:  req.Password,
		IsAdmin:   false,
		Enable:    true,
		InviterID: inviterID,
	}

	if err := h.userSvc.CreateUser(user); err != nil {
		slog.Error("user register failed", "email", email, "error", err)
		utils.InternalError(c, "创建账号失败")
		return
	}

	subscribeURL := buildSubscribeURL(user.Token)
	utils.Created(c, AuthResponse{
		Token:        user.Token,
		UUID:         user.UUID,
		Email:        user.Email,
		SubscribeURL: subscribeURL,
	})
}

// ── Login ─────────────────────────────────────────
func (h *UserAuthHandler) Login(c *gin.Context) {
	if !middleware.CheckLoginRate(c.ClientIP()) {
		utils.Error(c, 429, "请求过于频繁，请稍后再试")
		return
	}

	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// Empty body / wrong Content-Type / non-JSON — not a credential failure
		slog.Warn("user login bind failed", "ip", c.ClientIP(), "error", err.Error())
		utils.BadRequest(c, "请求体无效，请使用 JSON 提交邮箱和密码")
		return
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))
	// Trim edges only — matches admin password reset; avoids paste whitespace mismatches
	password := strings.TrimSpace(req.Password)
	if email == "" || password == "" {
		utils.BadRequest(c, "请输入邮箱和密码")
		return
	}
	if !strings.Contains(email, "@") || len(email) < 3 {
		utils.BadRequest(c, "邮箱格式不正确")
		return
	}
	if len(password) < 6 {
		utils.BadRequest(c, "密码至少 6 位")
		return
	}

	user, err := h.userSvc.Authenticate(email, password)
	if err != nil || user == nil {
		// Wrong credentials → 401 (bcrypt path; slower than bind failures)
		slog.Info("user login failed", "email", email, "ip", c.ClientIP())
		utils.Unauthorized(c, "邮箱或密码错误")
		return
	}

	if user.IsAdmin {
		slog.Info("admin blocked on user portal", "email", email, "ip", c.ClientIP())
		utils.Forbidden(c, "管理员请使用管理后台登录（用户中心不接受管理员账号）")
		return
	}

	// enable = admin ban only; expired accounts may still log in to renew
	if services.IsAccountBanned(user) {
		utils.Forbidden(c, "账号已被禁用，请联系管理员")
		return
	}
	slog.Info("user login ok", "email", user.Email, "user_id", user.ID, "ip", c.ClientIP())

	subscribeURL := buildSubscribeURL(user.Token)
	utils.Success(c, AuthResponse{
		Token:        user.Token,
		UUID:         user.UUID,
		Email:        user.Email,
		SubscribeURL: subscribeURL,
	})
}

// ── Forgot password / reset via email ─────────────

type ResetPasswordRequest struct {
	Email       string `json:"email" binding:"required,email"`
	Code        string `json:"code" binding:"required,len=6"`
	NewPassword string `json:"new_password" binding:"required,min=6"`
}

// SendResetCode POST /api/v1/user/forgot-password/send-code
// Sends a purpose=reset verification code. Always returns a generic success when
// the email is not registered (anti-enumeration), except SMTP/rate failures.
func (h *UserAuthHandler) SendResetCode(c *gin.Context) {
	if !middleware.CheckLoginRate(c.ClientIP()) {
		utils.Error(c, 429, "请求过于频繁，请稍后再试")
		return
	}

	var req SendCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "请输入有效的邮箱地址")
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))

	// Rate limit even for non-existent emails
	if ok, wait := canResend(codePurposeReset, email); !ok {
		utils.Error(c, 429, fmt.Sprintf("发送过于频繁，请 %.0f 秒后重试", wait.Seconds()))
		return
	}

	user, _ := h.userSvc.GetUserByEmail(email)
	// Admin / missing：不发邮件，但写入 rate window（防枚举 + 防刷）
	if user == nil || user.IsAdmin {
		if code, err := generateCode(6); err == nil && code != "" {
			// 未发信的随机码仅用于 60s 限流，校验会因用户不存在而失败
			storeCode(codePurposeReset, email, code)
		}
		utils.SuccessMessage(c, "若该邮箱已注册，验证码将发送至邮箱")
		return
	}

	cfg := loadSMTPConfigFromDB()
	if cfg == nil {
		utils.InternalError(c, "邮件服务未配置，请联系管理员在后台填写 SMTP")
		return
	}

	code, err := generateCode(6)
	if err != nil || code == "" {
		utils.InternalError(c, "生成验证码失败")
		return
	}

	subject, body := renderMailTemplate("reset", code)
	if err := cfg.SendMail(email, subject, body); err != nil {
		slog.Error("send reset password email failed", "email", email, "error", err)
		utils.InternalError(c, "邮件发送失败: "+err.Error())
		return
	}

	storeCode(codePurposeReset, email, code)
	masked := maskEmail(email)
	utils.SuccessMessage(c, "验证码已发送至 "+masked)
}

// ResetPassword POST /api/v1/user/reset-password
// Resets password with email verification code (no login token required).
func (h *UserAuthHandler) ResetPassword(c *gin.Context) {
	if !middleware.CheckLoginRate(c.ClientIP()) {
		utils.Error(c, 429, "请求过于频繁，请稍后再试")
		return
	}

	var req ResetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "请填写有效邮箱、6 位验证码和新密码（至少 6 位）")
		return
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))
	code := strings.TrimSpace(req.Code)
	newPassword := strings.TrimSpace(req.NewPassword)
	if len(newPassword) < 6 {
		utils.BadRequest(c, "新密码至少 6 位")
		return
	}

	if !verifyCode(codePurposeReset, email, code) {
		utils.BadRequest(c, "验证码错误或已过期")
		return
	}

	user, err := h.userSvc.GetUserByEmail(email)
	if err != nil || user == nil {
		utils.BadRequest(c, "账号不存在")
		return
	}
	if user.IsAdmin {
		utils.Forbidden(c, "管理员请在管理后台重置密码")
		return
	}

	if err := h.userSvc.UpdatePassword(user.ID, newPassword); err != nil {
		slog.Error("reset password failed", "email", email, "error", err)
		utils.InternalError(c, "重置密码失败，请稍后重试")
		return
	}

	slog.Info("user password reset ok", "email", email, "user_id", user.ID, "ip", c.ClientIP())
	utils.SuccessMessage(c, "密码已重置，请使用新密码登录")
}

// ── Helpers ───────────────────────────────────────

func isRegisterAllowed() bool {
	var s models.Setting
	if err := database.DB.Where("key = ?", "allow_register").First(&s).Error; err != nil {
		return false
	}
	return s.Value == "true"
}

func buildSubscribeURL(token string) string {
	// Prefer subscribe_url, then site_url. Use a fresh struct each query so GORM
	// does not append primary key from a previous row (e.g. empty subscribe_url
	// id=3 → wrongly "WHERE key=site_url AND id=3").
	base := settingValueByKey("subscribe_url")
	if base == "" {
		base = settingValueByKey("site_url")
	}
	base = strings.TrimRight(base, "/")
	if base == "" {
		return "/api/v1/client/subscribe?token=" + token
	}
	return base + "/api/v1/client/subscribe?token=" + token
}

func settingValueByKey(key string) string {
	var s models.Setting
	if err := database.DB.Where("key = ?", key).First(&s).Error; err != nil {
		return ""
	}
	return strings.TrimSpace(s.Value)
}

func generateCode(digits int) (string, error) {
	if digits <= 0 {
		digits = 6
	}
	max := big.NewInt(1)
	for i := 0; i < digits; i++ {
		max.Mul(max, big.NewInt(10))
	}
	n, err := rand.Int(rand.Reader, max)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%0*d", digits, n.Int64()), nil
}

func maskEmail(email string) string {
	at := strings.Index(email, "@")
	if at <= 0 {
		return "***"
	}
	local := email[:at]
	domain := email[at:]
	if len(local) <= 2 {
		return local[:1] + "***" + domain
	}
	return local[:2] + "***" + domain
}

func loadSMTPConfigFromDB() *utils.SMTPConfig {
	var items []models.Setting
	database.DB.Where("key IN ?", []string{"smtp_host", "smtp_port", "smtp_user", "smtp_pass", "smtp_from"}).Find(&items)
	m := make(map[string]string)
	for _, s := range items {
		m[s.Key] = s.Value
	}
	return utils.SMTPConfigFromMap(m)
}

// defaultMailTemplates used when settings are empty.
var defaultMailTemplates = map[string][2]string{
	"register": {
		"【{{site_name}}】邮箱验证码",
		"您的 {{site_name}} 验证码是: {{code}}\n\n" +
			"验证码 10 分钟内有效，请勿泄露给他人。\n\n" +
			"如果您没有注册 {{site_name}} 账号，请忽略此邮件。",
	},
	"reset": {
		"【{{site_name}}】重置密码验证码",
		"您正在重置 {{site_name}} 账号密码。\n\n" +
			"验证码：{{code}}\n\n" +
			"验证码 10 分钟内有效，请勿泄露给他人。\n\n" +
			"如非本人操作，请忽略本邮件，并建议尽快登录检查账号安全。",
	},
}

// renderMailTemplate loads subject/body from settings and substitutes {{code}} / {{site_name}}.
func renderMailTemplate(kind, code string) (subject, body string) {
	siteName := settingValueByKey("site_name")
	if siteName == "" {
		siteName = "东京热云"
	}

	var subjKey, bodyKey string
	switch kind {
	case "reset":
		subjKey, bodyKey = "mail_tpl_reset_subject", "mail_tpl_reset_body"
	default:
		subjKey, bodyKey = "mail_tpl_register_subject", "mail_tpl_register_body"
		kind = "register"
	}

	subject = strings.TrimSpace(settingValueByKey(subjKey))
	body = strings.TrimSpace(settingValueByKey(bodyKey))
	if subject == "" {
		subject = defaultMailTemplates[kind][0]
	}
	if body == "" {
		body = defaultMailTemplates[kind][1]
	}

	replacer := strings.NewReplacer(
		"{{code}}", code,
		"{{site_name}}", siteName,
		"{{CODE}}", code,
		"{{SITE_NAME}}", siteName,
	)
	return replacer.Replace(subject), replacer.Replace(body)
}
