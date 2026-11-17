-- Additive warning-only ledger. Previous binaries can ignore it on rollback.
CREATE TABLE IF NOT EXISTS bill_shipping_checks (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    bill_id uuid NOT NULL REFERENCES bills(id) ON DELETE CASCADE,
    message_key text NOT NULL,
    message_id text NOT NULL DEFAULT '',
    artifact_id uuid REFERENCES bill_artifacts(id) ON DELETE SET NULL,
    policy jsonb NOT NULL,
    result jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (bill_id, message_key)
);
CREATE INDEX IF NOT EXISTS bill_shipping_checks_history_idx ON bill_shipping_checks (bill_id, created_at DESC);
CREATE INDEX IF NOT EXISTS bills_shipping_attention_idx ON bills (created_at DESC, id DESC)
    WHERE raw_data->'shipping_check_summary'->>'attention' = 'true';
