package models

import (
	"encoding/json"
	"time"
)

// Node represents a proxy server node.
type Node struct {
	ID              uint            `gorm:"primaryKey" json:"id"`
	Name            string          `gorm:"size:128;not null" json:"name"`
	GroupID         uint            `gorm:"index;default:0" json:"group_id"`
	NodeType        string          `gorm:"size:32;index;not null" json:"node_type"`
	Cipher          string          `gorm:"size:32;default:aes-256-gcm" json:"cipher"`
	Host            string          `gorm:"size:255" json:"host"`
	Port            int             `gorm:"default:443" json:"port"`
	Network         string          `gorm:"size:32;default:ws" json:"network"`
	TLS             int             `gorm:"default:1" json:"tls"`
	TLStype         string          `gorm:"size:32" json:"tls_type"`
	Path            string          `gorm:"size:255" json:"path"`
	SNI             string          `gorm:"size:255" json:"sni"`
	ServiceName     string          `gorm:"size:255" json:"service_name"`
	Flow            string          `gorm:"size:64" json:"flow"`
	SpeedLimit      float64         `gorm:"default:0" json:"speed_limit"`
	RealitySettings json.RawMessage `gorm:"type:json" json:"reality_settings"`
	// VLESS Encryption (xray-core PQ payload) — node-level; empty / "none" = off (legacy compatible)
	VlessDecryption string `gorm:"type:text" json:"vless_decryption"`
	VlessEncryption string `gorm:"type:text" json:"vless_encryption"`
	Enable           bool            `gorm:"default:true" json:"enable"`
	LastHeartbeatAt  *time.Time      `json:"last_heartbeat_at"`
	CPU              float64         `gorm:"default:0" json:"cpu"`
	Mem              float64         `gorm:"default:0" json:"mem"`
	Disk             float64         `gorm:"default:0" json:"disk"`
	Uptime           int             `gorm:"default:0" json:"uptime"`
	ActiveConns      int             `gorm:"default:0" json:"active_conns"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`

	// Associations (eager-loaded)
	NodeTokens []NodeToken `gorm:"foreignKey:NodeID" json:"node_tokens,omitempty"`

	// Computed (not stored)
	OnlineCount int64  `gorm:"-" json:"online_count"`
	// UserCount: non-admin users whose group_id is in this node's mapped groups
	// (entitled / can pull this node). Distinct from online_count (currently connected).
	UserCount int64  `gorm:"-" json:"user_count"`
	Status    string `gorm:"-" json:"status"`    // online/warning/offline/disabled
	GroupIDs  []uint `gorm:"-" json:"group_ids"` // loaded from node_group_mappings
}

// NodeStatus returns the computed status based on last heartbeat time.
func (n *Node) ComputeStatus() string {
	if !n.Enable {
		return "disabled"
	}
	if n.LastHeartbeatAt == nil {
		return "offline"
	}
	elapsed := time.Since(*n.LastHeartbeatAt)
	switch {
	case elapsed < 2*time.Minute:
		return "online"
	case elapsed < 5*time.Minute:
		return "warning"
	default:
		return "offline"
	}
}

func (Node) TableName() string {
	return "nodes"
}

// RealityConfig represents the REALITY protocol settings (incl. optional PQ extensions).
// Missing optional fields keep legacy node behaviour (PQ off, min_client_ver applied only at UniProxy).
type RealityConfig struct {
	ServerName    string   `json:"server_name"`
	ServerPort    int      `json:"server_port"`
	PublicKey     string   `json:"public_key"`
	PrivateKey    string   `json:"private_key"`
	ShortID       string   `json:"short_id"`
	Fingerprint   string   `json:"fingerprint"`
	Dest          string   `json:"dest"`
	ServerNames   []string `json:"server_names,omitempty"`
	MinClientVer  string   `json:"min_client_ver,omitempty"`
	Mldsa65Seed   string   `json:"mldsa65_seed,omitempty"`
	Mldsa65Verify string   `json:"mldsa65_verify,omitempty"`
	Show          bool     `json:"show,omitempty"`
}
