package server

import (
	"time"

	"github.com/gin-gonic/gin"

	"K2board/internal/database"
	"K2board/internal/middleware"
	"K2board/internal/models"
)

// NodeStatusRequest is sent by XrayR4u to report node system status.
type NodeStatusRequest struct {
	CPU    float64 `json:"cpu"`
	Mem    float64 `json:"mem"`
	Disk        float64 `json:"disk"`
	Uptime      int     `json:"uptime"`
	ActiveConns int     `json:"active_conns"`
}

// ReportNodeStatus accepts node status reports and records heartbeat for status tracking.
func ReportNodeStatus(c *gin.Context) {
	node := middleware.GetCurrentNode(c)
	if node == nil {
		c.JSON(200, gin.H{"status": "ok"})
		return
	}

	var req NodeStatusRequest
	if err := c.ShouldBindJSON(&req); err == nil {
		database.DB.Model(node).Updates(map[string]any{"cpu": req.CPU, "mem": req.Mem, "disk": req.Disk, "uptime": req.Uptime, "active_conns": req.ActiveConns})
	database.DB.Create(&models.NodeMetric{NodeID: node.ID, CPU: req.CPU, Mem: req.Mem, Disk: req.Disk, Uptime: req.Uptime, ActiveConns: req.ActiveConns})
	}

	// Heartbeat is already recorded by NodeAuth middleware
	version := GetConfigVersion()
	c.JSON(200, gin.H{
		"status":         "ok",
		"config_version": version,
	})
}

// DetectRule represents an audit/detect rule.
type DetectRule struct {
	ID      int    `json:"id"`
	Pattern string `json:"pattern"`
}

// GetNodeRule returns audit/detect rules for the node.
func GetNodeRule(c *gin.Context) {
	c.JSON(200, []DetectRule{})
}

// DetectResult represents a single illegal access detection.
type DetectResult struct {
	RuleID int `json:"rule_id"`
	UID    int `json:"uid"`
}

// ReportIllegal accepts illegal access reports from the node.
func ReportIllegal(c *gin.Context) {
	var results []DetectResult
	if err := c.ShouldBindJSON(&results); err != nil {
		_ = err
	}
	c.JSON(200, gin.H{"status": "ok"})
}

// ValidateTrafficUsers returns the set of user IDs whose traffic may be recorded
// for the given node. It enforces:
//  1. User is active: enabled, not expired, not over traffic quota.
//  2. User belongs to one of the node's mapped groups (node-user binding).
//
// Rationale: a compromised node must not be able to inject traffic records for
// users outside its own group assignment. Traffic is already produced, so
// device-limit is intentionally NOT applied here (it only gates new connections).
//
// If groupIDs is empty the node has no group mapping → no users are valid
// (mirrors GetActiveUsersByGroup's strict "unmapped node = nobody" policy).
func ValidateTrafficUsers(nodeID uint, groupIDs []uint, trafficMap map[uint][]int64) map[uint]bool {
	valid := make(map[uint]bool)
	if len(trafficMap) == 0 || len(groupIDs) == 0 {
		return valid
	}

	ids := make([]uint, 0, len(trafficMap))
	for uid := range trafficMap {
		ids = append(ids, uid)
	}

	// Single query: active-user conditions + group binding check.
	// The sub-select mirrors GetActiveUsersByGroup's group filter so behaviour
	// is consistent between the pull (GetUser) and push (PushTraffic) paths.
	now := time.Now().Unix()
	var users []models.User
	database.DB.Select("id").
		Where("id IN ? AND enable = ? AND is_admin = ?", ids, true, false).
		Where("(traffic_limit = 0 OR traffic_used < traffic_limit)").
		Where("expire_at = 0 OR expire_at > ?", now).
		Where("group_id IN (SELECT id FROM groups WHERE enable = true AND id IN ?)", groupIDs).
		Find(&users)
	for _, u := range users {
		valid[u.ID] = true
	}
	return valid
}
