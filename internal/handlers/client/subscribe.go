package client

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"K2board/internal/services"
	"K2board/internal/utils"
)

type SubscribeHandler struct {
	subSvc *services.SubscribeService
}

func NewSubscribeHandler() *SubscribeHandler {
	return &SubscribeHandler{
		subSvc: services.NewSubscribeService(),
	}
}

var (
	subLimitMap   = make(map[string]*subLimit)
	subLimitMu    sync.Mutex
)

type subLimit struct {
	count       int
	windowStart time.Time
}

func checkSubLimit(ip string) bool {
	subLimitMu.Lock()
	defer subLimitMu.Unlock()
	now := time.Now()
	e, ok := subLimitMap[ip]
	if !ok {
		subLimitMap[ip] = &subLimit{count: 1, windowStart: now}
		return true
	}
	if now.Sub(e.windowStart) > time.Minute {
		e.count = 1
		e.windowStart = now
		return true
	}
	e.count++
	return e.count <= 30
}

func init() {
	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			cleanSubLimit()
		}
	}()
}

func cleanSubLimit() {
	subLimitMu.Lock()
	defer subLimitMu.Unlock()
	now := time.Now()
	for ip, e := range subLimitMap {
		if now.Sub(e.windowStart) > 5*time.Minute {
			delete(subLimitMap, ip)
		}
	}
}

// GetSubscription returns subscription content for the given token.
// GET /api/v1/client/subscribe?token=xxx&flag=clash|shadowrocket|surge
// Default / UA: clash (FlClash / Clash Meta YAML). Explicit flag always wins.
func (h *SubscribeHandler) GetSubscription(c *gin.Context) {
	if !checkSubLimit(c.ClientIP()) {
		utils.Error(c, 429, "rate limited")
		return
	}

	token := c.Query("token")
	if token == "" {
		utils.BadRequest(c, "missing token parameter")
		return
	}

	flag := resolveSubFlag(c)
	result, err := h.subSvc.GenerateSubscription(token, flag)

	// Never cache subscription bodies (token often appears in URL query logs/CDN).
	// Does not change URL shape or client config content.
	c.Header("Cache-Control", "no-store, private")
	c.Header("Pragma", "no-cache")

	if err != nil {
		c.Header("Content-Type", "text/plain; charset=utf-8")
		c.String(200, "") // empty subscription, no error popup
		return
	}

	// Clash YAML is text/yaml-friendly; keep text/plain for max client compatibility
	ct := "text/plain; charset=utf-8"
	if flag == "clash" {
		ct = "text/yaml; charset=utf-8"
	}
	c.Header("Content-Type", ct)
	c.Header("Profile-Update-Interval", "24")
	c.Header("Subscription-Userinfo", fmt.Sprintf("upload=%d; download=%d; total=%d; expire=%d",
		result.UploadUsed, result.DownloadUsed, result.TotalTraffic, result.ExpireAt))
	c.String(200, result.Content)
}

// resolveSubFlag: query flag > User-Agent > default clash (FlClash-first).
func resolveSubFlag(c *gin.Context) string {
	if f := strings.ToLower(strings.TrimSpace(c.Query("flag"))); f != "" {
		switch f {
		case "clash", "meta", "mihomo", "flclash":
			return "clash"
		case "surge":
			return "surge"
		case "shadowrocket", "rocket", "sr":
			return "shadowrocket"
		case "v2ray", "v2rayn", "base64": // still accepted, not promoted in UI
			return "v2ray"
		default:
			return f
		}
	}
	ua := strings.ToLower(c.GetHeader("User-Agent"))
	switch {
	case strings.Contains(ua, "surge"):
		return "surge"
	case strings.Contains(ua, "shadowrocket"),
		strings.Contains(ua, "quantumult"):
		return "shadowrocket"
	case strings.Contains(ua, "clash"),
		strings.Contains(ua, "flclash"),
		strings.Contains(ua, "mihomo"),
		strings.Contains(ua, "stash"),
		strings.Contains(ua, "meta"):
		return "clash"
	default:
		// FlClash / 主流 Clash 系默认
		return "clash"
	}
}
