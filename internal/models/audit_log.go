package models

import "time"

// AuditLog records admin operations for security auditing.
type AuditLog struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	AdminID   uint      `gorm:"index" json:"admin_id"`
	Action    string    `gorm:"size:64" json:"action"`   // create/update/delete/batch
	Target    string    `gorm:"size:64" json:"target"`    // user/node/group/plan/setting
	TargetID  uint      `json:"target_id"`
	Detail    string    `gorm:"size:512" json:"detail"`
	CreatedAt time.Time `json:"created_at"`
}

func (AuditLog) TableName() string {
	return "audit_logs"
}
