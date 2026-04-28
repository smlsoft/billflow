package emailservice

// Processors bundles the three downstream message handlers that the
// coordinator dispatches to based on each account's channel + the
// message's subject. One bundle is shared by all account pollers.
type Processors struct {
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
}
