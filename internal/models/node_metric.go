package models

import "time"

// NodeMetric stores time-series node health data for trend charts.
type NodeMetric struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	NodeID    uint      `gorm:"index;not null" json:"node_id"`
	CPU       float64   `json:"cpu"`
	Mem       float64   `json:"mem"`
	Disk      float64   `json:"disk"`
	Uptime      int       `json:"uptime"`
	ActiveConns int       `json:"active_conns"`
	CreatedAt time.Time `gorm:"autoCreateTime;index" json:"created_at"`
}

func (NodeMetric) TableName() string { return "node_metrics" }
