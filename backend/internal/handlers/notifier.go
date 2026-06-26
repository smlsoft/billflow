package handlers

// Notifier delivers admin push notifications (Telegram, LINE, etc.)
type Notifier interface {
	PushAdmin(text string) error
}
