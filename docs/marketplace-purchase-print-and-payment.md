# Marketplace Purchase Print And Payment Method

> Updated: 2026-06-09 +07
> Scope: `source='shopee_shipped'` and `source='lazada_email'`, `bill_type='purchase'`.

## Status

- Deployed to `billflow` main and `billflow-thaisunsport`.
- Current migrations include:
  - `058_channel_default_print_policy.sql`
  - `059_marketplace_print_payment_method.sql`
  - `060_marketplace_print_perf_indexes.sql`
- Payment method is stored in BillFlow only. It is not sent to SML.
- Print readiness is controlled by `/settings/channels` per channel and defaults to strict:
  - every order in the same email group must have an SML POL number
  - effective payment method must start with `TT`

## Canonical Terms

| Term | Meaning |
|---|---|
| Marketplace purchase email | Shopee shipped/payment email or Lazada purchase email that creates a local purchase bill |
| Email group | All BillFlow purchase bills created from the same source marketplace email |
| POL | SML purchase order document number stored as `bills.sml_doc_no` |
| Print payment method | BillFlow-only field `bills.print_payment_method`, used for print readiness and print output |
| Effective payment method | `print_payment_method` if explicitly set, otherwise `sml_payload.supplier_name` when it starts with `TT` |

## Payment Method Config

Configured in `/settings/channels` for:

- `shopee_shipped / purchase`
- `lazada_email / purchase`

Default payment method list:

```text
TT2789
TT9630
TT0972
TT9628
TT5128
TT5432
TT3086
TT8456
โอน Kbank
โอน TTB5074
โอน KTB
โอน BBL
โอน TTB1135
```

Values that do not start with `TT` can be saved for future use, but they are not printable in the current business rule.

## Send To SML Dialogs

Both single-bill and bulk-send dialogs include a required `วิธีการชำระเงิน` dropdown for marketplace purchase email bills.

Behavior:

- The dropdown defaults from the selected supplier when supplier code/name starts with `TT`.
- If a bill already has `print_payment_method`, that explicit value wins.
- Before sending SML, frontend calls `PATCH /api/bills/:id/print-payment-method`.
- Single-bill send applies to the whole email group when the bill has an email group.
- Bulk send dedupes by email group and saves the selected method before creating the bulk SML job.
- The selected method is not included in `RetryBillPayload` and is not sent to SML.

Backend guards:

- Only admin/staff can update the payment method.
- Only `shopee_shipped` and `lazada_email` purchase bills can be updated.
- Archived bills cannot be updated.
- The selected method must be in the channel print policy method list.
- Updating before SML send is allowed so staff can choose the method while sending the POL.

## Print Readiness

Backend is the source of truth for row print, bulk print, and detail print.

A marketplace email group is ready to print when:

1. The printable email artifact exists.
2. Every active order in the email group has `sml_doc_no`.
3. Every order has an effective payment method.
4. The effective payment method passes the configured prefix rule, currently `TT`.

The UI exposes this through:

- `/bills?print_ready=1`
- list row print action
- bulk "พิมพ์ทั้งหมดที่พร้อม" flow
- Bill Detail artifact print button

`recordArtifactPrint` still re-validates the rules before recording a print event. This prevents stale UI from printing when another user changes the bill or payment method.

## Print Output

Print HTML keeps the original email content, with BillFlow context added:

- Top banner keeps order/POL context.
- In the original Lazada email body, the POL label is inserted next to the order id.
- Bottom-right overlay shows only:

```text
ชำระด้วยบัตรเครดิต TTxxxx
```

For Lazada print cleanup:

- remote/inline marketplace images are preserved where possible
- broken HTML fragments such as escaped query text must be stripped
- promotional sections starting around "อย่าลืมซื้อสินค้านี้" are removed from the print artifact

## Related SML Mapping

For purchase orders sent to SML:

- `remark` uses seller/supplier name from the source email guard.
- Shopee and Lazada order ids go to `remark_5`.
- Lazada card `doc_ref` stores the summed paid total for every active Lazada purchase order with the exact same `raw_data.email_date`, because Lazada can split one card charge across multiple order emails.
- Lazada send blocks if the card-charge group is incomplete or if the current PO total does not match the order paid total after the `SHIP_CUS` fee line is present.
- User-entered `remark` is not available in purchase send dialogs to avoid overwriting the SML remark field.
- `remark_2` remains available because it is a separate SML field.

## Update Creditor After SML Send

Admins can update the creditor for sent marketplace PO bills:

- Scope: sent `shopee_shipped` or `lazada_email` purchase bills with `sml_doc_no`.
- BillFlow calls `sml-api-bybos` to patch the existing SML PO creditor.
- BillFlow then syncs `sml_payload.cust_code` and `sml_payload.supplier_name`.
- If `print_payment_method` is blank and the new supplier name starts with `TT`, that supplier name becomes the effective payment method.
- If `print_payment_method` is explicitly set, it remains unchanged.

## Performance Notes

`/api/bills` list enrichment batches print readiness by `email_message_id` per request. It must not call print readiness validation per row.

Migration `060_marketplace_print_perf_indexes.sql` adds idempotent indexes for marketplace email groups and printable artifact lookup. The strict `recordArtifactPrint` guard remains separate from list readiness for safety.

## Related Files

- `backend/internal/repository/bill_print_policy.go`
- `backend/internal/repository/bill_print_payment_method.go`
- `backend/internal/repository/bill_email_group.go`
- `backend/internal/database/migrations/058_channel_default_print_policy.sql`
- `backend/internal/database/migrations/059_marketplace_print_payment_method.sql`
- `backend/internal/database/migrations/060_marketplace_print_perf_indexes.sql`
- `frontend/src/pages/BillDetail/components/SendPurchaseDialog.tsx`
- `frontend/src/pages/BulkSendDialog.tsx`
- `frontend/src/pages/Bills.tsx`
- `frontend/src/components/BillTable.tsx`
- `frontend/src/pages/ChannelDefaults/EditDialog.tsx`
