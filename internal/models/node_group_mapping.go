package models

// NodeGroupMapping is the junction table for many-to-many relationship between nodes and groups.
// Replaces the legacy single group_id column on nodes with flexible multi-group support.
type NodeGroupMapping struct {
	ID      uint `gorm:"primaryKey" json:"id"`
	NodeID  uint `gorm:"uniqueIndex:idx_node_group;not null" json:"node_id"`
	GroupID uint `gorm:"uniqueIndex:idx_node_group;not null" json:"group_id"`
}

func (NodeGroupMapping) TableName() string {
	return "node_group_mappings"
}
