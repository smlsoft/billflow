package handlers

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"billflow/internal/services/shopeeapi"
)

const (
	shopeeAPIOAuthTTL           = 15 * time.Minute
	shopeeAPIAccessTokenSkew    = 10 * time.Minute
	shopeeAPIRefreshTokenTTL    = 30 * 24 * time.Hour
	shopeeAPIMaxRange           = 15 * 24 * time.Hour
	shopeeAPIDefaultPageSize    = 20
	shopeeAPIMaxDetailBatchSize = 50
)

type ShopeeAPIReadinessCheck struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	Status string `json:"status"` // ok | warning | blocked
	Detail string `json:"detail,omitempty"`
}

type ShopeeAPIStatus struct {
	Enabled          bool                      `json:"enabled"`
	Configured       bool                      `json:"configured"`
	Environment      string                    `json:"environment"`
	BaseURL          string                    `json:"base_url,omitempty"`
	PartnerID        int64                     `json:"partner_id,omitempty"`
	RedirectURL      string                    `json:"redirect_url,omitempty"`
	Connected        bool                      `json:"connected"`
	ShopID           int64                     `json:"shop_id,omitempty"`
	ShopName         string                    `json:"shop_name,omitempty"`
	AccessExpiresAt  string                    `json:"access_expires_at,omitempty"`
	RefreshExpiresAt string                    `json:"refresh_expires_at,omitempty"`
	LastSyncAt       string                    `json:"last_sync_at,omitempty"`
	LastSyncStatus   string                    `json:"last_sync_status,omitempty"`
	LastSyncError    string                    `json:"last_sync_error,omitempty"`
	TokenState       string                    `json:"token_state,omitempty"`
	CanConnect       bool                      `json:"can_connect"`
	CanFetch         bool                      `json:"can_fetch"`
	BlockingReason   string                    `json:"blocking_reason,omitempty"`
	Checks           []ShopeeAPIReadinessCheck `json:"checks"`
}

type ShopeeAPIConnection struct {
	ID               string
	ShopID           int64
	ShopName         string
	AccessToken      string
	RefreshToken     string
	AccessExpiresAt  time.Time
	RefreshExpiresAt time.Time
	Environment      string
	LastSyncAt       sql.NullTime
	LastSyncStatus   string
	LastSyncError    string
}

type ShopeeAPIPreviewRequest struct {
	TimeFrom       string `json:"time_from"`
	TimeTo         string `json:"time_to"`
	TimeRangeField string `json:"time_range_field"`
	OrderStatus    string `json:"order_status"`
	PageSize       int    `json:"page_size"`
	Cursor         string `json:"cursor"`
}

type shopeeOAuthState struct {
	UserID      string
	Environment string
	RedirectURL string
}

// GetAPIStatus returns Shopee Open API readiness and the active shop connection.
func (h *ShopeeImportHandler) GetAPIStatus(c *gin.Context) {
	status := h.shopeeAPIStatus()
	conn, err := h.getShopeeAPIConnection(c.Request.Context())
	if err != nil && err != sql.ErrNoRows {
		h.logger.Warn("shopee_api: status connection lookup failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "โหลดสถานะ Shopee API ไม่ได้"})
		return
	}
	if conn != nil {
		status.Connected = true
		status.ShopID = conn.ShopID
		status.ShopName = conn.ShopName
		status.AccessExpiresAt = conn.AccessExpiresAt.Format(time.RFC3339)
		status.RefreshExpiresAt = conn.RefreshExpiresAt.Format(time.RFC3339)
		status.LastSyncStatus = conn.LastSyncStatus
		status.LastSyncError = conn.LastSyncError
		if conn.LastSyncAt.Valid {
			status.LastSyncAt = conn.LastSyncAt.Time.Format(time.RFC3339)
		}
	}
	status.finalizeReadiness(time.Now())
	c.JSON(http.StatusOK, status)
}

// CreateAPIAuthURL creates a short-lived state and returns the Shopee authorize URL.
func (h *ShopeeImportHandler) CreateAPIAuthURL(c *gin.Context) {
	status := h.shopeeAPIStatus()
	if !status.Enabled || !status.Configured {
		respondShopeeAPIError(c, http.StatusBadRequest, fmt.Errorf("not configured"), "Shopee Open API ยังไม่ได้ตั้งค่า partner_id/key บน server")
		return
	}
	userID := c.GetString("user_id")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing user"})
		return
	}
	state, err := randomState()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "สร้าง OAuth state ไม่ได้"})
		return
	}
	redirectURL := h.shopeeAPIRedirectURL()
	if redirectURL == "" {
		respondShopeeAPIError(c, http.StatusBadRequest, fmt.Errorf("redirect URL is required"), "PUBLIC_BASE_URL หรือ SHOPEE_OPEN_API_REDIRECT_URL ยังไม่พร้อม")
		return
	}
	stateHash := hashState(state)
	_, err = h.billRepo.DB().ExecContext(
		c.Request.Context(),
		`INSERT INTO shopee_api_oauth_states
		   (state_hash, user_id, environment, redirect_url, expires_at)
		 VALUES ($1, $2, $3, $4, NOW() + $5::interval)`,
		stateHash, userID, status.Environment, redirectURL, fmt.Sprintf("%d seconds", int(shopeeAPIOAuthTTL.Seconds())),
	)
	if err != nil {
		h.logger.Warn("shopee_api: insert oauth state failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "บันทึก OAuth state ไม่ได้"})
		return
	}
	authURL, err := h.shopeeAPIClient().AuthURL(redirectURL, state, time.Now())
	if err != nil {
		respondShopeeAPIError(c, http.StatusBadRequest, err, "สร้างลิงก์ Shopee OAuth ไม่สำเร็จ")
		return
	}
	c.JSON(http.StatusOK, gin.H{"auth_url": authURL, "redirect_url": redirectURL})
}

// APICallback exchanges Shopee's one-time auth code for access/refresh tokens.
func (h *ShopeeImportHandler) APICallback(c *gin.Context) {
	code := strings.TrimSpace(c.Query("code"))
	state := strings.TrimSpace(c.Query("state"))
	shopID, _ := strconv.ParseInt(strings.TrimSpace(c.Query("shop_id")), 10, 64)
	if code == "" || shopID <= 0 {
		h.renderShopeeCallback(c, http.StatusBadRequest, "เชื่อมต่อ Shopee ไม่สำเร็จ", "Shopee callback ไม่มี code/shop_id ครบ")
		return
	}

	var createdState *shopeeOAuthState
	var err error
	if state == "" {
		h.logger.Warn("shopee_api: oauth callback missing state; trying sole pending-state fallback", zap.Int64("shop_id", shopID))
		createdState, err = h.consumeSolePendingShopeeOAuthState(c.Request.Context())
	} else {
		createdState, err = h.consumeShopeeOAuthState(c.Request.Context(), state)
	}
	if err != nil {
		h.logger.Warn("shopee_api: oauth state invalid", zap.Error(err))
		message := "OAuth state หมดอายุหรือถูกใช้ไปแล้ว"
		if state == "" {
			message = "Shopee ไม่ส่ง state กลับมา และไม่พบ session เชื่อมต่อที่ปลอดภัย กรุณากดเชื่อมต่อ Shopee API ใหม่อีกครั้ง"
		}
		h.renderShopeeCallback(c, http.StatusBadRequest, "เชื่อมต่อ Shopee ไม่สำเร็จ", message)
		return
	}
	tok, err := h.shopeeAPIClient().GetToken(c.Request.Context(), code, shopID)
	if err != nil {
		h.logger.Warn("shopee_api: token exchange failed", zap.Int64("shop_id", shopID), zap.Error(err))
		h.renderShopeeCallback(c, http.StatusBadGateway, "เชื่อมต่อ Shopee ไม่สำเร็จ", err.Error())
		return
	}
	if tok.ShopID > 0 {
		shopID = tok.ShopID
	}
	accessExpires := time.Now().Add(time.Duration(tok.ExpireIn) * time.Second)
	if tok.ExpireIn <= 0 {
		accessExpires = time.Now().Add(4 * time.Hour)
	}
	refreshExpires := time.Now().Add(shopeeAPIRefreshTokenTTL)
	if err := h.upsertShopeeAPIConnection(c.Request.Context(), shopID, tok.AccessToken, tok.RefreshToken, accessExpires, refreshExpires, createdState.UserID, createdState.Environment); err != nil {
		h.logger.Warn("shopee_api: upsert connection failed", zap.Int64("shop_id", shopID), zap.Error(err))
		h.renderShopeeCallback(c, http.StatusInternalServerError, "เชื่อมต่อ Shopee ไม่สำเร็จ", "บันทึก token ไม่สำเร็จ")
		return
	}
	h.renderShopeeCallback(c, http.StatusOK, "เชื่อมต่อ Shopee สำเร็จ", "กลับไปหน้า BillFlow แล้วกดดึงออเดอร์ทดสอบได้เลย")
}

// PreviewFromAPI fetches Shopee orders and returns the same preview shape as
// Shopee Excel. It does not write bills or call SML.
func (h *ShopeeImportHandler) PreviewFromAPI(c *gin.Context) {
	var req ShopeeAPIPreviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "request ไม่ถูกต้อง: " + err.Error()})
		return
	}
	timeFrom, timeTo, err := parseShopeeAPIRange(req.TimeFrom, req.TimeTo)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	conn, err := h.ensureShopeeAPIAccessToken(c.Request.Context())
	if err != nil {
		msg := shopeeAPIErrorMessage(err, err.Error()).Message
		h.markShopeeAPISync(c.Request.Context(), nil, "error", msg)
		respondShopeeAPIError(c, http.StatusBadRequest, err, err.Error())
		return
	}

	pageSize := req.PageSize
	if pageSize <= 0 {
		pageSize = shopeeAPIDefaultPageSize
	}
	if pageSize > shopeeAPIMaxDetailBatchSize {
		pageSize = shopeeAPIMaxDetailBatchSize
	}

	client := h.shopeeAPIClient()
	list, err := client.GetOrderList(c.Request.Context(), conn.AccessToken, conn.ShopID, shopeeapi.OrderListRequest{
		TimeRangeField: defaultShopeeAPITimeField(req.TimeRangeField),
		TimeFrom:       timeFrom.Unix(),
		TimeTo:         timeTo.Unix(),
		PageSize:       pageSize,
		Cursor:         req.Cursor,
		OrderStatus:    strings.TrimSpace(req.OrderStatus),
	})
	if err != nil {
		msg := shopeeAPIErrorMessage(err, "ดึงรายการ order จาก Shopee ไม่สำเร็จ").Message
		h.markShopeeAPISync(c.Request.Context(), &conn.ShopID, "error", msg)
		respondShopeeAPIError(c, http.StatusBadGateway, err, "ดึงรายการ order จาก Shopee ไม่สำเร็จ")
		return
	}
	orderSNs := make([]string, 0, len(list.Response.OrderList))
	for _, o := range list.Response.OrderList {
		if strings.TrimSpace(o.OrderSN) != "" {
			orderSNs = append(orderSNs, o.OrderSN)
		}
	}
	detail, err := client.GetOrderDetail(c.Request.Context(), conn.AccessToken, conn.ShopID, orderSNs, shopeeAPIOrderDetailFields())
	if err != nil {
		msg := shopeeAPIErrorMessage(err, "ดึงรายละเอียด order จาก Shopee ไม่สำเร็จ").Message
		h.markShopeeAPISync(c.Request.Context(), &conn.ShopID, "error", msg)
		respondShopeeAPIError(c, http.StatusBadGateway, err, "ดึงรายละเอียด order จาก Shopee ไม่สำเร็จ")
		return
	}
	orders, warnings := h.shopeeAPIOrdersToPreview(detail.Response.OrderList)
	dupCount := 0
	for i := range orders {
		if billID, exists, _ := h.findShopeeOrderBillID(orders[i].OrderID); exists {
			orders[i].Duplicate = true
			orders[i].ExistingBillID = billID
			dupCount++
		}
	}
	preflight := buildShopeePreflight(orders, 0, dupCount)
	importRunID := h.createShopeeImportRun(c, "Shopee API "+time.Now().Format("20060102-150405"), "", orders, warnings, preflight)
	h.markShopeeAPISync(c.Request.Context(), &conn.ShopID, "ok", "")

	c.JSON(http.StatusOK, gin.H{
		"orders":          orders,
		"warnings":        warnings,
		"total_orders":    len(orders),
		"new_count":       len(orders) - dupCount,
		"duplicate_count": dupCount,
		"skipped_count":   0,
		"import_run_id":   importRunID,
		"preflight":       preflight,
		"file_token":      "",
		"more":            list.Response.More,
		"next_cursor":     list.Response.NextCursor,
	})
}

func (h *ShopeeImportHandler) shopeeAPIStatus() ShopeeAPIStatus {
	env := strings.ToLower(strings.TrimSpace(h.cfg.ShopeeOpenAPIEnv))
	if env == "" {
		env = "sandbox"
	}
	return ShopeeAPIStatus{
		Enabled:     h.cfg.ShopeeOpenAPIEnabled,
		Configured:  h.cfg.ShopeeOpenAPIPartnerID > 0 && strings.TrimSpace(h.cfg.ShopeeOpenAPIPartnerKey) != "",
		Environment: env,
		BaseURL:     h.cfg.ShopeeOpenAPIBaseURL,
		PartnerID:   h.cfg.ShopeeOpenAPIPartnerID,
		RedirectURL: h.shopeeAPIRedirectURL(),
	}
}

func (s *ShopeeAPIStatus) finalizeReadiness(now time.Time) {
	s.Checks = nil
	add := func(key, label string, ok bool, detail string) {
		state := "ok"
		if !ok {
			state = "blocked"
		}
		s.Checks = append(s.Checks, ShopeeAPIReadinessCheck{
			Key:    key,
			Label:  label,
			Status: state,
			Detail: detail,
		})
	}
	addWarning := func(key, label, detail string) {
		s.Checks = append(s.Checks, ShopeeAPIReadinessCheck{
			Key:    key,
			Label:  label,
			Status: "warning",
			Detail: detail,
		})
	}

	add("enabled", "เปิด Shopee Open API บน server", s.Enabled, "ตั้งค่า SHOPEE_OPEN_API_ENABLED=true")
	add("partner_key", "ตั้งค่า Partner ID / Key", s.Configured, "ใส่ Partner ID และ Partner Key ให้ครบใน server env")

	redirectOK, redirectDetail := shopeeRedirectReady(s.RedirectURL)
	add("redirect_url", "Redirect URL พร้อมใช้งาน", redirectOK, redirectDetail)

	baseOK, baseDetail := shopeeBaseURLMatchesEnvironment(s.Environment, s.BaseURL)
	add("base_url", "Base URL ตรงกับ sandbox/live", baseOK, baseDetail)

	if strings.EqualFold(s.Environment, "live") {
		s.Checks = append(s.Checks, ShopeeAPIReadinessCheck{
			Key:    "live_key",
			Label:  "ใช้ Live key หลัง Shopee approve",
			Status: "ok",
			Detail: "environment=live",
		})
	} else {
		addWarning("live_key", "ใช้ Live key หลัง Shopee approve", "ตอนนี้ยังเป็น sandbox ระหว่างรอ Shopee Go-Live approve")
	}

	if s.Connected {
		s.Checks = append(s.Checks, ShopeeAPIReadinessCheck{
			Key:    "shop_connection",
			Label:  "เชื่อมร้านผ่าน OAuth",
			Status: "ok",
			Detail: fmt.Sprintf("shop_id=%d", s.ShopID),
		})
	} else {
		addWarning("shop_connection", "เชื่อมร้านผ่าน OAuth", "ยังไม่มีร้านที่เชื่อมกับ environment นี้")
	}

	if s.Connected {
		s.TokenState = shopeeTokenState(s.AccessExpiresAt, s.RefreshExpiresAt, now)
		tokenStatus := "ok"
		tokenDetail := "access token ยังใช้ได้"
		switch s.TokenState {
		case "access_expiring":
			tokenStatus = "warning"
			tokenDetail = "access token ใกล้หมดอายุ ระบบจะ refresh ก่อนดึงข้อมูล"
		case "refresh_required":
			tokenStatus = "warning"
			tokenDetail = "access token หมดอายุแล้ว แต่ refresh token ยังใช้ได้"
		case "refresh_expired":
			tokenStatus = "blocked"
			tokenDetail = "refresh token หมดอายุ ต้องเชื่อมร้านใหม่"
		}
		s.Checks = append(s.Checks, ShopeeAPIReadinessCheck{
			Key:    "token",
			Label:  "Token พร้อมสำหรับดึง order",
			Status: tokenStatus,
			Detail: tokenDetail,
		})
	}

	if s.LastSyncStatus == "error" && strings.TrimSpace(s.LastSyncError) != "" {
		addWarning("last_sync", "Last sync มี error", s.LastSyncError)
	}

	blockers := []string{}
	for _, check := range s.Checks {
		if check.Status == "blocked" {
			blockers = append(blockers, check.Detail)
		}
	}
	if len(blockers) > 0 {
		s.BlockingReason = blockers[0]
	}
	if s.BlockingReason == "" && !strings.EqualFold(s.Environment, "live") && !s.Connected {
		s.BlockingReason = "รอ Shopee approve แล้วเปลี่ยนเป็น live key ก่อนเชื่อมร้านจริง"
	}
	if s.BlockingReason == "" && !s.Connected {
		s.BlockingReason = "ยังไม่ได้เชื่อมต่อร้าน Shopee"
	}
	if s.TokenState == "refresh_expired" {
		s.BlockingReason = "Shopee refresh token หมดอายุ ต้องเชื่อมร้านใหม่"
	}

	s.CanConnect = s.Enabled && s.Configured && redirectOK && baseOK && (strings.EqualFold(s.Environment, "live") || s.Connected)
	s.CanFetch = s.Enabled && s.Configured && redirectOK && baseOK && s.Connected && s.TokenState != "refresh_expired"
}

func shopeeRedirectReady(raw string) (bool, string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false, "ตั้งค่า PUBLIC_BASE_URL หรือ SHOPEE_OPEN_API_REDIRECT_URL"
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return false, "Redirect URL ไม่ถูกต้อง"
	}
	if u.Scheme != "https" {
		return false, "Shopee OAuth ต้องใช้ HTTPS redirect URL"
	}
	if !strings.HasSuffix(u.Path, "/api/shopee-api/callback") {
		return false, "Redirect URL ต้องชี้ไปที่ /api/shopee-api/callback"
	}
	return true, "Redirect URL พร้อม"
}

func shopeeBaseURLMatchesEnvironment(env, raw string) (bool, string) {
	base := strings.ToLower(strings.TrimRight(strings.TrimSpace(raw), "/"))
	if base == "" {
		return false, "SHOPEE_OPEN_API_BASE_URL ว่าง"
	}
	if strings.EqualFold(env, "live") {
		if base != shopeeapi.DefaultLiveBaseURL {
			return false, "environment=live ต้องใช้ https://partner.shopeemobile.com"
		}
		return true, "Live base URL พร้อม"
	}
	if base == shopeeapi.DefaultLiveBaseURL {
		return false, "environment=sandbox แต่ base URL เป็น live"
	}
	return true, "Sandbox base URL พร้อม"
}

func shopeeTokenState(accessRaw, refreshRaw string, now time.Time) string {
	access, _ := time.Parse(time.RFC3339, strings.TrimSpace(accessRaw))
	refresh, _ := time.Parse(time.RFC3339, strings.TrimSpace(refreshRaw))
	if refresh.IsZero() || !now.Before(refresh) {
		return "refresh_expired"
	}
	if access.IsZero() || !now.Before(access) {
		return "refresh_required"
	}
	if !now.Before(access.Add(-shopeeAPIAccessTokenSkew)) {
		return "access_expiring"
	}
	return "access_valid"
}

type shopeeAPIErrorView struct {
	Code      string
	Message   string
	Retryable bool
}

func respondShopeeAPIError(c *gin.Context, status int, err error, fallback string) {
	view := shopeeAPIErrorMessage(err, fallback)
	c.JSON(status, gin.H{
		"error":      view.Message,
		"error_code": view.Code,
		"retryable":  view.Retryable,
	})
}

func shopeeAPIErrorMessage(err error, fallback string) shopeeAPIErrorView {
	raw := strings.TrimSpace(fallback)
	if err != nil {
		raw = strings.TrimSpace(err.Error())
	}
	lower := strings.ToLower(raw)
	switch {
	case lower == "not configured" || strings.Contains(lower, "not configured") || strings.Contains(lower, "partner_id/key") || strings.Contains(lower, "ยังไม่ได้เปิดใช้งาน"):
		return shopeeAPIErrorView{Code: "not_configured", Message: "Shopee Open API ยังไม่ได้ตั้งค่า Partner ID/Key บน server"}
	case strings.Contains(lower, "redirect"):
		return shopeeAPIErrorView{Code: "redirect_not_ready", Message: "Redirect URL ยังไม่พร้อม ให้ตรวจ PUBLIC_BASE_URL และ Shopee Console ว่าตรงกัน"}
	case strings.Contains(lower, "ยังไม่ได้เชื่อมต่อร้าน"):
		return shopeeAPIErrorView{Code: "not_connected", Message: "ยังไม่ได้เชื่อมต่อร้าน Shopee ให้รอ Go-Live approve แล้วกดเชื่อมต่อ API"}
	case strings.Contains(lower, "wrong sign") || strings.Contains(lower, "error_sign") || strings.Contains(lower, "signature"):
		return shopeeAPIErrorView{Code: "bad_signature", Message: "Shopee ปฏิเสธ signature ให้ตรวจ Partner ID/Key และ sandbox/live base URL"}
	case strings.Contains(lower, "access_token") || strings.Contains(lower, "refresh token") || strings.Contains(lower, "token/get") || strings.Contains(lower, "access_token/get"):
		return shopeeAPIErrorView{Code: "token_error", Message: "Shopee token ใช้งานไม่ได้หรือหมดอายุ ให้กดเชื่อมต่อร้านใหม่"}
	case strings.Contains(lower, "permission") || strings.Contains(lower, "access denied") || strings.Contains(lower, "forbidden") || strings.Contains(lower, "not authorized"):
		return shopeeAPIErrorView{Code: "permission_denied", Message: "Shopee ยังไม่อนุญาตสิทธิ์นี้ ให้ตรวจสถานะ Go-Live และ permission ของแอป"}
	case strings.Contains(lower, "rate") || strings.Contains(lower, "too many") || strings.Contains(lower, "http 429"):
		return shopeeAPIErrorView{Code: "rate_limited", Message: "Shopee rate limit ให้รอสักครู่แล้วลองใหม่", Retryable: true}
	case strings.Contains(lower, "deadline") || strings.Contains(lower, "timeout") || strings.Contains(lower, "connection reset") || strings.Contains(lower, "temporary"):
		return shopeeAPIErrorView{Code: "network_timeout", Message: "เชื่อมต่อ Shopee ชั่วคราวไม่สำเร็จ ให้ลองใหม่อีกครั้ง", Retryable: true}
	}
	if raw == "" {
		raw = fallback
	}
	return shopeeAPIErrorView{Code: "unknown", Message: raw}
}

func (h *ShopeeImportHandler) shopeeAPIClient() *shopeeapi.Client {
	baseURL := h.cfg.ShopeeOpenAPIBaseURL
	if strings.TrimSpace(baseURL) == "" {
		if strings.EqualFold(h.cfg.ShopeeOpenAPIEnv, "live") {
			baseURL = shopeeapi.DefaultLiveBaseURL
		} else {
			baseURL = shopeeapi.DefaultSandboxBaseURL
		}
	}
	return shopeeapi.New(shopeeapi.Config{
		BaseURL:    baseURL,
		PartnerID:  h.cfg.ShopeeOpenAPIPartnerID,
		PartnerKey: h.cfg.ShopeeOpenAPIPartnerKey,
	})
}

func (h *ShopeeImportHandler) shopeeAPIRedirectURL() string {
	if strings.TrimSpace(h.cfg.ShopeeOpenAPIRedirect) != "" {
		return strings.TrimSpace(h.cfg.ShopeeOpenAPIRedirect)
	}
	base := strings.TrimRight(strings.TrimSpace(h.cfg.PublicBaseURL), "/")
	if base == "" {
		return ""
	}
	return base + "/api/shopee-api/callback"
}

func (h *ShopeeImportHandler) consumeShopeeOAuthState(ctx context.Context, state string) (*shopeeOAuthState, error) {
	var out shopeeOAuthState
	err := h.billRepo.DB().QueryRowContext(ctx,
		`UPDATE shopee_api_oauth_states
		    SET consumed_at = NOW()
		  WHERE state_hash = $1
		    AND consumed_at IS NULL
		    AND expires_at > NOW()
		  RETURNING user_id::text, environment, redirect_url`,
		hashState(state),
	).Scan(&out.UserID, &out.Environment, &out.RedirectURL)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (h *ShopeeImportHandler) consumeSolePendingShopeeOAuthState(ctx context.Context) (*shopeeOAuthState, error) {
	env := defaultShopeeAPIEnv(h.cfg.ShopeeOpenAPIEnv)
	redirectURL := h.shopeeAPIRedirectURL()
	if redirectURL == "" {
		return nil, fmt.Errorf("redirect URL is required")
	}

	var out shopeeOAuthState
	err := h.billRepo.DB().QueryRowContext(ctx,
		`WITH candidates AS (
		    SELECT state_hash, COUNT(*) OVER() AS candidate_count
		      FROM shopee_api_oauth_states
		     WHERE consumed_at IS NULL
		       AND expires_at > NOW()
		       AND environment = $1
		       AND redirect_url = $2
		     ORDER BY created_at DESC
		     LIMIT 2
		  ),
		  picked AS (
		    SELECT state_hash
		      FROM candidates
		     WHERE candidate_count = 1
		  )
		  UPDATE shopee_api_oauth_states AS s
		     SET consumed_at = NOW()
		    FROM picked
		   WHERE s.state_hash = picked.state_hash
		     AND s.consumed_at IS NULL
		   RETURNING s.user_id::text, s.environment, s.redirect_url`,
		env, redirectURL,
	).Scan(&out.UserID, &out.Environment, &out.RedirectURL)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (h *ShopeeImportHandler) getShopeeAPIConnection(ctx context.Context) (*ShopeeAPIConnection, error) {
	var c ShopeeAPIConnection
	err := h.billRepo.DB().QueryRowContext(ctx,
		`SELECT id::text, shop_id, shop_name, access_token, refresh_token,
		        access_expires_at, refresh_expires_at, environment,
		        last_sync_at, last_sync_status, last_sync_error
		   FROM shopee_api_connections
		  WHERE environment = $1
		  ORDER BY updated_at DESC
		  LIMIT 1`,
		defaultShopeeAPIEnv(h.cfg.ShopeeOpenAPIEnv),
	).Scan(
		&c.ID, &c.ShopID, &c.ShopName, &c.AccessToken, &c.RefreshToken,
		&c.AccessExpiresAt, &c.RefreshExpiresAt, &c.Environment,
		&c.LastSyncAt, &c.LastSyncStatus, &c.LastSyncError,
	)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (h *ShopeeImportHandler) upsertShopeeAPIConnection(ctx context.Context, shopID int64, accessToken, refreshToken string, accessExpires, refreshExpires time.Time, userID, env string) error {
	_, err := h.billRepo.DB().ExecContext(ctx,
		`INSERT INTO shopee_api_connections
		  (shop_id, access_token, refresh_token, access_expires_at, refresh_expires_at, environment, connected_by)
		 VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, '')::uuid)
		 ON CONFLICT (shop_id) DO UPDATE
		    SET access_token = EXCLUDED.access_token,
		        refresh_token = EXCLUDED.refresh_token,
		        access_expires_at = EXCLUDED.access_expires_at,
		        refresh_expires_at = EXCLUDED.refresh_expires_at,
		        environment = EXCLUDED.environment,
		        connected_by = EXCLUDED.connected_by,
		        connected_at = NOW(),
		        updated_at = NOW(),
		        last_sync_status = '',
		        last_sync_error = ''`,
		shopID, accessToken, refreshToken, accessExpires, refreshExpires,
		defaultShopeeAPIEnv(env), userID,
	)
	return err
}

func (h *ShopeeImportHandler) ensureShopeeAPIAccessToken(ctx context.Context) (*ShopeeAPIConnection, error) {
	status := h.shopeeAPIStatus()
	if !status.Enabled || !status.Configured {
		return nil, fmt.Errorf("Shopee Open API ยังไม่ได้เปิดใช้งานหรือตั้งค่า partner_id/key")
	}
	conn, err := h.getShopeeAPIConnection(ctx)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("ยังไม่ได้เชื่อมต่อร้าน Shopee API")
		}
		return nil, err
	}
	if time.Now().Before(conn.AccessExpiresAt.Add(-shopeeAPIAccessTokenSkew)) {
		return conn, nil
	}
	tok, err := h.shopeeAPIClient().RefreshToken(ctx, conn.RefreshToken, conn.ShopID)
	if err != nil {
		return nil, err
	}
	conn.AccessToken = tok.AccessToken
	conn.RefreshToken = tok.RefreshToken
	conn.AccessExpiresAt = time.Now().Add(time.Duration(tok.ExpireIn) * time.Second)
	if tok.ExpireIn <= 0 {
		conn.AccessExpiresAt = time.Now().Add(4 * time.Hour)
	}
	conn.RefreshExpiresAt = time.Now().Add(shopeeAPIRefreshTokenTTL)
	_, err = h.billRepo.DB().ExecContext(ctx,
		`UPDATE shopee_api_connections
		    SET access_token = $2,
		        refresh_token = $3,
		        access_expires_at = $4,
		        refresh_expires_at = $5,
		        last_refreshed_at = NOW(),
		        updated_at = NOW()
		  WHERE shop_id = $1`,
		conn.ShopID, conn.AccessToken, conn.RefreshToken, conn.AccessExpiresAt, conn.RefreshExpiresAt,
	)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func (h *ShopeeImportHandler) markShopeeAPISync(ctx context.Context, shopID *int64, status, msg string) {
	if shopID == nil {
		return
	}
	if len(msg) > 500 {
		msg = msg[:500]
	}
	_, err := h.billRepo.DB().ExecContext(ctx,
		`UPDATE shopee_api_connections
		    SET last_sync_at = NOW(),
		        last_sync_status = $2,
		        last_sync_error = $3,
		        updated_at = NOW()
		  WHERE shop_id = $1`,
		*shopID, status, msg,
	)
	if err != nil {
		h.logger.Warn("shopee_api: mark sync failed", zap.Error(err))
	}
}

func (h *ShopeeImportHandler) shopeeAPIOrdersToPreview(details []shopeeapi.OrderDetail) ([]ShopeeOrder, []string) {
	orders := make([]ShopeeOrder, 0, len(details))
	warnings := []string{}
	for _, d := range details {
		orderSN := strings.TrimSpace(d.OrderSN)
		if orderSN == "" {
			continue
		}
		docTime := shopeeUnixTime(d.CreateTime)
		payTime := shopeeUnixTime(d.PayTime)
		docDate := time.Now().Format("2006-01-02")
		orderDateTime := ""
		if !docTime.IsZero() {
			docDate = docTime.Format("2006-01-02")
			orderDateTime = docTime.Format(time.RFC3339)
		}
		items := make([]ShopeeExcelItem, 0, len(d.ItemList))
		var totalQty, gross float64
		noSKUCount := 0
		for _, item := range d.ItemList {
			qty := item.ModelQuantityPurchased
			if qty <= 0 {
				qty = 1
			}
			price := item.ModelDiscountedPrice
			if price <= 0 {
				price = item.ModelOriginalPrice
			}
			sku := strings.TrimSpace(item.ModelSKU)
			if sku == "" {
				sku = strings.TrimSpace(item.ItemSKU)
			}
			rawName := shopeeItemRawName(item.ItemName, item.ModelName, "")
			noSKU := sku == ""
			if noSKU {
				noSKUCount++
			}
			items = append(items, ShopeeExcelItem{
				SKU:         sku,
				OrderItemID: strconv.FormatInt(item.ItemID, 10),
				ProductName: strings.TrimSpace(item.ItemName),
				OptionName:  strings.TrimSpace(item.ModelName),
				RawName:     rawName,
				Price:       price,
				Qty:         qty,
				NoSKU:       noSKU,
			})
			totalQty += qty
			gross += price * qty
		}
		if len(items) == 0 {
			warnings = append(warnings, fmt.Sprintf("Order %s: ไม่มีสินค้าใน response — ข้ามไป", orderSN))
			continue
		}
		paymentTime := ""
		if !payTime.IsZero() {
			paymentTime = payTime.Format(time.RFC3339)
		}
		paid := d.TotalAmount
		orders = append(orders, ShopeeOrder{
			OrderID:          orderSN,
			DocDate:          docDate,
			OrderDateTime:    orderDateTime,
			PaymentTime:      paymentTime,
			PaymentChannel:   d.PaymentMethod,
			BuyerUsername:    d.BuyerUsername,
			TrackingNo:       d.TrackingNumber,
			Status:           d.OrderStatus,
			Items:            items,
			ItemCount:        len(items),
			TotalQty:         totalQty,
			PaidAmount:       paid,
			OrderTotalAmount: paid,
			ItemGrossAmount:  gross,
			LinePaidAmount:   paid,
			NoSKUItemCount:   noSKUCount,
			HasNoSKU:         noSKUCount > 0,
			MultiLine:        len(items) > 1,
			AmountMismatch:   paid > 0 && math.Abs(gross-paid) > 0.01,
		})
	}
	if len(orders) == 0 {
		warnings = append(warnings, "ไม่พบ order ที่นำเข้าได้จากช่วงวันที่นี้")
	}
	return orders, warnings
}

func parseShopeeAPIRange(fromRaw, toRaw string) (time.Time, time.Time, error) {
	now := time.Now()
	to := time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 0, now.Location())
	from := to.AddDate(0, 0, -7)
	var err error
	if strings.TrimSpace(fromRaw) != "" {
		from, err = time.ParseInLocation("2006-01-02", strings.TrimSpace(fromRaw), now.Location())
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("time_from ต้องเป็น YYYY-MM-DD")
		}
	}
	if strings.TrimSpace(toRaw) != "" {
		parsed, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(toRaw), now.Location())
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("time_to ต้องเป็น YYYY-MM-DD")
		}
		to = time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 23, 59, 59, 0, parsed.Location())
	}
	if to.Before(from) {
		return time.Time{}, time.Time{}, fmt.Errorf("time_to ต้องมากกว่าหรือเท่ากับ time_from")
	}
	if to.Sub(from) > shopeeAPIMaxRange {
		return time.Time{}, time.Time{}, fmt.Errorf("Shopee API จำกัดช่วงเวลาดึง order ไม่เกิน 15 วันต่อครั้ง")
	}
	return from, to, nil
}

func randomState() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}

func hashState(state string) string {
	sum := sha256.Sum256([]byte(state))
	return hex.EncodeToString(sum[:])
}

func shopeeUnixTime(v int64) time.Time {
	if v <= 0 {
		return time.Time{}
	}
	return time.Unix(v, 0)
}

func shopeeAPIOrderDetailFields() []string {
	return []string{
		"buyer_username",
		"recipient_address",
		"item_list",
		"pay_time",
		"create_time",
		"update_time",
		"total_amount",
		"payment_method",
		"tracking_number",
		"order_status",
	}
}

func defaultShopeeAPITimeField(v string) string {
	switch strings.TrimSpace(v) {
	case "pay_time", "update_time", "create_time":
		return v
	default:
		return "create_time"
	}
}

func defaultShopeeAPIEnv(v string) string {
	if strings.EqualFold(strings.TrimSpace(v), "live") {
		return "live"
	}
	return "sandbox"
}

func (h *ShopeeImportHandler) renderShopeeCallback(c *gin.Context, status int, title, message string) {
	body, _ := json.Marshal(message)
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(status, `<!doctype html>
<html lang="th">
<head><meta charset="utf-8"><title>BillFlow Shopee</title></head>
<body style="font-family: system-ui, sans-serif; padding: 32px; line-height: 1.5">
<h1>%s</h1>
<p id="msg"></p>
<p>คุณสามารถปิดหน้านี้ แล้วกลับไปที่ BillFlow ได้</p>
<script>document.getElementById('msg').textContent = %s;</script>
</body>
</html>`, title, string(body))
}
