package queue

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

const maxJobEvents = 50

// JobEvent is one scheduler run result for the admin activity feed.
type JobEvent struct {
	Job       string    `json:"job"` // flush | aggregate | disable | reset | purge
	At        time.Time `json:"at"`
	Empty     bool      `json:"empty,omitempty"`
	Success   int       `json:"success,omitempty"`
	Failed    int       `json:"failed,omitempty"`
	Affected  int64     `json:"affected,omitempty"`
	Nodes     int       `json:"nodes,omitempty"`
	Users     int       `json:"users,omitempty"`
	Message   string    `json:"message"`
}

// QueueMetrics tracks runtime statistics for background pipelines.
// Counters are process-lifetime (reset on restart). Uses *time.Time so
// unset times serialize as null instead of zero-time.
type QueueMetrics struct {
	// Traffic buffer flush
	TotalFlushes       int64      `json:"total_flushes"`
	TotalEmptyFlushes  int64      `json:"total_empty_flushes"`
	TotalEntries       int64      `json:"total_entries"` // successful key writes cumulative
	TotalFlushFailed   int64      `json:"total_flush_failed"`
	LastFlushAt        *time.Time `json:"last_flush_at"`
	LastFlushPending   int        `json:"last_flush_pending"`
	LastFlushSuccess   int        `json:"last_flush_success"`
	LastFlushFailed    int        `json:"last_flush_failed"`

	// Stats aggregation
	TotalAggregations     int64      `json:"total_aggregations"`
	LastAggregationAt     *time.Time `json:"last_aggregation_at"`
	LastAggregationNodes  int        `json:"last_aggregation_nodes"`
	LastAggregationUsers  int        `json:"last_aggregation_users"`

	// Auto-disable
	TotalDisableRuns int64      `json:"total_disable_runs"`
	TotalDisabled    int64      `json:"total_disabled"`
	LastDisableAt    *time.Time `json:"last_disable_at"`
	LastDisabled     int64      `json:"last_disabled"`

	// Traffic auto-reset
	TotalResetRuns int64      `json:"total_reset_runs"`
	TotalReset     int64      `json:"total_reset"`
	LastResetAt    *time.Time `json:"last_reset_at"`
	LastReset      int64      `json:"last_reset"`

	// Online purge (same tick as auto-disable)
	TotalPurgeRuns int64      `json:"total_purge_runs"`
	TotalPurged    int64      `json:"total_purged"`
	LastPurgeAt    *time.Time `json:"last_purge_at"`
	LastPurged     int64      `json:"last_purged"`

	// System
	StartedAt *time.Time `json:"started_at"`
	StoreType string     `json:"store_type"` // "memory" or "redis"

	// Recent activity (newest first), not copied via atomic snapshot fields alone
	Recent []JobEvent `json:"recent,omitempty"`
}

var (
	Metrics = &QueueMetrics{
		StartedAt: timePtr(time.Now()),
		StoreType: "memory",
	}
	metricsMu sync.RWMutex
	events    []JobEvent // ring as slice, newest at end; Snapshot reverses
)

func timePtr(t time.Time) *time.Time { return &t }

func appendEvent(ev JobEvent) {
	events = append(events, ev)
	if len(events) > maxJobEvents {
		events = events[len(events)-maxJobEvents:]
	}
}

// FlushResult is returned by TrafficStore.Flush.
type FlushResult struct {
	Pending int // keys present when flush started
	Success int // keys written to DB
	Failed  int // keys requeued / restored
}

// RecordFlush records a traffic buffer flush operation.
func RecordFlush(r FlushResult) {
	atomic.AddInt64(&Metrics.TotalFlushes, 1)
	if r.Pending == 0 && r.Success == 0 && r.Failed == 0 {
		atomic.AddInt64(&Metrics.TotalEmptyFlushes, 1)
	}
	atomic.AddInt64(&Metrics.TotalEntries, int64(r.Success))
	if r.Failed > 0 {
		atomic.AddInt64(&Metrics.TotalFlushFailed, int64(r.Failed))
	}

	now := time.Now()
	metricsMu.Lock()
	Metrics.LastFlushAt = timePtr(now)
	Metrics.LastFlushPending = r.Pending
	Metrics.LastFlushSuccess = r.Success
	Metrics.LastFlushFailed = r.Failed

	msg := "流量刷盘"
	empty := r.Pending == 0 && r.Success == 0
	if empty {
		msg = "流量刷盘（缓冲为空）"
	} else if r.Failed > 0 {
		msg = fmt.Sprintf("流量刷盘：成功 %d，失败重试 %d", r.Success, r.Failed)
	} else {
		msg = fmt.Sprintf("流量刷盘：写入 traffic_logs + users.traffic_used，%d 条", r.Success)
	}
	appendEvent(JobEvent{
		Job:     "flush",
		At:      now,
		Empty:   empty,
		Success: r.Success,
		Failed:  r.Failed,
		Message: msg,
	})
	metricsMu.Unlock()
}

// RecordAggregation records a daily stats aggregation run.
func RecordAggregation(nodes, users int) {
	atomic.AddInt64(&Metrics.TotalAggregations, 1)
	now := time.Now()
	metricsMu.Lock()
	Metrics.LastAggregationAt = timePtr(now)
	Metrics.LastAggregationNodes = nodes
	Metrics.LastAggregationUsers = users
	appendEvent(JobEvent{
		Job:     "aggregate",
		At:      now,
		Empty:   nodes == 0 && users == 0,
		Nodes:   nodes,
		Users:   users,
		Message: fmt.Sprintf("日统计聚合：stat_servers %d 节点，stat_users %d 用户", nodes, users),
	})
	metricsMu.Unlock()
}

// RecordDisable records an account-maintenance tick (including empty runs).
// count is the number of users re-enabled by the one-time legacy repair
// (not "disabled"); metric field names are kept for API compatibility.
func RecordDisable(count int64) {
	atomic.AddInt64(&Metrics.TotalDisableRuns, 1)
	if count > 0 {
		atomic.AddInt64(&Metrics.TotalDisabled, count)
	}
	now := time.Now()
	metricsMu.Lock()
	Metrics.LastDisableAt = timePtr(now)
	Metrics.LastDisabled = count
	if count > 0 {
		appendEvent(JobEvent{
			Job:      "disable",
			At:       now,
			Affected: count,
			Message:  fmt.Sprintf("账号维护：修复 %d 个历史误封到期用户（enable 恢复）", count),
		})
	}
	metricsMu.Unlock()
}

// RecordReset records a traffic auto-reset run (including empty runs).
func RecordReset(count int64) {
	atomic.AddInt64(&Metrics.TotalResetRuns, 1)
	if count > 0 {
		atomic.AddInt64(&Metrics.TotalReset, count)
	}
	now := time.Now()
	metricsMu.Lock()
	Metrics.LastResetAt = timePtr(now)
	Metrics.LastReset = count
	if count > 0 {
		appendEvent(JobEvent{
			Job:      "reset",
			At:       now,
			Affected: count,
			Message:  fmt.Sprintf("流量自动重置：%d 个用户 traffic_used=0", count),
		})
	}
	metricsMu.Unlock()
}

// RecordPurge records a stale online purge (only when count > 0 for events;
// always updates last when called with any count including tracking runs).
func RecordPurge(count int64) {
	if count <= 0 {
		return
	}
	atomic.AddInt64(&Metrics.TotalPurgeRuns, 1)
	atomic.AddInt64(&Metrics.TotalPurged, count)
	now := time.Now()
	metricsMu.Lock()
	Metrics.LastPurgeAt = timePtr(now)
	Metrics.LastPurged = count
	appendEvent(JobEvent{
		Job:      "purge",
		At:       now,
		Affected: count,
		Message:  fmt.Sprintf("清理过期在线记录：%d 条", count),
	})
	metricsMu.Unlock()
}

// SetStoreType sets the active traffic store type.
func SetStoreType(t string) {
	metricsMu.Lock()
	Metrics.StoreType = t
	metricsMu.Unlock()
}

// Snapshot returns a copy of current metrics including recent events (newest first).
func (m *QueueMetrics) Snapshot() QueueMetrics {
	metricsMu.RLock()
	defer metricsMu.RUnlock()

	s := QueueMetrics{
		TotalFlushes:          atomic.LoadInt64(&m.TotalFlushes),
		TotalEmptyFlushes:     atomic.LoadInt64(&m.TotalEmptyFlushes),
		TotalEntries:          atomic.LoadInt64(&m.TotalEntries),
		TotalFlushFailed:      atomic.LoadInt64(&m.TotalFlushFailed),
		LastFlushAt:           m.LastFlushAt,
		LastFlushPending:      m.LastFlushPending,
		LastFlushSuccess:      m.LastFlushSuccess,
		LastFlushFailed:       m.LastFlushFailed,
		TotalAggregations:     atomic.LoadInt64(&m.TotalAggregations),
		LastAggregationAt:     m.LastAggregationAt,
		LastAggregationNodes:  m.LastAggregationNodes,
		LastAggregationUsers:  m.LastAggregationUsers,
		TotalDisableRuns:      atomic.LoadInt64(&m.TotalDisableRuns),
		TotalDisabled:         atomic.LoadInt64(&m.TotalDisabled),
		LastDisableAt:         m.LastDisableAt,
		LastDisabled:          m.LastDisabled,
		TotalResetRuns:        atomic.LoadInt64(&m.TotalResetRuns),
		TotalReset:            atomic.LoadInt64(&m.TotalReset),
		LastResetAt:           m.LastResetAt,
		LastReset:             m.LastReset,
		TotalPurgeRuns:        atomic.LoadInt64(&m.TotalPurgeRuns),
		TotalPurged:           atomic.LoadInt64(&m.TotalPurged),
		LastPurgeAt:           m.LastPurgeAt,
		LastPurged:            m.LastPurged,
		StartedAt:             m.StartedAt,
		StoreType:             m.StoreType,
	}

	if n := len(events); n > 0 {
		s.Recent = make([]JobEvent, n)
		for i := 0; i < n; i++ {
			s.Recent[i] = events[n-1-i]
		}
	}
	return s
}
