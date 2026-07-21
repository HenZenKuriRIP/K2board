package services

import (
	"time"

	"K2board/internal/models"
)

// Account access policy (enable vs expire_at):
//
//	enable    = admin ban / risk control only
//	expire_at = whether subscription service is usable
//
// Gates:
//   - Login / user portal / orders / profile / withdraw: require !IsAccountBanned
//   - Subscribe / UniProxy user list / traffic accept: require CanUseProxyService
//     (+ traffic quota where applicable)
//
// Expired + enabled → may log in and renew; must not receive proxy/nodes.
// Banned (enable=false) → must not log in, order, or pull subscription.

// IsAccountBanned reports admin/risk ban (enable=false). Nil user is treated as banned.
func IsAccountBanned(u *models.User) bool {
	return u == nil || !u.Enable
}

// IsServiceExpired reports plan window past (finite expire_at in the past).
// Permanent (expire_at==0) is never expired by time. Nil user → expired.
func IsServiceExpired(u *models.User, nowUnix int64) bool {
	if u == nil {
		return true
	}
	if nowUnix <= 0 {
		nowUnix = time.Now().Unix()
	}
	return u.ExpireAt > 0 && u.ExpireAt < nowUnix
}

// CanAccountLogin is true when the account is not banned.
// Caller still applies portal-specific rules (e.g. block is_admin on user portal).
func CanAccountLogin(u *models.User) bool {
	return !IsAccountBanned(u)
}

// CanCreateOrder is true when the account is not banned (expiry does not block renew).
func CanCreateOrder(u *models.User) bool {
	return !IsAccountBanned(u)
}

// CanUseProxyService is true when not banned and not expired.
// Traffic/device limits are checked separately at pull/flush sites.
func CanUseProxyService(u *models.User, nowUnix int64) bool {
	if IsAccountBanned(u) {
		return false
	}
	if IsServiceExpired(u, nowUnix) {
		return false
	}
	return true
}

// IsTrafficExceeded is true when traffic_limit > 0 and used >= limit.
func IsTrafficExceeded(u *models.User) bool {
	if u == nil {
		return true
	}
	return u.TrafficLimit > 0 && u.TrafficUsed >= u.TrafficLimit
}
