package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"

	"billflow/internal/config"
	"billflow/internal/database"
	"billflow/internal/handlers"
	"billflow/internal/jobs"
	"billflow/internal/middleware"
	"billflow/internal/models"
	"billflow/internal/repository"
	"billflow/internal/services/ai"
	"billflow/internal/services/anomaly"
	"billflow/internal/services/catalog"
	emailservice "billflow/internal/services/email"
	"billflow/internal/services/events"
	"billflow/internal/services/insight"
	lineservice "billflow/internal/services/line"
	"billflow/internal/services/mapper"
	"billflow/internal/services/media"
	"billflow/internal/services/mistral"
	"billflow/internal/services/artifact"
	"billflow/internal/services/sml"
	"billflow/internal/worker"
)

func main() {
	cfg := config.Load()

	// Logger
	var logger *zap.Logger
	var err error
	if cfg.Env == "production" {
		logger, err = zap.NewProduction()
	} else {
		logger, err = zap.NewDevelopment()
	}
	if err != nil {
		log.Fatal("init logger:", err)
	}
	defer logger.Sync()

	// Database
	db, err := database.Connect(cfg.DatabaseURL)
	if err != nil {
		logger.Fatal("database connect", zap.Error(err))
	}
	defer db.Close()

	seedAdminUser(db, logger)

	// On boot, fail any outgoing chat_messages stuck in 'pending' for >5 min.
	// These rows happen when the server crashes mid-send; without cleanup they
	// stay "กำลังส่ง…" in the UI forever. 5min covers slow LINE Push without
	// false positives for normal traffic (Reply/Push complete in ms).
	if res, err := db.Exec(
		`UPDATE chat_messages
		   SET delivery_status = 'failed',
		       delivery_error  = COALESCE(NULLIF(delivery_error, ''), 'server restart or send timeout')
		 WHERE direction = 'outgoing'
		   AND delivery_status = 'pending'
		   AND created_at < NOW() - INTERVAL '5 minutes'`,
	); err == nil {
		if n, _ := res.RowsAffected(); n > 0 {
			logger.Info("startup_pending_cleanup", zap.Int64("rows", n))
		}
	} else {
		logger.Warn("startup pending cleanup", zap.Error(err))
	}

	// Repositories
	userRepo := repository.NewUserRepo(db)
	billRepo := repository.NewBillRepo(db)
	mappingRepo := repository.NewMappingRepo(db)
	insightRepo := repository.NewInsightRepo(db)
	platformRepo := repository.NewPlatformMappingRepo(db)
	auditLogRepo := repository.NewAuditLogRepo(db)
	catalogRepo := repository.NewSMLCatalogRepo(db)
	artifactRepo := repository.NewBillArtifactRepo(db)
	channelDefaultRepo := repository.NewChannelDefaultRepo(db)
	docCounterRepo := repository.NewDocCounterRepo(db)
	chatConvRepo := repository.NewChatConversationRepo(db)
	chatMessageRepo := repository.NewChatMessageRepo(db)
	chatMediaRepo := repository.NewChatMediaRepo(db, cfg.ArtifactsDir, cfg.ArtifactsMaxBytes)
	chatQuickReplyRepo := repository.NewChatQuickReplyRepo(db)
	chatNoteRepo := repository.NewChatNoteRepo(db)
	chatTagRepo := repository.NewChatTagRepo(db)
	lineOARepo := repository.NewLineOAAccountRepo(db)

	// Services
	aiClient := ai.NewClient(cfg.OpenRouterAPIKey, cfg.OpenRouterModel, cfg.OpenRouterFallback, cfg.OpenRouterAudioModel)
	mapperSvc := mapper.New(mappingRepo)
	anomalySvc := anomaly.New(billRepo).WithCustomerLookup(billRepo)
	smlClient := sml.New(sml.Config{
		BaseURL: cfg.SMLBaseURL,
	})
	// MCPClient (sml.NewMCPClient) was used by the AI chatbot for product
	// inquiry — removed in session 13. The MCP client code itself is kept
	// in services/sml/mcp.go in case future flows need it.
	insightSvc := insight.New(aiClient)
	artifactSvc := artifact.New(cfg.ArtifactsDir, cfg.ArtifactsMaxBytes, artifactRepo, logger)
	pool := worker.New()

	// Shopee SML 248 REST clients — saleorder (default sale path), saleinvoice
	// (kept for admins who pin endpoint='saleinvoice' on a channel) + purchaseorder.
	// CustCode is filled at request time from channel_defaults — see handlers/bills.go.
	invoiceClient := sml.NewInvoiceClient(sml.InvoiceConfig{
		BaseURL:    cfg.ShopeeSMLURL,
		GUID:       cfg.ShopeeSMLGUID,
		Provider:   cfg.ShopeeSMLProvider,
		ConfigFile: cfg.ShopeeSMLConfigFile,
		Database:   cfg.ShopeeSMLDatabase,
		DocFormat:  cfg.ShopeeSMLDocFormat,
		SaleCode:   cfg.ShopeeSMLSaleCode,
		BranchCode: cfg.ShopeeSMLBranchCode,
		WHCode:     cfg.ShopeeSMLWHCode,
		ShelfCode:  cfg.ShopeeSMLShelfCode,
		UnitCode:   cfg.ShopeeSMLUnitCode,
		VATType:    cfg.ShopeeSMLVATType,
		VATRate:    cfg.ShopeeSMLVATRate,
		DocTime:    cfg.ShopeeSMLDocTime,
	}, logger)
	saleOrderClient := sml.NewSaleOrderClient(sml.SaleOrderConfig{
		BaseURL:    cfg.ShopeeSMLURL,
		GUID:       cfg.ShopeeSMLGUID,
		Provider:   cfg.ShopeeSMLProvider,
		ConfigFile: cfg.ShopeeSMLConfigFile,
		Database:   cfg.ShopeeSMLDatabase,
		DocFormat:  cfg.ShopeeSMLDocFormat,
		SaleCode:   cfg.ShopeeSMLSaleCode,
		BranchCode: cfg.ShopeeSMLBranchCode,
		WHCode:     cfg.ShopeeSMLWHCode,
		ShelfCode:  cfg.ShopeeSMLShelfCode,
		UnitCode:   cfg.ShopeeSMLUnitCode,
		VATType:    cfg.ShopeeSMLVATType,
		VATRate:    cfg.ShopeeSMLVATRate,
		DocTime:    cfg.ShopeeSMLDocTime,
	}, logger)
	productClient := sml.NewProductClient(
		cfg.ShopeeSMLURL,
		cfg.ShopeeSMLGUID,
		cfg.ShopeeSMLProvider,
		cfg.ShopeeSMLConfigFile,
		cfg.ShopeeSMLDatabase,
		logger,
	)
	poClient := sml.NewPurchaseOrderClient(sml.PurchaseOrderConfig{
		BaseURL:    cfg.ShopeeSMLURL,
		GUID:       cfg.ShopeeSMLGUID,
		Provider:   cfg.ShopeeSMLProvider,
		ConfigFile: cfg.ShopeeSMLConfigFile,
		Database:   cfg.ShopeeSMLDatabase,
		DocFormat:  cfg.ShippedSMLDocFormat,
		SaleCode:   cfg.ShopeeSMLSaleCode,
		BranchCode: cfg.ShopeeSMLBranchCode,
		WHCode:     cfg.ShopeeSMLWHCode,
		ShelfCode:  cfg.ShopeeSMLShelfCode,
		UnitCode:   cfg.ShopeeSMLUnitCode,
		VATType:    cfg.ShopeeSMLVATType,
		VATRate:    cfg.ShopeeSMLVATRate,
		DocTime:    cfg.ShopeeSMLDocTime,
	}, logger)

	// SML party cache — fetches all customer + supplier records from SML 248
	// at boot, refreshes every 6 h. Powers the /settings/channels picker.
	partyClient := sml.NewPartyClient(sml.PartyConfig{
		BaseURL:    cfg.ShopeeSMLURL,
		GUID:       cfg.ShopeeSMLGUID,
		Provider:   cfg.ShopeeSMLProvider,
		ConfigFile: cfg.ShopeeSMLConfigFile,
		Database:   cfg.ShopeeSMLDatabase,
	}, logger)
	partyCache := sml.NewPartyCache(partyClient, logger)
	partyCache.Start(context.Background())

	// SML catalog services for Shopee email smart matching
	smlHeaders := map[string]string{
		"guid":           cfg.ShopeeSMLGUID,
		"provider":       cfg.ShopeeSMLProvider,
		"configFileName": cfg.ShopeeSMLConfigFile,
		"databaseName":   cfg.ShopeeSMLDatabase,
	}
	catalogSvc := catalog.NewSMLCatalogService(catalogRepo, cfg.ShopeeSMLURL, smlHeaders, logger)
	embSvc := catalog.NewEmbeddingService(cfg.OpenRouterAPIKey)
	catalogIdx := catalog.NewCatalogIndex()
	// Load existing embeddings into memory at startup
	if err := catalogIdx.Reload(catalogRepo); err != nil {
		logger.Warn("catalog: reload index at startup", zap.Error(err))
	} else {
		logger.Info("catalog: index loaded", zap.Int("size", catalogIdx.Size()))
	}

	// LINE service (legacy single instance) — kept for PushAdmin paths used by
	// insight cron, disk monitor, and email coordinator error notifications.
	// The chat inbox uses lineRegistry instead so each conversation routes to
	// the right OA's access_token.
	var lineSvc *lineservice.Service
	if cfg.LineChannelSecret != "" && cfg.LineChannelAccessToken != "" {
		lineSvc, err = lineservice.New(cfg.LineChannelSecret, cfg.LineChannelAccessToken, cfg.LineAdminUserID)
		if err != nil {
			logger.Warn("LINE service init failed", zap.Error(err))
		}
	}

	// Multi-OA registry. Seeds a default OA from LINE_* env vars on first boot
	// (when line_oa_accounts is empty) so existing single-OA installs keep
	// working without admin intervention.
	lineRegistry := lineservice.NewRegistry(lineOARepo, logger)
	if empty, _ := lineOARepo.IsEmpty(); empty {
		if cfg.LineChannelSecret != "" && cfg.LineChannelAccessToken != "" {
			seed := &models.LineOAAccount{
				Name:               "Default (from .env)",
				ChannelSecret:      cfg.LineChannelSecret,
				ChannelAccessToken: cfg.LineChannelAccessToken,
				AdminUserID:        cfg.LineAdminUserID,
				Greeting:           cfg.LineGreeting,
				Enabled:            true,
			}
			if err := lineOARepo.Create(seed); err != nil {
				logger.Warn("seed default LINE OA failed", zap.Error(err))
			} else {
				logger.Info("seeded default LINE OA from env",
					zap.String("oa_id", seed.ID),
					zap.String("name", seed.Name))
			}
		}
	}
	if err := lineRegistry.Reload(); err != nil {
		logger.Warn("LINE OA registry initial reload failed", zap.Error(err))
	}

	// Email service — multi-account coordinator. Reads imap_accounts table
	// at boot, spawns one poller goroutine per enabled row. Admin edits
	// flow back through ReloadAccount/RemoveAccount via the settings API.
	imapAccountRepo := repository.NewImapAccountRepo(db)
	imapProcessors := &emailservice.Processors{
		Attachment:    nil, // wired below once emailH is built
		ShopeeOrder:   nil,
		ShopeeShipped: nil,
	}
	imapCoordinator := emailservice.NewCoordinator(imapAccountRepo, imapProcessors, lineSvc, logger)

	// Mistral OCR service (optional — used for PDF extraction)
	ocrClient := mistral.New(cfg.MistralAPIKey)

	// JWT
	middleware.SetJWTSecret(cfg.JWTSecret)

	// Gin
	if cfg.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.Logger(logger))

	// CORS
	r.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Authorization,Content-Type")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	})

	// Health
	r.GET("/health", func(c *gin.Context) {
		dbStatus := "ok"
		if err := db.PingContext(c.Request.Context()); err != nil {
			dbStatus = "error: " + err.Error()
		}
		status := "ok"
		if dbStatus != "ok" {
			status = "degraded"
		}
		c.JSON(http.StatusOK, gin.H{
			"status":   status,
			"env":      cfg.Env,
			"database": dbStatus,
		})
	})

	// Handlers
	authH := handlers.NewAuthHandler(userRepo, cfg.JWTExpireHours, logger)
	billH := handlers.NewBillHandler(billRepo, mapperSvc, smlClient, invoiceClient, saleOrderClient, poClient, cfg, lineSvc, auditLogRepo, catalogRepo, channelDefaultRepo, docCounterRepo, artifactSvc, logger)
	mappingH := handlers.NewMappingHandler(mappingRepo, mapperSvc, logger)
	dashH := handlers.NewDashboardHandler(billRepo, insightRepo, chatConvRepo, insightSvc, logger)
	imapConfigured := false
	if accs, err := imapAccountRepo.ListEnabled(); err == nil && len(accs) > 0 {
		imapConfigured = true
	}
	dashH.SetConfigStatus(
		cfg.LineChannelSecret != "" && cfg.LineChannelAccessToken != "",
		imapConfigured,
		cfg.SMLBaseURL != "",
		cfg.OpenRouterAPIKey != "",
		cfg.AutoConfirmThreshold,
	)
	// Media signer for /public/media/:id?t=<token>. Falls back to JWT_SECRET
	// when MEDIA_SIGNING_KEY is empty so single-secret deployments work.
	mediaKey := cfg.MediaSigningKey
	if mediaKey == "" {
		mediaKey = cfg.JWTSecret
	}
	mediaSigner := media.NewSigner(mediaKey)

	// In-process pub/sub for SSE — webhook handlers + admin actions Publish,
	// /api/admin/events subscribers stream events to admin browsers.
	eventBroker := events.NewBroker()

	lineH := handlers.NewLineHandler(lineRegistry, chatConvRepo, chatMessageRepo, chatMediaRepo, auditLogRepo, pool, cfg, eventBroker, logger)
	chatInboxH := handlers.NewChatInboxHandler(chatConvRepo, chatMessageRepo, chatMediaRepo, billRepo, auditLogRepo, lineRegistry, aiClient, ocrClient, mediaSigner, eventBroker, cfg.PublicBaseURL, logger)
	publicMediaH := handlers.NewPublicMediaHandler(chatMediaRepo, mediaSigner, logger)
	sseH := handlers.NewSSEHandler(eventBroker, mediaSigner)
	emailH := handlers.NewEmailHandler(aiClient, ocrClient, mapperSvc, anomalySvc, smlClient, billRepo, auditLogRepo, lineSvc, cfg.AutoConfirmThreshold, logger)
	emailH.SetCatalogServices(catalogSvc, embSvc, catalogIdx, catalogRepo)
	emailH.SetArtifactService(artifactSvc)
	catalogH := handlers.NewCatalogHandler(catalogSvc, embSvc, catalogIdx, catalogRepo, productClient, auditLogRepo, cfg.AutoConfirmThreshold, logger)
	importH := handlers.NewImportHandler(platformRepo, mapperSvc, anomalySvc, smlClient, billRepo, channelDefaultRepo, cfg.AutoConfirmThreshold, logger)
	shopeeH := handlers.NewShopeeImportHandler(billRepo, auditLogRepo, cfg, channelDefaultRepo, catalogSvc, embSvc, catalogIdx, logger)
	shopeeH.SetArtifactService(artifactSvc)
	settingsH := handlers.NewSettingsHandler(platformRepo, logger)
	imapSettingsH := handlers.NewIMAPSettingsHandler(imapAccountRepo, imapCoordinator, logger)
	channelDefaultsH := handlers.NewChannelDefaultsHandler(channelDefaultRepo, auditLogRepo, partyCache, logger)
	smlPartyH := handlers.NewSMLPartyHandler(partyCache, logger)
	logH := handlers.NewLogHandler(auditLogRepo, logger)

	// Webhooks (no auth)
	// Webhook routes:
	//   /webhook/line/:oaId  → multi-OA URL (admin pastes from /settings/line-oa)
	//   /webhook/line        → legacy single-OA fallback (resolves via Destination → Any())
	r.POST("/webhook/line/:oaId", lineH.Webhook)
	r.POST("/webhook/line", lineH.Webhook)

	// Public media endpoint — NO JWT, the HMAC token IS the auth.
	// LINE servers fetch this URL to deliver image messages to customers.
	r.GET("/public/media/:mediaID", publicMediaH.Serve)

	// SSE stream — NO JWT, the ?t=<token> query param IS the auth (since
	// EventSource doesn't support custom headers). Admin first calls
	// POST /api/admin/events/token (JWT-authenticated, see below) to get a
	// short-lived signed token, then opens EventSource with ?u=<userID>&t=<token>.
	r.GET("/api/admin/events", sseH.Stream)

	// Auth (rate-limited: 10 req/min per IP)
	r.POST("/api/auth/login", middleware.AuthRateLimit(10, time.Minute), authH.Login)

	// Protected routes
	api := r.Group("/api", middleware.Auth())
	{
		api.GET("/auth/me", authH.Me)

		// SSE token issuer — admin POSTs to get a short-lived HMAC token that
		// EventSource uses as ?t=<token> on /api/admin/events. JWT-protected.
		api.POST("/admin/events/token", sseH.IssueToken)

		// Bills
		api.GET("/bills", billH.List)
		api.GET("/bills/:id", billH.Get)
		api.POST("/bills/:id/retry", billH.Retry)
		api.PUT("/bills/:id/items/:item_id", middleware.RequireRole("admin", "staff"), billH.UpdateItem)
		api.POST("/bills/:id/items", middleware.RequireRole("admin", "staff"), billH.AddItem)
		api.DELETE("/bills/:id/items/:item_id", middleware.RequireRole("admin", "staff"), billH.DeleteItemRow)
		api.GET("/bills/:id/artifacts", billH.ListArtifacts)
		api.GET("/bills/:id/artifacts/:artifact_id/download", billH.DownloadArtifact)
		api.GET("/bills/:id/artifacts/:artifact_id/preview", billH.PreviewArtifact)

		// Mappings
		api.GET("/mappings", mappingH.List)
		api.POST("/mappings", middleware.RequireRole("admin", "staff"), mappingH.Create)
		api.PUT("/mappings/:id", middleware.RequireRole("admin", "staff"), mappingH.Update)
		api.DELETE("/mappings/:id", middleware.RequireRole("admin"), mappingH.Delete)
		api.GET("/mappings/stats", mappingH.Stats)
		api.POST("/mappings/feedback", middleware.RequireRole("admin", "staff"), mappingH.Feedback)

		// Dashboard
		api.GET("/dashboard/stats", dashH.Stats)
		api.GET("/dashboard/insights", dashH.Insights)
		api.POST("/dashboard/insights/generate", middleware.RequireRole("admin"), dashH.GenerateInsight)

		// Settings
		api.GET("/settings/status", dashH.SettingsStatus)

		// Logs (Activity Log)
		api.GET("/logs", logH.List)

		// Import — existing (JSON-RPC sale_reserve)
		api.POST("/import/upload", middleware.RequireRole("admin", "staff"), importH.Upload)
		api.POST("/import/confirm", middleware.RequireRole("admin", "staff"), importH.Confirm)

		// Shopee import — saleinvoice REST API (SML 224)
		api.GET("/settings/shopee-config", shopeeH.GetConfig)
		api.POST("/import/shopee/preview", middleware.RequireRole("admin", "staff"), shopeeH.Preview)
		api.POST("/import/shopee/confirm", middleware.RequireRole("admin", "staff"), shopeeH.Confirm)

		// Platform column mappings
		api.GET("/settings/column-mappings/:platform", settingsH.GetColumnMappings)
		api.PUT("/settings/column-mappings/:platform", middleware.RequireRole("admin"), settingsH.UpdateColumnMappings)

		// Channel defaults (admin only) — per-(channel, bill_type) party config
		api.GET("/settings/channel-defaults", middleware.RequireRole("admin"), channelDefaultsH.List)
		api.PUT("/settings/channel-defaults", middleware.RequireRole("admin"), channelDefaultsH.Upsert)
		api.DELETE("/settings/channel-defaults/:channel/:bill_type", middleware.RequireRole("admin"), channelDefaultsH.Delete)
		api.POST("/settings/channel-defaults/quick-setup", middleware.RequireRole("admin"), channelDefaultsH.QuickSetup)

		// SML party master proxy — search customers/suppliers from cache
		api.GET("/sml/customers", middleware.RequireRole("admin", "staff"), smlPartyH.SearchCustomers)
		api.GET("/sml/suppliers", middleware.RequireRole("admin", "staff"), smlPartyH.SearchSuppliers)
		api.POST("/sml/refresh-parties", middleware.RequireRole("admin"), smlPartyH.Refresh)
		api.GET("/sml/parties/last-sync", middleware.RequireRole("admin", "staff"), smlPartyH.LastSync)

		// IMAP accounts (admin only) — multi-mailbox config
		api.GET("/settings/imap-accounts", middleware.RequireRole("admin"), imapSettingsH.List)
		api.POST("/settings/imap-accounts", middleware.RequireRole("admin"), imapSettingsH.Create)
		api.POST("/settings/imap-accounts/test", middleware.RequireRole("admin"), imapSettingsH.TestConnection)
		api.POST("/settings/imap-accounts/list-folders", middleware.RequireRole("admin"), imapSettingsH.ListFolders)
		api.GET("/settings/imap-accounts/:id", middleware.RequireRole("admin"), imapSettingsH.Get)
		api.PUT("/settings/imap-accounts/:id", middleware.RequireRole("admin"), imapSettingsH.Update)
		api.DELETE("/settings/imap-accounts/:id", middleware.RequireRole("admin"), imapSettingsH.Delete)
		api.POST("/settings/imap-accounts/:id/poll", middleware.RequireRole("admin"), imapSettingsH.PollNow)

		// Catalog (SML product catalog + smart matching)
		api.GET("/catalog", catalogH.List)
		api.GET("/catalog/stats", catalogH.Stats)
		api.GET("/catalog/search", catalogH.Search)
		api.GET("/catalog/:code", catalogH.GetOne)
		api.POST("/catalog/products", middleware.RequireRole("admin", "staff"), catalogH.CreateProduct)
		api.POST("/catalog/sync", middleware.RequireRole("admin"), catalogH.SyncFromAPI)
		api.POST("/catalog/import-csv", middleware.RequireRole("admin"), catalogH.ImportCSV)
		api.POST("/catalog/embed-all", middleware.RequireRole("admin"), catalogH.EmbedAll)
		api.POST("/catalog/reload-index", middleware.RequireRole("admin"), catalogH.ReloadIndex)
		api.POST("/catalog/:code/embed", middleware.RequireRole("admin"), catalogH.EmbedOne)
		api.POST("/catalog/:code/refresh", middleware.RequireRole("admin"), catalogH.RefreshOne)
		api.DELETE("/catalog/:code", middleware.RequireRole("admin"), catalogH.DeleteOne)

		// Confirm catalog match for a needs_review bill item
		api.POST("/bills/:id/items/:item_id/confirm-match", middleware.RequireRole("admin", "staff"), catalogH.ConfirmMatch)

		// Chat inbox (LINE OA human-to-human conversations)
		chatGroup := api.Group("/admin/conversations")
		chatGroup.Use(middleware.RequireRole("admin", "staff"))
		{
			chatGroup.GET("", chatInboxH.ListConversations)
			chatGroup.GET("/unread-count", chatInboxH.UnreadCount)
			chatGroup.GET("/:lineUserId/messages", chatInboxH.ListMessages)
			chatGroup.POST("/:lineUserId/messages", chatInboxH.SendReply)
			chatGroup.POST("/:lineUserId/messages/media", chatInboxH.SendMedia)
			chatGroup.POST("/:lineUserId/mark-read", chatInboxH.MarkRead)
			chatGroup.PATCH("/:lineUserId/status", chatInboxH.SetStatus)
			chatGroup.PATCH("/:lineUserId/phone", chatInboxH.SetPhone)

			// Phase 4.8 internal notes (admin-only annotations).
			chatNotesH := handlers.NewChatNotesHandler(chatNoteRepo, auditLogRepo)
			chatGroup.GET("/:lineUserId/notes", chatNotesH.List)
			chatGroup.POST("/:lineUserId/notes", chatNotesH.Create)
			chatGroup.PUT("/:lineUserId/notes/:noteId", chatNotesH.Update)
			chatGroup.DELETE("/:lineUserId/notes/:noteId", chatNotesH.Delete)

			// Phase 4.9 tags — m2m attach for a single conversation.
			chatTagsH := handlers.NewChatTagsHandler(chatTagRepo, auditLogRepo, eventBroker)
			chatGroup.GET("/:lineUserId/tags", chatTagsH.TagsForConversation)
			chatGroup.PUT("/:lineUserId/tags", chatTagsH.SetTagsForConversation)
			chatGroup.GET("/:lineUserId/messages/:messageId/media", chatInboxH.DownloadMedia)
			chatGroup.POST("/:lineUserId/messages/:messageId/extract", chatInboxH.ExtractFromMedia)
			chatGroup.POST("/:lineUserId/bills", chatInboxH.CreateBill)
			chatGroup.GET("/:lineUserId/history", chatInboxH.CustomerHistory)
		}

		// LINE OA accounts (admin-only) — /settings/line-oa CRUD + test button
		lineOAH := handlers.NewLineOAHandler(lineOARepo, lineRegistry, auditLogRepo, logger)
		lineOAGroup := api.Group("/settings/line-oa")
		lineOAGroup.Use(middleware.RequireRole("admin"))
		{
			lineOAGroup.GET("", lineOAH.List)
			lineOAGroup.GET("/:id", lineOAH.Get)
			lineOAGroup.POST("", lineOAH.Create)
			lineOAGroup.PUT("/:id", lineOAH.Update)
			lineOAGroup.DELETE("/:id", lineOAH.Delete)
			lineOAGroup.POST("/:id/test", lineOAH.Test)
		}

		// Quick reply templates for the chat composer (Phase 4.4)
		quickReplyH := handlers.NewChatQuickReplyHandler(chatQuickReplyRepo, auditLogRepo)
		qrGroup := api.Group("/admin/quick-replies")
		qrGroup.Use(middleware.RequireRole("admin", "staff"))
		{
			qrGroup.GET("", quickReplyH.List)
			qrGroup.POST("", middleware.RequireRole("admin"), quickReplyH.Create)
			qrGroup.PUT("/:id", middleware.RequireRole("admin"), quickReplyH.Update)
			qrGroup.DELETE("/:id", middleware.RequireRole("admin"), quickReplyH.Delete)
		}

		// Phase 4.9 — global chat tag CRUD. /settings/chat-tags admin page.
		tagsAdminH := handlers.NewChatTagsHandler(chatTagRepo, auditLogRepo, eventBroker)
		tagsGroup := api.Group("/settings/chat-tags")
		tagsGroup.Use(middleware.RequireRole("admin", "staff"))
		{
			tagsGroup.GET("", tagsAdminH.ListAll)
			tagsGroup.POST("", middleware.RequireRole("admin"), tagsAdminH.Create)
			tagsGroup.PUT("/:id", middleware.RequireRole("admin"), tagsAdminH.Update)
			tagsGroup.DELETE("/:id", middleware.RequireRole("admin"), tagsAdminH.Delete)
		}
	}

	// Background jobs
	c := cron.New()
	insightCron := jobs.NewInsightCron(insightSvc, billRepo, insightRepo, lineSvc, cfg.InsightLineNotify, logger)
	insightCron.Register(c, cfg.InsightCronHour)

	// Backup cron runs pg_dump from inside the backend container against the
	// postgres service on the Docker network. Output goes to /app/backups
	// (mounted to ~/billflow/backups on the host via docker-compose.yml).
	backupCron := jobs.NewBackupCron(
		"postgres", "5432",
		cfg.DBUser, "billflow", cfg.DBPassword,
		"/app/backups", logger,
	)
	backupCron.Register(c, cfg.BackupCronHour)

	diskMon := jobs.NewDiskMonitor(cfg.DiskWarnPercent, lineSvc, logger)
	diskMon.Register(c)

	if lineSvc != nil {
		tokenChecker := jobs.NewTokenChecker(lineSvc, logger)
		tokenChecker.Register(c)
	}

	// Hourly: clear replyTokens > 1h old so admin replies don't waste a
	// LINE round-trip on a token we know is dead.
	replyTokenCleanup := jobs.NewReplyTokenCleanup(db, logger)
	replyTokenCleanup.Register(c)

	// Wire processors into the coordinator now that emailH is built, then
	// boot the multi-account poller. Coordinator reads imap_accounts and
	// spawns one goroutine per enabled row. Empty list = no polling, no
	// errors — admin needs to add accounts via /settings/email.
	imapProcessors.Attachment = emailH.ProcessAttachment
	imapProcessors.ShopeeOrder = emailH.ProcessShopeeEmailBody
	imapProcessors.ShopeeShipped = emailH.ProcessShopeeShippedEmailBody

	if err := imapCoordinator.Start(context.Background()); err != nil {
		logger.Error("imap coordinator start failed", zap.Error(err))
	}
	defer imapCoordinator.Stop()

	c.Start()
	defer c.Stop()

	// HTTP server
	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		logger.Info("server starting", zap.String("port", cfg.Port))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("server error", zap.Error(err))
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("server shutdown error", zap.Error(err))
	}
	logger.Info("server stopped")
}

// seedAdminUser creates a default admin if no users exist
func seedAdminUser(db *sql.DB, logger *zap.Logger) {
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM users").Scan(&count); err != nil {
		logger.Error("seed: count users", zap.Error(err))
		return
	}
	if count > 0 {
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte("admin1234"), bcrypt.DefaultCost)
	if err != nil {
		logger.Error("seed: bcrypt", zap.Error(err))
		return
	}

	_, err = db.Exec(
		`INSERT INTO users (email, name, role, password_hash) VALUES ($1, $2, $3, $4)`,
		"admin@billflow.local", "Admin", "admin", string(hash),
	)
	if err != nil {
		logger.Error("seed: insert admin", zap.Error(err))
		return
	}
	logger.Info("seeded default admin: admin@billflow.local / admin1234")
}
