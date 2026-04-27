package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"billflow/internal/models"
	"billflow/internal/repository"
	"billflow/internal/services/catalog"
	"billflow/internal/services/sml"
)

// CatalogHandler serves /api/catalog/* endpoints
type CatalogHandler struct {
	catalogSvc    *catalog.SMLCatalogService
	embSvc        *catalog.EmbeddingService
	catalogIdx    *catalog.CatalogIndex
	catalogRepo   *repository.SMLCatalogRepo
	productClient *sml.ProductClient
	auditRepo     *repository.AuditLogRepo
	logger        *zap.Logger
	threshold     float64 // auto-confirm threshold
}

func NewCatalogHandler(
	svc *catalog.SMLCatalogService,
	emb *catalog.EmbeddingService,
	idx *catalog.CatalogIndex,
	repo *repository.SMLCatalogRepo,
	productClient *sml.ProductClient,
	auditRepo *repository.AuditLogRepo,
	threshold float64,
	logger *zap.Logger,
) *CatalogHandler {
	return &CatalogHandler{
		catalogSvc:    svc,
		embSvc:        emb,
		catalogIdx:    idx,
		catalogRepo:   repo,
		productClient: productClient,
		auditRepo:     auditRepo,
		threshold:     threshold,
		logger:        logger,
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

// ─── Create new product ──────────────────────────────────────────────────────

// createProductRequest is the body the frontend sends. It's a compact "quick
// form" — only the minimum required for SML to accept the product.
type createProductRequest struct {
	Code     string  `json:"code" binding:"required"`     // SML item code (user-supplied)
	Name     string  `json:"name" binding:"required"`     // Product name (Thai or English)
	UnitCode string  `json:"unit_code" binding:"required"` // e.g. "ชิ้น", "ถุง"
	Price    float64 `json:"price"`                       // per-unit selling price (>= 0)
	WHCode   string  `json:"wh_code,omitempty"`           // optional default warehouse
	ShelfCode string `json:"shelf_code,omitempty"`        // optional default shelf
}

// POST /api/catalog/products — quick-create a product in SML and sync to local catalog.
//
// Flow:
//  1. Pre-check: reject if item_code already exists in local sml_catalog
//     (saves a round-trip to SML for an obvious duplicate).
//  2. Call SML POST /SMLJavaRESTService/v3/api/product. SML may return its
//     own assigned code in response.data.code (overrides the requested one).
//  3. Upsert into sml_catalog with status='pending' so embed runs.
//  4. Trigger embedding in background (non-blocking) + reload index.
//  5. Audit log.
//
// Returns the canonical code (from SML response) so the frontend can fill
// the bill_item with it.
func (h *CatalogHandler) CreateProduct(c *gin.Context) {
	if h.productClient == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "product client not configured"})
		return
	}

	var req createProductRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	req.Code = strings.TrimSpace(req.Code)
	req.Name = strings.TrimSpace(req.Name)
	req.UnitCode = strings.TrimSpace(req.UnitCode)
	if req.Code == "" || req.Name == "" || req.UnitCode == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "code, name, unit_code are required"})
		return
	}

	// 1. Local dup-check — fast fail before SML round-trip
	existing, _ := h.catalogRepo.GetOne(req.Code)
	if existing != nil {
		c.JSON(http.StatusConflict, gin.H{
			"error":   "product code already exists",
			"code":    req.Code,
			"existing": existing,
		})
		return
	}

	// 2. Build SML payload — defaults pulled from the request example
	priceStr := strconv.FormatFloat(req.Price, 'f', -1, 64)
	smlReq := sml.CreateProductRequest{
		Code:         req.Code,
		Name:         req.Name,
		TaxType:      0, // VAT แยกนอก (matches Shopee saleinvoice default)
		ItemType:     0, // สินค้าทั่วไป
		UnitType:     1,
		UnitCost:     req.UnitCode,
		UnitStandard: req.UnitCode,
		PurchasePoint: 0,
		Units: []sml.ProductUnit{
			{UnitCode: req.UnitCode, UnitName: req.UnitCode, StandValue: 1, DivideValue: 1},
		},
		PriceFormulas: []sml.ProductPriceFormula{
			{UnitCode: req.UnitCode, SaleType: 0, Price0: priceStr, TaxType: 0, PriceCurrency: 0},
		},
	}

	// 3. POST to SML
	statusCode, smlResp, err := h.productClient.CreateProduct(smlReq)
	if err != nil || smlResp == nil || !smlResp.Success {
		errMsg := ""
		switch {
		case err != nil:
			errMsg = err.Error()
		case smlResp != nil && smlResp.Message != "":
			errMsg = fmt.Sprintf("SML rejected (HTTP %d): %s", statusCode, smlResp.Message)
		default:
			errMsg = fmt.Sprintf("SML rejected (HTTP %d)", statusCode)
		}
		h.logger.Warn("create_product: SML failed", zap.String("code", req.Code), zap.String("error", errMsg))
		c.JSON(http.StatusBadGateway, gin.H{"error": errMsg})
		return
	}

	// 4. Use canonical code from SML response (may differ from request)
	finalCode := smlResp.Data.Code
	if finalCode == "" {
		finalCode = req.Code
	}

	// 5. Upsert into local catalog with status='pending' — embed will fill later
	priceVal := req.Price
	whCode := req.WHCode
	shelfCode := req.ShelfCode
	if err := h.catalogRepo.Upsert(models.CatalogItem{
		ItemCode:        finalCode,
		ItemName:        req.Name,
		UnitCode:        req.UnitCode,
		WHCode:          whCode,
		ShelfCode:       shelfCode,
		Price:           &priceVal,
		EmbeddingStatus: "pending",
	}); err != nil {
		h.logger.Error("create_product: catalog upsert failed",
			zap.String("code", finalCode), zap.Error(err))
		// SML already accepted — return success, just log the local-sync miss
	}

	// 6. Trigger embedding in background (non-blocking); reload index after
	go func(code string) {
		if !h.embSvc.IsConfigured() {
			return
		}
		if err := h.catalogSvc.EmbedProduct(h.embSvc, code); err != nil {
			h.logger.Warn("create_product: embed failed",
				zap.String("code", code), zap.Error(err))
			return
		}
		if err := h.catalogIdx.Reload(h.catalogRepo); err != nil {
			h.logger.Warn("create_product: index reload failed", zap.Error(err))
		}
	}(finalCode)

	// 7. Audit log
	if h.auditRepo != nil {
		_ = h.auditRepo.Log(models.AuditEntry{
			Action: "product_created",
			Source: "ui",
			Level:  "info",
			Detail: map[string]interface{}{
				"requested_code": req.Code,
				"final_code":     finalCode,
				"name":           req.Name,
				"unit_code":      req.UnitCode,
				"price":          req.Price,
			},
		})
	}

	c.JSON(http.StatusCreated, gin.H{
		"code":      finalCode,
		"name":      req.Name,
		"unit_code": req.UnitCode,
		"wh_code":   whCode,
		"shelf_code": shelfCode,
		"message":   "product created and queued for embedding",
	})
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
