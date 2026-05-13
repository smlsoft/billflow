package emailservice

import "fmt"

type MessageSkipError struct {
	Code  string
	Label string
}

func (e *MessageSkipError) Error() string {
	if e == nil {
		return ""
	}
	if e.Label != "" {
		return e.Label
	}
	if e.Code != "" {
		return e.Code
	}
	return "message skipped"
}

func SkipMessage(code, label string) error {
	if code == "" {
		code = "skipped"
	}
	if label == "" {
		label = fmt.Sprintf("ข้ามเมลนี้ (%s)", code)
	}
	return &MessageSkipError{Code: code, Label: label}
}

// MessagePrecheck can skip a message after its envelope is fetched but before
// downloading the body. This keeps large inbox backfills fast when most old
// messages are already represented by durable duplicate tombstones.
type MessagePrecheck func(channel, subject, messageID string) error

// Processors bundles the three downstream message handlers that the
// coordinator dispatches to based on each account's channel + the
// message's subject. One bundle is shared by all account pollers.
type Processors struct {
	Precheck MessagePrecheck

	// Attachment is the generic PDF/image/Excel pipeline used by
	// channel="general" and channel="lazada" (until the dedicated
	// Lazada handler ships).
	Attachment AttachmentProcessor

	// ShopeeOrder handles Shopee email order confirmations (saleinvoice
	// flow). Used for channel="shopee" when the subject does NOT contain
	// "ถูกจัดส่งแล้ว".
	ShopeeOrder ShopeeBodyProcessor

	// ShopeeShipped handles Shopee email shipping confirmations
	// (purchaseorder flow). Used for channel="shopee" when the subject
	// contains "ถูกจัดส่งแล้ว".
	ShopeeShipped ShopeeBodyProcessor

	// ShopeeStatus handles Shopee marketplace status notifications. It records
	// an informational timeline event only; it must not create SML documents.
	ShopeeStatus ShopeeBodyProcessor
}
