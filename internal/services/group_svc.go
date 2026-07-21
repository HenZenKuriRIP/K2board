package services

import (
	"errors"
	"fmt"

	"gorm.io/gorm"

	"K2board/internal/database"
	"K2board/internal/models"
)

var (
	// ErrGroupInUse: non-admin users still have this group_id (v2board-style delete guard).
	ErrGroupInUse = errors.New("group has users in use")
)

type GroupService struct{}

func NewGroupService() *GroupService { return &GroupService{} }

// CountNonAdminUsersByGroup returns how many non-admin users are bound to groupID.
func CountNonAdminUsersByGroup(groupID uint) (int64, error) {
	if groupID == 0 {
		return 0, nil
	}
	var n int64
	err := database.DB.Model(&models.User{}).
		Where("is_admin = ? AND group_id = ?", false, groupID).
		Count(&n).Error
	return n, err
}

func (s *GroupService) Create(g *models.Group) error {
	return database.DB.Create(g).Error
}

func (s *GroupService) GetByID(id uint) (*models.Group, error) {
	var g models.Group
	err := database.DB.First(&g, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &g, err
}

func (s *GroupService) ListAll() ([]models.Group, error) {
	var groups []models.Group
	if err := database.DB.Order("id ASC").Find(&groups).Error; err != nil {
		return nil, err
	}
	s.attachUserCounts(groups)
	return groups, nil
}

// attachUserCounts fills UserCount for each group (non-admin users only).
func (s *GroupService) attachUserCounts(groups []models.Group) {
	if len(groups) == 0 {
		return
	}
	type row struct {
		GroupID uint  `gorm:"column:group_id"`
		Cnt     int64 `gorm:"column:cnt"`
	}
	var rows []row
	_ = database.DB.Model(&models.User{}).
		Select("group_id, COUNT(*) AS cnt").
		Where("is_admin = ? AND group_id > 0", false).
		Group("group_id").
		Scan(&rows).Error
	m := make(map[uint]int64, len(rows))
	for _, r := range rows {
		m[r.GroupID] = r.Cnt
	}
	for i := range groups {
		groups[i].UserCount = m[groups[i].ID]
	}
}

func (s *GroupService) Update(id uint, updates map[string]any) error {
	return database.DB.Model(&models.Group{}).Where("id = ?", id).Updates(updates).Error
}

// Delete removes a group only when no non-admin user still uses it.
// Refuses with ErrGroupInUse (wrapped with count) instead of orphaning users.
// When allowed: cleans node mappings, legacy node.group_id, and plans.group_id.
func (s *GroupService) Delete(id uint) error {
	n, err := CountNonAdminUsersByGroup(id)
	if err != nil {
		return err
	}
	if n > 0 {
		return fmt.Errorf("%w: %d user(s)", ErrGroupInUse, n)
	}

	return database.DB.Transaction(func(tx *gorm.DB) error {
		// Re-check inside tx to avoid race
		var n2 int64
		if err := tx.Model(&models.User{}).
			Where("is_admin = ? AND group_id = ?", false, id).
			Count(&n2).Error; err != nil {
			return err
		}
		if n2 > 0 {
			return fmt.Errorf("%w: %d user(s)", ErrGroupInUse, n2)
		}

		if err := tx.Where("group_id = ?", id).Delete(&models.NodeGroupMapping{}).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.Node{}).Where("group_id = ?", id).Update("group_id", 0).Error; err != nil {
			return err
		}
		// Plans lose group binding (remain listed but ungrouped)
		if err := tx.Model(&models.Plan{}).Where("group_id = ?", id).Update("group_id", 0).Error; err != nil {
			return err
		}
		if err := tx.Delete(&models.Group{}, id).Error; err != nil {
			return err
		}
		return nil
	})
}
