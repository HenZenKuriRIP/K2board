package admin

import (
	"fmt"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"K2board/internal/config"
	"K2board/internal/database"
	"K2board/internal/models"
	"K2board/internal/utils"
)

type TrafficHandler struct{}

func NewTrafficHandler() *TrafficHandler {
	return &TrafficHandler{}
}

func isMySQL() bool {
	return config.AppConfig.Database.Driver == "mysql"
}

// seriesBucketExpr returns SQL for time-series buckets.
// short window (<=48h): hour buckets; longer: day buckets.
func seriesBucketExpr(hours int) (selectExpr, groupExpr, orderExpr string) {
	if hours <= 48 {
		if isMySQL() {
			e := "DATE_FORMAT(recorded_at, '%Y-%m-%d %H:00')"
			return e + " as bucket", e, e + " ASC"
		}
		e := "TO_CHAR(date_trunc('hour', recorded_at), 'YYYY-MM-DD HH24:00')"
		return e + " as bucket", "date_trunc('hour', recorded_at)", "date_trunc('hour', recorded_at) ASC"
	}
	if isMySQL() {
		e := "DATE_FORMAT(recorded_at, '%Y-%m-%d')"
		return e + " as bucket", e, e + " ASC"
	}
	e := "TO_CHAR(date_trunc('day', recorded_at), 'YYYY-MM-DD')"
	return e + " as bucket", "date_trunc('day', recorded_at)", "date_trunc('day', recorded_at) ASC"
}

// Stats returns aggregated traffic analytics tuned for large user bases.
// Always aggregates in SQL — never loads raw logs into memory.
func (h *TrafficHandler) Stats(c *gin.Context) {
	hours, _ := strconv.Atoi(c.DefaultQuery("hours", "24"))
	if hours < 1 {
		hours = 24
	}
	if hours > 720 {
		hours = 720 // max 30 days
	}
	userIDStr := c.Query("user_id")
	nodeIDStr := c.Query("node_id")

	since := time.Now().Add(-time.Duration(hours) * time.Hour)

	base := database.DB.Model(&models.TrafficLog{}).Where("recorded_at >= ?", since)
	if userIDStr != "" {
		if id, err := strconv.ParseUint(userIDStr, 10, 64); err == nil && id > 0 {
			base = base.Where("user_id = ?", uint(id))
		}
	}
	if nodeIDStr != "" {
		if id, err := strconv.ParseUint(nodeIDStr, 10, 64); err == nil && id > 0 {
			base = base.Where("node_id = ?", uint(id))
		}
	}

	// Totals + active users + row count (scale signal)
	var total struct {
		Upload      int64
		Download    int64
		ActiveUsers int64
		LogRows     int64
	}
	base.Select(`
		COALESCE(SUM(upload),0) as upload,
		COALESCE(SUM(download),0) as download,
		COUNT(DISTINCT user_id) as active_users,
		COUNT(*) as log_rows
	`).Scan(&total)

	// Time series (hour or day buckets)
	type SeriesPoint struct {
		Bucket   string `json:"bucket"`
		Upload   int64  `json:"upload"`
		Download int64  `json:"download"`
	}
	var series []SeriesPoint
	sel, grp, ord := seriesBucketExpr(hours)
	seriesQ := database.DB.Model(&models.TrafficLog{}).Where("recorded_at >= ?", since)
	if userIDStr != "" {
		if id, err := strconv.ParseUint(userIDStr, 10, 64); err == nil && id > 0 {
			seriesQ = seriesQ.Where("user_id = ?", uint(id))
		}
	}
	if nodeIDStr != "" {
		if id, err := strconv.ParseUint(nodeIDStr, 10, 64); err == nil && id > 0 {
			seriesQ = seriesQ.Where("node_id = ?", uint(id))
		}
	}
	seriesQ.Select(fmt.Sprintf("%s, SUM(upload) as upload, SUM(download) as download", sel)).
		Group(grp).Order(ord).Scan(&series)

	// User ranking — top N only (never return thousands of rows)
	rankLimit := 30
	type UserRank struct {
		UserID   uint   `json:"user_id"`
		Email    string `json:"email"`
		Upload   int64  `json:"upload"`
		Download int64  `json:"download"`
	}
	var ranking []UserRank
	rankQ := database.DB.Model(&models.TrafficLog{}).
		Select("traffic_logs.user_id, COALESCE(users.email,'') as email, SUM(upload) as upload, SUM(download) as download").
		Joins("LEFT JOIN users ON users.id = traffic_logs.user_id").
		Where("traffic_logs.recorded_at >= ?", since)
	if userIDStr != "" {
		if id, err := strconv.ParseUint(userIDStr, 10, 64); err == nil && id > 0 {
			rankQ = rankQ.Where("traffic_logs.user_id = ?", uint(id))
		}
	}
	if nodeIDStr != "" {
		if id, err := strconv.ParseUint(nodeIDStr, 10, 64); err == nil && id > 0 {
			rankQ = rankQ.Where("traffic_logs.node_id = ?", uint(id))
		}
	}
	rankQ.Group("traffic_logs.user_id, users.email").
		Order("SUM(upload) + SUM(download) DESC").
		Limit(rankLimit).
		Scan(&ranking)

	// Node breakdown — top nodes only; zero-traffic nodes omitted (scale)
	type NodeStat struct {
		NodeID   uint   `json:"node_id"`
		Name     string `json:"name"`
		Upload   int64  `json:"upload"`
		Download int64  `json:"download"`
	}
	var nodes []NodeStat
	nodeQ := database.DB.Model(&models.TrafficLog{}).
		Select("traffic_logs.node_id, COALESCE(nodes.name, CONCAT('#', traffic_logs.node_id)) as name, SUM(upload) as upload, SUM(download) as download").
		Joins("LEFT JOIN nodes ON nodes.id = traffic_logs.node_id").
		Where("traffic_logs.recorded_at >= ?", since).
		Where("traffic_logs.node_id > 0")
	if userIDStr != "" {
		if id, err := strconv.ParseUint(userIDStr, 10, 64); err == nil && id > 0 {
			nodeQ = nodeQ.Where("traffic_logs.user_id = ?", uint(id))
		}
	}
	if nodeIDStr != "" {
		if id, err := strconv.ParseUint(nodeIDStr, 10, 64); err == nil && id > 0 {
			nodeQ = nodeQ.Where("traffic_logs.node_id = ?", uint(id))
		}
	}
	// MySQL CONCAT works; for postgres CONCAT also works. Safer expression:
	if !isMySQL() {
		nodeQ = database.DB.Model(&models.TrafficLog{}).
			Select("traffic_logs.node_id, COALESCE(nodes.name, '#' || traffic_logs.node_id::text) as name, SUM(upload) as upload, SUM(download) as download").
			Joins("LEFT JOIN nodes ON nodes.id = traffic_logs.node_id").
			Where("traffic_logs.recorded_at >= ?", since).
			Where("traffic_logs.node_id > 0")
		if userIDStr != "" {
			if id, err := strconv.ParseUint(userIDStr, 10, 64); err == nil && id > 0 {
				nodeQ = nodeQ.Where("traffic_logs.user_id = ?", uint(id))
			}
		}
		if nodeIDStr != "" {
			if id, err := strconv.ParseUint(nodeIDStr, 10, 64); err == nil && id > 0 {
				nodeQ = nodeQ.Where("traffic_logs.node_id = ?", uint(id))
			}
		}
	}
	nodeQ.Group("traffic_logs.node_id, nodes.name").
		Order("SUM(upload) + SUM(download) DESC").
		Limit(20).
		Scan(&nodes)

	// Share percentages for ranking / nodes
	grand := total.Upload + total.Download
	type RankOut struct {
		UserID   uint    `json:"user_id"`
		Email    string  `json:"email"`
		Upload   int64   `json:"upload"`
		Download int64   `json:"download"`
		Total    int64   `json:"total"`
		Share    float64 `json:"share"`
	}
	rankOut := make([]RankOut, 0, len(ranking))
	for _, r := range ranking {
		t := r.Upload + r.Download
		share := 0.0
		if grand > 0 {
			share = float64(t) / float64(grand) * 100
		}
		email := r.Email
		if email == "" {
			email = fmt.Sprintf("user#%d", r.UserID)
		}
		rankOut = append(rankOut, RankOut{
			UserID: r.UserID, Email: email,
			Upload: r.Upload, Download: r.Download,
			Total: t, Share: share,
		})
	}

	type NodeOut struct {
		NodeID   uint    `json:"node_id"`
		Name     string  `json:"name"`
		Upload   int64   `json:"upload"`
		Download int64   `json:"download"`
		Total    int64   `json:"total"`
		Share    float64 `json:"share"`
	}
	nodeOut := make([]NodeOut, 0, len(nodes))
	for _, n := range nodes {
		t := n.Upload + n.Download
		share := 0.0
		if grand > 0 {
			share = float64(t) / float64(grand) * 100
		}
		name := n.Name
		if name == "" {
			name = fmt.Sprintf("#%d", n.NodeID)
		}
		nodeOut = append(nodeOut, NodeOut{
			NodeID: n.NodeID, Name: name,
			Upload: n.Upload, Download: n.Download,
			Total: t, Share: share,
		})
	}

	// Peak bucket
	var peakBucket string
	var peakTotal int64
	for _, s := range series {
		t := s.Upload + s.Download
		if t > peakTotal {
			peakTotal = t
			peakBucket = s.Bucket
		}
	}

	avgPerUser := int64(0)
	if total.ActiveUsers > 0 {
		avgPerUser = grand / total.ActiveUsers
	}

	granularity := "hour"
	if hours > 48 {
		granularity = "day"
	}

	utils.Success(c, gin.H{
		"total_upload":   total.Upload,
		"total_download": total.Download,
		"total":          grand,
		"active_users":   total.ActiveUsers,
		"log_rows":       total.LogRows,
		"avg_per_user":   avgPerUser,
		"peak_bucket":    peakBucket,
		"peak_total":     peakTotal,
		"series":         series,
		"granularity":    granularity,
		// legacy key for any old clients
		"hourly":     series,
		"ranking":    rankOut,
		"nodes":      nodeOut,
		"hours":      hours,
		"since":      since.UTC().Format(time.RFC3339),
		"rank_limit": rankLimit,
		"note":       "聚合在 SQL 完成；排行/节点仅返回 Top N。明细请走 /traffic-logs 并带过滤条件。",
	})
}

// List returns paginated traffic log rows with user/node labels.
// Designed for drill-down: always prefer filtering by user_id / node_id / time.
func (h *TrafficHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	userIDStr := c.Query("user_id")
	nodeIDStr := c.Query("node_id")
	email := c.Query("email")
	hours, _ := strconv.Atoi(c.DefaultQuery("hours", "24"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	if hours < 1 {
		hours = 24
	}
	if hours > 720 {
		hours = 720
	}

	since := time.Now().Add(-time.Duration(hours) * time.Hour)

	type Row struct {
		ID         uint      `json:"id"`
		UserID     uint      `json:"user_id"`
		Email      string    `json:"email"`
		NodeID     uint      `json:"node_id"`
		NodeName   string    `json:"node_name"`
		Upload     int64     `json:"upload"`
		Download   int64     `json:"download"`
		RecordedAt time.Time `json:"recorded_at"`
	}

	applyFilters := func(db *gorm.DB) *gorm.DB {
		db = db.Where("traffic_logs.recorded_at >= ?", since)
		if userIDStr != "" {
			if id, err := strconv.ParseUint(userIDStr, 10, 64); err == nil && id > 0 {
				db = db.Where("traffic_logs.user_id = ?", uint(id))
			}
		}
		if nodeIDStr != "" {
			if id, err := strconv.ParseUint(nodeIDStr, 10, 64); err == nil && id > 0 {
				db = db.Where("traffic_logs.node_id = ?", uint(id))
			}
		}
		if email != "" {
			if isMySQL() {
				db = db.Where("users.email LIKE ?", "%"+email+"%")
			} else {
				db = db.Where("users.email ILIKE ?", "%"+email+"%")
			}
		}
		return db
	}

	var total int64
	countQ := applyFilters(
		database.DB.Table("traffic_logs").
			Joins("LEFT JOIN users ON users.id = traffic_logs.user_id"),
	)
	if err := countQ.Count(&total).Error; err != nil {
		utils.InternalError(c, "failed to count traffic logs")
		return
	}

	q := applyFilters(
		database.DB.Table("traffic_logs").
			Select(`traffic_logs.id, traffic_logs.user_id, COALESCE(users.email,'') as email,
				traffic_logs.node_id, COALESCE(nodes.name,'') as node_name,
				traffic_logs.upload, traffic_logs.download, traffic_logs.recorded_at`).
			Joins("LEFT JOIN users ON users.id = traffic_logs.user_id").
			Joins("LEFT JOIN nodes ON nodes.id = traffic_logs.node_id"),
	)

	var rows []Row
	offset := (page - 1) * pageSize
	if err := q.Order("traffic_logs.id DESC").Offset(offset).Limit(pageSize).Scan(&rows).Error; err != nil {
		utils.InternalError(c, "failed to fetch traffic logs")
		return
	}

	// Fill display names
	for i := range rows {
		if rows[i].Email == "" {
			rows[i].Email = fmt.Sprintf("user#%d", rows[i].UserID)
		}
		if rows[i].NodeName == "" {
			rows[i].NodeName = fmt.Sprintf("#%d", rows[i].NodeID)
		}
	}

	utils.Success(c, gin.H{
		"list":      rows,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
		"hours":     hours,
		"filtered":  userIDStr != "" || nodeIDStr != "" || email != "",
	})
}
