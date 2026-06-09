package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	htmlstd "html"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/lib/pq"
	"go.uber.org/zap"

	"billflow/internal/models"
	"billflow/internal/repository"
	"billflow/internal/services/ai"
	emailservice "billflow/internal/services/email"
)

const (
	lazadaEmailSource       = "lazada_email"
	lazadaEmailFlow         = "lazada_email_purchase"
	lazadaAITextMaxRunes    = 12000
	lazadaBodyExcerptRunes  = 2000
	lazadaHighConfThreshold = 0.85
)

var (
	lazadaOrderIDPattern  = regexp.MustCompile(`(?i)(?:คำสั่งซื้อหมายเลข|order\s*(?:number|#)?)[^\d]{0,12}(\d{10,})`)
	lazadaAnyOrderPattern = regexp.MustCompile(`\b(\d{10,})\b`)
	lazadaAIOnce          sync.Once
	lazadaAISem           chan struct{}
	lazadaImgSrcPattern   = regexp.MustCompile(`(?is)<img\b[^>]*\bsrc\s*=\s*["']([^"']+)["'][^>]*>`)
)

type lazadaItemWithCandidates struct {
	item       models.BillItem
	candidates []models.CatalogMatch
}

// ProcessLazadaEmailBody handles Lazada Thailand purchase emails. It is
// intentionally separate from Lazada Excel (source='lazada') so retries,
// dashboards, and duplicate guards can distinguish IMAP-created purchase bills.
func (h *EmailHandler) ProcessLazadaEmailBody(subject, from, bodyText, bodyHTML, messageID string, source emailservice.MailSource) error {
	traceID := fmt.Sprintf("lazada-email-%d", time.Now().UnixMilli())
	startTime := time.Now()

	if h.aiClient == nil {
		return fmt.Errorf("AI client not configured")
	}
	if messageID != "" {
		var count int
		_ = h.billRepo.DB().QueryRow(
			`SELECT
			   (SELECT COUNT(*) FROM bills
			     WHERE source=$1
			       AND raw_data->>'email_message_id' = $2) +
			   (SELECT COUNT(*) FROM processed_email_keys
			     WHERE source=$1
			       AND message_id = $2
			       AND order_id = '')`,
			lazadaEmailSource, messageID,
		).Scan(&count)
		if count > 0 {
			h.logger.Info("lazada_email: skipping duplicate email before AI",
				zap.String("message_id", messageID),
				zap.Int("existing_bills", count),
			)
			return emailservice.SkipMessage("duplicate", "เมลนี้เคยประมวลผลแล้ว")
		}
	}

	plainText := prepareLazadaEmailText(bodyText, bodyHTML)
	if strings.TrimSpace(plainText) == "" {
		return emailservice.SkipMessage("lazada_no_new_bill", "เมล Lazada นี้ไม่มีเนื้อหาที่อ่านได้")
	}

	releaseSlot := acquireLazadaAISlot()
	orders, err := h.aiClient.ExtractLazadaOrders(plainText)
	releaseSlot()
	if err != nil || len(orders) == 0 {
		h.logger.Warn("lazada_email: AI extract failed or empty",
			zap.String("subject", subject), zap.Error(err))
		if err == nil {
			return fmt.Errorf("AI extract lazada_email: empty orders")
		}
		return fmt.Errorf("AI extract lazada_email: %w", err)
	}

	fallbackOrderID := extractLazadaOrderID(subject + "\n" + plainText)
	createdCount := 0
	skippedCount := 0
	failedCount := 0
	for _, order := range orders {
		if strings.TrimSpace(order.OrderID) == "" && fallbackOrderID != "" {
			order.OrderID = fallbackOrderID
		}
		created, err := h.processOneLazadaEmailOrder(
			order, subject, from, plainText, bodyHTML, messageID, traceID, startTime, source,
		)
		if err != nil {
			failedCount++
			h.logger.Warn("lazada_email: order processing failed",
				zap.String("order_id", order.OrderID), zap.Error(err))
			continue
		}
		if created {
			createdCount++
		} else {
			skippedCount++
		}
	}

	h.logger.Info("lazada_email: batch done",
		zap.String("trace_id", traceID),
		zap.Int("created", createdCount),
		zap.Int("skipped", skippedCount),
		zap.Int("failed", failedCount),
	)
	if messageID != "" && failedCount == 0 {
		_ = h.billRepo.MarkProcessedEmailKey(lazadaEmailSource, messageID, "")
	}
	return lazadaBatchCompletionError(createdCount, skippedCount, failedCount)
}

func lazadaBatchCompletionError(createdCount, skippedCount, failedCount int) error {
	if failedCount > 0 {
		return fmt.Errorf("lazada_email: %d order(s) failed after AI extract (created=%d skipped=%d)",
			failedCount, createdCount, skippedCount)
	}
	if createdCount == 0 && skippedCount > 0 {
		return emailservice.SkipMessage("duplicate_or_empty", "ไม่มีบิลใหม่จากเมล Lazada นี้ อาจซ้ำหรือไม่มีรายการสินค้าที่ใช้ได้")
	}
	return nil
}

func (h *EmailHandler) processOneLazadaEmailOrder(
	order ai.ExtractedOrder,
	subject, from, plainText, bodyHTML, messageID string,
	traceID string,
	startTime time.Time,
	source emailservice.MailSource,
) (bool, error) {
	orderID := normalizeLazadaOrderID(order.OrderID)
	if orderID == "" {
		h.logger.Warn("lazada_email: skipping order without order_id",
			zap.String("message_id", messageID),
			zap.String("subject", subject),
		)
		return false, nil
	}

	validItems := make([]ai.ExtractedItem, 0, len(order.Items))
	for _, extItem := range order.Items {
		extItem.RawName = strings.TrimSpace(extItem.RawName)
		if extItem.RawName == "" || extItem.Qty <= 0 {
			continue
		}
		validItems = append(validItems, extItem)
	}
	if len(validItems) == 0 {
		h.logger.Warn("lazada_email: skipping order without usable items",
			zap.String("message_id", messageID),
			zap.String("order_id", orderID),
			zap.String("subject", subject),
		)
		if messageID != "" {
			_ = h.billRepo.MarkProcessedEmailKey(lazadaEmailSource, messageID, orderID)
		}
		return false, nil
	}
	validItems = attachLazadaItemImages(validItems, bodyHTML)
	amountSummary := repository.ExtractLazadaAmountSummary(plainText, bodyHTML)
	validItems = applyLazadaEmailSummaryPrices(validItems, amountSummary)
	itemsGrossDelta := lazadaExtractedItemsGrossDelta(validItems, amountSummary)

	if existingBillID, exists, err := h.findExistingLazadaEmailBillID(orderID); err != nil {
		return false, fmt.Errorf("lookup existing lazada_email order: %w", err)
	} else if exists {
		if messageID != "" {
			_ = h.billRepo.MarkProcessedEmailKey(lazadaEmailSource, messageID, orderID)
		}
		h.saveLazadaEmailArtifacts(existingBillID, subject, from, plainText, bodyHTML, messageID)
		h.logger.Info("lazada_email: skipping duplicate order",
			zap.String("message_id", messageID),
			zap.String("order_id", orderID),
			zap.String("bill_id", existingBillID),
		)
		return false, nil
	}
	if messageID != "" {
		exists, err := h.billRepo.HasProcessedEmailKey(lazadaEmailSource, messageID, orderID)
		if err != nil {
			return false, fmt.Errorf("lookup processed lazada_email key: %w", err)
		}
		if exists {
			return false, nil
		}
	}

	itemsWithCandidates, allHighConfidence := h.mapLazadaItems(validItems)
	if len(itemsWithCandidates) == 0 {
		return false, nil
	}
	configReady := h.lazadaEmailChannelReady()
	feeConfigReady := h.lazadaEmailFeeConfigReady(amountSummary)
	amountReady := amountSummary.ReconciliationStatus == repository.LazadaReconciliationOK
	if amountSummary.HasGoodsTotalAmount && itemsGrossDelta != nil && absFloat(*itemsGrossDelta) > 0.01 {
		amountReady = false
	}
	status := "needs_review"
	if allHighConfidence && configReady && feeConfigReady && amountReady && order.Confidence >= lazadaHighConfThreshold {
		status = "pending"
	}
	applyLazadaEmailDiscounts(itemsWithCandidates, amountSummary)

	docDate := normalizeLazadaEmailDocDate(order.DocDate, source)
	sellerName := strings.TrimSpace(order.SellerName)
	if sellerName == "" {
		sellerName = "Lazada"
	}
	rawDataMap := map[string]interface{}{
		"subject":          subject,
		"from":             from,
		"email_message_id": messageID,
		"order_id":         orderID,
		"lazada_order_id":  orderID,
		"seller_name":      sellerName,
		"items":            validItems,
		"flow":             lazadaEmailFlow,
		"doc_date":         docDate,
		"body_excerpt":     truncateRunes(plainText, lazadaBodyExcerptRunes),
		"ai_text_chars":    len([]rune(plainText)),
		"config_ready":     configReady,
		"fee_config_ready": feeConfigReady,
	}
	applyLazadaAmountSummaryRawData(rawDataMap, amountSummary, itemsGrossDelta)
	if amountSummary.HasPaidTotalAmount {
		rawDataMap["total_amount"] = amountSummary.PaidTotalAmount
	} else if order.TotalAmount != nil {
		rawDataMap["total_amount"] = *order.TotalAmount
	}
	applyMailSource(rawDataMap, source)
	rawDataBytes, _ := json.Marshal(rawDataMap)

	conf := order.Confidence
	bill := &models.Bill{
		BillType:      "purchase",
		Source:        lazadaEmailSource,
		Status:        status,
		DocumentRoute: "purchaseorder",
		AIConfidence:  &conf,
		RawData:       json.RawMessage(rawDataBytes),
		SMLOrderID:    orderID,
	}
	if err := h.createLazadaEmailBillTx(bill, itemsWithCandidates, messageID, orderID); err != nil {
		if isUniqueViolation(err) {
			if messageID != "" {
				_ = h.billRepo.MarkProcessedEmailKey(lazadaEmailSource, messageID, orderID)
			}
			return false, nil
		}
		return false, fmt.Errorf("create lazada_email bill: %w", err)
	}

	h.saveLazadaEmailArtifacts(bill.ID, subject, from, plainText, bodyHTML, messageID)
	if h.auditRepo != nil {
		billIDStr := bill.ID
		durMs := int(time.Since(startTime).Milliseconds())
		_ = h.auditRepo.Log(models.AuditEntry{
			Action:     "lazada_email_received",
			TargetID:   &billIDStr,
			Source:     lazadaEmailSource,
			Level:      "info",
			TraceID:    traceID,
			DurationMs: &durMs,
			Detail: map[string]interface{}{
				"subject":       subject,
				"from":          from,
				"message_id":    messageID,
				"order_id":      orderID,
				"seller_name":   sellerName,
				"items_count":   len(itemsWithCandidates),
				"all_high_conf": allHighConfidence,
				"config_ready":  configReady,
				"fee_ready":     feeConfigReady,
				"amount_status": amountSummary.ReconciliationStatus,
				"status":        status,
			},
		})
	}
	h.adminNotify(fmt.Sprintf("Lazada Email: บิลซื้อรอตรวจสอบ\nOrder: %s (%s)\nItems: %d\nBill ID: %s",
		orderID, sellerName, len(itemsWithCandidates), bill.ID))
	h.logger.Info("lazada_email: bill created",
		zap.String("bill_id", bill.ID),
		zap.String("status", status),
		zap.String("order_id", orderID),
		zap.String("seller_name", sellerName),
		zap.Int("items", len(itemsWithCandidates)),
		zap.Bool("config_ready", configReady),
		zap.Bool("fee_ready", feeConfigReady),
		zap.String("amount_status", amountSummary.ReconciliationStatus),
	)
	return true, nil
}

func applyLazadaEmailSummaryPrices(items []ai.ExtractedItem, summary repository.LazadaAmountSummary) []ai.ExtractedItem {
	if !summary.HasGoodsTotalAmount || len(items) != 1 || items[0].Qty <= 0 {
		return items
	}
	out := make([]ai.ExtractedItem, len(items))
	copy(out, items)
	price := round2(summary.GoodsTotalAmount / out[0].Qty)
	out[0].Price = &price
	return out
}

func lazadaExtractedItemsGrossDelta(items []ai.ExtractedItem, summary repository.LazadaAmountSummary) *float64 {
	if !summary.HasGoodsTotalAmount {
		return nil
	}
	gross := 0.0
	hasPrice := false
	for _, item := range items {
		if item.Price == nil || item.Qty <= 0 {
			continue
		}
		hasPrice = true
		gross = round2(gross + item.Qty**item.Price)
	}
	if !hasPrice {
		return nil
	}
	delta := round2(gross - summary.GoodsTotalAmount)
	return &delta
}

func applyLazadaEmailDiscounts(items []lazadaItemWithCandidates, summary repository.LazadaAmountSummary) {
	if len(items) == 0 || summary.CouponDiscountAmount <= 0 {
		return
	}
	billItems := make([]models.BillItem, len(items))
	for i := range items {
		billItems[i] = items[i].item
	}
	repository.ApplyLazadaDiscountsToItems(billItems, summary.CouponDiscountAmount)
	for i := range items {
		items[i].item.DiscountAmount = billItems[i].DiscountAmount
	}
}

func applyLazadaAmountSummaryRawData(raw map[string]interface{}, summary repository.LazadaAmountSummary, itemsGrossDelta *float64) {
	raw["goods_total_amount"] = summary.GoodsTotalAmount
	raw["shipping_amount"] = summary.ShippingAmount
	raw["coupon_discount_amount"] = summary.CouponDiscountAmount
	raw["service_fee_amount"] = summary.ServiceFeeAmount
	raw["paid_total_amount"] = summary.PaidTotalAmount
	raw["shipping_method"] = summary.ShippingMethod
	raw["payment_method"] = summary.PaymentMethod
	raw["lazada_fee_amount"] = summary.FeeAmount()
	raw["amount_reconciliation_status"] = summary.ReconciliationStatus
	raw["amount_reconciliation_delta"] = summary.ReconciliationDelta
	raw["discount_summary"] = map[string]interface{}{
		"platform":               "lazada",
		"coupon_discount_amount": summary.CouponDiscountAmount,
		"total_discount_amount":  summary.TotalDiscountAmount,
		"allocation_method":      summary.AllocationMethod,
	}
	if itemsGrossDelta != nil {
		raw["items_gross_delta"] = *itemsGrossDelta
		if absFloat(*itemsGrossDelta) > 0.01 {
			raw["amount_reconciliation_status"] = repository.LazadaReconciliationMismatch
			raw["amount_reconciliation_warning"] = "ยอดรวมสินค้าในรายการไม่ตรงกับยอดรวมสินค้าจากอีเมล Lazada"
		}
	}
}

func (h *EmailHandler) mapLazadaItems(validItems []ai.ExtractedItem) ([]lazadaItemWithCandidates, bool) {
	const topK = 5
	out := make([]lazadaItemWithCandidates, 0, len(validItems))
	allHighConfidence := true
	for _, extItem := range validItems {
		var matches []models.CatalogMatch
		if h.embSvc != nil && h.embSvc.IsConfigured() && h.catalogIdx != nil && h.catalogIdx.Size() > 0 {
			queryEmb, err := h.embSvc.EmbedText(extItem.RawName)
			if err == nil {
				matches = h.catalogIdx.Search(queryEmb, topK)
			}
		}
		if len(matches) == 0 && h.catalogSvc != nil {
			matches, _ = h.catalogSvc.SearchByText(extItem.RawName, topK)
		}

		item := models.BillItem{
			RawName: extItem.RawName,
			Qty:     extItem.Qty,
			Mapped:  false,
		}
		if extItem.Price != nil {
			item.Price = extItem.Price
		}
		if strings.TrimSpace(extItem.Unit) != "" {
			unit := strings.TrimSpace(extItem.Unit)
			item.UnitCode = &unit
		}
		if extItem.ImageURL != "" {
			item.SourceImageURL = extItem.ImageURL
		}
		if len(matches) > 0 && matches[0].Score >= lazadaHighConfThreshold {
			item.ItemCode = &matches[0].ItemCode
			item.UnitCode = &matches[0].UnitCode
			item.Mapped = true
		} else {
			allHighConfidence = false
		}
		out = append(out, lazadaItemWithCandidates{item: item, candidates: matches})
	}
	return out, allHighConfidence
}

type lazadaImageRef struct {
	url   string
	start int
	end   int
}

func attachLazadaItemImages(items []ai.ExtractedItem, bodyHTML string) []ai.ExtractedItem {
	if strings.TrimSpace(bodyHTML) == "" || len(items) == 0 {
		return items
	}
	refs := lazadaProductImageRefs(bodyHTML)
	if len(refs) == 0 {
		return items
	}
	out := make([]ai.ExtractedItem, len(items))
	copy(out, items)
	for i := range out {
		if strings.TrimSpace(out[i].ImageURL) != "" {
			continue
		}
		pos := findLazadaItemHTMLPosition(bodyHTML, out[i].RawName)
		if pos < 0 {
			if len(out) == 1 {
				out[i].ImageURL = refs[0].url
			}
			continue
		}
		if url := nearestLazadaProductImageURL(refs, pos); url != "" {
			out[i].ImageURL = url
		}
	}
	return out
}

func lazadaProductImageRefs(bodyHTML string) []lazadaImageRef {
	matches := lazadaImgSrcPattern.FindAllStringSubmatchIndex(bodyHTML, -1)
	refs := make([]lazadaImageRef, 0, len(matches))
	for _, m := range matches {
		if len(m) < 4 || m[2] < 0 || m[3] < 0 {
			continue
		}
		url := htmlstd.UnescapeString(strings.TrimSpace(bodyHTML[m[2]:m[3]]))
		if !isLazadaProductImageURL(url) {
			continue
		}
		refs = append(refs, lazadaImageRef{url: url, start: m[0], end: m[1]})
	}
	return refs
}

func nearestLazadaProductImageURL(refs []lazadaImageRef, itemPos int) string {
	const maxBeforeDistance = 6000
	const maxAfterDistance = 2500

	bestURL := ""
	bestDistance := int(^uint(0) >> 1)
	for _, ref := range refs {
		if ref.end <= itemPos {
			dist := itemPos - ref.end
			if dist <= maxBeforeDistance && dist < bestDistance {
				bestDistance = dist
				bestURL = ref.url
			}
			continue
		}
		dist := ref.start - itemPos
		if dist >= 0 && dist <= maxAfterDistance && dist < bestDistance {
			bestDistance = dist
			bestURL = ref.url
		}
	}
	return bestURL
}

func isLazadaProductImageURL(raw string) bool {
	u := strings.ToLower(strings.TrimSpace(raw))
	if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
		return false
	}
	if !strings.Contains(u, "slatic.net/") {
		return false
	}
	if strings.Contains(u, "mmstat.com") ||
		strings.Contains(u, "/g/tps/") ||
		strings.Contains(u, "/tps/") ||
		strings.Contains(u, "imgextra") {
		return false
	}
	return strings.Contains(u, "/p/")
}

func findLazadaItemHTMLPosition(bodyHTML, rawName string) int {
	rawName = strings.Join(strings.Fields(strings.TrimSpace(rawName)), " ")
	if rawName == "" {
		return -1
	}
	candidates := []string{rawName}
	if short := firstRunes(rawName, 90); short != rawName {
		candidates = append(candidates, short)
	}
	if short := firstRunes(rawName, 45); short != rawName {
		candidates = append(candidates, short)
	}
	for _, candidate := range candidates {
		if idx := strings.Index(bodyHTML, candidate); idx >= 0 {
			return idx
		}
	}
	return -1
}

func firstRunes(s string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return string(runes[:limit])
}

func (h *EmailHandler) createLazadaEmailBillTx(
	bill *models.Bill,
	items []lazadaItemWithCandidates,
	messageID, orderID string,
) error {
	tx, err := h.billRepo.DB().Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	anomalies, _ := json.Marshal([]models.Anomaly{})
	var smlOrderID *string
	if bill.SMLOrderID != "" {
		smlOrderID = &bill.SMLOrderID
	}
	if err := tx.QueryRow(
		`INSERT INTO bills (bill_type, source, status, document_route, raw_data, ai_confidence, anomalies, created_by, sml_order_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 RETURNING id, created_at`,
		bill.BillType, bill.Source, bill.Status, bill.DocumentRoute, bill.RawData,
		bill.AIConfidence, anomalies, bill.CreatedBy, smlOrderID,
	).Scan(&bill.ID, &bill.CreatedAt); err != nil {
		return err
	}
	for _, iwc := range items {
		item := iwc.item
		candidatesJSON, _ := json.Marshal(iwc.candidates)
		if err := tx.QueryRow(
			`INSERT INTO bill_items (bill_id, raw_name, source_sku, source_image_url, item_code, qty, unit_code, price, discount_amount, mapped, mapping_id, candidates)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
			 RETURNING id`,
			bill.ID, item.RawName, item.SourceSKU, item.SourceImageURL, item.ItemCode, item.Qty,
			item.UnitCode, item.Price, item.DiscountAmount, item.Mapped, item.MappingID, candidatesJSON,
		).Scan(&item.ID); err != nil {
			return err
		}
	}
	if messageID != "" {
		if _, err := tx.Exec(
			`INSERT INTO processed_email_keys (source, message_id, order_id)
			 VALUES ($1, $2, $3)
			 ON CONFLICT (source, message_id, order_id) DO NOTHING`,
			lazadaEmailSource, messageID, orderID,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (h *EmailHandler) findExistingLazadaEmailBillID(orderID string) (string, bool, error) {
	if h == nil || h.billRepo == nil {
		return "", false, nil
	}
	normalized := normalizeLazadaOrderID(orderID)
	if normalized == "" {
		return "", false, nil
	}
	var id string
	err := h.billRepo.DB().QueryRow(
		`SELECT id::text
		   FROM bills
		  WHERE source = $1
		    AND archived_at IS NULL
		    AND (
		      UPPER(TRIM(COALESCE(sml_order_id, ''))) = $2
		      OR UPPER(TRIM(COALESCE(raw_data->>'order_id', ''))) = $2
		      OR UPPER(TRIM(COALESCE(raw_data->>'lazada_order_id', ''))) = $2
		    )
		  ORDER BY created_at ASC, id ASC
		  LIMIT 1`,
		lazadaEmailSource, strings.ToUpper(normalized),
	).Scan(&id)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", false, nil
		}
		return "", false, err
	}
	return id, true, nil
}

func (h *EmailHandler) lazadaEmailChannelReady() bool {
	if h == nil || h.channelDefaults == nil {
		return false
	}
	def, err := h.channelDefaults.Get(lazadaEmailSource, "purchase")
	if err != nil || def == nil {
		return false
	}
	return strings.TrimSpace(def.Endpoint) != "" &&
		strings.TrimSpace(def.DocFormatCode) != "" &&
		strings.TrimSpace(def.DocPrefix) != "" &&
		strings.TrimSpace(def.DocRunningFormat) != ""
}

func (h *EmailHandler) lazadaEmailFeeConfigReady(summary repository.LazadaAmountSummary) bool {
	if summary.FeeAmount() <= 0 {
		return true
	}
	if h == nil || h.channelDefaults == nil {
		return false
	}
	def, err := h.channelDefaults.Get(lazadaEmailSource, "purchase")
	if err != nil || def == nil {
		return false
	}
	return def.ShippingItemEnabled && strings.TrimSpace(def.ShippingItemCode) != ""
}

func (h *EmailHandler) saveLazadaEmailArtifacts(billID, subject, from, plainText, bodyHTML, messageID string) {
	if bodyHTML != "" {
		h.saveEmailArtifacts(billID, "email_html", "lazada-email.html", "text/html; charset=utf-8",
			[]byte(bodyHTML), subject, from, messageID)
		return
	}
	h.saveEmailArtifacts(billID, "email_text", "lazada-email.txt", "text/plain; charset=utf-8",
		[]byte(plainText), subject, from, messageID)
}

func prepareLazadaEmailText(bodyText, bodyHTML string) string {
	plain := htmlToText(bodyText)
	htmlPlain := htmlToText(bodyHTML)
	if len([]rune(htmlPlain)) > len([]rune(plain)) {
		plain = htmlPlain
	}
	plain = normalizeEmailText(plain)
	return truncateAroundLazadaOrder(plain, lazadaAITextMaxRunes)
}

func normalizeEmailText(text string) string {
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.Join(strings.Fields(strings.TrimSpace(line)), " ")
		if line != "" {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}

func truncateAroundLazadaOrder(text string, limit int) string {
	if limit <= 0 {
		return text
	}
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	idx := strings.Index(text, "คำสั่งซื้อหมายเลข")
	if idx < 0 {
		return string(runes[:limit])
	}
	prefixRunes := len([]rune(text[:idx]))
	start := prefixRunes - 1000
	if start < 0 {
		start = 0
	}
	end := start + limit
	if end > len(runes) {
		end = len(runes)
		start = end - limit
		if start < 0 {
			start = 0
		}
	}
	return string(runes[start:end])
}

func truncateRunes(text string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit])
}

func normalizeLazadaOrderID(orderID string) string {
	orderID = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(orderID), "#"))
	orderID = strings.Trim(orderID, " \t\r\n:")
	var b strings.Builder
	for _, r := range orderID {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	if b.Len() < 10 {
		return ""
	}
	return b.String()
}

func extractLazadaOrderID(text string) string {
	if m := lazadaOrderIDPattern.FindStringSubmatch(text); len(m) >= 2 {
		return normalizeLazadaOrderID(m[1])
	}
	if m := lazadaAnyOrderPattern.FindStringSubmatch(text); len(m) >= 2 {
		return normalizeLazadaOrderID(m[1])
	}
	return ""
}

func lazadaDocDateFromMailSource(source emailservice.MailSource) string {
	if strings.TrimSpace(source.EmailDate) == "" {
		return ""
	}
	if ts, err := time.Parse(time.RFC3339, source.EmailDate); err == nil {
		return ts.Format("2006-01-02")
	}
	return ""
}

func normalizeLazadaEmailDocDate(aiDocDate string, source emailservice.MailSource) string {
	mailDate := lazadaDocDateFromMailSource(source)
	if mailDate != "" {
		return mailDate
	}
	return normalizeLazadaDocDateString(aiDocDate)
}

func normalizeLazadaDocDateString(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	layouts := []string{
		time.RFC3339,
		"2006-01-02",
		"2006/01/02",
		"02/01/2006",
		"02-01-2006",
	}
	for _, layout := range layouts {
		if ts, err := time.Parse(layout, raw); err == nil {
			year := ts.Year()
			if year > 2400 {
				year -= 543
			}
			if year < 2000 || year > 2100 {
				return ""
			}
			return time.Date(year, ts.Month(), ts.Day(), 0, 0, 0, 0, time.UTC).Format("2006-01-02")
		}
	}
	if len(raw) >= 10 {
		return normalizeLazadaDocDateString(raw[:10])
	}
	return ""
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

func absFloat(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func acquireLazadaAISlot() func() {
	lazadaAIOnce.Do(func() {
		lazadaAISem = make(chan struct{}, lazadaAIConcurrencyLimit())
	})
	lazadaAISem <- struct{}{}
	return func() { <-lazadaAISem }
}

func lazadaAIConcurrencyLimit() int {
	raw := strings.TrimSpace(os.Getenv("LAZADA_EMAIL_AI_CONCURRENCY"))
	if raw == "" {
		return 2
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 2
	}
	if n < 1 {
		return 1
	}
	if n > 10 {
		return 10
	}
	return n
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == "23505" {
		return true
	}
	return false
}
