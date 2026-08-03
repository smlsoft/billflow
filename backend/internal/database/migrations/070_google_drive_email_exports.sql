-- Durable, restart-safe jobs for copying the original marketplace email to
-- the customer's Google Drive through the server-owned rclone remote.
CREATE TABLE IF NOT EXISTS google_drive_email_exports (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    bill_id UUID NOT NULL UNIQUE REFERENCES bills(id) ON DELETE CASCADE,
    source_artifact_id UUID REFERENCES bill_artifacts(id) ON DELETE SET NULL,
    source_sha256 TEXT NOT NULL DEFAULT '',
    source_content_type TEXT NOT NULL DEFAULT '',
    source_filename TEXT NOT NULL DEFAULT '',
    source_channel TEXT NOT NULL DEFAULT '',
    order_date DATE NOT NULL,
    payment_token TEXT NOT NULL DEFAULT '',
    sml_doc_no TEXT NOT NULL DEFAULT '',
    marketplace_order_id TEXT NOT NULL DEFAULT '',
    charge_amount TEXT NOT NULL DEFAULT 'NA',
    remote_path TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'queued'
        CHECK (status IN ('queued', 'running', 'succeeded', 'failed', 'skipped', 'conflict')),
    priority INTEGER NOT NULL DEFAULT 100,
    attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_attempt_at TIMESTAMPTZ,
    started_at TIMESTAMPTZ,
    uploaded_at TIMESTAMPTZ,
    last_error TEXT NOT NULL DEFAULT '',
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_google_drive_email_exports_due
    ON google_drive_email_exports (priority ASC, next_attempt_at ASC)
    WHERE status = 'queued';

CREATE INDEX IF NOT EXISTS idx_google_drive_email_exports_status_created
    ON google_drive_email_exports (status, created_at DESC);
