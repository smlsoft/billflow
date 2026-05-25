package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"billflow/internal/models"
	"billflow/internal/repository"
	"billflow/internal/services/sml"
)

// SMLPartyHandler proxies the SML 248 party master through an in-memory cache.
// Admin-only — used by /settings/channels picker and any future supplier UI.
type SMLPartyHandler struct {
	cache      *sml.PartyCache
	client     *sml.PartyClient
	auditRepo  *repository.AuditLogRepo
	smlBaseURL string // sml-api-bybos base URL (e.g. http://192.168.2.109:8200)
	smlGUID    string // API key for sml-api-bybos (used as guid header)
	smlTenant  string // database/tenant name for sml-api-bybos (X-Tenant header)
	logger     *zap.Logger
}

func NewSMLPartyHandler(cache *sml.PartyCache, client *sml.PartyClient, auditRepo *repository.AuditLogRepo, logger *zap.Logger) *SMLPartyHandler {
	return &SMLPartyHandler{cache: cache, client: client, auditRepo: auditRepo, logger: logger}
}

// SetSMLConfig injects the sml-api-bybos connection details needed for
// endpoints that call sml-api-bybos directly (e.g. doc-formats).
func (h *SMLPartyHandler) SetSMLConfig(baseURL, guid, tenant string) {
	h.smlBaseURL = strings.TrimRight(baseURL, "/")
	h.smlGUID = guid
	h.smlTenant = tenant
}

// GET /api/sml/customers?search=&limit=20
func (h *SMLPartyHandler) SearchCustomers(c *gin.Context) {
	h.search(c, "sale")
}

// GET /api/sml/suppliers?search=&limit=20
func (h *SMLPartyHandler) SearchSuppliers(c *gin.Context) {
	h.search(c, "purchase")
}

type createPartyRequest struct {
	Code  string `json:"code" binding:"required"`
	Name1 string `json:"name_1" binding:"required"`
}

func (h *SMLPartyHandler) search(c *gin.Context, billType string) {
	if h.cache == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "party cache not configured"})
		return
	}
	q := c.Query("search")
	limit := 20
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}
	results := h.cache.Search(billType, q, limit)
	status := h.cache.Status()
	total := status.Customers
	if billType == "purchase" {
		total = status.Suppliers
	}
	c.JSON(http.StatusOK, gin.H{
		"data":         results,
		"total":        total,
		"last_sync":    nullableTime(status.LastSync),
		"last_attempt": nullableTime(status.LastAttempt),
		"status":       status.Status,
		"error":        status.Error,
	})
}

func (h *SMLPartyHandler) CreateCustomer(c *gin.Context) {
	h.create(c, "sale")
}

func (h *SMLPartyHandler) CreateSupplier(c *gin.Context) {
	h.create(c, "purchase")
}

func (h *SMLPartyHandler) create(c *gin.Context, billType string) {
	if h.client == nil || !h.client.IsConfigured() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "SML party client not configured"})
		return
	}
	var req createPartyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.Code = strings.TrimSpace(req.Code)
	req.Name1 = strings.TrimSpace(req.Name1)
	if req.Code == "" || req.Name1 == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "code and name_1 are required"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	var party *sml.Party
	var statusCode int
	var err error
	if billType == "purchase" {
		statusCode, party, err = h.client.CreateSupplier(ctx, req.Code, req.Name1)
	} else {
		statusCode, party, err = h.client.CreateCustomer(ctx, req.Code, req.Name1)
	}
	if err != nil {
		if statusCode == http.StatusConflict {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error(), "code": req.Code})
			return
		}
		h.logger.Warn("sml_party_create_failed",
			zap.String("bill_type", billType),
			zap.String("code", req.Code),
			zap.Int("status_code", statusCode),
			zap.Error(err),
		)
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	if party == nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "SML returned empty party"})
		return
	}
	if h.cache != nil {
		h.cache.Upsert(billType, *party)
	}
	if h.auditRepo != nil {
		var userID *string
		if uid := c.GetString("user_id"); uid != "" {
			userID = &uid
		}
		action := "sml_customer_created"
		if billType == "purchase" {
			action = "sml_supplier_created"
		}
		_ = h.auditRepo.Log(models.AuditEntry{
			Action:  action,
			UserID:  userID,
			Source:  "ui",
			Level:   "info",
			TraceID: c.GetString("trace_id"),
			Detail: map[string]interface{}{
				"code": party.Code,
				"name": party.Name,
			},
		})
	}
	c.JSON(http.StatusCreated, gin.H{"party": party})
}

// POST /api/sml/refresh-parties — re-fetch both lists from SML.
func (h *SMLPartyHandler) Refresh(c *gin.Context) {
	if h.cache == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "party cache not configured"})
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := h.cache.RefreshNow(ctx); err != nil {
		status := h.cache.Status()
		c.JSON(http.StatusBadGateway, gin.H{
			"error":        "ดึงรายชื่อลูกค้า/ผู้ขายจาก SML ไม่สำเร็จ: " + err.Error(),
			"customers":    status.Customers,
			"suppliers":    status.Suppliers,
			"last_sync":    nullableTime(status.LastSync),
			"last_attempt": nullableTime(status.LastAttempt),
			"status":       status.Status,
		})
		return
	}
	status := h.cache.Status()
	c.JSON(http.StatusOK, gin.H{
		"customers":    status.Customers,
		"suppliers":    status.Suppliers,
		"last_sync":    nullableTime(status.LastSync),
		"last_attempt": nullableTime(status.LastAttempt),
		"status":       status.Status,
	})
}

// GET /api/sml/parties/last-sync
func (h *SMLPartyHandler) LastSync(c *gin.Context) {
	if h.cache == nil {
		c.JSON(http.StatusOK, gin.H{
			"customers":    0,
			"suppliers":    0,
			"last_sync":    nil,
			"last_attempt": nil,
			"status":       "not_configured",
		})
		return
	}
	status := h.cache.Status()
	c.JSON(http.StatusOK, gin.H{
		"customers":    status.Customers,
		"suppliers":    status.Suppliers,
		"last_sync":    nullableTime(status.LastSync),
		"last_attempt": nullableTime(status.LastAttempt),
		"status":       status.Status,
		"error":        status.Error,
	})
}

func nullableTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}

// DocFormatItem mirrors the erp_doc_format row returned by sml-api-bybos.
type DocFormatItem struct {
	Code       string `json:"code"`
	Name1      string `json:"name_1"`
	Name2      string `json:"name_2"`
	Format     string `json:"format"`
	ScreenCode string `json:"screen_code"`
}

// GET /api/sml/doc-formats?screen_code=PO|SI|SR
// Proxies to sml-api-bybos GET /api/v1/ic/doc-formats?screen_code=<code>.
// screen_code: PO=ใบสั่งซื้อ, SI=ขายสินค้าและบริการ, SR=ใบสั่งขาย
func (h *SMLPartyHandler) DocFormats(c *gin.Context) {
	if h.smlBaseURL == "" || h.smlGUID == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "SML REST URL ยังไม่ได้ตั้งค่า"})
		return
	}
	screenCode := strings.ToUpper(strings.TrimSpace(c.Query("screen_code")))

	targetURL := h.smlBaseURL + "/api/v1/ic/doc-formats"
	if screenCode != "" {
		targetURL += "?screen_code=" + screenCode
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	req.Header.Set("x-api-key", h.smlGUID)
	if h.smlTenant != "" {
		req.Header.Set("x-tenant", h.smlTenant)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("เรียก SML ไม่สำเร็จ: %v", err)})
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var result struct {
		Success bool            `json:"success"`
		Data    []DocFormatItem `json:"data"`
		Message string          `json:"message"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "parse SML response failed"})
		return
	}
	if !result.Success {
		c.JSON(http.StatusBadGateway, gin.H{"error": result.Message})
		return
	}
	if result.Data == nil {
		result.Data = []DocFormatItem{}
	}
	c.JSON(http.StatusOK, gin.H{"data": result.Data})
}
