package services

import (
	"testing"
	"time"

	"K2board/internal/models"
)

// Product contract: enable = ban; expire_at = service window.
func TestAccountAccess_SeparationMatrix(t *testing.T) {
	now := time.Now().Unix()

	type row struct {
		name string
		u    models.User
		// expected gates
		login   bool
		order   bool
		proxy   bool
		active  bool // UserHasActiveService
		traffic bool // IsTrafficExceeded
	}

	cases := []row{
		{
			name:   "new blank enabled",
			u:      models.User{Enable: true},
			login:  true,
			order:  true,
			proxy:  true, // no expire; proxy list may still filter by group
			active: false,
		},
		{
			name:   "active paid",
			u:      models.User{Enable: true, PlanID: 1, GroupID: 1, ExpireAt: now + 86400, TrafficLimit: 100, TrafficUsed: 10},
			login:  true,
			order:  true,
			proxy:  true,
			active: true,
		},
		{
			name:   "expired enabled — renew path",
			u:      models.User{Enable: true, PlanID: 1, GroupID: 1, ExpireAt: now - 60, TrafficLimit: 100, TrafficUsed: 10},
			login:  true,  // must login
			order:  true,  // must order/renew
			proxy:  false, // must not get nodes
			active: false,
		},
		{
			name:   "banned still active window",
			u:      models.User{Enable: false, PlanID: 1, GroupID: 1, ExpireAt: now + 86400},
			login:  false,
			order:  false,
			proxy:  false,
			active: true, // entitlement exists; free re-claim blocked
		},
		{
			name:   "banned and expired",
			u:      models.User{Enable: false, PlanID: 1, GroupID: 1, ExpireAt: now - 60},
			login:  false,
			order:  false,
			proxy:  false,
			active: false,
		},
		{
			name:   "permanent plan enabled",
			u:      models.User{Enable: true, PlanID: 2, GroupID: 2, ExpireAt: 0},
			login:  true,
			order:  true,
			proxy:  true,
			active: true,
		},
		{
			name:    "over traffic still may login/renew",
			u:       models.User{Enable: true, PlanID: 1, GroupID: 1, ExpireAt: now + 100, TrafficLimit: 100, TrafficUsed: 100},
			login:   true,
			order:   true,
			proxy:   true, // CanUseProxyService does not check traffic; IsTrafficExceeded separate
			active:  true,
			traffic: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			u := c.u
			if got := CanAccountLogin(&u); got != c.login {
				t.Errorf("CanAccountLogin=%v want %v", got, c.login)
			}
			if got := CanCreateOrder(&u); got != c.order {
				t.Errorf("CanCreateOrder=%v want %v", got, c.order)
			}
			if got := CanUseProxyService(&u, now); got != c.proxy {
				t.Errorf("CanUseProxyService=%v want %v", got, c.proxy)
			}
			if got := UserHasActiveService(&u, now); got != c.active {
				t.Errorf("UserHasActiveService=%v want %v", got, c.active)
			}
			if got := IsTrafficExceeded(&u); got != c.traffic {
				t.Errorf("IsTrafficExceeded=%v want %v", got, c.traffic)
			}
			// Cross-invariants
			if IsAccountBanned(&u) && (CanAccountLogin(&u) || CanCreateOrder(&u) || CanUseProxyService(&u, now)) {
				t.Error("banned account must not pass login/order/proxy")
			}
			if IsServiceExpired(&u, now) && CanUseProxyService(&u, now) {
				t.Error("expired account must not use proxy")
			}
			if !IsAccountBanned(&u) && IsServiceExpired(&u, now) {
				if !CanAccountLogin(&u) || !CanCreateOrder(&u) {
					t.Error("expired+enabled must login and order for renew")
				}
			}
		})
	}
}

func TestIsAccountBanned_Nil(t *testing.T) {
	if !IsAccountBanned(nil) {
		t.Fatal("nil user should be banned")
	}
	if CanAccountLogin(nil) || CanCreateOrder(nil) || CanUseProxyService(nil, time.Now().Unix()) {
		t.Fatal("nil user must fail all gates")
	}
}

func TestIsServiceExpired_PermanentAndBoundary(t *testing.T) {
	now := int64(1_700_000_000)
	perm := &models.User{Enable: true, PlanID: 1, ExpireAt: 0}
	if IsServiceExpired(perm, now) {
		t.Fatal("expire_at=0 is permanent, not expired")
	}
	// boundary: expire_at == now is NOT expired (strict <)
	atNow := &models.User{Enable: true, PlanID: 1, ExpireAt: now}
	if IsServiceExpired(atNow, now) {
		t.Fatal("expire_at == now should still be valid")
	}
	past := &models.User{Enable: true, PlanID: 1, ExpireAt: now - 1}
	if !IsServiceExpired(past, now) {
		t.Fatal("expire_at < now should be expired")
	}
}

func TestCanUserRenewPlan_ExpiredEnabled(t *testing.T) {
	now := time.Now().Unix()
	u := &models.User{Enable: true, PlanID: 5, ExpireAt: now - 100}
	p := &models.Plan{ID: 5, Enable: true, AllowRenew: true, ShowOnShop: false}
	if !CanUserRenewPlan(u, p) {
		t.Fatal("expired enabled user must be able to renew current plan")
	}
	if !CanUserPurchasePlan(u, p) {
		t.Fatal("expired enabled user must purchase renew plan")
	}
	// banned same plan: purchase policy does not check enable (API does)
	banned := &models.User{Enable: false, PlanID: 5, ExpireAt: now - 100}
	if !CanUserRenewPlan(banned, p) {
		// plan-level renew eligibility is independent; ban enforced at handler
		t.Log("note: CanUserRenewPlan ignores ban (handler uses IsAccountBanned)")
	}
	if CanCreateOrder(banned) {
		t.Fatal("banned must not create order")
	}
}
