package handlers

import (
	"encoding/json"
	"fmt"
	"time"

	"go.uber.org/zap"

	"billflow/internal/models"
)

// ProcessShopeeShippedEmailBody handles Shopee shipping-confirmation emails
// (subject contains "ถูกจัดส่งแล้ว"). Mirrors ProcessShopeeEmailBody but:
//   - Creates bill with bill_type='purchase' and source='shopee_shipped'
//   - Targets SML's purchaseorder endpoint when the user confirms in UI
//   - NEVER auto-sends — always pending or needs_review
func (h *EmailHandler) ProcessShopeeShippedEmailBody(subject, from, bodyText, messageID string) error {
	traceID := fmt.Sprintf("shopee-shipped-%d", time.Now().UnixMilli())
	startTime := time.Now()

	// Dedup by message_id
	if messageID != "" {
		exists, err := h.billRepo.FindByEmailMessageID(messageID)
		if err != nil {
			h.logger.Warn("shopee_shipped: dedup check failed", zap.String("message_id", messageID), zap.Error(err))
		} else if exists {
			h.logger.Info("shopee_shipped: skipping duplicate", zap.String("message_id", messageID))
			return nil
		}
	}

	if h.catalogSvc == nil {
		h.logger.Warn("shopee_shipped: catalog service not configured — skipping")
		return fmt.Errorf("catalog service not configured")
	}

	// AI-extract items from HTML body
	plainText := htmlToText(bodyText)
	extracted, err := h.aiClient.ExtractText(plainText)
	if err != nil || extracted == nil || len(extracted.Items) == 0 {
		h.logger.Warn("shopee_shipped: AI extract failed or empty",
			zap.String("subject", subject), zap.Error(err))
		return fmt.Errorf("AI extract shopee_shipped: %w", err)
	}

	// Order ID from subject (#XXX pattern)
	shopeeOrderID := extractShopeeOrderID(subject)

	// Dedup by order ID — note: a sale-side bill may also exist with the
	// same order_id (saleinvoice path); we only block when an existing
	// shopee_shipped bill is present, so check by source explicitly.
	if shopeeOrderID != "" {
		var count int
		err := h.billRepo.DB().QueryRow(
			`SELECT COUNT(*) FROM bills WHERE source='shopee_shipped' AND sml_order_id = $1`,
			shopeeOrderID,
		).Scan(&count)
		if err == nil && count > 0 {
			h.logger.Info("shopee_shipped: skipping duplicate order",
				zap.String("order_id", shopeeOrderID))
			return nil
		}
	}

	type itemWithCandidates struct {
		item       models.BillItem
		candidates []models.CatalogMatch
	}

	var itemsWithCandidates []itemWithCandidates
	allHighConfidence := true
	const topK = 5
	const highConfThreshold = 0.85

	// Per-item prices parsed from the email body — fallback for AI nulls.
	fallbackPrices := extractShopeePrices(plainText)

	for i, extItem := range extracted.Items {
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

	// doc_date: prefer the actual ship date from the email body.
	// Falls back to today at retry time if the regex didn't match.
	docDate := extractDocDate(plainText)

	rawDataMap := map[string]interface{}{
		"subject":          subject,
		"from":             from,
		"email_message_id": messageID,
		"shopee_order_id":  shopeeOrderID,
		"customer_name":    extracted.CustomerName,
		"customer_phone":   extracted.CustomerPhone,
		"note":             extracted.Note,
		"flow":             "shopee_shipped",
		"doc_date":         docDate,
	}
	rawDataBytes, _ := json.Marshal(rawDataMap)

	status := "needs_review"
	if allHighConfidence {
		status = "pending"
	}

	conf := extracted.Confidence
	bill := &models.Bill{
		BillType:     "purchase",
		Source:       "shopee_shipped",
		Status:       status,
		AIConfidence: &conf,
		RawData:      json.RawMessage(rawDataBytes),
		SMLOrderID:   shopeeOrderID,
	}
	if err := h.billRepo.Create(bill); err != nil {
		return fmt.Errorf("create shopee_shipped bill: %w", err)
	}

	// Save the original Shopee shipping email body + envelope as artifacts.
	h.saveEmailArtifacts(bill.ID, "email_html", "shopee-shipped.html", "text/html; charset=utf-8",
		[]byte(bodyText), subject, from, messageID)

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
				"order_id":      shopeeOrderID,
				"items_count":   len(itemsWithCandidates),
				"all_high_conf": allHighConfidence,
				"status":        status,
			},
		})
	}

	h.adminNotify(fmt.Sprintf("📦 Shopee Shipped: บิลรอตรวจสอบ\nSubject: %s\nOrder: %s\nItems: %d\nBill ID: %s",
		subject, shopeeOrderID, len(itemsWithCandidates), bill.ID))

	h.logger.Info("shopee_shipped: bill created",
		zap.String("bill_id", bill.ID),
		zap.String("status", status),
		zap.String("order_id", shopeeOrderID),
		zap.Int("items", len(itemsWithCandidates)),
	)

	return nil
}
