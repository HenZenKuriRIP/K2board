package admin

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"K2board/internal/database"
	"K2board/internal/models"
	"K2board/internal/payment"
	_ "K2board/internal/payment/gateways"
	"K2board/internal/services"
	"K2board/internal/utils"
)

// methodCodeRe: frog | frog_alipay | frog_wx01
var methodCodeRe = regexp.MustCompile(`^[a-z][a-z0-9]*(_[a-z0-9]+)?$`)

type PaymentHandler struct {
	orders *services.OrderService
}

func NewPaymentHandler() *PaymentHandler {
	return &PaymentHandler{orders: services.NewOrderService()}
}

// ListMethods GET /admin/payment-methods
func (h *PaymentHandler) ListMethods(c *gin.Context) {
	var list []models.PaymentMethod
	if err := database.DB.Order("sort ASC, id ASC").Find(&list).Error; err != nil {
		utils.InternalError(c, "failed to list payment methods")
		return
	}
	// mask secrets lightly: keep config for admin edit, but empty if huge
	utils.Success(c, list)
}

// CreateMethod POST /admin/payment-methods
func (h *PaymentHandler) CreateMethod(c *gin.Context) {
	var req models.PaymentMethod
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "invalid request")
		return
	}
	req.Code = strings.ToLower(strings.TrimSpace(req.Code))
	req.Name = strings.TrimSpace(req.Name)
	if req.Code == "" || req.Name == "" {
		utils.BadRequest(c, "code and name required")
		return
	}
	if !methodCodeRe.MatchString(req.Code) {
		utils.BadRequest(c, "code 格式无效：仅小写字母数字，多通道用下划线，如 frog_alipay / frog_wx")
		return
	}
	if payment.BaseCode(req.Code) == "mock" || req.Code == "mock" {
		utils.BadRequest(c, "模拟支付已永久移除，不可添加")
		return
	}
	if _, ok := payment.Get(req.Code); !ok {
		utils.BadRequest(c, "unknown gateway code: "+req.Code+" (use frog, frog_alipay, frog_wx, epay, …)")
		return
	}
	if req.Config == "" {
		req.Config = "{}"
	}
	if err := database.DB.Create(&req).Error; err != nil {
		utils.BadRequest(c, "create failed (code may already exist)")
		return
	}
	utils.Created(c, &req)
}

// UpdateMethod PUT /admin/payment-methods/:id
func (h *PaymentHandler) UpdateMethod(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		utils.BadRequest(c, "invalid id")
		return
	}
	var req map[string]any
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "invalid request")
		return
	}
	// code immutable if present
	delete(req, "code")
	delete(req, "id")
	// Refuse re-enabling residual mock rows
	var existing models.PaymentMethod
	if database.DB.First(&existing, uint(id)).Error == nil && existing.Code == "mock" {
		utils.BadRequest(c, "模拟支付已永久移除，请删除该条记录")
		return
	}
	if err := database.DB.Model(&models.PaymentMethod{}).Where("id = ?", uint(id)).Updates(req).Error; err != nil {
		utils.InternalError(c, "update failed")
		return
	}
	var m models.PaymentMethod
	database.DB.First(&m, id)
	utils.Success(c, &m)
}

// DeleteMethod DELETE /admin/payment-methods/:id
func (h *PaymentHandler) DeleteMethod(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		utils.BadRequest(c, "invalid id")
		return
	}
	var m models.PaymentMethod
	if database.DB.First(&m, id).Error != nil {
		utils.NotFound(c, "not found")
		return
	}
	database.DB.Delete(&m)
	utils.SuccessMessage(c, "deleted")
}

// GatewayCodes GET /admin/payment-methods/gateways
func (h *PaymentHandler) GatewayCodes(c *gin.Context) {
	codes := payment.ListCodes()
	filtered := make([]string, 0, len(codes))
	multi := make([]string, 0, 4)
	for _, c0 := range codes {
		if c0 == "mock" {
			continue
		}
		filtered = append(filtered, c0)
		if payment.MultiInstance(c0) {
			multi = append(multi, c0)
		}
	}
	utils.Success(c, gin.H{
		"codes":          filtered,
		"multi_instance": multi, // 可重复添加：frog / epay
	})
}

// ListOrders GET /admin/orders
func (h *PaymentHandler) ListOrders(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	status := c.Query("status")
	list, total, err := h.orders.AdminList(status, page, pageSize)
	if err != nil {
		utils.InternalError(c, "failed to list orders")
		return
	}
	utils.Success(c, gin.H{"list": list, "total": total, "page": page, "page_size": pageSize})
}

// CloseOrder POST /admin/orders/:id/close
func (h *PaymentHandler) CloseOrder(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		utils.BadRequest(c, "invalid id")
		return
	}
	if err := h.orders.CloseOrder(uint(id)); err != nil {
		utils.BadRequest(c, err.Error())
		return
	}
	utils.SuccessMessage(c, "closed")
}

// MarkPaid POST /admin/orders/:id/mark-paid
// Recovers pending, paid-unfulfilled, and auto-expired cancelled orders.
func (h *PaymentHandler) MarkPaid(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		utils.BadRequest(c, "invalid id")
		return
	}
	if err := h.orders.MarkPaidAndFulfill(uint(id), "manual", "admin-manual"); err != nil {
		utils.BadRequest(c, err.Error())
		return
	}
	h.orders.AfterFulfill()
	o, _ := h.orders.GetByID(uint(id))
	utils.Success(c, o)
}

// SyncOrder POST /admin/orders/:id/sync
// Pulls status from payment gateway (alipay.trade.query / epusdt check-status) and fulfills if paid.
func (h *PaymentHandler) SyncOrder(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		utils.BadRequest(c, "invalid id")
		return
	}
	o, err := h.orders.ReconcileByID(c.Request.Context(), uint(id))
	if err != nil {
		utils.BadRequest(c, err.Error())
		return
	}
	utils.Success(c, o)
}
