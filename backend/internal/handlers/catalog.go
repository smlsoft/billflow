package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"billflow/internal/models"
	"billflow/internal/repository"
	"billflow/internal/services/catalog"
)

// CatalogHandler serves /api/catalog/* endpoints
type CatalogHandler struct {
	catalogSvc  *catalog.SMLCatalogService
	embSvc      *catalog.EmbeddingService
	catalogIdx  *catalog.CatalogIndex
	catalogRepo *repository.SMLCatalogRepo
	logger      *zap.Logger
	threshold   float64 // auto-confirm threshold
}

func NewCatalogHandler(
	svc *catalog.SMLCatalogService,
	emb *catalog.EmbeddingService,
	idx *catalog.CatalogIndex,
	repo *repository.SMLCatalogRepo,
	threshold float64,
	logger *zap.Logger,
) *CatalogHandler {
	return &CatalogHandler{
		catalogSvc:  svc,
		embSvc:      emb,
		catalogIdx:  idx,
		catalogRepo: repo,
		threshold:   threshold,
		logger:      logger,
	}
}

// GET /api/catalog
func (h *CatalogHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "50"))
	status := c.Query("status") // "pending" | "done" | "error" | ""
	q := c.Query("q")          // free-text search on item_code / item_name
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 200 {
		perPage = 50
	}

	items, total, err := h.catalogRepo.List(page, perPage, status, q)
	if err != nil {
		h.logger.Error("catalog list", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"data":     items,
		"total":    total,
		"page":     page,
		"per_page": perPage,
	})
}

// GET /api/catalog/stats
func (h *CatalogHandler) Stats(c *gin.Context) {
	total, done, pending, errCount, err := h.catalogRepo.Stats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"total":         total,
		"embedded":      done,
		"pending":       pending,
		"error":         errCount,
		"index_size":    h.catalogIdx.Size(),
		"embed_running": h.catalogSvc.IsEmbedRunning(),
	})
}

// POST /api/catalog/sync  — sync from SML REST API
func (h *CatalogHandler) SyncFromAPI(c *gin.Context) {
	count, err := h.catalogSvc.SyncFromAPI()
	if err != nil {
		h.logger.Error("catalog sync", zap.Error(err))
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"synced": count, "message": fmt.Sprintf("synced %d items from SML", count)})
}

// POST /api/catalog/import-csv  — upload CSV file
func (h *CatalogHandler) ImportCSV(c *gin.Context) {
	file, _, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file required"})
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "read file: " + err.Error()})
		return
	}

	count, err := h.catalogSvc.SyncFromCSV(data)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"imported": count, "message": fmt.Sprintf("imported %d items from CSV", count)})
}

// POST /api/catalog/:code/embed  — embed a single product
func (h *CatalogHandler) EmbedOne(c *gin.Context) {
	code := c.Param("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "item code required"})
		return
	}
	if !h.embSvc.IsConfigured() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "GEMINI_API_KEY not configured"})
		return
	}
	if err := h.catalogSvc.EmbedProduct(h.embSvc, code); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// Reload index
	if err := h.catalogIdx.Reload(h.catalogRepo); err != nil {
		h.logger.Warn("catalog: reload index after single embed", zap.Error(err))
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// POST /api/catalog/embed-all  — background embed all pending items
func (h *CatalogHandler) EmbedAll(c *gin.Context) {
	if !h.embSvc.IsConfigured() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "GEMINI_API_KEY not configured"})
		return
	}
	if h.catalogSvc.IsEmbedRunning() {
		c.JSON(http.StatusConflict, gin.H{"error": "embedding already running"})
		return
	}

	// Run in background goroutine
	go func() {
		done, errs, err := h.catalogSvc.EmbedAllPending(h.embSvc)
		if err != nil {
			h.logger.Error("catalog: embed-all background", zap.Error(err))
		}
		// Reload memory index after embedding
		if err := h.catalogIdx.Reload(h.catalogRepo); err != nil {
			h.logger.Warn("catalog: reload index after embed-all", zap.Error(err))
		}
		h.logger.Info("catalog: embed-all done", zap.Int("done", done), zap.Int("errors", errs))
	}()

	c.JSON(http.StatusAccepted, gin.H{"message": "embedding started in background"})
}

// POST /api/catalog/reload-index  — manually reload in-memory index
func (h *CatalogHandler) ReloadIndex(c *gin.Context) {
	if err := h.catalogIdx.Reload(h.catalogRepo); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"size": h.catalogIdx.Size()})
}

// GET /api/catalog/search?q=...&top=5  — similarity search (for testing)
func (h *CatalogHandler) Search(c *gin.Context) {
	q := c.Query("q")
	if q == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "q required"})
		return
	}
	top, _ := strconv.Atoi(c.DefaultQuery("top", "5"))
	if top < 1 || top > 20 {
		top = 5
	}

	var results []models.CatalogMatch
	var method string

	if h.embSvc.IsConfigured() && h.catalogIdx.Size() > 0 {
		// Embedding search
		queryEmb, err := h.embSvc.EmbedText(q)
		if err == nil {
			results = h.catalogIdx.Search(queryEmb, top)
			method = "embedding"
		} else {
			h.logger.Warn("catalog search: embed query failed, fallback to text", zap.Error(err))
		}
	}

	if len(results) == 0 {
		// Fallback: text similarity
		var err error
		results, err = h.catalogSvc.SearchByText(q, top)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		method = "text"
	}

	c.JSON(http.StatusOK, gin.H{
		"query":   q,
		"method":  method,
		"results": results,
	})
}

// GET /api/catalog/:code  — get single product detail
func (h *CatalogHandler) GetOne(c *gin.Context) {
	code := c.Param("code")
	item, err := h.catalogRepo.GetOne(code)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if item == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	c.JSON(http.StatusOK, item)
}

// POST /api/bills/:id/items/:item_id/confirm-match
// Body: {"item_code": "...", "unit_code": "...", "wh_code": "...", "shelf_code": "..."}
func (h *CatalogHandler) ConfirmMatch(c *gin.Context) {
	billID := c.Param("id")
	itemID := c.Param("item_id")
	if billID == "" || itemID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bill_id and item_id required"})
		return
	}

	var req struct {
		ItemCode  string `json:"item_code" binding:"required"`
		UnitCode  string `json:"unit_code"`
		WHCode    string `json:"wh_code"`
		ShelfCode string `json:"shelf_code"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Look up catalog item for defaults
	catalogItem, _ := h.catalogRepo.GetOne(req.ItemCode)
	unitCode := req.UnitCode
	whCode := req.WHCode
	shelfCode := req.ShelfCode
	if catalogItem != nil {
		if unitCode == "" {
			unitCode = catalogItem.UnitCode
		}
		if whCode == "" {
			whCode = catalogItem.WHCode
		}
		if shelfCode == "" {
			shelfCode = catalogItem.ShelfCode
		}
	}

	// Update bill_item
	db := h.catalogRepo.DB()
	_, err := db.Exec(`
		UPDATE bill_items
		SET item_code = $1, unit_code = $2, mapped = TRUE
		WHERE id = $3 AND bill_id = $4
	`, req.ItemCode, unitCode, itemID, billID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Check if all items in this bill are now mapped
	var unmapped int
	_ = db.QueryRow(`
		SELECT COUNT(*) FROM bill_items WHERE bill_id = $1 AND mapped = FALSE
	`, billID).Scan(&unmapped)

	allConfirmed := unmapped == 0

	// Build response
	resp := gin.H{
		"ok":            true,
		"all_confirmed": allConfirmed,
		"item_code":     req.ItemCode,
		"unit_code":     unitCode,
		"wh_code":       whCode,
		"shelf_code":    shelfCode,
	}

	// Optionally store wh/shelf back to bill_item (for SML payload)
	if whCode != "" || shelfCode != "" {
		_, _ = db.Exec(`
			UPDATE bill_items
			SET unit_code = $1
			WHERE id = $2
		`, unitCode, itemID)

		// Store wh/shelf in candidates field as confirmed metadata
		meta, _ := json.Marshal(map[string]string{
			"confirmed_wh":    whCode,
			"confirmed_shelf": shelfCode,
		})
		_, _ = db.Exec(`
			UPDATE bill_items SET candidates = $1 WHERE id = $2
		`, meta, itemID)
	}

	c.JSON(http.StatusOK, resp)
}
