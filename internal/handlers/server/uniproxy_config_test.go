package server

import (
	"encoding/json"
	"testing"

	"K2board/internal/models"
)

// Legacy REALITY node: no PQ fields — must stay connectable for old clients.
func TestBuildUniProxyConfig_LegacyReality(t *testing.T) {
	rs, _ := json.Marshal(map[string]any{
		"public_key":  "pub",
		"private_key": "priv",
		"short_id":    "abcd",
		"fingerprint": "chrome",
		// no dest, no min_client_ver, no mldsa
	})
	node := &models.Node{
		NodeType:        "vless",
		Port:            8443,
		Network:         "tcp",
		TLS:             2,
		Flow:            "xtls-rprx-vision",
		SNI:             "www.microsoft.com",
		RealitySettings: rs,
	}
	cfg := buildUniProxyConfig(node)

	if cfg.ServerPort != 8443 {
		t.Fatalf("listen server_port: got %d", cfg.ServerPort)
	}
	if cfg.Decryption != "none" {
		t.Fatalf("legacy decryption want none, got %q", cfg.Decryption)
	}
	if cfg.TLSSettings == nil {
		t.Fatal("tls_settings nil")
	}
	ts := cfg.TLSSettings
	if ts.MinClientVer != "1.8.0" {
		t.Fatalf("min_client_ver default: got %q", ts.MinClientVer)
	}
	if ts.Mldsa65Seed != "" {
		t.Fatalf("legacy must not emit mldsa65_seed, got %q", ts.Mldsa65Seed)
	}
	if ts.Show != nil {
		t.Fatal("legacy must not emit show")
	}
	if ts.PublicKey != "pub" || ts.PrivateKey != "priv" || ts.ShortID != "abcd" {
		t.Fatalf("keys: %+v", ts)
	}
	// dest falls back to SNI; dest port is 443 (not listen 8443)
	if ts.Dest != "www.microsoft.com" {
		t.Fatalf("dest host: got %q", ts.Dest)
	}
	if ts.ServerPort != "443" {
		t.Fatalf("dest server_port want 443 string, got %q (must not be listen port)", ts.ServerPort)
	}
	if cfg.BaseConfig == nil || cfg.BaseConfig.PushInterval != 60 || cfg.BaseConfig.PullInterval != 60 {
		t.Fatalf("base_config: %+v", cfg.BaseConfig)
	}
	// pure tcp REALITY: no top-level host/path/network_settings (legacy shape)
	if cfg.Host != "" || cfg.Path != "" || cfg.NetworkSettings != nil {
		t.Fatalf("legacy REALITY tcp should omit transport meta: host=%q path=%q ns=%+v", cfg.Host, cfg.Path, cfg.NetworkSettings)
	}
}

func TestBuildUniProxyConfig_ExplicitDestAndPQ(t *testing.T) {
	rs, _ := json.Marshal(map[string]any{
		"public_key":     "pub",
		"private_key":    "priv",
		"short_id":       "ab",
		"fingerprint":    "chrome",
		"dest":           "www.cloudflare.com:443",
		"min_client_ver": "1.8.0",
		"mldsa65_seed":   "SEED",
		"show":           true,
	})
	node := &models.Node{
		NodeType:         "vless",
		Port:             443,
		Network:          "tcp",
		TLS:              2,
		SNI:              "www.cloudflare.com",
		RealitySettings:  rs,
		VlessDecryption:  "mlkem768x25519plus.test",
		VlessEncryption:  "client-enc",
	}
	cfg := buildUniProxyConfig(node)
	if cfg.Decryption != "mlkem768x25519plus.test" {
		t.Fatalf("decryption: %q", cfg.Decryption)
	}
	if cfg.TLSSettings.Dest != "www.cloudflare.com" || cfg.TLSSettings.ServerPort != "443" {
		t.Fatalf("dest parse: %+v", cfg.TLSSettings)
	}
	if cfg.TLSSettings.Mldsa65Seed != "SEED" {
		t.Fatalf("seed: %q", cfg.TLSSettings.Mldsa65Seed)
	}
	if cfg.TLSSettings.Show == nil || !*cfg.TLSSettings.Show {
		t.Fatal("show should be true")
	}
}

func TestBuildUniProxyConfig_PlainTLS_NoMinClientVer(t *testing.T) {
	node := &models.Node{
		NodeType: "vless",
		Port:     443,
		Network:  "ws",
		TLS:      1,
		SNI:      "a.example.com",
		Host:     "a.example.com",
		Path:     "/ws",
	}
	cfg := buildUniProxyConfig(node)
	if cfg.Decryption != "none" {
		t.Fatalf("decryption: %q", cfg.Decryption)
	}
	if cfg.TLSSettings == nil || cfg.TLSSettings.MinClientVer != "" {
		t.Fatalf("plain TLS should not force min_client_ver: %+v", cfg.TLSSettings)
	}
	if cfg.Host != "a.example.com" || cfg.Path != "/ws" {
		t.Fatalf("ws host/path: %q %q", cfg.Host, cfg.Path)
	}
	if cfg.NetworkSettings == nil || cfg.NetworkSettings.Path != "/ws" {
		t.Fatalf("network_settings: %+v", cfg.NetworkSettings)
	}
}

// 形态 B: VLESS + TLS + XHTTP + CDN
func TestBuildUniProxyConfig_XHTTPCDN(t *testing.T) {
	node := &models.Node{
		NodeType: "vless",
		Port:     443,
		Network:  "xhttp",
		TLS:      1,
		Flow:     "", // Vision off
		Host:     "cdn.example.com",
		Path:     "/vless-cdn",
		SNI:      "cdn.example.com",
	}
	cfg := buildUniProxyConfig(node)
	if cfg.Network != "xhttp" {
		t.Fatalf("network: %q", cfg.Network)
	}
	if cfg.Flow != "" {
		t.Fatalf("CDN flow should be empty, got %q", cfg.Flow)
	}
	if cfg.Host != "cdn.example.com" || cfg.Path != "/vless-cdn" {
		t.Fatalf("host/path: %q %q", cfg.Host, cfg.Path)
	}
	if cfg.NetworkSettings == nil {
		t.Fatal("network_settings nil")
	}
	if cfg.NetworkSettings.Host != "cdn.example.com" || cfg.NetworkSettings.Path != "/vless-cdn" || cfg.NetworkSettings.Mode != "auto" {
		t.Fatalf("network_settings: %+v", cfg.NetworkSettings)
	}
	if cfg.TLSSettings == nil || cfg.TLSSettings.ServerName != "cdn.example.com" {
		t.Fatalf("tls_settings: %+v", cfg.TLSSettings)
	}
	if cfg.TLSSettings.PrivateKey != "" || cfg.TLSSettings.PublicKey != "" {
		t.Fatal("CDN TLS must not include REALITY keys")
	}
	if cfg.Decryption != "none" {
		t.Fatalf("decryption: %q", cfg.Decryption)
	}
	if cfg.BaseConfig == nil {
		t.Fatal("base_config required")
	}
}

func TestBuildUniProxyConfig_AnyTLS_NoReality(t *testing.T) {
	node := &models.Node{
		NodeType: "anytls",
		Port:     443,
		Network:  "tcp",
		TLS:      1,
		SNI:      "anytls.example.com",
		Flow:     "should-not-matter",
	}
	cfg := buildUniProxyConfig(node)
	if cfg.Flow != "" {
		t.Fatalf("anytls should omit flow, got %q", cfg.Flow)
	}
	if cfg.Decryption != "" {
		t.Fatalf("anytls should omit decryption, got %q", cfg.Decryption)
	}
	if cfg.TLSSettings != nil && (cfg.TLSSettings.PrivateKey != "" || cfg.TLSSettings.MinClientVer != "") {
		t.Fatalf("anytls must not have REALITY fields: %+v", cfg.TLSSettings)
	}
	if cfg.TLSSettings == nil || cfg.TLSSettings.ServerName != "anytls.example.com" {
		t.Fatalf("anytls sni: %+v", cfg.TLSSettings)
	}
}

func TestBuildUniProxyConfig_JSONOmitsEmptyPQ(t *testing.T) {
	rs, _ := json.Marshal(map[string]any{
		"public_key": "p", "private_key": "k", "short_id": "s",
	})
	node := &models.Node{
		NodeType: "vless", Port: 443, Network: "tcp", TLS: 2, SNI: "x.com",
		RealitySettings: rs,
	}
	raw, err := json.Marshal(buildUniProxyConfig(node))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	// decryption present as none
	if m["decryption"] != "none" {
		t.Fatalf("decryption in json: %v", m["decryption"])
	}
	ts := m["tls_settings"].(map[string]any)
	if _, ok := ts["mldsa65_seed"]; ok {
		t.Fatal("mldsa65_seed must be omitted when empty")
	}
	if _, ok := ts["show"]; ok {
		t.Fatal("show must be omitted when false")
	}
	if ts["min_client_ver"] != "1.8.0" {
		t.Fatalf("min_client_ver: %v", ts["min_client_ver"])
	}
}
