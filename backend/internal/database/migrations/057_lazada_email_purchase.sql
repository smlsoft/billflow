-- 057_lazada_email_purchase.sql — Lazada email purchase intake
--
-- Lazada Excel keeps source/channel='lazada' for sale imports. IMAP-created
-- Lazada purchase bills use source/channel='lazada_email' so routing, queues,
-- and duplicate guards can stay separate.

ALTER TABLE bills DROP CONSTRAINT IF EXISTS bills_source_check;
ALTER TABLE bills ADD CONSTRAINT bills_source_check
  CHECK (source IN (
    'line',
    'email',
    'lazada',
    'lazada_email',
    'tiktok',
    'shopee',
    'shopee_email',
    'shopee_shipped',
    'manual'
  ));

ALTER TABLE channel_defaults DROP CONSTRAINT IF EXISTS channel_defaults_channel_check;
ALTER TABLE channel_defaults ADD CONSTRAINT channel_defaults_channel_check
  CHECK (channel IN (
    'line',
    'email',
    'shopee',
    'shopee_email',
    'shopee_shipped',
    'lazada',
    'lazada_email',
    'tiktok',
    'manual',
    'shopee_settlement'
  ));

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
      FROM pg_indexes
     WHERE schemaname = current_schema()
       AND indexname = 'bills_lazada_email_order_id_unique'
  ) AND NOT EXISTS (
    SELECT 1
      FROM bills
     WHERE source = 'lazada_email'
       AND raw_data ? 'order_id'
       AND COALESCE(raw_data->>'order_id', '') <> ''
     GROUP BY raw_data->>'order_id'
    HAVING COUNT(*) > 1
  ) THEN
    CREATE UNIQUE INDEX bills_lazada_email_order_id_unique
      ON bills ((raw_data->>'order_id'))
      WHERE source = 'lazada_email'
        AND raw_data ? 'order_id'
        AND COALESCE(raw_data->>'order_id', '') <> '';
  END IF;
END $$;
