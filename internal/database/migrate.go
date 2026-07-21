package database

import (
	"log/slog"
	"os"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"K2board/internal/config"
	"K2board/internal/models"
	"K2board/internal/utils"
)

func AutoMigrate() error {
	if err := DB.AutoMigrate(
		&models.User{}, &models.Node{}, &models.NodeToken{},
		&models.TrafficLog{}, &models.NodeOnline{}, &models.Setting{},
		&models.Group{}, &models.Plan{}, &models.AuditLog{},
		&models.StatServer{}, &models.StatUser{}, &models.NodeMetric{},
		&models.NodeGroupMapping{},
		&models.Order{}, &models.PaymentMethod{},
		&models.FreePlanClaim{},
		&models.CommissionLedger{}, &models.CommissionWithdraw{},
	); err != nil {
		return err
	}
	seedPaymentMethods()
	return nil
}

// seedPaymentMethods used to insert a lab "mock" row. Mock payment is permanently
// removed: disable and delete residual mock methods so they cannot be re-enabled
// for free fulfillment.
func seedPaymentMethods() {
	res := DB.Where("code = ?", "mock").Delete(&models.PaymentMethod{})
	if res.Error != nil {
		slog.Warn("purge mock payment method failed", "error", res.Error)
		// Fallback: force disable if delete fails (e.g. FK)
		_ = DB.Model(&models.PaymentMethod{}).Where("code = ?", "mock").Update("enable", false).Error
		return
	}
	if res.RowsAffected > 0 {
		slog.Info("removed residual mock payment method(s)", "count", res.RowsAffected)
	}
}

// MigrateNodeGroupMappings copies legacy node.group_id values into the
// node_group_mappings junction table. Idempotent per-node upsert:
// only inserts missing (node_id, group_id) pairs — never skips the whole table
// just because some mappings already exist.
func MigrateNodeGroupMappings() error {
	// Insert any nodes that still only have legacy group_id and no matching mapping
	// PostgreSQL / MySQL compatible via NOT EXISTS (no ON CONFLICT dialect issues).
	return DB.Exec(`
		INSERT INTO node_group_mappings (node_id, group_id)
		SELECT n.id, n.group_id
		FROM nodes n
		WHERE n.group_id > 0
		  AND NOT EXISTS (
			SELECT 1 FROM node_group_mappings m
			WHERE m.node_id = n.id AND m.group_id = n.group_id
		  )
	`).Error
}

func SeedDefaultAdmin(cfg *config.AdminConfig) error {
	// Ensure JWT secret exists
	if config.AppConfig.JWT.Secret == "" {
		config.AppConfig.JWT.Secret = mustGenerateToken(32)
	}
	// Always sync to .env if missing
	if _, err := os.Stat(".env"); os.IsNotExist(err) || !envHasKey(".env", "jwt.secret") {
		config.SaveSecret("jwt.secret", config.AppConfig.JWT.Secret)
	}

	// Ensure admin password exists
	password := cfg.Password
	if password == "" {
		password = mustGenerateToken(12)
	}
	if !envHasKey(".env", "admin.password") {
		config.SaveSecret("admin.password", password)
	}

	var count int64
	DB.Model(&models.User{}).Count(&count)
	if count > 0 {
		// Sync .env password to DB on reinstall or first upgrade
		currentHash := utils.SHA256(password)
		var s models.Setting
		err := DB.Where("key = ?", "admin_pass_sync").First(&s).Error
		needSync := err != nil || s.Value != currentHash
		if needSync {
			hashed, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
			DB.Model(&models.User{}).Where("is_admin = ?", true).Updates(map[string]any{
				"password": string(hashed),
			})
			if err != nil {
				DB.Create(&models.Setting{Key: "admin_pass_sync", Value: currentHash})
			} else {
				s.Value = currentHash
				DB.Save(&s)
			}
			slog.Info("admin password synced from .env")
		}
		seedDefaultSettings()
		return nil
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	admin := &models.User{
		Email:    strings.ToLower(strings.TrimSpace(cfg.Email)),
		Password: string(hashedPassword),
		UUID:     utils.GenerateUUID(),
		Token:    mustGenerateToken(16),
		IsAdmin:  true,
		Enable:   true,
	}
	if err := DB.Create(admin).Error; err != nil {
		return err
	}

	// Record synced password hash for reinstall detection
	DB.Create(&models.Setting{Key: "admin_pass_sync", Value: utils.SHA256(password)})

	// Generate panel token
	var pc int64
	DB.Model(&models.Setting{}).Where("key = ?", "panel_token").Count(&pc)
	if pc == 0 {
		plain, _ := utils.GenerateToken(16)
		DB.Create(&models.Setting{Key: "panel_token", Value: utils.SHA256(plain)})
		slog.Info("panel token generated (hash stored, plaintext below)", "token", plain)
	}

	seedDefaultSettings()
	return nil
}

// SeedDefaultSettings inserts missing system setting keys (idempotent).
func SeedDefaultSettings() { seedDefaultSettings() }

func seedDefaultSettings() {
	defaults := map[string]string{
		"allow_register": "false",
		"smtp_host":      "",
		"smtp_port":      "587",
		"smtp_user":      "",
		"smtp_pass":      "",
		"smtp_from":      "",
		"site_name":      "东京热云",
		"site_url":       "",
		"subscribe_url":  "",
		// Shadow user portals: newline-separated https://origin (CORS + payment return)
		"allowed_origins": "",
		// 邮件模板占位符: {{code}} {{site_name}}
		"mail_tpl_register_subject": "【{{site_name}}】邮箱验证码",
		"mail_tpl_register_body": "您的 {{site_name}} 验证码是: {{code}}\n\n" +
			"验证码 10 分钟内有效，请勿泄露给他人。\n\n" +
			"如果您没有注册 {{site_name}} 账号，请忽略此邮件。",
		"mail_tpl_reset_subject": "【{{site_name}}】重置密码验证码",
		"mail_tpl_reset_body": "您正在重置 {{site_name}} 账号密码。\n\n" +
			"验证码：{{code}}\n\n" +
			"验证码 10 分钟内有效，请勿泄露给他人。\n\n" +
			"如非本人操作，请忽略本邮件，并建议尽快登录检查账号安全。",
		// 推广返佣：默认开启、每笔 10%、最低提现 100 元（分）
		"referral_enable":         "true",
		"referral_rate":           "10",
		"referral_min_withdraw":   "10000",
		"referral_payout_methods": `[{"code":"alipay","name":"支付宝"},{"code":"wechat","name":"微信"},{"code":"usdt_trc20","name":"USDT-TRC20"},{"code":"bank","name":"银行卡"}]`,
	}
	for k, v := range defaults {
		var n int64
		DB.Model(&models.Setting{}).Where("key = ?", k).Count(&n)
		if n == 0 {
			DB.Create(&models.Setting{Key: k, Value: v})
		}
	}
}

func envHasKey(path, key string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return strings.Contains(string(data), key+"=")
}

func mustGenerateToken(n int) string {
	t, err := utils.GenerateToken(n)
	if err != nil {
		panic(err)
	}
	return t
}
