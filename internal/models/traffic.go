package models

import "time"

// TrafficLog records upload/download traffic per user per node.
// Flush creates one row per (user, node) buffer key each cycle — grows quickly
// at scale; prefer aggregated stats endpoints for analytics.
type TrafficLog struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	UserID     uint      `gorm:"index:idx_traffic_user_time;index;not null" json:"user_id"`
	NodeID     uint      `gorm:"index:idx_traffic_node_time;index;not null" json:"node_id"`
	Upload     int64     `gorm:"default:0" json:"upload"`
	Download   int64     `gorm:"default:0" json:"download"`
	RecordedAt time.Time `gorm:"autoCreateTime;index;index:idx_traffic_user_time;index:idx_traffic_node_time" json:"recorded_at"`
}

func (TrafficLog) TableName() string {
	return "traffic_logs"
}
