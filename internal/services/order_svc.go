package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"K2board/internal/database"
	"K2board/internal/models"
	"K2board/internal/payment"
	// register built-in gateways
	_ "K2board/internal/payment/gateways"
)

const defaultOrderTTL = 30 * time.Minute

// Anti-abuse caps for user order creation (in-process + DB checks).
const (
	maxPendingOrdersPerUser = 3
	minCreateInterval       = 5 * time.Second
)

var (
	ErrPlanNotForSale     = errors.New("plan not available for purchase")
	ErrOrderNotFound      = errors.New("order not found")
	ErrOrderExpired       = errors.New("order expired")
	ErrOrderStatus        = errors.New("invalid order status")
	ErrMethodDisabled     = errors.New("payment method disabled or unknown")
	ErrAlreadyPaid        = errors.New("order already paid")
	ErrAmountMismatch     = errors.New("paid amount mismatch")
	ErrSiteURL            = errors.New("site_url not configured")
	ErrNotifyIgnored      = errors.New("notify ignored") // waiting/timeout — not an HTTP error path
	ErrTooManyPending     = errors.New("too many pending orders")
	ErrCreateTooFast      = errors.New("order create too frequent")
	ErrFreeAlreadyClaimed = errors.New("free plan already claimed for this email")
	ErrFreeWhileActive    = errors.New("free plan not available while service is active")
)

// NormalizeEmail lowercases and trims (claim key for free plans).
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// UserHasActiveService is true when the user already has an activated entitlement
// (plan or group) that is not expired. Used to block free re-claim while still active.
// New users (plan_id=0 and group_id=0) are not active even if expire_at=0.
//
// Account ban (enable=false) is orthogonal: enforced at login / order / API gates.
// Entitlement is plan/group + expire_at only so “套餐是否可用” stays independent of 封禁.
func UserHasActiveService(u *models.User, nowUnix int64) bool {
	if u == nil {
		return false
	}
	if u.PlanID == 0 && u.GroupID == 0 {
		return false
	}
	if u.ExpireAt > 0 && u.ExpireAt < nowUnix {
		return false
	}
	// expire_at == 0 with plan/group → permanent active
	return true
}

// CanUserPurchasePlan: public shop new sale OR renew for current plan holder.
// Plan must be enable. Renew requires AllowRenew and user.PlanID == plan.ID.
func CanUserPurchasePlan(u *models.User, plan *models.Plan) bool {
	if u == nil || plan == nil || !plan.Enable {
		return false
	}
	if plan.ShowOnShop {
		return true
	}
	// Off-shelf renew: only the current bound plan
	return plan.AllowRenew && u.PlanID == plan.ID
}

// CanUserRenewPlan is true when the user may renew their *current* plan off the public shop list.
func CanUserRenewPlan(u *models.User, plan *models.Plan) bool {
	if u == nil || plan == nil || !plan.Enable || !plan.AllowRenew {
		return false
	}
	if u.PlanID == 0 || u.PlanID != plan.ID {
		return false
	}
	// Off-shop renew is the intended product path; if still on shop, renew is still allowed
	// (dashboard can show “续费” for current plan too).
	return true
}

// HasFreePlanClaim reports whether this verified email already claimed any free plan.
func HasFreePlanClaim(email string) bool {
	email = NormalizeEmail(email)
	if email == "" {
		return false
	}
	var n int64
	database.DB.Model(&models.FreePlanClaim{}).Where("email = ?", email).Count(&n)
	return n > 0
}

// HasPaidFreeOrderForUser is order-audit helper: any paid free order for user_id.
func HasPaidFreeOrderForUser(userID uint) bool {
	var n int64
	database.DB.Model(&models.Order{}).
		Where("user_id = ? AND status = ? AND total_amount <= 0", userID, models.OrderPaid).
		Count(&n)
	return n > 0
}

// assertFreePlanAllowed enforces dual protection before creating/checking out free plans.
func assertFreePlanAllowed(user *models.User, plan *models.Plan) error {
	if plan == nil || plan.Price > 0 {
		return nil
	}
	if user == nil {
		return ErrOrderNotFound
	}
	email := NormalizeEmail(user.Email)
	// 1) Email-level claim (survives re-register if same email)
	if HasFreePlanClaim(email) {
		return ErrFreeAlreadyClaimed
	}
	// 2) Order audit: this account already paid a free order
	if HasPaidFreeOrderForUser(user.ID) {
		return ErrFreeAlreadyClaimed
	}
	// 3) Still within active entitlement window — no free re-buy / traffic reset
	if UserHasActiveService(user, time.Now().Unix()) {
		return ErrFreeWhileActive
	}
	return nil
}

// recordFreePlanClaim upserts claim after successful free fulfill (idempotent per email).
func recordFreePlanClaim(tx *gorm.DB, user *models.User, order *models.Order) error {
	if user == nil || order == nil || order.TotalAmount > 0 {
		return nil
	}
	email := NormalizeEmail(user.Email)
	if email == "" {
		return nil
	}
	var existing models.FreePlanClaim
	err := tx.Where("email = ?", email).First(&existing).Error
	if err == nil {
		// already claimed — keep first claim for audit
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	claim := models.FreePlanClaim{
		Email:     email,
		UserID:    user.ID,
		PlanID:    order.PlanID,
		TradeNo:   order.TradeNo,
		ClaimedAt: time.Now(),
	}
	return tx.Create(&claim).Error
}

// SettingValue returns a settings row value or empty string.
func SettingValue(key string) string {
	if database.DB == nil {
		return ""
	}
	var s models.Setting
	if err := database.DB.Where("key = ?", key).First(&s).Error; err != nil {
		return ""
	}
	return strings.TrimSpace(s.Value)
}

// PublicBaseURL is the panel public origin used for payment callbacks.
func PublicBaseURL() string {
	return strings.TrimRight(SettingValue("site_url"), "/")
}

type OrderService struct{}

func NewOrderService() *OrderService { return &OrderService{} }

func genTradeNo() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return fmt.Sprintf("K2%s%s", time.Now().Format("20060102150405"), hex.EncodeToString(b))
}

// CreateOrder builds a pending order from the live plan price (server-side).
// Anti-abuse: max pending per user, min interval between creates, expire stale first.
// Free plans (price<=0): dual guard — verified-email claim + not while service active.
func (s *OrderService) CreateOrder(userID, planID uint) (*models.Order, error) {
	var plan models.Plan
	if err := database.DB.First(&plan, planID).Error; err != nil {
		return nil, ErrPlanNotForSale
	}

	var user models.User
	if err := database.DB.First(&user, userID).Error; err != nil {
		return nil, ErrOrderNotFound
	}

	// New sale (show_on_shop) OR renew for current holders (allow_renew && user.plan_id == plan.id)
	if !CanUserPurchasePlan(&user, &plan) {
		return nil, ErrPlanNotForSale
	}
	// Selling path still needs group with ≥1 enabled node
	if err := ValidatePlanForShop(true, plan.GroupID); err != nil {
		return nil, ErrPlanNotForSale
	}

	if err := assertFreePlanAllowed(&user, &plan); err != nil {
		return nil, err
	}

	// Expire this user's overdue pending orders first (keeps caps meaningful)
	s.expirePendingForUser(userID)

	// Min interval: last create within N seconds (any status)
	var last models.Order
	if err := database.DB.Where("user_id = ?", userID).Order("id DESC").First(&last).Error; err == nil {
		if time.Since(last.CreatedAt) < minCreateInterval {
			return nil, ErrCreateTooFast
		}
	}
	// err == not found is fine (first order)

	// Cap concurrent pending
	var pendingCount int64
	database.DB.Model(&models.Order{}).
		Where("user_id = ? AND status = ?", userID, models.OrderPending).
		Count(&pendingCount)
	if pendingCount >= maxPendingOrdersPerUser {
		return nil, ErrTooManyPending
	}

	currency := plan.Currency
	if currency == "" {
		currency = "CNY"
	}

	now := time.Now()
	order := &models.Order{
		TradeNo:      genTradeNo(),
		UserID:       userID,
		PlanID:       plan.ID,
		PlanName:     plan.Name,
		GroupID:      plan.GroupID,
		Duration:     plan.Duration,
		TrafficLimit: plan.TrafficLimit,
		SpeedLimit:   plan.SpeedLimit,
		DeviceLimit:  plan.DeviceLimit,
		TotalAmount:  plan.Price,
		Currency:     currency,
		Status:       models.OrderPending,
		ExpiredAt:    now.Add(defaultOrderTTL),
	}
	if err := database.DB.Create(order).Error; err != nil {
		return nil, err
	}
	return order, nil
}

// expirePendingForUser marks expired pending orders as cancelled (batch).
func (s *OrderService) expirePendingForUser(userID uint) {
	database.DB.Model(&models.Order{}).
		Where("user_id = ? AND status = ? AND expired_at < ?", userID, models.OrderPending, time.Now()).
		Updates(map[string]any{
			"status": models.OrderCancelled,
			"remark": "auto-expired",
		})
}

// GetByTradeNo loads an order by trade number.
func (s *OrderService) GetByTradeNo(tradeNo string) (*models.Order, error) {
	var o models.Order
	if err := database.DB.Where("trade_no = ?", tradeNo).First(&o).Error; err != nil {
		return nil, ErrOrderNotFound
	}
	s.expireIfNeeded(&o)
	return &o, nil
}

// GetByID loads by primary key.
func (s *OrderService) GetByID(id uint) (*models.Order, error) {
	var o models.Order
	if err := database.DB.First(&o, id).Error; err != nil {
		return nil, ErrOrderNotFound
	}
	s.expireIfNeeded(&o)
	return &o, nil
}

func (s *OrderService) expireIfNeeded(o *models.Order) {
	if o.Status != models.OrderPending || !time.Now().After(o.ExpiredAt) {
		return
	}
	// CAS: only cancel if still pending (never clobber paid)
	res := database.DB.Model(&models.Order{}).
		Where("id = ? AND status = ?", o.ID, models.OrderPending).
		Updates(map[string]any{
			"status": models.OrderCancelled,
			"remark": "auto-expired",
		})
	if res.Error == nil && res.RowsAffected > 0 {
		o.Status = models.OrderCancelled
		o.Remark = "auto-expired"
		// Best-effort remote close (e.g. alipay.trade.close)
		s.tryCloseRemote(context.Background(), o)
	} else {
		// concurrent pay may have won — reload status
		var cur models.Order
		if database.DB.Select("status, remark").First(&cur, o.ID).Error == nil {
			o.Status = cur.Status
			o.Remark = cur.Remark
		}
	}
}

// tryCloseRemote asks the payment gateway to close an unpaid remote trade (optional).
func (s *OrderService) tryCloseRemote(ctx context.Context, order *models.Order) {
	if order == nil || order.PaymentMethod == "" || order.PaymentMethod == "mock" || order.PaymentMethod == "free" {
		return
	}
	var pm models.PaymentMethod
	if err := database.DB.Where("code = ?", order.PaymentMethod).First(&pm).Error; err != nil {
		return
	}
	gw, ok := payment.Get(order.PaymentMethod)
	if !ok {
		return
	}
	closer, ok := payment.AsCloser(gw)
	if !ok {
		return
	}
	if err := closer.ClosePayment(ctx, order, pm.Config); err != nil {
		slog.Warn("remote trade close failed", "trade_no", order.TradeNo, "method", order.PaymentMethod, "error", err)
	}
}

// ListByUser returns recent orders for a user.
func (s *OrderService) ListByUser(userID uint, limit int) ([]models.Order, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var list []models.Order
	err := database.DB.Where("user_id = ?", userID).Order("id DESC").Limit(limit).Find(&list).Error
	for i := range list {
		s.expireIfNeeded(&list[i])
	}
	return list, err
}

// AdminList returns paginated orders with optional status filter.
func (s *OrderService) AdminList(status string, page, pageSize int) ([]models.Order, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	q := database.DB.Model(&models.Order{})
	if status != "" {
		q = q.Where("status = ?", status)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []models.Order
	err := q.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&list).Error
	if err != nil {
		return nil, 0, err
	}
	// attach emails
	if len(list) > 0 {
		ids := make([]uint, 0, len(list))
		for _, o := range list {
			ids = append(ids, o.UserID)
		}
		var users []models.User
		database.DB.Select("id, email").Where("id IN ?", ids).Find(&users)
		em := map[uint]string{}
		for _, u := range users {
			em[u.ID] = u.Email
		}
		for i := range list {
			list[i].UserEmail = em[list[i].UserID]
			s.expireIfNeeded(&list[i])
		}
	}
	return list, total, nil
}

// CloseOrder cancels a pending order (admin).
func (s *OrderService) CloseOrder(id uint) error {
	var order models.Order
	if err := database.DB.First(&order, id).Error; err != nil {
		return ErrOrderNotFound
	}
	res := database.DB.Model(&models.Order{}).
		Where("id = ? AND status = ?", id, models.OrderPending).
		Updates(map[string]any{"status": models.OrderCancelled, "remark": "closed by admin"})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrOrderStatus
	}
	order.Status = models.OrderCancelled
	order.Remark = "closed by admin"
	s.tryCloseRemote(context.Background(), &order)
	return nil
}

// CancelByUser cancels a pending order owned by userID.
// Does not notify external payment gateways (no public cancel API for GMPay).
// Remark is "closed by user" so late payment callbacks do NOT auto-recover
// (unlike auto-expired).
func (s *OrderService) CancelByUser(tradeNo string, userID uint) (*models.Order, error) {
	var order models.Order
	if err := database.DB.Where("trade_no = ?", tradeNo).First(&order).Error; err != nil {
		return nil, ErrOrderNotFound
	}
	if order.UserID != userID {
		return nil, ErrOrderNotFound
	}
	// Expire first if past deadline so status is consistent
	s.expireIfNeeded(&order)
	if order.Status == models.OrderCancelled {
		return &order, nil // idempotent
	}
	if order.Status != models.OrderPending {
		return nil, ErrOrderStatus
	}
	res := database.DB.Model(&models.Order{}).
		Where("id = ? AND status = ? AND user_id = ?", order.ID, models.OrderPending, userID).
		Updates(map[string]any{"status": models.OrderCancelled, "remark": "closed by user"})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		// concurrent pay/cancel — reload
		_ = database.DB.Where("trade_no = ?", tradeNo).First(&order).Error
		if order.Status == models.OrderCancelled {
			return &order, nil
		}
		if order.Status == models.OrderPaid {
			return &order, ErrAlreadyPaid
		}
		return nil, ErrOrderStatus
	}
	order.Status = models.OrderCancelled
	order.Remark = "closed by user"
	// Best-effort close remote unpaid trade (alipay); epusdt has no public close API
	s.tryCloseRemote(context.Background(), &order)
	return &order, nil
}

// Checkout starts payment for a pending order owned by userID (IDOR-safe).
// returnURL is optional; only same-host as site_url is accepted (open-redirect guard).
func (s *OrderService) Checkout(ctx context.Context, tradeNo string, userID uint, methodCode, returnURL, clientIP string) (*payment.PaymentIntent, *models.Order, error) {
	order, err := s.GetByTradeNo(tradeNo)
	if err != nil {
		return nil, nil, err
	}
	if order.UserID != userID {
		return nil, nil, ErrOrderNotFound
	}
	if order.Status == models.OrderPaid {
		return nil, order, ErrAlreadyPaid
	}
	if order.Status != models.OrderPending {
		return nil, order, ErrOrderStatus
	}
	if time.Now().After(order.ExpiredAt) {
		s.expireIfNeeded(order)
		return nil, order, ErrOrderExpired
	}

	// Free order: re-check dual guard then auto complete (no payment gateway)
	if order.TotalAmount <= 0 {
		var user models.User
		if err := database.DB.First(&user, userID).Error; err != nil {
			return nil, order, ErrOrderNotFound
		}
		// Snapshot plan price already 0; still enforce claim/active rules at checkout
		pseudo := &models.Plan{Price: 0}
		if err := assertFreePlanAllowed(&user, pseudo); err != nil {
			return nil, order, err
		}
		if err := s.MarkPaidAndFulfill(order.ID, "free", "free-0"); err != nil {
			return nil, order, err
		}
		order, _ = s.GetByTradeNo(tradeNo)
		return &payment.PaymentIntent{
			Type:     payment.IntentCompleted,
			TradeNo:  order.TradeNo,
			Amount:   0,
			Currency: order.Currency,
			Message:  "免费套餐已开通",
		}, order, nil
	}

	// Mock payment permanently disabled (security: was free fulfillment when enabled)
	if methodCode == "mock" || payment.BaseCode(methodCode) == "mock" {
		return nil, order, ErrMethodDisabled
	}

	var pm models.PaymentMethod
	if err := database.DB.Where("code = ? AND enable = ?", methodCode, true).First(&pm).Error; err != nil {
		return nil, order, ErrMethodDisabled
	}
	// Never list/use residual mock rows even if enable was flipped in DB
	if pm.Code == "mock" {
		return nil, order, ErrMethodDisabled
	}
	gw, ok := payment.Get(methodCode)
	if !ok {
		return nil, order, ErrMethodDisabled
	}

	database.DB.Model(order).Update("payment_method", methodCode)
	order.PaymentMethod = methodCode

	opts := payment.CreateOptions{
		ClientIP: strings.TrimSpace(clientIP),
	}
	// Gateways that need public callbacks (e.g. bepusdt)
	if methodCode != "mock" {
		base := PublicBaseURL()
		if base == "" {
			return nil, order, fmt.Errorf("%w: 请在系统设置中填写 site_url（面板公网地址）", ErrSiteURL)
		}
		opts.NotifyURL = base + "/api/v1/payment/notify/" + methodCode
		safeReturn := sanitizeReturnURL(returnURL, base)
		if safeReturn != "" {
			opts.RedirectURL = safeReturn
		} else {
			// Default: hash-mode user portal order result on same host.
			// Use tn= (not trade_no=) so epay/frog return appends of platform trade_no do not clobber merchant id.
			opts.RedirectURL = base + "/#/user/order-result?tn=" + url.QueryEscape(order.TradeNo)
		}
	}

	intent, err := gw.CreatePayment(ctx, order, pm.Config, opts)
	if err != nil {
		// giftcard: platform already paid (lost notify) — try reconcile once.
		// Error prefix is fixed by giftcard gateway: "giftcard: already_paid:"
		if methodCode == "giftcard" && strings.Contains(err.Error(), "giftcard: already_paid:") {
			ord, rerr := s.ReconcileFromGateway(ctx, order.TradeNo, userID)
			if rerr != nil {
				return nil, order, rerr
			}
			if ord.Status != models.OrderPaid {
				return nil, ord, fmt.Errorf("giftcard: already_paid but order not paid yet; try sync later")
			}
			return &payment.PaymentIntent{
				Type:     payment.IntentCompleted,
				TradeNo:  ord.TradeNo,
				Amount:   ord.TotalAmount,
				Currency: ord.Currency,
				Message:  "支付已同步",
			}, ord, nil
		}
		return nil, order, err
	}

	// Persist gateway extras for ops / support
	if intent != nil && intent.Extra != nil {
		if b, e := json.Marshal(intent.Extra); e == nil {
			database.DB.Model(order).Update("meta", string(b))
			order.Meta = string(b)
		}
	}

	return intent, order, nil
}

// sanitizeReturnURL allows absolute return URLs whose host is site_url or any
// origin in allowed_origins (shadow portals), or site-relative paths resolved
// against site_url. Blocks open redirects to unlisted hosts.
func sanitizeReturnURL(raw, siteBase string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || siteBase == "" {
		return ""
	}
	base, err := url.Parse(siteBase)
	if err != nil || base.Host == "" {
		return ""
	}
	// Relative path → always on primary site_url (safe default)
	if strings.HasPrefix(raw, "/") && !strings.HasPrefix(raw, "//") {
		return strings.TrimRight(siteBase, "/") + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return ""
	}
	// Primary site host OR allow-listed shadow portal hosts
	if !IsReturnURLHostAllowed(u.Host, siteBase) {
		slog.Warn("checkout return_url rejected (host not allow-listed)",
			"got", u.Host, "site", base.Host)
		return ""
	}
	// Strip userinfo to prevent https://user:pass@host tricks in redirects
	u.User = nil
	return u.String()
}

// loadOrderForNotify loads by trade_no WITHOUT auto-cancelling expired pending orders.
// Late payment callbacks must still be able to fulfill.
func (s *OrderService) loadOrderForNotify(tradeNo string) (*models.Order, error) {
	var o models.Order
	if err := database.DB.Where("trade_no = ?", tradeNo).First(&o).Error; err != nil {
		return nil, ErrOrderNotFound
	}
	return &o, nil
}

// ProcessNotify handles an async payment callback for the given gateway code.
// Waiting/timeout notifies return ack=true without fulfilling.
func (s *OrderService) ProcessNotify(ctx context.Context, methodCode string, headers map[string]string, body []byte) (ack bool, err error) {
	// Never accept public notify for mock
	if methodCode == "mock" {
		return false, fmt.Errorf("mock notify disabled")
	}

	var pm models.PaymentMethod
	// Allow notify even if method temporarily disabled (order already in flight)
	if err := database.DB.Where("code = ?", methodCode).First(&pm).Error; err != nil {
		return false, ErrMethodDisabled
	}
	gw, ok := payment.Get(methodCode)
	if !ok {
		return false, ErrMethodDisabled
	}

	result, err := gw.HandleNotify(ctx, headers, body, pm.Config)
	if err != nil {
		return false, err
	}
	if result == nil || result.TradeNo == "" {
		return false, fmt.Errorf("empty notify result")
	}

	order, err := s.loadOrderForNotify(result.TradeNo)
	if err != nil {
		return false, err
	}

	// Paid but not fulfilled — resume fulfill (do not early-ACK only)
	if order.Status == models.OrderPaid {
		if order.FulfilledAt == nil {
			if err := s.MarkPaidAndFulfill(order.ID, methodCode, result.CallbackNo); err != nil {
				return false, err
			}
			s.AfterFulfill()
		}
		return true, nil
	}

	if !result.Success {
		return true, nil
	}

	// User-cancelled: never auto-fulfill; ACK so gateway stops retrying (ops may manual补单)
	if order.Status == models.OrderCancelled && order.Remark == "closed by user" {
		slog.Warn("notify ignored: order closed by user",
			"trade_no", order.TradeNo, "method", methodCode, "callback", result.CallbackNo)
		return true, nil
	}

	// Strict amount check for non-zero orders (fail closed)
	if order.TotalAmount > 0 {
		if result.PaidAmount != order.TotalAmount {
			slog.Warn("payment amount mismatch",
				"trade_no", order.TradeNo,
				"expected", order.TotalAmount,
				"got", result.PaidAmount,
			)
			return false, ErrAmountMismatch
		}
	}

	// Allow recover from auto-expired cancel when money actually arrived
	if err := s.MarkPaidAndFulfill(order.ID, methodCode, result.CallbackNo); err != nil {
		return false, err
	}
	s.AfterFulfill()
	return true, nil
}

// ConfirmMockPay is permanently disabled. Mock payment was removed to prevent
// free plan fulfillment via a leftover enable flag or public API abuse.
func (s *OrderService) ConfirmMockPay(_ string, _ uint) (*models.Order, error) {
	return nil, ErrMethodDisabled
}

// MarkPaidAndFulfill transitions to paid and applies plan (idempotent).
// Allows pending, paid-unfulfilled, and auto-expired cancelled recovery.
// Never recovers "closed by user" except admin manual mark-paid.
func (s *OrderService) MarkPaidAndFulfill(orderID uint, method, callbackNo string) error {
	return database.DB.Transaction(func(tx *gorm.DB) error {
		var order models.Order
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&order, orderID).Error; err != nil {
			return ErrOrderNotFound
		}
		if order.Status == models.OrderPaid && order.FulfilledAt != nil {
			return nil
		}

		adminManual := method == "manual" || method == "admin-manual" || strings.HasPrefix(callbackNo, "admin")
		// User cancelled: only admin can reopen
		if order.Status == models.OrderCancelled && order.Remark == "closed by user" && !adminManual {
			return ErrOrderStatus
		}
		// Recover auto-expired when money arrived, or any cancel via admin
		recoverableCancel := order.Status == models.OrderCancelled &&
			(order.Remark == "auto-expired" || adminManual)
		if order.Status != models.OrderPending && order.Status != models.OrderPaid && !recoverableCancel {
			return ErrOrderStatus
		}

		now := time.Now()
		if order.Status != models.OrderPaid {
			updates := map[string]any{
				"status":         models.OrderPaid,
				"paid_at":        now,
				"payment_method": method,
				"callback_no":    callbackNo,
				"remark":         "",
			}
			if err := tx.Model(&order).Updates(updates).Error; err != nil {
				return err
			}
			order.Status = models.OrderPaid
			order.PaidAt = &now
			order.PaymentMethod = method
		} else if callbackNo != "" && order.CallbackNo == "" {
			_ = tx.Model(&order).Update("callback_no", callbackNo)
		}

		if order.FulfilledAt != nil {
			return nil
		}

		if err := fulfillUser(tx, &order); err != nil {
			return err
		}
		return tx.Model(&order).Update("fulfilled_at", now).Error
	})
}

func fulfillUser(tx *gorm.DB, order *models.Order) error {
	var user models.User
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&user, order.UserID).Error; err != nil {
		return err
	}

	// Free fulfill: enforce dual guard again inside tx (race-safe with claim write)
	if order.TotalAmount <= 0 {
		if err := assertFreePlanAllowedTx(tx, &user, order); err != nil {
			return err
		}
	}

	now := time.Now().Unix()
	var newExpire int64
	if order.Duration > 0 {
		base := now
		if user.ExpireAt > now {
			base = user.ExpireAt
		}
		newExpire = base + order.Duration
	} else {
		newExpire = 0 // permanent
	}

	// Always apply group_id from snapshot (including 0 to clear VIP group on downgrade).
	// Zero traffic on purchase and stamp last_traffic_reset_at so the same-day
	// monthly job does not thrash if reset_day == today.
	//
	// Do NOT force enable=true: enable is admin ban/risk only. Forcing it here
	// would let a banned user with a pre-ban pending order unban themselves via pay.
	// Expired-but-enabled users already have enable=true and can renew normally.
	updates := map[string]any{
		"plan_id":               order.PlanID,
		"group_id":              order.GroupID,
		"traffic_used":          0,
		"last_traffic_reset_at": now,
		"expire_at":             newExpire,
		"traffic_limit":         order.TrafficLimit,
		"speed_limit":           order.SpeedLimit,
		"device_limit":          order.DeviceLimit,
	}

	if err := tx.Model(&user).Updates(updates).Error; err != nil {
		return err
	}

	if order.TotalAmount <= 0 {
		if err := recordFreePlanClaim(tx, &user, order); err != nil {
			return err
		}
	}

	// Affiliate commission (inviter of buyer); free orders skipped inside
	if err := RecordCommissionOnFulfill(tx, order); err != nil {
		return err
	}

	slog.Info("order fulfilled",
		"trade_no", order.TradeNo,
		"user_id", order.UserID,
		"plan_id", order.PlanID,
		"group_id", order.GroupID,
		"expire_at", newExpire,
	)
	return nil
}

// assertFreePlanAllowedTx is the in-transaction version used at fulfill time.
// Skips "active service" check when this is the first free fulfill path for a blank user;
// still blocks email claim and prior free paid orders (using tx for claim lookup).
func assertFreePlanAllowedTx(tx *gorm.DB, user *models.User, order *models.Order) error {
	if user == nil || order == nil || order.TotalAmount > 0 {
		return nil
	}
	email := NormalizeEmail(user.Email)
	var n int64
	if email != "" {
		tx.Model(&models.FreePlanClaim{}).Where("email = ?", email).Count(&n)
		if n > 0 {
			return ErrFreeAlreadyClaimed
		}
	}
	tx.Model(&models.Order{}).
		Where("user_id = ? AND status = ? AND total_amount <= 0 AND id <> ?", user.ID, models.OrderPaid, order.ID).
		Count(&n)
	if n > 0 {
		return ErrFreeAlreadyClaimed
	}
	// Active service: only block if user already has plan/group from a *previous* entitlement
	// (not this order's fulfill). Before updates, user still has old state.
	if UserHasActiveService(user, time.Now().Unix()) {
		return ErrFreeWhileActive
	}
	return nil
}

// After successful fulfill outside tx
func (s *OrderService) AfterFulfill() {
	BumpConfigVersion()
}

// ReconcileFromGateway queries the payment provider and fulfills if paid.
// userID > 0 enforces ownership (user API); userID == 0 is admin/system.
func (s *OrderService) ReconcileFromGateway(ctx context.Context, tradeNo string, userID uint) (*models.Order, error) {
	order, err := s.loadOrderForNotify(tradeNo)
	if err != nil {
		return nil, err
	}
	if userID > 0 && order.UserID != userID {
		return nil, ErrOrderNotFound
	}
	// Already paid — ensure fulfilled
	if order.Status == models.OrderPaid {
		if order.FulfilledAt == nil {
			if err := s.MarkPaidAndFulfill(order.ID, order.PaymentMethod, order.CallbackNo); err != nil {
				return nil, err
			}
			s.AfterFulfill()
		}
		return order, nil
	}

	// Only pending or auto-expired can be recovered via gateway query
	canQuery := order.Status == models.OrderPending ||
		(order.Status == models.OrderCancelled && order.Remark == "auto-expired")
	if !canQuery {
		return order, nil
	}

	method := order.PaymentMethod
	if method == "" || method == "mock" || method == "free" {
		s.expireIfNeeded(order)
		_ = database.DB.Where("trade_no = ?", tradeNo).First(order).Error
		return order, nil
	}

	var pm models.PaymentMethod
	if err := database.DB.Where("code = ?", method).First(&pm).Error; err != nil {
		return order, ErrMethodDisabled
	}
	gw, ok := payment.Get(method)
	if !ok {
		return order, ErrMethodDisabled
	}

	qr, err := gw.QueryPayment(ctx, order, pm.Config)
	if err != nil {
		return order, err
	}
	if qr == nil || !qr.Paid {
		s.expireIfNeeded(order)
		_ = database.DB.Where("trade_no = ?", tradeNo).First(order).Error
		return order, nil
	}
	if order.TotalAmount > 0 && qr.PaidAmount > 0 && qr.PaidAmount != order.TotalAmount {
		slog.Warn("reconcile amount mismatch", "trade_no", order.TradeNo, "expected", order.TotalAmount, "got", qr.PaidAmount)
		return order, ErrAmountMismatch
	}
	cb := qr.CallbackNo
	if cb == "" {
		cb = "query-" + order.TradeNo
	}
	if err := s.MarkPaidAndFulfill(order.ID, method, cb); err != nil {
		return order, err
	}
	s.AfterFulfill()
	_ = database.DB.Where("trade_no = ?", tradeNo).First(order).Error
	return order, nil
}

// ReconcileByID admin/system entry.
func (s *OrderService) ReconcileByID(ctx context.Context, id uint) (*models.Order, error) {
	// Load without expireIfNeeded side effects for auto-expired recovery path
	var raw models.Order
	if err := database.DB.First(&raw, id).Error; err != nil {
		return nil, ErrOrderNotFound
	}
	return s.ReconcileFromGateway(ctx, raw.TradeNo, 0)
}

// ReconcileStalePending queries gateways for recent unpaid orders (notify fallback).
// Returns number of orders newly fulfilled.
func (s *OrderService) ReconcileStalePending(ctx context.Context, limit int) int {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	// Pending with a real gateway, created at least 1 minute ago (give notify time)
	cutoff := time.Now().Add(-1 * time.Minute)
	var list []models.Order
	database.DB.Where(
		"status = ? AND payment_method <> '' AND payment_method NOT IN ? AND created_at < ? AND expired_at > ?",
		models.OrderPending,
		[]string{"mock", "free"},
		cutoff,
		time.Now(), // still within pay window or just after — also try auto-expired
	).Order("id ASC").Limit(limit).Find(&list)

	// Also try auto-expired cancelled that might have been paid late (within 24h)
	var late []models.Order
	database.DB.Where(
		"status = ? AND remark = ? AND payment_method <> '' AND payment_method NOT IN ? AND updated_at > ?",
		models.OrderCancelled,
		"auto-expired",
		[]string{"mock", "free"},
		time.Now().Add(-24*time.Hour),
	).Order("id DESC").Limit(limit / 2).Find(&late)
	list = append(list, late...)

	fulfilled := 0
	seen := map[uint]bool{}
	for i := range list {
		o := &list[i]
		if seen[o.ID] {
			continue
		}
		seen[o.ID] = true
		updated, err := s.ReconcileFromGateway(ctx, o.TradeNo, 0)
		if err != nil {
			slog.Debug("reconcile skip", "trade_no", o.TradeNo, "error", err)
			continue
		}
		if updated != nil && updated.Status == models.OrderPaid {
			fulfilled++
		}
	}
	return fulfilled
}

// ListEnabledMethods for user checkout (no config secrets).
// Mock is never returned even if a residual DB row is still enable=true.
func (s *OrderService) ListEnabledMethods() ([]models.PaymentMethod, error) {
	var list []models.PaymentMethod
	err := database.DB.Where("enable = ? AND code <> ?", true, "mock").Order("sort ASC, id ASC").Find(&list).Error
	// strip config for safety when returning to callers who re-serialize
	for i := range list {
		list[i].Config = ""
	}
	return list, err
}
