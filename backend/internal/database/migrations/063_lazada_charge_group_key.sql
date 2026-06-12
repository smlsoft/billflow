-- Partial index for lazada_charge_group_key — used by the new minute-level grouping logic.
-- Existing bills without this key retain their email_date-based grouping (migration 062).
-- CONCURRENTLY so it can run on a live instance without blocking writes.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_bills_lazada_email_purchase_group_key
  ON bills ((raw_data->>'lazada_charge_group_key'), created_at, id)
  WHERE source = 'lazada_email'
    AND bill_type = 'purchase'
    AND archived_at IS NULL
    AND raw_data->>'lazada_charge_group_key' IS NOT NULL;
