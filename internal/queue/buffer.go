package queue

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"K2board/internal/database"
	"K2board/internal/models"
)

// TrafficStore is the pluggable interface for traffic buffering.
type TrafficStore interface {
	Add(userID, nodeID uint, upload, download int64)
	// Flush drains buffered traffic into the database.
	Flush() FlushResult
	Size() int
}

var DefaultStore TrafficStore = NewInMemoryStore()

// trafficKey uniquely identifies buffered traffic per user per node.
type trafficKey struct {
	UserID uint
	NodeID uint
}

type trafficEntry struct {
	Upload   int64
	Download int64
}

type InMemoryStore struct {
	mu     sync.Mutex
	buffer map[trafficKey]*trafficEntry
}

func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{buffer: make(map[trafficKey]*trafficEntry)}
}

func (s *InMemoryStore) Add(userID, nodeID uint, upload, download int64) {
	if upload == 0 && download == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	k := trafficKey{UserID: userID, NodeID: nodeID}
	e, ok := s.buffer[k]
	if !ok {
		e = &trafficEntry{}
		s.buffer[k] = e
	}
	e.Upload += upload
	e.Download += download
}

// requeue merges failed flush entries back into the live buffer so no traffic is lost.
func (s *InMemoryStore) requeue(failed map[trafficKey]*trafficEntry) {
	if len(failed) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, e := range failed {
		if e.Upload == 0 && e.Download == 0 {
			continue
		}
		cur, ok := s.buffer[k]
		if !ok {
			// copy to avoid sharing pointers with caller
			s.buffer[k] = &trafficEntry{Upload: e.Upload, Download: e.Download}
			continue
		}
		cur.Upload += e.Upload
		cur.Download += e.Download
	}
}

func (s *InMemoryStore) Flush() FlushResult {
	s.mu.Lock()
	old := s.buffer
	s.buffer = make(map[trafficKey]*trafficEntry)
	s.mu.Unlock()

	pending := len(old)
	if pending == 0 {
		return FlushResult{}
	}

	failed := make(map[trafficKey]*trafficEntry)
	success := 0
	for k, entry := range old {
		u, d := entry.Upload, entry.Download
		if u == 0 && d == 0 {
			continue
		}
		err := database.DB.Transaction(func(tx *gorm.DB) error {
			if err := tx.Create(&models.TrafficLog{
				UserID:     k.UserID,
				NodeID:     k.NodeID,
				Upload:     u,
				Download:   d,
				RecordedAt: time.Now(),
			}).Error; err != nil {
				return err
			}
			return tx.Model(&models.User{}).
				Where("id = ?", k.UserID).
				UpdateColumn("traffic_used", gorm.Expr("traffic_used + ?", u+d)).Error
		})
		if err != nil {
			slog.Error("traffic flush failed, will retry",
				"user_id", k.UserID, "node_id", k.NodeID, "error", err)
			failed[k] = &trafficEntry{Upload: u, Download: d}
		} else {
			success++
		}
	}
	if len(failed) > 0 {
		s.requeue(failed)
		slog.Warn("traffic flush partial failure", "requeued", len(failed))
	}
	return FlushResult{Pending: pending, Success: success, Failed: len(failed)}
}

func (s *InMemoryStore) Size() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.buffer)
}

// AggregateDailyStats rolls up daily traffic into stat_server and stat_user tables.
// Returns how many node and user rows were upserted (attempted).
func AggregateDailyStats() (nodes, users int) {
	today := time.Now().Truncate(24 * time.Hour).Unix()

	var ss []struct {
		NodeID   uint
		Upload   int64
		Download int64
	}
	database.DB.Model(&models.TrafficLog{}).
		Select("node_id, SUM(upload) as upload, SUM(download) as download").
		Where("recorded_at >= ?", time.Unix(today, 0)).
		Group("node_id").Scan(&ss)

	for _, s := range ss {
		if err := database.DB.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "record_at"}, {Name: "node_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"upload", "download"}),
		}).Create(&models.StatServer{
			RecordAt: today, NodeID: s.NodeID, Upload: s.Upload, Download: s.Download,
		}).Error; err != nil {
			slog.Error("aggregate stat_server failed", "node_id", s.NodeID, "error", err)
			continue
		}
		nodes++
	}

	var us []struct {
		UserID   uint
		Upload   int64
		Download int64
	}
	database.DB.Model(&models.TrafficLog{}).
		Select("user_id, SUM(upload) as upload, SUM(download) as download").
		Where("recorded_at >= ?", time.Unix(today, 0)).
		Group("user_id").Scan(&us)

	for _, s := range us {
		if err := database.DB.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "record_at"}, {Name: "user_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"upload", "download"}),
		}).Create(&models.StatUser{
			RecordAt: today, UserID: s.UserID, Upload: s.Upload, Download: s.Download,
		}).Error; err != nil {
			slog.Error("aggregate stat_user failed", "user_id", s.UserID, "error", err)
			continue
		}
		users++
	}
	return nodes, users
}

// FormatTrafficKey is exported for tests / debugging: "user:node".
func FormatTrafficKey(userID, nodeID uint) string {
	return fmt.Sprintf("%d:%d", userID, nodeID)
}
