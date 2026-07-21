package admin

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"K2board/internal/models"
	"K2board/internal/services"
	"K2board/internal/utils"
)

type PlanHandler struct {
	svc *services.PlanService
}

func NewPlanHandler() *PlanHandler {
	return &PlanHandler{svc: services.NewPlanService()}
}

func (h *PlanHandler) List(c *gin.Context) {
	plans, err := h.svc.ListAll()
	if err != nil {
		utils.InternalError(c, "failed to fetch plans")
		return
	}
	if plans == nil {
		plans = []models.Plan{}
	}
	utils.Success(c, plans)
}

func (h *PlanHandler) Create(c *gin.Context) {
	var req models.Plan
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "invalid request")
		return
	}
	if req.Name == "" {
		utils.BadRequest(c, "name required")
		return
	}
	if req.Price < 0 {
		utils.BadRequest(c, "price must be >= 0")
		return
	}
	if req.Currency == "" {
		req.Currency = "CNY"
	}
	if err := h.svc.Create(&req); err != nil {
		if writePlanShopErr(c, err) {
			return
		}
		utils.InternalError(c, "failed to create plan")
		return
	}
	utils.Created(c, &req)
	services.BumpConfigVersion()
}

func writePlanShopErr(c *gin.Context, err error) bool {
	switch {
	case errors.Is(err, services.ErrPlanShopNoGroup):
		utils.BadRequest(c, "上架或开启续费前请先绑定权限组")
		return true
	case errors.Is(err, services.ErrPlanShopGroupDisabled):
		utils.BadRequest(c, "权限组已禁用，无法上架或开启续费")
		return true
	case errors.Is(err, services.ErrPlanShopNoNode):
		utils.BadRequest(c, "上架或开启续费前请为该权限组绑定至少一个启用中的节点")
		return true
	default:
		return false
	}
}

func (h *PlanHandler) Update(c *gin.Context) {
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
	if v, ok := req["price"]; ok {
		switch n := v.(type) {
		case float64:
			if n < 0 {
				utils.BadRequest(c, "price must be >= 0")
				return
			}
		case int:
			if n < 0 {
				utils.BadRequest(c, "price must be >= 0")
				return
			}
		}
	}
	if err := h.svc.Update(uint(id), req); err != nil {
		if writePlanShopErr(c, err) {
			return
		}
		utils.InternalError(c, "failed to update plan")
		return
	}
	p, _ := h.svc.GetByID(uint(id))
	utils.Success(c, p)
}

func (h *PlanHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		utils.BadRequest(c, "invalid id")
		return
	}
	if err := h.svc.Delete(uint(id)); err != nil {
		if errors.Is(err, services.ErrPlanInUse) {
			n := parseInUseCount(err)
			if n > 0 {
				utils.BadRequest(c, fmt.Sprintf("无法删除：仍有 %d 名用户在使用该订阅计划，请先为用户更换或清空套餐", n))
			} else {
				utils.BadRequest(c, "无法删除：仍有用户在使用该订阅计划，请先为用户更换或清空套餐")
			}
			return
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.NotFound(c, "计划不存在")
			return
		}
		utils.InternalError(c, "删除失败")
		return
	}
	utils.SuccessMessage(c, "已删除")
	services.BumpConfigVersion()
}
