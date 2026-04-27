package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
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
	"billflow/internal/repository"
	"billflow/internal/services/ai"
	"billflow/internal/services/anomaly"
	"billflow/internal/services/catalog"
	emailservice "billflow/internal/services/email"
	"billflow/internal/services/insight"
	lineservice "billflow/internal/services/line"
	"billflow/internal/services/mapper"
	"billflow/internal/services/mistral"
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

	// Repositories
	userRepo := repository.NewUserRepo(db)
	billRepo := repository.NewBillRepo(db)
	mappingRepo := repository.NewMappingRepo(db)
	insightRepo := repository.NewInsightRepo(db)
	platformRepo := repository.NewPlatformMappingRepo(db)
	auditLogRepo := repository.NewAuditLogRepo(db)
	chatRepo := repository.NewChatSessionRepo(db)
	catalogRepo := repository.NewSMLCatalogRepo(db)

	// Services
	aiClient := ai.NewClient(cfg.OpenRouterAPIKey, cfg.OpenRouterModel, cfg.OpenRouterFallback, cfg.OpenRouterAudioModel)
	mapperSvc := mapper.New(mappingRepo)
	anomalySvc := anomaly.New(billRepo).WithCustomerLookup(billRepo)
	smlClient := sml.New(sml.Config{
		BaseURL: cfg.SMLBaseURL,
	})
	mcpClient := sml.NewMCPClient(cfg.SMLBaseURL)
	insightSvc := insight.New(aiClient)
	pool := worker.New()

	// Shopee SML 248 REST clients — saleinvoice (existing flow) + purchaseorder (shipped emails)
	invoiceClient := sml.NewInvoiceClient(sml.InvoiceConfig{
		BaseURL:    cfg.ShopeeSMLURL,
		GUID:       cfg.ShopeeSMLGUID,
		Provider:   cfg.ShopeeSMLProvider,
		ConfigFile: cfg.ShopeeSMLConfigFile,
		Database:   cfg.ShopeeSMLDatabase,
		DocFormat:  cfg.ShopeeSMLDocFormat,
		CustCode:   cfg.ShopeeSMLCustCode,
		SaleCode:   cfg.ShopeeSMLSaleCode,
		BranchCode: cfg.ShopeeSMLBranchCode,
		WHCode:     cfg.ShopeeSMLWHCode,
		ShelfCode:  cfg.ShopeeSMLShelfCode,
		UnitCode:   cfg.ShopeeSMLUnitCode,
		VATType:    cfg.ShopeeSMLVATType,
		VATRate:    cfg.ShopeeSMLVATRate,
		DocTime:    cfg.ShopeeSMLDocTime,
	}, logger)
	shippedCustCode := cfg.ShippedSMLCustCode
	if shippedCustCode == "" {
		shippedCustCode = cfg.ShopeeSMLCustCode
	}
	poClient := sml.NewPurchaseOrderClient(sml.PurchaseOrderConfig{
		BaseURL:    cfg.ShopeeSMLURL,
		GUID:       cfg.ShopeeSMLGUID,
		Provider:   cfg.ShopeeSMLProvider,
		ConfigFile: cfg.ShopeeSMLConfigFile,
		Database:   cfg.ShopeeSMLDatabase,
		DocFormat:  cfg.ShippedSMLDocFormat,
		CustCode:   shippedCustCode,
		SaleCode:   cfg.ShopeeSMLSaleCode,
		BranchCode: cfg.ShopeeSMLBranchCode,
		WHCode:     cfg.ShopeeSMLWHCode,
		ShelfCode:  cfg.ShopeeSMLShelfCode,
		UnitCode:   cfg.ShopeeSMLUnitCode,
		VATType:    cfg.ShopeeSMLVATType,
		VATRate:    cfg.ShopeeSMLVATRate,
		DocTime:    cfg.ShopeeSMLDocTime,
	}, logger)

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

	// LINE service (optional — skip if not configured)
	var lineSvc *lineservice.Service
	if cfg.LineChannelSecret != "" && cfg.LineChannelAccessToken != "" {
		lineSvc, err = lineservice.New(cfg.LineChannelSecret, cfg.LineChannelAccessToken, cfg.LineAdminUserID)
		if err != nil {
			logger.Warn("LINE service init failed", zap.Error(err))
		}
	}

	// Email service
	imapSvc := emailservice.New(
		cfg.IMAPHost, cfg.IMAPPort, cfg.IMAPUser, cfg.IMAPPassword,
		cfg.IMAPFilterFrom, cfg.IMAPFilterSubject, logger,
	)

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
	billH := handlers.NewBillHandler(billRepo, mapperSvc, smlClient, invoiceClient, poClient, cfg, lineSvc, auditLogRepo, logger)
	mappingH := handlers.NewMappingHandler(mappingRepo, mapperSvc, logger)
	dashH := handlers.NewDashboardHandler(billRepo, insightRepo, insightSvc, logger)
	dashH.SetConfigStatus(
		cfg.LineChannelSecret != "" && cfg.LineChannelAccessToken != "",
		cfg.IMAPHost != "",
		cfg.SMLBaseURL != "",
		cfg.OpenRouterAPIKey != "",
		cfg.AutoConfirmThreshold,
	)
	lineH := handlers.NewLineHandler(lineSvc, aiClient, ocrClient, mapperSvc, anomalySvc, smlClient, mcpClient, billRepo, auditLogRepo, chatRepo, pool, cfg.AutoConfirmThreshold, logger)
	emailH := handlers.NewEmailHandler(aiClient, ocrClient, mapperSvc, anomalySvc, smlClient, billRepo, auditLogRepo, lineSvc, cfg.AutoConfirmThreshold, logger)
	emailH.SetCatalogServices(catalogSvc, embSvc, catalogIdx, catalogRepo)
	catalogH := handlers.NewCatalogHandler(catalogSvc, embSvc, catalogIdx, catalogRepo, cfg.AutoConfirmThreshold, logger)
	importH := handlers.NewImportHandler(platformRepo, mapperSvc, anomalySvc, smlClient, billRepo, cfg.AutoConfirmThreshold, logger)
	shopeeH := handlers.NewShopeeImportHandler(billRepo, auditLogRepo, cfg, logger)
	settingsH := handlers.NewSettingsHandler(platformRepo, logger)
	logH := handlers.NewLogHandler(auditLogRepo, logger)

	// Webhooks (no auth)
	r.POST("/webhook/line", lineH.Webhook)

	// Auth (rate-limited: 10 req/min per IP)
	r.POST("/api/auth/login", middleware.AuthRateLimit(10, time.Minute), authH.Login)

	// Protected routes
	api := r.Group("/api", middleware.Auth())
	{
		api.GET("/auth/me", authH.Me)

		// Bills
		api.GET("/bills", billH.List)
		api.GET("/bills/:id", billH.Get)
		api.POST("/bills/:id/retry", billH.Retry)
		api.PUT("/bills/:id/items/:item_id", middleware.RequireRole("admin", "staff"), billH.UpdateItem)

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

		// Catalog (SML product catalog + smart matching)
		api.GET("/catalog", catalogH.List)
		api.GET("/catalog/stats", catalogH.Stats)
		api.GET("/catalog/search", catalogH.Search)
		api.GET("/catalog/:code", catalogH.GetOne)
		api.POST("/catalog/sync", middleware.RequireRole("admin"), catalogH.SyncFromAPI)
		api.POST("/catalog/import-csv", middleware.RequireRole("admin"), catalogH.ImportCSV)
		api.POST("/catalog/embed-all", middleware.RequireRole("admin"), catalogH.EmbedAll)
		api.POST("/catalog/reload-index", middleware.RequireRole("admin"), catalogH.ReloadIndex)
		api.POST("/catalog/:code/embed", middleware.RequireRole("admin"), catalogH.EmbedOne)

		// Confirm catalog match for a needs_review bill item
		api.POST("/bills/:id/items/:item_id/confirm-match", middleware.RequireRole("admin", "staff"), catalogH.ConfirmMatch)
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

	emailPoller := jobs.NewEmailPoller(imapSvc, lineSvc, emailH, cfg.IMAPPollInterval, logger)
	if cfg.IMAPHost != "" {
		// Wire Shopee email processors (order + shipped) — both gated by domain
		if cfg.ShopeeEmailDomains != "" {
			domains := strings.Split(cfg.ShopeeEmailDomains, ",")
			imapSvc.SetShopeeProcessor(emailH.ProcessShopeeEmailBody, domains)
			imapSvc.SetShippedProcessor(emailH.ProcessShopeeShippedEmailBody)
			logger.Info("shopee email processors wired", zap.Strings("domains", domains))
		}
		emailPoller.Register(c)
	}

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
