package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"

	k2board "K2board"
	"K2board/internal/config"
	"K2board/internal/database"
	"K2board/internal/queue"
	"K2board/internal/router"
	"K2board/internal/services"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	// Load configuration
	if err := config.Load("config.yml"); err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}
	slog.Info("configuration loaded")

	// Initialize database
	if err := database.Init(&config.AppConfig.Database); err != nil {
		slog.Error("failed to init database", "error", err)
		os.Exit(1)
	}

	// Run migrations
	if err := database.AutoMigrate(); err != nil {
		slog.Error("failed to auto migrate", "error", err)
		os.Exit(1)
	}

	// Migrate legacy node.group_id → node_group_mappings (many-to-many)
	if err := database.MigrateNodeGroupMappings(); err != nil {
		slog.Warn("node group mapping migration failed (non-fatal)", "error", err)
	}

	slog.Info("database migration completed")

	// Seed default admin
	if err := database.SeedDefaultAdmin(&config.AppConfig.Admin); err != nil {
		slog.Error("failed to seed admin", "error", err)
		os.Exit(1)
	}

	// Multi-instance config_version from DB
	services.InitConfigVersion()

	// Ensure registration/SMTP/referral setting keys exist
	database.SeedDefaultSettings()
	// Backfill invite codes for legacy users
	services.EnsureAllInviteCodes()

	// Initialize Redis traffic store if configured (optional, for production robustness)
	if config.AppConfig.Redis.Enabled {
		redisStore, err := queue.NewRedisStore(
			config.AppConfig.Redis.Addr,
			config.AppConfig.Redis.Password,
			config.AppConfig.Redis.DB,
		)
		if err != nil {
			slog.Warn("redis connection failed, using in-memory buffer", "error", err)
		} else {
			queue.DefaultStore = redisStore
			queue.SetStoreType("redis")
			// Shared INCR for multi-instance subscribe node fair-rotate
			services.IncrCounterFn = redisStore.Incr
			slog.Info("redis traffic store activated — crash-safe & multi-instance ready")
		}
	}

	// Start background scheduler (traffic buffer flush + stats aggregation)
	queue.StartScheduler()

	// Setup router
	r := router.Setup()

	// Set Gin mode
	ginMode := config.AppConfig.Server.Mode
	if ginMode == "release" {
		gin.SetMode(gin.ReleaseMode)
	}

	// Serve embedded frontend static files
	serveFrontend(r)

	addr := fmt.Sprintf("%s:%d", config.AppConfig.Server.Host, config.AppConfig.Server.Port)
	slog.Info("K2Board server starting", "addr", addr)

	if err := r.Run(addr); err != nil {
		slog.Error("server failed to start", "error", err)
		os.Exit(1)
	}
}

// serveFrontend serves the embedded Vue frontend.
// Falls back to index.html for SPA routing.
// Use Nginx to add a secret admin path prefix if needed (see deploy/nginx.conf).
func serveFrontend(r *gin.Engine) {
	frontendFS := k2board.FrontendFS()
	if frontendFS == nil {
		slog.Warn("frontend not embedded, running API-only mode")
		return
	}

	fileServer := http.FileServer(frontendFS)

	r.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path
		f, err := frontendFS.Open(path[1:])
		if err == nil {
			f.Close()
			fileServer.ServeHTTP(c.Writer, c.Request)
			return
		}
		// SPA fallback: serve index.html
		c.FileFromFS("/", frontendFS)
	})
}
