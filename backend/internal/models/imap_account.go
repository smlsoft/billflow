package models

import "time"

// IMAPAccount is one mailbox the email coordinator polls.
//
// Channel routes a poll's processed messages to the right body handler:
//   - "general" → attachment pipeline (PDF/Excel attached files)
//   - "shopee"  → Shopee email order/shipped flow (subject decides which)
//   - "lazada"  → reserved (Phase 4b WIP, currently routes like general)
type IMAPAccount struct {
	ID                  string     `json:"id"`
	Name                string     `json:"name"`
	Host                string     `json:"host"`
	Port                int        `json:"port"`
	Username            string     `json:"username"`
	Password            string     `json:"password,omitempty"`
	Mailbox             string     `json:"mailbox"`
	FilterFrom          string     `json:"filter_from"`
	FilterSubjects      string     `json:"filter_subjects"`
	Channel             string     `json:"channel"`
	ShopeeDomains       string     `json:"shopee_domains"`
	LookbackDays        int        `json:"lookback_days"`
	PollIntervalSeconds int        `json:"poll_interval_seconds"`
	Enabled             bool       `json:"enabled"`
	LastPolledAt        *time.Time `json:"last_polled_at"`
	LastPollStatus      *string    `json:"last_poll_status"`
	LastPollError       *string    `json:"last_poll_error"`
	LastPollMessages    *int       `json:"last_poll_messages"`
	ConsecutiveFailures int        `json:"consecutive_failures"`
	LastAdminAlertAt    *time.Time `json:"last_admin_alert_at"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

// IMAPAccountUpsert is the user-editable subset (no runtime status fields).
type IMAPAccountUpsert struct {
	Name                string `json:"name" binding:"required"`
	Host                string `json:"host" binding:"required"`
	Port                int    `json:"port" binding:"required,min=1,max=65535"`
	Username            string `json:"username" binding:"required"`
	Password            string `json:"password"`
	Mailbox             string `json:"mailbox"`
	FilterFrom          string `json:"filter_from"`
	FilterSubjects      string `json:"filter_subjects"`
	Channel             string `json:"channel" binding:"required,oneof=general shopee lazada"`
	ShopeeDomains       string `json:"shopee_domains"`
	LookbackDays        int    `json:"lookback_days" binding:"required,min=1,max=90"`
	PollIntervalSeconds int    `json:"poll_interval_seconds" binding:"required,min=300"`
	Enabled             bool   `json:"enabled"`
}
