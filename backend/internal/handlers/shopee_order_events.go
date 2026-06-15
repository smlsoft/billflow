package handlers

import (
	"encoding/json"
	"strings"
	"time"

	"go.uber.org/zap"

	"billflow/internal/models"
	emailservice "billflow/internal/services/email"
)

const (
	shopeeEventPaymentConfirmed = "payment_confirmed"
	shopeeEventShipped          = "shipped"
)

func shopeeOrderEventFromSubject(subject string) (eventType, label, orderID string, ok bool) {
	switch {
	case strings.Contains(subject, "ยืนยันการชำระเงิน"):
		eventType = shopeeEventPaymentConfirmed
		label = "ยืนยันการชำระเงินแล้ว"
	case strings.Contains(subject, "ถูกจัดส่งแล้ว"):
		eventType = shopeeEventShipped
		label = "ถูกจัดส่งแล้ว"
	default:
		return "", "", "", false
	}
	orderID = normalizeShopeeOrderID(extractShopeeOrderID(subject))
	return eventType, label, orderID, orderID != ""
}

func normalizeShopeeOrderID(orderID string) string {
	orderID = strings.TrimSpace(orderID)
	orderID = strings.TrimPrefix(orderID, "#")
	return strings.ToUpper(strings.TrimSpace(orderID))
}

func (h *EmailHandler) recordShopeeOrderEvent(
	billID string,
	subject string,
	from string,
	messageID string,
	source emailservice.MailSource,
	orderID string,
) {
	if h == nil || h.billRepo == nil {
		return
	}
	eventType, label, subjectOrderID, ok := shopeeOrderEventFromSubject(subject)
	if !ok {
		return
	}
	orderID = normalizeShopeeOrderID(orderID)
	if orderID == "" {
		orderID = subjectOrderID
	}
	if orderID == "" {
		return
	}
	if messageID == "" {
		messageID = "bill:" + billID
	}
	var emailDate *time.Time
	if source.EmailDate != "" {
		if t, err := time.Parse(time.RFC3339, source.EmailDate); err == nil {
			emailDate = &t
		}
	}
	raw, _ := json.Marshal(map[string]interface{}{
		"imap_account_id": source.AccountID,
		"imap_username":   source.Username,
		"source":          "email_pipeline",
	})
	billIDPtr := billID
	if err := h.billRepo.InsertShopeeOrderEvent(&models.ShopeeOrderEvent{
		BillID:      &billIDPtr,
		OrderID:     orderID,
		EventType:   eventType,
		StatusLabel: label,
		Subject:     subject,
		FromAddr:    from,
		MessageID:   messageID,
		EmailDate:   emailDate,
		RawData:     raw,
	}); err != nil {
		h.logger.Warn("shopee_order_event: record failed",
			zap.String("bill_id", billID),
			zap.String("order_id", orderID),
			zap.String("event_type", eventType),
			zap.Error(err),
		)
	}
}

func (h *EmailHandler) linkShopeeOrphanEventsToBill(billID, orderID string) {
	if h == nil || h.billRepo == nil || billID == "" || orderID == "" {
		return
	}
	affected, err := h.billRepo.LinkShopeeOrderEventsToBill(orderID, billID)
	if err != nil {
		h.logger.Warn("shopee_order_event: link orphan events failed",
			zap.String("bill_id", billID),
			zap.String("order_id", orderID),
			zap.Error(err),
		)
		return
	}
	if affected > 0 {
		h.logger.Info("shopee_order_event: linked orphan events",
			zap.String("bill_id", billID),
			zap.String("order_id", orderID),
			zap.Int64("events", affected),
		)
	}
}
