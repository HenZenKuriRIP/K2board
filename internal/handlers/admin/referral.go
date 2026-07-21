package admin

import (
	"errors"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"K2board/internal/services"
	"K2board/internal/utils"
)

type ReferralHandler struct{}

func NewReferralHandler() *ReferralHandler { return &ReferralHandler{} }

// ListWithdraws GET /api/v1/admin/referral/withdrawals
func (h *ReferralHandler) ListWithdraws(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	status := strings.TrimSpace(c.Query("status"))
	list, total, err := services.AdminListWithdraws(status, page, pageSize)
	if err != nil {
		utils.InternalError(c, "list failed")
		return
	}
	utils.Success(c, gin.H{"list": list, "total": total, "page": page, "page_size": pageSize})
}

// ApproveWithdraw POST /api/v1/admin/referral/withdrawals/:id/approve
func (h *ReferralHandler) ApproveWithdraw(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		utils.BadRequest(c, "invalid id")
		return
	}
	var body struct {
		Remark string `json:"remark"`
	}
	_ = c.ShouldBindJSON(&body)
	if err := services.AdminApproveWithdraw(uint(id), body.Remark); err != nil {
		if errors.Is(err, services.ErrWithdrawNotFound) {
			utils.BadRequest(c, "提现单不存在")
			return
		}
		if errors.Is(err, services.ErrWithdrawStatus) {
			utils.BadRequest(c, "当前状态不可审核通过")
			return
		}
		utils.InternalError(c, err.Error())
		return
	}
	utils.SuccessMessage(c, "已标记为已打款")
}

// RejectWithdraw POST /api/v1/admin/referral/withdrawals/:id/reject
func (h *ReferralHandler) RejectWithdraw(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		utils.BadRequest(c, "invalid id")
		return
	}
	var body struct {
		Remark string `json:"remark"`
	}
	_ = c.ShouldBindJSON(&body)
	if err := services.AdminRejectWithdraw(uint(id), body.Remark); err != nil {
		if errors.Is(err, services.ErrWithdrawNotFound) {
			utils.BadRequest(c, "提现单不存在")
			return
		}
		if errors.Is(err, services.ErrWithdrawStatus) {
			utils.BadRequest(c, "当前状态不可驳回")
			return
		}
		utils.InternalError(c, err.Error())
		return
	}
	utils.SuccessMessage(c, "已驳回并退回余额")
}

// ListLedgers GET /api/v1/admin/referral/ledgers
func (h *ReferralHandler) ListLedgers(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	userID64, _ := strconv.ParseUint(c.Query("user_id"), 10, 64)
	list, total, err := services.AdminListLedgers(page, pageSize, uint(userID64))
	if err != nil {
		utils.InternalError(c, "list failed")
		return
	}
	utils.Success(c, gin.H{"list": list, "total": total, "page": page, "page_size": pageSize})
}

// Config returns current referral settings snapshot (read-only convenience).
// GET /api/v1/admin/referral/config
func (h *ReferralHandler) Config(c *gin.Context) {
	cfg := services.LoadReferralConfig()
	utils.Success(c, gin.H{
		"enable":       cfg.Enable,
		"rate_percent": cfg.RatePercent,
		"min_withdraw": cfg.MinWithdraw,
		"methods":      cfg.Methods,
	})
}
