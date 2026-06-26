package jobs

// Notifier is implemented by any alert delivery backend (Telegram, LINE, etc.)
type Notifier interface {
	PushAdmin(text string) error
}
