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
- Originally, `GET /api/bills?shipping_check=attention` and counts used the cumulative warning
  flag. Shipping alerts do not participate in SML validation.

### Thaisunsport deployment and acceptance

Use `scripts/update-customer-shipping-checks.sh` from an immutable release checkout
with `SOURCE_DIR`, `DEPLOY_DIR=/home/thaisunspot/billflow-thaisunsport`,
`EXPECTED_COMMIT=<full release hash>`, `BACKEND_PORT=8100`, `FRONTEND_PORT=3020`.
The operator runs it via AnyDesk; no direct customer-server deployment was performed
from this workspace. The operator subsequently confirmed both containers healthy
for release `207228e` (backup `shipping-checks-20261117-163147`). The script preserves Compose/feature flags, backs up source
and database, tags previous images, builds/recreates backend/frontend and waits
for both health endpoints. It prints a standalone `backups/shipping-checks-*/rollback.sh`
command. Rollback restores previous source/images, retains the additive migration
and does not restore the database over newer customer transactions.

After configuration, customer acceptance needs three **new orders/new emails**:
approved name/address, changed recipient, changed destination. Changing only the
marketplace address book or refreshing an old bill does not trigger inspection.
Confirm both the list badge/filter and field-level evidence in detail.

Local verification is recorded in `docs/current-state.md`; customer Docker deployment/health has been confirmed by operator output;
real IMAP warning behaviour and customer acceptance remain pending configuration/tests.


To fill the provided company address without typing it repeatedly, run
`scripts/configure-customer-shipping-profile.sh` on the customer server. It adds
profile `thaisunsport-hq-368` (368 moo 10, Khlong Ma Duea, Krathum Baen, Samut Sakhon,
74110) if absent and assigns it only to mailbox policies not already configured.
It does not overwrite saved profiles/policies, approve names from historical
emails, enable checks, reprocess bills or require a service restart. The operator
must still enter approved recipient names per mailbox, preview, enable and save.
The config change records before/after values under actor `server_operator` rather
than attributing the shell operation to an arbitrary application user.


On 2026-11-18 the user approved the company-name variants in the supplied
screenshots and `PD01` for use across current company inboxes. After the profile
helper, `scripts/enable-customer-shipping-checks.sh` merges those explicit names
and enables checking for every **active** Shopee/Lazada IMAP account. It does not
learn names from incoming emails, enable a disabled IMAP account, affect general
mailboxes, or rewrite old bills' frozen rules. It verifies the central address,
uses one config/audit transaction and can be rerun without another change event.
The printed policy table is the operator's activation evidence; local helper tests
alone do not establish production activation or successful new-order alerts.


## Manual historical shipping checks (prepared 2026-11-18)

Bill Detail now displays the shipping recipient/address from the original purchase
email even when the bill was never enrolled. `GET /api/bills/:id/shipping-details`
is read-only and returns extracted fields, an evidence reference and whether
current mailbox rules are ready for a manual check. Reading the source does not
mean the address matches. Unsupported/ambiguous/missing evidence shows its reason.

Admins and staff can click **ตรวจข้อมูลจัดส่ง** (or **ตรวจจัดส่งอีกครั้ง**) on
one historical or current marketplace purchase bill. The new
`POST /api/bills/:id/shipping-checks` accepts `{request_id:<UUID>}`, loads the
**current** rules of that bill's recorded mailbox, checks the original email and
atomically stores the result, cumulative attention summary and user audit. It
works for manual checks even when that mailbox's automatic switch is off, provided
its profile/approved names are valid. Viewers can read evidence/history but cannot
run a manual check. A missing mailbox association/rule set is reported, never
inferred from another inbox.

The UI labels manual results separately from automatic results, with rule revision,
time and checking user's email. Request IDs prevent a retry from duplicating
history/audit. Earlier warnings are retained. Manual checks update the list's
shipping warning/filter only; they do not create a bill, edit items/totals, change
bill/SML status or write `shipping_check_policy`. Old bills therefore remain
unenrolled for subsequent automatic email checks. Existing automatically enrolled
bills retain their original rules for automatic follow-ups; the manual result has
its own current-rule snapshot.

Evidence selection remains source/mailbox/message-scoped. A legacy bill with no
message id can use its own HTML only when exactly one such artifact exists;
otherwise it is unverifiable. No schema migration is needed: manual actor/trigger
metadata uses the existing result JSON, ledger and audit table. Deploy both backend
and frontend using the pinned customer updater, preserving current configuration.
The addition is not deployed until the operator runs that updater.


Shipping evidence dialogs must retain `charset=utf-8` when creating their Blob
URLs, even if the preview response already has a UTF-8 Content-Type: wrapping a
Blob with only `type: text/html` discards that charset. The 2026-11-18 fix explicitly
sets UTF-8 and leaves original artifacts untouched. Browser regression: serve a
Thai HTML body **without** a meta charset via the authenticated preview endpoint,
open the shipping evidence dialog and verify the Thai recipient/address and UTF-8
character set; the empty iframe sandbox must continue blocking embedded scripts.


Parser `shipping-v2` (2026-11-18) also reads a bare house number immediately after
an exact normalized recipient/approved-name prefix. This handles a shipping line
such as `บริษัท ไทยซันสปอร์ต จำกัด(สำนักงานใหญ่) 368 หมู่10 ...` without an
explicit `เลขที่` label. The longest known prefix wins so a tax id included in an
approved recipient name is not misread as the house number. Unknown prefixes and
numbers embedded in other address details remain unverifiable. It does not infer
house numbers by searching for the configured expected number. Old results are
retained; click **ตรวจจัดส่งอีกครั้ง** to record a new v2 result after deployment.
The parser fix alone preserves warning-only behavior. The subsequently approved
opt-in mandatory manual-pass policy is described below.


### Mandatory manual passing check before SML (2026-11-18)

The Thaisunsport customer approved requiring a human click **and a passing result**.
Set `email.shipping_checks.require_manual_pass=true` to opt an instance into this
rule (default false elsewhere). It covers both Shopee/Lazada purchase email sources,
including unsent old bills. Sent documents are not recalled or resent. Disabling
mailbox automatic checks does not disable this requirement; manual checks use the
current profile and approved names even when the automatic switch is off.

The shared single/bulk backend send boundary checks the authoritative latest ledger
record, not `raw_data` or client flags. It must be manual, `match`, current parser
version and equivalent current mailbox rules. No result, an automatic-only pass,
mismatch, unverifiable result, missing configuration or read failure blocks send.
New automatic shipping evidence after a manual check requires another manual check.
Changing unrelated global revision/display names alone does not invalidate a pass.
No staff/admin send override is provided. Prior warnings remain in history (the open-work filter below uses current results);
a current passing manual result can permit send without deleting those warnings.

`GET /api/bills` and `GET /api/bills/:id` include optional `shipping_send_check`
(`required`, `can_send`, `reason`). The detail and bulk dialogs use it for disabled
states and explanation; the backend rechecks before sending, including queued jobs.
Existing item, amount, email completeness and SML readiness checks still apply.

For this customer, the pinned `scripts/update-customer-shipping-checks.sh` now runs
`scripts/require-customer-shipping-checks.sh` after backend/frontend health checks.
The activation transaction requires complete rules for every active marketplace
inbox, preserves existing config, and audits only an actual flag change. If it
fails, the updater exits without SUCCESS and prints its rollback command. Rollback
to older warning-only binaries also restores their older send policy; the additive
ledger/settings are retained. Do not describe rollback as preserving the new gate.

Local validation: all backend tests; PostgreSQL 14 UTF-8 ledger integration and
activation SQL; 10 frontend tests and production build (existing chunk warnings).
Mock browser verified disabled send -> manual check -> refreshed passing state ->
enabled send, without a real SML request. Customer email/UAT and Docker execution
remain separate operator checks.


### Open-work attention counters (2026-11-18)

`GET /api/bills/attention-counts` returns `{shipping, email}` for **bills**, not
email groups. Both counts cover unarchived Shopee/Lazada purchase email bills in
`pending`, `needs_review` or `failed`, independently of search/date/account/source,
other UI filters and pagination. A bill may appear in both groups; never sum them.
Email groups without any BillFlow bill remain visible in the existing email-group
attention panel but are not included in these bill counters.

The shipping count/filter uses the latest summary `status` of `mismatch` or
`unverifiable`, rather than cumulative `attention`. A later matching check removes
the bill from open shipping work, preserving historical ledger/attention flags and
the existing history badge. Missing checks and automatic matches awaiting a manual
click are not shipping abnormalities; the separate SML manual-pass gate still
applies. Email count/filter uses groups in `processing` or `attention`, with EXISTS
so multiple matching group rows do not duplicate a bill. Sent/skipped/archived bills
are excluded from both attention filters as well as the global counts.

The Bills page shows two clickable counters above its filters. Clicking an inactive
counter clears conflicting filters and selects its queue on page 1. Clicking an
active counter again now cancels only that filter (the original repeated-click
refresh behavior was superseded). Both dropdown selections are reflected separately.
Active counters display an X and remain clickable at zero and during count errors.
The always-visible **ล้างตัวกรอง** clears all list filters, returns to active
bills/page 1 and preserves page size/sort preference. Filtered empty results say
**ไม่พบรายการตามตัวกรอง** and offer the same clear action. Existing dropdowns still allow narrowing the queue; the counter
labels explicitly describe the global queue. Refresh on mount, window focus,
visibility return, local work-queue change/manual check and every 30 seconds while
visible. This is polling, not server push. Loading shows `…`; failure clears stale
numbers to `—` with a retry, never fabricated zero. No sound, popup or LINE.

No migration or config change is required for the counters. Deploy backend and
frontend together using the pinned customer updater; it preserves the existing
mandatory shipping gate. Verified PostgreSQL 14 integration: historical match,
latest mismatch/unverifiable, sent/skipped/archived/nonpurchase exclusion, email
join duplication, membership parity and count decrease after resolution. Browser
API mocks verified badge clicks/filter reset, same-badge refresh, failure/retry,
zero and 30-second polling. Customer deployment and production data remain operator
validation.


### Lazada area-first address / shipping-v3 (2026-11-18)

Some original Lazada emails serialize the shipping address as amphoe, province,
postcode, then `368 หมู่10`, followed by district and repeated postcode. v3 adds
house extraction from a comma/semicolon/middle-dot/newline/start-delimited segment
with a numeric house immediately followed by `หมู่` / `หมู่ที่`. This is structural
parsing independent of the configured expected house. Existing explicit house and
known-name-prefix rules remain; all extracted candidates must agree. Full slash,
dash, dot and letter suffixes are retained, and soi/postcode tokens alone do not
satisfy the house field. Unapproved recipient PD02 remains a separate name mismatch;
no automated allowlist expansion was added.

After deployment, rerun the manual check on bills to be sent: the mandatory gate
rejects older passing parser versions, including v2, until checked again. Existing
history stays unchanged. The regression uses synthetic HTML containing the text
visible in the customer's screenshots, not the customer's full source HTML.

Permission clarification for support: `staff` cannot archive/restore/delete bills
or delete bill items via the configured API routes. Marketplace price/discount/
line-total edits require `admin`; mapping product codes permits admin/staff.
These are application roles, not job titles. A purchasing employee using an admin
account has those capabilities. Verify customer account assignments before saying
that a named employee cannot delete. This patch does not change permissions or
claim that permanent deletion retains all historical bill evidence.


### Compact list header (2026-11-18)

The list now leads with its title and the two attention counters. Status counts
appear once in the status tabs (a standard Select on small screens). Search stays
visible. **ตัวกรองเพิ่มเติม** expands the existing channel, shipping/email,
payment, order-status, date, inbox/shop and archive controls; disclosure starts
collapsed. Selected values remain visible as removable chips even when the panel
is closed, with a count on the extra-filter trigger and **ล้างตัวกรอง** alongside
search. Removing one chip changes only its corresponding filter and returns to
page 1. Existing attention toggle/clear behavior stays in effect.

Sort and bulk-send actions are next to the bill list. **พิมพ์อีเมล** contains the
print-ready filter, print action and explanatory text; **ปลายทาง SML** reveals the
route and channel settings link. These are layout/disclosure changes: the same
source/status/archive/role restrictions, printing prerequisites and backend manual
shipping pass still apply. Counter API failure/retry remains visible in the compact
counter display. No hidden filter is silently removed by collapsing its panel.
