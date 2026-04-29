-- 009_channel_defaults_endpoint.sql — per-channel SML endpoint override
--
-- Lets admins switch which SML API a channel posts to without a code change.
-- Empty string = auto-resolve by (channel, bill_type) — matches the original
-- hardcoded routing in handlers/bills.go:Retry.
--
-- Allowed values:
--   'saleorder'     — SML 248  POST /v3/api/saleorder      (ใบสั่งขาย)
--   'saleinvoice'   — SML 248  POST /restapi/saleinvoice   (ใบกำกับภาษี — legacy)
--   'purchaseorder' — SML 248  POST /v3/api/purchaseorder  (ใบสั่งซื้อ/สั่งจอง)
--   'sale_reserve'  — SML 213  POST /api/sale_reserve      (ใบจอง — JSON-RPC)
--   ''              — auto by channel+bill_type

ALTER TABLE channel_defaults
  ADD COLUMN IF NOT EXISTS endpoint TEXT NOT NULL DEFAULT '';

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT FROM information_schema.constraint_column_usage
    WHERE constraint_name = 'channel_defaults_endpoint_check'
  ) THEN
    ALTER TABLE channel_defaults
      ADD CONSTRAINT channel_defaults_endpoint_check
      CHECK (endpoint IN ('', 'saleorder', 'saleinvoice', 'purchaseorder', 'sale_reserve'));
  END IF;
END $$;
