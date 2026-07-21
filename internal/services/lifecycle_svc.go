package services

import (
	"log/slog"
	"strconv"
	"sync/atomic"
	"time"

	"K2board/internal/database"
	"K2board/internal/models"
)

const configVersionKey = "config_version"

var configVersion atomic.Int64

func init() {
	configVersion.Store(1)
}

// InitConfigVersion loads the shared config version from DB (multi-instance safe).
// Call once after database is ready.
func InitConfigVersion() {
	var s models.Setting
	if err := database.DB.Where("key = ?", configVersionKey).First(&s).Error; err != nil {
		database.DB.Create(&models.Setting{Key: configVersionKey, Value: "1"})
		configVersion.Store(1)
		return
	}
	if v, err := strconv.ParseInt(s.Value, 10, 64); err == nil && v > 0 {
		configVersion.Store(v)
	} else {
		configVersion.Store(1)
	}
}

// GetConfigVersion returns the current config version (memory cache; refreshed on bump).
func GetConfigVersion() int64 {
	return configVersion.Load()
}

// BumpConfigVersion increments config version in DB and local cache so all instances converge.
func BumpConfigVersion() {
	// Optimistic local bump for same-process readers
	next := configVersion.Add(1)

	var s models.Setting
	err := database.DB.Where("key = ?", configVersionKey).First(&s).Error
	if err != nil {
		if createErr := database.DB.Create(&models.Setting{
			Key:   configVersionKey,
			Value: strconv.FormatInt(next, 10),
		}).Error; createErr != nil {
			slog.Error("config_version create failed", "error", createErr)
		}
		return
	}
	// Use max(local, db+1) to reduce races across instances
	dbVal, _ := strconv.ParseInt(s.Value, 10, 64)
	if dbVal >= next {
		next = dbVal + 1
	}
	if err := database.DB.Model(&s).Update("value", strconv.FormatInt(next, 10)).Error; err != nil {
		slog.Error("config_version bump failed", "error", err)
		return
	}
	// Ensure local cache is at least the stored value
	for {
		cur := configVersion.Load()
		if cur >= next || configVersion.CompareAndSwap(cur, next) {
			break
		}
	}
}

// RefreshConfigVersion re-reads version from DB (optional periodic sync for multi-instance).
func RefreshConfigVersion() {
	var s models.Setting
	if err := database.DB.Where("key = ?", configVersionKey).First(&s).Error; err != nil {
		return
	}
	v, err := strconv.ParseInt(s.Value, 10, 64)
	if err != nil || v <= 0 {
		return
	}
	for {
		cur := configVersion.Load()
		if cur >= v || configVersion.CompareAndSwap(cur, v) {
			return
		}
	}
}

// AutoResetTraffic zeros traffic_used for users whose plan.reset_day is today's
// calendar day (1–28, server local timezone). Runs at most once per user per day
// via last_traffic_reset_at. Plans with reset_day=0 never auto-reset (quota is
// for the whole subscription period until expire).
// Uses users.plan_id (not group_id) so multi-plan groups stay correct.
func AutoResetTraffic() int64 {
	now := time.Now()
	day := now.Day()
	if day > 28 {
		// reset_day UI/model only allows 1–28; skip 29–31
		return 0
	}

	// Start of local calendar day — users already stamped today are skipped
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Unix()
	nowUnix := now.Unix()

	var userIDs []uint
	err := database.DB.Model(&models.User{}).
		Joins("JOIN plans ON plans.id = users.plan_id").
		Where("users.plan_id > 0").
		Where("plans.enable = ?", true).
		Where("plans.reset_day = ?", day).
		Where("plans.reset_day > 0").
		Where("users.is_admin = ?", false).
		// Once per calendar day (even if traffic_used is already 0 — stamp only once)
		Where("users.last_traffic_reset_at < ? OR users.last_traffic_reset_at = 0", todayStart).
		Pluck("users.id", &userIDs).Error
	if err != nil {
		slog.Error("auto reset traffic query failed", "error", err)
		return 0
	}
	if len(userIDs) == 0 {
		return 0
	}

	res := database.DB.Model(&models.User{}).
		Where("id IN ?", userIDs).
		Updates(map[string]any{
			"traffic_used":           0,
			"last_traffic_reset_at": nowUnix,
		})
	if res.Error != nil {
		slog.Error("auto reset traffic failed", "error", res.Error)
		return 0
	}
	if res.RowsAffected > 0 {
		BumpConfigVersion()
		slog.Info("auto reset traffic", "users", res.RowsAffected, "reset_day", day)
	}
	return res.RowsAffected
}

// Product model (enable vs expire_at):
//
//	enable    = admin ban / risk control only
//	expire_at = whether subscription service is usable
//
// Expired users (enable=true, past expire_at) may still log in and renew.
// Banned users (enable=false) cannot log in, order, or pull subscriptions.
// Proxy / UniProxy / subscribe continue to require enable + not-expired + quota.
//
// AutoDisableExpiredUsers is the scheduler entrypoint name (kept for metrics /
// admin UI compatibility). It no longer sets enable=false on expiry.
// On first run after upgrade it re-enables non-admin accounts that the legacy
// job had flipped to enable=false solely because they were expired, so those
// users can log in and renew. Intentional admin bans of still-active users
// (expire_at == 0 permanent, or expire_at > now) are never touched.
//
// Returns number of accounts repaired on the one-time migration; 0 afterwards.
func AutoDisableExpiredUsers() int64 {
	return repairLegacyAutoDisabledExpiredUsersOnce()
}

const legacyAutoDisableRepairKey = "repair_legacy_auto_disable_v1"

// repairLegacyAutoDisabledExpiredUsersOnce re-enables expired non-admin users
// that still have enable=false from the old auto-disable job. Runs at most once
// (flagged in settings). Safe to call every scheduler tick.
func repairLegacyAutoDisabledExpiredUsersOnce() int64 {
	var s models.Setting
	err := database.DB.Where("key = ?", legacyAutoDisableRepairKey).First(&s).Error
	if err == nil && s.Value == "done" {
		return 0
	}
	// Setting missing or incomplete → attempt repair then mark done.
	now := time.Now().Unix()
	res := database.DB.Model(&models.User{}).
		Where("enable = ?", false).
		Where("is_admin = ?", false).
		// Only accounts the legacy job would have touched: had a finite expiry in the past.
		Where("expire_at > 0 AND expire_at < ?", now).
		Update("enable", true)
	if res.Error != nil {
		slog.Error("legacy auto-disable repair failed", "error", res.Error)
		return 0
	}
	// Mark done even if 0 rows — avoid re-scanning every tick forever.
	// If mark fails, next tick will retry (idempotent enable=true).
	if markErr := upsertSetting(legacyAutoDisableRepairKey, "done"); markErr != nil {
		slog.Error("legacy auto-disable repair flag write failed", "error", markErr)
	}
	if res.RowsAffected > 0 {
		BumpConfigVersion()
		slog.Info("repaired legacy auto-disabled expired users (enable restored)",
			"count", res.RowsAffected)
	} else {
		slog.Info("legacy auto-disable repair: nothing to restore")
	}
	return res.RowsAffected
}

func upsertSetting(key, value string) error {
	var s models.Setting
	err := database.DB.Where("key = ?", key).First(&s).Error
	if err != nil {
		return database.DB.Create(&models.Setting{Key: key, Value: value}).Error
	}
	return database.DB.Model(&s).Update("value", value).Error
}
