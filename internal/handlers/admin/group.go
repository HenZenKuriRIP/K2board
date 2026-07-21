package admin

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"K2board/internal/models"
	"K2board/internal/services"
	"K2board/internal/utils"
)

type GroupHandler struct {
	svc *services.GroupService
}

func NewGroupHandler() *GroupHandler {
	return &GroupHandler{svc: services.NewGroupService()}
}

func (h *GroupHandler) List(c *gin.Context) {
	groups, err := h.svc.ListAll()
	if err != nil {
		utils.InternalError(c, "failed")
		return
	}
	if groups == nil {
		groups = []models.Group{}
	}
	utils.Success(c, groups)
}

func (h *GroupHandler) Create(c *gin.Context) {
	var req struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
		utils.BadRequest(c, "name required")
		return
	}
	g := &models.Group{Name: req.Name, Enable: true}
	if err := h.svc.Create(g); err != nil {
		utils.InternalError(c, "failed")
		return
	}
	utils.Created(c, g)
	services.BumpConfigVersion()
}

func (h *GroupHandler) Update(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var req map[string]any
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "invalid")
		return
	}
	if err := h.svc.Update(uint(id), req); err != nil {
		utils.InternalError(c, "failed")
		return
	}
	g, _ := h.svc.GetByID(uint(id))
	utils.Success(c, g)
	services.BumpConfigVersion() // notify nodes to re-pull users on group changes
}

func (h *GroupHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		utils.BadRequest(c, "invalid id")
		return
	}
	if err := h.svc.Delete(uint(id)); err != nil {
		if errors.Is(err, services.ErrGroupInUse) {
			n := parseInUseCount(err)
			if n > 0 {
				utils.BadRequest(c, fmt.Sprintf("无法删除：仍有 %d 名用户在使用该权限组，请先调整用户分组", n))
			} else {
				utils.BadRequest(c, "无法删除：仍有用户在使用该权限组，请先调整用户分组")
			}
			return
		}
		utils.InternalError(c, "删除失败")
		return
	}
	utils.SuccessMessage(c, "已删除")
	services.BumpConfigVersion()
}

// parseInUseCount extracts trailing "N user(s)" from fmt.Errorf("%w: %d user(s)", ...).
func parseInUseCount(err error) int64 {
	if err == nil {
		return 0
	}
	msg := err.Error()
	// e.g. "group has users in use: 3 user(s)"
	i := strings.LastIndex(msg, ": ")
	if i < 0 {
		return 0
	}
	rest := strings.TrimSpace(msg[i+2:])
	rest = strings.TrimSuffix(rest, " user(s)")
	rest = strings.TrimSuffix(rest, " users")
	n, e := strconv.ParseInt(rest, 10, 64)
	if e != nil {
		return 0
	}
	return n
}
