package utils

import (
	"encoding/json"
	"testing"
)

func TestParseDestHostPort(t *testing.T) {
	h, p := ParseDestHostPort("www.example.com:8443", "sni.com")
	if h != "www.example.com" || p != "8443" {
		t.Fatalf("got %s %s", h, p)
	}
	h, p = ParseDestHostPort("", "sni.only")
	if h != "sni.only" || p != "443" {
		t.Fatalf("fallback sni: %s %s", h, p)
	}
	h, p = ParseDestHostPort("bare.host", "")
	if h != "bare.host" || p != "443" {
		t.Fatalf("bare: %s %s", h, p)
	}
}

func TestMergeRealityJSON_PreservesPQFields(t *testing.T) {
	in, _ := json.Marshal(map[string]any{
		"public_key":   "P",
		"private_key":  "K",
		"short_id":     "sid",
		"mldsa65_seed": "SEED",
		"min_client_ver": "1.8.0",
	})
	out := MergeRealityJSON(in, "www.x.com")
	var m map[string]any
	_ = json.Unmarshal(out, &m)
	if m["mldsa65_seed"] != "SEED" {
		t.Fatalf("lost seed: %v", m)
	}
	if m["public_key"] != "P" {
		t.Fatalf("lost key: %v", m)
	}
	if m["fingerprint"] == nil || m["fingerprint"] == "" {
		t.Fatal("should fill fingerprint default")
	}
}

func TestNormalizeVlessCrypto(t *testing.T) {
	if NormalizeVlessCrypto("") != "" || NormalizeVlessCrypto("none") != "" {
		t.Fatal("empty/none should normalize off")
	}
	if NormalizeVlessCrypto("  mlkem  ") != "mlkem" {
		t.Fatal("trim")
	}
}
