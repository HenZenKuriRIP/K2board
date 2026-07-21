package models

import "time"

// Plan defines a subscription plan (e.g., Monthly 100GB, Yearly 1TB).
// Plans belong to a group and define duration + optional limit overrides.
// Pricing (Phase 1): single SKU price in minor units (cents).
type Plan struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Name         string    `gorm:"size:128;not null" json:"name"`
	GroupID      uint      `gorm:"index;default:0" json:"group_id"`
	Duration     int64     `gorm:"default:0" json:"duration"` // seconds: 2592000=30d, 31536000=365d
	TrafficLimit int64     `gorm:"default:0" json:"traffic_limit"`
	SpeedLimit   int64     `gorm:"default:0" json:"speed_limit"`
	DeviceLimit  int       `gorm:"default:0" json:"device_limit"`
	ResetDay     int       `gorm:"default:0" json:"reset_day"` // 1-28: monthly reset day, 0: no auto-reset
	// Price is in minor currency units (e.g. cents). 0 = free checkout still creates an order.
	Price      int64  `gorm:"default:0" json:"price"`
	Currency   string `gorm:"size:8;default:CNY" json:"currency"`
	// ShowOnShop: list + new purchase in public shop (新客可见可购).
	ShowOnShop bool `gorm:"default:false" json:"show_on_shop"`
	// AllowRenew: when true, users whose current plan_id is this plan may purchase again
	// even if ShowOnShop is false (停售后已购用户续费). Default true so unlisting does not
	// silently kill renewals until admin turns it off.
	AllowRenew bool `gorm:"default:true" json:"allow_renew"`
	Enable     bool `gorm:"default:true" json:"enable"`
	Sort       int  `gorm:"default:0" json:"sort"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (Plan) TableName() string {
	return "plans"
}
