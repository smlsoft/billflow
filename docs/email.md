# Email IMAP — การทำงานของ Email Pipeline

> อัพเดตล่าสุด: 2026-06-15
> สถานะ: ✅ multi-account IMAP deployed; config อยู่ใน `/settings/email` และ `imap_accounts` table. Marketplace purchase email ใช้ manual-review flow ไม่ auto-send SML.

---

## ภาพรวม

BillFlow poll Gmail/Outlook/IMAP อื่นตาม inbox ที่ admin เพิ่มใน `/settings/email` เพื่อตรวจหา email ใหม่ แล้ว route ตาม `imap_accounts.channel`.

| channel | ใช้กับ | ผลลัพธ์ |
|---|---|---|
| `general` | บิลทั่วไป PDF/Excel/รูปแนบ | AI/OCR → local bill → admin review/retry |
| `shopee` | Shopee purchase/order email | `shopee_shipped` purchase flow หรือ Shopee sale routing ตาม subject |
| `lazada` | Lazada Thailand purchase email | `source='lazada_email'`, `bill_type='purchase'`, admin review ก่อนส่ง SML |

> Marketplace email purchase flow ไม่ auto-send เข้า SML. ระบบสร้างบิล, map/candidate items, เก็บ artifacts, แล้วให้ admin ตรวจใน `/bills` / Bill Detail ก่อนกดส่ง.

---

## Flow ทั้งหมด

```
┌────────────────────────────────────────────────────────────┐
│  Background: EmailCoordinator                                │
│                                                            │
│  ► one goroutine per enabled imap_accounts row             │
│  ► poll ทุก poll_interval_seconds (ขั้นต่ำ 300 วินาที)    │
└────────────────┬───────────────────────────────────────────┘
                 │
                 ▼
        IMAP.Poll(account) — connect → mailbox TLS
                 │
                 ▼
        SELECT INBOX WHERE UNSEEN
        (ค้นหา email ที่ยังไม่อ่าน)
                 │
          ┌──────┴──────┐
          │ ไม่มี email  │  → disconnect → รอ poll ถัดไป
          └─────────────┘
                 │
          มี email UNSEEN
                 │
                 ▼
        ┌ Loop ทุก message ┐
        │                  │
        │  filter ตาม account config:
        │  - filter_from
        │  - filter_subjects[]
        │  - channel: general / shopee / lazada
        │  - shopee_domains[] สำหรับ Shopee routing
        │  ถ้าไม่ผ่าน filter → ข้ามไป
        │                  │
        │  route ตาม channel/subject:
        │    - Lazada purchase guard: from+subject ต้องตรง Lazada เท่านั้น
        │    - Shopee purchase guard: subject/accepted domain ต้องตรง
        │    - general: parse body → attachments
        │
        │  parse email body/attachments
        │  รองรับ:
        │    application/pdf (AttachmentHeader หรือ InlineHeader)
        │    image/jpeg, image/png
        │    application/vnd.openxmlformats-officedocument.spreadsheetml.sheet
        │                  │
        │  ┌ Loop ทุก attachment ┐
        │  │                     │
        │  │  ProcessAttachment(data, mimeType, filename)
        │  │     │
        │  │     ▼
        │  │  ถ้า PDF → Mistral OCR → markdown text
        │  │  ถ้า รูป → ส่ง Gemini โดยตรง (base64)
        │  │     │
        │  │     ▼
        │  │  AI Extract (Gemini 2.5 Flash)
        │  │  → {customer_name, items[{raw_name, qty, unit, price}], confidence}
        │  │     │
        │  │     ▼
        │  │  F1 Mapper: raw_name → item_code/unit_code
        │  │     │
        │  │     ▼
        │  │  F2 Anomaly: ตรวจราคา, qty, ซ้ำ
        │  │     │
        │  │     ▼
        │  │  Save bill + items → PostgreSQL
        │  │     │
        │  │     ├── allMapped AND confidence ≥ 0.85 AND ไม่มี block
        │  │     │         │
        │  │     │         ▼
        │  │     │   SML CreateSaleReserve
        │  │     │   → success: status = 'sent', doc_no = BS...
        │  │     │   → fail:    status = 'failed' + LINE admin notify ⚠️
        │  │     │
        │  │     └── ไม่ผ่าน condition
        │  │               │
        │  │               ▼
        │  │         status = 'pending'
        │  │         LINE admin notify 📋
        │  │
        │  └────────────────────┘
        │
        │  mark email เป็น SEEN (อ่านแล้ว)
        │  ← เฉพาะเมื่อ process/skip เสร็จอย่างตรวจสอบได้
        │        │  dedup check: Message-ID
        │  SELECT COUNT(*) FROM bills WHERE raw_data->>'message_id' = ?
        │  ← ป้องกัน process ซ้ำ ถ้า email ถูก mark unread โดยไม่ตั้งใจ
        │        └───────────────────────┘
                 │
                 ▼
        disconnect IMAP
```

---

## Lazada Email Purchase Flow

เริ่มใช้จริงบน `billflow-thaisunsport` วันที่ 2026-06-05 สำหรับอีเมล Lazada Thailand purchase (`channel='lazada'` ใน IMAP account แต่ bill source เป็น `lazada_email`).

### Guard / routing

- รับเฉพาะเมล Lazada ที่ผ่าน whitelist sender/domain + subject guard.
- noise เช่น E-invoice, cancellation, dispute, survey/review, delivery-reschedule ไม่สร้าง bill.
- duplicate guard ใช้ `source='lazada_email' + order_id` และ `processed_email_keys`; confirm/shipped ของ order เดียวกันต้องไม่สร้างซ้ำ.
- Lazada IMAP accounts บน thaisunsport เปิดใช้งานแล้ว 3 กล่อง, `lookback_days=1`, `poll_interval_seconds=600` เพื่อคุม AI/token cost.

### Amount reconciliation

Lazada ไม่ใช้ Shopee Coin logic. ระบบ parse HTML summary จริง แล้ว validate:

```text
ยอดรวมสินค้า + ค่าจัดส่ง + Service fee - คูปองส่วนลด = ยอดรวมทั้งหมด(รวม VAT)
```

Tolerance: `±0.01`.

Fields ที่เก็บใน `bills.raw_data`:

- `goods_total_amount`
- `shipping_amount`
- `coupon_discount_amount`
- `service_fee_amount`
- `paid_total_amount`
- `shipping_method`
- `payment_method`
- `amount_reconciliation_status`
- `amount_reconciliation_delta`

ถ้า `amount_reconciliation_status != "ok"` backend จะ block การส่ง SML แม้ user/bulk-send เรียก API โดยตรง.

### Items / discount / fee line

- `price` ของสินค้า = ราคาก่อนคูปอง.
- `bill_items.discount_amount` = คูปอง Lazada กระจาย proportional ตามมูลค่าสินค้า ไม่รวมค่าส่ง/fee.
- ค่าส่ง + service fee ใช้ fee line เดียว:
  - source SKU: `__lazada_shipping_fee__`
  - SML item config: `/settings/channels` row `lazada_email/purchase`
  - ใช้ fields เดิม `shipping_item_enabled`, `shipping_item_code`, `shipping_item_unit_code`
- ถ้ายอด Lazada มีค่าส่ง/fee แต่ยังไม่ได้ตั้งค่าสินค้า SML สำหรับ fee line ระบบจะ block ส่ง SML.
- เมื่อ config พร้อมแล้ว Lazada bill ใหม่จะเติม fee line ตั้งแต่ตอนสร้างบิล; หน้า Bill Detail จะไม่ auto-add ตอนเปิดหน้าแล้ว.

### Current thaisunsport rollout snapshot

- Backfilled 7 Lazada bills: reconciliation `ok` ทั้ง 7; customer confirmed the numbers and at least one Lazada PO sent to SML successfully.
- Current active Lazada email purchase rows are mixed across `needs_review`, `pending`, and `sent` as customer testing continues.
- Latest Lazada charge-group backfill updated 28 rows. The 8-order group confirmed at `2026-06-11 16:45` totals `7417.69`; already-sent POL docs in that group were repaired to `doc_ref=7417.69`.
- `channel_defaults/lazada_email/purchase` ตั้ง fee item แล้ว: `SHIP_CUS`, unit `บาท`.
- บิล Lazada active unsent ที่สร้างก่อน behavior นี้ให้ซ่อมด้วย `./lazada_fee_line --dry-run` แล้ว `./lazada_fee_line --apply`; command เป็น idempotent และข้าม sent/archived.

Runbook เพิ่มเติม: [Lazada Email Purchase Intake](lazada-email-purchase.md).

### Marketplace print/payment method

Shopee/Lazada purchase email print is controlled by BillFlow-only payment method rules:

- single and bulk SML send dialogs do not require choosing `วิธีการชำระเงิน`
- when selected supplier code/name starts with `TT`, the method auto-syncs and locks to that TT value
- when selected supplier is non-TT, the saved method can be blank
- value is saved in `bills.print_payment_method`
- value is not sent to SML
- default effective value comes from `sml_payload.supplier_name` only when it starts with `TT`
- print readiness requires every order in the email group to have POL and effective payment method to pass the configured prefix rule, currently `TT`

Runbook: [Marketplace Purchase Print And Payment Method](marketplace-purchase-print-and-payment.md).

### Marketplace email completeness

สำหรับ Shopee/Lazada purchase email ระบบจะบันทึกเลขคำสั่งซื้อที่พบจากอีเมลต้นฉบับก่อนเรียก AI แล้วเทียบกับบิลที่สร้างสำเร็จภายหลัง เพื่อให้กรณีอ่านพลาดหรือสร้างบิลไม่ครบไม่หายไปจากงานของผู้ใช้:

- หน้า `/bills` แสดงป้าย `ครบ X/Y` หรือ `ขาด X/Y` บนบิลที่เกี่ยวข้อง และมีตัวกรอง `อีเมลที่ต้องตรวจสอบ`
- หน้า `/bills` มีแถบเตือนแยกสำหรับอีเมลที่ไม่มีบิลถูกสร้างเลย พร้อมบอกจำนวนที่ขาด
- Email group ที่ยังไม่ครบพิมพ์ไม่ได้
- ตั้ง `EMAIL_GROUP_COMPLETENESS_ENFORCED=true` เพื่อ block การส่ง SML ด้วย backend ทุก route รวม bulk send; admin override ได้เฉพาะการส่งทีละบิลและต้องระบุเหตุผล ส่วน bulk send จะข้ามรายการที่ยังไม่ครบเสมอ
- ค่า default เป็น `false` เพื่อ rollout แบบ shadow mode: ระบบแสดงสถานะและ log ก่อน โดยยังไม่ block SML

---

## คำถามที่พบบ่อย

### ถ้า mark email กลับเป็น unread แล้วรอ 5 นาที จะส่ง SML ได้เลยไหม?

**ได้เลย** — IMAP poller ค้นหา `UNSEEN` (unread) messages
ถ้า mark กลับเป็น unread → email กลายเป็น UNSEEN → poll ถัดไป (ภายใน 5 นาที) จะ pick up ใหม่

```
timeline:
  14:00  ← email ถูก process → mark SEEN → bill = pending (unmapped)
  14:05  ← poll: ไม่เจออะไร (email SEEN อยู่)
  14:10  ← พนักงานเพิ่ม mapping ใน /mappings
  14:10  ← mark email กลับเป็น UNREAD
  14:15  ← poll: เจอ UNSEEN → process ใหม่ → allMapped = true → SML ✅
```

> **หรือใช้ Retry Handler แทน** (ไม่ต้อง unread email):
> `POST /api/bills/:id/retry` → re-map items ด้วย mapping ใหม่ → ส่ง SML ทันที

---

### ถ้าเพิ่ม mapping แล้ว ต้องรอ 5 นาทีไหม?

ไม่ต้องรอ — ใช้ **retry handler** แทน:
1. เปิด Web UI → `/bills` → เลือก bill ที่ pending
2. กด **Retry** → ระบบ re-map ด้วย mapping ใหม่ → ส่ง SML ทันที

---

### poll ถี่ได้ไหม?

Gmail มี rate limit — ถ้า poll ถี่กว่า 5 นาที จะเกิด `unexpected EOF`
ระบบบังคับ `poll_interval_seconds >= 300` ใน DB

---

## PDF ทำงานยังไง (Mistral OCR)

Gmail ส่ง PDF บางฉบับเป็น `Content-Disposition: inline` (ไม่ใช่ attachment)
BillFlow รองรับทั้ง 2 กรณี:

```
Email → Part header
  AttachmentHeader (Content-Disposition: attachment)  → ดาวน์โหลด
  InlineHeader     (Content-Disposition: inline)      → ดาวน์โหลด (ถ้าเป็น PDF หรือรูป)
```

หลัง download PDF:
```
PDF bytes (base64)
    │
    ▼
Mistral OCR API (mistral-ocr-2512)
    │
    ▼
Markdown text (ข้อความจากทุกหน้า)
    │
    ▼
Gemini ExtractText(markdownText)
    │
    ▼
{customer_name, items, confidence} JSON
```

เหตุผลที่ใช้ Mistral OCR แทน Gemini PDF:
OpenRouter route Gemini ผ่าน Amazon Bedrock → ไม่รองรับ `application/pdf` MIME type โดยตรง

---

## IMAP Authentication

ใช้ **SASL PLAIN** (ไม่ใช่ `Login` command ธรรมดา)

```go
// go-imap/v2 beta.8 + go-sasl
c.Authenticate(sasl.NewPlainClient("", user, password))
```

สาเหตุ: Gmail advertises `AUTH=PLAIN AUTH=XOAUTH2` via CAPABILITY
`Login` command ถูก reject → ต้องใช้ `AUTHENTICATE PLAIN` แทน

---

## Error Handling

| กรณี | การจัดการ |
|---|---|
| IMAP connect ล้มเหลว | log error + LINE admin notify (throttle 1 ครั้ง/ชม.) |
| AI extract ล้มเหลว | log error + LINE admin notify, ไม่สร้าง bill |
| AI อ่านเลข Shopee ไม่ตรงกับเลขในอีเมล | ไม่สร้าง bill, เก็บอีเมลต้นฉบับใน `email_ingestion_failures` เพื่อ replay เฉพาะฉบับ |
| ไม่มี items ใน extract | log warning, ไม่สร้าง bill |
| Items ไม่ match mapping | bill = `needs_review`/`pending` ตาม flow + LINE admin notify 📋 |
| Lazada amount formula mismatch | bill = `needs_review`, block send SML จนกว่าจะตรวจ/แก้ |
| Lazada fee config missing | block send SML ถ้ามีค่าส่ง/fee ที่ต้องส่งเข้า PO |
| SML ล้มเหลว (3 retry) | bill = 'failed' + LINE admin notify ⚠️ |
| Email mark SEEN ล้มเหลว | ถูก process ซ้ำใน poll ถัดไป (idempotent ถ้า auto-confirm ผ่าน) |

---

## Config ที่เกี่ยวข้อง

IMAP ไม่มี `.env IMAP_*` singleton แล้ว ให้ตั้งผ่าน UI:

| Field | Table column |
|---|---|
| Host/Port/User/Password/Mailbox | `imap_accounts.host`, `port`, `username`, `password`, `mailbox` |
| Filters | `filter_from`, `filter_subjects[]` |
| Routing | `channel`, `shopee_domains[]` |
| Timing | `poll_interval_seconds` |
| Runtime status | `last_polled_at`, `last_poll_status`, `last_poll_error`, `consecutive_failures` |

AI/OCR ยังมาจาก env:

```bash
OPENROUTER_MODEL=google/gemini-2.5-flash
OPENROUTER_FALLBACK_MODEL=anthropic/claude-3-5-haiku  # server current value
MISTRAL_API_KEY=
AUTO_CONFIRM_THRESHOLD=0.85
```

---

## ขั้นตอน Debug เมื่อ email ไม่ถูก process

### Replay เฉพาะอีเมล Shopee

เมื่อ audit log แสดง `shopee_shipped_order_ids_rejected` ระบบจะไม่สร้างบิลหรือส่ง SML จากอีเมลฉบับนั้น และเก็บหลักฐานไว้ใน `email_ingestion_failures` แทนที่จะบันทึกเป็นอีเมลที่เสร็จแล้ว

ผู้ดูแลสามารถเรียก `POST /api/settings/imap-accounts/:id/replay-message` พร้อม JSON `{"message_id":"..."}` เพื่ออ่านเฉพาะ Message-ID นั้นใหม่ได้ การ replay จะไม่ reset cursor และไม่สแกนอีเมลอื่นในกล่อง เมื่อลองอ่านใหม่ ระบบยังตรวจเลขคำสั่งซื้อจากอีเมลต้นทางก่อนสร้างบิลทุกครั้ง

```bash
# 1. ดู logs
docker logs billflow-backend --tail=50 2>&1 | grep -i "imap\|email\|poll"

# 2. ตรวจ IMAP config/status ใน DB
docker exec billflow-postgres psql -U billflow -d billflow \
  -c "SELECT name, enabled, last_poll_status, consecutive_failures, last_poll_error FROM imap_accounts;"

# 3. ทดสอบ IMAP connection ด้วย curl
curl -v --ssl-reqd 'imaps://imap.gmail.com:993/INBOX' \
  --user 'email@gmail.com:apppassword16หลัก' 2>&1 | head -20

# 4. ดูบิลใน DB
docker exec billflow-postgres psql -U billflow -d billflow \
  -c "SELECT id, status, error_msg, created_at FROM bills WHERE source='email' ORDER BY created_at DESC LIMIT 5;"
```

**Checklist:**
- [ ] `poll_interval_seconds` ≥ 300
- [ ] Gmail: 2FA เปิดอยู่ + ใช้ App Password (ไม่ใช่ password จริง)
- [ ] Gmail: เปิด IMAP ใน Settings → Forwarding and POP/IMAP
- [ ] Email เป็น UNSEEN (ยังไม่ได้อ่าน)
- [ ] Filter ตรงกับ email ที่ส่ง (หรือลอง clear filter ก่อน)

---

## ไฟล์ที่เกี่ยวข้อง

| ไฟล์ | หน้าที่ |
|---|---|
| `backend/internal/services/email/coordinator.go` | one goroutine per enabled account |
| `backend/internal/services/email/account.go` | account runtime/update helpers |
| `backend/internal/services/email/imap.go` | IMAP connect, search UNSEEN, fetch, parse, mark SEEN |
| `backend/internal/handlers/email.go` | AttachmentProcessor: OCR → extract → map → anomaly → DB → SML |
| `backend/internal/handlers/lazada_email.go` | Lazada email purchase intake → local purchase bill |
| `backend/internal/repository/bill_lazada_summary.go` | Lazada HTML amount summary parser + discount allocation |
| `backend/internal/handlers/imap_settings.go` | `/settings/email` APIs |
| `backend/internal/services/mistral/ocr.go` | Mistral OCR API (PDF → markdown) |
| `backend/internal/services/ai/openrouter.go` | ExtractText, ExtractImage, ExtractPDF |
| `backend/internal/repository/bill_repo.go` | Create, UpdateStatus, UpdateBillItem, UpdatePriceHistory |
| `backend/internal/handlers/bills.go` | Retry handler — re-map + re-send SML |


## Shipping checks — warning-only (2026-11-17)

Shipping checks apply to new marketplace purchase bills (`shopee_shipped`,
`lazada_email`) only when enabled for their IMAP account. They do not alter bill
status, totals, mapping, SML eligibility, or send payloads. No LINE notification is
sent. Missing invoice information is irrelevant: only the explicit shipping
section is inspected, deterministically, from retained/source HTML.

### Configure and preview

1. Open `/settings/email` → **ตั้งค่าและทดลองตรวจ** → select the mailbox.
2. Create/select the shared company address profile and fill house number, moo,
   district, amphoe, province and postcode. Add English area aliases where used in
   the actual emails (for example `Khlong Ma Duea`, `Krathum Baen`, `Samut Sakhon`).
3. Enter each approved recipient name and press Enter. Names such as `PD01` or a
   company name ending `05` are not automatically approved.
4. Select a historical sample from that mailbox and click **ทดลองตรวจ**. This uses
   the unsaved form values and the original purchase email; it does not save a
   result, enrol an old bill, create a bill, or send anything to SML.
5. Enable the switch and save. Empty deployment config is OFF by default. Only
   admins can read/change this config or run previews; changes are audited.

A shared profile edit affects future bills in every mailbox that selects it.
Each newly enrolled bill freezes its own rules; follow-up emails use that frozen
snapshot. Turning off a mailbox stops enrolment of future bills, but enrolled
bills continue recording follow-ups. Neither editing config nor receiving a
matching follow-up clears an earlier warning.

### Results and evidence

The bills list shows a shipping badge separately from the normal bill/SML status.
The **ข้อมูลจัดส่งผิดปกติ** filter includes both mismatches and unverifiable checks,
and applies before pagination. Detail shows each field's expected/observed values,
rule revision, timestamp, history and a sandboxed original-email viewer.

- `match`: required recipient and address components match the frozen rules.
- `mismatch`: at least one observed required value conflicts with the rules.
- `unverifiable`: missing/unsupported/ambiguous source data; never treated as a pass.
- No enrolment/result: **ยังไม่ได้ตรวจ**, including historical bills.

A single unambiguous shared shipping block can serve a multi-order email; multiple
shipping blocks or recipients are unverifiable. Extra soi text is allowed; soi is
not separately validated in v1. Unknown English address text remains unverifiable
instead of being silently ignored. HTML templates can change, so customer smoke
checks remain necessary. This feature flags source recipient/destination changes;
it does not establish whether a purchase was authorised or what happened to goods.

### Storage and interfaces

- Config: non-secret `app_settings` key `email.shipping_checks`; revision-checked
  writes and audit insert occur in one transaction.
- New bills: `raw_data.shipping_check_policy` snapshots the applicable profile,
  recipient names and config revision; `shipping_check_summary` carries current
  status plus cumulative `attention`/`has_warning` flags.
- Migration `074_shipping_checks.sql`: append-only per-bill/message result ledger,
  evidence reference, history index and partial attention index. No old-bill scan.
- `GET/PUT /api/settings/shipping-checks` (admin); PUT sends the full returned
  config with `revision`, returns 409 for stale edits.
- `POST /api/settings/shipping-checks/preview` (admin): `{account_id,bill_id,config}`
  returns `{result,artifact_id?,artifact_bill_id?}`; bill and shared evidence must
  belong to the same mailbox/source.
- `GET /api/bills/:id/shipping-checks`: `{data:[...]}` using existing bill-reader
  access. Config endpoints expose no mailbox credentials.
- `GET /api/bills?shipping_check=attention` and counts use the cumulative warning
  flag. Shipping alerts do not participate in SML validation.

### Thaisunsport deployment and acceptance

Use `scripts/update-customer-shipping-checks.sh` from an immutable release checkout
with `SOURCE_DIR`, `DEPLOY_DIR=/home/thaisunspot/billflow-thaisunsport`,
`EXPECTED_COMMIT=<full release hash>`, `BACKEND_PORT=8100`, `FRONTEND_PORT=3020`.
The operator runs it via AnyDesk; no direct customer-server deployment was performed
from this workspace. The script preserves Compose/feature flags, backs up source
and database, tags previous images, builds/recreates backend/frontend and waits
for both health endpoints. It prints a standalone `backups/shipping-checks-*/rollback.sh`
command. Rollback restores previous source/images, retains the additive migration
and does not restore the database over newer customer transactions.

After configuration, customer acceptance needs three **new orders/new emails**:
approved name/address, changed recipient, changed destination. Changing only the
marketplace address book or refreshing an old bill does not trigger inspection.
Confirm both the list badge/filter and field-level evidence in detail.

Local verification is recorded in `docs/current-state.md`; production migration,
Docker build, real IMAP intake and customer acceptance are pending operator results.
