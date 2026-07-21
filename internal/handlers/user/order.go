package user

import (
	"errors"
	"strings"

	"github.com/gin-gonic/gin"

	"K2board/internal/database"
	"K2board/internal/models"
	"K2board/internal/services"
	"K2board/internal/utils"
)

func resolveUserByToken(c *gin.Context) (*models.User, bool) {
	// Prefer Authorization / body over query so logs are less likely to hold secrets.
	// Query remains last fallback — existing SPA and bookmarks keep working unchanged.
	token := ""
	if auth := c.GetHeader("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		token = strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	}
	if token == "" {
		var body struct {
			Token string `json:"token"`
		}
		// Best-effort: GET has no body; POST may already be bound elsewhere — ignore bind errors.
		_ = c.ShouldBindJSON(&body)
		token = strings.TrimSpace(body.Token)
	}
	if token == "" {
		token = strings.TrimSpace(c.Query("token"))
	}
	if token == "" {
		utils.Unauthorized(c, "missing token")
		return nil, false
	}
	var user models.User
	if err := database.DB.Where("token = ?", token).First(&user).Error; err != nil {
		utils.Unauthorized(c, "invalid token")
		return nil, false
	}
	// enable = admin ban only; expired users may still create/pay orders to renew
	if services.IsAccountBanned(&user) {
		utils.Forbidden(c, "account is disabled")
		return nil, false
	}
	return &user, true
}

// GetPlans lists shop-visible enabled plans.
func GetPlans(c *gin.Context) {
	var plans []models.Plan
	database.DB.Where("enable = ? AND show_on_shop = ?", true, true).
		Order("sort ASC, id ASC").Find(&plans)
	if plans == nil {
		plans = []models.Plan{}
	}
	utils.Success(c, plans)
}

// ListPaymentMethods GET /user/payment-methods
func ListPaymentMethods(c *gin.Context) {
	// public list of enabled methods (no secrets)
	svc := services.NewOrderService()
	list, err := svc.ListEnabledMethods()
	if err != nil {
		utils.InternalError(c, "failed to list methods")
		return
	}
	// only fields safe for client
	type item struct {
		Code string `json:"code"`
		Name string `json:"name"`
		Sort int    `json:"sort"`
	}
	out := make([]item, 0, len(list))
	for _, m := range list {
		out = append(out, item{Code: m.Code, Name: m.Name, Sort: m.Sort})
	}
	utils.Success(c, out)
}

// CreateOrder POST /user/orders  { token, plan_id }
func CreateOrder(c *gin.Context) {
	var req struct {
		Token  string `json:"token" binding:"required"`
		PlanID uint   `json:"plan_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "invalid request")
		return
	}
	var user models.User
	if err := database.DB.Where("token = ?", req.Token).First(&user).Error; err != nil {
		utils.Unauthorized(c, "invalid token")
		return
	}
	if services.IsAccountBanned(&user) {
		utils.Forbidden(c, "account is disabled")
		return
	}

	svc := services.NewOrderService()
	order, err := svc.CreateOrder(user.ID, req.PlanID)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrPlanNotForSale):
			utils.BadRequest(c, "套餐不可购买或不存在")
		case errors.Is(err, services.ErrTooManyPending):
			utils.Error(c, 429, "待支付订单过多，请先完成或取消后再下单（最多 3 笔）")
		case errors.Is(err, services.ErrCreateTooFast):
			utils.Error(c, 429, "下单过于频繁，请稍后再试")
		case errors.Is(err, services.ErrFreeAlreadyClaimed):
			utils.BadRequest(c, "该邮箱已领取过免费/试用套餐，无法再次领取")
		case errors.Is(err, services.ErrFreeWhileActive):
			utils.BadRequest(c, "当前套餐仍在有效期内，无法再次领取免费/试用套餐")
		default:
			utils.InternalError(c, "create order failed")
		}
		return
	}
	utils.Created(c, toUserOrderView(order))
}

// ListOrders GET /user/orders?token=
func ListOrders(c *gin.Context) {
	user, ok := resolveUserByToken(c)
	if !ok {
		return
	}
	svc := services.NewOrderService()
	list, err := svc.ListByUser(user.ID, 50)
	if err != nil {
		utils.InternalError(c, "list failed")
		return
	}
	utils.Success(c, toUserOrderViews(list))
}

// GetOrder GET /user/orders/:trade_no?token=
func GetOrder(c *gin.Context) {
	user, ok := resolveUserByToken(c)
	if !ok {
		return
	}
	tradeNo := c.Param("trade_no")
	svc := services.NewOrderService()
	order, err := svc.GetByTradeNo(tradeNo)
	if err != nil || order.UserID != user.ID {
		utils.NotFound(c, "order not found")
		return
	}
	utils.Success(c, toUserOrderView(order))
}

// Checkout POST /user/orders/:trade_no/checkout  { token, method, return_url? }
func Checkout(c *gin.Context) {
	var req struct {
		Token     string `json:"token" binding:"required"`
		Method    string `json:"method" binding:"required"`
		ReturnURL string `json:"return_url"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "invalid request")
		return
	}
	var user models.User
	if err := database.DB.Where("token = ?", req.Token).First(&user).Error; err != nil {
		utils.Unauthorized(c, "invalid token")
		return
	}
	if services.IsAccountBanned(&user) {
		utils.Forbidden(c, "account is disabled")
		return
	}

	tradeNo := c.Param("trade_no")
	svc := services.NewOrderService()
	// Ownership enforced inside Checkout (userID)
	intent, order, err := svc.Checkout(c.Request.Context(), tradeNo, user.ID, req.Method, req.ReturnURL, c.ClientIP())
	safeOrder := func(o *models.Order) any {
		if o == nil {
			return nil
		}
		return toUserOrderView(o)
	}
	if err != nil {
		msg := err.Error()
		switch {
		case errors.Is(err, services.ErrOrderNotFound):
			utils.NotFound(c, "order not found")
			return
		case errors.Is(err, services.ErrAlreadyPaid):
			utils.Success(c, gin.H{"intent": intent, "order": safeOrder(order), "message": "already paid"})
			return
		case errors.Is(err, services.ErrOrderExpired):
			msg = "订单已过期"
		case errors.Is(err, services.ErrMethodDisabled):
			msg = "支付方式不可用"
		case errors.Is(err, services.ErrOrderStatus):
			msg = "订单状态不可支付"
		case errors.Is(err, services.ErrSiteURL):
			msg = "站点地址未配置：请管理员在系统设置填写 site_url"
		case errors.Is(err, services.ErrFreeAlreadyClaimed):
			msg = "该邮箱已领取过免费/试用套餐，无法再次领取"
		case errors.Is(err, services.ErrFreeWhileActive):
			msg = "当前套餐仍在有效期内，无法再次领取免费/试用套餐"
		}
		utils.BadRequest(c, msg)
		return
	}

	// free / completed path already fulfilled inside service
	if intent != nil && intent.Type == "completed" {
		svc.AfterFulfill()
	}

	// Reload for latest meta (payment_url etc.)
	if order != nil {
		if fresh, e := svc.GetByTradeNo(tradeNo); e == nil {
			order = fresh
		}
	}
	utils.Success(c, gin.H{"intent": intent, "order": safeOrder(order)})
}

// CancelOrder POST /user/orders/:trade_no/cancel  { token }
func CancelOrder(c *gin.Context) {
	var req struct {
		Token string `json:"token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "invalid request")
		return
	}
	var user models.User
	if err := database.DB.Where("token = ?", req.Token).First(&user).Error; err != nil {
		utils.Unauthorized(c, "invalid token")
		return
	}
	if services.IsAccountBanned(&user) {
		utils.Forbidden(c, "account is disabled")
		return
	}

	tradeNo := c.Param("trade_no")
	svc := services.NewOrderService()
	order, err := svc.CancelByUser(tradeNo, user.ID)
	if err != nil {
		msg := err.Error()
		switch {
		case errors.Is(err, services.ErrOrderNotFound):
			utils.NotFound(c, "order not found")
			return
		case errors.Is(err, services.ErrAlreadyPaid):
			msg = "订单已支付，无法取消"
		case errors.Is(err, services.ErrOrderStatus):
			msg = "当前状态不可取消"
		case errors.Is(err, services.ErrOrderExpired):
			msg = "订单已过期"
		}
		utils.BadRequest(c, msg)
		return
	}
	utils.Success(c, toUserOrderView(order))
}

// SyncOrder POST /user/orders/:trade_no/sync  { token }
// Queries payment gateway (e.g. alipay.trade.query) when async notify is delayed.
func SyncOrder(c *gin.Context) {
	var req struct {
		Token string `json:"token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "invalid request")
		return
	}
	var user models.User
	if err := database.DB.Where("token = ?", req.Token).First(&user).Error; err != nil {
		utils.Unauthorized(c, "invalid token")
		return
	}
	if services.IsAccountBanned(&user) {
		utils.Forbidden(c, "account is disabled")
		return
	}
	tradeNo := c.Param("trade_no")
	svc := services.NewOrderService()
	order, err := svc.ReconcileFromGateway(c.Request.Context(), tradeNo, user.ID)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrOrderNotFound):
			utils.NotFound(c, "order not found")
		case errors.Is(err, services.ErrAmountMismatch):
			utils.BadRequest(c, "支付金额与订单不一致，请联系管理员")
		default:
			// Soft: return local order state if query fails (network)
			o, e2 := svc.GetByTradeNo(tradeNo)
			if e2 == nil && o.UserID == user.ID {
				utils.Success(c, gin.H{"order": toUserOrderView(o), "synced": false, "message": err.Error()})
				return
			}
			utils.BadRequest(c, err.Error())
		}
		return
	}
	utils.Success(c, gin.H{"order": toUserOrderView(order), "synced": true})
}

// ConfirmMockPay was POST /user/orders/:trade_no/confirm-mock — permanently removed.
// Kept as a 410 handler only if re-registered; mock cannot fulfill orders.
func ConfirmMockPay(c *gin.Context) {
	utils.Error(c, 410, "模拟支付已永久关闭")
}
