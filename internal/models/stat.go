package models

// StatServer stores daily aggregated traffic per node.
type StatServer struct {
	ID       uint  `gorm:"primaryKey" json:"id"`
	RecordAt int64 `gorm:"uniqueIndex:idx_stat_server_day;not null" json:"record_at"`
	NodeID   uint  `gorm:"uniqueIndex:idx_stat_server_day;not null" json:"node_id"`
	Upload   int64 `gorm:"default:0" json:"upload"`
	Download int64 `gorm:"default:0" json:"download"`
}

func (StatServer) TableName() string { return "stat_servers" }

// StatUser stores daily aggregated traffic per user.
type StatUser struct {
	ID       uint  `gorm:"primaryKey" json:"id"`
	RecordAt int64 `gorm:"uniqueIndex:idx_stat_user_day;not null" json:"record_at"`
	UserID   uint  `gorm:"uniqueIndex:idx_stat_user_day;not null" json:"user_id"`
	Upload   int64 `gorm:"default:0" json:"upload"`
	Download int64 `gorm:"default:0" json:"download"`
}

func (StatUser) TableName() string { return "stat_users" }
