package emailservice

import "fmt"

type ProcessOutcomeKind string

const (
	ProcessOutcomeCreatedBill     ProcessOutcomeKind = "created_bill"
	ProcessOutcomeUpdatedExisting ProcessOutcomeKind = "updated_existing"
	ProcessOutcomeSkipped         ProcessOutcomeKind = "skipped"
	// ProcessOutcomeQuarantined means the source email was retained for an
	// admin replay, but must not create a bill until its discrepancy is fixed.
	ProcessOutcomeQuarantined ProcessOutcomeKind = "quarantined"
)

type ProcessOutcome struct {
	Kind  ProcessOutcomeKind
	Code  string
	Label string
}

func CreatedBillOutcome() ProcessOutcome {
	return ProcessOutcome{Kind: ProcessOutcomeCreatedBill, Code: "created_bill", Label: "สร้างบิลใหม่แล้ว"}
}

func UpdatedExistingOutcome(code, label string) ProcessOutcome {
	if code == "" {
		code = "updated_existing"
	}
	if label == "" {
		label = "อัปเดตข้อมูลบนบิลเดิมแล้ว"
	}
	return ProcessOutcome{Kind: ProcessOutcomeUpdatedExisting, Code: code, Label: label}
}

func SkippedOutcome(code, label string) ProcessOutcome {
	if code == "" {
		code = "skipped"
	}
	if label == "" {
		label = fmt.Sprintf("ไม่สร้างบิลใหม่ (%s)", code)
	}
	return ProcessOutcome{Kind: ProcessOutcomeSkipped, Code: code, Label: label}
}

func QuarantinedOutcome(code, label string) ProcessOutcome {
	if code == "" {
		code = "quarantined"
	}
	if label == "" {
		label = "เก็บอีเมลไว้ให้ผู้ดูแลตรวจสอบและสั่งอ่านซ้ำ"
	}
	return ProcessOutcome{Kind: ProcessOutcomeQuarantined, Code: code, Label: label}
}

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
		label = fmt.Sprintf("ไม่สร้างบิลใหม่ (%s)", code)
	}
	return &MessageSkipError{Code: code, Label: label}
}

// Processors bundles the downstream message handlers that the
// coordinator dispatches to based on each account's channel + the
// message's subject. One bundle is shared by all account pollers.
type Processors struct {
	// Attachment is the generic PDF/image/Excel pipeline used by
	// channel="general".
	Attachment AttachmentProcessor

	// ShopeeOrder handles Shopee email order confirmations (saleinvoice
	// flow). Used for channel="shopee" when the subject does NOT contain
	// "ถูกจัดส่งแล้ว".
	ShopeeOrder ShopeeBodyProcessor

	// ShopeeShipped handles Shopee purchase-related status emails. Payment
	// confirmations create purchase bills; shipping confirmations only record
	// status events.
	ShopeeShipped ShopeeBodyProcessor

	// LazadaPurchase handles Lazada order confirmation / shipped emails
	// (purchaseorder flow). Used for channel="lazada" after strict sender
	// and subject checks pass.
	LazadaPurchase ShopeeBodyProcessor

	// DuplicateMessage returns true when a Message-ID has already been
	// processed. It lets the poller avoid fetching body/AI work for old
	// read+unread messages while still reporting a user-friendly summary.
	DuplicateMessage func(messageID string) (bool, error)

	// DuplicateMessages is the batch form used by IMAP envelope polling so a
	// mailbox with hundreds of old read+unread messages does one DB lookup per
	// batch instead of one lookup per message.
	DuplicateMessages func(messageIDs []string) (map[string]bool, error)
}
