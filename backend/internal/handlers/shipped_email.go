package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"go.uber.org/zap"

	"billflow/internal/models"
	"billflow/internal/repository"
	"billflow/internal/services/ai"
	emailservice "billflow/internal/services/email"
)

const (
	shopeeAITextBudgetChars = 25000
	shopeeAIHTMLBudgetChars = 12000
	shopeeAIChunkOrderLimit = 3
)

var shopeeOrderIDTokenPattern = regexp.MustCompile(`#([0-9A-Za-z]{8,})`)

type shopeeOrderExtractor interface {
	ExtractOrdersCompact(text string) ([]ai.ExtractedOrder, error)
	ExtractOrdersWithHTML(text, html string) ([]ai.ExtractedOrder, error)
}

func (h *EmailHandler) shopeeOrderExtractor() shopeeOrderExtractor {
	if h.shopeeAI != nil {
		return h.shopeeAI
	}
	if h.aiClient != nil {
		return h.aiClient
	}
	return nil
}

// ProcessShopeeShippedEmailBody handles Shopee purchase-related emails.
// Payment confirmations create purchase bills. Shipping confirmations are
// status-only and never create bills because they are not reliable PO source
// documents.
func (h *EmailHandler) ProcessShopeeShippedEmailBody(subject, from, bodyText, bodyHTML, messageID string, source emailservice.MailSource) (emailservice.ProcessOutcome, error) {
	traceID := fmt.Sprintf("shopee-shipped-%d", time.Now().UnixMilli())
	startTime := time.Now()

	// bodyText is already plain text (extractBodyText prefers text/plain).
	// htmlToText is a no-op when input has no HTML tags, so it's safe to call.
	plainText := htmlToText(bodyText)
	if strings.TrimSpace(plainText) == "" {
		plainText = htmlToText(bodyHTML)
	}
	eventType, _, subjectOrderID, _ := shopeeOrderEventFromSubject(subject)
	if eventType == shopeeEventShipped {
		return h.processShopeeShippingStatusEmail(subject, from, bodyText, bodyHTML, messageID, source, subjectOrderID)
	}
	detectedOrderIDs := detectedShopeeBodyOrderIDs(plainText, bodyHTML)
	isMultiOrderEmail := len(detectedOrderIDs) > 1

	if messageID != "" && !isMultiOrderEmail {
		var count int
		_ = h.billRepo.DB().QueryRow(
			`SELECT
			   (SELECT COUNT(*) FROM bills
			     WHERE source='shopee_shipped'
			       AND raw_data->>'email_message_id' = $1) +
			   (SELECT COUNT(*) FROM processed_email_keys
			     WHERE source='shopee_shipped'
			       AND message_id = $1
			       AND order_id = '')`,
			messageID,
		).Scan(&count)
		if count > 0 {
			h.logger.Info("shopee_shipped: skipping duplicate email before AI",
				zap.String("message_id", messageID),
				zap.Int("existing_bills", count),
			)
			return emailservice.ProcessOutcome{}, emailservice.SkipMessage("duplicate", "เมลนี้เคยประมวลผลแล้ว")
		}
	}

	statusEventUpdated := false
	if updated, err := h.recordShopeeStatusEventBeforeAI(subject, from, plainText, bodyHTML, messageID, source, !isMultiOrderEmail); err != nil {
		return emailservice.ProcessOutcome{}, err
	} else if updated {
		statusEventUpdated = true
		if !isMultiOrderEmail {
			return emailservice.UpdatedExistingOutcome("status_event_recorded", "บันทึกสถานะบนบิลเดิมแล้ว"), nil
		}
	}

	if h.catalogSvc == nil {
		h.logger.Warn("shopee_shipped: catalog service not configured — skipping")
		return emailservice.ProcessOutcome{}, fmt.Errorf("catalog service not configured")
	}
	if h.shopeeOrderExtractor() == nil {
		return emailservice.ProcessOutcome{}, fmt.Errorf("AI client not configured")
	}

	// AI extracts all orders from bounded input. Small emails can include HTML
	// for image URLs; large/status emails use compact text-only jobs.
	orders, err := h.extractShopeeOrdersBounded(subject, plainText, bodyHTML, traceID)
	if err != nil || len(orders) == 0 {
		h.logger.Warn("shopee_shipped: AI extract failed or empty",
			zap.String("subject", subject), zap.Error(err))
		if err == nil {
			return emailservice.ProcessOutcome{}, fmt.Errorf("AI extract shopee_shipped: empty orders")
		}
		return emailservice.ProcessOutcome{}, fmt.Errorf("AI extract shopee_shipped: %w", err)
	}
	if missing := missingShopeeExtractedOrderIDs(detectedOrderIDs, orders); len(missing) > 0 {
		h.logger.Warn("shopee_shipped: AI extract incomplete",
			zap.String("trace_id", traceID),
			zap.Int("detected_order_count", len(detectedOrderIDs)),
			zap.Int("extracted_order_count", len(orders)),
			zap.Strings("missing_order_ids", missing),
		)
		return emailservice.ProcessOutcome{}, fmt.Errorf("AI extract shopee_shipped: incomplete orders, missing %s", strings.Join(missing, ","))
	}

	h.logger.Info("shopee_shipped: orders extracted",
		zap.String("trace_id", traceID),
		zap.Int("order_count", len(orders)),
	)

	// Per-item prices parsed from the email body — fallback for AI nulls.
	fallbackPrices := extractShopeePrices(plainText)

	createdCount := 0
	skippedCount := 0
	failedCount := 0
	for _, order := range orders {
		created, err := h.processOneShippedOrder(
			order, subject, from, bodyText, bodyHTML, messageID, fallbackPrices, traceID, startTime, source,
		)
		if err != nil {
			failedCount++
			h.logger.Warn("shopee_shipped: order processing failed",
				zap.String("order_id", order.OrderID), zap.Error(err))
		}
		if created {
			createdCount++
		} else {
			skippedCount++
		}
	}

	h.logger.Info("shopee_shipped: batch done",
		zap.String("trace_id", traceID),
		zap.Int("created", createdCount),
		zap.Int("skipped", skippedCount),
		zap.Int("failed", failedCount),
	)
	if messageID != "" && failedCount == 0 {
		_ = h.billRepo.MarkProcessedEmailKey("shopee_shipped", messageID, "")
	}
	if createdCount == 0 && statusEventUpdated && failedCount == 0 {
		return emailservice.UpdatedExistingOutcome("status_event_recorded", "บันทึกสถานะบนบิลเดิมแล้ว"), nil
	}
	if createdCount == 0 && skippedCount > 0 && failedCount == 0 {
		return emailservice.ProcessOutcome{}, emailservice.SkipMessage("duplicate_or_empty", "ไม่มีบิลใหม่จากเมลนี้ อาจซ้ำหรือไม่มีรายการสินค้าที่ใช้ได้")
	}
	return emailservice.CreatedBillOutcome(), nil
}

func (h *EmailHandler) processShopeeShippingStatusEmail(subject, from, bodyText, bodyHTML, messageID string, source emailservice.MailSource, orderID string) (emailservice.ProcessOutcome, error) {
	orderID = normalizeShopeeOrderID(orderID)
	if orderID == "" {
		if messageID != "" {
			_ = h.billRepo.MarkProcessedEmailKey("shopee_shipped", messageID, "")
		}
		return emailservice.SkippedOutcome("shopee_shipping_missing_order_id", "อีเมลจัดส่ง Shopee ไม่มีเลขคำสั่งซื้อที่ระบบอ่านได้"), nil
	}

	existingBillID, exists, err := h.findExistingShopeeShippedBillID(orderID)
	if err != nil {
		return emailservice.ProcessOutcome{}, fmt.Errorf("lookup existing shopee_shipped order for shipping event: %w", err)
	}
	if exists {
		h.recordShopeeOrderEvent(existingBillID, subject, from, messageID, source, orderID)
		h.linkShopeeOrphanEventsToBill(existingBillID, orderID)
		h.saveShopeeShippedEmailArtifacts(existingBillID, subject, from, bodyText, bodyHTML, messageID)
		if messageID != "" {
			_ = h.billRepo.MarkProcessedEmailKey("shopee_shipped", messageID, orderID)
			_ = h.billRepo.MarkProcessedEmailKey("shopee_shipped", messageID, "")
		}
		h.logger.Info("shopee_shipped: recorded shipping status on existing bill",
			zap.String("message_id", messageID),
			zap.String("order_id", orderID),
			zap.String("bill_id", existingBillID),
		)
		return emailservice.UpdatedExistingOutcome("shopee_shipping_status_recorded", "บันทึกสถานะจัดส่งบนบิลเดิมแล้ว"), nil
	}

	h.recordShopeeOrderEvent("", subject, from, messageID, source, orderID)
	if messageID != "" {
		_ = h.billRepo.MarkProcessedEmailKey("shopee_shipped", messageID, orderID)
		_ = h.billRepo.MarkProcessedEmailKey("shopee_shipped", messageID, "")
	}
	h.logger.Info("shopee_shipped: skipped shipping email without payment bill",
		zap.String("message_id", messageID),
		zap.String("order_id", orderID),
	)
	return emailservice.SkippedOutcome("shopee_shipped_without_payment_bill", "อีเมลจัดส่ง: รออีเมลยืนยันการชำระเงินก่อนสร้างบิล"), nil
}

func (h *EmailHandler) recordShopeeStatusEventBeforeAI(subject, from, bodyText, bodyHTML, messageID string, source emailservice.MailSource, markEmailProcessed bool) (bool, error) {
	eventType, _, subjectOrderID, ok := shopeeOrderEventFromSubject(subject)
	if !ok || subjectOrderID == "" {
		return false, nil
	}
	existingBillID, exists, err := h.findExistingShopeeShippedBillID(subjectOrderID)
	if err != nil {
		return false, fmt.Errorf("lookup existing shopee_shipped order before AI: %w", err)
	}
	if !exists {
		return false, nil
	}

	h.recordShopeeOrderEvent(existingBillID, subject, from, messageID, source, subjectOrderID)
	h.linkShopeeOrphanEventsToBill(existingBillID, subjectOrderID)
	h.saveShopeeShippedEmailArtifacts(existingBillID, subject, from, bodyText, bodyHTML, messageID)
	discountSummary := repository.ExtractShopeeDiscountSummary(bodyText, bodyHTML, subjectOrderID)
	if ok, err := h.billRepo.ApplyShopeePurchaseDiscountsToBill(existingBillID, discountSummary); err != nil {
		h.logger.Warn("shopee_shipped: pre-AI existing bill discount update failed",
			zap.String("message_id", messageID),
			zap.String("order_id", subjectOrderID),
			zap.String("bill_id", existingBillID),
			zap.Error(err))
	} else if ok {
		h.logger.Info("shopee_shipped: pre-AI updated existing bill discounts",
			zap.String("message_id", messageID),
			zap.String("order_id", subjectOrderID),
			zap.String("bill_id", existingBillID),
			zap.Float64("discount", discountSummary.TotalDiscountAmount))
	}
	paymentSummary := repository.ExtractShopeePaymentSummary(bodyText, bodyHTML, subjectOrderID)
	if ok, err := h.billRepo.ApplyShopeePurchasePaymentSummaryToBill(existingBillID, paymentSummary); err != nil {
		h.logger.Warn("shopee_shipped: pre-AI existing bill payment summary update failed",
			zap.String("message_id", messageID),
			zap.String("order_id", subjectOrderID),
			zap.String("bill_id", existingBillID),
			zap.Error(err))
	} else if ok {
		h.logger.Info("shopee_shipped: pre-AI updated existing bill payment summary",
			zap.String("message_id", messageID),
			zap.String("order_id", subjectOrderID),
			zap.String("bill_id", existingBillID),
			zap.String("payment_method", paymentSummary.PaymentMethod))
	}
	if messageID != "" {
		_ = h.billRepo.MarkProcessedEmailKey("shopee_shipped", messageID, subjectOrderID)
		if markEmailProcessed {
			_ = h.billRepo.MarkProcessedEmailKey("shopee_shipped", messageID, "")
		}
	}
	h.logger.Info("shopee_shipped: recorded status event on existing bill before AI",
		zap.String("message_id", messageID),
		zap.String("order_id", subjectOrderID),
		zap.String("event_type", eventType),
		zap.String("bill_id", existingBillID),
	)
	return true, nil
}

type shopeeExtractionJob struct {
	Text    string
	HTML    string
	Compact bool
	OrderID string
}

func (h *EmailHandler) extractShopeeOrdersBounded(subject, plainText, bodyHTML, traceID string) ([]ai.ExtractedOrder, error) {
	jobs := buildShopeeExtractionJobs(subject, plainText, bodyHTML)
	if len(jobs) == 0 {
		return nil, fmt.Errorf("AI extract shopee_shipped: empty extraction input")
	}
	extractor := h.shopeeOrderExtractor()
	if extractor == nil {
		return nil, fmt.Errorf("AI client not configured")
	}
	ordersByID := map[string]ai.ExtractedOrder{}
	orderSequence := []string{}
	for i, job := range jobs {
		h.logger.Info("shopee_shipped: AI extraction job",
			zap.String("trace_id", traceID),
			zap.Int("job_index", i+1),
			zap.Int("job_count", len(jobs)),
			zap.Bool("compact", job.Compact),
			zap.String("order_id", job.OrderID),
			zap.Int("text_chars", len([]rune(job.Text))),
			zap.Int("html_chars", len([]rune(job.HTML))),
		)
		var (
			orders []ai.ExtractedOrder
			err    error
		)
		if job.Compact {
			orders, err = extractor.ExtractOrdersCompact(job.Text)
		} else {
			orders, err = extractor.ExtractOrdersWithHTML(job.Text, job.HTML)
		}
		if err != nil {
			return nil, fmt.Errorf("job %d/%d: %w", i+1, len(jobs), err)
		}
		for _, order := range orders {
			canonical := normalizeShopeeOrderID(order.OrderID)
			if canonical == "" {
				continue
			}
			order.OrderID = canonical
			if _, exists := ordersByID[canonical]; !exists {
				orderSequence = append(orderSequence, canonical)
			}
			ordersByID[canonical] = order
		}
	}
	out := make([]ai.ExtractedOrder, 0, len(orderSequence))
	for _, orderID := range orderSequence {
		out = append(out, ordersByID[orderID])
	}
	return out, nil
}

func buildShopeeExtractionJobs(subject, plainText, bodyHTML string) []shopeeExtractionJob {
	plainText = strings.TrimSpace(plainText)
	bodyHTML = strings.TrimSpace(bodyHTML)
	detectionText := shopeeOrderDetectionText(plainText, bodyHTML)
	if plainText == "" {
		plainText = detectionText
	}

	matches := uniqueShopeeOrderIDMatches(detectionText)
	if len(matches) > 1 {
		jobs := []shopeeExtractionJob{}
		for start := 0; start < len(matches); start += shopeeAIChunkOrderLimit {
			end := start + shopeeAIChunkOrderLimit
			if end > len(matches) {
				end = len(matches)
			}
			parts := []string{}
			orderIDs := []string{}
			for _, m := range matches[start:end] {
				block := scopeShopeeTextToOrder(detectionText, m.id)
				if block == "" {
					continue
				}
				parts = append(parts, block)
				orderIDs = append(orderIDs, m.id)
			}
			text := strings.Join(parts, "\n\n--- NEXT ORDER ---\n\n")
			text, _ = clampRunes(text, shopeeAITextBudgetChars)
			if strings.TrimSpace(text) == "" {
				continue
			}
			jobs = append(jobs, shopeeExtractionJob{
				Text:    text,
				Compact: true,
				OrderID: strings.Join(orderIDs, ","),
			})
		}
		if len(jobs) > 0 {
			return jobs
		}
	}

	subjectOrderID := normalizeShopeeOrderID(extractShopeeOrderID(subject))
	if subjectOrderID != "" {
		scopedText := scopeShopeeTextToOrder(plainText, subjectOrderID)
		if scopedText == "" {
			scopedText = scopeShopeeTextToOrder(detectionText, subjectOrderID)
		}
		if scopedText == "" {
			scopedText = plainText
		}
		scopedText, textClipped := clampRunes(scopedText, shopeeAITextBudgetChars)
		scopedHTML, htmlClipped := clampRunes(scopeShopeeHTMLToOrder(bodyHTML, subjectOrderID), shopeeAIHTMLBudgetChars)
		compact := textClipped || htmlClipped || scopedHTML == ""
		if compact {
			scopedHTML = ""
		}
		return []shopeeExtractionJob{{
			Text:    scopedText,
			HTML:    scopedHTML,
			Compact: compact,
			OrderID: subjectOrderID,
		}}
	}

	text, textClipped := clampRunes(plainText, shopeeAITextBudgetChars)
	html, htmlClipped := clampRunes(bodyHTML, shopeeAIHTMLBudgetChars)
	compact := textClipped || htmlClipped || html == ""
	if compact {
		html = ""
	}
	return []shopeeExtractionJob{{Text: text, HTML: html, Compact: compact}}
}

type shopeeOrderIDMatch struct {
	id    string
	start int
	end   int
}

func uniqueShopeeOrderIDMatches(text string) []shopeeOrderIDMatch {
	raw := shopeeOrderIDTokenPattern.FindAllStringSubmatchIndex(text, -1)
	out := []shopeeOrderIDMatch{}
	seen := map[string]bool{}
	for _, m := range raw {
		if len(m) < 4 {
			continue
		}
		id := normalizeShopeeOrderID(text[m[2]:m[3]])
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, shopeeOrderIDMatch{id: id, start: m[0], end: m[1]})
	}
	return out
}

func shopeeOrderDetectionText(plainText, bodyHTML string) string {
	plainText = strings.TrimSpace(plainText)
	htmlText := strings.TrimSpace(htmlToText(bodyHTML))
	switch {
	case plainText == "":
		return htmlText
	case htmlText == "":
		return plainText
	default:
		return plainText + "\n" + htmlText
	}
}

func detectedShopeeBodyOrderIDs(plainText, bodyHTML string) []string {
	matches := uniqueShopeeOrderIDMatches(shopeeOrderDetectionText(plainText, bodyHTML))
	ids := make([]string, 0, len(matches))
	for _, m := range matches {
		ids = append(ids, m.id)
	}
	return ids
}

func DetectShopeeBodyOrderIDs(plainText, bodyHTML string) []string {
	return detectedShopeeBodyOrderIDs(plainText, bodyHTML)
}

func missingShopeeExtractedOrderIDs(expected []string, orders []ai.ExtractedOrder) []string {
	if len(expected) <= 1 {
		return nil
	}
	extracted := map[string]bool{}
	for _, order := range orders {
		id := normalizeShopeeOrderID(order.OrderID)
		if id != "" {
			extracted[id] = true
		}
	}
	missing := []string{}
	for _, id := range expected {
		if id != "" && !extracted[id] {
			missing = append(missing, id)
		}
	}
	return missing
}

func scopeShopeeTextToOrder(text, orderID string) string {
	text = strings.TrimSpace(text)
	orderID = normalizeShopeeOrderID(orderID)
	if text == "" || orderID == "" {
		return text
	}
	matches := uniqueShopeeOrderIDMatches(text)
	if len(matches) == 0 {
		return text
	}
	target := -1
	for i, m := range matches {
		if m.id == orderID {
			target = i
			break
		}
	}
	if target < 0 {
		return text
	}
	start := matches[target].start
	end := len(text)
	if target+1 < len(matches) {
		end = matches[target+1].start
	}
	if start < 0 || end <= start || end > len(text) {
		return text
	}
	return strings.TrimSpace(text[start:end])
}

func scopeShopeeHTMLToOrder(html, orderID string) string {
	html = strings.TrimSpace(html)
	orderID = normalizeShopeeOrderID(orderID)
	if html == "" || orderID == "" {
		return html
	}
	upper := strings.ToUpper(html)
	idx := strings.Index(upper, "#"+orderID)
	if idx < 0 {
		idx = strings.Index(upper, orderID)
	}
	if idx < 0 {
		return html
	}
	start := idx - shopeeAIHTMLBudgetChars/2
	if start < 0 {
		start = 0
	}
	end := idx + shopeeAIHTMLBudgetChars/2
	if end > len(html) {
		end = len(html)
	}
	return strings.TrimSpace(html[start:end])
}

func clampRunes(s string, max int) (string, bool) {
	s = strings.TrimSpace(s)
	if max <= 0 {
		return s, false
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s, false
	}
	return string(runes[:max]), true
}

// processOneShippedOrder creates a single purchase bill for one Shopee order.
// Returns (true, nil) when the bill was created, (false, nil) when skipped (dedup).
func (h *EmailHandler) processOneShippedOrder(
	order ai.ExtractedOrder,
	subject, from, bodyText, bodyHTML, messageID string,
	fallbackPrices []float64,
	traceID string,
	startTime time.Time,
	source emailservice.MailSource,
) (bool, error) {
	orderID := normalizeShopeeOrderID(order.OrderID)
	if orderID == "" || strings.EqualFold(orderID, "#unknown") {
		h.logger.Warn("shopee_shipped: skipping order without order_id",
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
		h.logger.Warn("shopee_shipped: skipping order without usable items",
			zap.String("message_id", messageID),
			zap.String("order_id", orderID),
			zap.String("subject", subject),
		)
		if messageID != "" {
			_ = h.billRepo.MarkProcessedEmailKey("shopee_shipped", messageID, orderID)
		}
		return false, nil
	}

	// Dedup: skip if a bill with the same (email_message_id, order_id) already exists.
	var count int
	_ = h.billRepo.DB().QueryRow(
		`SELECT
		   (SELECT COUNT(*) FROM bills
		     WHERE source='shopee_shipped'
		       AND raw_data->>'email_message_id' = $1
		       AND UPPER(TRIM(LEADING '#' FROM COALESCE(raw_data->>'order_id', ''))) = $2) +
		   (SELECT COUNT(*) FROM processed_email_keys
		     WHERE source='shopee_shipped'
		       AND message_id = $1
		       AND UPPER(TRIM(LEADING '#' FROM COALESCE(order_id, ''))) = $2)`,
		messageID, orderID,
	).Scan(&count)
	if count > 0 {
		h.logger.Info("shopee_shipped: skipping duplicate",
			zap.String("message_id", messageID),
			zap.String("order_id", orderID),
		)
		return false, nil
	}
	if existingBillID, exists, err := h.findExistingShopeeShippedBillID(orderID); err != nil {
		return false, fmt.Errorf("lookup existing shopee_shipped order: %w", err)
	} else if exists {
		h.recordShopeeOrderEvent(existingBillID, subject, from, messageID, source, orderID)
		h.linkShopeeOrphanEventsToBill(existingBillID, orderID)
		h.saveShopeeShippedEmailArtifacts(existingBillID, subject, from, bodyText, bodyHTML, messageID)
		discountSummary := repository.ExtractShopeeDiscountSummary(bodyText, bodyHTML, orderID)
		if ok, err := h.billRepo.ApplyShopeePurchaseDiscountsToBill(existingBillID, discountSummary); err != nil {
			h.logger.Warn("shopee_shipped: existing bill discount update failed",
				zap.String("message_id", messageID),
				zap.String("order_id", orderID),
				zap.String("bill_id", existingBillID),
				zap.Error(err))
		} else if ok {
			h.logger.Info("shopee_shipped: updated existing bill discounts",
				zap.String("message_id", messageID),
				zap.String("order_id", orderID),
				zap.String("bill_id", existingBillID),
				zap.Float64("discount", discountSummary.TotalDiscountAmount))
		}
		paymentSummary := repository.ExtractShopeePaymentSummary(bodyText, bodyHTML, orderID)
		if ok, err := h.billRepo.ApplyShopeePurchasePaymentSummaryToBill(existingBillID, paymentSummary); err != nil {
			h.logger.Warn("shopee_shipped: existing bill payment summary update failed",
				zap.String("message_id", messageID),
				zap.String("order_id", orderID),
				zap.String("bill_id", existingBillID),
				zap.Error(err))
		} else if ok {
			h.logger.Info("shopee_shipped: updated existing bill payment summary",
				zap.String("message_id", messageID),
				zap.String("order_id", orderID),
				zap.String("bill_id", existingBillID),
				zap.String("payment_method", paymentSummary.PaymentMethod))
		}
		if messageID != "" {
			_ = h.billRepo.MarkProcessedEmailKey("shopee_shipped", messageID, orderID)
		}
		h.logger.Info("shopee_shipped: recorded status event on existing bill",
			zap.String("message_id", messageID),
			zap.String("order_id", orderID),
			zap.String("bill_id", existingBillID),
		)
		return false, nil
	}

	const topK = 5
	const highConfThreshold = 0.85

	type itemWithCandidates struct {
		item       models.BillItem
		candidates []models.CatalogMatch
	}

	var itemsWithCandidates []itemWithCandidates
	allHighConfidence := true

	for i, extItem := range validItems {
		var matches []models.CatalogMatch

		if h.embSvc != nil && h.embSvc.IsConfigured() && h.catalogIdx != nil && h.catalogIdx.Size() > 0 {
			queryEmb, err := h.embSvc.EmbedText(extItem.RawName)
			if err == nil {
				matches = h.catalogIdx.Search(queryEmb, topK)
			}
		}
		if len(matches) == 0 {
			matches, _ = h.catalogSvc.SearchByText(extItem.RawName, topK)
		}

		item := models.BillItem{
			RawName: extItem.RawName,
			Qty:     extItem.Qty,
			Mapped:  false,
		}
		if extItem.Price != nil {
			item.Price = extItem.Price
		} else if i < len(fallbackPrices) {
			p := fallbackPrices[i]
			item.Price = &p
		}
		if extItem.ImageURL != "" {
			item.SourceImageURL = extItem.ImageURL
		}

		if len(matches) > 0 && matches[0].Score >= highConfThreshold {
			item.ItemCode = &matches[0].ItemCode
			item.UnitCode = &matches[0].UnitCode
			item.Mapped = true
		} else {
			allHighConfidence = false
		}

		itemsWithCandidates = append(itemsWithCandidates, itemWithCandidates{
			item:       item,
			candidates: matches,
		})
	}

	shippingAmount, hasShippingAmount := repository.ExtractShopeeShippingAmount(bodyText, bodyHTML, orderID)
	shippingItem, shippingReady := h.configuredShopeeShippingLine(orderID, shippingAmount, hasShippingAmount)
	shippingLineAdded := shippingItem != nil
	if shippingLineAdded {
		itemsWithCandidates = append(itemsWithCandidates, itemWithCandidates{item: *shippingItem})
		if !shippingReady {
			allHighConfidence = false
		}
	}
	discountSummary := repository.ExtractShopeeDiscountSummary(bodyText, bodyHTML, orderID)

	// คำนวณ Shopee Coin = gross_goods - coupon_discount - (paid_total - shipping)
	// ใช้ bodyHTML เป็น primary เพราะ Shopee email มักไม่มี text/plain part
	paidTotal, hasPaidTotal := repository.ExtractShopeeMoneyLabel("", bodyHTML, orderID, "ยอดที่ต้องชำระทั้งหมด")
	if !hasPaidTotal {
		paidTotal, hasPaidTotal = repository.ExtractShopeeMoneyLabel(bodyText, "", orderID, "ยอดที่ต้องชำระทั้งหมด")
	}
	goodsTotalForCoin, hasGoodsTotal := repository.ExtractShopeeMoneyLabel("", bodyHTML, orderID, "ยอดรวมค่าสินค้า")
	if !hasGoodsTotal {
		goodsTotalForCoin, hasGoodsTotal = repository.ExtractShopeeMoneyLabel(bodyText, "", orderID, "ยอดรวมค่าสินค้า")
	}
	var coinAmount float64
	var hasCoin bool
	if hasGoodsTotal {
		coinAmount, hasCoin = repository.CalcShopeeCoinAmount(goodsTotalForCoin, shippingAmount, discountSummary.TotalDiscountAmount, paidTotal, hasPaidTotal)
	}

	effectiveDiscount := discountSummary.TotalDiscountAmount
	if hasCoin {
		effectiveDiscount = discountSummary.TotalDiscountAmount + coinAmount
	}

	if discountSummary.HasAny() || hasCoin {
		itemCopies := make([]models.BillItem, len(itemsWithCandidates))
		for i := range itemsWithCandidates {
			itemCopies[i] = itemsWithCandidates[i].item
		}
		repository.ApplyShopeeDiscountsToItems(itemCopies, effectiveDiscount)
		for i := range itemsWithCandidates {
			itemsWithCandidates[i].item.DiscountAmount = itemCopies[i].DiscountAmount
		}
	}

	// doc_date: prefer AI-extracted date, then regex from body, then empty string
	// (falls back to today at retry time via docDateFromBill).
	docDate := order.DocDate
	if docDate == "" {
		docDate = extractDocDate(bodyText)
	}

	rawDataMap := map[string]interface{}{
		"subject":          subject,
		"from":             from,
		"email_message_id": messageID,
		"order_id":         orderID,
		"seller_name":      order.SellerName,
		"items":            validItems,
		"flow":             "shopee_shipped",
		"doc_date":         docDate,
		"body_text":        bodyText,
		"body_html":        bodyHTML,
	}
	if orderURL := repository.ExtractShopeeMarketplaceOrderURL(bodyText, bodyHTML, orderID); orderURL != "" {
		rawDataMap["marketplace_order_url"] = orderURL
		rawDataMap["marketplace_order_url_source"] = "email_html"
	}
	if hasShippingAmount {
		rawDataMap["shipping_amount"] = shippingAmount
	}
	if discountSummary.HasAny() {
		rawDataMap["discount_summary"] = discountSummary
	}
	if hasCoin {
		rawDataMap["shopee_coin_amount"] = coinAmount
	}
	if paymentSummary := repository.ExtractShopeePaymentSummary(bodyText, bodyHTML, orderID); paymentSummary.HasAny() {
		rawDataMap["payment_summary"] = paymentSummary
	}
	applyMailSource(rawDataMap, source)
	rawDataBytes, _ := json.Marshal(rawDataMap)

	status := "needs_review"
	if allHighConfidence && len(itemsWithCandidates) > 0 {
		status = "pending"
	}

	conf := order.Confidence
	bill := &models.Bill{
		BillType:     "purchase",
		Source:       "shopee_shipped",
		Status:       status,
		AIConfidence: &conf,
		RawData:      json.RawMessage(rawDataBytes),
		SMLOrderID:   orderID,
	}
	if err := h.billRepo.Create(bill); err != nil {
		return false, fmt.Errorf("create shopee_shipped bill: %w", err)
	}
	h.recordShopeeOrderEvent(bill.ID, subject, from, messageID, source, orderID)
	h.linkShopeeOrphanEventsToBill(bill.ID, orderID)
	_ = h.billRepo.MarkProcessedEmailKey("shopee_shipped", messageID, orderID)

	// Save original email body as artifact on the first order only to avoid
	// storing N copies of the same email. Prefer HTML body (renders nicely in
	// the bill detail viewer) and fall back to plain text when HTML is absent.
	if count == 0 {
		h.saveShopeeShippedEmailArtifacts(bill.ID, subject, from, bodyText, bodyHTML, messageID)
	}

	for _, iwc := range itemsWithCandidates {
		item := iwc.item
		item.BillID = bill.ID
		candidatesJSON, _ := json.Marshal(iwc.candidates)
		_ = h.billRepo.InsertItemWithCandidates(&item, candidatesJSON)
	}

	if h.auditRepo != nil {
		billIDStr := bill.ID
		durMs := int(time.Since(startTime).Milliseconds())
		_ = h.auditRepo.Log(models.AuditEntry{
			Action:     "shopee_shipped_received",
			TargetID:   &billIDStr,
			Source:     "shopee_shipped",
			Level:      "info",
			TraceID:    traceID,
			DurationMs: &durMs,
			Detail: map[string]interface{}{
				"subject":             subject,
				"from":                from,
				"message_id":          messageID,
				"order_id":            orderID,
				"seller_name":         order.SellerName,
				"items_count":         len(itemsWithCandidates),
				"all_high_conf":       allHighConfidence,
				"shipping_line_added": shippingLineAdded,
				"status":              status,
			},
		})
	}

	h.adminNotify(fmt.Sprintf("📦 Shopee Shipped: บิลรอตรวจสอบ\nOrder: %s (%s)\nItems: %d\nBill ID: %s",
		orderID, order.SellerName, len(itemsWithCandidates), bill.ID))

	h.logger.Info("shopee_shipped: bill created",
		zap.String("bill_id", bill.ID),
		zap.String("status", status),
		zap.String("order_id", orderID),
		zap.String("seller_name", order.SellerName),
		zap.Int("items", len(itemsWithCandidates)),
	)

	return true, nil
}

func (h *EmailHandler) saveShopeeShippedEmailArtifacts(billID, subject, from, bodyText, bodyHTML, messageID string) {
	if bodyHTML != "" {
		h.saveEmailArtifacts(billID, "email_html", "shopee-shipped.html", "text/html; charset=utf-8",
			[]byte(bodyHTML), subject, from, messageID)
		return
	}
	h.saveEmailArtifacts(billID, "email_text", "shopee-shipped.txt", "text/plain; charset=utf-8",
		[]byte(bodyText), subject, from, messageID)
}

func (h *EmailHandler) configuredShopeeShippingLine(orderID string, amount float64, hasAmount bool) (*models.BillItem, bool) {
	return h.configuredMarketplaceFeeLine(
		"shopee_shipped",
		orderID,
		"ค่าจัดส่งสินค้า",
		models.ShopeeShippingSourceSKU,
		amount,
		hasAmount,
	)
}

func (h *EmailHandler) configuredLazadaFeeLine(orderID string, amount float64, hasAmount bool) (*models.BillItem, bool) {
	return h.configuredMarketplaceFeeLine(
		lazadaEmailSource,
		orderID,
		"ค่าจัดส่ง/ค่าธรรมเนียม Lazada",
		models.LazadaFeeSourceSKU,
		amount,
		hasAmount,
	)
}

func (h *EmailHandler) configuredMarketplaceFeeLine(source, orderID, fallbackRawName, sourceSKU string, amount float64, hasAmount bool) (*models.BillItem, bool) {
	if !hasAmount || amount < 0 || h.channelDefaults == nil {
		return nil, false
	}
	def, err := h.channelDefaults.Get(source, "purchase")
	if err != nil {
		h.logger.Warn("marketplace fee config lookup failed",
			zap.String("source", source), zap.String("order_id", orderID), zap.Error(err))
		return nil, false
	}
	if def == nil || !def.ShippingItemEnabled {
		return nil, false
	}
	code := strings.TrimSpace(def.ShippingItemCode)
	if code == "" {
		h.logger.Warn("marketplace fee config enabled without item code",
			zap.String("source", source), zap.String("order_id", orderID))
		return nil, false
	}

	rawName := fallbackRawName
	unit := strings.TrimSpace(def.ShippingItemUnitCode)
	if h.catalogRepo != nil {
		if cat, err := h.catalogRepo.GetOne(code); err != nil {
			h.logger.Warn("marketplace fee catalog lookup failed",
				zap.String("source", source), zap.String("order_id", orderID), zap.String("item_code", code), zap.Error(err))
		} else if cat != nil {
			if strings.TrimSpace(cat.ItemName) != "" {
				rawName = strings.TrimSpace(cat.ItemName)
			}
			if unit == "" {
				unit = strings.TrimSpace(cat.UnitCode)
			}
		}
	}

	itemCode := code
	price := amount
	item := &models.BillItem{
		RawName:   rawName,
		SourceSKU: sourceSKU,
		ItemCode:  &itemCode,
		Qty:       1,
		Price:     &price,
		Mapped:    true,
	}
	if unit != "" {
		item.UnitCode = &unit
	}
	return item, unit != ""
}

func (h *EmailHandler) findExistingShopeeShippedBillID(orderID string) (string, bool, error) {
	if h == nil || h.billRepo == nil {
		return "", false, nil
	}
	normalized := normalizeShopeeOrderID(orderID)
	if normalized == "" {
		return "", false, nil
	}
	var id string
	err := h.billRepo.DB().QueryRow(
		`SELECT b.id::text
		   FROM bills b
		   JOIN (
		     SELECT id FROM bills
		      WHERE archived_at IS NULL
		        AND UPPER(TRIM(LEADING '#' FROM COALESCE(sml_order_id, ''))) = $1
		     UNION
		     SELECT id FROM bills
		      WHERE archived_at IS NULL
		        AND UPPER(TRIM(LEADING '#' FROM COALESCE(raw_data->>'order_id', ''))) = $1
		     UNION
		     SELECT id FROM bills
		      WHERE archived_at IS NULL
		        AND UPPER(TRIM(LEADING '#' FROM COALESCE(raw_data->>'shopee_order_id', ''))) = $1
		     UNION
		     SELECT bill_id FROM shopee_order_events
		      WHERE UPPER(order_id) = $1
		   ) existing ON existing.id = b.id
		  WHERE b.source = 'shopee_shipped'
		    AND b.archived_at IS NULL
		  ORDER BY b.created_at ASC, b.id ASC
		  LIMIT 1`,
		strings.ToUpper(normalized),
	).Scan(&id)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", false, nil
		}
		return "", false, err
	}
	return id, true, nil
}
