package services

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"K2board/internal/database"
	"K2board/internal/models"
)

var (
	ErrReferralDisabled   = errors.New("referral disabled")
	ErrInviteInvalid      = errors.New("invalid invite code")
	ErrInviteSelf         = errors.New("cannot use own invite code")
	ErrWithdrawMin        = errors.New("below minimum withdraw amount")
	ErrWithdrawBalance    = errors.New("insufficient commission balance")
	ErrWithdrawMethod     = errors.New("invalid payout method")
	ErrWithdrawStatus     = errors.New("invalid withdraw status")
	ErrWithdrawNotFound   = errors.New("withdraw not found")
	ErrWithdrawAmount     = errors.New("invalid withdraw amount")
	ErrReferralNotEnabled = errors.New("referral feature is off")
)

// PayoutMethod is one allowed cash-out channel shown to users.
type PayoutMethod struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// DefaultPayoutMethods used when settings are empty.
var DefaultPayoutMethods = []PayoutMethod{
	{Code: "alipay", Name: "支付宝"},
	{Code: "wechat", Name: "微信"},
	{Code: "usdt_trc20", Name: "USDT-TRC20"},
	{Code: "bank", Name: "银行卡"},
}

// ReferralConfig is the runtime config loaded from settings.
type ReferralConfig struct {
	Enable      bool
	RatePercent int   // e.g. 10 = 10%
	MinWithdraw int64 // cents
	Methods     []PayoutMethod
}

// LoadReferralConfig reads settings (defaults: enable, 10%, ¥100 min).
func LoadReferralConfig() ReferralConfig {
	cfg := ReferralConfig{
		Enable:      true,
		RatePercent: 10,
		MinWithdraw: 10000, // 100.00 CNY
		Methods:     append([]PayoutMethod(nil), DefaultPayoutMethods...),
	}
	en := strings.TrimSpace(SettingValue("referral_enable"))
	if en == "false" || en == "0" {
		cfg.Enable = false
	} else if en == "true" || en == "1" {
		cfg.Enable = true
	}
	if r := strings.TrimSpace(SettingValue("referral_rate")); r != "" {
		if v, err := strconv.Atoi(r); err == nil && v >= 0 && v <= 100 {
			cfg.RatePercent = v
		}
	}
	if m := strings.TrimSpace(SettingValue("referral_min_withdraw")); m != "" {
		// Accept cents (integer) or yuan with optional decimal (e.g. "100" or "100.00")
		if v, err := parseMoneyToCents(m); err == nil && v >= 0 {
			cfg.MinWithdraw = v
		}
	}
	if raw := strings.TrimSpace(SettingValue("referral_payout_methods")); raw != "" {
		var list []PayoutMethod
		if err := json.Unmarshal([]byte(raw), &list); err == nil && len(list) > 0 {
			cleaned := make([]PayoutMethod, 0, len(list))
			for _, p := range list {
				code := strings.TrimSpace(p.Code)
				name := strings.TrimSpace(p.Name)
				if code == "" {
					continue
				}
				if name == "" {
					name = code
				}
				cleaned = append(cleaned, PayoutMethod{Code: code, Name: name})
			}
			if len(cleaned) > 0 {
				cfg.Methods = cleaned
			}
		}
	}
	return cfg
}

// parseMoneyToCents parses stored setting value. Settings always store integer cents
// (e.g. "10000" = ¥100). Decimal forms are treated as yuan for safety.
func parseMoneyToCents(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty")
	}
	if strings.Contains(s, ".") {
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return 0, err
		}
		return int64(math.Round(f * 100)), nil
	}
	return strconv.ParseInt(s, 10, 64)
}

// GenerateInviteCode creates an 8-char uppercase alphanumeric code.
func GenerateInviteCode() (string, error) {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // no 0/O/1/I
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	for i := range b {
		b[i] = alphabet[int(b[i])%len(alphabet)]
	}
	return string(b), nil
}

// EnsureInviteCode assigns a unique invite code if missing. Safe to call repeatedly.
func EnsureInviteCode(userID uint) (string, error) {
	var u models.User
	if err := database.DB.Select("id", "invite_code").First(&u, userID).Error; err != nil {
		return "", err
	}
	if u.InviteCode != "" {
		return u.InviteCode, nil
	}
	for i := 0; i < 12; i++ {
		code, err := GenerateInviteCode()
		if err != nil {
			return "", err
		}
		var n int64
		database.DB.Model(&models.User{}).Where("invite_code = ?", code).Count(&n)
		if n > 0 {
			continue
		}
		res := database.DB.Model(&models.User{}).
			Where("id = ? AND (invite_code = '' OR invite_code IS NULL)", userID).
			Update("invite_code", code)
		if res.Error != nil {
			return "", res.Error
		}
		if res.RowsAffected > 0 {
			return code, nil
		}
		// concurrent write — re-read
		_ = database.DB.Select("invite_code").First(&u, userID).Error
		if u.InviteCode != "" {
			return u.InviteCode, nil
		}
	}
	return "", errors.New("failed to allocate invite code")
}

// EnsureAllInviteCodes backfills missing invite codes (startup), then ensures a
// partial unique index so concurrent allocations cannot collide.
func EnsureAllInviteCodes() {
	var users []models.User
	database.DB.Select("id").Where("invite_code = '' OR invite_code IS NULL").Find(&users)
	for _, u := range users {
		if _, err := EnsureInviteCode(u.ID); err != nil {
			slog.Warn("backfill invite code failed", "user_id", u.ID, "error", err)
		}
	}
	if n := len(users); n > 0 {
		slog.Info("backfilled invite codes", "count", n)
	}
	// Partial unique: empty strings remain allowed only if any slipped through;
	// non-empty codes must be unique (Postgres + MySQL 8.0.13+ functional/partial).
	// Best-effort: log and continue if dialect lacks partial indexes.
	if err := ensureInviteCodeUniqueIndex(); err != nil {
		slog.Warn("invite_code unique index not applied", "error", err)
	}
}

func ensureInviteCodeUniqueIndex() error {
	// Postgres partial unique index (empty codes excluded)
	if err := database.DB.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS idx_users_invite_code_nonzero
		ON users (invite_code)
		WHERE invite_code IS NOT NULL AND invite_code <> ''
	`).Error; err == nil {
		return nil
	}
	// MySQL / others: plain unique (idempotent — ignore "already exists")
	err := database.DB.Exec(`
		CREATE UNIQUE INDEX idx_users_invite_code_nonzero ON users (invite_code)
	`).Error
	if err == nil {
		return nil
	}
	s := strings.ToLower(err.Error())
	if strings.Contains(s, "already exists") ||
		strings.Contains(s, "duplicate key name") ||
		strings.Contains(s, "duplicate name") {
		return nil
	}
	// Non-empty duplicates would fail create — surface that
	return err
}

// ResolveInviterID returns inviter user id for a code, or 0 if empty/invalid.
// Does not error on empty code (optional).
func ResolveInviterID(code string, selfEmail string) (uint, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" {
		return 0, nil
	}
	// normalize: only A-Z0-9
	var b strings.Builder
	for _, r := range code {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(unicode.ToUpper(r))
		}
	}
	code = b.String()
	if len(code) < 4 || len(code) > 16 {
		return 0, ErrInviteInvalid
	}
	var inv models.User
	if err := database.DB.Select("id", "email", "enable").Where("invite_code = ?", code).First(&inv).Error; err != nil {
		return 0, ErrInviteInvalid
	}
	if !inv.Enable {
		return 0, ErrInviteInvalid
	}
	if selfEmail != "" && strings.EqualFold(inv.Email, selfEmail) {
		return 0, ErrInviteSelf
	}
	return inv.ID, nil
}

// RecordCommissionOnFulfill credits inviter when a paid order is fulfilled.
// Must run inside the same transaction as fulfill. Idempotent via unique order_id.
// Free orders (total_amount<=0) and users without inviter are no-ops.
//
// Deadlock note: fulfillUser already holds FOR UPDATE on the buyer. We must NOT
// take FOR UPDATE on the inviter here — circular invite graphs (A→B and B→A)
// would lock (buyer then inviter) in opposite order and deadlock. Balance is
// updated with atomic SQL expressions instead.
func RecordCommissionOnFulfill(tx *gorm.DB, order *models.Order) error {
	if order == nil || order.TotalAmount <= 0 || order.UserID == 0 {
		return nil
	}
	cfg := LoadReferralConfig()
	if !cfg.Enable || cfg.RatePercent <= 0 {
		return nil
	}

	var buyer models.User
	if err := tx.Select("id", "inviter_id").First(&buyer, order.UserID).Error; err != nil {
		return err
	}
	if buyer.InviterID == 0 {
		return nil
	}
	// No self-referral even if data corrupted
	if buyer.InviterID == buyer.ID {
		return nil
	}

	// Idempotent: already credited for this order?
	var exist int64
	tx.Model(&models.CommissionLedger{}).Where("order_id = ?", order.ID).Count(&exist)
	if exist > 0 {
		return nil
	}

	amount := order.TotalAmount * int64(cfg.RatePercent) / 100
	if amount <= 0 {
		return nil
	}

	// Existence only (no row lock — see deadlock note above)
	var inviter models.User
	if err := tx.Select("id").First(&inviter, buyer.InviterID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			slog.Warn("commission skip: inviter missing", "inviter_id", buyer.InviterID, "order", order.TradeNo)
			return nil
		}
		return err
	}

	ledger := &models.CommissionLedger{
		UserID:      inviter.ID,
		FromUserID:  buyer.ID,
		OrderID:     order.ID,
		TradeNo:     order.TradeNo,
		OrderAmount: order.TotalAmount,
		RatePercent: cfg.RatePercent,
		Amount:      amount,
		Status:      models.CommissionCredited,
		Remark:      fmt.Sprintf("订单 %s 返佣 %d%%", order.TradeNo, cfg.RatePercent),
	}
	if err := tx.Create(ledger).Error; err != nil {
		// Concurrent fulfill of same order: unique(order_id) — treat as already done
		if isUniqueViolation(err) {
			return nil
		}
		return err
	}

	// Atomic increment (safe under concurrent commissions to the same inviter)
	if err := tx.Model(&models.User{}).Where("id = ?", inviter.ID).Updates(map[string]any{
		"balance":          gorm.Expr("balance + ?", amount),
		"commission_total": gorm.Expr("commission_total + ?", amount),
	}).Error; err != nil {
		return err
	}

	slog.Info("commission credited",
		"inviter_id", inviter.ID,
		"buyer_id", buyer.ID,
		"order_id", order.ID,
		"trade_no", order.TradeNo,
		"amount", amount,
		"rate", cfg.RatePercent,
	)
	return nil
}

// RequestWithdraw holds amount from balance and creates a pending withdraw.
// Allowed even when referral_enable=false so users can still cash out accrued balance.
func RequestWithdraw(userID uint, amount int64, method, account, accountName string) (*models.CommissionWithdraw, error) {
	cfg := LoadReferralConfig()
	// Payout methods still come from config; do not block on Enable
	if amount <= 0 {
		return nil, ErrWithdrawAmount
	}
	if amount < cfg.MinWithdraw {
		return nil, ErrWithdrawMin
	}
	method = strings.TrimSpace(method)
	account = strings.TrimSpace(account)
	accountName = strings.TrimSpace(accountName)
	if method == "" || account == "" {
		return nil, ErrWithdrawMethod
	}
	if !payoutMethodAllowed(cfg.Methods, method) {
		return nil, ErrWithdrawMethod
	}
	if len(account) > 255 || len(accountName) > 128 {
		return nil, ErrWithdrawMethod
	}

	var out *models.CommissionWithdraw
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		var u models.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&u, userID).Error; err != nil {
			return err
		}
		if IsAccountBanned(&u) {
			return errors.New("account disabled")
		}
		if u.Balance < amount {
			return ErrWithdrawBalance
		}
		// pending count limit (anti-spam)
		var pendingN int64
		tx.Model(&models.CommissionWithdraw{}).
			Where("user_id = ? AND status = ?", userID, models.WithdrawPending).
			Count(&pendingN)
		if pendingN >= 5 {
			return errors.New("too many pending withdrawals")
		}

		// Conditional deduct: never go negative even under concurrent requests
		res := tx.Model(&models.User{}).
			Where("id = ? AND balance >= ?", userID, amount).
			Update("balance", gorm.Expr("balance - ?", amount))
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrWithdrawBalance
		}
		w := &models.CommissionWithdraw{
			UserID:      userID,
			Amount:      amount,
			Status:      models.WithdrawPending,
			Method:      method,
			Account:     account,
			AccountName: accountName,
		}
		if err := tx.Create(w).Error; err != nil {
			return err
		}
		out = w
		return nil
	})
	return out, err
}

// AdminApproveWithdraw marks pending withdraw as paid (funds already held).
func AdminApproveWithdraw(id uint, remark string) error {
	return database.DB.Transaction(func(tx *gorm.DB) error {
		var w models.CommissionWithdraw
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&w, id).Error; err != nil {
			return ErrWithdrawNotFound
		}
		if w.Status != models.WithdrawPending {
			return ErrWithdrawStatus
		}
		now := time.Now()
		// Conditional update prevents double-approve races even if status check races
		res := tx.Model(&models.CommissionWithdraw{}).
			Where("id = ? AND status = ?", id, models.WithdrawPending).
			Updates(map[string]any{
				"status":       models.WithdrawPaid,
				"admin_remark": strings.TrimSpace(remark),
				"processed_at": now,
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrWithdrawStatus
		}
		return nil
	})
}

// AdminRejectWithdraw refunds balance and marks rejected.
func AdminRejectWithdraw(id uint, remark string) error {
	return database.DB.Transaction(func(tx *gorm.DB) error {
		var w models.CommissionWithdraw
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&w, id).Error; err != nil {
			return ErrWithdrawNotFound
		}
		if w.Status != models.WithdrawPending {
			return ErrWithdrawStatus
		}
		now := time.Now()
		// Mark rejected first (conditional) so a crash/retry cannot double-refund
		res := tx.Model(&models.CommissionWithdraw{}).
			Where("id = ? AND status = ?", id, models.WithdrawPending).
			Updates(map[string]any{
				"status":       models.WithdrawRejected,
				"admin_remark": strings.TrimSpace(remark),
				"processed_at": now,
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrWithdrawStatus
		}
		// Refund held amount
		if err := tx.Model(&models.User{}).Where("id = ?", w.UserID).
			Update("balance", gorm.Expr("balance + ?", w.Amount)).Error; err != nil {
			return err
		}
		return nil
	})
}

func payoutMethodAllowed(list []PayoutMethod, code string) bool {
	for _, p := range list {
		if p.Code == code {
			return true
		}
	}
	return false
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	// Do NOT match generic "constraint" — that swallows FK/check errors and can
	// skip commission credit incorrectly.
	return strings.Contains(s, "unique") ||
		strings.Contains(s, "duplicate key") ||
		strings.Contains(s, "duplicate entry")
}

// ── Query helpers ─────────────────────────────────

// ReferralOverview for user portal.
type ReferralOverview struct {
	Enable          bool           `json:"enable"`
	InviteCode      string         `json:"invite_code"`
	InviteURL       string         `json:"invite_url"`
	RatePercent     int            `json:"rate_percent"`
	MinWithdraw     int64          `json:"min_withdraw"` // cents
	Balance         int64          `json:"balance"`
	CommissionTotal int64          `json:"commission_total"`
	InviteeCount    int64          `json:"invitee_count"`
	PendingWithdraw int64          `json:"pending_withdraw"` // cents held in pending
	PayoutMethods   []PayoutMethod `json:"payout_methods"`
}

func GetReferralOverview(userID uint) (*ReferralOverview, error) {
	cfg := LoadReferralConfig()
	code, err := EnsureInviteCode(userID)
	if err != nil {
		return nil, err
	}
	var u models.User
	if err := database.DB.Select("id", "balance", "commission_total").First(&u, userID).Error; err != nil {
		return nil, err
	}
	var inviteeCount int64
	database.DB.Model(&models.User{}).Where("inviter_id = ?", userID).Count(&inviteeCount)
	var pendingSum int64
	database.DB.Model(&models.CommissionWithdraw{}).
		Where("user_id = ? AND status = ?", userID, models.WithdrawPending).
		Select("COALESCE(SUM(amount),0)").Scan(&pendingSum)

	siteURL := strings.TrimRight(SettingValue("site_url"), "/")
	inviteURL := ""
	if siteURL != "" {
		inviteURL = siteURL + "/#/user/register?invite=" + code
	} else {
		inviteURL = "/#/user/register?invite=" + code
	}

	return &ReferralOverview{
		Enable:          cfg.Enable,
		InviteCode:      code,
		InviteURL:       inviteURL,
		RatePercent:     cfg.RatePercent,
		MinWithdraw:     cfg.MinWithdraw,
		Balance:         u.Balance,
		CommissionTotal: u.CommissionTotal,
		InviteeCount:    inviteeCount,
		PendingWithdraw: pendingSum,
		PayoutMethods:   cfg.Methods,
	}, nil
}

// ListUserLedgers paginated commission history for inviter.
func ListUserLedgers(userID uint, page, pageSize int) ([]models.CommissionLedger, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	var total int64
	q := database.DB.Model(&models.CommissionLedger{}).Where("user_id = ?", userID)
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []models.CommissionLedger
	err := q.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&list).Error
	if err != nil {
		return nil, 0, err
	}
	// Fill from emails
	ids := make([]uint, 0, len(list))
	for _, l := range list {
		ids = append(ids, l.FromUserID)
	}
	emailMap := loadUserEmails(ids)
	for i := range list {
		list[i].FromUserEmail = emailMap[list[i].FromUserID]
	}
	return list, total, nil
}

// ListUserWithdraws for user portal.
func ListUserWithdraws(userID uint, page, pageSize int) ([]models.CommissionWithdraw, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	var total int64
	q := database.DB.Model(&models.CommissionWithdraw{}).Where("user_id = ?", userID)
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []models.CommissionWithdraw
	err := q.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&list).Error
	return list, total, err
}

// ListInvitees returns users invited by userID.
func ListInvitees(userID uint, page, pageSize int) ([]map[string]any, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	var total int64
	database.DB.Model(&models.User{}).Where("inviter_id = ?", userID).Count(&total)
	var users []models.User
	err := database.DB.Select("id", "email", "created_at", "plan_id", "enable").
		Where("inviter_id = ?", userID).
		Order("id DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).
		Find(&users).Error
	if err != nil {
		return nil, 0, err
	}
	out := make([]map[string]any, 0, len(users))
	for _, u := range users {
		// mask email: a***@x.com
		out = append(out, map[string]any{
			"id":         u.ID,
			"email":      maskEmailLocal(u.Email),
			"created_at": u.CreatedAt,
			"plan_id":    u.PlanID,
			"enable":     u.Enable,
		})
	}
	return out, total, nil
}

// Admin list helpers

func AdminListWithdraws(status string, page, pageSize int) ([]models.CommissionWithdraw, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	q := database.DB.Model(&models.CommissionWithdraw{})
	if status != "" {
		q = q.Where("status = ?", status)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []models.CommissionWithdraw
	if err := q.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&list).Error; err != nil {
		return nil, 0, err
	}
	ids := make([]uint, 0, len(list))
	for _, w := range list {
		ids = append(ids, w.UserID)
	}
	em := loadUserEmails(ids)
	for i := range list {
		list[i].UserEmail = em[list[i].UserID]
	}
	return list, total, nil
}

func AdminListLedgers(page, pageSize int, userID uint) ([]models.CommissionLedger, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	q := database.DB.Model(&models.CommissionLedger{})
	if userID > 0 {
		q = q.Where("user_id = ?", userID)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []models.CommissionLedger
	if err := q.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&list).Error; err != nil {
		return nil, 0, err
	}
	ids := make([]uint, 0, len(list)*2)
	for _, l := range list {
		ids = append(ids, l.UserID, l.FromUserID)
	}
	em := loadUserEmails(ids)
	for i := range list {
		list[i].UserEmail = em[list[i].UserID]
		list[i].FromUserEmail = em[list[i].FromUserID]
	}
	return list, total, nil
}

func loadUserEmails(ids []uint) map[uint]string {
	out := make(map[uint]string)
	if len(ids) == 0 {
		return out
	}
	// unique
	seen := make(map[uint]struct{})
	uniq := make([]uint, 0, len(ids))
	for _, id := range ids {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		uniq = append(uniq, id)
	}
	var users []models.User
	database.DB.Select("id", "email").Where("id IN ?", uniq).Find(&users)
	for _, u := range users {
		out[u.ID] = u.Email
	}
	return out
}

func maskEmailLocal(email string) string {
	email = strings.TrimSpace(email)
	at := strings.Index(email, "@")
	if at <= 0 {
		return "***"
	}
	local := email[:at]
	domain := email[at:]
	if len(local) <= 1 {
		return "*" + domain
	}
	if len(local) == 2 {
		return local[:1] + "*" + domain
	}
	return local[:1] + "***" + local[len(local)-1:] + domain
}
