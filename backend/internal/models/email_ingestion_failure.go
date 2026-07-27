package models

import "time"

// EmailIngestionFailure retains an email that was intentionally quarantined
// before it could create a BillFlow bill. It is operational evidence, not a bill.
type EmailIngestionFailure struct {
	ID            string                 `json:"id"`
	Source        string                 `json:"source"`
	MessageID     string                 `json:"message_id"`
	IMAPAccountID string                 `json:"imap_account_id,omitempty"`
	IMAPMailbox   string                 `json:"imap_mailbox,omitempty"`
	Subject       string                 `json:"subject"`
	FromAddr      string                 `json:"from_addr"`
	EmailDate     string                 `json:"email_date,omitempty"`
	BodyText      string                 `json:"-"`
	BodyHTML      string                 `json:"-"`
	FailureCode   string                 `json:"failure_code"`
	FailureDetail map[string]interface{} `json:"failure_detail,omitempty"`
	Status        string                 `json:"status"`
	AttemptCount  int                    `json:"attempt_count"`
	FirstSeenAt   time.Time              `json:"first_seen_at"`
	LastSeenAt    time.Time              `json:"last_seen_at"`
	ResolvedAt    *time.Time             `json:"resolved_at,omitempty"`
}
