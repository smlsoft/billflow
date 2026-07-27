-- Preserve the complete set of marketplace order IDs found in one email.
-- bills alone cannot represent an order that failed before a bill was created.
CREATE TABLE IF NOT EXISTS marketplace_email_groups (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source TEXT NOT NULL,
    message_id TEXT NOT NULL,
    imap_account_id UUID REFERENCES imap_accounts(id) ON DELETE SET NULL,
    imap_mailbox TEXT NOT NULL DEFAULT '',
    subject TEXT NOT NULL DEFAULT '',
    from_addr TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'processing'
        CHECK (status IN ('processing', 'complete', 'attention', 'legacy_unknown')),
    expected_order_count INTEGER NOT NULL DEFAULT 0 CHECK (expected_order_count >= 0),
    resolved_order_count INTEGER NOT NULL DEFAULT 0 CHECK (resolved_order_count >= 0),
    missing_order_count INTEGER NOT NULL DEFAULT 0 CHECK (missing_order_count >= 0),
    failure_code TEXT NOT NULL DEFAULT '',
    failure_detail JSONB NOT NULL DEFAULT '{}'::jsonb,
    attempt_count INTEGER NOT NULL DEFAULT 1 CHECK (attempt_count >= 1),
    first_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ,
    UNIQUE (source, message_id)
);

CREATE TABLE IF NOT EXISTS marketplace_email_group_orders (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id UUID NOT NULL REFERENCES marketplace_email_groups(id) ON DELETE CASCADE,
    order_id TEXT NOT NULL,
    bill_id UUID REFERENCES bills(id) ON DELETE SET NULL,
    status TEXT NOT NULL DEFAULT 'expected'
        CHECK (status IN ('expected', 'created', 'existing', 'missing', 'failed', 'archived')),
    error_code TEXT NOT NULL DEFAULT '',
    error_detail JSONB NOT NULL DEFAULT '{}'::jsonb,
    resolved_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (group_id, order_id)
);

CREATE INDEX IF NOT EXISTS idx_marketplace_email_groups_attention
    ON marketplace_email_groups (source, status, last_seen_at DESC)
    WHERE status IN ('processing', 'attention');

CREATE INDEX IF NOT EXISTS idx_marketplace_email_group_orders_status
    ON marketplace_email_group_orders (group_id, status);
