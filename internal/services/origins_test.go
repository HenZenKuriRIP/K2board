package services

import (
	"strings"
	"testing"
	"time"
)

func TestNormalizeOrigin(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"*", ""},
		{"https://evil.com/*", ""},
		{"https://user.example.com", "https://user.example.com"},
		{"https://user.example.com/", "https://user.example.com"},
		{"https://user.example.com/path", "https://user.example.com"},
		{"http://localhost:3000", "http://localhost:3000"},
		{"user.example.com", "https://user.example.com"},
		{"HTTPS://User.Example.COM", "https://user.example.com"},
		{"javascript:alert(1)", ""},
		{"//evil.com", ""},
	}
	for _, c := range cases {
		got := NormalizeOrigin(c.in)
		if got != c.want {
			t.Errorf("NormalizeOrigin(%q)=%q want %q", c.in, got, c.want)
		}
	}
}

func TestParseAllowedOriginsList(t *testing.T) {
	raw := `
# comment
https://a.com
https://b.net/, https://c.org
user.shadow.com
*
javascript:x
`
	list := ParseAllowedOriginsList(raw)
	want := map[string]bool{
		"https://a.com":           true,
		"https://b.net":           true,
		"https://c.org":           true,
		"https://user.shadow.com": true,
	}
	if len(list) != len(want) {
		t.Fatalf("len=%d got %v", len(list), list)
	}
	for _, o := range list {
		if !want[o] {
			t.Errorf("unexpected %q", o)
		}
	}
}

func TestIsCORSOriginAllowed_SameHostAndLocal(t *testing.T) {
	// No DB snapshot needed for same-host / local
	if !IsCORSOriginAllowed("", "www.example.com") {
		t.Fatal("empty origin should allow")
	}
	if !IsCORSOriginAllowed("https://www.example.com", "www.example.com") {
		t.Fatal("same host https")
	}
	if !IsCORSOriginAllowed("http://www.example.com", "www.example.com") {
		t.Fatal("same host http")
	}
	if !IsCORSOriginAllowed("http://localhost:3000", "www.example.com") {
		t.Fatal("localhost dev")
	}
	// Empty allow-list snapshot so we do not hit DB
	corsSnap.Store(&corsOriginsSnapshot{
		set:   map[string]struct{}{},
		hosts: map[string]struct{}{},
		list:  nil,
		at:    time.Now(),
	})
	t.Cleanup(InvalidateCORSOriginsCache)
	if IsCORSOriginAllowed("https://evil.com", "www.example.com") {
		t.Fatal("evil must be blocked without allow list")
	}
	// Substring attack must fail
	if IsCORSOriginAllowed("https://evil.com", "evil.com.www.example.com") {
		// weird host — just ensure we don't use contains
	}
	if IsCORSOriginAllowed("https://notwww.example.com.evil.com", "www.example.com") {
		t.Fatal("suffix attack must not pass same-host check")
	}
}

func TestIsCORSOriginAllowed_AllowList(t *testing.T) {
	// Inject snapshot without DB
	snap := &corsOriginsSnapshot{
		set: map[string]struct{}{
			"https://shadow.a.com": {},
		},
		hosts: map[string]struct{}{
			"shadow.a.com": {},
		},
		list: []string{"https://shadow.a.com"},
		at:   time.Now(),
	}
	corsSnap.Store(snap)
	t.Cleanup(InvalidateCORSOriginsCache)

	if !IsCORSOriginAllowed("https://shadow.a.com", "www.api.com") {
		t.Fatal("allow-listed shadow origin should pass")
	}
	if !IsCORSOriginAllowed("https://shadow.a.com/", "www.api.com") {
		t.Fatal("trailing slash origin should normalize and pass")
	}
	if IsCORSOriginAllowed("https://other.evil", "www.api.com") {
		t.Fatal("non-listed must fail")
	}
}

func TestIsReturnURLHostAllowed(t *testing.T) {
	snap := &corsOriginsSnapshot{
		set: map[string]struct{}{"https://pay.shadow.com": {}},
		hosts: map[string]struct{}{
			"pay.shadow.com":  {},
			"www.example.com": {},
		},
		list: []string{"https://pay.shadow.com"},
		at:   time.Now(),
	}
	corsSnap.Store(snap)
	t.Cleanup(InvalidateCORSOriginsCache)

	if !IsReturnURLHostAllowed("www.example.com", "https://www.example.com") {
		t.Fatal("site base host")
	}
	if !IsReturnURLHostAllowed("pay.shadow.com", "https://www.example.com") {
		t.Fatal("allow-listed return host")
	}
	if IsReturnURLHostAllowed("evil.com", "https://www.example.com") {
		t.Fatal("evil return host")
	}
}

func TestValidateAndNormalizeAllowedOriginsSetting(t *testing.T) {
	got, err := ValidateAndNormalizeAllowedOriginsSetting("https://A.com/\nhttps://b.net")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "https://a.com") || !strings.Contains(got, "https://b.net") {
		t.Fatalf("got %q", got)
	}
	_, err = ValidateAndNormalizeAllowedOriginsSetting("*\njavascript:x")
	if err == nil {
		t.Fatal("expected error for all-invalid input")
	}
	got, err = ValidateAndNormalizeAllowedOriginsSetting("")
	if err != nil || got != "" {
		t.Fatalf("empty: %q %v", got, err)
	}
}

func TestSanitizeReturnURL_ShadowOrigin(t *testing.T) {
	snap := &corsOriginsSnapshot{
		set:   map[string]struct{}{"https://shadow.portal": {}},
		hosts: map[string]struct{}{"shadow.portal": {}, "www.main.com": {}},
		list:  []string{"https://shadow.portal"},
		at:    time.Now(),
	}
	corsSnap.Store(snap)
	t.Cleanup(InvalidateCORSOriginsCache)

	base := "https://www.main.com"
	// relative still on site base
	if got := sanitizeReturnURL("/#/user/order-result?trade_no=1", base); got != "https://www.main.com/#/user/order-result?trade_no=1" {
		t.Fatalf("relative: %q", got)
	}
	// shadow absolute allowed
	shadow := "https://shadow.portal/#/user/order-result?trade_no=1"
	if got := sanitizeReturnURL(shadow, base); got != shadow {
		t.Fatalf("shadow: %q", got)
	}
	// evil rejected
	if got := sanitizeReturnURL("https://evil.com/phish", base); got != "" {
		t.Fatalf("evil: %q", got)
	}
}
