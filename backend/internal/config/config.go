package config

import (
	"log"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	// Server
	Port string
	Env  string

	// Database
	DatabaseURL string
	DBUser      string
	DBPassword  string

	// JWT
	JWTSecret      string
	JWTExpireHours int

	// LINE
	LineChannelSecret      string
	LineChannelAccessToken string
	LineAdminUserID        string

	// IMAP — moved to imap_accounts table (DB-driven, multi-account).
	// Manage via /settings/email instead of env vars.

	// OpenRouter
	OpenRouterAPIKey     string
	OpenRouterModel      string
	OpenRouterFallback   string
	OpenRouterAudioModel string

	// Mistral
	MistralAPIKey string

	// SML (existing — JSON-RPC for LINE/Email)
	SMLBaseURL string

	// Shopee SML (REST API — saleinvoice)
	ShopeeSMLURL        string
	ShopeeSMLGUID       string
	ShopeeSMLProvider   string
	ShopeeSMLConfigFile string
	ShopeeSMLDatabase   string
	ShopeeSMLDocFormat  string
	ShopeeSMLCustCode   string
	ShopeeSMLSaleCode   string
	ShopeeSMLBranchCode string
	ShopeeSMLWHCode     string
	ShopeeSMLShelfCode  string
	ShopeeSMLUnitCode   string
	ShopeeSMLVATType    int
	ShopeeSMLVATRate    float64
	ShopeeSMLDocTime    string

	// Shopee shipped email → SML purchaseorder
	// Reuses all SHOPEE_SML_* fields above; only doc_format and cust_code differ.
	ShippedSMLDocFormat string
	ShippedSMLCustCode  string

	// Gemini (for text-embedding-004)
	GeminiAPIKey string

	// Shopee email detection moved to per-account `shopee_domains` column
	// in imap_accounts table.

	// Auto-confirm
	AutoConfirmThreshold float64

	// Cron
	InsightCronHour   int
	BackupCronHour    int
	InsightLineNotify bool
	DiskWarnPercent   int

	// Artifacts (original source files attached to each bill)
	ArtifactsDir      string
	ArtifactsMaxBytes int64
}

func Load() *Config {
	// Load .env if present (ignore error — production uses OS env)
	_ = godotenv.Load()

	c := &Config{
		Port:                   getEnv("PORT", "8090"),
		Env:                    getEnv("ENV", "development"),
		DatabaseURL:            getEnv("DATABASE_URL", ""),
		DBUser:                 getEnv("DB_USER", "billflow"),
		DBPassword:             getEnv("DB_PASSWORD", "changeme"),
		JWTSecret:              getEnv("JWT_SECRET", ""),
		JWTExpireHours:         getEnvInt("JWT_EXPIRE_HOURS", 24),
		LineChannelSecret:      getEnv("LINE_CHANNEL_SECRET", ""),
		LineChannelAccessToken: getEnv("LINE_CHANNEL_ACCESS_TOKEN", ""),
		LineAdminUserID:        getEnv("LINE_ADMIN_USER_ID", ""),
		OpenRouterAPIKey:       getEnv("OPENROUTER_API_KEY", ""),
		OpenRouterModel:        getEnv("OPENROUTER_MODEL", "google/gemini-2.5-flash"),
		OpenRouterFallback:     getEnv("OPENROUTER_FALLBACK_MODEL", "google/gemini-flash-1.5"),
		OpenRouterAudioModel:   getEnv("OPENROUTER_AUDIO_MODEL", "openai/whisper-1"),
		MistralAPIKey:          getEnv("MISTRAL_API_KEY", ""),
		SMLBaseURL:             getEnv("SML_BASE_URL", "http://192.168.2.213:3248"),
		ShopeeSMLURL:           getEnv("SHOPEE_SML_URL", "http://192.168.2.248:8080"),
		ShopeeSMLGUID:          getEnv("SHOPEE_SML_GUID", "SMLX"),
		ShopeeSMLProvider:      getEnv("SHOPEE_SML_PROVIDER", "SML1"),
		ShopeeSMLConfigFile:    getEnv("SHOPEE_SML_CONFIG_FILE", "SMLConfigSML1.xml"),
		ShopeeSMLDatabase:      getEnv("SHOPEE_SML_DATABASE", "SMLPLOY"),
		ShopeeSMLDocFormat:     getEnv("SHOPEE_SML_DOC_FORMAT", "RU"),
		ShopeeSMLCustCode:      getEnv("SHOPEE_SML_CUST_CODE", ""),
		ShopeeSMLSaleCode:      getEnv("SHOPEE_SML_SALE_CODE", ""),
		ShopeeSMLBranchCode:    getEnv("SHOPEE_SML_BRANCH_CODE", "001"),
		ShopeeSMLWHCode:        getEnv("SHOPEE_SML_WH_CODE", ""),
		ShopeeSMLShelfCode:     getEnv("SHOPEE_SML_SHELF_CODE", ""),
		ShopeeSMLUnitCode:      getEnv("SHOPEE_SML_UNIT_CODE", ""),
		ShopeeSMLVATType:       getEnvInt("SHOPEE_SML_VAT_TYPE", 0),
		ShopeeSMLVATRate:       getEnvFloat("SHOPEE_SML_VAT_RATE", 7),
		ShopeeSMLDocTime:       getEnv("SHOPEE_SML_DOC_TIME", "09:00"),
		ShippedSMLDocFormat:    getEnv("SHIPPED_SML_DOC_FORMAT", "PO"),
		ShippedSMLCustCode:     getEnv("SHIPPED_SML_CUST_CODE", getEnv("SHOPEE_SML_CUST_CODE", "")),
		GeminiAPIKey:           getEnv("GEMINI_API_KEY", ""),
		AutoConfirmThreshold:   getEnvFloat("AUTO_CONFIRM_THRESHOLD", 0.85),
		InsightCronHour:        getEnvInt("INSIGHT_CRON_HOUR", 8),
		BackupCronHour:         getEnvInt("BACKUP_CRON_HOUR", 0),
		InsightLineNotify:      getEnvBool("INSIGHT_LINE_NOTIFY", true),
		DiskWarnPercent:        getEnvInt("DISK_WARN_PERCENT", 90),
		ArtifactsDir:           getEnv("ARTIFACTS_DIR", "/app/artifacts"),
		ArtifactsMaxBytes:      int64(getEnvInt("ARTIFACTS_MAX_BYTES", 10*1024*1024)), // 10 MB
	}

	if c.JWTSecret == "" {
		log.Fatal("JWT_SECRET must be set (min 32 chars)")
	}
	if len(c.JWTSecret) < 32 {
		log.Fatal("JWT_SECRET must be at least 32 characters")
	}
	if c.DatabaseURL == "" {
		log.Fatal("DATABASE_URL must be set")
	}

	return c
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func getEnvFloat(key string, fallback float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return fallback
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}
