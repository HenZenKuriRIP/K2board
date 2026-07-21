package models

import "time"

// FreePlanClaim records that a verified email has already claimed a free (price=0) plan.
// One row per normalized email — blocks re-register + re-claim abuse.
// Order trade_no is kept for audit; enforcement does not rely on user_id alone.
type FreePlanClaim struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Email     string    `gorm:"uniqueIndex;size:128;not null" json:"email"` // lowercased
	UserID    uint      `gorm:"index;not null" json:"user_id"`
	PlanID    uint      `gorm:"index;not null" json:"plan_id"`
	TradeNo   string    `gorm:"size:64;index" json:"trade_no"`
	ClaimedAt time.Time `json:"claimed_at"`
	CreatedAt time.Time `json:"created_at"`
}

func (FreePlanClaim) TableName() string {
	return "free_plan_claims"
}
