package user

import (
	"time"

	"github.com/gin-gonic/gin"

	"K2board/internal/database"
	"K2board/internal/models"
	"K2board/internal/services"
	"K2board/internal/utils"
)

// ── UserInfo ──────────────────────────────────────
// GET /api/v1/user/info?token=xxx
func GetInfo(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		utils.Unauthorized(c, "missing token")
		return
	}

	var user models.User
	if err := database.DB.Where("token = ?", token).First(&user).Error; err != nil {
		utils.Unauthorized(c, "invalid token")
		return
	}
	// enable = admin ban only; expired users may load profile to renew
	if services.IsAccountBanned(&user) {
		utils.Forbidden(c, "account is disabled")
		return
	}

	// Load group name (empty group → 未分组, never placeholder "-")
	groupName := "未分组"
	if user.GroupID > 0 {
		var g models.Group
		if database.DB.Select("name").First(&g, user.GroupID).Error == nil && g.Name != "" {
			groupName = g.Name
		}
	}

	// Traffic
	trafficUsed := user.TrafficUsed
	trafficLimit := user.TrafficLimit
	usagePercent := 0.0
	if trafficLimit > 0 {
		usagePercent = float64(trafficUsed) / float64(trafficLimit) * 100
	}

	// Expiry — new users without plan must not show「永久有效」
	nowUnix := time.Now().Unix()
	hasService := services.UserHasActiveService(&user, nowUnix)
	// Also treat expired plan holders as “had service” for UI (show renew, hide fake permanent)
	hadPlanOrGroup := user.PlanID > 0 || user.GroupID > 0

	expireText := "未开通"
	expired := false
	if hadPlanOrGroup {
		if user.ExpireAt > 0 {
			t := time.Unix(user.ExpireAt, 0)
			expireText = t.Format("2006-01-02 15:04")
			expired = time.Now().After(t)
		} else {
			expireText = "永久有效"
			expired = false
		}
	}

	// Subscribe URL (frontend should hide until has_service)
	subscribeURL := buildSubscribeURL(user.Token)

	// Monthly traffic reset from bound plan (0 = no calendar reset)
	resetDay := 0
	planName := ""
	var renewPlan any
	canRenew := false
	if user.PlanID > 0 {
		var plan models.Plan
		if database.DB.First(&plan, user.PlanID).Error == nil {
			resetDay = plan.ResetDay
			planName = plan.Name
			// 续费：当前绑定套餐且 allow_renew；组内仍须有节点（与下单一致）
			if services.CanUserRenewPlan(&user, &plan) {
				if err := services.ValidatePlanForShop(true, plan.GroupID); err == nil {
					canRenew = true
					renewPlan = gin.H{
						"id":            plan.ID,
						"name":          plan.Name,
						"group_id":      plan.GroupID,
						"duration":      plan.Duration,
						"traffic_limit": plan.TrafficLimit,
						"speed_limit":   plan.SpeedLimit,
						"device_limit":  plan.DeviceLimit,
						"price":         plan.Price,
						"currency":      plan.Currency,
						"show_on_shop":  plan.ShowOnShop,
						"allow_renew":   plan.AllowRenew,
					}
				}
			}
		}
	}

	// No plan: do not expose misleading ∞ quotas to the client
	if !hadPlanOrGroup {
		trafficUsed = 0
		trafficLimit = 0
		usagePercent = 0
	}

	// Do not echo subscribe token in /info — client already has session token from
	// login/register (localStorage). Subscription uses subscribe_url only.
	// Keeps UX unchanged; reduces XSS surface if a third-party script scrapes /info.
	utils.Success(c, gin.H{
		"id":                    user.ID,
		"email":                 user.Email,
		"uuid":                  user.UUID,
		"group_name":            groupName,
		"plan_id":               user.PlanID,
		"plan_name":             planName,
		"has_service":           hasService,
		"traffic_used":          trafficUsed,
		"traffic_limit":         trafficLimit,
		"usage_percent":         usagePercent,
		"traffic_reset_day":     resetDay,
		"last_traffic_reset_at": user.LastTrafficResetAt,
		"speed_limit":           user.SpeedLimit,
		"device_limit":          user.DeviceLimit,
		"expire_at":             user.ExpireAt,
		"expire_text":           expireText,
		"expired":               expired,
		"subscribe_url":         subscribeURL,
		"can_renew":             canRenew,
		"renew_plan":            renewPlan,
	})
}

// ── ChangePassword ────────────────────────────────
// POST /api/v1/user/change-password
func ChangePassword(c *gin.Context) {
	var req struct {
		Token       string `json:"token" binding:"required"`
		OldPassword string `json:"old_password" binding:"required"`
		NewPassword string `json:"new_password" binding:"required,min=6"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "invalid request: "+err.Error())
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

	if !utils.CheckPassword(req.OldPassword, user.Password) {
		utils.BadRequest(c, "原密码错误")
		return
	}

	hashed, _ := utils.HashPassword(req.NewPassword)
	database.DB.Model(&user).Update("password", hashed)

	utils.SuccessMessage(c, "密码已修改")
}

// Plans listing for the shop lives in order.go (enable + show_on_shop).
