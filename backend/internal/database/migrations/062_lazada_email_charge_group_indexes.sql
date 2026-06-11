-- 062_lazada_email_charge_group_indexes.sql
-- Fast lookup for Lazada card-charge grouping by exact IMAP email timestamp.

CREATE INDEX IF NOT EXISTS idx_bills_lazada_email_purchase_email_date
  ON bills ((raw_data->>'email_date'), created_at, id)
  WHERE source = 'lazada_email'
    AND bill_type = 'purchase'
    AND archived_at IS NULL
    AND raw_data ? 'email_date';
