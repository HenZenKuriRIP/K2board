package admin

import (
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"K2board/internal/database"
	"K2board/internal/models"
	"K2board/internal/services"
	"K2board/internal/utils"
)

type UserHandler struct {
	userSvc  *services.UserService
	groupSvc *services.GroupService
	planSvc  *services.PlanService
}

func NewUserHandler() *UserHandler {
	return &UserHandler{
		userSvc:  services.NewUserService(),
		groupSvc: services.NewGroupService(),
		planSvc:  services.NewPlanService(),
	}
}

type CreateUserRequest struct {
	Email        string `json:"email" binding:"required,email"`
	Password     string `json:"password" binding:"required,min=6"`
	GroupID      uint   `json:"group_id"`
	PlanID       uint   `json:"plan_id"`
	TrafficLimit int64  `json:"traffic_limit"`
	SpeedLimit   int64  `json:"speed_limit"`
	DeviceLimit  int    `json:"device_limit"`
	ExpireAt     int64  `json:"expire_at"`
}

type UpdateUserRequest struct {
	Email        *string `json:"email"`
	Password     *string `json:"password"`
	GroupID      *uint   `json:"group_id"`
	PlanID       *uint   `json:"plan_id"`
	TrafficLimit *int64  `json:"traffic_limit"`
	SpeedLimit   *int64  `json:"speed_limit"`
	DeviceLimit  *int    `json:"device_limit"`
	Enable       *bool   `json:"enable"`
	ExpireAt     *int64  `json:"expire_at"`
}

// List returns a paginated user list (full-table sort via sort_by / sort_order).
func (h *UserHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	search := c.Query("search")
	sortBy := c.DefaultQuery("sort_by", "id")
	sortOrder := c.DefaultQuery("sort_order", "desc")

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize
	if offset > 100000 {
		utils.BadRequest(c, "page too large")
		return
	}

	users, total, err := h.userSvc.ListUsers(page, pageSize, search, sortBy, sortOrder)
	if err != nil {
		utils.InternalError(c, "failed to fetch users")
		return
	}

	// OnlineTTL device aggregation (shared helper)
	services.LoadOnlineIPsForUsers(users)

	col, dir := services.NormalizeUserListSort(sortBy, sortOrder)
	utils.Success(c, gin.H{
		"list":       users,
		"total":      total,
		"page":       page,
		"page_size":  pageSize,
		"sort_by":    col,
		"sort_order": strings.ToLower(dir),
	})
}

// Get returns a single user by ID.
func (h *UserHandler) Get(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		utils.BadRequest(c, "invalid user id")
		return
	}

	user, err := h.userSvc.GetUserByID(uint(id))
	if err != nil {
		utils.InternalError(c, "failed to fetch user")
		return
	}
	if user == nil {
		utils.NotFound(c, "user not found")
		return
	}
	list := []models.User{*user}
	services.LoadOnlineIPsForUsers(list)
	*user = list[0]

	utils.Success(c, user)
}

// Create creates a new user with optional group/plan assignment.
func (h *UserHandler) Create(c *gin.Context) {
	var req CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "请求参数无效: "+err.Error())
		return
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))
	if email == "" || !strings.Contains(email, "@") {
		utils.BadRequest(c, "请输入有效邮箱")
		return
	}
	if len(req.Password) < 6 {
		utils.BadRequest(c, "密码至少 6 位")
		return
	}

	// Duplicate check with clear message
	if existing, _ := h.userSvc.GetUserByEmail(email); existing != nil {
		utils.BadRequest(c, "该邮箱已注册")
		return
	}

	trafficLimit := req.TrafficLimit
	if trafficLimit < 0 {
		trafficLimit = 0
	}
	speedLimit := req.SpeedLimit
	if speedLimit < 0 {
		speedLimit = 0
	}
	deviceLimit := req.DeviceLimit
	if deviceLimit < 0 {
		deviceLimit = 0
	}
	expireAt := req.ExpireAt
	if expireAt < 0 {
		expireAt = 0
	}
	finalGroupID := req.GroupID
	planID := req.PlanID

	if planID > 0 {
		p, _ := h.planSvc.GetByID(planID)
		if p == nil {
			utils.BadRequest(c, "订阅计划不存在")
			return
		}
		if !p.Enable {
			utils.BadRequest(c, "订阅计划已禁用")
			return
		}
		if p.GroupID > 0 {
			finalGroupID = p.GroupID
		}
		if trafficLimit == 0 {
			trafficLimit = p.TrafficLimit
		}
		if speedLimit == 0 {
			speedLimit = p.SpeedLimit
		}
		if deviceLimit == 0 {
			deviceLimit = p.DeviceLimit
		}
		if expireAt == 0 && p.Duration > 0 {
			expireAt = time.Now().Unix() + p.Duration
		}
	}

	user := &models.User{
		Email:        email,
		Password:     req.Password,
		GroupID:      finalGroupID,
		PlanID:       planID,
		TrafficLimit: trafficLimit,
		SpeedLimit:   speedLimit,
		DeviceLimit:  deviceLimit,
		ExpireAt:     expireAt,
		Enable:       true,
	}

	if err := h.userSvc.CreateUser(user); err != nil {
		utils.InternalError(c, "创建用户失败: "+err.Error())
		return
	}

	utils.Created(c, user)
	services.BumpConfigVersion()
}

// Update updates an existing user.
func (h *UserHandler) Update(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		utils.BadRequest(c, "invalid user id")
		return
	}

	var req UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	updates := make(map[string]interface{})
	if req.Email != nil {
		email := strings.ToLower(strings.TrimSpace(*req.Email))
		if email == "" || !strings.Contains(email, "@") || len(email) < 3 {
			utils.BadRequest(c, "邮箱格式无效")
			return
		}
		// Unique among other users — avoid opaque 500 from DB unique index
		var n int64
		database.DB.Model(&models.User{}).Where("email = ? AND id <> ?", email, uint(id)).Count(&n)
		if n > 0 {
			utils.BadRequest(c, "邮箱已被使用")
			return
		}
		updates["email"] = email
	}
	if req.TrafficLimit != nil {
		updates["traffic_limit"] = *req.TrafficLimit
	}
	if req.SpeedLimit != nil {
		updates["speed_limit"] = *req.SpeedLimit
	}
	if req.DeviceLimit != nil {
		updates["device_limit"] = *req.DeviceLimit
	}
	if req.Enable != nil {
		updates["enable"] = *req.Enable
	}
	if req.ExpireAt != nil {
		updates["expire_at"] = *req.ExpireAt
	}
	if req.GroupID != nil {
		updates["group_id"] = *req.GroupID
	}

	// Apply plan: bind plan_id and optionally refresh group/limits/expiry
	if req.PlanID != nil {
		planID := *req.PlanID
		updates["plan_id"] = planID
		if planID > 0 {
			p, _ := h.planSvc.GetByID(planID)
			if p == nil || !p.Enable {
				utils.BadRequest(c, "plan not found or disabled")
				return
			}
			// Plan group always applies when plan is set
			if p.GroupID > 0 {
				updates["group_id"] = p.GroupID
			}
			// Only fill limits when caller did not explicitly set them
			if req.TrafficLimit == nil && p.TrafficLimit > 0 {
				updates["traffic_limit"] = p.TrafficLimit
			}
			if req.SpeedLimit == nil && p.SpeedLimit > 0 {
				updates["speed_limit"] = p.SpeedLimit
			}
			if req.DeviceLimit == nil && p.DeviceLimit > 0 {
				updates["device_limit"] = p.DeviceLimit
			}
			if req.ExpireAt == nil && p.Duration > 0 {
				updates["expire_at"] = time.Now().Unix() + p.Duration
			}
		}
	}

	passwordUpdated := false
	if req.Password != nil && strings.TrimSpace(*req.Password) != "" {
		if err := h.userSvc.UpdatePassword(uint(id), *req.Password); err != nil {
			slog.Error("admin update password failed", "user_id", id, "error", err)
			utils.BadRequest(c, "密码更新失败: "+err.Error())
			return
		}
		passwordUpdated = true
	}

	if len(updates) > 0 {
		if err := h.userSvc.UpdateUser(uint(id), updates); err != nil {
			utils.InternalError(c, "failed to update user")
			return
		}
	}

	user, _ := h.userSvc.GetUserByID(uint(id))
	msg := "用户已更新"
	if passwordUpdated {
		msg = "用户已更新，密码已重置（请用新密码在用户端登录）"
	} else if req.Password != nil {
		// explicit empty password field — no-op
		msg = "用户已更新（未修改密码：密码框为空）"
	}
	utils.Success(c, gin.H{
		"user":             user,
		"password_updated": passwordUpdated,
		"message":          msg,
	})
	services.BumpConfigVersion()
}

// Delete deletes a user. Admin users cannot be deleted.
func (h *UserHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		utils.BadRequest(c, "invalid user id")
		return
	}

	u, _ := h.userSvc.GetUserByID(uint(id))
	if u != nil && u.IsAdmin {
		utils.Forbidden(c, "cannot delete admin account")
		return
	}

	if err := h.userSvc.DeleteUser(uint(id)); err != nil {
		utils.InternalError(c, "failed to delete user")
		return
	}

	utils.SuccessMessage(c, "deleted")
	services.BumpConfigVersion()
}

// ResetUUID regenerates the V2Ray UUID for a user.
func (h *UserHandler) ResetUUID(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		utils.BadRequest(c, "invalid user id")
		return
	}

	newUUID, err := h.userSvc.ResetUserUUID(uint(id))
	if err != nil {
		utils.InternalError(c, "failed to reset uuid")
		return
	}

	utils.Success(c, gin.H{"uuid": newUUID})
	services.BumpConfigVersion()
}

// BatchDelete deletes multiple users at once. Skips admin accounts.
func (h *UserHandler) BatchDelete(c *gin.Context) {
	var req struct {
		IDs []uint `json:"ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 {
		utils.BadRequest(c, "invalid ids")
		return
	}
	if len(req.IDs) > 1000 {
		utils.BadRequest(c, "too many ids")
		return
	}

	var failed []uint
	var skippedAdmin []uint
	var ok int
	for _, id := range req.IDs {
		u, _ := h.userSvc.GetUserByID(id)
		if u != nil && u.IsAdmin {
			skippedAdmin = append(skippedAdmin, id)
			continue
		}
		if err := h.userSvc.DeleteUser(id); err != nil {
			failed = append(failed, id)
		} else {
			ok++
		}
	}
	if len(failed) > 0 || len(skippedAdmin) > 0 {
		msg := fmt.Sprintf("deleted %d", ok)
		if len(skippedAdmin) > 0 {
			msg += fmt.Sprintf(", skipped admin: %v", skippedAdmin)
		}
		if len(failed) > 0 {
			msg += fmt.Sprintf(", failed: %v", failed)
			utils.BadRequest(c, msg)
			return
		}
		utils.SuccessMessage(c, msg)
		if ok > 0 {
			services.BumpConfigVersion()
		}
		return
	}
	utils.SuccessMessage(c, fmt.Sprintf("deleted %d users", ok))
	services.BumpConfigVersion()
}

// BatchUpdateGroup assigns a group to multiple users at once.
// group_id=0 clears the group (groupless users).
func (h *UserHandler) BatchUpdateGroup(c *gin.Context) {
	var req struct {
		IDs     []uint `json:"ids"`
		GroupID uint   `json:"group_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 {
		utils.BadRequest(c, "invalid request")
		return
	}
	if len(req.IDs) > 1000 {
		utils.BadRequest(c, "too many ids")
		return
	}
	if req.GroupID > 0 {
		var group models.Group
		if database.DB.Where("id = ?", req.GroupID).First(&group).Error != nil {
			utils.BadRequest(c, "group not found")
			return
		}
	}
	result := database.DB.Model(&models.User{}).
		Where("id IN ? AND is_admin = ?", req.IDs, false).
		Update("group_id", req.GroupID)
	if result.Error != nil {
		utils.InternalError(c, "batch update failed")
		return
	}
	utils.SuccessMessage(c, fmt.Sprintf("updated %d users", result.RowsAffected))
	services.BumpConfigVersion()
}

// OnlineUsers returns IDs of users currently online (OnlineTTL window).
func (h *UserHandler) OnlineUsers(c *gin.Context) {
	utils.Success(c, services.ListOnlineUserIDs())
}

// ResetToken regenerates the subscribe token for a user.
func (h *UserHandler) ResetToken(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		utils.BadRequest(c, "invalid user id")
		return
	}

	newToken, err := h.userSvc.ResetUserToken(uint(id))
	if err != nil {
		utils.InternalError(c, "failed to reset token")
		return
	}

	utils.Success(c, gin.H{"token": newToken})
	services.BumpConfigVersion()
}

// ResetTraffic zeros the user's traffic_used counter.
func (h *UserHandler) ResetTraffic(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		utils.BadRequest(c, "invalid user id")
		return
	}

	if err := h.userSvc.ResetTraffic(uint(id)); err != nil {
		utils.InternalError(c, "failed to reset traffic")
		return
	}

	utils.SuccessMessage(c, "traffic reset")
	services.BumpConfigVersion()
}
