package handlers

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"billflow/internal/models"
	"billflow/internal/repository"
	"billflow/internal/services/ai"
	"billflow/internal/services/anomaly"
	"billflow/internal/services/catalog"
	lineservice "billflow/internal/services/line"
	"billflow/internal/services/mapper"
	"billflow/internal/services/mistral"
	"billflow/internal/services/sml"
)

// EmailHandler processes email attachments through the AI pipeline
// It is NOT an HTTP handler — it implements emailservice.AttachmentProcessor
type EmailHandler struct {
	aiClient   *ai.Client
	ocrClient  *mistral.OCRClient
	mapperSvc  *mapper.Service
	anomalySvc *anomaly.Service
	smlClient  *sml.Client
	billRepo   *repository.BillRepo
	auditRepo  *repository.AuditLogRepo
	lineSvc    *lineservice.Service
	threshold  float64
	logger     *zap.Logger
	// Catalog-based matching (Shopee email flow)
	catalogSvc  *catalog.SMLCatalogService
	embSvc      *catalog.EmbeddingService
	catalogIdx  *catalog.CatalogIndex
	catalogRepo *repository.SMLCatalogRepo
}

func NewEmailHandler(
	aiClient *ai.Client,
	ocrClient *mistral.OCRClient,
	mapperSvc *mapper.Service,
	anomalySvc *anomaly.Service,
	smlClient *sml.Client,
	billRepo *repository.BillRepo,
	auditRepo *repository.AuditLogRepo,
	lineSvc *lineservice.Service,
	threshold float64,
	logger *zap.Logger,
) *EmailHandler {
	return &EmailHandler{
		aiClient:   aiClient,
		ocrClient:  ocrClient,
		mapperSvc:  mapperSvc,
		anomalySvc: anomalySvc,
		smlClient:  smlClient,
		billRepo:   billRepo,
		auditRepo:  auditRepo,
		lineSvc:    lineSvc,
		threshold:  threshold,
		logger:     logger,
	}
}

// SetCatalogServices wires catalog-based search for Shopee email flow
func (h *EmailHandler) SetCatalogServices(
	catalogSvc *catalog.SMLCatalogService,
	embSvc *catalog.EmbeddingService,
	catalogIdx *catalog.CatalogIndex,
	catalogRepo *repository.SMLCatalogRepo,
) {
	h.catalogSvc = catalogSvc
	h.embSvc = embSvc
	h.catalogIdx = catalogIdx
	h.catalogRepo = catalogRepo
}

// ProcessAttachment is called once per qualifying email attachment.
// It satisfies emailservice.AttachmentProcessor signature.
func (h *EmailHandler) ProcessAttachment(data []byte, mimeType, filename, messageID string) error {
	// Use message-id as trace_id so all events for the same email are correlated.
	traceID := messageID
	if traceID == "" {
		traceID = fmt.Sprintf("email-%d", time.Now().UnixMilli())
	}
	attachStart := time.Now()

	// Deduplication: skip if we've already processed this email
	if messageID != "" {
		exists, err := h.billRepo.FindByEmailMessageID(messageID)
		if err != nil {
			h.logger.Warn("email: dedup check failed", zap.String("message_id", messageID), zap.Error(err))
		} else if exists {
			h.logger.Info("email: skipping duplicate", zap.String("message_id", messageID))
			return nil
		}
	}
	var extracted *ai.ExtractedBill
	var err error

	b64 := base64.StdEncoding.EncodeToString(data)

	switch {
	case strings.HasPrefix(mimeType, "image/"):
		extracted, err = h.aiClient.ExtractImage(b64, mimeType)
	case mimeType == "application/pdf" || strings.HasSuffix(strings.ToLower(filename), ".pdf"):
		if h.ocrClient != nil && h.ocrClient.IsConfigured() {
			// Use Mistral OCR to extract markdown text, then parse with OpenRouter
			var ocrText string
			ocrText, err = h.ocrClient.ExtractTextFromPDF(b64)
			if err == nil {
				extracted, err = h.aiClient.ExtractText(ocrText)
			}
		} else {
			extracted, err = h.aiClient.ExtractPDF(b64)
		}
	default:
		return fmt.Errorf("unsupported attachment type: %s", mimeType)
	}

	if err != nil {
		h.logger.Error("email: AI extract failed",
			zap.String("mime", mimeType), zap.String("file", filename), zap.Error(err))
		h.adminNotify(fmt.Sprintf("⚠️ Email AI extract failed\nFile: %s\nError: %s", filename, err.Error()))
		return err
	}

	if extracted == nil || len(extracted.Items) == 0 {
		h.logger.Warn("email: no items extracted", zap.String("file", filename))
		return fmt.Errorf("no items extracted from %s", filename)
	}

	return h.handleExtracted(extracted, filename, messageID, traceID, attachStart)
}

// handleExtracted runs mapper → anomaly, then leaves the bill pending for user confirmation.
// SML send happens later via the Retry handler when the user clicks "ส่ง SML" in the UI.
func (h *EmailHandler) handleExtracted(extracted *ai.ExtractedBill, filename, messageID, traceID string, startTime time.Time) error {
	var billItems []models.BillItem
	var itemCodes []string

	for _, extItem := range extracted.Items {
		match := h.mapperSvc.Match(extItem.RawName)

		item := models.BillItem{
			RawName: extItem.RawName,
			Qty:     extItem.Qty,
			Mapped:  match.Mapping != nil && !match.NeedsReview,
		}
		if extItem.Price != nil {
			item.Price = extItem.Price
		}

		if match.Mapping != nil {
			item.ItemCode = &match.Mapping.ItemCode
			item.UnitCode = &match.Mapping.UnitCode
			item.MappingID = &match.Mapping.ID
			itemCodes = append(itemCodes, match.Mapping.ItemCode)
		}

		billItems = append(billItems, item)
	}

	// F2 Anomaly detection
	avgPrices, maxQtys, _ := h.billRepo.GetPriceHistories(itemCodes)
	checkItems := make([]models.BillItem, len(billItems))
	copy(checkItems, billItems)
	anomalies := h.anomalySvc.Check(anomaly.CheckInput{
		Items:     checkItems,
		AvgPrices: avgPrices,
		MaxQtys:   maxQtys,
	})

	// Save bill
	conf := extracted.Confidence
	rawDataBytes, _ := json.Marshal(map[string]interface{}{
		"customer_name":    extracted.CustomerName,
		"customer_phone":   extracted.CustomerPhone,
		"note":             extracted.Note,
		"email_file":       filename,
		"email_message_id": messageID,
	})
	bill := &models.Bill{
		BillType:     "sale",
		Source:       "email",
		AIConfidence: &conf,
		RawData:      json.RawMessage(rawDataBytes),
	}
	if err := h.billRepo.Create(bill); err != nil {
		return fmt.Errorf("create bill: %w", err)
	}

	// Audit: bill received from email
	if h.auditRepo != nil {
		billIDStr := bill.ID
		durMs := int(time.Since(startTime).Milliseconds())
		_ = h.auditRepo.Log(models.AuditEntry{
			Action:     "bill_created",
			TargetID:   &billIDStr,
			Source:     "email",
			Level:      "info",
			TraceID:    traceID,
			DurationMs: &durMs,
			Detail: map[string]interface{}{
				"filename":      filename,
				"message_id":    messageID,
				"customer_name": extracted.CustomerName,
				"items_count":   len(extracted.Items),
				"confidence":    extracted.Confidence,
				"raw_data":      json.RawMessage(rawDataBytes),
			},
		})
	}

	if len(anomalies) > 0 {
		_ = h.billRepo.UpdateAnomalies(bill.ID, anomalies)
	}
	for i := range billItems {
		billItems[i].BillID = bill.ID
		_ = h.billRepo.InsertItem(&billItems[i])
	}

	// Manual-confirm flow: every email-sourced bill stays pending (or
	// needs_review when items aren't all mapped) until a user confirms it
	// in the UI and clicks "ส่ง SML". No auto-send anymore.
	allMapped := true
	for _, item := range billItems {
		if !item.Mapped {
			allMapped = false
			break
		}
	}

	status := "pending"
	if !allMapped {
		status = "needs_review"
	}
	_ = h.billRepo.UpdateStatus(bill.ID, status, nil, nil, nil)

	if h.auditRepo != nil {
		billIDStr := bill.ID
		durMs := int(time.Since(startTime).Milliseconds())
		_ = h.auditRepo.Log(models.AuditEntry{
			Action:     "bill_pending",
			TargetID:   &billIDStr,
			Source:     "email",
			Level:      "info",
			TraceID:    traceID,
			DurationMs: &durMs,
			Detail: map[string]interface{}{
				"filename":      filename,
				"all_mapped":    allMapped,
				"confidence":    extracted.Confidence,
				"anomaly_count": len(anomalies),
				"status":        status,
			},
		})
	}
	h.adminNotify(fmt.Sprintf("📋 Email bill pending review\nBill: %s\nFile: %s\nStatus: %s\nConfidence: %.2f\nAnomalies: %d",
		bill.ID, filename, status, extracted.Confidence, len(anomalies)))

	return nil
}

func (h *EmailHandler) adminNotify(msg string) {
	if h.lineSvc != nil {
		_ = h.lineSvc.PushAdmin(msg)
	}
}

// ProcessShopeeEmailBody handles Shopee order confirmation emails.
// bodyText is the raw HTML (or plain text) from the email body.
// This method:
//  1. Uses AI to extract order items from the HTML body
//  2. Runs catalog similarity search for each item
//  3. Creates a bill with source='shopee_email'
//     - status='needs_review' if any item has low confidence
//     - status='pending' if all items are high confidence (and sends to SML)
func (h *EmailHandler) ProcessShopeeEmailBody(subject, from, bodyText, messageID string) error {
	traceID := fmt.Sprintf("shopee-email-%d", time.Now().UnixMilli())
	startTime := time.Now()

	// Deduplication check
	if messageID != "" {
		exists, err := h.billRepo.FindByEmailMessageID(messageID)
		if err != nil {
			h.logger.Warn("shopee_email: dedup check failed", zap.String("message_id", messageID), zap.Error(err))
		} else if exists {
			h.logger.Info("shopee_email: skipping duplicate", zap.String("message_id", messageID))
			return nil
		}
	}

	if h.catalogSvc == nil {
		h.logger.Warn("shopee_email: catalog service not configured — skipping")
		return fmt.Errorf("catalog service not configured")
	}

	// Use AI to extract order info from HTML body
	// Strip HTML tags for cleaner extraction
	plainText := htmlToText(bodyText)
	extracted, err := h.aiClient.ExtractText(plainText)
	if err != nil || extracted == nil || len(extracted.Items) == 0 {
		h.logger.Warn("shopee_email: AI extract failed or empty",
			zap.String("subject", subject), zap.Error(err))
		return fmt.Errorf("AI extract shopee email: %w", err)
	}

	// Extract Shopee order ID from subject (e.g. "คำสั่งซื้อ #2501234567890")
	shopeeOrderID := extractShopeeOrderID(subject)

	// Dedup by order ID
	if shopeeOrderID != "" {
		existsByOrderID, _ := h.billRepo.FindByShopeeOrderID(shopeeOrderID)
		if existsByOrderID {
			h.logger.Info("shopee_email: skipping duplicate order", zap.String("order_id", shopeeOrderID))
			return nil
		}
	}

	// For each extracted item, run catalog search
	type itemWithCandidates struct {
		item       models.BillItem
		candidates []models.CatalogMatch
		topMatch   *models.CatalogMatch
	}

	var itemsWithCandidates []itemWithCandidates
	allHighConfidence := true
	const topK = 5
	const highConfThreshold = 0.85

	for _, extItem := range extracted.Items {
		var matches []models.CatalogMatch

		// Try embedding search first
		if h.embSvc != nil && h.embSvc.IsConfigured() && h.catalogIdx != nil && h.catalogIdx.Size() > 0 {
			queryEmb, err := h.embSvc.EmbedText(extItem.RawName)
			if err == nil {
				matches = h.catalogIdx.Search(queryEmb, topK)
			}
		}

		// Fallback to text search
		if len(matches) == 0 {
			matches, _ = h.catalogSvc.SearchByText(extItem.RawName, topK)
		}

		price := 0.0
		if extItem.Price != nil {
			price = *extItem.Price
		}

		item := models.BillItem{
			RawName: extItem.RawName,
			Qty:     extItem.Qty,
			Mapped:  false,
		}
		if extItem.Price != nil {
			item.Price = extItem.Price
		}

		var topMatch *models.CatalogMatch
		if len(matches) > 0 {
			if matches[0].Score >= highConfThreshold {
				topMatch = &matches[0]
				item.ItemCode = &matches[0].ItemCode
				item.UnitCode = &matches[0].UnitCode
				item.Mapped = true
				_ = price // keep the assignment for SML payload
			} else {
				allHighConfidence = false
			}
		} else {
			allHighConfidence = false
		}

		itemsWithCandidates = append(itemsWithCandidates, itemWithCandidates{
			item:       item,
			candidates: matches,
			topMatch:   topMatch,
		})
	}

	// Build raw_data payload
	rawDataMap := map[string]interface{}{
		"subject":          subject,
		"from":             from,
		"email_message_id": messageID,
		"shopee_order_id":  shopeeOrderID,
		"customer_name":    extracted.CustomerName,
		"customer_phone":   extracted.CustomerPhone,
		"note":             extracted.Note,
	}
	rawDataBytes, _ := json.Marshal(rawDataMap)

	// Determine bill status
	status := "needs_review"
	if allHighConfidence {
		status = "pending"
	}

	conf := extracted.Confidence
	bill := &models.Bill{
		BillType:     "sale",
		Source:       "shopee_email",
		Status:       status,
		AIConfidence: &conf,
		RawData:      json.RawMessage(rawDataBytes),
		SMLOrderID:   shopeeOrderID,
	}
	if err := h.billRepo.Create(bill); err != nil {
		return fmt.Errorf("create shopee_email bill: %w", err)
	}

	// Insert bill items with candidates
	for _, iwc := range itemsWithCandidates {
		item := iwc.item
		item.BillID = bill.ID

		// Store top-5 candidates as JSON
		candidatesJSON, _ := json.Marshal(iwc.candidates)

		_ = h.billRepo.InsertItemWithCandidates(&item, candidatesJSON)
	}

	// Audit log
	if h.auditRepo != nil {
		billIDStr := bill.ID
		durMs := int(time.Since(startTime).Milliseconds())
		_ = h.auditRepo.Log(models.AuditEntry{
			Action:     "shopee_email_received",
			TargetID:   &billIDStr,
			Source:     "shopee_email",
			Level:      "info",
			TraceID:    traceID,
			DurationMs: &durMs,
			Detail: map[string]interface{}{
				"subject":       subject,
				"from":          from,
				"message_id":    messageID,
				"order_id":      shopeeOrderID,
				"items_count":   len(itemsWithCandidates),
				"all_high_conf": allHighConfidence,
				"status":        status,
			},
		})
	}

	if status == "needs_review" {
		h.adminNotify(fmt.Sprintf("🛒 Shopee Email: บิลรอยืนยัน\nSubject: %s\nOrder: %s\nItems: %d\nBill ID: %s",
			subject, shopeeOrderID, len(itemsWithCandidates), bill.ID))
	}

	h.logger.Info("shopee_email: bill created",
		zap.String("bill_id", bill.ID),
		zap.String("status", status),
		zap.String("order_id", shopeeOrderID),
		zap.Int("items", len(itemsWithCandidates)),
	)

	return nil
}

// htmlToText strips HTML tags for cleaner AI extraction
func htmlToText(html string) string {
	// Replace common block tags with newlines
	replacer := strings.NewReplacer(
		"<br>", "\n", "<br/>", "\n", "<br />", "\n",
		"</p>", "\n", "</div>", "\n", "</tr>", "\n",
		"</td>", " ", "</th>", " ",
		"&nbsp;", " ", "&amp;", "&", "&lt;", "<", "&gt;", ">",
	)
	text := replacer.Replace(html)

	// Remove remaining tags
	var result strings.Builder
	inTag := false
	for _, r := range text {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			result.WriteRune(r)
		}
	}

	// Collapse multiple spaces/newlines
	lines := strings.Split(result.String(), "\n")
	var cleaned []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			cleaned = append(cleaned, line)
		}
	}
	return strings.Join(cleaned, "\n")
}

// extractShopeeOrderID tries to find the order number from the email subject
// Shopee subjects are like "คำสั่งซื้อ #250123456789012" or "Order #250123456789012 shipped"
func extractShopeeOrderID(subject string) string {
	// Look for "#" followed by alphanumeric characters (Shopee uses both
	// pure-digit and alphanumeric IDs, e.g. "260404V08VQU10").
	idx := strings.Index(subject, "#")
	if idx < 0 {
		return ""
	}
	rest := subject[idx+1:]
	end := 0
	for end < len(rest) {
		c := rest[end]
		if (c >= '0' && c <= '9') || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
			end++
		} else {
			break
		}
	}
	if end > 0 {
		return rest[:end]
	}
	return ""
}
