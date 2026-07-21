package router

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"K2board/internal/config"
	"K2board/internal/handlers/admin"
	"K2board/internal/handlers/client"
	paynotify "K2board/internal/handlers/payment"
	"K2board/internal/handlers/server"
	"K2board/internal/handlers/user"
	"K2board/internal/middleware"
)

func Setup() *gin.Engine {
	r := gin.New()
	// Trust local reverse proxies (nginx / docker bridge)
	_ = r.SetTrustedProxies([]string{"127.0.0.1", "::1", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"})
	r.Use(func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2<<20)
		c.Next()
	})
	r.Use(middleware.SecurityHeaders())
	r.Use(middleware.CORSMiddleware())
	r.Use(middleware.LoggerMiddleware())
	r.Use(gin.Recovery())

	// Handlers
	authHandler := admin.NewAuthHandler()
	userHandler := admin.NewUserHandler()
	nodeHandler := admin.NewNodeHandler()
	dashboardHandler := admin.NewDashboardHandler()
	settingHandler := admin.NewSettingHandler()
	uniproxyHandler := server.NewUniProxyHandler()
	subscribeHandler := client.NewSubscribeHandler()
	userAuthHandler := user.NewUserAuthHandler()

	// === Admin API (JWT + AdminOnly) ===
	adminGroup := r.Group("/api/v1/admin")
	{
		adminGroup.POST("/login", authHandler.Login)
		protected := adminGroup.Group("")
		protected.Use(middleware.JWTAuth(), middleware.AdminOnly())
		{
			protected.GET("/dashboard", dashboardHandler.GetStats)
			protected.GET("/dashboard/trend", dashboardHandler.Trend)

			protected.GET("/users", userHandler.List)
			protected.POST("/users", userHandler.Create)
			protected.GET("/users/:id", userHandler.Get)
			protected.PUT("/users/:id", userHandler.Update)
			protected.DELETE("/users/:id", userHandler.Delete)
			protected.POST("/users/:id/reset-uuid", userHandler.ResetUUID)
			protected.POST("/users/:id/reset-token", userHandler.ResetToken)
			protected.POST("/users/:id/reset-traffic", userHandler.ResetTraffic)
			protected.GET("/users/online", userHandler.OnlineUsers)
			protected.POST("/users/batch-delete", userHandler.BatchDelete)
			protected.POST("/users/batch-group", userHandler.BatchUpdateGroup)

			protected.GET("/nodes", nodeHandler.List)
			protected.POST("/nodes", nodeHandler.Create)
			protected.GET("/nodes/:id", nodeHandler.Get)
			protected.PUT("/nodes/:id", nodeHandler.Update)
			protected.DELETE("/nodes/:id", nodeHandler.Delete)
			protected.GET("/nodes/reality/generate", nodeHandler.GenerateRealityParams)
			protected.GET("/nodes/:id/metrics", nodeHandler.Metrics)
			protected.POST("/nodes/:id/token", nodeHandler.GenerateToken)
			protected.POST("/nodes/token/custom", nodeHandler.AddCustomToken)
			protected.DELETE("/nodes/token/:tokenId", nodeHandler.DeleteToken)

			protected.GET("/traffic-logs", admin.NewTrafficHandler().List)
			protected.GET("/traffic-stats", admin.NewTrafficHandler().Stats)

			groupHandler := admin.NewGroupHandler()
			protected.GET("/groups", groupHandler.List)
			protected.POST("/groups", groupHandler.Create)
			protected.PUT("/groups/:id", groupHandler.Update)
			protected.DELETE("/groups/:id", groupHandler.Delete)

			planHandler := admin.NewPlanHandler()
			protected.GET("/plans", planHandler.List)
			protected.POST("/plans", planHandler.Create)
			protected.PUT("/plans/:id", planHandler.Update)
			protected.DELETE("/plans/:id", planHandler.Delete)

			payHandler := admin.NewPaymentHandler()
			protected.GET("/payment-methods/gateways", payHandler.GatewayCodes)
			protected.GET("/payment-methods", payHandler.ListMethods)
			protected.POST("/payment-methods", payHandler.CreateMethod)
			protected.PUT("/payment-methods/:id", payHandler.UpdateMethod)
			protected.DELETE("/payment-methods/:id", payHandler.DeleteMethod)
			protected.GET("/orders", payHandler.ListOrders)
			protected.POST("/orders/:id/close", payHandler.CloseOrder)
			protected.POST("/orders/:id/mark-paid", payHandler.MarkPaid)
			protected.POST("/orders/:id/sync", payHandler.SyncOrder)

			auditHandler := admin.NewAuditHandler()
			protected.GET("/audit-logs", auditHandler.List)

			queueHandler := admin.NewQueueHandler()
			protected.GET("/queue/stats", queueHandler.Stats)

			protected.GET("/settings", settingHandler.GetAll)
			protected.PUT("/settings", settingHandler.UpdateAll)
			protected.POST("/settings/test-email", settingHandler.TestEmail)

			// Referral / commission
			refHandler := admin.NewReferralHandler()
			protected.GET("/referral/config", refHandler.Config)
			protected.GET("/referral/withdrawals", refHandler.ListWithdraws)
			protected.POST("/referral/withdrawals/:id/approve", refHandler.ApproveWithdraw)
			protected.POST("/referral/withdrawals/:id/reject", refHandler.RejectWithdraw)
			protected.GET("/referral/ledgers", refHandler.ListLedgers)
		}
	}

	// === Server API (XrayR4u — VLESS / AnyTLS) ===
	serverGroup := r.Group("/api/v1/server")
	serverGroup.Use(middleware.NodeAuth(config.AppConfig.Server.NodeRateLimit))
	{
		serverGroup.GET("/UniProxy/config", uniproxyHandler.GetConfig)
		serverGroup.GET("/UniProxy/user", uniproxyHandler.GetUser)
		serverGroup.POST("/UniProxy/push", uniproxyHandler.PushTraffic)
		serverGroup.POST("/UniProxy/alive", uniproxyHandler.AliveUsers)
		serverGroup.POST("/UniProxy/status", server.ReportNodeStatus)
		serverGroup.POST("/UniProxy/info", server.ReportNodeStatus)
		serverGroup.GET("/UniProxy/rule", server.GetNodeRule)
		serverGroup.POST("/UniProxy/illegal", server.ReportIllegal)
	}

	// === Client API ===
	clientGroup := r.Group("/api/v1/client")
	{
		clientGroup.GET("/subscribe", subscribeHandler.GetSubscription)
	}

	// === Payment provider callbacks (public, no JWT) ===
	notifyHandler := paynotify.NewNotifyHandler()
	payGroup := r.Group("/api/v1/payment")
	{
		// POST: alipay/epusdt/… ; GET: 易支付等常见异步通知
		payGroup.POST("/notify/:code", notifyHandler.Notify)
		payGroup.GET("/notify/:code", notifyHandler.Notify)
		payGroup.GET("/return", notifyHandler.Return)
		payGroup.POST("/return", notifyHandler.Return)
	}

	// === User API (public + token-based) ===
	userGroup := r.Group("/api/v1/user")
	{
		userGroup.POST("/send-code", userAuthHandler.SendCode)
		userGroup.POST("/forgot-password/send-code", userAuthHandler.SendResetCode)
		userGroup.POST("/reset-password", userAuthHandler.ResetPassword)
		userGroup.GET("/plans", user.GetPlans)
		userGroup.GET("/info", user.GetInfo)
		userGroup.POST("/change-password", user.ChangePassword)
		userGroup.POST("/register", userAuthHandler.Register)
		userGroup.POST("/login", userAuthHandler.Login)

		// Shop / orders (auth via body/query token) — IP rate limits against flood
		userGroup.GET("/payment-methods", user.ListPaymentMethods)
		userGroup.POST("/orders", middleware.UserOrderCreateIP.Middleware(), user.CreateOrder)
		userGroup.GET("/orders", middleware.UserOrderReadIP.Middleware(), user.ListOrders)
		userGroup.GET("/orders/:trade_no", middleware.UserOrderReadIP.Middleware(), user.GetOrder)
		userGroup.POST("/orders/:trade_no/checkout", middleware.UserOrderActionIP.Middleware(), user.Checkout)
		userGroup.POST("/orders/:trade_no/cancel", middleware.UserOrderActionIP.Middleware(), user.CancelOrder)
		userGroup.POST("/orders/:trade_no/sync", middleware.UserOrderActionIP.Middleware(), user.SyncOrder)
		// confirm-mock permanently removed (mock payment disabled for security)


		// Referral / commission (auth via token query/body)
		userGroup.GET("/referral", user.GetReferral)
		userGroup.GET("/referral/ledgers", user.ListReferralLedgers)
		userGroup.GET("/referral/withdrawals", user.ListReferralWithdraws)
		userGroup.GET("/referral/invitees", user.ListInvitees)
		userGroup.POST("/referral/withdraw", middleware.UserOrderActionIP.Middleware(), user.CreateWithdraw)
	}

	return r
}
