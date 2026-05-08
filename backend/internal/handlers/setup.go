package handlers

import (
	"database/sql"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"billflow/internal/config"
	"billflow/internal/repository"
)

type SetupHandler struct {
	db          *sql.DB
	cfg         *config.Config
	appSettings *repository.AppSettingsRepo
	logger      *zap.Logger
}

func NewSetupHandler(db *sql.DB, cfg *config.Config, appSettings *repository.AppSettingsRepo, logger *zap.Logger) *SetupHandler {
	return &SetupHandler{db: db, cfg: cfg, appSettings: appSettings, logger: logger}
}

func (h *SetupHandler) Status(c *gin.Context) {
	settings, err := h.appSettings.All()
	if err != nil {
		h.logger.Error("setup status settings", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	pendingRestart, pendingKeys, err := h.appSettings.PendingRestart(h.cfg)
	if err != nil {
		h.logger.Warn("setup status pending restart", zap.Error(err))
	}

	smlMissing := missingSettings(settings, []string{
		"instance.name",
		"sml.rest_base_url",
		"sml.guid",
		"sml.provider",
		"sml.config_file",
		"sml.database",
	})
	smlReady := len(smlMissing) == 0 && !pendingRestart

	channelReady, channelMissing := h.channelReady()
	emailReady, emailDetail := h.emailReady()
	catalogReady, catalogDetail := h.catalogReady()

	steps := []gin.H{
		{
			"key":         "instance",
			"title":       "ข้อมูลร้านและ SML",
			"description": "กรอก SML REST URL, GUID, Provider, Config file และ Database ของร้านนี้",
			"href":        "/settings/instance",
			"ready":       smlReady,
			"status":      statusText(smlReady, pendingRestart),
			"missing":     smlMissing,
			"blocking":    true,
		},
		{
			"key":         "channels",
			"title":       "เส้นทางเอกสาร SML",
			"description": "ตั้งปลายทาง Email บิลซื้อ Shopee -> ซื้อ -> ใบสั่งซื้อ และรูปแบบเลขเอกสาร",
			"href":        "/settings/channels",
			"ready":       channelReady,
			"status":      statusText(channelReady, false),
			"missing":     channelMissing,
			"blocking":    true,
		},
		{
			"key":         "email",
			"title":       "กล่องอีเมลรับบิล",
			"description": "เพิ่ม inbox และทดสอบการเชื่อมต่อ IMAP ให้พร้อมรับบิล",
			"href":        "/settings/email",
			"ready":       emailReady,
			"status":      emailDetail,
			"blocking":    true,
		},
		{
			"key":         "catalog",
			"title":       "สินค้าใน SML",
			"description": "Sync สินค้าจาก SML และสร้างข้อมูลจับคู่เพื่อช่วยตรวจบิล",
			"href":        "/settings/catalog",
			"ready":       catalogReady,
			"status":      catalogDetail,
			"blocking":    true,
		},
	}

	readyCount := 0
	for _, s := range steps {
		if s["ready"] == true {
			readyCount++
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"ready":                    readyCount == len(steps),
		"ready_count":              readyCount,
		"total_count":              len(steps),
		"pending_restart":          pendingRestart,
		"pending_restart_settings": pendingKeys,
		"steps":                    steps,
	})
}

func missingSettings(settings map[string]repository.AppSetting, keys []string) []string {
	missing := []string{}
	for _, key := range keys {
		if strings.TrimSpace(settings[key].Value) == "" {
			missing = append(missing, key)
		}
	}
	return missing
}

func statusText(ready, pendingRestart bool) string {
	if pendingRestart {
		return "รอรีสตาร์ทเพื่อใช้ค่าใหม่"
	}
	if ready {
		return "พร้อมใช้งาน"
	}
	return "ยังตั้งค่าไม่ครบ"
}

func (h *SetupHandler) channelReady() (bool, []string) {
	var endpoint, docFormat, prefix, running string
	err := h.db.QueryRow(`
		SELECT endpoint, doc_format_code, doc_prefix, doc_running_format
		  FROM channel_defaults
		 WHERE channel='shopee_shipped' AND bill_type='purchase'`,
	).Scan(&endpoint, &docFormat, &prefix, &running)
	if err != nil {
		return false, []string{"Email บิลซื้อ Shopee"}
	}
	missing := []string{}
	if strings.TrimSpace(endpoint) == "" {
		missing = append(missing, "ปลายทาง SML")
	}
	if strings.TrimSpace(docFormat) == "" {
		missing = append(missing, "รหัสเอกสาร")
	}
	if strings.TrimSpace(prefix) == "" {
		missing = append(missing, "doc prefix")
	}
	if strings.TrimSpace(running) == "" {
		missing = append(missing, "รูปแบบเลขรัน")
	}
	return len(missing) == 0, missing
}

func (h *SetupHandler) emailReady() (bool, string) {
	var total, okCount int
	_ = h.db.QueryRow(`
		SELECT COUNT(*),
		       COUNT(*) FILTER (WHERE last_poll_status='ok')
		  FROM imap_accounts
		 WHERE enabled=TRUE`,
	).Scan(&total, &okCount)
	if total == 0 {
		return false, "ยังไม่มี inbox ที่เปิดใช้งาน"
	}
	if okCount == 0 {
		return false, "เพิ่ม inbox แล้ว แต่ยังไม่เคยทดสอบ/poll ผ่าน"
	}
	return true, "พร้อมใช้งาน"
}

func (h *SetupHandler) catalogReady() (bool, string) {
	var total, embedded int
	_ = h.db.QueryRow(`
		SELECT COUNT(*),
		       COUNT(*) FILTER (WHERE embedding_status='done')
		  FROM sml_catalog`,
	).Scan(&total, &embedded)
	if total == 0 {
		return false, "ยังไม่ได้ Sync สินค้า"
	}
	if embedded == 0 {
		return false, "Sync แล้ว แต่ยังไม่ได้สร้างข้อมูลจับคู่"
	}
	return true, "พร้อมใช้งาน"
}
