package queue

import (
	"context"
	"log/slog"
	"time"

	"K2board/internal/config"
	"K2board/internal/services"
)

// StartScheduler runs periodic background tasks.
func StartScheduler() {
	cfg := config.AppConfig.Scheduler

	// Traffic flush
	go func() {
		d := time.Duration(cfg.FlushInterval) * time.Second
		if d <= 0 {
			d = 60 * time.Second
		}
		ticker := time.NewTicker(d)
		defer ticker.Stop()
		for range ticker.C {
			result := DefaultStore.Flush()
			if result.Success > 0 || result.Failed > 0 {
				slog.Info("traffic flushed",
					"success", result.Success,
					"failed", result.Failed,
					"pending", result.Pending,
				)
			}
			RecordFlush(result)
		}
	}()

	// Daily stats aggregation
	go func() {
		d := time.Duration(cfg.StatsInterval) * time.Second
		if d <= 0 {
			d = 300 * time.Second
		}
		ticker := time.NewTicker(d)
		defer ticker.Stop()
		for range ticker.C {
			nodes, users := AggregateDailyStats()
			RecordAggregation(nodes, users)
		}
	}()

	// Monthly traffic auto-reset (plan.reset_day == today; check hourly, once per user/day)
	go func() {
		runReset := func() {
			n := services.AutoResetTraffic()
			RecordReset(n)
		}
		// Run once at startup so restart on reset-day does not wait a full hour
		runReset()
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			runReset()
		}
	}()

	// Account maintenance tick: one-time legacy repair (no expire→enable=false) + purge stale online
	go func() {
		d := time.Duration(cfg.AutoDisableInterval) * time.Second
		if d <= 0 {
			d = 60 * time.Second
		}
		ticker := time.NewTicker(d)
		defer ticker.Stop()
		trafficSvc := services.NewTrafficService()
		for range ticker.C {
			// Multi-instance: refresh config_version from DB
			services.RefreshConfigVersion()

			// Name kept for metrics; does not ban on expiry (enable = admin ban only)
			n := services.AutoDisableExpiredUsers()
			RecordDisable(n)

			if purged, err := trafficSvc.PurgeStaleOnline(); err != nil {
				slog.Warn("purge stale online failed", "error", err)
			} else {
				RecordPurge(purged)
				if purged > 0 {
					slog.Info("purged stale online records", "count", purged)
				}
			}
			// Bound node_metrics growth (heartbeat inserts every status tick)
			if n, err := trafficSvc.PurgeOldNodeMetrics(); err != nil {
				slog.Warn("purge old node metrics failed", "error", err)
			} else if n > 0 {
				slog.Info("purged old node metrics", "count", n, "retention", "14d")
			}
		}
	}()

	// Payment gateway reconcile (notify fallback: alipay query / epusdt status)
	go func() {
		ticker := time.NewTicker(2 * time.Minute)
		defer ticker.Stop()
		orders := services.NewOrderService()
		for range ticker.C {
			n := orders.ReconcileStalePending(context.Background(), 40)
			if n > 0 {
				slog.Info("payment reconcile fulfilled", "count", n)
			}
		}
	}()

	slog.Info("scheduler started",
		"flush", cfg.FlushInterval,
		"stats", cfg.StatsInterval,
		"auto_disable", cfg.AutoDisableInterval,
	)
}
