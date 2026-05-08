package handlers

import (
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"billflow/internal/config"
	"billflow/internal/repository"
)

type InstanceSettingsHandler struct {
	repo *repository.AppSettingsRepo
	cfg  *config.Config
	log  *zap.Logger
}

func NewInstanceSettingsHandler(repo *repository.AppSettingsRepo, cfg *config.Config, log *zap.Logger) *InstanceSettingsHandler {
	return &InstanceSettingsHandler{repo: repo, cfg: cfg, log: log}
}

type settingDef struct {
	Key          string `json:"key"`
	Label        string `json:"label"`
	Group        string `json:"group"`
	Type         string `json:"type"`
	EnvKey       string `json:"env_key,omitempty"`
	DefaultValue string `json:"default_value,omitempty"`
	Secret       bool   `json:"secret,omitempty"`
	Restart      bool   `json:"restart_required,omitempty"`
	Description  string `json:"description,omitempty"`
}

var instanceSettingDefs = []settingDef{
	{Key: "instance.name", Label: "ชื่อร้าน", Group: "instance", Type: "text", DefaultValue: "BillFlow", Description: "ไม่บังคับ ใช้ให้ทีมดูแลรู้ว่า BillFlow ชุดนี้เป็นของร้านไหน"},
	{Key: "instance.slug", Label: "รหัสร้าน", Group: "instance", Type: "text", DefaultValue: "default", Description: "ไม่บังคับ ใช้เป็นชื่อสั้นสำหรับแยกเอกสาร backup และ deploy"},
	{Key: "instance.support_contact", Label: "ผู้ดูแลระบบ", Group: "instance", Type: "text", DefaultValue: "", Description: "ไม่บังคับ เบอร์หรือชื่อคนที่ดูแลระบบชุดนี้"},

	{Key: "sml.json_rpc_base_url", Label: "SML JSON-RPC URL", Group: "sml", Type: "url", EnvKey: "SML_BASE_URL", Restart: true, Description: "ใช้กับ sale_reserve / LINE / email sale flow"},
	{Key: "sml.rest_base_url", Label: "SML REST URL", Group: "sml", Type: "url", EnvKey: "SHOPEE_SML_URL", Restart: true, Description: "ใช้กับ catalog, saleorder, purchaseorder"},
	{Key: "sml.guid", Label: "GUID", Group: "sml", Type: "text", EnvKey: "SHOPEE_SML_GUID", Restart: true},
	{Key: "sml.provider", Label: "Provider", Group: "sml", Type: "text", EnvKey: "SHOPEE_SML_PROVIDER", Restart: true},
	{Key: "sml.config_file", Label: "Config file", Group: "sml", Type: "text", EnvKey: "SHOPEE_SML_CONFIG_FILE", Restart: true},
	{Key: "sml.database", Label: "Database", Group: "sml", Type: "text", EnvKey: "SHOPEE_SML_DATABASE", Restart: true},

	{Key: "line.notify_channel_secret", Label: "LINE Channel secret", Group: "line", Type: "password", EnvKey: "LINE_CHANNEL_SECRET", Secret: true, Restart: true, Description: "ใช้กับ LINE OA ที่ส่งแจ้งเตือนระบบ"},
	{Key: "line.notify_channel_access_token", Label: "LINE Channel access token", Group: "line", Type: "password", EnvKey: "LINE_CHANNEL_ACCESS_TOKEN", Secret: true, Restart: true, Description: "ใช้ส่ง Push แจ้งเตือน error และสถานะระบบไปยังแอดมิน"},
	{Key: "line.notify_admin_user_id", Label: "LINE admin user ID", Group: "line", Type: "text", EnvKey: "LINE_ADMIN_USER_ID", Restart: true, Description: "userId ของผู้รับแจ้งเตือนระบบ เช่น SML error, email error, disk/tunnel warning"},

	{Key: "ai.openrouter_api_key", Label: "OpenRouter API key", Group: "ai", Type: "password", EnvKey: "OPENROUTER_API_KEY", Secret: true, Restart: true},
	{Key: "ai.openrouter_model", Label: "Model หลัก", Group: "ai", Type: "text", EnvKey: "OPENROUTER_MODEL", Restart: true},
	{Key: "ai.openrouter_fallback_model", Label: "Model สำรอง", Group: "ai", Type: "text", EnvKey: "OPENROUTER_FALLBACK_MODEL", Restart: true},
	{Key: "ai.openrouter_audio_model", Label: "Audio model", Group: "ai", Type: "text", EnvKey: "OPENROUTER_AUDIO_MODEL", Restart: true},
	{Key: "automation.auto_confirm_threshold", Label: "เกณฑ์ confidence", Group: "automation", Type: "number", EnvKey: "AUTO_CONFIRM_THRESHOLD", Restart: true},
}

func (h *InstanceSettingsHandler) Get(c *gin.Context) {
	dbSettings, err := h.repo.All()
	if err != nil {
		h.log.Error("instance settings list", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	runtimeValues := repository.RuntimeSettingValues(h.cfg)
	settings := make([]gin.H, 0, len(instanceSettingDefs))
	pendingKeys := []string{}
	for _, def := range instanceSettingDefs {
		value, source := h.effectiveValue(def, dbSettings)
		runtimeValue := runtimeValues[def.Key]
		active := true
		pendingRestart := false
		if def.Restart && runtimeValue != "" && strings.TrimSpace(value) != strings.TrimSpace(runtimeValue) {
			active = false
			pendingRestart = true
			pendingKeys = append(pendingKeys, def.Key)
		}
		displayValue := value
		displayRuntimeValue := runtimeValue
		hasSecret := false
		if def.Secret && value != "" {
			hasSecret = true
			displayValue = maskSecret(value)
		}
		if def.Secret && runtimeValue != "" {
			displayRuntimeValue = maskSecret(runtimeValue)
		}
		_, fromDB := dbSettings[def.Key]
		settings = append(settings, gin.H{
			"key":              def.Key,
			"label":            def.Label,
			"group":            def.Group,
			"type":             def.Type,
			"value":            displayValue,
			"source":           source,
			"env_key":          def.EnvKey,
			"secret":           def.Secret,
			"has_secret":       hasSecret,
			"restart_required": def.Restart,
			"description":      def.Description,
			"overridden":       fromDB,
			"runtime_value":    displayRuntimeValue,
			"active":           active,
			"pending_restart":  pendingRestart,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"settings":                 settings,
		"restart_required":         len(pendingKeys) > 0,
		"pending_restart":          len(pendingKeys) > 0,
		"pending_restart_settings": pendingKeys,
		"note":                     "SML/AI clients are created at backend boot; restart backend after changing those values.",
	})
}

func (h *InstanceSettingsHandler) Update(c *gin.Context) {
	var body struct {
		Settings map[string]string `json:"settings"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	allowed := map[string]settingDef{}
	secretKeys := map[string]bool{}
	for _, def := range instanceSettingDefs {
		allowed[def.Key] = def
		if def.Secret {
			secretKeys[def.Key] = true
		}
	}

	values := map[string]string{}
	for key, value := range body.Settings {
		def, ok := allowed[key]
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "unknown setting: " + key})
			return
		}
		trimmed := strings.TrimSpace(value)
		if def.Secret && (trimmed == "" || strings.Contains(trimmed, "••••")) {
			continue
		}
		values[key] = trimmed
	}
	if len(values) == 0 {
		c.JSON(http.StatusOK, gin.H{"ok": true, "updated": 0})
		return
	}

	userID := c.GetString("user_id")
	if err := h.repo.UpsertMany(values, secretKeys, userID); err != nil {
		h.log.Error("instance settings update", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":               true,
		"updated":          len(values),
		"restart_required": true,
	})
}

func (h *InstanceSettingsHandler) Restart(c *gin.Context) {
	h.log.Warn("admin requested backend restart",
		zap.String("user_id", c.GetString("user_id")),
		zap.String("user_email", c.GetString("user_email")),
	)
	c.JSON(http.StatusAccepted, gin.H{
		"ok":      true,
		"message": "backend restart scheduled",
	})

	go func() {
		time.Sleep(500 * time.Millisecond)
		os.Exit(0)
	}()
}

func (h *InstanceSettingsHandler) effectiveValue(def settingDef, dbSettings map[string]repository.AppSetting) (string, string) {
	if s, ok := dbSettings[def.Key]; ok && strings.TrimSpace(s.Value) != "" {
		return s.Value, "database"
	}
	if def.EnvKey != "" {
		if v := strings.TrimSpace(os.Getenv(def.EnvKey)); v != "" {
			return v, "env"
		}
	}
	switch def.Key {
	case "sml.json_rpc_base_url":
		return h.cfg.SMLBaseURL, "default"
	case "sml.rest_base_url":
		return h.cfg.ShopeeSMLURL, "default"
	case "sml.guid":
		return h.cfg.ShopeeSMLGUID, "default"
	case "sml.provider":
		return h.cfg.ShopeeSMLProvider, "default"
	case "sml.config_file":
		return h.cfg.ShopeeSMLConfigFile, "default"
	case "sml.database":
		return h.cfg.ShopeeSMLDatabase, "default"
	case "line.notify_channel_secret":
		return h.cfg.LineChannelSecret, "default"
	case "line.notify_channel_access_token":
		return h.cfg.LineChannelAccessToken, "default"
	case "line.notify_admin_user_id":
		return h.cfg.LineAdminUserID, "default"
	case "ai.openrouter_api_key":
		return h.cfg.OpenRouterAPIKey, "default"
	case "ai.openrouter_model":
		return h.cfg.OpenRouterModel, "default"
	case "ai.openrouter_fallback_model":
		return h.cfg.OpenRouterFallback, "default"
	case "ai.openrouter_audio_model":
		return h.cfg.OpenRouterAudioModel, "default"
	case "automation.auto_confirm_threshold":
		return floatString(h.cfg.AutoConfirmThreshold), "default"
	}
	return def.DefaultValue, "default"
}

func maskSecret(v string) string {
	if len(v) <= 8 {
		return "••••••••"
	}
	return v[:4] + "••••••••" + v[len(v)-4:]
}

func floatString(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}
