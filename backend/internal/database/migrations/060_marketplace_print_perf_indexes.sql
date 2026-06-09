-- 060_marketplace_print_perf_indexes.sql
-- Index-only, idempotent support for marketplace email print/readiness queries.

CREATE INDEX IF NOT EXISTS idx_bills_marketplace_email_group_key
  ON bills ((
    COALESCE(
      NULLIF(raw_data->>'email_message_id', ''),
      NULLIF(raw_data->>'message_id', ''),
      'bill:' || id::text
    )
  ))
  WHERE source IN ('shopee_shipped', 'lazada_email')
    AND bill_type = 'purchase';

CREATE INDEX IF NOT EXISTS idx_bills_marketplace_raw_email_message_id
  ON bills ((raw_data->>'email_message_id'))
  WHERE source IN ('shopee_shipped', 'lazada_email')
    AND bill_type = 'purchase'
    AND raw_data ? 'email_message_id';

CREATE INDEX IF NOT EXISTS idx_bills_marketplace_raw_message_id
  ON bills ((raw_data->>'message_id'))
  WHERE source IN ('shopee_shipped', 'lazada_email')
    AND bill_type = 'purchase'
    AND raw_data ? 'message_id'
    AND COALESCE(raw_data->>'email_message_id', '') = '';

CREATE INDEX IF NOT EXISTS idx_bill_artifacts_printable_bill_id
  ON bill_artifacts (bill_id)
  WHERE kind IN ('email_html', 'email_text');
