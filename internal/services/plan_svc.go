package services

import (
	"errors"
	"fmt"

	"gorm.io/gorm"

	"K2board/internal/database"
	"K2board/internal/models"
)

var (
	ErrPlanShopNoGroup       = errors.New("shop plan requires a permission group")
	ErrPlanShopNoNode        = errors.New("shop plan group has no enabled nodes")
	ErrPlanShopGroupDisabled = errors.New("shop plan group is disabled")
	// ErrPlanInUse: non-admin users still have this plan_id (v2board-style delete guard).
	ErrPlanInUse = errors.New("plan has users in use")
)

// CountNonAdminUsersByPlan returns how many non-admin users are bound to planID.
func CountNonAdminUsersByPlan(planID uint) (int64, error) {
	if planID == 0 {
		return 0, nil
	}
	var n int64
	err := database.DB.Model(&models.User{}).
		Where("is_admin = ? AND plan_id = ?", false, planID).
		Count(&n).Error
	return n, err
}

type PlanService struct{}

func NewPlanService() *PlanService { return &PlanService{} }

// CountEnabledNodesInGroup returns how many enabled nodes are mapped to groupID.
func CountEnabledNodesInGroup(groupID uint) (int64, error) {
	if groupID == 0 {
		return 0, nil
	}
	var n int64
	err := database.DB.Model(&models.Node{}).
		Where("enable = ?", true).
		Where(`id IN (SELECT node_id FROM node_group_mappings WHERE group_id = ?)`, groupID).
		Count(&n).Error
	return n, err
}

// ValidatePlanForShop enforces: when a plan is sold (shop and/or renew), it needs
// a permission group with ≥1 enabled node. inactive=false skips the check (pure draft).
func ValidatePlanForShop(needsNodes bool, groupID uint) error {
	if !needsNodes {
		return nil
	}
	if groupID == 0 {
		return ErrPlanShopNoGroup
	}
	var g models.Group
	if err := database.DB.Select("id", "enable").First(&g, groupID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrPlanShopNoGroup
		}
		return err
	}
	if !g.Enable {
		return ErrPlanShopGroupDisabled
	}
	n, err := CountEnabledNodesInGroup(groupID)
	if err != nil {
		return err
	}
	if n < 1 {
		return fmt.Errorf("%w: group_id=%d", ErrPlanShopNoNode, groupID)
	}
	return nil
}

// PlanNeedsNodeCapacity is true if the plan is listed for new sales or allows renewals.
func PlanNeedsNodeCapacity(showOnShop, allowRenew bool) bool {
	return showOnShop || allowRenew
}

func (s *PlanService) Create(p *models.Plan) error {
	if err := ValidatePlanForShop(PlanNeedsNodeCapacity(p.ShowOnShop, p.AllowRenew), p.GroupID); err != nil {
		return err
	}
	return database.DB.Create(p).Error
}

func (s *PlanService) GetByID(id uint) (*models.Plan, error) {
	var p models.Plan
	err := database.DB.First(&p, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &p, err
}

func (s *PlanService) ListAll() ([]models.Plan, error) {
	var plans []models.Plan
	err := database.DB.Order("sort ASC, id ASC").Find(&plans).Error
	return plans, err
}

func (s *PlanService) Update(id uint, updates map[string]any) error {
	cur, err := s.GetByID(id)
	if err != nil {
		return err
	}
	if cur == nil {
		return gorm.ErrRecordNotFound
	}

	show := cur.ShowOnShop
	renew := cur.AllowRenew
	gid := cur.GroupID
	if v, ok := updates["show_on_shop"]; ok {
		switch t := v.(type) {
		case bool:
			show = t
		case float64:
			show = t != 0
		case int:
			show = t != 0
		}
	}
	if v, ok := updates["allow_renew"]; ok {
		switch t := v.(type) {
		case bool:
			renew = t
		case float64:
			renew = t != 0
		case int:
			renew = t != 0
		}
	}
	if v, ok := updates["group_id"]; ok {
		switch t := v.(type) {
		case float64:
			gid = uint(t)
		case int:
			gid = uint(t)
		case uint:
			gid = t
		case int64:
			gid = uint(t)
		}
	}
	if err := ValidatePlanForShop(PlanNeedsNodeCapacity(show, renew), gid); err != nil {
		return err
	}
	return database.DB.Model(&models.Plan{}).Where("id = ?", id).Updates(updates).Error
}

// Delete removes a plan only when no non-admin user still has plan_id = id.
func (s *PlanService) Delete(id uint) error {
	n, err := CountNonAdminUsersByPlan(id)
	if err != nil {
		return err
	}
	if n > 0 {
		return fmt.Errorf("%w: %d user(s)", ErrPlanInUse, n)
	}
	result := database.DB.Delete(&models.Plan{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
