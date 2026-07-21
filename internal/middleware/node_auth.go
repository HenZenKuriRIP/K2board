package middleware

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"K2board/internal/database"
	"K2board/internal/models"
)

// Rate limiter for node auth.
var (
	nodeRateLimit   = make(map[string]*rateEntry)
	nodeRateLimitMu sync.Mutex
)

type rateEntry struct {
	count        int
	windowStart  time.Time
	blockedUntil time.Time
}

func checkRateLimit(ip string, maxReq int) bool {
	// 0 or negative means rate limiting is disabled
	if maxReq <= 0 {
		return true
	}
	nodeRateLimitMu.Lock()
	defer nodeRateLimitMu.Unlock()
	now := time.Now()
	e, ok := nodeRateLimit[ip]
	if !ok {
		nodeRateLimit[ip] = &rateEntry{count: 1, windowStart: now}
		return true
	}
	if now.Sub(e.windowStart) > 5*time.Minute {
		e.count = 1
		e.windowStart = now
		return true
	}
	if now.Before(e.blockedUntil) {
		return false
	}
	e.count++
	if e.count >= maxReq {
		e.blockedUntil = now.Add(5 * time.Minute)
		return false
	}
	return true
}

// cleanNodeRateLimit periodically removes expired entries to prevent unbounded memory growth.
func init() {
	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			cleanNodeRateLimit()
		}
	}()
}

func cleanNodeRateLimit() {
	nodeRateLimitMu.Lock()
	defer nodeRateLimitMu.Unlock()
	now := time.Now()
	for ip, e := range nodeRateLimit {
		if now.After(e.blockedUntil) && now.Sub(e.windowStart) > 10*time.Minute {
			delete(nodeRateLimit, ip)
		}
	}
}

// GetPanelToken returns the unified panel communication key from settings.
// This is the v2board-standard single key used by all nodes.
var (
	panelTokenCache  string
	panelTokenMu     sync.RWMutex
	panelTokenExpiry time.Time
)

func GetPanelToken() string {
	panelTokenMu.RLock()
	if time.Now().Before(panelTokenExpiry) && panelTokenCache != "" {
		panelTokenMu.RUnlock()
		return panelTokenCache
	}
	panelTokenMu.RUnlock()
	var s models.Setting
	if err := database.DB.Where("key = ?", "panel_token").First(&s).Error; err != nil {
		return ""
	}
	panelTokenMu.Lock()
	panelTokenCache = s.Value
	panelTokenExpiry = time.Now().Add(1 * time.Hour)
	panelTokenMu.Unlock()
	return s.Value
}

// InvalidatePanelTokenCache forces a re-read from DB on next GetPanelToken call.
func InvalidatePanelTokenCache() {
	panelTokenMu.Lock()
	panelTokenExpiry = time.Time{}
	panelTokenMu.Unlock()
}

// verifyToken reports whether plaintext token matches the stored SHA-256 hex hash
// using a constant-time comparison to prevent timing side-channel attacks.
func verifyToken(token string, storedHash string) bool {
	hashB, err := hex.DecodeString(storedHash)
	if err != nil {
		return false
	}
	if len(hashB) != sha256.Size {
		return false
	}
	sum := sha256.Sum256([]byte(token))
	return subtle.ConstantTimeCompare(sum[:], hashB) == 1
}

// NodeAuth authenticates backend nodes.
// Priority: unified panel_token (v2board standard) → per-node token (legacy).
// rateLimit: max requests per IP per 5-minute window (0 = disabled).
func NodeAuth(rateLimit int) gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		token := c.Query("token")
		nodeIDStr := c.Query("node_id")
		if token == "" || nodeIDStr == "" {
			c.AbortWithStatusJSON(401, gin.H{"code": 401, "message": "missing token or node_id"})
			return
		}

		nodeID, err := strconv.ParseUint(nodeIDStr, 10, 64)
		if err != nil {
			c.AbortWithStatusJSON(400, gin.H{"code": 400, "message": "invalid node_id"})
			return
		}

		// Rate limit BEFORE any database queries to prevent DoS
		if !checkRateLimit(ip, rateLimit) {
			c.AbortWithStatusJSON(429, gin.H{"code": 429, "message": "rate limited"})
			return
		}

		// Check unified panel token (SHA256 hashed, same as per-node tokens).
		// Uses constant-time comparison to prevent timing side-channel attacks.
		panelTokenHash := GetPanelToken()
		if panelTokenHash != "" && verifyToken(token, panelTokenHash) {
			var node models.Node
			if err := database.DB.First(&node, nodeID).Error; err != nil || !node.Enable {
				c.AbortWithStatusJSON(403, gin.H{"code": 403, "message": "node not found or disabled"})
				return
			}
			populateNodeGroupIDs(&node)
			c.Set("node_id", uint(nodeID))
			c.Set("node", &node)
			database.DB.Model(&node).Update("last_heartbeat_at", time.Now())
			c.Next()
			return
		}

		// Fallback: per-node token (SHA256 hashed).
		// DB lookup uses the hex hash (index scan); constant-time compare is done by the DB
		// equality check on the fixed-length hash column, which is acceptable here.
		tokenHashHex := hex.EncodeToString(func() []byte { s := sha256.Sum256([]byte(token)); return s[:] }())
		var nodeToken models.NodeToken
		if err := database.DB.Where("node_id = ? AND token = ?", nodeID, tokenHashHex).First(&nodeToken).Error; err == nil {
			var node models.Node
			if err := database.DB.First(&node, nodeID).Error; err == nil && node.Enable {
				populateNodeGroupIDs(&node)
				c.Set("node_id", uint(nodeID))
				c.Set("node", &node)
				// Same heartbeat as panel_token path so status stays online
				database.DB.Model(&node).Update("last_heartbeat_at", time.Now())
				c.Next()
				return
			}
		}

		c.AbortWithStatusJSON(401, gin.H{"code": 401, "message": "invalid token or node_id"})
	}
}

// LoggerMiddleware logs incoming HTTP requests.
func LoggerMiddleware() gin.HandlerFunc { return gin.Logger() }

// GetCurrentNode extracts the authenticated node from context.
func GetCurrentNode(c *gin.Context) *models.Node {
	node, _ := c.Get("node")
	if node == nil {
		return nil
	}
	return node.(*models.Node)
}

// populateNodeGroupIDs loads the group IDs for a node from the junction table.
// Must be called after the node is loaded from DB but before it is used by handlers.
func populateNodeGroupIDs(node *models.Node) {
	var mappings []models.NodeGroupMapping
	database.DB.Where("node_id = ?", node.ID).Find(&mappings)
	for _, m := range mappings {
		node.GroupIDs = append(node.GroupIDs, m.GroupID)
	}
}
