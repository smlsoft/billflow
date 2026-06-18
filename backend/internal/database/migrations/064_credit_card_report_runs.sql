CREATE TABLE IF NOT EXISTS credit_card_report_runs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    report_name TEXT NOT NULL DEFAULT '',
    date_from DATE NOT NULL,
    date_to DATE NOT NULL,
    payment_method TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT 'all',
    include_incomplete BOOLEAN NOT NULL DEFAULT false,
    selected_group_ids TEXT[] NOT NULL DEFAULT '{}',
    snapshot JSONB NOT NULL,
    summary JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_by UUID REFERENCES users(id),
    created_by_email TEXT NOT NULL DEFAULT '',
    exported_at TIMESTAMPTZ,
    exported_by UUID REFERENCES users(id),
    printed_at TIMESTAMPTZ,
    printed_by UUID REFERENCES users(id),
    print_summary JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_credit_card_report_runs_created_at
    ON credit_card_report_runs (created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_credit_card_report_runs_created_by
    ON credit_card_report_runs (created_by, created_at DESC)
    WHERE created_by IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_credit_card_report_runs_payment_period
    ON credit_card_report_runs (payment_method, date_from, date_to, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_credit_card_report_runs_selected_groups
    ON credit_card_report_runs USING GIN (selected_group_ids);

CREATE INDEX IF NOT EXISTS idx_bills_credit_card_report_lazada_group
    ON bills ((raw_data->>'lazada_charge_group_key'), (raw_data->>'lazada_confirmed_at'), created_at, id)
    WHERE source = 'lazada_email'
      AND bill_type = 'purchase'
      AND archived_at IS NULL
      AND COALESCE(raw_data->>'lazada_charge_group_key', '') <> '';

CREATE INDEX IF NOT EXISTS idx_bills_credit_card_report_shopee_message
    ON bills ((raw_data->>'email_message_id'), (raw_data->>'email_date'), created_at, id)
    WHERE source = 'shopee_shipped'
      AND bill_type = 'purchase'
      AND archived_at IS NULL
      AND COALESCE(raw_data->>'email_message_id', '') <> '';
