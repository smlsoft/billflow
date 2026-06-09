# Lazada Email Purchase Intake

> Updated: 2026-06-09 +07
> Current prod scope: `billflow-thaisunsport`; shared marketplace print/payment UI is also deployed to `billflow` main.

## Status

- Deployed to thaisunsport: frontend `3020`, backend `8100`, PostgreSQL `5448`.
- Migration deployed for intake: `057_lazada_email_purchase.sql`.
- Latest deployed migrations include print/payment support through `060_marketplace_print_perf_indexes.sql`.
- Lazada IMAP accounts are enabled on thaisunsport with `lookback_days=1` and `poll_interval_seconds=600`.
- Initial 7 Lazada email bills were backfilled from stored email HTML artifacts; customer later confirmed the numbers were correct and one Lazada PO was sent to SML successfully.
- Current flow remains review-first. Lazada email bills are not auto-sent to SML.
- Historical backup before the amount backfill: `/home/bosscatdog/billflow-thaisunsport/manual-backups/lazada-amount-20260605_112633`.

## Business Rule

Lazada email purchase orders must reconcile to the paid total before sending to SML:

```text
goods_total_amount + shipping_amount + service_fee_amount - coupon_discount_amount = paid_total_amount
```

Tolerance is `±0.01`.

The PO target is the real Lazada paid amount. AI output is not trusted for money totals.

## Data Shape

`bills.raw_data` for `source='lazada_email'` stores:

- `goods_total_amount`
- `shipping_amount`
- `coupon_discount_amount`
- `service_fee_amount`
- `paid_total_amount`
- `shipping_method`
- `payment_method`
- `lazada_fee_amount`
- `amount_reconciliation_status`
- `amount_reconciliation_delta`
- `discount_summary.platform = "lazada"`
- `discount_summary.total_discount_amount`
- `discount_summary.allocation_method = "proportional_by_gross_excluding_shipping"`

`bill_items.discount_amount` stores the allocated Lazada coupon amount per item.

## Fee Line

Shipping + service fee use one SML item line in v1:

- source SKU: `__lazada_shipping_fee__`
- config row: `/settings/channels` → `lazada_email/purchase`
- config columns reused:
  - `shipping_item_enabled`
  - `shipping_item_code`
  - `shipping_item_unit_code`

Current thaisunsport config:

```text
shipping_item_enabled = true
shipping_item_code = SHIP_CUS
shipping_item_unit_code = บาท
```

Bill Detail auto-adds this fee line when the bill is opened and config is ready. The system does not add the line in bulk automatically outside that workflow.

## Send Guard

Backend blocks SML send when:

- `amount_reconciliation_status != "ok"`
- Lazada paid total includes shipping/fee but fee item config is missing
- normal bill validation fails: unmapped item, unconfirmed match, missing unit, qty <= 0, invalid price

This applies to normal retry and bulk send.

## SML Header Mapping

- `remark` uses the seller/supplier name from the Lazada email, not user-entered text.
- `remark_5` stores the Lazada order id, matching the Shopee purchase behavior.
- `doc_ref` is normally empty for Lazada email.
- If the Lazada email payment method is Credit/Debit Card, `doc_ref` stores the paid total amount from `ยอดรวมทั้งหมด(รวม VAT)`.
- Purchase send dialogs do not show a free-form `remark` field, to avoid overwriting the SML remark that must carry the shop/seller name.

## Print And Payment Method

Lazada purchase email print readiness now uses the shared marketplace print/payment rule:

- every active order in the same email group must have an SML POL number
- effective payment method must start with `TT` by default
- payment method is stored only in BillFlow as `bills.print_payment_method`
- single and bulk SML send dialogs require choosing `วิธีการชำระเงิน` before send
- the selected payment method is not sent to SML

Details: [Marketplace Purchase Print And Payment Method](marketplace-purchase-print-and-payment.md).

## Initial Backfilled 7 Bills

Backfilled values checked on 2026-06-05:

| Order ID | Goods | Shipping | Coupon | Paid | Status |
|---|---:|---:|---:|---:|---|
| `1107071348495692` | 199.00 | 54.00 | 36.11 | 216.89 | needs_review |
| `1107071348695692` | 808.00 | 139.00 | 115.07 | 831.93 | needs_review |
| `1107071348295692` | 799.00 | 109.00 | 127.79 | 780.21 | needs_review |
| `1107315719495692` | 1618.00 | 235.00 | 38.07 | 1814.93 | needs_review |
| `1107315719095692` | 299.00 | 48.00 | 22.50 | 324.50 | needs_review |
| `1107315719295692` | 180.00 | 54.00 | 7.38 | 226.62 | needs_review |
| `1107473377495692` | 798.00 | 139.00 | 60.40 | 876.60 | needs_review |

Totals:

```text
paid_total = 5071.68
coupon_total = 407.32
shipping_total = 778.00
reconcile_ok = 7/7
```

## User Review Checklist

For each bill detail:

1. Open the bill detail so `SHIP_CUS` fee line is added.
2. Confirm item price is the Lazada goods price before coupon.
3. Confirm Lazada coupon appears as item discount.
4. Confirm `SHIP_CUS` line equals shipping + service fee.
5. Confirm net total matches Lazada paid total.
6. Map item code/unit and confirm match.

Do not send all 7 at once. After user confirms the 7 bills look right, send only 1 bill to SML first.

## Rollout / Rollback

Original rollout order:

1. Keep Lazada IMAP accounts disabled.
2. Backfill existing Lazada bills from artifacts.
3. Set `lazada_email/purchase` fee item config.
4. User opens/checks 7 bill details.
5. Send 1 bill to SML and verify PO total/doc.
6. Poll the next mailbox manually.
7. Enable auto poll one inbox at a time.

Current thaisunsport production state:

- 3 Lazada IMAP accounts are enabled.
- Each account uses `lookback_days=1` to limit AI/token cost.
- Poll interval is 600 seconds.
- Rollback remains disabling the Lazada IMAP accounts in `/settings/email`.

Rollback:

- Disable Lazada IMAP accounts.
- If data rollback is needed, restore from `manual-backups/lazada-amount-20260605_112633`.
- No schema rollback is normally required; migration is additive/idempotent.

## Useful Checks

```sql
SELECT
  COUNT(*) AS lazada_bills,
  COUNT(*) FILTER (WHERE raw_data->>'amount_reconciliation_status'='ok') AS recon_ok,
  COUNT(*) FILTER (WHERE status='needs_review') AS needs_review,
  COALESCE(SUM((raw_data->>'paid_total_amount')::numeric),0) AS paid_total,
  COALESCE(SUM((raw_data->>'coupon_discount_amount')::numeric),0) AS coupon_total,
  COALESCE(SUM((raw_data->>'shipping_amount')::numeric),0) AS shipping_total
FROM bills
WHERE source='lazada_email' AND archived_at IS NULL;
```

```sql
SELECT channel, bill_type, shipping_item_enabled, shipping_item_code, shipping_item_unit_code
FROM channel_defaults
WHERE channel='lazada_email' AND bill_type='purchase';
```

```sql
SELECT name, username, channel, enabled
FROM imap_accounts
WHERE channel='lazada'
ORDER BY username, name;
```

## Related Files

- `backend/internal/handlers/lazada_email.go`
- `backend/internal/repository/bill_lazada_summary.go`
- `backend/internal/repository/bill_print_policy.go`
- `backend/internal/repository/bill_print_payment_method.go`
- `backend/internal/handlers/bills.go`
- `frontend/src/pages/BillDetail/components/BillTotal.tsx`
- `frontend/src/pages/BillDetail/components/BillItemsTable.tsx`
- `frontend/src/pages/BillDetail/components/SendPurchaseDialog.tsx`
- `frontend/src/pages/BulkSendDialog.tsx`
- `frontend/src/pages/ChannelDefaults/EditDialog.tsx`
