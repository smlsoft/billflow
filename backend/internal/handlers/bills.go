package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"billflow/internal/config"
	"billflow/internal/models"
	"billflow/internal/repository"
	"billflow/internal/services/artifact"
	lineservice "billflow/internal/services/line"
	"billflow/internal/services/mapper"
	"billflow/internal/services/sml"
)

type BillHandler struct {
	billRepo        *repository.BillRepo
	mapperSvc       *mapper.Service
	smlClient       *sml.Client                    // SML 213 JSON-RPC (sale_reserve)
	invoiceClient   *sml.InvoiceClient             // SML 248 saleinvoice REST (legacy)
	saleOrderClient *sml.SaleOrderClient           // SML 248 saleorder REST (default)
	poClient        *sml.PurchaseOrderClient       // SML 248 purchaseorder REST
	cfg             *config.Config
	lineSvc         *lineservice.Service
	auditRepo       *repository.AuditLogRepo
	catalogRepo     *repository.SMLCatalogRepo     // for unit_code defaults on item edit
	channelDefaults *repository.ChannelDefaultRepo // per-(channel,bill_type) party config
	docCounters     *repository.DocCounterRepo     // atomic doc_no generator
	artifactSvc     *artifact.Service              // source-artifact storage (PDF/HTML/etc.)
	log             *zap.Logger
}

func NewBillHandler(
	billRepo *repository.BillRepo,
	mapperSvc *mapper.Service,
	smlClient *sml.Client,
	invoiceClient *sml.InvoiceClient,
	saleOrderClient *sml.SaleOrderClient,
	poClient *sml.PurchaseOrderClient,
	cfg *config.Config,
	lineSvc *lineservice.Service,
	auditRepo *repository.AuditLogRepo,
	catalogRepo *repository.SMLCatalogRepo,
	channelDefaults *repository.ChannelDefaultRepo,
	docCounters *repository.DocCounterRepo,
	artifactSvc *artifact.Service,
	log *zap.Logger,
) *BillHandler {
	return &BillHandler{
		billRepo:        billRepo,
		mapperSvc:       mapperSvc,
		smlClient:       smlClient,
		invoiceClient:   invoiceClient,
		saleOrderClient: saleOrderClient,
		poClient:        poClient,
		cfg:             cfg,
		lineSvc:         lineSvc,
		auditRepo:       auditRepo,
		catalogRepo:     catalogRepo,
		channelDefaults: channelDefaults,
		docCounters:     docCounters,
		artifactSvc:     artifactSvc,
		log:             log,
	}
}

// resolveDocNo returns the doc_no to use for sending bill to SML. Reuses the
// existing bill.sml_doc_no when set (so re-retry of a failed bill doesn't
// inflate the counter or create duplicate docs in SML), otherwise generates
// a fresh one from def.DocPrefix + def.DocRunningFormat.
//
// fallbackPrefix is used when def has no prefix configured — typically the
// endpoint-flavored default ("BF-SO" for saleorder, "BF-PO" for PO, etc.)
func (h *BillHandler) resolveDocNo(bill *models.Bill, def *models.ChannelDefault, fallbackPrefix string) (string, error) {
	if bill.SMLDocNo != nil && *bill.SMLDocNo != "" {
		return *bill.SMLDocNo, nil
	}
	prefix := fallbackPrefix
	format := "YYMM####"
	if def != nil {
		if def.DocPrefix != "" {
			prefix = def.DocPrefix
		}
		if def.DocRunningFormat != "" {
			format = def.DocRunningFormat
		}
	}
	return h.docCounters.GenerateDocNo(prefix, format, time.Now())
}

// resolveEndpoint figures out which SML client to use for a channel.
//
// The admin-supplied `endpoint` is now a free-form URL/path (e.g.
// "/SMLJavaRESTService/v3/api/saleorder" or "https://sml/.../saleinvoice").
// We pick the client by keyword match — saleorder/saleinvoice/purchaseorder/
// sale_reserve in the URL — and pass the URL through as override so the
// client posts to the admin's chosen path.
//
// Returns:
//   kind        — "saleorder" | "saleinvoice" | "purchaseorder" | "sale_reserve"
//   urlOverride — the URL to send to (empty = use client's default)
func resolveEndpoint(def *models.ChannelDefault, source, billType string) (kind, urlOverride string) {
	raw := ""
	if def != nil {
		raw = def.Endpoint
	}
	rawLower := strings.ToLower(raw)

	switch {
	case strings.Contains(rawLower, "purchaseorder"):
		return "purchaseorder", raw
	case strings.Contains(rawLower, "saleinvoice"):
		return "saleinvoice", raw
	case strings.Contains(rawLower, "saleorder"):
		return "saleorder", raw
	case strings.Contains(rawLower, "sale_reserve"):
		// SML 213 sale_reserve uses MCP/SSE — URL not overridable
		return "sale_reserve", ""
	}

	// No keyword match (or empty) → default routing by channel + bill_type
	if source == "shopee_shipped" || billType == "purchase" {
		return "purchaseorder", ""
	}
	if source == "shopee" || source == "shopee_email" {
		return "saleorder", ""
	}
	return "sale_reserve", ""
}

// resolveItemName returns the catalog name for a mapped item_code, falling
// back to the source raw_name when no catalog row exists. Used so SML
// receives the canonical SML name instead of the original source product
// name (e.g. Shopee's verbose listing title).
func (h *BillHandler) resolveItemName(itemCode, rawName string) string {
	if h.catalogRepo != nil && itemCode != "" {
		if cat, _ := h.catalogRepo.GetOne(itemCode); cat != nil && cat.ItemName != "" {
			return cat.ItemName
		}
	}
	return rawName
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
//
// Response includes a "preview" object showing the SML route + endpoint +
// doc_no pattern that THIS bill would hit on retry. The preview is purely
// informational — it surfaces routing decisions in the BillDetail UI so
// admins catch misconfigured channels (e.g. shopee bill routed to
// sale_reserve because endpoint string doesn't match keywords) BEFORE
// they click Send and have to debug a failed bill afterwards.
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

	// Resolve which SML route + endpoint + doc_format this bill would use
	// today. Mirror the channel lookup that retry would do — same key
	// (channel, bill_type) — but never fail the GET if the row is missing
	// (e.g. legacy bills from before channel_defaults existed).
	channel := mapSourceToChannel(bill.Source)
	preview := gin.H{
		"channel":   channel,
		"bill_type": bill.BillType,
	}
	if h.channelDefaults != nil {
		def, _ := h.channelDefaults.Get(channel, bill.BillType)
		if def != nil {
			route, urlOverride := resolveEndpoint(def, bill.Source, bill.BillType)
			preview["route"] = route
			if urlOverride != "" {
				preview["endpoint"] = urlOverride
			}
			if def.DocPrefix != "" || def.DocRunningFormat != "" {
				preview["doc_format"] = def.DocPrefix + def.DocRunningFormat
			}
			if def.DocFormatCode != "" {
				preview["doc_format_code"] = def.DocFormatCode
			}
		} else {
			// No channel_default row — admin needs to set one up. Surface
			// this as a preview-level warning so the UI can render a hint.
			preview["error"] = "ยังไม่ได้ตั้งค่า channel — ไปที่ /settings/channels"
		}
	}

	// Wrap bill + preview in a single response. The bill struct is
	// preserved unchanged at the top level so existing consumers keep
	// working without a type migration.
	billJSON, _ := json.Marshal(bill)
	out := gin.H{}
	if err := json.Unmarshal(billJSON, &out); err == nil {
		out["preview"] = preview
		c.JSON(http.StatusOK, out)
		return
	}
	// Fallback if marshal/unmarshal hiccups — return the bill alone.
	c.JSON(http.StatusOK, bill)
}

// mapSourceToChannel mirrors the same logic the retry handler uses to look
// up a channel_defaults row. Kept private to this file.
func mapSourceToChannel(source string) string {
	switch source {
	case "shopee_shipped":
		return "shopee_shipped"
	case "shopee_email", "shopee":
		return "shopee"
	case "lazada":
		return "lazada"
	case "email":
		return "email"
	}
	return "line"
}

// GET /api/bills/:id/timeline
//
// Returns every audit_log row whose target_id matches this bill, oldest
// first. The BillDetail page renders these as a vertical activity feed so
// admin can answer "ทำไมบิลนี้ถึงเป็นแบบนี้" without leaving the page.
func (h *BillHandler) Timeline(c *gin.Context) {
	id := c.Param("id")
	if h.auditRepo == nil {
		c.JSON(http.StatusOK, gin.H{"data": []any{}})
		return
	}
	rows, err := h.auditRepo.ListByTarget(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": rows})
}

// POST /api/bills/:id/retry
// Routes to one of three SML clients based on bill.Source / bill.BillType:
//
//	line / email / lazada / manual (sale)  → smlClient.CreateSaleReserve  (SML 213 JSON-RPC)
//	shopee / shopee_email           (sale) → saleOrderClient.CreateSaleOrder (SML 248 saleorder — ใบสั่งขาย)
//	shopee_shipped              (purchase) → poClient.CreatePurchaseOrder (SML 248 purchaseorder)
// RetryRequest is the optional POST body for POST /api/bills/:id/retry.
// For purchase bills: party_code overrides channel_defaults.party_code and
// remark is stored on the bill + forwarded to SML.
type RetryRequest struct {
	PartyCode string `json:"party_code"`
	Remark    string `json:"remark"`
}

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

	// Parse optional request body (party_code / remark overrides for purchase bills)
	var req RetryRequest
	_ = c.ShouldBindJSON(&req)

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

	// Persist remark on the bill for all routes (not just purchase).
	// retryPurchaseOrder also calls UpdateRemark with its own value — that's
	// fine because req.Remark is the same string both times.
	if req.Remark != "" {
		if err := h.billRepo.UpdateRemark(id, req.Remark); err != nil {
			h.log.Warn("UpdateRemark failed", zap.Error(err))
		}
	}

	// Look up channel default once; pass to retry handlers + use to decide
	// which SML endpoint to dispatch to. The URL override (if admin typed one)
	// is threaded down to the client via context.
	def, _ := h.channelDefaults.Get(bill.Source, bill.BillType)
	kind, urlOverride := resolveEndpoint(def, bill.Source, bill.BillType)
	c.Set("sml_url_override", urlOverride)

	switch kind {
	case "purchaseorder":
		h.retryPurchaseOrder(c, bill, req.PartyCode, req.Remark)
	case "saleinvoice":
		h.retrySaleInvoice(c, bill)
	case "saleorder":
		h.retrySaleOrder(c, bill)
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

	// Override AI-extracted contact_name with the channel default so SML 213
	// doesn't create a fresh AR row for every chatbot session. Phone still
	// comes from chat (the user's real number) when present, falling back to
	// the default's phone snapshot only when chat didn't capture one.
	def, err := h.lookupChannelDefault(bill.Source, "sale")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	phone := ""
	if rawData.CustomerPhone != nil && *rawData.CustomerPhone != "" {
		phone = *rawData.CustomerPhone
	} else {
		phone = def.PartyPhone
	}
	req := sml.SaleOrderRequest{
		ContactName:  def.PartyName,
		ContactPhone: phone,
		Items:        smlItems,
	}

	reqJSON, _ := json.Marshal(req)
	start := time.Now()
	result, err := h.smlClient.CreateSaleReserve(req)
	if err != nil {
		// SaleReserve has no client-side doc_no — SML 213 generates BS… on success.
		h.recordFailure(c, id, bill.Source, reqJSON, err, start, "SaleReserve", "")
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

// ─── Route 2: SML 248 saleorder REST (shopee, shopee_email) ──────────────────
// ใบสั่งขาย — landed in /v3/api/saleorder, the sale-side counterpart to
// purchaseorder. Replaces the legacy /restapi/saleinvoice path so Shopee
// orders show up under "ใบสั่งขาย" in SML instead of "ใบกำกับภาษี".
func (h *BillHandler) retrySaleOrder(c *gin.Context, bill *models.Bill) {
	id := bill.ID
	if h.saleOrderClient == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "saleorder client not configured"})
		return
	}

	items := make([]sml.SOItem, 0, len(bill.Items))
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
		items = append(items, sml.SOItem{
			ItemCode: *it.ItemCode,
			ItemName: h.resolveItemName(*it.ItemCode, it.RawName),
			Qty:      it.Qty,
			Price:    price,
			UnitCode: unit,
		})
	}

	def, err := h.lookupChannelDefault(bill.Source, "sale")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	cfg := h.shopeeSaleOrderConfig()
	cfg.CustCode = def.PartyCode
	if def.DocFormatCode != "" {
		cfg.DocFormat = def.DocFormatCode
	}
	applyChannelOverrides(def, &cfg.WHCode, &cfg.ShelfCode, &cfg.VATType, &cfg.VATRate)

	docDate := docDateFromBill(bill)
	reqDocNo, err := h.resolveDocNo(bill, def, "BF-SO")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "generate doc_no: " + err.Error()})
		return
	}
	// Stamp doc_no on the bill BEFORE calling SML so a re-retry uses the same
	// number (no counter inflation, no duplicate docs in SML on transient fail).
	_ = h.billRepo.UpdateStatus(id, bill.Status, &reqDocNo, nil, nil)
	payload := sml.BuildSaleOrderPayload(reqDocNo, docDate, items, cfg)
	reqJSON, _ := json.Marshal(payload)

	start := time.Now()
	urlOverride := c.GetString("sml_url_override")
	statusCode, resp, err := h.saleOrderClient.CreateSaleOrder(payload, urlOverride)
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
		h.recordFailure(c, id, bill.Source, reqJSON, fmt.Errorf("%s", errMsg), start, "SaleOrder", reqDocNo)
		return
	}

	respJSON, _ := json.Marshal(resp)
	// SML often returns success with an empty data.doc_no — fall back to
	// the client-generated code so the bill is still trackable.
	docNo := resp.GetDocNo()
	if docNo == "" {
		docNo = reqDocNo
	}
	_ = h.billRepo.UpdateStatus(id, "sent", &docNo, respJSON, nil)
	_ = h.billRepo.UpdateSMLPayload(id, reqJSON)
	h.recordSuccess(c, id, bill.Source, reqJSON, respJSON, docNo, start)
	c.JSON(http.StatusOK, gin.H{"message": "bill sent to SML (saleorder)", "doc_no": docNo})
}

// ─── Route 2b: SML 248 saleinvoice REST (legacy ใบกำกับภาษี) ─────────────────
// Kept for admins who explicitly select endpoint="saleinvoice" on a channel
// (e.g. they need invoices instead of sale orders for tax purposes).
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
			ProductName: h.resolveItemName(*it.ItemCode, it.RawName),
			Price:       price,
			Qty:         it.Qty,
		})
	}

	def, err := h.lookupChannelDefault(bill.Source, "sale")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	cfg := sml.InvoiceConfig{
		BaseURL:    h.cfg.ShopeeSMLURL,
		GUID:       h.cfg.ShopeeSMLGUID,
		Provider:   h.cfg.ShopeeSMLProvider,
		ConfigFile: h.cfg.ShopeeSMLConfigFile,
		Database:   h.cfg.ShopeeSMLDatabase,
		DocFormat:  h.cfg.ShopeeSMLDocFormat,
		CustCode:   def.PartyCode,
		SaleCode:   h.cfg.ShopeeSMLSaleCode,
		BranchCode: h.cfg.ShopeeSMLBranchCode,
		WHCode:     h.cfg.ShopeeSMLWHCode,
		ShelfCode:  h.cfg.ShopeeSMLShelfCode,
		UnitCode:   h.cfg.ShopeeSMLUnitCode,
		VATType:    h.cfg.ShopeeSMLVATType,
		VATRate:    h.cfg.ShopeeSMLVATRate,
		DocTime:    h.cfg.ShopeeSMLDocTime,
	}
	if def.DocFormatCode != "" {
		cfg.DocFormat = def.DocFormatCode
	}
	applyChannelOverrides(def, &cfg.WHCode, &cfg.ShelfCode, &cfg.VATType, &cfg.VATRate)
	productCache := map[string]*sml.ProductInfo{}
	for _, it := range bill.Items {
		if it.ItemCode == nil || it.UnitCode == nil {
			continue
		}
		productCache[*it.ItemCode] = &sml.ProductInfo{
			Code:          *it.ItemCode,
			StartSaleUnit: *it.UnitCode,
		}
	}

	docDate := docDateFromBill(bill)
	reqDocNo, err := h.resolveDocNo(bill, def, "BF-INV")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "generate doc_no: " + err.Error()})
		return
	}
	_ = h.billRepo.UpdateStatus(id, bill.Status, &reqDocNo, nil, nil)
	payload := sml.BuildInvoicePayload(reqDocNo, docDate, items, cfg, productCache)
	reqJSON, _ := json.Marshal(payload)

	start := time.Now()
	urlOverride := c.GetString("sml_url_override")
	statusCode, resp, err := h.invoiceClient.CreateInvoice(payload, urlOverride)
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
		h.recordFailure(c, id, bill.Source, reqJSON, fmt.Errorf("%s", errMsg), start, "SaleInvoice", reqDocNo)
		return
	}

	respJSON, _ := json.Marshal(resp)
	docNo := resp.GetDocNo()
	if docNo == "" {
		docNo = reqDocNo
	}
	_ = h.billRepo.UpdateStatus(id, "sent", &docNo, respJSON, nil)
	_ = h.billRepo.UpdateSMLPayload(id, reqJSON)
	h.recordSuccess(c, id, bill.Source, reqJSON, respJSON, docNo, start)
	c.JSON(http.StatusOK, gin.H{"message": "bill sent to SML (saleinvoice)", "doc_no": docNo})
}

// ─── Route 3: SML 248 purchaseorder REST (shopee_shipped) ────────────────────
func (h *BillHandler) retryPurchaseOrder(c *gin.Context, bill *models.Bill, partyCodeOverride, remark string) {
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
			ItemName: h.resolveItemName(*it.ItemCode, it.RawName),
			Qty:      it.Qty,
			Price:    price,
			UnitCode: unit,
		})
	}

	def, err := h.lookupChannelDefault(bill.Source, "purchase")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	cfg := h.shopeePurchaseConfig()
	cfg.CustCode = def.PartyCode
	if partyCodeOverride != "" {
		cfg.CustCode = partyCodeOverride
	}
	if def.DocFormatCode != "" {
		cfg.DocFormat = def.DocFormatCode
	}
	applyChannelOverrides(def, &cfg.WHCode, &cfg.ShelfCode, &cfg.VATType, &cfg.VATRate)
	docDate := docDateFromBill(bill)
	reqDocNo, err := h.resolveDocNo(bill, def, "BF-PO")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "generate doc_no: " + err.Error()})
		return
	}
	// Persist remark before SML call so it's available even on failure
	if remark != "" {
		_ = h.billRepo.UpdateRemark(id, remark)
	}
	_ = h.billRepo.UpdateStatus(id, bill.Status, &reqDocNo, nil, nil)
	payload := sml.BuildPurchaseOrderPayload(reqDocNo, docDate, items, cfg, remark)
	reqJSON, _ := json.Marshal(payload)

	start := time.Now()
	urlOverride := c.GetString("sml_url_override")
	statusCode, resp, err := h.poClient.CreatePurchaseOrder(payload, urlOverride)
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
		h.recordFailure(c, id, bill.Source, reqJSON, fmt.Errorf("%s", errMsg), start, "PurchaseOrder", reqDocNo)
		return
	}

	respJSON, _ := json.Marshal(resp)
	// SML purchaseorder returns success but with an empty doc_no field —
	// fall back to the doc_no we generated client-side so the bill is
	// still trackable in the UI.
	docNo := resp.GetDocNo()
	if docNo == "" {
		docNo = reqDocNo
	}
	_ = h.billRepo.UpdateStatus(id, "sent", &docNo, respJSON, nil)
	_ = h.billRepo.UpdateSMLPayload(id, reqJSON)
	h.recordSuccess(c, id, bill.Source, reqJSON, respJSON, docNo, start)
	c.JSON(http.StatusOK, gin.H{"message": "bill sent to SML (purchaseorder)", "doc_no": docNo})
}

// docDateFromBill returns "YYYY-MM-DD" — the email-extracted doc_date stored
// in raw_data["doc_date"] when present, else today's date.
// Used by saleinvoice + purchaseorder retry paths so SML records reflect the
// real order/ship date rather than the moment the user clicked "ส่ง".
func docDateFromBill(bill *models.Bill) string {
	if bill != nil && bill.RawData != nil {
		var rd map[string]interface{}
		if err := json.Unmarshal(bill.RawData, &rd); err == nil {
			if v, ok := rd["doc_date"].(string); ok && v != "" {
				return v
			}
		}
	}
	return time.Now().Format("2006-01-02")
}

// ─── helpers ─────────────────────────────────────────────────────────────────

// shopeeSaleOrderConfig returns the static SML 248 saleorder config without
// CustCode — caller fills it from channel_defaults via lookupChannelDefault.
func (h *BillHandler) shopeeSaleOrderConfig() sml.SaleOrderConfig {
	return sml.SaleOrderConfig{
		BaseURL:    h.cfg.ShopeeSMLURL,
		GUID:       h.cfg.ShopeeSMLGUID,
		Provider:   h.cfg.ShopeeSMLProvider,
		ConfigFile: h.cfg.ShopeeSMLConfigFile,
		Database:   h.cfg.ShopeeSMLDatabase,
		DocFormat:  h.cfg.ShopeeSMLDocFormat,
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
	return sml.PurchaseOrderConfig{
		BaseURL:    h.cfg.ShopeeSMLURL,
		GUID:       h.cfg.ShopeeSMLGUID,
		Provider:   h.cfg.ShopeeSMLProvider,
		ConfigFile: h.cfg.ShopeeSMLConfigFile,
		Database:   h.cfg.ShopeeSMLDatabase,
		DocFormat:  h.cfg.ShippedSMLDocFormat,
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

// applyChannelOverrides overlays the per-channel WH/Shelf/VAT settings onto
// the env-derived dst values. Sentinel ('' / -1) means "no override — keep env".
// Pointer args so we can leave dst untouched when the channel didn't override.
func applyChannelOverrides(def *models.ChannelDefault, wh, shelf *string, vatType *int, vatRate *float64) {
	if def == nil {
		return
	}
	if def.WHCode != "" {
		*wh = def.WHCode
	}
	if def.ShelfCode != "" {
		*shelf = def.ShelfCode
	}
	if def.VATType >= 0 {
		*vatType = def.VATType
	}
	if def.VATRate >= 0 {
		*vatRate = def.VATRate
	}
}

// lookupChannelDefault fetches the (channel, bill_type) party config or
// returns an error suitable for a 400 response when nothing's set.
func (h *BillHandler) lookupChannelDefault(channel, billType string) (*models.ChannelDefault, error) {
	if h.channelDefaults == nil {
		return nil, fmt.Errorf("channel defaults not configured")
	}
	def, err := h.channelDefaults.Get(channel, billType)
	if err != nil {
		return nil, fmt.Errorf("lookup channel default: %w", err)
	}
	if def == nil {
		return nil, fmt.Errorf("ยังไม่ได้ตั้งค่าลูกค้า default สำหรับ %s/%s — ไปที่ /settings/channels", channel, billType)
	}
	return def, nil
}

// failureDetail is the JSON shape persisted to bills.error_msg when an SML
// retry fails. Storing structured data instead of a plain string lets the
// BillDetail UI render route + attempted doc_no + monospace error
// separately, and lets admin copy the raw error text to share with dev
// without other UI clutter.
//
// For backwards compat: the frontend tries JSON.parse first; if it fails
// (i.e. an old plain-text error_msg from before this change) it falls back
// to displaying the string verbatim.
type failureDetail struct {
	Route          string `json:"route"`            // SaleReserve / SaleOrder / SaleInvoice / PurchaseOrder
	DocNoAttempted string `json:"doc_no_attempted"` // empty for SaleReserve (SML generates)
	Error          string `json:"error"`
	OccurredAt     string `json:"occurred_at"` // RFC3339
}

func (h *BillHandler) recordFailure(c *gin.Context, id, source string, reqJSON []byte, err error, start time.Time, route, docNoAttempted string) {
	rawErr := err.Error()
	fail := failureDetail{
		Route:          route,
		DocNoAttempted: docNoAttempted,
		Error:          rawErr,
		OccurredAt:     time.Now().UTC().Format(time.RFC3339),
	}
	errMsgJSON, _ := json.Marshal(fail)
	errMsg := string(errMsgJSON)
	respJSON, _ := json.Marshal(map[string]string{"error": rawErr})
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

// POST /api/bills/:id/items — add a new line item to a not-yet-sent bill.
type addItemRequest struct {
	RawName  string  `json:"raw_name" binding:"required"`
	ItemCode *string `json:"item_code"`
	UnitCode *string `json:"unit_code"`
	Qty      float64 `json:"qty" binding:"required"`
	Price    *float64 `json:"price"`
}

func (h *BillHandler) AddItem(c *gin.Context) {
	billID := c.Param("id")

	bill, err := h.billRepo.FindByID(billID)
	if err != nil || bill == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "bill not found"})
		return
	}
	if bill.Status == "sent" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot add items to a bill already sent to SML"})
		return
	}

	var req addItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	mapped := req.ItemCode != nil && *req.ItemCode != ""
	item := &models.BillItem{
		BillID:   billID,
		RawName:  req.RawName,
		ItemCode: req.ItemCode,
		UnitCode: req.UnitCode,
		Qty:      req.Qty,
		Price:    req.Price,
		Mapped:   mapped,
	}
	if err := h.billRepo.InsertItem(item); err != nil {
		h.log.Error("AddItem", zap.String("bill", billID), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "insert failed"})
		return
	}

	if h.auditRepo != nil {
		_ = h.auditRepo.Log(models.AuditEntry{
			Action:   "bill_item_added",
			TargetID: &billID,
			Source:   bill.Source,
			Level:    "info",
			Detail: map[string]interface{}{
				"item_id":   item.ID,
				"raw_name":  req.RawName,
				"item_code": req.ItemCode,
				"qty":       req.Qty,
			},
		})
	}

	c.JSON(http.StatusCreated, item)
}

// DELETE /api/bills/:id/items/:item_id — remove a line item from a not-yet-sent bill.
func (h *BillHandler) DeleteItemRow(c *gin.Context) {
	billID := c.Param("id")
	itemID := c.Param("item_id")

	bill, err := h.billRepo.FindByID(billID)
	if err != nil || bill == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "bill not found"})
		return
	}
	if bill.Status == "sent" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot delete items from a bill already sent to SML"})
		return
	}

	if err := h.billRepo.DeleteItem(billID, itemID); err != nil {
		h.log.Error("DeleteItem", zap.String("bill", billID), zap.String("item", itemID), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "delete failed"})
		return
	}

	if h.auditRepo != nil {
		_ = h.auditRepo.Log(models.AuditEntry{
			Action:   "bill_item_deleted",
			TargetID: &billID,
			Source:   bill.Source,
			Level:    "info",
			Detail: map[string]interface{}{
				"item_id": itemID,
			},
		})
	}

	c.JSON(http.StatusOK, gin.H{"message": "item deleted"})
}

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

	// Find the item being edited so we know its raw_name for F1 feedback
	var existingItem *models.BillItem
	for i := range bill.Items {
		if bill.Items[i].ID == itemID {
			existingItem = &bill.Items[i]
			break
		}
	}
	if existingItem == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "item not found in bill"})
		return
	}

	var req updateItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// If user is changing item_code, fill unit_code from catalog if not provided.
	// This makes the F1 feedback richer and the SML payload more correct.
	if req.ItemCode != nil && *req.ItemCode != "" && (req.UnitCode == nil || *req.UnitCode == "") && h.catalogRepo != nil {
		if cat, _ := h.catalogRepo.GetOne(*req.ItemCode); cat != nil && cat.UnitCode != "" {
			u := cat.UnitCode
			req.UnitCode = &u
		}
	}

	if err := h.billRepo.UpdateBillItemFields(itemID, req.ItemCode, req.UnitCode, req.Qty, req.Price); err != nil {
		h.log.Error("UpdateItem", zap.String("bill", billID), zap.String("item", itemID), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "update failed"})
		return
	}

	// F1 learning loop: if the user supplied a non-empty item_code that's
	// different from what we previously had, save the (raw_name → item_code)
	// pair as an ai_learned mapping. Future bills with similar raw_name will
	// auto-resolve.
	if req.ItemCode != nil && *req.ItemCode != "" && existingItem.RawName != "" {
		prev := ""
		if existingItem.ItemCode != nil {
			prev = *existingItem.ItemCode
		}
		if prev != *req.ItemCode {
			unit := ""
			if req.UnitCode != nil {
				unit = *req.UnitCode
			}
			if err := h.mapperSvc.LearnFromFeedback(existingItem.RawName, *req.ItemCode, unit, &billID); err != nil {
				h.log.Warn("UpdateItem: F1 feedback save failed",
					zap.String("raw_name", existingItem.RawName),
					zap.String("item_code", *req.ItemCode),
					zap.Error(err))
			} else if h.auditRepo != nil {
				_ = h.auditRepo.Log(models.AuditEntry{
					Action:   "mapping_feedback",
					TargetID: &itemID,
					Source:   bill.Source,
					Level:    "info",
					Detail: map[string]interface{}{
						"raw_name":      existingItem.RawName,
						"prev_code":     prev,
						"new_code":      *req.ItemCode,
						"unit_code":     unit,
						"bill_id":       billID,
					},
				})
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "item updated"})
}

// ─── Source artifact endpoints ────────────────────────────────────────────────

// GET /api/bills/:id/artifacts
func (h *BillHandler) ListArtifacts(c *gin.Context) {
	if h.artifactSvc == nil {
		c.JSON(http.StatusOK, gin.H{"data": []models.BillArtifact{}})
		return
	}
	billID := c.Param("id")
	items, err := h.artifactSvc.ListByBill(billID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if items == nil {
		items = []models.BillArtifact{}
	}
	c.JSON(http.StatusOK, gin.H{"data": items})
}

func (h *BillHandler) serveArtifact(c *gin.Context, inline bool) {
	if h.artifactSvc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "artifact service not configured"})
		return
	}
	billID := c.Param("id")
	artID := c.Param("artifact_id")

	data, art, err := h.artifactSvc.Read(artID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if art == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "artifact not found"})
		return
	}
	// Scope check: artifact must belong to the requested bill
	if art.BillID != billID {
		c.JSON(http.StatusNotFound, gin.H{"error": "artifact not found for this bill"})
		return
	}

	contentType := art.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	// All our text artifacts (email_html, JSON envelope, etc.) are stored
	// as UTF-8 bytes. Browsers default text/html / text/plain to Latin-1
	// when the Content-Type header has no charset, which mangles Thai
	// (e.g. "เรียน" → "à¹€à¸£à¸µà¸¢à¸™"). Backfill charset=utf-8 so
	// historical artifacts saved before the canonical fix still render.
	if (strings.HasPrefix(contentType, "text/") || contentType == "application/json") &&
		!strings.Contains(strings.ToLower(contentType), "charset=") {
		contentType = contentType + "; charset=utf-8"
	}
	disposition := "attachment"
	if inline {
		disposition = "inline"
	}
	c.Header("Content-Type", contentType)
	c.Header("Content-Disposition", fmt.Sprintf(`%s; filename="%s"`, disposition, art.Filename))
	c.Header("X-Content-SHA256", art.SHA256)
	c.Data(http.StatusOK, contentType, data)
}

// GET /api/bills/:id/artifacts/:artifact_id/download
func (h *BillHandler) DownloadArtifact(c *gin.Context) { h.serveArtifact(c, false) }

// GET /api/bills/:id/artifacts/:artifact_id/preview
func (h *BillHandler) PreviewArtifact(c *gin.Context) { h.serveArtifact(c, true) }
