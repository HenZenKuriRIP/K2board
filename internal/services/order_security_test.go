package services

import (
	"errors"
	"testing"
	"time"

	"K2board/internal/models"
)

func TestSanitizeReturnURL(t *testing.T) {
	base := "https://www.example.com"
	// Ensure cache does not leak other tests' allow-lists
	InvalidateCORSOriginsCache()
	// Inject empty snapshot so only site base host is trusted
	corsSnap.Store(&corsOriginsSnapshot{
		set:   map[string]struct{}{"https://www.example.com": {}},
		hosts: map[string]struct{}{"www.example.com": {}},
		list:  []string{"https://www.example.com"},
		at:    time.Now(),
	})
	t.Cleanup(InvalidateCORSOriginsCache)

	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"https://www.example.com/#/user/order-result?trade_no=1", "https://www.example.com/#/user/order-result?trade_no=1"},
		{"https://evil.com/phish", ""},
		{"//evil.com/x", ""},
		{"/user/orders", "https://www.example.com/user/orders"},
		{"http://www.example.com/ok", "http://www.example.com/ok"},
		{"javascript:alert(1)", ""},
	}
	for _, c := range cases {
		got := sanitizeReturnURL(c.in, base)
		if got != c.want {
			t.Errorf("sanitizeReturnURL(%q)=%q want %q", c.in, got, c.want)
		}
	}
}

func TestNormalizeEmail(t *testing.T) {
	if got := NormalizeEmail("  Foo@Bar.COM "); got != "foo@bar.com" {
		t.Fatalf("NormalizeEmail=%q", got)
	}
}

func TestUserHasActiveService(t *testing.T) {
	now := time.Now().Unix()
	cases := []struct {
		name string
		u    models.User
		want bool
	}{
		{"blank new user", models.User{Enable: true}, false},
		// Ban is orthogonal: permanent plan still has entitlement (free re-claim blocked).
		// Login/order still refuse enable=false via IsAccountBanned.
		{"banned with permanent plan", models.User{Enable: false, PlanID: 1, GroupID: 1}, true},
		{"banned expired plan", models.User{Enable: false, PlanID: 1, ExpireAt: now - 10}, false},
		{"plan permanent", models.User{Enable: true, PlanID: 1, ExpireAt: 0}, true},
		{"group permanent", models.User{Enable: true, GroupID: 2, ExpireAt: 0}, true},
		{"not expired", models.User{Enable: true, PlanID: 1, ExpireAt: now + 3600}, true},
		{"expired", models.User{Enable: true, PlanID: 1, ExpireAt: now - 10}, false},
	}
	for _, c := range cases {
		got := UserHasActiveService(&c.u, now)
		if got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

func TestAssertFreePlanAllowed_NilAndPaid(t *testing.T) {
	// price > 0 always ok without DB claim checks
	u := &models.User{ID: 1, Email: "a@b.com", Enable: true}
	if err := assertFreePlanAllowed(u, &models.Plan{Price: 100}); err != nil {
		t.Fatal(err)
	}
}

func TestValidatePlanForShop_DraftAndNoGroup(t *testing.T) {
	if err := ValidatePlanForShop(false, 0); err != nil {
		t.Fatalf("draft should skip: %v", err)
	}
	if err := ValidatePlanForShop(true, 0); err == nil || !errors.Is(err, ErrPlanShopNoGroup) {
		t.Fatalf("want ErrPlanShopNoGroup, got %v", err)
	}
}

func TestCanUserPurchasePlan(t *testing.T) {
	u := &models.User{ID: 1, PlanID: 5}
	onShop := &models.Plan{ID: 9, Enable: true, ShowOnShop: true, AllowRenew: false}
	if !CanUserPurchasePlan(u, onShop) {
		t.Fatal("on-shop should allow any user")
	}
	offNoRenew := &models.Plan{ID: 5, Enable: true, ShowOnShop: false, AllowRenew: false}
	if CanUserPurchasePlan(u, offNoRenew) {
		t.Fatal("off-shop without renew must deny")
	}
	offRenewOther := &models.Plan{ID: 99, Enable: true, ShowOnShop: false, AllowRenew: true}
	if CanUserPurchasePlan(u, offRenewOther) {
		t.Fatal("renew only for current plan_id")
	}
	offRenewSelf := &models.Plan{ID: 5, Enable: true, ShowOnShop: false, AllowRenew: true}
	if !CanUserPurchasePlan(u, offRenewSelf) {
		t.Fatal("holder should renew off-shop when allow_renew")
	}
	disabled := &models.Plan{ID: 5, Enable: false, ShowOnShop: true, AllowRenew: true}
	if CanUserPurchasePlan(u, disabled) {
		t.Fatal("disabled plan never purchasable")
	}
}

func TestCanUserRenewPlan(t *testing.T) {
	u := &models.User{PlanID: 3}
	p := &models.Plan{ID: 3, Enable: true, AllowRenew: true, ShowOnShop: false}
	if !CanUserRenewPlan(u, p) {
		t.Fatal("expected renew")
	}
	p.AllowRenew = false
	if CanUserRenewPlan(u, p) {
		t.Fatal("allow_renew off")
	}
}
