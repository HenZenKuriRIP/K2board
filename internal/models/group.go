package models

import "time"

// Group is a label/category that bundles nodes. Limits are defined in Plans, not here.
type Group struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"size:128;not null" json:"name"`
	Enable    bool      `gorm:"default:true" json:"enable"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	// UserCount is populated on list APIs only (non-admin users with this group_id).
	UserCount int64 `gorm:"-" json:"user_count"`
}

func (Group) TableName() string {
	return "groups"
}
