package queue

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"K2board/internal/database"
	"K2board/internal/models"
)

// RedisStore implements TrafficStore using Redis.
// Traffic is stored as: k2board:traffic:{userID}:{nodeID} → {upload, download}
// On Flush, entries are drained and written to DB; failed writes restore the key.
type RedisStore struct {
	client *redis.Client
	ctx    context.Context
}

// NewRedisStore connects to Redis and returns a RedisStore.
func NewRedisStore(addr, password string, db int) (*RedisStore, error) {
	client := redis.NewClient(&redis.Options{
		Addr:         addr,
		Password:     password,
		DB:           db,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		PoolSize:     20,
		MinIdleConns: 5,
	})

	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis ping failed: %w", err)
	}

	slog.Info("redis connected", "addr", addr, "db", db)
	return &RedisStore{client: client, ctx: ctx}, nil
}

const trafficKeyPrefix = "k2board:traffic"

func trafficRedisKey(userID, nodeID uint) string {
	return fmt.Sprintf("%s:%d:%d", trafficKeyPrefix, userID, nodeID)
}

// Add increments traffic counters in Redis per (user, node).
func (r *RedisStore) Add(userID, nodeID uint, upload, download int64) {
	if upload == 0 && download == 0 {
		return
	}
	key := trafficRedisKey(userID, nodeID)
	pipe := r.client.Pipeline()
	pipe.HIncrBy(r.ctx, key, "upload", upload)
	pipe.HIncrBy(r.ctx, key, "download", download)
	pipe.Expire(r.ctx, key, 2*time.Hour)
	if _, err := pipe.Exec(r.ctx); err != nil {
		slog.Warn("redis traffic add failed, writing directly to DB", "error", err)
		_ = database.DB.Transaction(func(tx *gorm.DB) error {
			if err := tx.Create(&models.TrafficLog{
				UserID: userID, NodeID: nodeID, Upload: upload, Download: download, RecordedAt: time.Now(),
			}).Error; err != nil {
				return err
			}
			return tx.Model(&models.User{}).Where("id = ?", userID).
				UpdateColumn("traffic_used", gorm.Expr("traffic_used + ?", upload+download)).Error
		})
	}
}

// parseTrafficKey extracts userID and nodeID from "k2board:traffic:{uid}:{nid}".
// Also accepts legacy "k2board:traffic:{uid}" keys (node_id field in hash).
func parseTrafficKey(key string) (userID, nodeID uint, legacy bool, ok bool) {
	// strip prefix
	if !strings.HasPrefix(key, trafficKeyPrefix+":") {
		return 0, 0, false, false
	}
	rest := key[len(trafficKeyPrefix)+1:]
	parts := strings.Split(rest, ":")
	if len(parts) == 1 {
		// legacy: k2board:traffic:{userID}
		uid, err := strconv.ParseUint(parts[0], 10, 64)
		if err != nil {
			return 0, 0, false, false
		}
		return uint(uid), 0, true, true
	}
	if len(parts) >= 2 {
		uid, err1 := strconv.ParseUint(parts[0], 10, 64)
		nid, err2 := strconv.ParseUint(parts[1], 10, 64)
		if err1 != nil || err2 != nil {
			return 0, 0, false, false
		}
		return uint(uid), uint(nid), false, true
	}
	return 0, 0, false, false
}

// Flush drains all Redis traffic counters and writes them to the database.
func (r *RedisStore) Flush() FlushResult {
	pattern := trafficKeyPrefix + ":*"
	var cursor uint64
	pending := 0
	total := 0
	failed := 0

	for {
		keys, nextCursor, err := r.client.Scan(r.ctx, cursor, pattern, 200).Result()
		if err != nil {
			slog.Error("redis scan failed during flush", "error", err)
			return FlushResult{Pending: pending, Success: total, Failed: failed}
		}

		for _, key := range keys {
			// skip in-flight temp keys
			if strings.HasSuffix(key, ":flush") {
				continue
			}

			userID, nodeID, legacy, ok := parseTrafficKey(key)
			if !ok {
				continue
			}

			tempKey := key + ":flush"
			if err := r.client.Rename(r.ctx, key, tempKey).Err(); err != nil {
				continue
			}
			pending++

			vals, err := r.client.HGetAll(r.ctx, tempKey).Result()
			if err != nil || len(vals) == 0 {
				r.client.Del(r.ctx, tempKey)
				continue
			}

			upload, _ := strconv.ParseInt(vals["upload"], 10, 64)
			download, _ := strconv.ParseInt(vals["download"], 10, 64)
			if legacy {
				if n, err := strconv.ParseUint(vals["node_id"], 10, 64); err == nil {
					nodeID = uint(n)
				}
			}

			if upload == 0 && download == 0 {
				r.client.Del(r.ctx, tempKey)
				continue
			}

			dbErr := database.DB.Transaction(func(tx *gorm.DB) error {
				if err := tx.Create(&models.TrafficLog{
					UserID:     userID,
					NodeID:     nodeID,
					Upload:     upload,
					Download:   download,
					RecordedAt: time.Now(),
				}).Error; err != nil {
					return err
				}
				return tx.Model(&models.User{}).
					Where("id = ?", userID).
					UpdateColumn("traffic_used", gorm.Expr("traffic_used + ?", upload+download)).Error
			})

			if dbErr != nil {
				// Restore original key so next flush can retry
				slog.Error("redis traffic flush DB failed, restoring key",
					"user_id", userID, "node_id", nodeID, "error", dbErr)
				if err := r.client.Rename(r.ctx, tempKey, key).Err(); err != nil {
					// last resort: re-HINCR on original key
					pipe := r.client.Pipeline()
					pipe.HIncrBy(r.ctx, key, "upload", upload)
					pipe.HIncrBy(r.ctx, key, "download", download)
					if legacy {
						pipe.HSet(r.ctx, key, "node_id", nodeID)
					}
					pipe.Expire(r.ctx, key, 2*time.Hour)
					pipe.Del(r.ctx, tempKey)
					_, _ = pipe.Exec(r.ctx)
				}
				failed++
				continue
			}

			r.client.Del(r.ctx, tempKey)
			total++
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	if total > 0 || failed > 0 {
		slog.Info("redis traffic flushed to db", "ok", total, "failed", failed)
	}
	return FlushResult{Pending: pending, Success: total, Failed: failed}
}

// Size returns approximate number of traffic entries in Redis (excludes :flush temps).
func (r *RedisStore) Size() int {
	keys, err := r.client.Keys(r.ctx, trafficKeyPrefix+":*").Result()
	if err != nil {
		return 0
	}
	n := 0
	for _, k := range keys {
		if !strings.HasSuffix(k, ":flush") {
			n++
		}
	}
	return n
}

// Incr atomically increments a counter key (used e.g. for subscribe node rotation).
// Key is stored as-is (callers should use a namespaced prefix). Refreshes a long TTL
// so counters do not grow forever if a plan is deleted.
func (r *RedisStore) Incr(key string) (int64, error) {
	if r == nil || r.client == nil {
		return 0, fmt.Errorf("redis not available")
	}
	n, err := r.client.Incr(r.ctx, key).Result()
	if err != nil {
		return 0, err
	}
	// Best-effort TTL refresh (180 days); ignore expire errors
	_ = r.client.Expire(r.ctx, key, 180*24*time.Hour).Err()
	return n, nil
}

// Close gracefully shuts down the Redis connection.
func (r *RedisStore) Close() error {
	return r.client.Close()
}
