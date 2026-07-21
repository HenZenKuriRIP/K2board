package models

import "time"

// NodeToken stores the SHA256 hash of the API token used by backend nodes.
// The plaintext token is returned once at creation and never stored.
type NodeToken struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	NodeID    uint      `gorm:"index;not null" json:"node_id"`
	Token     string    `gorm:"uniqueIndex;size:64;not null" json:"token"` // SHA256 hash, not plaintext
	CreatedAt time.Time `json:"created_at"`
}

func (NodeToken) TableName() string {
	return "node_tokens"
}
