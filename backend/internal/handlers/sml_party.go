package handlers

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"billflow/internal/services/sml"
)

// SMLPartyHandler proxies the SML 248 party master through an in-memory cache.
// Admin/staff SML party lookup for the per-bill SML send dialogs.
type SMLPartyHandler struct {
	cache  *sml.PartyCache
	logger *zap.Logger
}

func NewSMLPartyHandler(cache *sml.PartyCache, logger *zap.Logger) *SMLPartyHandler {
	return &SMLPartyHandler{cache: cache, logger: logger}
}

// GET /api/sml/customers?search=&limit=20
func (h *SMLPartyHandler) SearchCustomers(c *gin.Context) {
	h.search(c, "sale")
}

// GET /api/sml/suppliers?search=&limit=20
func (h *SMLPartyHandler) SearchSuppliers(c *gin.Context) {
	h.search(c, "purchase")
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
