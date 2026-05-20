package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"billflow/internal/config"
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
	appSettings   *repository.AppSettingsRepo
	cfg           *config.Config
	logger        *zap.Logger
	threshold     float64 // auto-confirm threshold
}

type catalogImageMeta struct {
	Roworder   int    `json:"roworder"`
	ImageOrder int    `json:"image_order"`
	Guid       string `json:"guid"`
	Bytes      int64  `json:"bytes"`
	ImageURL   string `json:"image_url"`
}

func NewCatalogHandler(
	svc *catalog.SMLCatalogService,
	emb *catalog.EmbeddingService,
	idx *catalog.CatalogIndex,
	repo *repository.SMLCatalogRepo,
	productClient *sml.ProductClient,
	auditRepo *repository.AuditLogRepo,
	appSettings *repository.AppSettingsRepo,
	cfg *config.Config,
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
		appSettings:   appSettings,
		cfg:           cfg,
		threshold:     threshold,
		logger:        logger,
	}
}

// GET /api/catalog
func (h *CatalogHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "50"))
	status := c.Query("status") // "pending" | "done" | "error" | ""
	q := c.Query("q")           // free-text search on item_code / item_name
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
	attachCatalogImageURLs(items)
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
	syncStatus := h.catalogSvc.SyncStatus()
	embedStatus := h.catalogSvc.EmbedStatus()
	c.JSON(http.StatusOK, gin.H{
		"total":         total,
		"embedded":      done,
		"pending":       pending,
		"error":         errCount,
		"index_size":    h.catalogIdx.Size(),
		"embed_running": h.catalogSvc.IsEmbedRunning(),
		"embed_status":  embedStatus,
		"sync_running":  syncStatus.Running,
		"sync_status":   syncStatus,
	})
}

// POST /api/catalog/sync  — sync from SML REST API
func (h *CatalogHandler) SyncFromAPI(c *gin.Context) {
	if h.hasPendingRestart(c) {
		return
	}
	if !h.catalogSvc.BeginSync() {
		c.JSON(http.StatusAccepted, gin.H{"message": "catalog sync already running", "sync_running": true})
		return
	}
	go func() {
		h.logger.Info("catalog: sync from API started")
		count, err := h.catalogSvc.SyncFromAPI()
		if err != nil {
			h.logger.Error("catalog sync", zap.Error(err))
		}
		if err == nil {
			// Reload in-memory index so /api/catalog/search reflects the new rows
			// without waiting for the next embed batch.
			if reloadErr := h.catalogIdx.Reload(h.catalogRepo); reloadErr != nil {
				h.logger.Warn("catalog: reload index after sync", zap.Error(reloadErr))
			}
		}
		h.catalogSvc.FinishSync(count, err)
	}()
	c.JSON(http.StatusAccepted, gin.H{"message": "catalog sync started", "sync_running": true})
}

func (h *CatalogHandler) hasPendingRestart(c *gin.Context) bool {
	if h.appSettings == nil || h.cfg == nil {
		return false
	}
	pending, keys, err := h.appSettings.PendingRestart(h.cfg)
	if err != nil {
		h.logger.Warn("catalog: check pending restart", zap.Error(err))
		return false
	}
	if !pending {
		return false
	}
	c.JSON(http.StatusConflict, gin.H{
		"error":                    "มีการเปลี่ยนค่า SML/AI ที่ยังไม่ได้เริ่มใช้ กรุณากดรีสตาร์ท backend ในหน้าการเชื่อมต่อระบบก่อน Sync สินค้า",
		"pending_restart":          true,
		"pending_restart_settings": keys,
	})
	return true
}

// POST /api/catalog/:code/refresh — refresh a single row from SML 248
// Used by the per-row "รีเฟรช" button on /settings/catalog.
func (h *CatalogHandler) RefreshOne(c *gin.Context) {
	code := c.Param("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "item code required"})
		return
	}
	item, notFound, err := h.catalogSvc.RefreshOne(code)
	if err != nil {
		h.logger.Error("catalog refresh one", zap.String("code", code), zap.Error(err))
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	if notFound {
		c.JSON(http.StatusNotFound, gin.H{
			"error":     "ไม่พบสินค้า " + code + " ใน SML — อาจถูกลบจาก SML แล้ว",
			"not_found": true,
		})
		return
	}
	if err := h.catalogIdx.Reload(h.catalogRepo); err != nil {
		h.logger.Warn("catalog: reload index after refresh", zap.Error(err))
	}
	if item != nil {
		item.ImageURL = catalogImageURL(item.ItemCode, item.ImageCount, item.PrimaryImageRoworder)
	}
	if h.auditRepo != nil {
		var userID *string
		if uid := c.GetString("user_id"); uid != "" {
			userID = &uid
		}
		_ = h.auditRepo.Log(models.AuditEntry{
			Action:  "catalog_refresh_one",
			UserID:  userID,
			Source:  "catalog",
			Level:   "info",
			TraceID: c.GetString("trace_id"),
			Detail:  map[string]interface{}{"item_code": code, "item_name": item.ItemName},
		})
	}
	c.JSON(http.StatusOK, gin.H{"item": item, "message": "refreshed from SML"})
}

// DELETE /api/catalog/:code — delete a single row from BillFlow's catalog.
// SML 248 is NOT touched — this is for pruning local zombies left over after
// an SML-side delete, or for clearing rows the admin doesn't want matched.
func (h *CatalogHandler) DeleteOne(c *gin.Context) {
	code := c.Param("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "item code required"})
		return
	}
	if err := h.catalogRepo.Delete(code); err != nil {
		// repo returns sql.ErrNoRows when the code wasn't there — surface as 404
		// so the UI can distinguish "already gone" from "real failure".
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "ไม่พบสินค้า " + code + " ใน BillFlow"})
			return
		}
		h.logger.Error("catalog delete one", zap.String("code", code), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := h.catalogIdx.Reload(h.catalogRepo); err != nil {
		h.logger.Warn("catalog: reload index after delete", zap.Error(err))
	}
	if h.auditRepo != nil {
		var userID *string
		if uid := c.GetString("user_id"); uid != "" {
			userID = &uid
		}
		_ = h.auditRepo.Log(models.AuditEntry{
			Action:  "catalog_delete_one",
			UserID:  userID,
			Source:  "catalog",
			Level:   "info",
			TraceID: c.GetString("trace_id"),
			Detail:  map[string]interface{}{"item_code": code},
		})
	}
	c.JSON(http.StatusOK, gin.H{"deleted": code, "message": "ลบจาก BillFlow แล้ว (SML ไม่ถูกแตะ)"})
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
	started, err := h.StartEmbedAll("manual")
	if err != nil {
		if errors.Is(err, errCatalogEmbedNotConfigured) {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "GEMINI_API_KEY not configured"})
			return
		}
		if errors.Is(err, errCatalogEmbedAlreadyRunning) {
			c.JSON(http.StatusConflict, gin.H{"error": "embedding already running"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !started {
		c.JSON(http.StatusOK, gin.H{"message": "no pending items"})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"message": "embedding started in background"})
}

var (
	errCatalogEmbedNotConfigured  = errors.New("embedding service not configured")
	errCatalogEmbedAlreadyRunning = errors.New("embedding already running")
)

func (h *CatalogHandler) StartEmbedAll(reason string) (bool, error) {
	if !h.embSvc.IsConfigured() {
		return false, errCatalogEmbedNotConfigured
	}
	if h.catalogSvc.IsEmbedRunning() {
		return false, errCatalogEmbedAlreadyRunning
	}
	pending, err := h.catalogRepo.CountPending()
	if err != nil {
		return false, err
	}
	if pending == 0 {
		return false, nil
	}

	// Run in background goroutine
	go func() {
		h.logger.Info("catalog: embed-all started", zap.String("reason", reason), zap.Int("pending", pending))
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
	return true, nil
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

	textResults, textErr := h.catalogSvc.SearchByText(q, top)
	if textErr == nil && len(textResults) > 0 && textResults[0].Score >= 0.95 {
		results = textResults
		method = "text"
	}

	if len(results) == 0 && h.embSvc.IsConfigured() && h.catalogIdx.Size() > 0 {
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
		if textErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": textErr.Error()})
			return
		}
		results = textResults
		method = "text"
	}
	attachCatalogMatchImageURLs(results)

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
	item.ImageURL = catalogImageURL(item.ItemCode, item.ImageCount, item.PrimaryImageRoworder)
	c.JSON(http.StatusOK, item)
}

// GET /api/catalog/:code/image — authenticated proxy for the primary SML image.
func (h *CatalogHandler) GetImage(c *gin.Context) {
	code := strings.TrimSpace(c.Param("code"))
	if code == "" || strings.ContainsAny(code, "\x00\r\n") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid item code"})
		return
	}
	if h.cfg == nil || strings.TrimSpace(h.cfg.ShopeeSMLURL) == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "SML API is not configured"})
		return
	}

	item, err := h.catalogRepo.GetOne(code)
	if err != nil {
		h.logger.Warn("catalog image: lookup failed", zap.String("code", code), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "catalog lookup failed"})
		return
	}
	if item == nil || item.ImageCount <= 0 || item.PrimaryImageRoworder == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "product image not found"})
		return
	}
	h.streamCatalogImage(c, item.ItemCode, *item.PrimaryImageRoworder)
}

// GET /api/catalog/:code/images — authenticated metadata list for SML product images.
func (h *CatalogHandler) GetImages(c *gin.Context) {
	code := strings.TrimSpace(c.Param("code"))
	if code == "" || strings.ContainsAny(code, "\x00\r\n") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid item code"})
		return
	}
	if h.cfg == nil || strings.TrimSpace(h.cfg.ShopeeSMLURL) == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "SML API is not configured"})
		return
	}

	item, err := h.catalogRepo.GetOne(code)
	if err != nil {
		h.logger.Warn("catalog images: lookup failed", zap.String("code", code), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "catalog lookup failed"})
		return
	}
	if item == nil || item.ImageCount <= 0 {
		c.JSON(http.StatusOK, gin.H{"images": []catalogImageMeta{}})
		return
	}

	listURL := fmt.Sprintf("%s/api/v1/ic/products/%s/images",
		strings.TrimRight(h.cfg.ShopeeSMLURL, "/"),
		url.PathEscape(item.ItemCode),
	)
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, listURL, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "build image list request failed"})
		return
	}
	for k, v := range h.smlImageHeaders() {
		req.Header.Set(k, v)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		h.logger.Warn("catalog images: SML request failed", zap.String("code", code), zap.Error(err))
		c.JSON(http.StatusBadGateway, gin.H{"error": "read product images failed"})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		c.JSON(http.StatusOK, gin.H{"images": []catalogImageMeta{}})
		return
	}
	if resp.StatusCode != http.StatusOK {
		h.logger.Warn("catalog images: SML returned non-OK",
			zap.String("code", code), zap.Int("status", resp.StatusCode))
		c.JSON(http.StatusBadGateway, gin.H{"error": "read product images failed"})
		return
	}

	var payload struct {
		Data *struct {
			Images []catalogImageMeta `json:"images"`
		} `json:"data"`
		Images []catalogImageMeta `json:"images"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "decode product images failed"})
		return
	}
	images := payload.Images
	if payload.Data != nil {
		images = payload.Data.Images
	}
	if images == nil {
		images = []catalogImageMeta{}
	}
	for i := range images {
		if images[i].Roworder <= 0 {
			continue
		}
		images[i].ImageURL = catalogImageRowURL(item.ItemCode, images[i].Roworder)
	}

	c.Header("Cache-Control", "private, max-age=300")
	c.JSON(http.StatusOK, gin.H{"images": images})
}

// GET /api/catalog/:code/images/:roworder — authenticated proxy for a specific SML image.
func (h *CatalogHandler) GetImageByRoworder(c *gin.Context) {
	code := strings.TrimSpace(c.Param("code"))
	roworder, err := strconv.Atoi(c.Param("roworder"))
	if code == "" || strings.ContainsAny(code, "\x00\r\n") || err != nil || roworder < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid image request"})
		return
	}
	if h.cfg == nil || strings.TrimSpace(h.cfg.ShopeeSMLURL) == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "SML API is not configured"})
		return
	}

	item, err := h.catalogRepo.GetOne(code)
	if err != nil {
		h.logger.Warn("catalog image: lookup failed", zap.String("code", code), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "catalog lookup failed"})
		return
	}
	if item == nil || item.ImageCount <= 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "product image not found"})
		return
	}
	h.streamCatalogImage(c, item.ItemCode, roworder)
}

func (h *CatalogHandler) streamCatalogImage(c *gin.Context, itemCode string, roworder int) {
	imageURL := fmt.Sprintf("%s/api/v1/ic/products/%s/images/%d",
		strings.TrimRight(h.cfg.ShopeeSMLURL, "/"),
		url.PathEscape(itemCode),
		roworder,
	)
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, imageURL, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "build image proxy request failed"})
		return
	}
	for k, v := range h.smlImageHeaders() {
		req.Header.Set(k, v)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		h.logger.Warn("catalog image: SML request failed", zap.String("code", itemCode), zap.Int("roworder", roworder), zap.Error(err))
		c.JSON(http.StatusBadGateway, gin.H{"error": "read product image failed"})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		c.JSON(http.StatusNotFound, gin.H{"error": "product image not found"})
		return
	}
	if resp.StatusCode != http.StatusOK {
		h.logger.Warn("catalog image: SML returned non-OK",
			zap.String("code", itemCode), zap.Int("roworder", roworder), zap.Int("status", resp.StatusCode))
		c.JSON(http.StatusBadGateway, gin.H{"error": "read product image failed"})
		return
	}

	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	extraHeaders := map[string]string{
		"Cache-Control": "private, max-age=3600",
	}
	if etag := strings.TrimSpace(resp.Header.Get("ETag")); etag != "" {
		extraHeaders["ETag"] = etag
	}
	c.DataFromReader(http.StatusOK, resp.ContentLength, contentType, resp.Body, extraHeaders)
}

func (h *CatalogHandler) smlImageHeaders() map[string]string {
	if h.cfg == nil {
		return map[string]string{}
	}
	return map[string]string{
		"guid":           h.cfg.ShopeeSMLGUID,
		"provider":       h.cfg.ShopeeSMLProvider,
		"configFileName": h.cfg.ShopeeSMLConfigFile,
		"databaseName":   h.cfg.ShopeeSMLDatabase,
		"X-Tenant":       h.cfg.ShopeeSMLDatabase,
	}
}

func attachCatalogImageURLs(items []models.CatalogItem) {
	for i := range items {
		items[i].ImageURL = catalogImageURL(items[i].ItemCode, items[i].ImageCount, items[i].PrimaryImageRoworder)
	}
}

func attachCatalogMatchImageURLs(items []models.CatalogMatch) {
	for i := range items {
		items[i].ImageURL = catalogImageURL(items[i].ItemCode, items[i].ImageCount, items[i].PrimaryImageRoworder)
	}
}

func catalogImageURL(itemCode string, imageCount int, primaryRoworder *int) string {
	itemCode = strings.TrimSpace(itemCode)
	if itemCode == "" || imageCount <= 0 || primaryRoworder == nil {
		return ""
	}
	return "/api/catalog/" + url.PathEscape(itemCode) + "/image"
}

func catalogImageRowURL(itemCode string, roworder int) string {
	itemCode = strings.TrimSpace(itemCode)
	if itemCode == "" || roworder <= 0 {
		return ""
	}
	return "/api/catalog/" + url.PathEscape(itemCode) + "/images/" + strconv.Itoa(roworder)
}

// ─── Create new product ──────────────────────────────────────────────────────

// createProductRequest is the body the frontend sends. It's a compact "quick
// form" — only the minimum required for SML to accept the product.
type createProductRequest struct {
	Code      string  `json:"code" binding:"required"`      // SML item code (user-supplied)
	Name      string  `json:"name" binding:"required"`      // Product name (Thai or English)
	UnitCode  string  `json:"unit_code" binding:"required"` // e.g. "ชิ้น", "ถุง"
	Price     float64 `json:"price"`                        // per-unit selling price (>= 0)
	WHCode    string  `json:"wh_code,omitempty"`            // optional default warehouse
	ShelfCode string  `json:"shelf_code,omitempty"`         // optional default shelf
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
			"error":    "product code already exists",
			"code":     req.Code,
			"existing": existing,
		})
		return
	}

	// 2. Build SML payload — defaults pulled from the request example
	priceStr := strconv.FormatFloat(req.Price, 'f', -1, 64)
	smlReq := sml.CreateProductRequest{
		Code:          req.Code,
		Name:          req.Name,
		TaxType:       0, // VAT แยกนอก (matches Shopee saleinvoice default)
		ItemType:      0, // สินค้าทั่วไป
		UnitType:      1,
		UnitCost:      req.UnitCode,
		UnitStandard:  req.UnitCode,
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
		case smlResp != nil && smlResp.GetMessage() != "":
			errMsg = fmt.Sprintf("SML rejected (HTTP %d): %s", statusCode, smlResp.GetMessage())
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
		"code":       finalCode,
		"name":       req.Name,
		"unit_code":  req.UnitCode,
		"wh_code":    whCode,
		"shelf_code": shelfCode,
		"message":    "product created and queued for embedding",
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
