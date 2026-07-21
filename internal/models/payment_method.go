package models

import "time"

// PaymentMethod is an admin-configured payment channel instance.
// Code maps to a registered PaymentGateway implementation.
type PaymentMethod struct {
	ID     uint   `gorm:"primaryKey" json:"id"`
	Code   string `gorm:"uniqueIndex;size:32;not null" json:"code"` // mock, alipay, stripe, …
	Name   string `gorm:"size:64;not null" json:"name"`
	Enable bool   `gorm:"default:true" json:"enable"`
	Sort   int    `gorm:"default:0" json:"sort"`
	// Config is JSON credentials; never return full secrets to end users.
	Config    string    `gorm:"type:text" json:"config"`
	Remark    string    `gorm:"size:255" json:"remark"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (PaymentMethod) TableName() string { return "payment_methods" }
