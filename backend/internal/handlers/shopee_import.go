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
	"billflow/internal/services/sml"
)

// ShopeeImportHandler handles Shopee Excel import via saleinvoice REST API (SML 224).
type ShopeeImportHandler struct {
	billRepo  *repository.BillRepo
	auditRepo *repository.AuditLogRepo
	cfg       *config.Config
	logger    *zap.Logger
}

func NewShopeeImportHandler(
	billRepo *repository.BillRepo,
	auditRepo *repository.AuditLogRepo,
	cfg *config.Config,
	logger *zap.Logger,
) *ShopeeImportHandler {
	return &ShopeeImportHandler{billRepo: billRepo, auditRepo: auditRepo, cfg: cfg, logger: logger}
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

	// Build set of selected order IDs
	selectedSet := make(map[string]bool, len(req.OrderIDs))
	for _, id := range req.OrderIDs {
		selectedSet[id] = true
	}

	// Build SML invoice client from request config
	invoiceCfg := sml.InvoiceConfig{
		BaseURL:    req.Config.ServerURL,
		GUID:       req.Config.GUID,
		Provider:   req.Config.Provider,
		ConfigFile: req.Config.ConfigFile,
		Database:   req.Config.Database,
		DocFormat:  req.Config.DocFormat,
		CustCode:   req.Config.CustCode,
		SaleCode:   req.Config.SaleCode,
		BranchCode: req.Config.BranchCode,
		WHCode:     req.Config.WHCode,
		ShelfCode:  req.Config.ShelfCode,
		UnitCode:   req.Config.UnitCode,
		VATType:    req.Config.VATType,
		VATRate:    req.Config.VATRate,
		DocTime:    req.Config.DocTime,
	}
	invoiceClient := sml.NewInvoiceClient(invoiceCfg, h.logger)

	// Get user from JWT for audit
	var userID *string
	if uid := c.GetString("user_id"); uid != "" {
		userID = &uid
	}
	traceID := c.GetString("trace_id")
	confirmStart := time.Now()

	// Pre-fetch product info for all unique SKUs (product cache)
	productCache := map[string]*sml.ProductInfo{}
	for _, order := range req.Orders {
		if !selectedSet[order.OrderID] {
			continue
		}
		for _, item := range order.Items {
			if _, seen := productCache[item.SKU]; !seen {
				prod, err := invoiceClient.GetProduct(item.SKU)
				if err != nil {
					h.logger.Warn("GetProduct failed", zap.String("sku", item.SKU), zap.Error(err))
					prod = nil
				}
				productCache[item.SKU] = prod // nil is ok — will use config fallbacks
			}
		}
	}

	results := []ConfirmResult{}

	for _, order := range req.Orders {
		if !selectedSet[order.OrderID] {
			continue
		}

		// Skip duplicates
		if exists, _ := h.existsShopeeOrder(order.OrderID); exists {
			results = append(results, ConfirmResult{
				OrderID: order.OrderID,
				Success: false,
				Message: "order นี้มีอยู่ในระบบแล้ว (ข้าม)",
			})
			continue
		}

		// Build saleinvoice payload
		// Pass empty doc_no — let SML generate it (omitempty). The Shopee order_id
		// is preserved in bills.sml_order_id and raw_data for BillFlow-side tracking.
		// Why: SML's ic_trans table has UNIQUE (doc_no, trans_flag); reusing the
		// Shopee order_id as doc_no triggers a duplicate-key violation if SML had
		// previously imported a row with the same string.
		payload := sml.BuildInvoicePayload(
			"",
			order.DocDate,
			order.Items,
			invoiceCfg,
			productCache,
		)
		payloadJSON, _ := json.Marshal(payload)

		// Save bill as pending first
		aiConf := 1.0
		rawData, _ := json.Marshal(map[string]interface{}{
			"order_id": order.OrderID,
			"doc_date": order.DocDate,
			"status":   order.Status,
		})
		bill := &models.Bill{
			BillType:     "sale",
			Source:       "shopee",
			Status:       "pending",
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

		// Insert bill items
		for _, item := range order.Items {
			bi := &models.BillItem{
				BillID:   bill.ID,
				RawName:  item.ProductName,
				ItemCode: strPtr(item.SKU),
				Qty:      item.Qty,
				UnitCode: strPtr(invoiceCfg.UnitCode),
				Price:    &item.Price,
				Mapped:   true,
			}
			_ = h.billRepo.InsertItem(bi)
		}

		// Store SML payload
		_ = h.billRepo.UpdateSMLPayload(bill.ID, payloadJSON)

		// Call SML saleinvoice
		var lastErr error
		var smlResp *sml.InvoiceResponse
		var statusCode int

		for attempt := 0; attempt < 3; attempt++ {
			if attempt > 0 {
				waitSecs := []int{1, 3, 5}[attempt-1]
				time.Sleep(time.Duration(waitSecs) * time.Second)
			}
			statusCode, smlResp, lastErr = invoiceClient.CreateInvoice(payload)
			if lastErr == nil && smlResp != nil && smlResp.IsSuccess() {
				break
			}
		}

		smlRespJSON, _ := json.Marshal(smlResp)

		if lastErr != nil || smlResp == nil || !smlResp.IsSuccess() {
			errMsg := "SML ไม่ตอบสนอง"
			if lastErr != nil {
				errMsg = lastErr.Error()
			} else if smlResp != nil {
				errMsg = fmt.Sprintf("HTTP %d — %s", statusCode, smlResp.Message)
			}
			_ = h.billRepo.UpdateStatus(bill.ID, "failed", nil, smlRespJSON, &errMsg)
			results = append(results, ConfirmResult{
				OrderID: order.OrderID,
				Success: false,
				Message: errMsg,
				BillID:  bill.ID,
			})
			h.logger.Error("saleinvoice failed",
				zap.String("order_id", order.OrderID),
				zap.String("bill_id", bill.ID),
				zap.String("error", errMsg),
			)
			if h.auditRepo != nil {
				billIDStr := bill.ID
				durMs := int(time.Since(confirmStart).Milliseconds())
				_ = h.auditRepo.Log(models.AuditEntry{
					Action:     "sml_failed",
					TargetID:   &billIDStr,
					UserID:     userID,
					Source:     "shopee_excel",
					Level:      "error",
					TraceID:    traceID,
					DurationMs: &durMs,
					Detail: map[string]interface{}{
						"order_id":     order.OrderID,
						"error":        errMsg,
						"sml_payload":  json.RawMessage(payloadJSON),
						"sml_response": json.RawMessage(smlRespJSON),
					},
				})
			}
			continue
		}

		// Success
		docNo := smlResp.GetDocNo()
		_ = h.billRepo.UpdateStatus(bill.ID, "sent", &docNo, smlRespJSON, nil)
		// Update price history for this bill's items
		fullBill, err := h.billRepo.FindByID(bill.ID)
		if err == nil && fullBill != nil {
			_ = h.billRepo.UpdatePriceHistory(fullBill.Items)
		}

		if h.auditRepo != nil {
			billIDStr := bill.ID
			durMs := int(time.Since(confirmStart).Milliseconds())
			_ = h.auditRepo.Log(models.AuditEntry{
				Action:     "sml_sent",
				TargetID:   &billIDStr,
				UserID:     userID,
				Source:     "shopee_excel",
				Level:      "info",
				TraceID:    traceID,
				DurationMs: &durMs,
				Detail: map[string]interface{}{
					"order_id":     order.OrderID,
					"doc_no":       docNo,
					"sml_payload":  json.RawMessage(payloadJSON),
					"sml_response": json.RawMessage(smlRespJSON),
				},
			})
		}

		results = append(results, ConfirmResult{
			OrderID: order.OrderID,
			Success: true,
			DocNo:   docNo,
			Message: smlResp.Message,
			BillID:  bill.ID,
		})
		h.logger.Info("saleinvoice sent",
			zap.String("order_id", order.OrderID),
			zap.String("doc_no", docNo),
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
