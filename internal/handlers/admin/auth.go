package admin

import (
	"log/slog"
	"strings"

	"github.com/gin-gonic/gin"

	"K2board/internal/middleware"
	"K2board/internal/services"
	"K2board/internal/utils"
)

type AuthHandler struct {
	userSvc *services.UserService
}

func NewAuthHandler() *AuthHandler {
	return &AuthHandler{
		userSvc: services.NewUserService(),
	}
}

type LoginRequest struct {
	Email    string `json:"email" binding:"required"`
	Password string `json:"password" binding:"required,min=6"`
}

type LoginResponse struct {
	Token string `json:"token"`
	Email string `json:"email"`
}

// Login handles admin authentication with rate limiting (10/minute/IP).
func (h *AuthHandler) Login(c *gin.Context) {
	if !middleware.CheckLoginRate(c.ClientIP()) {
		utils.Error(c, 429, "登录尝试过于频繁，请稍后再试")
		return
	}

	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Warn("admin login bind failed", "ip", c.ClientIP(), "error", err)
		utils.BadRequest(c, "请求格式错误，请检查邮箱和密码")
		return
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))
	if email == "" || !strings.Contains(email, "@") {
		utils.BadRequest(c, "请输入有效邮箱")
		return
	}

	user, err := h.userSvc.Authenticate(email, req.Password)
	if err != nil || user == nil {
		slog.Info("admin login failed", "email", email, "ip", c.ClientIP())
		utils.Unauthorized(c, "邮箱或密码错误")
		return
	}
	if !user.Enable {
		utils.Forbidden(c, "账号已被禁用")
		return
	}
	if !user.IsAdmin {
		slog.Info("non-admin login rejected", "email", email, "ip", c.ClientIP())
		utils.Forbidden(c, "需要管理员账号登录后台")
		return
	}

	token, err := utils.GenerateJWT(user.ID, user.Email, user.IsAdmin)
	if err != nil {
		slog.Error("admin jwt failed", "error", err)
		utils.InternalError(c, "生成登录凭证失败")
		return
	}

	slog.Info("admin login ok", "email", user.Email, "ip", c.ClientIP())
	utils.Success(c, LoginResponse{
		Token: token,
		Email: user.Email,
	})
}
