package payment

import "testing"

func TestSignMD5_BEpusdtDocExample(t *testing.T) {
	// From BEpusdt docs/api/api.md signature example
	params := map[string]string{
		"order_id":     "20220201030210321",
		"amount":       "42",
		"notify_url":   "http://example.com/notify",
		"redirect_url": "http://example.com/redirect",
	}
	token := "epusdt_password_xasddawqe"
	got := SignMD5(params, token)
	want := "1cd4b52df5587cfb1968b0c0c6e156cd"
	if got != want {
		t.Fatalf("signature mismatch:\n got %s\nwant %s", got, want)
	}
}

func TestSignMD5_SkipsEmptyAndSignature(t *testing.T) {
	params := map[string]string{
		"order_id":   "A1",
		"amount":     "10.00",
		"name":       "",
		"signature":  "should-ignore",
		"notify_url": "https://x/n",
	}
	s1 := SignMD5(params, "tok")
	s2 := SignMD5(map[string]string{
		"amount":     "10.00",
		"notify_url": "https://x/n",
		"order_id":   "A1",
	}, "tok")
	if s1 != s2 {
		t.Fatalf("empty/signature should not affect sign: %s vs %s", s1, s2)
	}
}

func TestFormatFiatFromCents(t *testing.T) {
	cases := map[int64]string{
		0:    "0.00",
		1:    "0.01",
		100:  "1.00",
		2888: "28.88",
		42:   "0.42",
	}
	for in, want := range cases {
		if got := FormatFiatFromCents(in); got != want {
			t.Errorf("FormatFiatFromCents(%d)=%q want %q", in, got, want)
		}
	}
}

func TestFiatToCents(t *testing.T) {
	if FiatToCents(28.88) != 2888 {
		t.Fatalf("28.88 → %d", FiatToCents(28.88))
	}
	if FiatToCents(0.01) != 1 {
		t.Fatalf("0.01 → %d", FiatToCents(0.01))
	}
}

func TestSignMD5_GMPayDocExample(t *testing.T) {
	// From GMWalletApp/epusdt wiki/API.md
	params := map[string]string{
		"pid":          "1000",
		"order_id":     "ORD202605230001",
		"currency":     "cny",
		"token":        "usdt",
		"network":      "tron",
		"amount":       "100",
		"notify_url":   "https://merchant.example/notify",
		"redirect_url": "https://merchant.example/return",
		"name":         "VIP",
	}
	got := SignMD5(params, "epusdt_secret_key")
	want := "476412c422f4dd75c3d533f5c47a9cac"
	if got != want {
		t.Fatalf("gmpay signature:\n got %s\nwant %s", got, want)
	}
}
