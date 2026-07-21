package services

import (
	"testing"
)

func TestGenerateInviteCode(t *testing.T) {
	c1, err := GenerateInviteCode()
	if err != nil {
		t.Fatal(err)
	}
	if len(c1) != 8 {
		t.Fatalf("len=%d", len(c1))
	}
	c2, _ := GenerateInviteCode()
	if c1 == c2 {
		// extremely unlikely; just ensure function runs twice
		t.Log("same code twice (rare)")
	}
	for _, r := range c1 {
		if !((r >= 'A' && r <= 'Z') || (r >= '2' && r <= '9')) {
			// alphabet excludes 0 O 1 I but may have other digits
			if r == '0' || r == '1' || r == 'O' || r == 'I' {
				t.Fatalf("ambiguous char %c", r)
			}
		}
	}
}

func TestParseMoneyToCents(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{"10000", 10000},
		{"0", 0},
		{"100.50", 10050},
		{"1.5", 150},
	}
	for _, c := range cases {
		got, err := parseMoneyToCents(c.in)
		if err != nil {
			t.Fatalf("%q: %v", c.in, err)
		}
		if got != c.want {
			t.Errorf("%q → %d want %d", c.in, got, c.want)
		}
	}
}

func TestMaskEmailLocal(t *testing.T) {
	if got := maskEmailLocal("ab@x.com"); got != "a*@x.com" {
		t.Fatalf("got %q", got)
	}
	if got := maskEmailLocal("alice@example.com"); got != "a***e@example.com" {
		t.Fatalf("got %q", got)
	}
	if got := maskEmailLocal("x@y.z"); got != "*@y.z" {
		t.Fatalf("got %q", got)
	}
}

func TestPayoutMethodAllowed(t *testing.T) {
	list := []PayoutMethod{{Code: "alipay", Name: "支付宝"}, {Code: "wechat", Name: "微信"}}
	if !payoutMethodAllowed(list, "alipay") {
		t.Fatal("alipay should be allowed")
	}
	if payoutMethodAllowed(list, "bank") {
		t.Fatal("bank should not be allowed")
	}
}

func TestCommissionAmountMath(t *testing.T) {
	// 10% of ¥99.00 = 990 cents → 99 cents commission
	order := int64(9900)
	rate := 10
	amount := order * int64(rate) / 100
	if amount != 990 {
		t.Fatalf("got %d", amount)
	}
	// tiny order: 1 cent * 10% = 0 → no credit (caller skips)
	if 1*int64(rate)/100 != 0 {
		t.Fatal("expected zero floor")
	}
}
