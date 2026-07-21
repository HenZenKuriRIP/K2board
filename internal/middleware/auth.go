package middleware

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"K2board/internal/database"
	"K2board/internal/models"
	"K2board/internal/services"
	"K2board/internal/utils"
)

// CORSMiddleware enforces a strict Origin allow-list for browser cross-origin
// calls (shadow user portals → panel API). Empty Origin (curl / same-site
// navigations) is allowed. Allowed set = request host + localhost + site_url +
// subscribe_url + settings.allowed_origins. Never reflects arbitrary Origin.
func CORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		host := c.Request.Host
		allowed := services.IsCORSOriginAllowed(origin, host)
		if allowed && origin != "" {
			// Echo only validated origin (required for credentialed CORS)
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
			c.Header("Access-Control-Allow-Credentials", "true")
		}
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, PATCH")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization, Accept")
		c.Header("Access-Control-Max-Age", "86400")
		if c.Request.Method == http.MethodOptions {
			if !allowed && origin != "" {
				// Preflight from disallowed origin: no ACAO, 403
				c.AbortWithStatus(http.StatusForbidden)
				return
			}
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// SecurityHeaders adds common security headers to all responses.
// HSTS is intentionally left to reverse proxy / Cloudflare (TLS terminates there).
// CSP is site-specific (admin vs user SPA) and should be set at the edge if needed.
func SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Header("X-XSS-Protection", "1; mode=block") // legacy browsers; harmless
		c.Next()
	}
}

// JWTAuth validates JWT tokens from the Authorization header.
// After signature checks, reloads the user row so a banned or demoted admin
// cannot keep using a still-unexpired JWT (issued enable/is_admin are not enough).
func JWTAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			utils.Unauthorized(c, "missing authorization header")
			c.Abort()
			return
		}
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			utils.Unauthorized(c, "invalid authorization format")
			c.Abort()
			return
		}
		claims, err := utils.ParseJWT(parts[1])
		if err != nil {
			utils.Unauthorized(c, "invalid or expired token")
			c.Abort()
			return
		}
		// Live account state (ban / demote) — small SELECT, admin traffic only.
		var u models.User
		if err := database.DB.Select("id", "email", "enable", "is_admin").First(&u, claims.UserID).Error; err != nil {
			utils.Unauthorized(c, "invalid or expired token")
			c.Abort()
			return
		}
		if !u.Enable {
			utils.Forbidden(c, "账号已被禁用")
			c.Abort()
			return
		}
		// Prefer DB flags over stale JWT claims
		c.Set("user_id", u.ID)
		c.Set("email", u.Email)
		c.Set("is_admin", u.IsAdmin)
		c.Next()
	}
}

// AdminOnly checks if the authenticated user has admin privileges.
func AdminOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		isAdmin, _ := c.Get("is_admin")
		if isAdmin == nil || !isAdmin.(bool) {
			utils.Forbidden(c, "admin access required")
			c.Abort()
			return
		}
		c.Next()
	}
}

// LoginRateLimit limits login attempts: 10 per minute per IP.
var loginRateMap = make(map[string]*loginRate)
var loginRateMu sync.Mutex

type loginRate struct {
	count       int
	windowStart time.Time
}

func CheckLoginRate(ip string) bool {
	loginRateMu.Lock()
	defer loginRateMu.Unlock()
	now := time.Now()
	e, ok := loginRateMap[ip]
	if !ok {
		loginRateMap[ip] = &loginRate{count: 1, windowStart: now}
		return true
	}
	if now.Sub(e.windowStart) > time.Minute {
		e.count = 1
		e.windowStart = now
		return true
	}
	e.count++
	return e.count <= 10
}

func init() {
	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			cleanLoginRateLimit()
		}
	}()
}

func cleanLoginRateLimit() {
	loginRateMu.Lock()
	defer loginRateMu.Unlock()
	now := time.Now()
	for ip, e := range loginRateMap {
		if now.Sub(e.windowStart) > 5*time.Minute {
			delete(loginRateMap, ip)
		}
	}
}

func GetCurrentUserID(c *gin.Context) uint {
	id, _ := c.Get("user_id")
	if id == nil {
		return 0
	}
	return id.(uint)
}
