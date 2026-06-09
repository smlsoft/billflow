-- 059_marketplace_print_payment_method.sql
-- BillFlow-only payment method used to gate marketplace email printing.
-- This value is not sent to SML and does not modify existing SML payloads.

ALTER TABLE bills
  ADD COLUMN IF NOT EXISTS print_payment_method TEXT NOT NULL DEFAULT '';
