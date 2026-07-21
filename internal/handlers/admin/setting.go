package admin

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"K2board/internal/database"
	"K2board/internal/middleware"
	"K2board/internal/models"
	"K2board/internal/services"
	"K2board/internal/utils"
)

type SettingHandler struct{}

func NewSettingHandler() *SettingHandler { return &SettingHandler{} }

var allowedSettings = map[string]bool{
	"site_name": true, "site_url": true, "subscribe_url": true,
	// Shadow user-portal origins (CORS + payment return_url host allow-list)
	"allowed_origins": true,
	"panel_token":     true, "allow_register": true,
	"smtp_host": true, "smtp_port": true, "smtp_user": true, "smtp_pass": true, "smtp_from": true,
	// 邮件模板（支持 {{code}} {{site_name}}）
	"mail_tpl_register_subject": true, "mail_tpl_register_body": true,
	"mail_tpl_reset_subject": true, "mail_tpl_reset_body": true,
	// 推广返佣
	"referral_enable": true, "referral_rate": true,
	"referral_min_withdraw": true, "referral_payout_methods": true,
}

const smtpPassMask = "********"

func (h *SettingHandler) GetAll(c *gin.Context) {
	var settings []models.Setting
	database.DB.Find(&settings)
	result := make(map[string]string)
	for _, s := range settings {
		switch s.Key {
		case "panel_token":
			if len(s.Value) >= 8 {
				result[s.Key] = "****" + s.Value[len(s.Value)-8:]
			} else if s.Value == "" {
				result[s.Key] = "未设置"
			} else {
				result[s.Key] = "已设置"
			}
		case "smtp_pass":
			if s.Value != "" {
				result[s.Key] = smtpPassMask
			} else {
				result[s.Key] = ""
			}
		default:
			result[s.Key] = s.Value
		}
	}

	// Expose current admin account (do not hardcode id=1)
	var admin models.User
	if err := database.DB.Select("id", "email").Where("is_admin = ?", true).Order("id ASC").First(&admin).Error; err == nil {
		result["admin_email"] = admin.Email
		result["admin_id"] = strconv.FormatUint(uint64(admin.ID), 10)
	}

	// Defaults for first-run UI
	if _, ok := result["allow_register"]; !ok {
		result["allow_register"] = "false"
	}
	if _, ok := result["smtp_port"]; !ok {
		result["smtp_port"] = "587"
	}
	if _, ok := result["referral_enable"]; !ok {
		result["referral_enable"] = "true"
	}
	if _, ok := result["referral_rate"]; !ok {
		result["referral_rate"] = "10"
	}
	// Expose min withdraw in yuan for admin form (stored as cents)
	if raw, ok := result["referral_min_withdraw"]; ok && raw != "" {
		if cents, err := strconv.ParseInt(raw, 10, 64); err == nil {
			result["referral_min_withdraw"] = formatYuanFromCents(cents)
		}
	} else {
		result["referral_min_withdraw"] = "100"
	}
	if _, ok := result["referral_payout_methods"]; !ok {
		result["referral_payout_methods"] = `[{"code":"alipay","name":"支付宝"},{"code":"wechat","name":"微信"},{"code":"usdt_trc20","name":"USDT-TRC20"},{"code":"bank","name":"银行卡"}]`
	}
	if _, ok := result["allowed_origins"]; !ok {
		result["allowed_origins"] = ""
	}
	// Effective list (site_url + subscribe_url + allowed_origins) for admin UI hint
	if eff := services.ListEffectiveCORSOrigins(); len(eff) > 0 {
		result["allowed_origins_effective"] = strings.Join(eff, "\n")
	} else {
		result["allowed_origins_effective"] = ""
	}

	utils.Success(c, result)
}

func (h *SettingHandler) UpdateAll(c *gin.Context) {
	var req map[string]string
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "invalid request")
		return
	}

	for key, value := range req {
		if !allowedSettings[key] {
			continue
		}

		// Never overwrite panel token with masked display value
		if key == "panel_token" {
			if value == "" || strings.HasPrefix(value, "****") || value == "未设置" || value == "已设置" {
				continue
			}
			if !isHashed(value) {
				value = utils.SHA256(value)
			}
			middleware.InvalidatePanelTokenCache()
		}

		// Referral: normalize rate (0-100) and min withdraw (store cents)
		if key == "referral_rate" {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			n, err := strconv.Atoi(value)
			if err != nil || n < 0 || n > 100 {
				utils.BadRequest(c, "返佣比例须为 0–100 的整数")
				return
			}
			value = strconv.Itoa(n)
		}
		if key == "referral_min_withdraw" {
			// Admin UI sends yuan (e.g. "100" or "100.5"); store as cents string
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			// If client already sends cents with key hint "_cents" not used —
			// prefer: if value has decimal or is small, treat as yuan; else if
			// ends with explicit "cents" skip. Simpler: always yuan from admin UI.
			// Frontend sends yuan; convert here.
			cents, err := yuanToCents(value)
			if err != nil || cents < 0 {
				utils.BadRequest(c, "最低提现金额无效")
				return
			}
			value = strconv.FormatInt(cents, 10)
		}
		if key == "referral_enable" {
			v := strings.ToLower(strings.TrimSpace(value))
			if v == "1" || v == "true" || v == "on" {
				value = "true"
			} else {
				value = "false"
			}
		}
		if key == "referral_payout_methods" {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			// basic JSON array check
			if !strings.HasPrefix(value, "[") {
				utils.BadRequest(c, "收款方式须为 JSON 数组")
				return
			}
		}

		// allowed_origins: normalize to https://host per line; reject all-invalid
		if key == "allowed_origins" {
			norm, err := services.ValidateAndNormalizeAllowedOriginsSetting(value)
			if err != nil {
				utils.BadRequest(c, err.Error())
				return
			}
			value = norm
		}

		// site_url / subscribe_url: trim trailing slash for stable origins
		if key == "site_url" || key == "subscribe_url" {
			value = strings.TrimRight(strings.TrimSpace(value), "/")
			if value != "" {
				// Soft normalize if looks like URL; keep as-is if admin uses bare host
				if o := services.NormalizeOrigin(value); o != "" {
					// Preserve path-less origin only when input was origin-like
					if !strings.Contains(strings.TrimPrefix(strings.TrimPrefix(value, "https://"), "http://"), "/") {
						value = o
					}
				}
			}
		}

		// Keep existing SMTP password when client sends empty / mask
		if key == "smtp_pass" {
			if value == "" || value == smtpPassMask {
				continue
			}
		}

		var setting models.Setting
		result := database.DB.Where("key = ?", key).First(&setting)
		if result.Error != nil {
			database.DB.Create(&models.Setting{Key: key, Value: value})
		} else {
			database.DB.Model(&setting).Update("value", value)
		}
	}

	// CORS / return_url allow-list may have changed
	services.InvalidateCORSOriginsCache()

	utils.SuccessMessage(c, "settings updated")
}

// TestEmailRequest supports testing with saved settings or form overrides (unsaved draft).
type TestEmailRequest struct {
	To       string `json:"to" binding:"required,email"`
	SMTPHost string `json:"smtp_host"`
	SMTPPort string `json:"smtp_port"`
	SMTPUser string `json:"smtp_user"`
	SMTPPass string `json:"smtp_pass"`
	SMTPFrom string `json:"smtp_from"`
}

// TestEmail sends a test email using form overrides when provided, else saved settings.
// POST /api/v1/admin/settings/test-email
func (h *SettingHandler) TestEmail(c *gin.Context) {
	var req TestEmailRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "请提供有效的测试邮箱地址")
		return
	}

	cfg := loadSMTPConfig()
	// Merge form overrides so admin can test without saving first
	if req.SMTPHost != "" || req.SMTPUser != "" || req.SMTPPort != "" || req.SMTPFrom != "" {
		m := map[string]string{}
		if cfg != nil {
			m["smtp_host"] = cfg.Host
			m["smtp_port"] = strconv.Itoa(cfg.Port)
			m["smtp_user"] = cfg.User
			m["smtp_pass"] = cfg.Pass
			m["smtp_from"] = cfg.From
		}
		if req.SMTPHost != "" {
			m["smtp_host"] = req.SMTPHost
		}
		if req.SMTPPort != "" {
			m["smtp_port"] = req.SMTPPort
		}
		if req.SMTPUser != "" {
			m["smtp_user"] = req.SMTPUser
		}
		if req.SMTPFrom != "" {
			m["smtp_from"] = req.SMTPFrom
		}
		// Only override pass when not mask/empty
		if req.SMTPPass != "" && req.SMTPPass != smtpPassMask {
			m["smtp_pass"] = req.SMTPPass
		} else if cfg != nil {
			m["smtp_pass"] = cfg.Pass
		}
		cfg = utils.SMTPConfigFromMap(m)
	}

	if cfg == nil {
		utils.BadRequest(c, "SMTP 未配置，请先填写邮件服务器信息")
		return
	}

	err := cfg.SendMail(req.To,
		"K2Board 邮件测试",
		"这是一封来自 K2Board 管理面板的测试邮件。\n\n"+
			"如果您收到此邮件，说明 SMTP 邮件服务配置成功。\n\n"+
			"SMTP 服务器: "+cfg.Host+"\n"+
			"发件邮箱: "+cfg.FromAddr()+"\n",
	)
	if err != nil {
		utils.InternalError(c, "邮件发送失败: "+err.Error())
		return
	}

	utils.SuccessMessage(c, "测试邮件已发送至 "+req.To)
}

// ── Helpers ───────────────────────────────────────

func loadSMTPConfig() *utils.SMTPConfig {
	var items []models.Setting
	database.DB.Where("key IN ?", []string{"smtp_host", "smtp_port", "smtp_user", "smtp_pass", "smtp_from"}).Find(&items)
	m := make(map[string]string)
	for _, s := range items {
		m[s.Key] = s.Value
	}
	return utils.SMTPConfigFromMap(m)
}

func isHashed(v string) bool {
	if len(v) != 64 {
		return false
	}
	for _, c := range v {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

// yuanToCents parses admin form yuan amount (e.g. "100" or "100.50") to cents.
func yuanToCents(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty")
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, err
	}
	if f < 0 || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, fmt.Errorf("invalid")
	}
	return int64(math.Round(f * 100)), nil
}

func formatYuanFromCents(cents int64) string {
	if cents%100 == 0 {
		return strconv.FormatInt(cents/100, 10)
	}
	return fmt.Sprintf("%.2f", float64(cents)/100)
}
