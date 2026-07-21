package models

import "time"

// Commission ledger status.
const (
	CommissionCredited = "credited"
)

// Withdrawal status values.
const (
	WithdrawPending  = "pending"
	WithdrawApproved = "approved" // legacy alias; prefer paid
	WithdrawRejected = "rejected"
	WithdrawPaid     = "paid"
)

// CommissionLedger records one commission credit from a paid order to an inviter.
// Amounts are minor currency units (cents). Unique on OrderID for idempotency.
type CommissionLedger struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	UserID      uint      `gorm:"index;not null" json:"user_id"`      // inviter (beneficiary)
	FromUserID  uint      `gorm:"index;not null" json:"from_user_id"` // buyer
	OrderID     uint      `gorm:"uniqueIndex;not null" json:"order_id"`
	TradeNo     string    `gorm:"size:64;index" json:"trade_no"`
	OrderAmount int64     `gorm:"not null;default:0" json:"order_amount"`
	RatePercent int       `gorm:"not null;default:0" json:"rate_percent"` // e.g. 10 = 10%
	Amount      int64     `gorm:"not null;default:0" json:"amount"`
	Status      string    `gorm:"size:16;default:credited" json:"status"`
	Remark      string    `gorm:"size:255" json:"remark"`
	CreatedAt   time.Time `json:"created_at"`

	// Optional join fields (not persisted)
	UserEmail     string `gorm:"-" json:"user_email,omitempty"`
	FromUserEmail string `gorm:"-" json:"from_user_email,omitempty"`
}

func (CommissionLedger) TableName() string { return "commission_ledgers" }

// CommissionWithdraw is a user cash-out request against commission balance.
// Amount is in cents. On create, balance is held (deducted); reject refunds.
type CommissionWithdraw struct {
	ID          uint       `gorm:"primaryKey" json:"id"`
	UserID      uint       `gorm:"index;not null" json:"user_id"`
	Amount      int64      `gorm:"not null;default:0" json:"amount"`
	Status      string     `gorm:"size:16;index;default:pending" json:"status"`
	Method      string     `gorm:"size:32;not null" json:"method"`   // alipay / wechat / usdt_trc20 / bank
	Account     string     `gorm:"size:255;not null" json:"account"` // account / address / card
	AccountName string     `gorm:"size:128" json:"account_name"`     // real name (optional)
	AdminRemark string     `gorm:"size:255" json:"admin_remark"`
	ProcessedAt *time.Time `json:"processed_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`

	// Optional join
	UserEmail string `gorm:"-" json:"user_email,omitempty"`
}

func (CommissionWithdraw) TableName() string { return "commission_withdraws" }
