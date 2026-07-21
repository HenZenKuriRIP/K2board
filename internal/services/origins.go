package services

import (
	"net"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	settingAllowedOrigins = "allowed_origins"
	maxAllowedOrigins     = 64
	maxOriginLen          = 256
	// corsCacheTTL avoids hitting DB on every preflight while still picking up
	// admin changes reasonably fast even if Invalidate is missed.
	corsCacheTTL = 30 * time.Second
)

// corsOriginsSnapshot is an immutable allow-list of normalized origins
// (scheme://host[:port], lowercase host).
type corsOriginsSnapshot struct {
	// exact origin strings for O(1) match
	set map[string]struct{}
	// hosts only (lowercase, may include port) for return_url host checks
	hosts map[string]struct{}
	// raw list for admin/debug
	list []string
	at   time.Time
}

var (
	corsSnap atomic.Pointer[corsOriginsSnapshot]
	corsMu   sync.Mutex // single-flight rebuild
)

// InvalidateCORSOriginsCache forces the next CORS/return-url check to reload
// allowed_origins / site_url / subscribe_url from DB.
func InvalidateCORSOriginsCache() {
	corsSnap.Store(nil)
}

// NormalizeOrigin converts user/admin input into scheme://host[:port].
// Accepts full URLs or bare hosts. Defaults to https when scheme omitted.
// Returns empty string if invalid (open redirect / CORS unsafe).
func NormalizeOrigin(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > maxOriginLen {
		return ""
	}
	// Reject wildcards and dangerous schemes early
	lower := strings.ToLower(raw)
	if lower == "*" || strings.Contains(raw, "*") {
		return ""
	}
	if strings.Contains(lower, "javascript:") || strings.Contains(lower, "data:") {
		return ""
	}

	// Bare host → assume https
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}

	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return ""
	}
	// Strip userinfo, path, query, fragment — origin only
	host := strings.ToLower(u.Host)
	// url.Parse may leave brackets on IPv6; keep as-is via u.Hostname/Port
	hostname := strings.ToLower(u.Hostname())
	if hostname == "" {
		return ""
	}
	// Reject empty host tricks
	if hostname == "." || strings.Contains(hostname, " ") {
		return ""
	}
	port := u.Port()
	if port != "" {
		host = hostname + ":" + port
	} else {
		host = hostname
	}
	return scheme + "://" + host
}

// ParseAllowedOriginsList splits admin setting text (newlines, commas, spaces)
// into unique normalized origins. Invalid entries are skipped.
func ParseAllowedOriginsList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	// Normalize separators
	replacer := strings.NewReplacer("\r\n", "\n", "\r", "\n", ",", "\n", ";", "\n", "\t", "\n")
	raw = replacer.Replace(raw)
	seen := make(map[string]struct{})
	out := make([]string, 0, 8)
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// allow space-separated on one line
		fields := strings.Fields(line)
		for _, f := range fields {
			o := NormalizeOrigin(f)
			if o == "" {
				continue
			}
			if _, ok := seen[o]; ok {
				continue
			}
			seen[o] = struct{}{}
			out = append(out, o)
			if len(out) >= maxAllowedOrigins {
				return out
			}
		}
	}
	return out
}

// FormatAllowedOrigins stores a clean newline-separated list for admin UI.
func FormatAllowedOrigins(origins []string) string {
	if len(origins) == 0 {
		return ""
	}
	return strings.Join(origins, "\n")
}

// ValidateAndNormalizeAllowedOriginsSetting parses admin input and returns
// the canonical stored value, or an error message for the client.
func ValidateAndNormalizeAllowedOriginsSetting(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	// Detect totally invalid non-empty input (every line bad)
	lines := 0
	bad := 0
	replacer := strings.NewReplacer("\r\n", "\n", "\r", "\n", ",", "\n", ";", "\n")
	for _, line := range strings.Split(replacer.Replace(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		for _, f := range strings.Fields(line) {
			lines++
			if NormalizeOrigin(f) == "" {
				bad++
			}
		}
	}
	parsed := ParseAllowedOriginsList(raw)
	if lines > 0 && len(parsed) == 0 {
		return "", errInvalidOrigins
	}
	if bad > 0 && len(parsed) == 0 {
		return "", errInvalidOrigins
	}
	// If some lines invalid but some ok, keep valid only (lenient)
	if len(parsed) > maxAllowedOrigins {
		parsed = parsed[:maxAllowedOrigins]
	}
	return FormatAllowedOrigins(parsed), nil
}

var errInvalidOrigins = errOrigins("allowed_origins 格式无效：请使用 https://域名 ，每行一个，勿使用 *")

type errOrigins string

func (e errOrigins) Error() string { return string(e) }

// buildCORSSnapshot loads settings and builds the allow snapshot.
func buildCORSSnapshot() *corsOriginsSnapshot {
	set := make(map[string]struct{})
	hosts := make(map[string]struct{})
	list := make([]string, 0, 8)

	add := func(o string) {
		o = NormalizeOrigin(o)
		if o == "" {
			return
		}
		if _, ok := set[o]; ok {
			return
		}
		set[o] = struct{}{}
		list = append(list, o)
		if u, err := url.Parse(o); err == nil && u.Host != "" {
			hosts[strings.ToLower(u.Host)] = struct{}{}
			// also hostname without default port ambiguity
			h := strings.ToLower(u.Hostname())
			if h != "" {
				hosts[h] = struct{}{}
			}
		}
	}

	// Always include site_url + subscribe_url so primary panel works without
	// re-listing them in allowed_origins.
	if su := SettingValue("site_url"); su != "" {
		add(su)
	}
	if sub := SettingValue("subscribe_url"); sub != "" {
		add(sub)
	}
	for _, o := range ParseAllowedOriginsList(SettingValue(settingAllowedOrigins)) {
		add(o)
	}

	return &corsOriginsSnapshot{
		set:   set,
		hosts: hosts,
		list:  list,
		at:    time.Now(),
	}
}

// getCORSSnapshot returns a cached allow-list (rebuilds if missing/stale).
func getCORSSnapshot() *corsOriginsSnapshot {
	if s := corsSnap.Load(); s != nil && time.Since(s.at) < corsCacheTTL {
		return s
	}
	corsMu.Lock()
	defer corsMu.Unlock()
	// double-check
	if s := corsSnap.Load(); s != nil && time.Since(s.at) < corsCacheTTL {
		return s
	}
	s := buildCORSSnapshot()
	corsSnap.Store(s)
	return s
}

// ListEffectiveCORSOrigins returns the current effective origin allow-list
// (site_url + subscribe_url + allowed_origins) for admin display.
func ListEffectiveCORSOrigins() []string {
	s := getCORSSnapshot()
	out := make([]string, len(s.list))
	copy(out, s.list)
	return out
}

// IsLocalDevOrigin allows local Vite/dev servers without listing them.
func IsLocalDevOrigin(origin string) bool {
	o := NormalizeOrigin(origin)
	if o == "" {
		return false
	}
	u, err := url.Parse(o)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

// requestHostOrigin builds http(s)://requestHost for same-origin checks.
func requestHostOrigins(requestHost string) []string {
	requestHost = strings.TrimSpace(requestHost)
	if requestHost == "" {
		return nil
	}
	// Host header may be "www.example.com" or "www.example.com:443"
	h := strings.ToLower(requestHost)
	return []string{"https://" + h, "http://" + h}
}

// IsCORSOriginAllowed reports whether browser Origin may call the API.
// Empty origin (non-browser / same-origin navigations) is allowed without CORS headers.
func IsCORSOriginAllowed(origin, requestHost string) bool {
	origin = strings.TrimSpace(origin)
	if origin == "" {
		return true
	}
	norm := NormalizeOrigin(origin)
	if norm == "" {
		return false
	}
	// Same host as the API request (www panel serving both SPA and API)
	for _, cand := range requestHostOrigins(requestHost) {
		if NormalizeOrigin(cand) == norm {
			return true
		}
	}
	// Strip port from request host for comparison when Origin has default ports
	if rh := strings.ToLower(strings.Split(requestHost, ":")[0]); rh != "" {
		if u, err := url.Parse(norm); err == nil {
			if strings.EqualFold(u.Hostname(), rh) {
				return true
			}
		}
	}
	if IsLocalDevOrigin(norm) {
		return true
	}
	snap := getCORSSnapshot()
	if _, ok := snap.set[norm]; ok {
		return true
	}
	return false
}

// IsReturnURLHostAllowed checks payment redirect host against site base + allow-list.
func IsReturnURLHostAllowed(host, siteBase string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return false
	}
	// Strip brackets for IPv6 if present
	if h, p, err := net.SplitHostPort(host); err == nil {
		_ = p
		host = strings.ToLower(h)
	}
	if siteBase != "" {
		if b, err := url.Parse(strings.TrimSpace(siteBase)); err == nil && b.Host != "" {
			if strings.EqualFold(host, b.Host) || strings.EqualFold(host, b.Hostname()) {
				return true
			}
		}
	}
	snap := getCORSSnapshot()
	if _, ok := snap.hosts[host]; ok {
		return true
	}
	// host without port vs hosts map entries with port
	if hostname, _, err := net.SplitHostPort(host); err == nil {
		if _, ok := snap.hosts[strings.ToLower(hostname)]; ok {
			return true
		}
	}
	return false
}
