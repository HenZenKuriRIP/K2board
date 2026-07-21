package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// Sliding fixed-window counter per key (IP or custom).
type windowCounter struct {
	count       int
	windowStart time.Time
}

// RateLimiter is a simple in-process fixed-window limiter.
type RateLimiter struct {
	mu       sync.Mutex
	entries  map[string]*windowCounter
	limit    int
	window   time.Duration
	name     string
}

// NewRateLimiter creates a limiter: max `limit` hits per `window` per key.
func NewRateLimiter(name string, limit int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		entries: make(map[string]*windowCounter),
		limit:   limit,
		window:  window,
		name:    name,
	}
	go func() {
		t := time.NewTicker(5 * time.Minute)
		defer t.Stop()
		for range t.C {
			rl.cleanup()
		}
	}()
	return rl
}

func (rl *RateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := time.Now()
	for k, e := range rl.entries {
		if now.Sub(e.windowStart) > rl.window*3 {
			delete(rl.entries, k)
		}
	}
}

// Allow returns true if the key is under the limit (and counts this hit).
func (rl *RateLimiter) Allow(key string) bool {
	if rl.limit <= 0 {
		return true
	}
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := time.Now()
	e, ok := rl.entries[key]
	if !ok || now.Sub(e.windowStart) > rl.window {
		rl.entries[key] = &windowCounter{count: 1, windowStart: now}
		return true
	}
	e.count++
	return e.count <= rl.limit
}

// Middleware limits by client IP.
func (rl *RateLimiter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		if !rl.Allow(ip) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"code":    429,
				"message": "请求过于频繁，请稍后再试",
			})
			return
		}
		c.Next()
	}
}

// Shared limiters for user shop / payment endpoints.
var (
	// Create order: 10 / minute / IP
	UserOrderCreateIP = NewRateLimiter("user-order-create-ip", 10, time.Minute)
	// Checkout / cancel / mock: 30 / minute / IP
	UserOrderActionIP = NewRateLimiter("user-order-action-ip", 30, time.Minute)
	// List / get order: 60 / minute / IP
	UserOrderReadIP = NewRateLimiter("user-order-read-ip", 60, time.Minute)
)
