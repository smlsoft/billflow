package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"billflow/internal/config"
	"billflow/internal/models"
	"billflow/internal/repository"
	lineservice "billflow/internal/services/line"
	"billflow/internal/services/mapper"
	"billflow/internal/services/sml"
)

type BillHandler struct {
	billRepo       *repository.BillRepo
	mapperSvc      *mapper.Service
	smlClient      *sml.Client                // SML 213 JSON-RPC (LINE/email/lazada/manual)
	invoiceClient  *sml.InvoiceClient         // SML 248 saleinvoice REST (shopee/shopee_email)
	poClient       *sml.PurchaseOrderClient   // SML 248 purchaseorder REST (shopee_shipped)
	cfg            *config.Config
	lineSvc        *lineservice.Service
	auditRepo      *repository.AuditLogRepo
	log            *zap.Logger
}

func NewBillHandler(
	billRepo *repository.BillRepo,
	mapperSvc *mapper.Service,
	smlClient *sml.Client,
	invoiceClient *sml.InvoiceClient,
	poClient *sml.PurchaseOrderClient,
	cfg *config.Config,
	lineSvc *lineservice.Service,
	auditRepo *repository.AuditLogRepo,
	log *zap.Logger,
) *BillHandler {
	return &BillHandler{
		billRepo:      billRepo,
		mapperSvc:     mapperSvc,
		smlClient:     smlClient,
		invoiceClient: invoiceClient,
		poClient:      poClient,
		cfg:           cfg,
		lineSvc:       lineSvc,
		auditRepo:     auditRepo,
		log:           log,
	}
}

// GET /api/bills
func (h *BillHandler) List(c *gin.Context) {
	var f models.BillListFilter
	if err := c.ShouldBindQuery(&f); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	bills, total, err := h.billRepo.List(f)
	if err != nil {
		h.log.Error("List bills", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":      bills,
		"total":     total,
		"page":      f.Page,
		"page_size": f.PageSize,
	})
}

// GET /api/bills/:id
func (h *BillHandler) Get(c *gin.Context) {
	id := c.Param("id")
	bill, err := h.billRepo.FindByID(id)
	if err != nil {
		h.log.Error("FindByID", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	if bill == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "bill not found"})
		return
	}
	c.JSON(http.StatusOK, bill)
}

// POST /api/bills/:id/retry
// Routes to one of three SML clients based on bill.Source / bill.BillType:
//
//	line / email / lazada / manual (sale)  → smlClient.CreateSaleReserve  (SML 213 JSON-RPC)
//	shopee / shopee_email           (sale) → invoiceClient.CreateInvoice  (SML 248 saleinvoice)
//	shopee_shipped              (purchase) → poClient.CreatePurchaseOrder (SML 248 purchaseorder)
func (h *BillHandler) Retry(c *gin.Context) {
	id := c.Param("id")
	bill, err := h.billRepo.FindByID(id)
	if err != nil || bill == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "bill not found"})
		return
	}
	switch bill.Status {
	case "failed", "pending", "needs_review":
		// ok
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "only failed/pending/needs_review bills can be sent"})
		return
	}

	// Verify all items mapped — required regardless of route
	allMapped := true
	for _, item := range bill.Items {
		if item.ItemCode == nil || *item.ItemCode == "" {
			allMapped = false
			break
		}
	}
	if !allMapped {
		_ = h.billRepo.UpdateStatus(id, "needs_review", nil, nil, nil)
		c.JSON(http.StatusAccepted, gin.H{"message": "some items still unmapped — bill set to needs_review"})
		return
	}

	switch {
	case bill.Source == "shopee_shipped":
		h.retryPurchaseOrder(c, bill)
	case bill.Source == "shopee" || bill.Source == "shopee_email":
		h.retrySaleInvoice(c, bill)
	default:
		h.retrySaleReserve(c, bill)
	}
}

// ─── Route 1: SML 213 JSON-RPC SaleReserve (LINE/email/lazada/manual) ────────
func (h *BillHandler) retrySaleReserve(c *gin.Context, bill *models.Bill) {
	id := bill.ID

	// Re-run mapper to pick up any newly-added mappings
	var smlItems []sml.SMLItem
	for _, item := range bill.Items {
		match := h.mapperSvc.Match(item.RawName)
		itemCode := item.RawName
		unitCode := ""
		if item.UnitCode != nil {
			unitCode = *item.UnitCode
		}
		if item.ItemCode != nil {
			itemCode = *item.ItemCode
		}
		if match.Mapping != nil && !match.NeedsReview {
			itemCode = match.Mapping.ItemCode
			unitCode = match.Mapping.UnitCode
			_ = h.billRepo.UpdateBillItem(item.ID, match.Mapping.ItemCode, match.Mapping.UnitCode, match.Mapping.ID, true)
		}
		price := 0.0
		if item.Price != nil {
			price = *item.Price
		}
		smlItems = append(smlItems, sml.SMLItem{
			ItemCode: itemCode,
			Qty:      item.Qty,
			UnitCode: unitCode,
			Price:    price,
		})
	}

	var rawData struct {
		CustomerName  string  `json:"customer_name"`
		CustomerPhone *string `json:"customer_phone"`
	}
	if bill.RawData != nil {
		_ = json.Unmarshal(bill.RawData, &rawData)
	}

	req := sml.SaleOrderRequest{
		ContactName: rawData.CustomerName,
		Items:       smlItems,
	}
	if rawData.CustomerPhone != nil {
		req.ContactPhone = *rawData.CustomerPhone
	}

	reqJSON, _ := json.Marshal(req)
	start := time.Now()
	result, err := h.smlClient.CreateSaleReserve(req)
	if err != nil {
		h.recordFailure(c, id, bill.Source, reqJSON, err, start, "SaleReserve")
		return
	}

	respJSON, _ := json.Marshal(result)
	_ = h.billRepo.UpdateStatus(id, "sent", &result.DocNo, respJSON, nil)
	_ = h.billRepo.UpdateSMLPayload(id, reqJSON)
	if b, err := h.billRepo.FindByID(id); err == nil && b != nil {
		_ = h.billRepo.UpdatePriceHistory(b.Items)
	}
	h.recordSuccess(c, id, bill.Source, reqJSON, respJSON, result.DocNo, start)
	c.JSON(http.StatusOK, gin.H{"message": "bill sent to SML", "doc_no": result.DocNo})
}

// ─── Route 2: SML 248 saleinvoice REST (shopee, shopee_email) ────────────────
func (h *BillHandler) retrySaleInvoice(c *gin.Context, bill *models.Bill) {
	id := bill.ID
	if h.invoiceClient == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "saleinvoice client not configured"})
		return
	}

	items := make([]sml.ShopeeOrderItem, 0, len(bill.Items))
	for _, it := range bill.Items {
		if it.ItemCode == nil {
			continue
		}
		price := 0.0
		if it.Price != nil {
			price = *it.Price
		}
		items = append(items, sml.ShopeeOrderItem{
			SKU:         *it.ItemCode,
			ProductName: it.RawName,
			Price:       price,
			Qty:         it.Qty,
		})
	}

	cfg := h.shopeeInvoiceConfig()
	productCache := map[string]*sml.ProductInfo{} // already mapped — no extra lookup needed
	for _, it := range bill.Items {
		if it.ItemCode == nil || it.UnitCode == nil {
			continue
		}
		productCache[*it.ItemCode] = &sml.ProductInfo{
			Code:          *it.ItemCode,
			StartSaleUnit: *it.UnitCode,
		}
	}

	docDate := time.Now().Format("2006-01-02")
	payload := sml.BuildInvoicePayload("", docDate, items, cfg, productCache)
	reqJSON, _ := json.Marshal(payload)

	start := time.Now()
	statusCode, resp, err := h.invoiceClient.CreateInvoice(payload)
	if err != nil || resp == nil || !resp.IsSuccess() {
		errMsg := ""
		switch {
		case err != nil:
			errMsg = err.Error()
		case resp != nil:
			errMsg = fmt.Sprintf("HTTP %d — %s", statusCode, resp.Message)
		default:
			errMsg = fmt.Sprintf("HTTP %d", statusCode)
		}
		h.recordFailure(c, id, bill.Source, reqJSON, fmt.Errorf("%s", errMsg), start, "SaleInvoice")
		return
	}

	respJSON, _ := json.Marshal(resp)
	docNo := resp.GetDocNo()
	_ = h.billRepo.UpdateStatus(id, "sent", &docNo, respJSON, nil)
	_ = h.billRepo.UpdateSMLPayload(id, reqJSON)
	h.recordSuccess(c, id, bill.Source, reqJSON, respJSON, docNo, start)
	c.JSON(http.StatusOK, gin.H{"message": "bill sent to SML (saleinvoice)", "doc_no": docNo})
}

// ─── Route 3: SML 248 purchaseorder REST (shopee_shipped) ────────────────────
func (h *BillHandler) retryPurchaseOrder(c *gin.Context, bill *models.Bill) {
	id := bill.ID
	if h.poClient == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "purchaseorder client not configured"})
		return
	}

	items := make([]sml.POItem, 0, len(bill.Items))
	for _, it := range bill.Items {
		if it.ItemCode == nil {
			continue
		}
		price := 0.0
		if it.Price != nil {
			price = *it.Price
		}
		unit := ""
		if it.UnitCode != nil {
			unit = *it.UnitCode
		}
		items = append(items, sml.POItem{
			ItemCode: *it.ItemCode,
			ItemName: it.RawName,
			Qty:      it.Qty,
			Price:    price,
			UnitCode: unit,
		})
	}

	cfg := h.shopeePurchaseConfig()
	docDate := time.Now().Format("2006-01-02")
	payload := sml.BuildPurchaseOrderPayload("", docDate, items, cfg)
	reqJSON, _ := json.Marshal(payload)

	start := time.Now()
	statusCode, resp, err := h.poClient.CreatePurchaseOrder(payload)
	if err != nil || resp == nil || !resp.IsSuccess() {
		errMsg := ""
		switch {
		case err != nil:
			errMsg = err.Error()
		case resp != nil:
			errMsg = fmt.Sprintf("HTTP %d — %s", statusCode, resp.Message)
		default:
			errMsg = fmt.Sprintf("HTTP %d", statusCode)
		}
		h.recordFailure(c, id, bill.Source, reqJSON, fmt.Errorf("%s", errMsg), start, "PurchaseOrder")
		return
	}

	respJSON, _ := json.Marshal(resp)
	docNo := resp.GetDocNo()
	_ = h.billRepo.UpdateStatus(id, "sent", &docNo, respJSON, nil)
	_ = h.billRepo.UpdateSMLPayload(id, reqJSON)
	h.recordSuccess(c, id, bill.Source, reqJSON, respJSON, docNo, start)
	c.JSON(http.StatusOK, gin.H{"message": "bill sent to SML (purchaseorder)", "doc_no": docNo})
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func (h *BillHandler) shopeeInvoiceConfig() sml.InvoiceConfig {
	return sml.InvoiceConfig{
		BaseURL:    h.cfg.ShopeeSMLURL,
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
	}
}

func (h *BillHandler) shopeePurchaseConfig() sml.PurchaseOrderConfig {
	custCode := h.cfg.ShippedSMLCustCode
	if custCode == "" {
		custCode = h.cfg.ShopeeSMLCustCode
	}
	return sml.PurchaseOrderConfig{
		BaseURL:    h.cfg.ShopeeSMLURL,
		GUID:       h.cfg.ShopeeSMLGUID,
		Provider:   h.cfg.ShopeeSMLProvider,
		ConfigFile: h.cfg.ShopeeSMLConfigFile,
		Database:   h.cfg.ShopeeSMLDatabase,
		DocFormat:  h.cfg.ShippedSMLDocFormat,
		CustCode:   custCode,
		SaleCode:   h.cfg.ShopeeSMLSaleCode,
		BranchCode: h.cfg.ShopeeSMLBranchCode,
		WHCode:     h.cfg.ShopeeSMLWHCode,
		ShelfCode:  h.cfg.ShopeeSMLShelfCode,
		UnitCode:   h.cfg.ShopeeSMLUnitCode,
		VATType:    h.cfg.ShopeeSMLVATType,
		VATRate:    h.cfg.ShopeeSMLVATRate,
		DocTime:    h.cfg.ShopeeSMLDocTime,
	}
}

func (h *BillHandler) recordFailure(c *gin.Context, id, source string, reqJSON []byte, err error, start time.Time, route string) {
	errMsg := err.Error()
	respJSON, _ := json.Marshal(map[string]string{"error": errMsg})
	_ = h.billRepo.UpdateStatus(id, "failed", nil, respJSON, &errMsg)
	h.log.Error("Retry: SML failed", zap.String("bill", id), zap.String("route", route), zap.Error(err))
	if h.auditRepo != nil {
		billID := id
		durMs := int(time.Since(start).Milliseconds())
		_ = h.auditRepo.Log(models.AuditEntry{
			Action:     "sml_failed",
			TargetID:   &billID,
			Source:     source,
			Level:      "error",
			TraceID:    c.GetString("trace_id"),
			DurationMs: &durMs,
			Detail: map[string]interface{}{
				"sml_payload": json.RawMessage(reqJSON),
				"error":       errMsg,
				"route":       route,
				"via":         "retry",
			},
		})
	}
	if h.lineSvc != nil {
		_ = h.lineSvc.PushAdmin(fmt.Sprintf("⚠️ Bill retry SML failed (%s)\nBill: %s\nError: %s", route, id, errMsg))
	}
	c.JSON(http.StatusBadGateway, gin.H{"error": "SML send failed: " + errMsg})
}

func (h *BillHandler) recordSuccess(c *gin.Context, id, source string, reqJSON, respJSON []byte, docNo string, start time.Time) {
	if h.auditRepo == nil {
		return
	}
	billID := id
	durMs := int(time.Since(start).Milliseconds())
	_ = h.auditRepo.Log(models.AuditEntry{
		Action:     "sml_sent",
		TargetID:   &billID,
		Source:     source,
		Level:      "info",
		TraceID:    c.GetString("trace_id"),
		DurationMs: &durMs,
		Detail: map[string]interface{}{
			"doc_no":       docNo,
			"sml_payload":  json.RawMessage(reqJSON),
			"sml_response": json.RawMessage(respJSON),
			"via":          "retry",
		},
	})
	h.log.Info("Retry: bill sent", zap.String("bill", id), zap.String("doc", docNo))
}

// ─── Item edit ───────────────────────────────────────────────────────────────

// PUT /api/bills/:id/items/:item_id — edit item code/unit/qty/price before sending.
type updateItemRequest struct {
	ItemCode *string  `json:"item_code"`
	UnitCode *string  `json:"unit_code"`
	Qty      *float64 `json:"qty"`
	Price    *float64 `json:"price"`
}

func (h *BillHandler) UpdateItem(c *gin.Context) {
	billID := c.Param("id")
	itemID := c.Param("item_id")

	bill, err := h.billRepo.FindByID(billID)
	if err != nil || bill == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "bill not found"})
		return
	}
	if bill.Status == "sent" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot edit items on a bill already sent to SML"})
		return
	}

	var req updateItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.billRepo.UpdateBillItemFields(itemID, req.ItemCode, req.UnitCode, req.Qty, req.Price); err != nil {
		h.log.Error("UpdateItem", zap.String("bill", billID), zap.String("item", itemID), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "update failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "item updated"})
}
