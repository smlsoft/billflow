-- 061_bills_search_payment_filter_indexes.sql
-- Keep POL lookup and marketplace payment-method filters fast as bill volume grows.

CREATE INDEX IF NOT EXISTS idx_bills_sml_doc_no_norm
  ON bills (UPPER(TRIM(LEADING '#' FROM COALESCE(sml_doc_no, ''))))
  WHERE archived_at IS NULL
    AND COALESCE(sml_doc_no, '') <> '';

CREATE INDEX IF NOT EXISTS idx_bills_marketplace_effective_print_payment_created
  ON bills (
    (COALESCE(
      NULLIF(BTRIM(print_payment_method), ''),
      CASE
        WHEN UPPER(BTRIM(COALESCE(sml_payload->>'supplier_name', ''))) LIKE 'TT%'
          THEN BTRIM(COALESCE(sml_payload->>'supplier_name', ''))
        ELSE ''
      END
    )),
    created_at DESC,
    id DESC
  )
  WHERE archived_at IS NULL
    AND source IN ('shopee_shipped', 'lazada_email')
    AND bill_type = 'purchase';
