-- Keep source email evidence when ingestion is deliberately quarantined.
-- This is distinct from processed_email_keys: a quarantined email has not
-- created a bill and must remain recoverable by an administrator.
CREATE TABLE IF NOT EXISTS email_ingestion_failures (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source TEXT NOT NULL,
    message_id TEXT NOT NULL,
    imap_account_id UUID REFERENCES imap_accounts(id) ON DELETE SET NULL,
    imap_mailbox TEXT NOT NULL DEFAULT '',
    subject TEXT NOT NULL DEFAULT '',
    from_addr TEXT NOT NULL DEFAULT '',
    email_date TEXT NOT NULL DEFAULT '',
    body_text TEXT NOT NULL DEFAULT '',
    body_html TEXT NOT NULL DEFAULT '',
    failure_code TEXT NOT NULL,
    failure_detail JSONB NOT NULL DEFAULT '{}'::jsonb,
    status TEXT NOT NULL DEFAULT 'open'
        CHECK (status IN ('open', 'resolved', 'ignored')),
    attempt_count INTEGER NOT NULL DEFAULT 1,
    first_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_at TIMESTAMPTZ,
    UNIQUE (source, message_id, failure_code)
);

CREATE INDEX IF NOT EXISTS idx_email_ingestion_failures_open
    ON email_ingestion_failures (source, status, last_seen_at DESC)
    WHERE status = 'open';
