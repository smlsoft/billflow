package handlers

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"billflow/internal/models"
	"billflow/internal/services/ai"
	emailservice "billflow/internal/services/email"
)

// ProcessShopeeShippedEmailBody handles Shopee shipping-confirmation emails
// (subject contains "ถูกจัดส่งแล้ว" or "ยืนยันการชำระเงิน").
// One email may contain multiple Shopee orders (one per seller) — this
// function creates a separate purchase bill for each order_id found by AI.
// Bills are never auto-sent — status is always pending or needs_review.
func (h *EmailHandler) ProcessShopeeShippedEmailBody(subject, from, bodyText, bodyHTML, messageID string, source emailservice.MailSource) error {
	traceID := fmt.Sprintf("shopee-shipped-%d", time.Now().UnixMilli())
	startTime := time.Now()

	if h.catalogSvc == nil {
		h.logger.Warn("shopee_shipped: catalog service not configured — skipping")
		return fmt.Errorf("catalog service not configured")
	}

	if messageID != "" {
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
			return emailservice.SkipMessage("duplicate", "เมลนี้เคยประมวลผลแล้ว")
		}
	}

	// bodyText is already plain text (extractBodyText prefers text/plain).
	// htmlToText is a no-op when input has no HTML tags, so it's safe to call.
	plainText := htmlToText(bodyText)

	// AI extracts all orders from this email.
	orders, err := h.aiClient.ExtractOrders(plainText)
	if err != nil || len(orders) == 0 {
		h.logger.Warn("shopee_shipped: AI extract failed or empty",
			zap.String("subject", subject), zap.Error(err))
		if err == nil {
			return fmt.Errorf("AI extract shopee_shipped: empty orders")
		}
		return fmt.Errorf("AI extract shopee_shipped: %w", err)
	}

	h.logger.Info("shopee_shipped: orders extracted",
		zap.String("trace_id", traceID),
		zap.Int("order_count", len(orders)),
	)

	// Per-item prices parsed from the email body — fallback for AI nulls.
	fallbackPrices := extractShopeePrices(plainText)
	sourceImages := extractShopeeImageURLs(bodyHTML)
	if len(sourceImages) == 0 {
		sourceImages = extractShopeeImageURLs(bodyText)
	}

	createdCount := 0
	skippedCount := 0
	failedCount := 0
	for _, order := range orders {
		created, err := h.processOneShippedOrder(
			order, subject, from, bodyText, bodyHTML, messageID, fallbackPrices, sourceImages, traceID, startTime, source,
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
	if createdCount == 0 && skippedCount > 0 && failedCount == 0 {
		return emailservice.SkipMessage("duplicate_or_empty", "ไม่มีบิลใหม่จากเมลนี้ อาจซ้ำหรือไม่มีรายการสินค้าที่ใช้ได้")
	}
	return nil
}

// processOneShippedOrder creates a single purchase bill for one Shopee order.
// Returns (true, nil) when the bill was created, (false, nil) when skipped (dedup).
func (h *EmailHandler) processOneShippedOrder(
	order ai.ExtractedOrder,
	subject, from, bodyText, bodyHTML, messageID string,
	fallbackPrices []float64,
	sourceImages []string,
	traceID string,
	startTime time.Time,
	source emailservice.MailSource,
) (bool, error) {
	orderID := strings.TrimSpace(order.OrderID)
	if orderID == "" || strings.EqualFold(orderID, "#unknown") {
		h.logger.Warn("shopee_shipped: skipping order without order_id",
			zap.String("message_id", messageID),
			zap.String("subject", subject),
		)
		return false, nil
	}
	if err := h.recordShopeeOrderEvent(subject, from, bodyText, messageID, source, orderID); err != nil {
		return false, err
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
		       AND raw_data->>'order_id' = $2) +
		   (SELECT COUNT(*) FROM processed_email_keys
		     WHERE source='shopee_shipped'
		       AND message_id = $1
		       AND order_id = $2)`,
		messageID, orderID,
	).Scan(&count)
	if count > 0 {
		h.logger.Info("shopee_shipped: skipping duplicate",
			zap.String("message_id", messageID),
			zap.String("order_id", orderID),
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
		if i < len(sourceImages) {
			item.SourceImageURL = sourceImages[i]
		}
		if extItem.Price != nil {
			item.Price = extItem.Price
		} else if i < len(fallbackPrices) {
			p := fallbackPrices[i]
			item.Price = &p
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
	_ = h.billRepo.MarkProcessedEmailKey("shopee_shipped", messageID, orderID)

	// Save original email body as artifact on the first order only to avoid
	// storing N copies of the same email. Prefer HTML body (renders nicely in
	// the bill detail viewer) and fall back to plain text when HTML is absent.
	if count == 0 {
		if bodyHTML != "" {
			h.saveEmailArtifacts(bill.ID, "email_html", "shopee-shipped.html", "text/html; charset=utf-8",
				[]byte(bodyHTML), subject, from, messageID)
		} else {
			h.saveEmailArtifacts(bill.ID, "email_text", "shopee-shipped.txt", "text/plain; charset=utf-8",
				[]byte(bodyText), subject, from, messageID)
		}
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
				"subject":       subject,
				"from":          from,
				"message_id":    messageID,
				"order_id":      orderID,
				"seller_name":   order.SellerName,
				"items_count":   len(itemsWithCandidates),
				"all_high_conf": allHighConfidence,
				"status":        status,
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
