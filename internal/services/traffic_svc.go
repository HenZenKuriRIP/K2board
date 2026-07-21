package services

import (
	"fmt"
	"net"
	"strings"
	"time"

	"gorm.io/gorm"

	"K2board/internal/database"
	"K2board/internal/models"
)

// OnlineTTL is how long a node_online record is considered valid.
const OnlineTTL = 5 * time.Minute

// Caps for UniProxy /alive payloads (malicious or buggy nodes).
const (
	maxOnlineIPsPerUser = 32
	maxOnlineIPLen      = 45 // IPv6 textual max
)

// OnlineCutoff returns the earliest CreatedAt still considered online.
func OnlineCutoff() time.Time {
	return time.Now().Add(-OnlineTTL)
}

type TrafficService struct{}

func NewTrafficService() *TrafficService {
	return &TrafficService{}
}

// RecordTraffic logs upload/download traffic for a user on a node,
// and increments the user's total traffic_used.
func (s *TrafficService) RecordTraffic(userID, nodeID uint, upload, download int64) error {
	return database.DB.Transaction(func(tx *gorm.DB) error {
		log := &models.TrafficLog{
			UserID:   userID,
			NodeID:   nodeID,
			Upload:   upload,
			Download: download,
		}
		if err := tx.Create(log).Error; err != nil {
			return err
		}
		return tx.Model(&models.User{}).
			Where("id = ?", userID).
			UpdateColumn("traffic_used", gorm.Expr("traffic_used + ?", upload+download)).Error
	})
}

// SanitizeOnlineIP accepts only valid IPv4/IPv6 text (no host:port, no junk).
// Empty string means reject. Pure function — safe for unit tests without DB.
func SanitizeOnlineIP(ip string) string {
	ip = strings.TrimSpace(ip)
	if ip == "" || len(ip) > maxOnlineIPLen {
		return ""
	}
	// Reject host:port and accidental URLs
	if strings.Contains(ip, "://") || strings.Contains(ip, "/") {
		return ""
	}
	// If "ip:port", take host only when it parses as addr
	if host, _, err := net.SplitHostPort(ip); err == nil {
		ip = host
	}
	if net.ParseIP(ip) == nil {
		return ""
	}
	return ip
}

// RecordNodeOnline logs online user IPs for a node.
// Clears previous records for this node and stale records before inserting.
//
// Security (aligned with ValidateTrafficUsers / PushTraffic):
//   - Only users in the node's mapped enabled groups are accepted
//   - groupIDs empty → no inserts (unmapped node)
//   - IPs must pass SanitizeOnlineIP; max maxOnlineIPsPerUser per user
//
// Does not change how legitimate XrayR4u alive reports work for in-group users.
func (s *TrafficService) RecordNodeOnline(nodeID uint, groupIDs []uint, onlineMap map[uint][]string) error {
	return database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("node_id = ?", nodeID).Delete(&models.NodeOnline{}).Error; err != nil {
			return err
		}
		if err := tx.Where("created_at < ?", OnlineCutoff()).Delete(&models.NodeOnline{}).Error; err != nil {
			return err
		}

		if len(groupIDs) == 0 || len(onlineMap) == 0 {
			return nil
		}

		ids := make([]uint, 0, len(onlineMap))
		for uid := range onlineMap {
			ids = append(ids, uid)
		}
		nowUnix := time.Now().Unix()
		var allowed []models.User
		if err := tx.Select("id").
			Where("id IN ? AND enable = ? AND is_admin = ?", ids, true, false).
			Where("expire_at = 0 OR expire_at > ?", nowUnix).
			Where("group_id IN (SELECT id FROM groups WHERE enable = true AND id IN ?)", groupIDs).
			Find(&allowed).Error; err != nil {
			return err
		}
		allowedSet := make(map[uint]bool, len(allowed))
		for _, u := range allowed {
			allowedSet[u.ID] = true
		}

		now := time.Now()
		for userID, ips := range onlineMap {
			if !allowedSet[userID] {
				continue
			}
			if err := tx.Model(&models.User{}).Where("id = ?", userID).Update("last_active_at", now).Error; err != nil {
				return err
			}
			seen := make(map[string]struct{})
			for _, raw := range ips {
				if len(seen) >= maxOnlineIPsPerUser {
					break
				}
				ip := SanitizeOnlineIP(raw)
				if ip == "" {
					continue
				}
				if _, ok := seen[ip]; ok {
					continue
				}
				seen[ip] = struct{}{}
				record := &models.NodeOnline{NodeID: nodeID, UserID: userID, IP: ip}
				if err := tx.Create(record).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// PurgeStaleOnline deletes node_online rows older than OnlineTTL.
func (s *TrafficService) PurgeStaleOnline() (int64, error) {
	res := database.DB.Where("created_at < ?", OnlineCutoff()).Delete(&models.NodeOnline{})
	return res.RowsAffected, res.Error
}

// NodeMetricRetention is how long heartbeat time-series rows are kept for admin charts.
// Default 14 days covers Metrics(hours=24|168) with headroom without unbounded growth.
const NodeMetricRetention = 14 * 24 * time.Hour

// PurgeOldNodeMetrics deletes node_metrics older than NodeMetricRetention.
func (s *TrafficService) PurgeOldNodeMetrics() (int64, error) {
	cutoff := time.Now().Add(-NodeMetricRetention)
	res := database.DB.Where("created_at < ?", cutoff).Delete(&models.NodeMetric{})
	return res.RowsAffected, res.Error
}

// CountDistinctOnlineUsers returns distinct online user IDs within OnlineTTL.
func CountDistinctOnlineUsers() int64 {
	var count int64
	database.DB.Model(&models.NodeOnline{}).
		Where("created_at > ?", OnlineCutoff()).
		Select("COUNT(DISTINCT user_id)").
		Scan(&count)
	return count
}

// ListOnlineUserIDs returns user IDs currently online (within OnlineTTL).
func ListOnlineUserIDs() []uint {
	var ids []uint
	database.DB.Model(&models.NodeOnline{}).
		Where("created_at > ?", OnlineCutoff()).
		Distinct("user_id").
		Pluck("user_id", &ids)
	if ids == nil {
		ids = []uint{}
	}
	return ids
}

// CountOnlineUsersOnNode returns distinct online users on a node within OnlineTTL.
func CountOnlineUsersOnNode(nodeID uint) int64 {
	var count int64
	database.DB.Model(&models.NodeOnline{}).
		Where("node_id = ? AND created_at > ?", nodeID, OnlineCutoff()).
		Select("COUNT(DISTINCT user_id)").
		Scan(&count)
	return count
}

// DeviceIPCounts returns map[userID]distinctIPCount for the given users (OnlineTTL window).
func DeviceIPCounts(userIDs []uint) map[uint]int64 {
	out := make(map[uint]int64)
	if len(userIDs) == 0 {
		return out
	}
	var rows []models.NodeOnline
	database.DB.Where("user_id IN ? AND created_at > ?", userIDs, OnlineCutoff()).Find(&rows)
	seen := make(map[string]struct{})
	for _, r := range rows {
		key := fmt.Sprintf("%d\x00%s", r.UserID, r.IP)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out[r.UserID]++
	}
	return out
}

// LoadOnlineIPsForUsers fills device counts and unique IPs (OnlineTTL, cross-node dedupe).
func LoadOnlineIPsForUsers(users []models.User) {
	if len(users) == 0 {
		return
	}
	ids := make([]uint, len(users))
	for i, u := range users {
		ids[i] = u.ID
	}
	var onlineRows []models.NodeOnline
	database.DB.Where("user_id IN ? AND created_at > ?", ids, OnlineCutoff()).Find(&onlineRows)
	ipMap := make(map[uint][]string)
	ipCount := make(map[uint]int64)
	seen := make(map[string]bool)
	for _, r := range onlineRows {
		sk := fmt.Sprintf("%d-%s", r.UserID, r.IP)
		if seen[sk] {
			continue
		}
		seen[sk] = true
		ipMap[r.UserID] = append(ipMap[r.UserID], r.IP)
		ipCount[r.UserID]++
	}
	for i := range users {
		users[i].DeviceCount = ipCount[users[i].ID]
		if ips, ok := ipMap[users[i].ID]; ok {
			users[i].OnlineIPs = ips
		} else {
			users[i].OnlineIPs = []string{}
		}
	}
}

// QueryTrafficLogs returns paginated traffic logs with optional filters.
func (s *TrafficService) QueryTrafficLogs(userID, nodeID uint, page, pageSize int) ([]models.TrafficLog, int64, error) {
	var logs []models.TrafficLog
	var total int64

	query := database.DB.Model(&models.TrafficLog{})
	if userID > 0 {
		query = query.Where("user_id = ?", userID)
	}
	if nodeID > 0 {
		query = query.Where("node_id = ?", nodeID)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	if err := query.Order("id DESC").Offset(offset).Limit(pageSize).Find(&logs).Error; err != nil {
		return nil, 0, err
	}

	return logs, total, nil
}

// GetActiveUsers returns users with enable=true, not expired, and not over traffic.
func (s *TrafficService) GetActiveUsers() ([]models.User, error) {
	var users []models.User
	err := activeUserQuery().Find(&users).Error
	return users, err
}

// GetActiveUsersByGroup returns active users allowed on a specific node.
// Strict access: empty groupIDs (no node↔group mapping) → no users.
// Non-empty → only users whose group_id is in the enabled mapped groups.
// Users over device_limit are excluded so nodes drop them on next pull.
func (s *TrafficService) GetActiveUsersByGroup(nodeID uint, groupIDs []uint) ([]models.User, error) {
	var node models.Node
	if err := database.DB.Select("enable").First(&node, nodeID).Error; err != nil {
		return nil, err
	}
	if !node.Enable {
		return []models.User{}, nil
	}
	// Unmapped node: open to nobody (prevents free-for-all misconfiguration)
	if len(groupIDs) == 0 {
		return []models.User{}, nil
	}

	var users []models.User
	query := activeUserQuery().
		Where("group_id IN (SELECT id FROM groups WHERE enable = true AND id IN ?)", groupIDs)
	if err := query.Find(&users).Error; err != nil {
		return nil, err
	}
	return filterOverDeviceLimit(users), nil
}

func activeUserQuery() *gorm.DB {
	return database.DB.Where("enable = ?", true).
		Where("is_admin = ?", false).
		Where("(traffic_limit = 0 OR traffic_used < traffic_limit)").
		Where("expire_at = 0 OR expire_at > ?", time.Now().Unix())
}

// filterOverDeviceLimit drops users whose unique online IP count exceeds device_limit.
func filterOverDeviceLimit(users []models.User) []models.User {
	if len(users) == 0 {
		return users
	}
	ids := make([]uint, 0, len(users))
	needCheck := false
	for _, u := range users {
		if u.DeviceLimit > 0 {
			needCheck = true
		}
		ids = append(ids, u.ID)
	}
	if !needCheck {
		return users
	}
	counts := DeviceIPCounts(ids)
	out := make([]models.User, 0, len(users))
	for _, u := range users {
		if u.DeviceLimit > 0 && counts[u.ID] > int64(u.DeviceLimit) {
			continue
		}
		out = append(out, u)
	}
	return out
}

// NodesVisibleToUser returns enabled nodes the user is allowed to use.
// Inverse of GetActiveUsersByGroup (strict):
//   - Unmapped nodes (no node_group_mappings): visible to nobody
//   - User group_id=0 or disabled/missing group: no nodes
//   - Otherwise: only enabled nodes mapped to the user's enabled group
// Order is stable id ASC (no rotation — used by UniProxy / entitlement checks).
func NodesVisibleToUser(user *models.User) ([]models.Node, error) {
	if user == nil || user.GroupID == 0 {
		return []models.Node{}, nil
	}
	var g models.Group
	if err := database.DB.Select("id", "enable").First(&g, user.GroupID).Error; err != nil || !g.Enable {
		return []models.Node{}, nil
	}

	var nodes []models.Node
	err := database.DB.Where("enable = ?", true).
		Where(
			`id IN (
				SELECT ngm.node_id FROM node_group_mappings ngm
				INNER JOIN groups g ON g.id = ngm.group_id
				WHERE ngm.group_id = ? AND g.enable = true
			)`,
			user.GroupID,
		).
		Order("id ASC").
		Find(&nodes).Error
	if err != nil {
		return nil, err
	}
	return nodes, nil
}

// NodesForSubscribe returns the same visibility set as NodesVisibleToUser, but
// with fair first-position rotation for client subscription lists (Clash/V2Ray/…).
// UniProxy must keep using NodesVisibleToUser (no rotation).
func NodesForSubscribe(user *models.User) ([]models.Node, error) {
	nodes, err := NodesVisibleToUser(user)
	if err != nil {
		return nil, err
	}
	return RotateNodesFair(user, nodes), nil
}
