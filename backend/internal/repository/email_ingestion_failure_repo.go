package repository

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"billflow/internal/models"
)

type EmailIngestionFailureRepo struct {
	db *sql.DB
}

func NewEmailIngestionFailureRepo(db *sql.DB) *EmailIngestionFailureRepo {
	return &EmailIngestionFailureRepo{db: db}
}

func (r *EmailIngestionFailureRepo) Upsert(f models.EmailIngestionFailure) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("email ingestion failure repository is not configured")
	}
	f.Source = strings.TrimSpace(f.Source)
	f.MessageID = strings.TrimSpace(f.MessageID)
	f.FailureCode = strings.TrimSpace(f.FailureCode)
	if f.Source == "" || f.MessageID == "" || f.FailureCode == "" {
		return fmt.Errorf("source, message id, and failure code are required")
	}
	detail, err := json.Marshal(f.FailureDetail)
	if err != nil {
		return fmt.Errorf("marshal email ingestion failure detail: %w", err)
	}
	var accountID interface{}
	if strings.TrimSpace(f.IMAPAccountID) != "" {
		accountID = f.IMAPAccountID
	}
	_, err = r.db.Exec(
		`INSERT INTO email_ingestion_failures (
		    source, message_id, imap_account_id, imap_mailbox, subject, from_addr,
		    email_date, body_text, body_html, failure_code, failure_detail
		 ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		 ON CONFLICT (source, message_id, failure_code) DO UPDATE SET
		    imap_account_id=EXCLUDED.imap_account_id,
		    imap_mailbox=EXCLUDED.imap_mailbox,
		    subject=EXCLUDED.subject,
		    from_addr=EXCLUDED.from_addr,
		    email_date=EXCLUDED.email_date,
		    body_text=EXCLUDED.body_text,
		    body_html=EXCLUDED.body_html,
		    failure_detail=EXCLUDED.failure_detail,
		    status='open',
		    attempt_count=email_ingestion_failures.attempt_count + 1,
		    last_seen_at=NOW(),
		    resolved_at=NULL`,
		f.Source, f.MessageID, accountID, f.IMAPMailbox, f.Subject, f.FromAddr,
		f.EmailDate, f.BodyText, f.BodyHTML, f.FailureCode, detail,
	)
	return err
}

func (r *EmailIngestionFailureRepo) Resolve(source, messageID, failureCode string) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("email ingestion failure repository is not configured")
	}
	_, err := r.db.Exec(
		`UPDATE email_ingestion_failures
		    SET status='resolved', resolved_at=NOW(), last_seen_at=NOW()
		  WHERE source=$1 AND message_id=$2 AND failure_code=$3 AND status='open'`,
		strings.TrimSpace(source), strings.TrimSpace(messageID), strings.TrimSpace(failureCode),
	)
	return err
}
