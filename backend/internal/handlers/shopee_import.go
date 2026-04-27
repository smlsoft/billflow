package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/xuri/excelize/v2"
	"go.uber.org/zap"

	"billflow/internal/config"
	"billflow/internal/models"
	"billflow/internal/repository"
	"billflow/internal/services/catalog"
	"billflow/internal/services/sml"
)

// ShopeeImportHandler handles Shopee Excel import.
//
// Behavior change (2026-04-27): Confirm no longer pushes to SML inline.
// Bills are created with catalog-matched items and saved as pending /
// needs_review; the user reviews them in BillDetail and clicks "ส่ง SML",
// which routes through bills.go retrySaleInvoice (same path as Shopee
// email orders). This unifies all manual-confirm flows.
type ShopeeImportHandler struct {
	billRepo    *repository.BillRepo
	auditRepo   *repository.AuditLogRepo
	cfg         *config.Config
	catalogSvc  *catalog.SMLCatalogService
	embSvc      *catalog.EmbeddingService
	catalogIdx  *catalog.CatalogIndex
	logger      *zap.Logger
}

func NewShopeeImportHandler(
	billRepo *repository.BillRepo,
	auditRepo *repository.AuditLogRepo,
	cfg *config.Config,
	catalogSvc *catalog.SMLCatalogService,
	embSvc *catalog.EmbeddingService,
	catalogIdx *catalog.CatalogIndex,
	logger *zap.Logger,
) *ShopeeImportHandler {
	return &ShopeeImportHandler{
		billRepo:   billRepo,
		auditRepo:  auditRepo,
		cfg:        cfg,
		catalogSvc: catalogSvc,
		embSvc:     embSvc,
		catalogIdx: catalogIdx,
		logger:     logger,
	}
}

// ─── Shopee column name candidates ───────────────────────────────────────────

// shopeeColCandidates maps field names to keyword substrings.
// Matching uses strings.Contains so partial header names work
// even when Shopee adds English translations like "หมายเลขคำสั่งซื้อ (Order No.)"
var shopeeColCandidates = map[string][]string{
	"order_id":     {"หมายเลขคำสั่งซื้อ"},
	"status":       {"สถานะการสั่งซื้อ"},
	"order_date":   {"วันที่สั่งซื้อ", "วันที่ทำการสั่งซื้อ", "Order Creation Date", "Order Date"},
	"product_name": {"ชื่อสินค้า"},
	"sku":          {"เลขอ้างอิง SKU", "SKU Reference No."},
	"price":        {"ราคาขาย"},
	"qty":          {"จำนวน"},
}

var excludeStatuses = map[string]bool{
	"ที่ต้องจัดส่ง": true,
	"ยกเลิกแล้ว":    true,
}

// ─── Request / Response types ─────────────────────────────────────────────────

// ShopeeConfigRequest holds the config fields sent from the frontend dialog.
type ShopeeConfigRequest struct {
	ServerURL  string  `json:"server_url"`
	GUID       string  `json:"guid"`
	Provider   string  `json:"provider"`
	ConfigFile string  `json:"config_file_name"`
	Database   string  `json:"database_name"`
	DocFormat  string  `json:"doc_format_code"`
	CustCode   string  `json:"cust_code"`
	SaleCode   string  `json:"sale_code"`
	BranchCode string  `json:"branch_code"`
	WHCode     string  `json:"wh_code"`
	ShelfCode  string  `json:"shelf_code"`
	UnitCode   string  `json:"unit_code"`
	VATType    int     `json:"vat_type"`
	VATRate    float64 `json:"vat_rate"`
	DocTime    string  `json:"doc_time"`
}

// ShopeeOrder is one parsed Shopee order (returned in preview).
type ShopeeOrder struct {
	OrderID   string                `json:"order_id"`
	DocDate   string                `json:"doc_date"`
	Status    string                `json:"status"`
	Items     []sml.ShopeeOrderItem `json:"items"`
	ItemCount int                   `json:"item_count"`
	TotalQty  float64               `json:"total_qty"`
	// preview-only
	Duplicate bool `json:"duplicate"`
}

// PreviewResponse is returned from POST /api/import/shopee/preview
type PreviewResponse struct {
	Orders         []ShopeeOrder `json:"orders"`
	Warnings       []string      `json:"warnings"`
	TotalOrders    int           `json:"total_orders"`
	DuplicateCount int           `json:"duplicate_count"`
	SkippedCount   int           `json:"skipped_count"`
}

// ConfirmRequest is sent by the frontend for POST /api/import/shopee/confirm
type ConfirmRequest struct {
	Config   ShopeeConfigRequest `json:"config"`
	OrderIDs []string            `json:"order_ids"` // only these order IDs will be processed
	Orders   []ShopeeOrder       `json:"orders"`    // full parsed order data
}

// ConfirmResult is one processed order result.
type ConfirmResult struct {
	OrderID string `json:"order_id"`
	Success bool   `json:"success"`
	DocNo   string `json:"doc_no,omitempty"`
	Message string `json:"message,omitempty"`
	BillID  string `json:"bill_id,omitempty"`
}

// ─── GET /api/settings/shopee-config ─────────────────────────────────────────

// GetConfig returns the default Shopee SML config from env (pre-fill for dialog).
func (h *ShopeeImportHandler) GetConfig(c *gin.Context) {
	c.JSON(http.StatusOK, ShopeeConfigRequest{
		ServerURL:  h.cfg.ShopeeSMLURL,
		GUID:       h.cfg.ShopeeSMLGUID,
		Provider:   h.cfg.ShopeeSMLProvider,
		ConfigFile: h.cfg.ShopeeSMLConfigFile,
		Database:   h.cfg.ShopeeSMLDatabase,
		DocFormat:  h.cfg.ShopeeSMLDocFormat,
		CustCode:   h.cfg.ShopeeSMLCustCode,
		SaleCode:   h.cfg.ShopeeSMLSaleCode,
		BranchCode: h.cfg.ShopeeSMLBranchCode,
		WHCode:     h.cfg.ShopeeSMLWHCode,
		ShelfCode:  h.cfg.ShopeeSMLShelfCode,
		UnitCode:   h.cfg.ShopeeSMLUnitCode,
		VATType:    h.cfg.ShopeeSMLVATType,
		VATRate:    h.cfg.ShopeeSMLVATRate,
		DocTime:    h.cfg.ShopeeSMLDocTime,
	})
}

// ─── POST /api/import/shopee/preview ─────────────────────────────────────────

// Preview parses the uploaded Shopee Excel and returns order previews + warnings.
// Does NOT write to DB or call SML.
func (h *ShopeeImportHandler) Preview(c *gin.Context) {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "กรุณาแนบไฟล์ Excel (.xlsx)"})
		return
	}
	if !strings.HasSuffix(strings.ToLower(fileHeader.Filename), ".xlsx") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "รองรับเฉพาะไฟล์ .xlsx เท่านั้น"})
		return
	}

	file, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "เปิดไฟล์ไม่ได้"})
		return
	}
	defer file.Close()

	orders, warnings, err := parseShopeeExcel(file)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Mark duplicates (orders already in DB)
	dupCount := 0
	for i := range orders {
		exists, _ := h.existsShopeeOrder(orders[i].OrderID)
		if exists {
			orders[i].Duplicate = true
			dupCount++
		}
	}

	if h.auditRepo != nil {
		traceID := c.GetString("trace_id")
		var userID *string
		if uid := c.GetString("user_id"); uid != "" {
			userID = &uid
		}
		_ = h.auditRepo.Log(models.AuditEntry{
			Action:  "shopee_import_preview",
			UserID:  userID,
			Source:  "shopee_excel",
			Level:   "info",
			TraceID: traceID,
			Detail: map[string]interface{}{
				"filename":        fileHeader.Filename,
				"total_orders":    len(orders),
				"duplicate_count": dupCount,
			},
		})
	}

	c.JSON(http.StatusOK, PreviewResponse{
		Orders:         orders,
		Warnings:       warnings,
		TotalOrders:    len(orders),
		DuplicateCount: dupCount,
	})
}

// ─── POST /api/import/shopee/confirm ─────────────────────────────────────────

// Confirm processes the selected orders: calls SML 224 and saves bills to DB.
func (h *ShopeeImportHandler) Confirm(c *gin.Context) {
	var req ConfirmRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "request ไม่ถูกต้อง: " + err.Error()})
		return
	}

	selectedSet := make(map[string]bool, len(req.OrderIDs))
	for _, id := range req.OrderIDs {
		selectedSet[id] = true
	}

	// Default unit code from the request config; used as a fallback when
	// catalog matching doesn't pick a specific unit.
	defaultUnit := req.Config.UnitCode

	var userID *string
	if uid := c.GetString("user_id"); uid != "" {
		userID = &uid
	}
	traceID := c.GetString("trace_id")
	confirmStart := time.Now()

	const topK = 5
	const highConfThreshold = 0.85

	results := []ConfirmResult{}

	for _, order := range req.Orders {
		if !selectedSet[order.OrderID] {
			continue
		}
		if exists, _ := h.existsShopeeOrder(order.OrderID); exists {
			results = append(results, ConfirmResult{
				OrderID: order.OrderID,
				Success: false,
				Message: "order นี้มีอยู่ในระบบแล้ว (ข้าม)",
			})
			continue
		}

		// Catalog match each item BEFORE creating the bill so we know the
		// final status (pending vs needs_review).
		type itemEnriched struct {
			item       models.BillItem
			candidates []models.CatalogMatch
		}
		var enriched []itemEnriched
		allHigh := true

		for _, it := range order.Items {
			var matches []models.CatalogMatch
			if h.embSvc != nil && h.embSvc.IsConfigured() && h.catalogIdx != nil && h.catalogIdx.Size() > 0 {
				if emb, err := h.embSvc.EmbedText(it.ProductName); err == nil {
					matches = h.catalogIdx.Search(emb, topK)
				}
			}
			if len(matches) == 0 && h.catalogSvc != nil {
				matches, _ = h.catalogSvc.SearchByText(it.ProductName, topK)
			}

			price := it.Price
			bi := models.BillItem{
				RawName: it.ProductName,
				Qty:     it.Qty,
				Price:   &price,
			}

			// Priority: catalog top score ≥ 0.85 wins; else use Excel SKU
			// as best guess (mapped=false), keeps existing behaviour for
			// sellers whose Shopee SKUs already equal their SML codes.
			switch {
			case len(matches) > 0 && matches[0].Score >= highConfThreshold:
				bi.ItemCode = &matches[0].ItemCode
				unit := matches[0].UnitCode
				if unit == "" {
					unit = defaultUnit
				}
				bi.UnitCode = &unit
				bi.Mapped = true
			case it.SKU != "":
				sku := it.SKU
				bi.ItemCode = &sku
				unit := defaultUnit
				bi.UnitCode = &unit
				bi.Mapped = false
				allHigh = false
			default:
				if len(matches) > 0 {
					bi.ItemCode = &matches[0].ItemCode
					unit := matches[0].UnitCode
					if unit == "" {
						unit = defaultUnit
					}
					bi.UnitCode = &unit
				}
				bi.Mapped = false
				allHigh = false
			}

			enriched = append(enriched, itemEnriched{item: bi, candidates: matches})
		}

		status := "pending"
		if !allHigh {
			status = "needs_review"
		}

		aiConf := 1.0
		rawData, _ := json.Marshal(map[string]interface{}{
			"flow":            "shopee_excel",
			"shopee_order_id": order.OrderID,
			"order_id":        order.OrderID,
			"doc_date":        order.DocDate,
			"status":          order.Status,
		})
		bill := &models.Bill{
			BillType:     "sale",
			Source:       "shopee",
			Status:       status,
			AIConfidence: &aiConf,
			RawData:      rawData,
			SMLOrderID:   order.OrderID,
		}
		if userID != nil {
			bill.CreatedBy = userID
		}
		if err := h.billRepo.Create(bill); err != nil {
			h.logger.Error("create bill", zap.String("order_id", order.OrderID), zap.Error(err))
			results = append(results, ConfirmResult{
				OrderID: order.OrderID,
				Success: false,
				Message: "บันทึก bill ล้มเหลว: " + err.Error(),
			})
			continue
		}

		for i := range enriched {
			enriched[i].item.BillID = bill.ID
			candidatesJSON, _ := json.Marshal(enriched[i].candidates)
			_ = h.billRepo.InsertItemWithCandidates(&enriched[i].item, candidatesJSON)
		}

		// Audit log — bill created (no SML call, that happens later via Retry)
		if h.auditRepo != nil {
			billIDStr := bill.ID
			durMs := int(time.Since(confirmStart).Milliseconds())
			_ = h.auditRepo.Log(models.AuditEntry{
				Action:     "bill_created",
				TargetID:   &billIDStr,
				UserID:     userID,
				Source:     "shopee_excel",
				Level:      "info",
				TraceID:    traceID,
				DurationMs: &durMs,
				Detail: map[string]interface{}{
					"order_id":      order.OrderID,
					"items_count":   len(enriched),
					"all_high_conf": allHigh,
					"status":        status,
				},
			})
		}

		results = append(results, ConfirmResult{
			OrderID: order.OrderID,
			Success: true,
			BillID:  bill.ID,
			Message: fmt.Sprintf("สร้างบิลแล้ว (status=%s) — รอตรวจสอบใน /bills", status),
		})
		h.logger.Info("shopee_excel: bill created",
			zap.String("order_id", order.OrderID),
			zap.String("bill_id", bill.ID),
			zap.String("status", status),
		)
	}

	successCount := 0
	for _, r := range results {
		if r.Success {
			successCount++
		}
	}

	if h.auditRepo != nil {
		totalDurMs := int(time.Since(confirmStart).Milliseconds())
		_ = h.auditRepo.Log(models.AuditEntry{
			Action:     "shopee_import_done",
			UserID:     userID,
			Source:     "shopee_excel",
			Level:      "info",
			TraceID:    traceID,
			DurationMs: &totalDurMs,
			Detail: map[string]interface{}{
				"total":         len(results),
				"success_count": successCount,
				"fail_count":    len(results) - successCount,
			},
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"results":       results,
		"success_count": successCount,
		"fail_count":    len(results) - successCount,
		"total":         len(results),
		"message":       "บิลถูกสร้างแล้ว — กรุณาเข้าไปตรวจสอบและกดยืนยันส่งใน /bills",
	})
}

// ─── Excel Parser ─────────────────────────────────────────────────────────────

func parseShopeeExcel(src interface{ Read([]byte) (int, error) }) ([]ShopeeOrder, []string, error) {
	f, err := excelize.OpenReader(src)
	if err != nil {
		return nil, nil, fmt.Errorf("เปิดไฟล์ Excel ไม่ได้: %w", err)
	}
	defer f.Close()

	sheetName := f.GetSheetName(0)
	rows, err := f.GetRows(sheetName)
	if err != nil {
		return nil, nil, fmt.Errorf("อ่าน sheet ไม่ได้: %w", err)
	}
	if len(rows) < 2 {
		return nil, nil, fmt.Errorf("ไฟล์ว่างหรือไม่มีข้อมูล")
	}

	// Find header row: first row that contains an order_id candidate keyword
	headerRowIdx := 0
	orderIDCandidates := shopeeColCandidates["order_id"]
	for i, row := range rows {
		for _, cell := range row {
			trimmed := strings.TrimSpace(cell)
			for _, candidate := range orderIDCandidates {
				if strings.Contains(trimmed, candidate) {
					headerRowIdx = i
					goto foundHeader
				}
			}
		}
	}
foundHeader:

	headerRow := rows[headerRowIdx]

	// Map field → column index using substring matching.
	// This handles Shopee headers that include English translations, e.g.
	// "หมายเลขคำสั่งซื้อ (Order No.)" matches candidate "หมายเลขคำสั่งซื้อ".
	colIdx := map[string]int{}
	for field, candidates := range shopeeColCandidates {
		for j, cell := range headerRow {
			trimmed := strings.TrimSpace(cell)
			for _, c := range candidates {
				if strings.Contains(trimmed, c) {
					colIdx[field] = j
					break
				}
			}
			if _, found := colIdx[field]; found {
				break
			}
		}
	}

	// Check required columns
	required := []string{"order_id", "status", "order_date", "sku", "price", "qty"}
	for _, f := range required {
		if _, ok := colIdx[f]; !ok {
			return nil, nil, fmt.Errorf("ไม่พบ column '%s' ในไฟล์ — columns ที่พบ: %s",
				f, strings.Join(headerRow[:min(len(headerRow), 15)], ", "))
		}
	}

	warnings := []string{} // initialize as empty slice (never nil) to avoid JSON null
	orderMap := map[string]*ShopeeOrder{}
	orderKeys := []string{} // preserve insertion order
	skuMissingOrders := map[string]bool{}
	skippedCount := 0

	for _, row := range rows[headerRowIdx+1:] {
		if len(row) == 0 {
			continue
		}
		orderID := cellStr(row, colIdx["order_id"])
		if orderID == "" || strings.EqualFold(orderID, "nan") {
			continue
		}

		// Filter excluded statuses
		status := cellStr(row, colIdx["status"])
		if excludeStatuses[status] {
			skippedCount++
			continue
		}

		// Parse date
		docDate := cellStr(row, colIdx["order_date"])
		if len(docDate) >= 10 {
			docDate = docDate[:10]
		} else {
			docDate = time.Now().Format("2006-01-02")
		}

		if _, exists := orderMap[orderID]; !exists {
			orderMap[orderID] = &ShopeeOrder{
				OrderID: orderID,
				DocDate: docDate,
				Status:  status,
				Items:   []sml.ShopeeOrderItem{},
			}
			orderKeys = append(orderKeys, orderID)
		}

		sku := cellStr(row, colIdx["sku"])
		productName := ""
		if idx, ok := colIdx["product_name"]; ok {
			productName = cellStr(row, idx)
		}

		if sku == "" || strings.EqualFold(sku, "nan") {
			if !skuMissingOrders[orderID] {
				skuMissingOrders[orderID] = true
				shortName := productName
				if len(shortName) > 40 {
					shortName = shortName[:40]
				}
				warnings = append(warnings,
					fmt.Sprintf("Order %s: ไม่มีรหัส SKU สำหรับ \"%s\" — กรุณากำหนด SKU ใน Shopee Seller Center", orderID, shortName))
			}
			continue
		}

		price := cellFloat(row, colIdx["price"])
		qty := cellFloat(row, colIdx["qty"])
		if qty <= 0 {
			qty = 1
		}

		orderMap[orderID].Items = append(orderMap[orderID].Items, sml.ShopeeOrderItem{
			SKU:         sku,
			ProductName: productName,
			Price:       price,
			Qty:         qty,
		})
	}

	// Build result list in original order, skip orders with no items
	var orders []ShopeeOrder
	for _, id := range orderKeys {
		o := orderMap[id]
		if len(o.Items) == 0 {
			if !skuMissingOrders[id] {
				warnings = append(warnings, fmt.Sprintf("Order %s: ไม่มีสินค้า — ข้ามไป", id))
			}
			continue
		}
		o.ItemCount = len(o.Items)
		for _, it := range o.Items {
			o.TotalQty += it.Qty
		}
		orders = append(orders, *o)
	}

	if skippedCount > 0 {
		warnings = append([]string{fmt.Sprintf("กรอง %d แถว (สถานะ: ที่ต้องจัดส่ง, ยกเลิกแล้ว)", skippedCount)}, warnings...)
	}

	return orders, warnings, nil
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func (h *ShopeeImportHandler) existsShopeeOrder(orderID string) (bool, error) {
	var count int
	err := h.billRepo.DB().QueryRow(
		`SELECT COUNT(*) FROM bills WHERE source='shopee' AND raw_data->>'order_id' = $1`,
		orderID,
	).Scan(&count)
	return count > 0, err
}

func cellStr(row []string, idx int) string {
	if idx >= 0 && idx < len(row) {
		v := strings.TrimSpace(row[idx])
		if strings.EqualFold(v, "nan") {
			return ""
		}
		return v
	}
	return ""
}

func cellFloat(row []string, idx int) float64 {
	s := cellStr(row, idx)
	if s == "" {
		return 0
	}
	// Remove commas (Thai number formatting)
	s = strings.ReplaceAll(s, ",", "")
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

func strPtr(s string) *string { return &s }

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
