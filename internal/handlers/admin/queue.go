package admin

import (
	"time"

	"github.com/gin-gonic/gin"

	"K2board/internal/config"
	"K2board/internal/database"
	"K2board/internal/models"
	"K2board/internal/queue"
	"K2board/internal/utils"
)

type QueueHandler struct{}

func NewQueueHandler() *QueueHandler { return &QueueHandler{} }

func (h *QueueHandler) Stats(c *gin.Context) {
	m := queue.Metrics.Snapshot()

	var uptime int
	if m.StartedAt != nil {
		uptime = int(time.Since(*m.StartedAt).Seconds())
	}

	var todayTraffic struct{ Upload, Download int64 }
	today := time.Now().Truncate(24 * time.Hour)
	database.DB.Model(&models.StatServer{}).
		Select("COALESCE(SUM(upload),0) as upload, COALESCE(SUM(download),0) as download").
		Where("record_at >= ?", today.Unix()).
		Scan(&todayTraffic)

	var totalUsers, enabledUsers, totalNodes, enabledNodes int64
	database.DB.Model(&models.User{}).Count(&totalUsers)
	database.DB.Model(&models.User{}).Where("enable = ?", true).Count(&enabledUsers)
	database.DB.Model(&models.Node{}).Count(&totalNodes)
	database.DB.Model(&models.Node{}).Where("enable = ?", true).Count(&enabledNodes)

	flushInterval := config.AppConfig.Scheduler.FlushInterval
	if flushInterval <= 0 {
		flushInterval = 60
	}
	statsInterval := config.AppConfig.Scheduler.StatsInterval
	if statsInterval <= 0 {
		statsInterval = 300
	}
	autoDisableInterval := config.AppConfig.Scheduler.AutoDisableInterval
	if autoDisableInterval <= 0 {
		autoDisableInterval = 60
	}
	const resetInterval = 3600 // fixed in scheduler

	// last_flush_count kept for backward compatibility (= last success)
	utils.Success(c, gin.H{
		"queue": gin.H{
			"total_flushes":        m.TotalFlushes,
			"total_empty_flushes":  m.TotalEmptyFlushes,
			"total_entries":        m.TotalEntries,
			"total_flush_failed":   m.TotalFlushFailed,
			"last_flush_at":        m.LastFlushAt,
			"last_flush_count":     m.LastFlushSuccess, // legacy alias
			"last_flush_pending":   m.LastFlushPending,
			"last_flush_success":   m.LastFlushSuccess,
			"last_flush_failed":    m.LastFlushFailed,
			"buffer_size":          queue.DefaultStore.Size(),
			"store_type":           m.StoreType,
		},
		"scheduler": gin.H{
			"started_at":              m.StartedAt,
			"uptime_seconds":          uptime,
			"total_aggregations":      m.TotalAggregations,
			"last_aggregation_at":     m.LastAggregationAt,
			"last_aggregation_nodes":  m.LastAggregationNodes,
			"last_aggregation_users":  m.LastAggregationUsers,
			"total_disable_runs":      m.TotalDisableRuns,
			"total_disabled":          m.TotalDisabled,
			"last_disable_at":         m.LastDisableAt,
			"last_disabled":           m.LastDisabled,
			"total_reset_runs":        m.TotalResetRuns,
			"total_reset":             m.TotalReset,
			"last_reset_at":           m.LastResetAt,
			"last_reset":              m.LastReset,
			"total_purge_runs":        m.TotalPurgeRuns,
			"total_purged":            m.TotalPurged,
			"last_purge_at":           m.LastPurgeAt,
			"last_purged":             m.LastPurged,
		},
		"scheduler_config": gin.H{
			"flush_interval":        flushInterval,
			"stats_interval":        statsInterval,
			"auto_disable_interval": autoDisableInterval,
			"reset_interval":        resetInterval,
		},
		"pipelines": gin.H{
			"flush": gin.H{
				"id":          "flush",
				"name":        "流量缓冲刷盘",
				"writes":      []string{"traffic_logs (insert)", "users.traffic_used (+=)"},
				"interval":    flushInterval,
				"last_at":     m.LastFlushAt,
				"last_success": m.LastFlushSuccess,
				"last_failed": m.LastFlushFailed,
				"last_pending": m.LastFlushPending,
				"total_runs":  m.TotalFlushes,
				"total_ok":    m.TotalEntries,
				"total_fail":  m.TotalFlushFailed,
				"buffer_size": queue.DefaultStore.Size(),
				"store_type":  m.StoreType,
			},
			"aggregate": gin.H{
				"id":       "aggregate",
				"name":     "日统计聚合",
				"writes":   []string{"stat_servers (upsert)", "stat_users (upsert)"},
				"interval": statsInterval,
				"last_at":  m.LastAggregationAt,
				"last_nodes": m.LastAggregationNodes,
				"last_users": m.LastAggregationUsers,
				"total_runs": m.TotalAggregations,
			},
			"disable": gin.H{
				"id":   "disable",
				"name": "账号维护（过期不改 enable）",
				// enable = admin ban only; expire_at gates service. Scheduler no longer
				// sets enable=false on expiry. First run after upgrade may re-enable
				// legacy auto-disabled expired accounts once.
				"writes":       []string{"users.enable = true (one-time legacy repair only)", "config_version++ (if repair)"},
				"side_effects": []string{"RefreshConfigVersion", "PurgeStaleOnline"},
				"interval":     autoDisableInterval,
				"last_at":      m.LastDisableAt,
				"last_affected": m.LastDisabled,
				"total_runs":   m.TotalDisableRuns,
				"total_affected": m.TotalDisabled,
				"last_purge_at": m.LastPurgeAt,
				"last_purged":   m.LastPurged,
				"total_purged":  m.TotalPurged,
			},
			"reset": gin.H{
				"id":             "reset",
				"name":           "月流量自动重置",
				"writes":         []string{"users.traffic_used = 0 (by plan.reset_day)"},
				"interval":       resetInterval,
				"last_at":        m.LastResetAt,
				"last_affected":  m.LastReset,
				"total_runs":     m.TotalResetRuns,
				"total_affected": m.TotalReset,
			},
		},
		"recent": m.Recent,
		"stats": gin.H{
			"today_upload":   todayTraffic.Upload,
			"today_download": todayTraffic.Download,
			"total_users":    totalUsers,
			"enabled_users":  enabledUsers,
			"total_nodes":    totalNodes,
			"enabled_nodes":  enabledNodes,
			"today_note":     "今日流量来自 stat_servers 聚合表，依赖日统计管道；不含尚未刷盘的缓冲流量",
		},
	})
}
