package models

import "time"

// Order status values.
const (
	OrderPending   = "pending"
	OrderPaid      = "paid"
	OrderCancelled = "cancelled"
	OrderFailed    = "failed"
)

// Order is a purchase of a plan. Amounts are minor currency units (cents).
type Order struct {
	ID     uint   `gorm:"primaryKey" json:"id"`
	TradeNo string `gorm:"uniqueIndex;size:64;not null" json:"trade_no"`
	UserID uint   `gorm:"index;not null" json:"user_id"`

	// Plan snapshot at order time (immutable for this order)
	PlanID       uint   `gorm:"index;not null" json:"plan_id"`
	PlanName     string `gorm:"size:128" json:"plan_name"`
	GroupID      uint   `gorm:"default:0" json:"group_id"`
	Duration     int64  `gorm:"default:0" json:"duration"`
	TrafficLimit int64  `gorm:"default:0" json:"traffic_limit"`
	SpeedLimit   int64  `gorm:"default:0" json:"speed_limit"`
	DeviceLimit  int    `gorm:"default:0" json:"device_limit"`

	TotalAmount int64  `gorm:"not null;default:0" json:"total_amount"`
	Currency    string `gorm:"size:8;default:CNY" json:"currency"`

	Status        string `gorm:"size:16;index;default:pending" json:"status"`
	PaymentMethod string `gorm:"size:32" json:"payment_method"`
	CallbackNo    string `gorm:"size:128" json:"callback_no"`
	Meta          string `gorm:"type:text" json:"meta"` // JSON extras

	PaidAt      *time.Time `json:"paid_at"`
	ExpiredAt   time.Time  `gorm:"index" json:"expired_at"`
	FulfilledAt *time.Time `json:"fulfilled_at"`
	Remark      string     `gorm:"size:255" json:"remark"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Optional join for admin list
	UserEmail string `gorm:"-" json:"user_email,omitempty"`
}

func (Order) TableName() string { return "orders" }
