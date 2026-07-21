package services

import "testing"

func TestSanitizeOnlineIP(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"  1.2.3.4  ", "1.2.3.4"},
		{"2001:db8::1", "2001:db8::1"},
		{"1.2.3.4:443", "1.2.3.4"},
		{"not-an-ip", ""},
		{"http://evil.com", ""},
		{"1.2.3.4/24", ""},
		{string(make([]byte, 100)), ""}, // too long
	}
	for _, c := range cases {
		got := SanitizeOnlineIP(c.in)
		if got != c.want {
			t.Errorf("SanitizeOnlineIP(%q)=%q want %q", c.in, got, c.want)
		}
	}
}
