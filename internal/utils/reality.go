package utils

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net"
	"strings"
)

// RealityParams holds auto-generated REALITY protocol settings (core fields).
type RealityParams struct {
	PublicKey   string `json:"public_key"`
	PrivateKey  string `json:"private_key"`
	ShortID     string `json:"short_id"`
	Fingerprint string `json:"fingerprint"`
	Dest        string `json:"dest"`
}

// GenerateReality creates a new X25519 keypair + random ShortID for REALITY protocol.
// Dest defaults to sni:443 if sni is provided.
func GenerateReality(sni string) RealityParams {
	priv, _ := ecdh.X25519().GenerateKey(rand.Reader)
	pub := priv.PublicKey()

	sid := make([]byte, 4)
	rand.Read(sid)

	dest := ""
	if sni != "" {
		dest = sni + ":443"
	}

	return RealityParams{
		PublicKey:   base64.RawURLEncoding.EncodeToString(pub.Bytes()),
		PrivateKey:  base64.RawURLEncoding.EncodeToString(priv.Bytes()),
		ShortID:     hex.EncodeToString(sid),
		Fingerprint: "chrome",
		Dest:        dest,
	}
}

// MergeRealityJSON merges incoming REALITY settings with auto-generated defaults.
// Preserves unknown / PQ extension fields (min_client_ver, mldsa65_*, show, …).
// Does not invent PQ fields for old/partial payloads (empty = off at UniProxy layer).
func MergeRealityJSON(incoming json.RawMessage, sni string) json.RawMessage {
	m := map[string]any{}
	if len(incoming) > 0 {
		_ = json.Unmarshal(incoming, &m)
	}

	auto := GenerateReality(sni)

	if strVal(m["public_key"]) == "" {
		m["public_key"] = auto.PublicKey
	}
	if strVal(m["private_key"]) == "" {
		m["private_key"] = auto.PrivateKey
	}
	if strVal(m["short_id"]) == "" {
		m["short_id"] = auto.ShortID
	}
	if strVal(m["fingerprint"]) == "" {
		m["fingerprint"] = auto.Fingerprint
	}
	if strVal(m["dest"]) == "" {
		m["dest"] = auto.Dest
	}

	out, _ := json.Marshal(m)
	return out
}

func strVal(v any) string {
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

// ParseDestHostPort splits REALITY dest (host or host:port). Empty dest falls back to sni.
// Default port is "443". Compatible with legacy nodes that only store SNI.
func ParseDestHostPort(dest, sni string) (host, port string) {
	dest = strings.TrimSpace(dest)
	sni = strings.TrimSpace(sni)
	if dest == "" {
		dest = sni
	}
	if dest == "" {
		return "", "443"
	}
	// Try host:port (also handles [ipv6]:port)
	if h, p, err := net.SplitHostPort(dest); err == nil {
		if p == "" {
			p = "443"
		}
		return h, p
	}
	// Bare hostname / IP without port
	return dest, "443"
}

// RealityMap unmarshals reality_settings JSON into a generic map (nil if empty/invalid).
func RealityMap(raw json.RawMessage) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil
	}
	return m
}

// RealityString reads a string field from reality_settings.
func RealityString(raw json.RawMessage, key string) string {
	m := RealityMap(raw)
	if m == nil {
		return ""
	}
	return strVal(m[key])
}

// RealityBool reads a bool field from reality_settings.
func RealityBool(raw json.RawMessage, key string) bool {
	m := RealityMap(raw)
	if m == nil {
		return false
	}
	switch v := m[key].(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(strings.TrimSpace(v), "true") || v == "1"
	default:
		return false
	}
}

// NormalizeVlessCrypto returns trimmed decryption/encryption; empty or "none" means off.
func NormalizeVlessCrypto(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || strings.EqualFold(s, "none") {
		return ""
	}
	return s
}
