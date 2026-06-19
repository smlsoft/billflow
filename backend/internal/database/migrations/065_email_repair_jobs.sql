CREATE TABLE IF NOT EXISTS email_repair_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    bill_id UUID REFERENCES bills(id) ON DELETE SET NULL,
    source TEXT NOT NULL DEFAULT 'shopee_shipped',
    message_id TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'queued'
        CHECK (status IN ('queued', 'running', 'succeeded', 'failed')),
    snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
    result JSONB NOT NULL DEFAULT '{}'::jsonb,
    error TEXT NOT NULL DEFAULT '',
    created_by UUID REFERENCES users(id),
    created_by_email TEXT NOT NULL DEFAULT '',
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_email_repair_jobs_bill_created
    ON email_repair_jobs (bill_id, created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_email_repair_jobs_message_created
    ON email_repair_jobs (source, message_id, created_at DESC, id DESC);

CREATE UNIQUE INDEX IF NOT EXISTS idx_email_repair_jobs_active_message
    ON email_repair_jobs (source, message_id)
    WHERE status IN ('queued', 'running');
