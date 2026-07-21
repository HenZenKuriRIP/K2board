package payment

import "testing"

func TestPickReturnTradeNo(t *testing.T) {
	// Epay: merchant out_trade_no first, ignore platform trade_no
	if got := pickReturnTradeNo("K220260721abc", "", "", "202407219999"); got != "K220260721abc" {
		t.Fatalf("out_trade_no preferred: got %q", got)
	}
	if got := pickReturnTradeNo("", "K220260721tn", "", "EPAY999"); got != "K220260721tn" {
		t.Fatalf("tn preferred over trade_no: got %q", got)
	}
	// Only platform trade_no present (legacy) — still return it
	if got := pickReturnTradeNo("", "", "", "ONLY_PLATFORM"); got != "ONLY_PLATFORM" {
		t.Fatalf("fallback trade_no: got %q", got)
	}
	// Comma-joined duplicates: prefer K2*
	if got := pickReturnTradeNo("", "", "", "K220260721abc,202407219999"); got != "K220260721abc" {
		t.Fatalf("K2 from comma list: got %q", got)
	}
	if got := pickReturnTradeNo("", "", "", ""); got != "" {
		t.Fatalf("empty: got %q", got)
	}
}
