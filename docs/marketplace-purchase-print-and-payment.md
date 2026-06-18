# Marketplace Purchase Print And Payment Method

> Updated: 2026-06-18 +07
> Scope: `source='shopee_shipped'` and `source='lazada_email'`, `bill_type='purchase'`.

## Status

- Deployed to `billflow` main and `billflow-thaisunsport` for the base print/payment workflow. Latest TT auto-sync/clear-blank hardening was deployed to `billflow-thaisunsport` on 2026-06-15.
- Current migrations include:
  - `058_channel_default_print_policy.sql`
  - `059_marketplace_print_payment_method.sql`
  - `060_marketplace_print_perf_indexes.sql`
  - `063_lazada_charge_group_key.sql`
  - `064_credit_card_report_runs.sql` (deployed on `billflow-thaisunsport`; check main before using there)
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

Dynamic `TTxxxx` values derived from selected supplier code/name are allowed even when they are not in the static dropdown config. This prevents SML supplier master drift from blocking users.

## Send To SML Dialogs

Both single-bill and bulk-send dialogs include an optional `วิธีการชำระเงิน` dropdown for marketplace purchase email bills.

Behavior:

- The dropdown auto-syncs and locks to the selected supplier when supplier code/name starts with `TT`.
- If the selected supplier is not `TT`, the dialog clears to blank and the user can choose manually only when needed.
- The field is optional for sending SML. Blank is valid and is persisted as blank when the user clears a previous value.
- Before sending SML, frontend calls `PATCH /api/bills/:id/print-payment-method`.
- Single-bill send applies to the whole email group when the bill has an email group.
- Bulk send dedupes by email group and saves the selected method before creating the bulk SML job.
- The selected method is not included in `RetryBillPayload` and is not sent to SML.

Backend guards:

- Only admin/staff can update the payment method.
- Only `shopee_shipped` and `lazada_email` purchase bills can be updated.
- Archived bills cannot be updated.
- Empty string is accepted to clear the saved method.
- Non-empty selected method must be in the channel print policy method list, or match the dynamic `TTxxxx` prefix policy.
- Updating before SML send is allowed so staff can choose the method while sending the POL.

## Print Readiness

Backend is the source of truth for row print, bulk print, and detail print.

A marketplace email group is ready to print when:

1. The printable email artifact exists.
2. Every active order in the email group has `sml_doc_no`.
3. Every order has an effective payment method.
4. The effective payment method passes the configured prefix rule, currently `TT`.

The row/detail `email_group.print_ready` flag means the group passes those readiness rules. The `/bills?print_ready=1` queue is narrower: it shows only groups that are ready and do not yet have an `email_print_events` record.

The UI exposes the unprinted-ready queue through:

- `/bills?print_ready=1`
- bulk print flow on `/bills`

The list row print action and Bill Detail artifact print button can still print a ready group that has print history when the user intentionally reprints it.

BillFlow treats an email as "printed" after a print event/request is recorded. Browsers do not reliably tell the app whether the user completed or cancelled the native print dialog.

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
- Lazada card `doc_ref` stores the summed paid total for every active Lazada purchase order with the same `raw_data.lazada_charge_group_key`, because Lazada can split one card charge across multiple order emails whose envelope seconds differ.
- Lazada send blocks if the card-charge group is incomplete or if the current PO total does not match the order paid total after the `SHIP_CUS` fee line is present.
- User-entered `remark` is not available in purchase send dialogs to avoid overwriting the SML remark field.
- `remark_2` remains available because it is a separate SML field.

## Credit Card Report

`/credit-card-reports` is a BillFlow-only report for reconciling card statement rows manually. It does not import statement files, upload to Google Drive, or use AI to read bank statements.

Workflow:

1. User selects `date_from`, `date_to`, optional `TTxxxx` payment method, optional source, and whether to include incomplete groups.
2. Backend returns a preview at card-charge group level, not order level.
3. User selects/deselects whole charge groups. This is how users handle statement periods where the first/last day only partially belongs to the card cycle.
4. User creates a report run. The run stores filters, selected group ids, snapshot JSON, and summary JSON.
5. Excel export and report-order print use the stored snapshot only.

Grouping rules:

- Scope is active marketplace purchase bills only: `shopee_shipped` and `lazada_email`.
- Shopee group id is `shopee:<email_message_id>` and charge amount comes from `raw_data.payment_summary.doc_ref_amount`, falling back to `payment_paid_amount`.
- Shopee shipped-only legacy rows without payment summary are excluded by default, unless `include_incomplete=true`.
- Lazada group id is `lazada:<raw_data.lazada_charge_group_key>` and charge amount is the sum of `raw_data.paid_total_amount` in the group.
- Lazada does not fall back to `email_date`; missing group key is an issue because envelope time can split one card charge.
- `payment_method=TTxxxx` filters at group level: if any order in a charge group matches the selected card, the whole group stays together.

Excel output:

- `รายงานบัตรเครดิต`: one row per POL/order, repeating charge group data.
- `สรุปยอด`: totals by source/platform plus run summary.
- `ต้องตรวจสอบ`: groups with issues such as missing POL, missing charge amount, amount mismatch, missing payment method, or mixed payment method.

Print behavior:

- `พิมพ์รายการที่เลือกตามลำดับรายงาน` uses existing email artifact print rendering.
- Print order follows the report snapshot.
- Groups that are not ready to print are skipped with a reason and do not receive fake print events.

## Update Creditor After SML Send

Admins can update the creditor for sent marketplace PO bills:

- Scope: sent `shopee_shipped` or `lazada_email` purchase bills with `sml_doc_no`.
- BillFlow calls `sml-api-bybos` to patch the existing SML PO creditor.
- BillFlow then syncs `sml_payload.cust_code` and `sml_payload.supplier_name`.
- If `print_payment_method` is blank and the new supplier name starts with `TT`, that supplier name becomes the effective payment method.
- If `print_payment_method` is explicitly set, it remains unchanged.

Current hardening note: when a supplier is changed back to another `TTxxxx` supplier, the dialogs sync the payment method to the new TT value again; they do not keep the stale previous TT.

## Performance Notes

`/api/bills` list enrichment batches print readiness by `email_message_id` per request. It must not call print readiness validation per row.

`/bills?print_ready=1`, counts, and email print candidates all use the same ready-and-unprinted predicate. The predicate reuses `idx_email_print_events_message_created` to exclude email groups that already have print events.

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
