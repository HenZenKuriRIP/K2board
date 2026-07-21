package models

import "time"

// NodeOnline records the online status of users on a specific node.
type NodeOnline struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	NodeID    uint      `gorm:"index;not null" json:"node_id"`
	UserID    uint      `gorm:"index;not null" json:"user_id"`
	IP        string    `gorm:"size:64" json:"ip"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
}

func (NodeOnline) TableName() string {
	return "node_onlines"
}
