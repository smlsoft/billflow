package handlers

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq" // PostgreSQL driver for DB ping in TestConnection
	"go.uber.org/zap"

	"billflow/internal/config"
	"billflow/internal/repository"
	"billflow/internal/services/sml"
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
	Required     bool   `json:"required,omitempty"`
	Locked       bool   `json:"locked,omitempty"` // ค่าตายตัว ห้ามแก้ผ่าน UI
	Description  string `json:"description,omitempty"`
}

var instanceSettingDefs = []settingDef{
	{Key: "instance.name", Label: "ชื่อร้าน", Group: "instance", Type: "text", DefaultValue: "BillFlow", Description: "ไม่บังคับ ใช้ให้ทีมดูแลรู้ว่า BillFlow ชุดนี้เป็นของร้านไหน"},
	{Key: "instance.slug", Label: "รหัสร้าน", Group: "instance", Type: "text", DefaultValue: "default", Description: "ไม่บังคับ ใช้เป็นชื่อสั้นสำหรับแยกเอกสาร backup และ deploy"},
	{Key: "instance.support_contact", Label: "ผู้ดูแลระบบ", Group: "instance", Type: "text", DefaultValue: "", Description: "ไม่บังคับ เบอร์หรือชื่อคนที่ดูแลระบบชุดนี้"},

	{Key: "sml.rest_base_url", Label: "SML REST URL", Group: "sml", Type: "url", Restart: true, Required: true, Description: "URL ของ sml-api-byboss เช่น http://172.24.0.1:8200 (ใช้ร่วมกันทุกร้าน)"},
	{Key: "sml.guid", Label: "API Key (guid)", Group: "sml", Type: "text", Restart: true, Required: true, Locked: true, Description: "ค่านี้ตายตัว ต้องตรงกับ API key ที่ตั้งใน sml-api-byboss"},
	{Key: "sml.database", Label: "Database (tenant)", Group: "sml", Type: "text", Restart: true, Required: true, Description: "ชื่อ database SML ของร้านนี้ เช่น SML1_2026"},
	{Key: "sml.stock_request_url", Label: "Stock Request URL", Group: "sml", Type: "url", Restart: false, Required: false, Description: "URL ของ SML server คำนวณต้นทุนสต๊อก (ไม่ใช่ sml-api-byboss) — path /SMLJavaWebService/rest/v1/processstockrequest จะถูกเติมอัตโนมัติ เช่น http://192.168.2.248:8080 (ว่าง = ข้ามการคำนวณ)"},

	// sml_db group — PostgreSQL credentials ของร้านค้า ส่งเป็น X-DB-* headers ไปยัง sml-api-byboss
	// Required: false ทั้งหมด — ใช้ custom validation ใน Update() แทน
	// เพื่อไม่ให้ setup_complete=false สำหรับ instance ที่ยังไม่ได้ตั้งค่า
	{Key: "sml_db.host", Label: "DB Host", Group: "sml_db", Type: "text", Required: false, Description: "hostname หรือ IP ของ PostgreSQL ร้านนี้ เช่น demserver.3bbddns.com"},
	{Key: "sml_db.port", Label: "DB Port", Group: "sml_db", Type: "number", Required: false, Description: "port ของ PostgreSQL เช่น 47309 (default PostgreSQL = 5432)"},
	{Key: "sml_db.user", Label: "DB User", Group: "sml_db", Type: "text", Required: false, Description: "เช่น postgres"},
	{Key: "sml_db.password", Label: "DB Password", Group: "sml_db", Type: "password", Required: false, Secret: true, Description: "รหัสผ่าน PostgreSQL — ไม่แสดงใน log"},
	{Key: "sml_db.name", Label: "DB Name", Group: "sml_db", Type: "text", Required: false, Description: "ชื่อ database หลัก เช่น aoy หรือ SML1_2026"},
	{Key: "sml_db.images_name", Label: "DB Images Name", Group: "sml_db", Type: "text", Required: false, Description: "ชื่อ database รูปภาพ เช่น demo_images (ว่าง = sml-api-byboss ใช้ค่าเริ่มต้น)"},
	{Key: "sml_db.log_name", Label: "DB Log Name", Group: "sml_db", Type: "text", Required: false, Description: "ชื่อ database log เช่น demo_logs (ว่าง = sml-api-byboss ใช้ค่าเริ่มต้น)"},

	{Key: "line.notify_channel_secret", Label: "LINE Channel secret", Group: "line", Type: "password", Secret: true, Restart: true, Description: "ใช้กับ LINE OA ที่ส่งแจ้งเตือนระบบ"},
	{Key: "line.notify_channel_access_token", Label: "LINE Channel access token", Group: "line", Type: "password", Secret: true, Restart: true, Description: "ใช้ส่ง Push แจ้งเตือน error และสถานะระบบไปยังแอดมิน"},
	{Key: "line.notify_admin_user_id", Label: "LINE admin user ID", Group: "line", Type: "text", Restart: true, Description: "userId ของผู้รับแจ้งเตือนระบบ เช่น SML error, email error, disk/tunnel warning"},

	{Key: "ai.openrouter_api_key", Label: "OpenRouter API key", Group: "ai", Type: "password", Secret: true, Restart: true, Required: true},
	{Key: "ai.openrouter_model", Label: "Model หลัก", Group: "ai", Type: "text", Restart: true, Required: true},
	{Key: "ai.openrouter_fallback_model", Label: "Model สำรอง", Group: "ai", Type: "text", Restart: true},
	{Key: "ai.openrouter_audio_model", Label: "Audio model", Group: "ai", Type: "text", Restart: true},
	{Key: "automation.auto_confirm_threshold", Label: "เกณฑ์ confidence", Group: "automation", Type: "number", Restart: true},
}

var smlDatabaseNamePattern = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

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
	missingRequired := []string{}
	for _, def := range instanceSettingDefs {
		dbVal, fromDB := dbSettings[def.Key]
		dbValue := ""
		if fromDB {
			dbValue = strings.TrimSpace(dbVal.Value)
		}
		runtimeValue := strings.TrimSpace(runtimeValues[def.Key])
		value := dbValue
		source := "unset"
		if value != "" {
			source = "database"
		} else if runtimeValue != "" {
			value = runtimeValue
			source = "env"
		} else if def.DefaultValue != "" {
			value = def.DefaultValue
			source = "default"
		}

		missing := def.Required && value == ""

		active := true
		pendingRestart := false
		if def.Restart && !def.Locked && dbValue != "" && runtimeValue != "" && dbValue != runtimeValue {
			active = false
			pendingRestart = true
			pendingKeys = append(pendingKeys, def.Key)
		}
		if missing {
			missingRequired = append(missingRequired, def.Key)
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

		settings = append(settings, gin.H{
			"key":              def.Key,
			"label":            def.Label,
			"group":            def.Group,
			"type":             def.Type,
			"value":            displayValue,
			"source":           source,
			"secret":           def.Secret,
			"has_secret":       hasSecret,
			"required":         def.Required,
			"locked":           def.Locked,
			"missing":          missing,
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
		"missing_required":         missingRequired,
		"setup_complete":           len(missingRequired) == 0,
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

	// optional fields that may be explicitly cleared to empty string
	clearableKeys := map[string]bool{
		"sml.stock_request_url": true,
		"sml_db.images_name":    true,
		"sml_db.log_name":       true,
	}

	values := map[string]string{}
	for key, value := range body.Settings {
		def, ok := allowed[key]
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "unknown setting: " + key})
			return
		}
		if def.Locked {
			continue // ค่าตายตัว ไม่อนุญาตให้แก้ผ่าน API
		}
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			if clearableKeys[key] {
				values[key] = "" // explicit clear allowed for optional fields
			}
			continue // skip blank for non-clearable fields
		}
		if def.Secret && strings.Contains(trimmed, "••••••••") {
			continue // skip masked placeholder — user didn't change the secret
		}
		if normalized, msg := normalizeInstanceSetting(def, trimmed); msg != "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": msg, "key": key})
			return
		} else {
			trimmed = normalized
		}
		values[key] = trimmed
	}

	// Merge existing sml_db values with incoming values before validating
	// completeness, so that a masked password doesn't cause a false "incomplete" error.
	existing, _ := h.repo.All()
	smlDBAllKeys := []string{
		"sml_db.host", "sml_db.port", "sml_db.user",
		"sml_db.password", "sml_db.name", "sml_db.images_name", "sml_db.log_name",
	}
	mergedDB := map[string]string{}
	for _, k := range smlDBAllKeys {
		newVal := strings.TrimSpace(body.Settings[k])
		def, _ := allowed[k]
		if newVal == "" || (def.Secret && strings.Contains(newVal, "••••••••")) {
			mergedDB[k] = strings.TrimSpace(existing[k].Value)
		} else {
			mergedDB[k] = newVal
		}
	}

	// Custom group validation: if any required sml_db field is provided,
	// all 5 required fields must be present (partial config is rejected).
	smlDBRequired := []string{"sml_db.host", "sml_db.port", "sml_db.user", "sml_db.password", "sml_db.name"}
	smlDBFilled := 0
	for _, k := range smlDBRequired {
		if mergedDB[k] != "" {
			smlDBFilled++
		}
	}
	if smlDBFilled > 0 && smlDBFilled < len(smlDBRequired) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "กรุณากรอกข้อมูล SML Database Connection ให้ครบ (Host, Port, User, Password, DB Name)",
		})
		return
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

	// Force fresh DB config on the next SML request — no restart needed
	sml.InvalidateDBHeaderCache()

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

// effectiveValue is kept only for optional fields that have a built-in default
// (instance.name / instance.slug). All SML/AI/LINE values must be set via UI.
func (h *InstanceSettingsHandler) effectiveValue(def settingDef, dbSettings map[string]repository.AppSetting) (string, string) {
	if s, ok := dbSettings[def.Key]; ok && strings.TrimSpace(s.Value) != "" {
		return s.Value, "database"
	}
	if def.DefaultValue != "" {
		return def.DefaultValue, "default"
	}
	return "", "unset"
}

// TestConnections tests SML, LINE, and OpenRouter connectivity using saved DB values.
// Each check is independent — partial success is returned so the UI can show per-service status.
func (h *InstanceSettingsHandler) TestConnection(c *gin.Context) {
	dbSettings, err := h.repo.All()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "โหลด config ไม่ได้"})
		return
	}

	allowed := map[string]settingDef{}
	for _, def := range instanceSettingDefs {
		allowed[def.Key] = def
	}
	var body struct {
		Settings map[string]string `json:"settings"`
	}
	if c.Request.Body != nil && c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}
	overrides := map[string]string{}
	for key, value := range body.Settings {
		def, ok := allowed[key]
		if !ok || def.Locked {
			continue
		}
		trimmed := strings.TrimSpace(value)
		if trimmed == "" || (def.Secret && strings.Contains(trimmed, "••••••••")) {
			continue
		}
		if normalized, msg := normalizeInstanceSetting(def, trimmed); msg != "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": msg, "key": key})
			return
		} else {
			overrides[key] = normalized
		}
	}

	cfgFallback := repository.RuntimeSettingValues(h.cfg)
	get := func(key string) string {
		if v := strings.TrimSpace(overrides[key]); v != "" {
			return v
		}
		if v := strings.TrimSpace(dbSettings[key].Value); v != "" {
			return v
		}
		return strings.TrimSpace(cfgFallback[key])
	}

	httpClient := &http.Client{Timeout: 8 * time.Second}

	type checkResult struct {
		OK     bool   `json:"ok"`
		Error  string `json:"error,omitempty"`
		Detail string `json:"detail,omitempty"`
	}

	doGET := func(url string, headers map[string]string) (int, []byte, error) {
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			return 0, nil, err
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		resp, err := httpClient.Do(req)
		if err != nil {
			return 0, nil, err
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, body, nil
	}

	// ── SML ──────────────────────────────────────────────────────────────────
	smlResult := checkResult{}
	baseURL := get("sml.rest_base_url")
	guid := get("sml.guid")
	database := get("sml.database")
	if baseURL == "" || guid == "" || database == "" {
		smlResult.Error = "ยังไม่ได้ตั้งค่า SML REST URL, guid หรือ database"
	} else {
		// Use supplier list — always exists, returns 403 on wrong tenant, 401 on bad guid.
		smlURL := strings.TrimRight(baseURL, "/") + "/api/v1/ic/products?page=1"
		code, body, err := doGET(smlURL, map[string]string{
			"guid":           guid,
			"provider":       get("sml.provider"),
			"configFileName": get("sml.config_file"),
			"databaseName":   database,
			"X-Tenant":       database,
		})
		if err != nil {
			smlResult.Error = fmt.Sprintf("เชื่อมต่อไม่ได้: %v", err)
		} else if code == http.StatusOK {
			smlResult.OK = true
			smlResult.Detail = strings.TrimRight(baseURL, "/")
		} else if code == http.StatusForbidden {
			smlResult.Error = fmt.Sprintf("database '%s' ไม่ถูกต้องหรือไม่มีสิทธิ์เข้าถึง", database)
		} else if code == http.StatusUnauthorized {
			smlResult.Error = "guid (API key) ไม่ถูกต้อง"
		} else {
			smlResult.Error = fmt.Sprintf("server ตอบ %d: %s", code, strings.TrimSpace(string(body)))
		}
	}

	// ── LINE ─────────────────────────────────────────────────────────────────
	lineResult := checkResult{}
	lineToken := get("line.notify_channel_access_token")
	if lineToken == "" {
		lineResult.Error = "ยังไม่ได้ตั้งค่า LINE Channel access token"
	} else {
		code, body, err := doGET("https://api.line.me/v2/bot/info",
			map[string]string{"Authorization": "Bearer " + lineToken})
		if err != nil {
			lineResult.Error = fmt.Sprintf("เชื่อมต่อ LINE API ไม่ได้: %v", err)
		} else if code == http.StatusOK {
			lineResult.OK = true
			// extract displayName from JSON cheaply
			s := string(body)
			if i := strings.Index(s, `"displayName":"`); i >= 0 {
				rest := s[i+15:]
				if j := strings.Index(rest, `"`); j >= 0 {
					lineResult.Detail = rest[:j]
				}
			}
		} else {
			lineResult.Error = "access token ไม่ถูกต้องหรือหมดอายุแล้ว"
		}
	}

	// ── OpenRouter ───────────────────────────────────────────────────────────
	orResult := checkResult{}
	orKey := get("ai.openrouter_api_key")
	if orKey == "" {
		orResult.Error = "ยังไม่ได้ตั้งค่า OpenRouter API key"
	} else {
		code, body, err := doGET("https://openrouter.ai/api/v1/auth/key",
			map[string]string{"Authorization": "Bearer " + orKey})
		if err != nil {
			orResult.Error = fmt.Sprintf("เชื่อมต่อ OpenRouter ไม่ได้: %v", err)
		} else if code == http.StatusOK {
			orResult.OK = true
			// extract limit_remaining from JSON cheaply
			s := string(body)
			if i := strings.Index(s, `"limit_remaining":`); i >= 0 {
				rest := strings.TrimSpace(s[i+18:])
				end := strings.IndexAny(rest, ",}")
				if end > 0 {
					orResult.Detail = "credit คงเหลือ: " + strings.TrimSpace(rest[:end])
				}
			}
		} else {
			orResult.Error = "API key ไม่ถูกต้อง"
		}
	}

	// ── SML Database Ping ────────────────────────────────────────────────────
	dbResult := checkResult{}
	dbHost := get("sml_db.host")
	dbPort := get("sml_db.port")
	dbUser := get("sml_db.user")
	dbPass := get("sml_db.password") // never log
	dbName := get("sml_db.name")
	if dbHost == "" || dbPort == "" || dbUser == "" || dbPass == "" || dbName == "" {
		dbResult.Error = "ยังไม่ได้ตั้งค่า SML Database Connection ให้ครบ"
	} else {
		// Build DSN — do NOT log this string (it contains password)
		dsn := fmt.Sprintf(
			"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable connect_timeout=5",
			dbHost, dbPort, dbUser, dbPass, dbName,
		)
		dbConn, err := sql.Open("postgres", dsn)
		if err != nil {
			dbResult.Error = "ตั้งค่า connection ไม่ได้: config ไม่ถูกต้อง"
		} else {
			defer dbConn.Close()
			pingCtx, pingCancel := timeoutContext(5 * time.Second)
			defer pingCancel()
			if err := dbConn.PingContext(pingCtx); err != nil {
				dbResult.Error = postgresErrToThai(err)
			} else {
				dbResult.OK = true
				dbResult.Detail = fmt.Sprintf("%s:%s/%s (เชื่อมต่อได้)", dbHost, dbPort, dbName)
			}
		}
	}

	allOK := smlResult.OK && lineResult.OK && orResult.OK
	// db not configured is not a failure (instance may not use sml_db yet)
	if dbResult.Error != "" && dbResult.Error != "ยังไม่ได้ตั้งค่า SML Database Connection ให้ครบ" {
		allOK = false
	}
	c.JSON(http.StatusOK, gin.H{
		"ok":         allOK,
		"sml":        smlResult,
		"line":       lineResult,
		"openrouter": orResult,
		"db":         dbResult,
	})
}

func timeoutContext(d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), d)
}

// postgresErrToThai converts a libpq error to a safe Thai message.
// Never include the raw error string — it may contain the DSN or password.
func postgresErrToThai(err error) string {
	s := err.Error()
	switch {
	case strings.Contains(s, "password authentication failed"):
		return "รหัสผ่านไม่ถูกต้อง"
	case strings.Contains(s, "does not exist"):
		return "ไม่พบ database นี้ใน server"
	case strings.Contains(s, "connection refused"):
		return "server ปฏิเสธการเชื่อมต่อ — ตรวจสอบ host/port"
	case strings.Contains(s, "no such host"):
		return "ไม่พบ hostname — ตรวจสอบ DB Host"
	case strings.Contains(s, "timeout"), strings.Contains(s, "i/o timeout"):
		return "connection timeout — ตรวจสอบ firewall หรือ host"
	default:
		return "เชื่อมต่อไม่ได้ — ตรวจสอบ DB config"
	}
}

func maskSecret(v string) string {
	if len(v) <= 8 {
		return "••••••••"
	}
	return v[:4] + "••••••••" + v[len(v)-4:]
}

func normalizeInstanceSetting(def settingDef, value string) (string, string) {
	value = strings.TrimSpace(value)
	switch def.Key {
	case "sml.rest_base_url":
		return normalizeInstanceURL(value)
	case "sml.stock_request_url":
		if value == "" {
			return "", "" // allow clear
		}
		return normalizeInstanceURL(value)
	case "sml.database":
		if !smlDatabaseNamePattern.MatchString(value) {
			return "", "Database (tenant) ใช้ได้เฉพาะตัวอักษร ตัวเลข และ _ เท่านั้น"
		}
	case "sml_db.port":
		p, err := strconv.Atoi(value)
		if err != nil || p < 1 || p > 65535 {
			return "", "DB Port ต้องเป็นตัวเลข 1–65535"
		}
		return value, ""
	case "automation.auto_confirm_threshold":
		f, err := strconv.ParseFloat(value, 64)
		if err != nil || f < 0 || f > 1 {
			return "", "เกณฑ์ confidence ต้องเป็นตัวเลขระหว่าง 0 ถึง 1"
		}
		return floatString(f), ""
	}
	return value, ""
}

func normalizeInstanceURL(value string) (string, string) {
	value = strings.TrimSpace(value)
	u, err := url.Parse(value)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", "SML REST URL ต้องเป็น URL เต็ม เช่น http://192.168.2.109:8200"
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", "SML REST URL ต้องขึ้นต้นด้วย http:// หรือ https://"
	}
	u.Path = strings.TrimRight(u.Path, "/")
	u.RawQuery = ""
	u.Fragment = ""
	return strings.TrimRight(u.String(), "/"), ""
}

func floatString(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}
