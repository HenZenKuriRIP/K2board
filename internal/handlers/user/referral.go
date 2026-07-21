package user

import (
	"errors"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"K2board/internal/database"
	"K2board/internal/models"
	"K2board/internal/services"
	"K2board/internal/utils"
)

// Uses resolveUserByToken from order.go for GET endpoints.

// GetReferral overview: invite link, balance, rate, methods.
// GET /api/v1/user/referral?token=
func GetReferral(c *gin.Context) {
	user, ok := resolveUserByToken(c)
	if !ok {
		return
	}
	ov, err := services.GetReferralOverview(user.ID)
	if err != nil {
		utils.InternalError(c, "load referral failed")
		return
	}
	utils.Success(c, ov)
}

// ListReferralLedgers GET /api/v1/user/referral/ledgers?token=&page=&page_size=
func ListReferralLedgers(c *gin.Context) {
	user, ok := resolveUserByToken(c)
	if !ok {
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	list, total, err := services.ListUserLedgers(user.ID, page, pageSize)
	if err != nil {
		utils.InternalError(c, "list failed")
		return
	}
	utils.Success(c, gin.H{"list": list, "total": total, "page": page, "page_size": pageSize})
}

// ListReferralWithdraws GET /api/v1/user/referral/withdrawals?token=
func ListReferralWithdraws(c *gin.Context) {
	user, ok := resolveUserByToken(c)
	if !ok {
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	list, total, err := services.ListUserWithdraws(user.ID, page, pageSize)
	if err != nil {
		utils.InternalError(c, "list failed")
		return
	}
	utils.Success(c, gin.H{"list": list, "total": total, "page": page, "page_size": pageSize})
}

// ListInvitees GET /api/v1/user/referral/invitees?token=
func ListInvitees(c *gin.Context) {
	user, ok := resolveUserByToken(c)
	if !ok {
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	list, total, err := services.ListInvitees(user.ID, page, pageSize)
	if err != nil {
		utils.InternalError(c, "list failed")
		return
	}
	utils.Success(c, gin.H{"list": list, "total": total, "page": page, "page_size": pageSize})
}

type withdrawReq struct {
	Token       string `json:"token"`
	Amount      int64  `json:"amount"` // cents
	Method      string `json:"method"`
	Account     string `json:"account"`
	AccountName string `json:"account_name"`
}

// CreateWithdraw POST /api/v1/user/referral/withdraw
func CreateWithdraw(c *gin.Context) {
	var req withdrawReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "请求参数无效")
		return
	}
	// Same priority as resolveUserByToken: Authorization > body > query.
	// Existing SPA sends token in JSON body — unchanged happy path.
	token := ""
	if auth := c.GetHeader("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		token = strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	}
	if token == "" {
		token = strings.TrimSpace(req.Token)
	}
	if token == "" {
		token = strings.TrimSpace(c.Query("token"))
	}
	if token == "" {
		utils.Unauthorized(c, "missing token")
		return
	}
	var user models.User
	if err := database.DB.Where("token = ?", token).First(&user).Error; err != nil {
		utils.Unauthorized(c, "invalid token")
		return
	}
	if services.IsAccountBanned(&user) {
		utils.Forbidden(c, "account is disabled")
		return
	}

	w, err := services.RequestWithdraw(user.ID, req.Amount, req.Method, req.Account, req.AccountName)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrReferralDisabled):
			utils.BadRequest(c, "推广功能未开启")
		case errors.Is(err, services.ErrWithdrawMin):
			cfg := services.LoadReferralConfig()
			utils.BadRequest(c, "低于最低提现金额 ¥"+formatYuan(cfg.MinWithdraw))
		case errors.Is(err, services.ErrWithdrawBalance):
			utils.BadRequest(c, "可提现余额不足")
		case errors.Is(err, services.ErrWithdrawMethod):
			utils.BadRequest(c, "收款方式或账号无效")
		case errors.Is(err, services.ErrWithdrawAmount):
			utils.BadRequest(c, "提现金额无效")
		case err.Error() == "too many pending withdrawals":
			utils.BadRequest(c, "待审核提现过多，请等待处理后再申请")
		default:
			utils.BadRequest(c, err.Error())
		}
		return
	}
	utils.Created(c, w)
}

func formatYuan(cents int64) string {
	return strconv.FormatFloat(float64(cents)/100, 'f', 2, 64)
}
