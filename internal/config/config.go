package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	Server    ServerConfig    `mapstructure:"server"`
	Database  DatabaseConfig  `mapstructure:"database"`
	Redis     RedisConfig     `mapstructure:"redis"`
	Scheduler SchedulerConfig `mapstructure:"scheduler"`
	JWT       JWTConfig       `mapstructure:"jwt"`
	Admin     AdminConfig     `mapstructure:"admin"`
}

type ServerConfig struct {
	Host          string `mapstructure:"host"`
	Port          int    `mapstructure:"port"`
	Mode          string `mapstructure:"mode"`
	NodeRateLimit int    `mapstructure:"node_rate_limit"`
}

type DatabaseConfig struct {
	Driver string `mapstructure:"driver"`
	DSN    string `mapstructure:"dsn"`
}

type JWTConfig struct {
	Secret      string `mapstructure:"secret"`
	ExpireHours int    `mapstructure:"expire_hours"`
}

type RedisConfig struct {
	Enabled  bool   `mapstructure:"enabled"`
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

type SchedulerConfig struct {
	FlushInterval       int `mapstructure:"flush_interval"`
	StatsInterval       int `mapstructure:"stats_interval"`
	// AutoDisableInterval is the scheduler tick for account maintenance + stale online purge.
	// Product model: enable is admin ban only; this tick no longer sets enable=false on expiry.
	AutoDisableInterval int `mapstructure:"auto_disable_interval"` // seconds
}

type AdminConfig struct {
	Email    string `mapstructure:"email"`
	Password string `mapstructure:"password"`
}

var AppConfig *Config

func Load(path string) error {
	viper.SetConfigFile(path)
	viper.SetConfigType("yaml")

	viper.SetDefault("server.host", "0.0.0.0")
	viper.SetDefault("server.port", 8080)
	viper.SetDefault("server.mode", "release")
	viper.SetDefault("server.node_rate_limit", 50)
	viper.SetDefault("database.driver", "postgres")
	viper.SetDefault("database.dsn", "host=localhost user=k2board password=k2board dbname=k2board port=5432 sslmode=disable TimeZone=Asia/Shanghai")
	viper.SetDefault("scheduler.flush_interval", 60)
	viper.SetDefault("scheduler.stats_interval", 300)
	viper.SetDefault("scheduler.auto_disable_interval", 60)
	viper.SetDefault("jwt.expire_hours", 24)

	if err := viper.ReadInConfig(); err != nil {
		return fmt.Errorf("failed to read config: %w", err)
	}

	// Override with .env file if present (secrets stored separately)
	loadDotEnv()

	cfg := &Config{}
	if err := viper.Unmarshal(cfg); err != nil {
		return fmt.Errorf("failed to unmarshal config: %w", err)
	}

	AppConfig = cfg
	return nil
}

// loadDotEnv reads .env file and overrides Viper values.
func loadDotEnv() {
	data, err := os.ReadFile(".env")
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			viper.Set(strings.TrimSpace(parts[0]), strings.Trim(strings.TrimSpace(parts[1]), `"`))
		}
	}
}

// SaveSecret writes a key=value to .env file. Creates file if not exists.
func SaveSecret(key, value string) {
	data, _ := os.ReadFile(".env")
	lines := strings.Split(string(data), "\n")
	found := false
	prefix := key + "="
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), prefix) {
			lines[i] = fmt.Sprintf("%s=%s", key, value)
			found = true
			break
		}
	}
	if !found {
		if len(lines) > 0 && lines[len(lines)-1] != "" {
			lines = append(lines, "")
		}
		lines = append(lines, fmt.Sprintf("%s=%s", key, value))
	}
	os.WriteFile(".env", []byte(strings.Join(lines, "\n")+"\n"), 0600)
}
