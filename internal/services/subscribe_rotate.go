package services

import (
	"fmt"
	"hash/fnv"
	"log/slog"
	"sync"
	"sync/atomic"

	"K2board/internal/models"
)

// IncrCounterFn optional Redis-backed counter (set from main when Redis is ready).
// Signature: key → new value after increment. Nil / error → process-local fallback.
var IncrCounterFn func(key string) (int64, error)

// process-local fallback (single instance; multi-instance uses Redis via IncrCounterFn)
var (
	memRotateMu  sync.Mutex
	memRotateMap = map[string]*atomic.Int64{}
)

func incrSubscribeCounter(key string) int64 {
	if IncrCounterFn != nil {
		n, err := IncrCounterFn(key)
		if err == nil && n > 0 {
			return n
		}
		if err != nil {
			slog.Debug("subscribe rotate redis incr failed, using memory", "key", key, "error", err)
		}
	}
	memRotateMu.Lock()
	c, ok := memRotateMap[key]
	if !ok {
		c = &atomic.Int64{}
		memRotateMap[key] = c
	}
	memRotateMu.Unlock()
	return c.Add(1)
}

// rotateKey builds a per-plan (preferred) or per-group counter key for fair first-node rotation.
func subscribeRotateKey(user *models.User) string {
	if user == nil {
		return "k2board:subrot:global"
	}
	if user.PlanID > 0 {
		return fmt.Sprintf("k2board:subrot:plan:%d", user.PlanID)
	}
	if user.GroupID > 0 {
		return fmt.Sprintf("k2board:subrot:group:%d", user.GroupID)
	}
	return "k2board:subrot:global"
}

// stableUserSalt returns a stable non-negative salt from user id (for scattering concurrent refreshes).
func stableUserSalt(userID uint) uint64 {
	h := fnv.New64a()
	_, _ = fmt.Fprintf(h, "u:%d", userID)
	return h.Sum64()
}

// RotateNodesFair reorders nodes so that over many pulls, each node appears first equally often.
// Algorithm: base order preserved (id ASC from caller); offset =
// (AtomicIncr(planKey) + hash(userID)) % n, then rotate left by offset.
// n<=1 is a no-op. Does not mutate the input slice (returns a new slice).
func RotateNodesFair(user *models.User, nodes []models.Node) []models.Node {
	n := len(nodes)
	if n <= 1 {
		return nodes
	}

	key := subscribeRotateKey(user)
	seq := incrSubscribeCounter(key)
	if seq < 1 {
		seq = 1
	}
	var uid uint
	if user != nil {
		uid = user.ID
	}
	// Incr gives global fairness; user salt scatters simultaneous clients with same plan
	offset := int((uint64(seq-1) + stableUserSalt(uid)) % uint64(n))

	out := make([]models.Node, 0, n)
	out = append(out, nodes[offset:]...)
	out = append(out, nodes[:offset]...)

	slog.Debug("subscribe node rotate",
		"user_id", uid,
		"key", key,
		"n", n,
		"seq", seq,
		"offset", offset,
		"first_node_id", out[0].ID,
		"first_node_name", out[0].Name,
	)
	return out
}

// RotateOffsetForTest exposes offset math for unit tests (no Redis).
func RotateOffsetForTest(seq int64, userID uint, n int) int {
	if n <= 0 {
		return 0
	}
	if seq < 1 {
		seq = 1
	}
	return int((uint64(seq-1) + stableUserSalt(userID)) % uint64(n))
}
