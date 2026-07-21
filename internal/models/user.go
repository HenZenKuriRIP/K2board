package models

import "time"

// User represents a proxy service user (v2board compatible).
type User struct {
	ID       uint   `gorm:"primaryKey" json:"id"`
	Email    string `gorm:"uniqueIndex;size:128;not null" json:"email"`
	Password string `gorm:"size:255;not null" json:"-"`
	UUID     string `gorm:"uniqueIndex;size:64" json:"uuid"`
	Token    string `gorm:"uniqueIndex;size:64" json:"token"`
	GroupID  uint   `gorm:"default:0" json:"group_id"`
	IsAdmin  bool   `gorm:"default:false" json:"is_admin"`
	PlanID   uint   `gorm:"default:0" json:"plan_id"`
	// Balance is commission wallet in minor currency units (cents). Credited on
	// referred users' paid orders; held/deducted on withdraw request.
	Balance int64 `gorm:"default:0" json:"balance"`
	// CommissionTotal is lifetime credited commission (cents), never decreases.
	CommissionTotal int64 `gorm:"default:0" json:"commission_total"`
	// InviteCode is this user's unique referral code (auto-generated).
	// Uniqueness is enforced in application code (CreateUser / EnsureInviteCode)
	// so legacy empty values do not break AutoMigrate unique constraints.
	InviteCode string `gorm:"index;size:16" json:"invite_code"`
	// InviterID is the user who invited this account (0 = none). Set once at register.
	InviterID    uint  `gorm:"index;default:0" json:"inviter_id"`
	TrafficLimit int64 `gorm:"default:0" json:"traffic_limit"`
	TrafficUsed  int64 `gorm:"default:0" json:"traffic_used"`
	// LastTrafficResetAt is unix seconds when traffic_used was last auto-reset or
	// zeroed by purchase fulfill. Prevents hourly double-reset on the same calendar day.
	LastTrafficResetAt int64      `gorm:"default:0;index" json:"last_traffic_reset_at"`
	SpeedLimit         int64      `gorm:"default:0" json:"speed_limit"`
	DeviceLimit        int        `gorm:"default:0" json:"device_limit"`
	Enable             bool       `gorm:"default:true" json:"enable"`
	ExpireAt           int64      `gorm:"default:0" json:"expire_at"`
	DeviceCount        int64      `gorm:"-" json:"device_count"` // computed from node_onlines
	OnlineIPs          []string   `gorm:"-" json:"online_ips"`   // computed from node_onlines
	LastActiveAt       *time.Time `json:"last_active_at"`        // persisted — last seen online
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

func (User) TableName() string {
	return "users"
}
