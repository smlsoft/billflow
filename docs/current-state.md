# BillFlow — Current State

> Updated: 2026-05-09 07:40 +07
> Source of truth checked: local code/migrations, `docker compose` on `192.168.2.109`, production `/health`, production PostgreSQL schema, and active Cloudflare Quick Tunnel checks.

## Latest Handoff For New Chat

ถ้าเปิดแชทใหม่ ให้เริ่มจากสถานะนี้:

- BillFlow ปกติยังอยู่ที่ `http://192.168.2.109:3010` / backend `8090`.
- Thaisunsport แยก instance อยู่ที่ frontend `3020`, backend `8100`, postgres `5448`; ยังไม่ควร deploy ทับจนกว่าจะ demo เสร็จ.
- Henna customer trial ถูกสร้างใหม่จาก BillFlow ปกติ ไม่ใช่ Thaisunsport:
  - Public URL: `https://aurora-enjoyed-backup-lines.trycloudflare.com/login`
  - Frontend `3030`, backend `8110`, postgres `5458`
  - Server folder `/home/bosscatdog/billflow-henna`
  - Containers `billflow-henna-frontend`, `billflow-henna-backend`, `billflow-henna-postgres`
- Instance/port registry อยู่ที่ [deploy-instances.md](deploy-instances.md). ใช้ไฟล์นี้เป็น source of truth เมื่อต้องจำ port/tunnel ของแต่ละร้าน.
- `/setup` ถูกยกระดับเป็นหน้าเริ่มต้นใช้งาน:
  - แสดงความพร้อมของ SML, เส้นทางเอกสาร, email, สินค้าใน SML, AI
  - แสดงชื่อร้าน, ฐานข้อมูล SML, AI ที่ใช้งาน, เวลาดึงสินค้า/อ่านอีเมล/นำเข้าล่าสุด
  - แสดงจำนวนเอกสารค้างแยกซื้อ/ขาย/saleinvoice และจำนวน import/log
  - มีปุ่ม `ล้างข้อมูลทดสอบ` สำหรับ admin ล้าง bills/import runs/logs โดยไม่แตะ settings/catalog/mappings/AI usage
  - ตัวเลือกรีเซ็ตเลขรันเอกสารและล้างประวัติอีเมลที่เคยอ่านแล้วต้องเลือกเอง เพราะมีความเสี่ยง doc_no ซ้ำหรืออ่านอีเมลเก่าซ้ำ
  - ถ้าการตั้งค่าหลักพร้อมแล้วแต่ยังมีเอกสารค้าง ระบบจะแยกข้อความเป็น “ระบบพร้อมใช้งาน มีงานค้างให้จัดการ” พร้อมปุ่มไปตรวจเอกสาร/เอกสารพร้อมส่ง/log ที่เกี่ยวข้อง
- BillFlow main deploy แล้วและทดสอบล้างข้อมูลทดสอบจริงแล้ว:
  - bills/import runs ถูกล้างเป็น 0
  - audit logs เหลือ 1 รายการจาก action reset เอง
  - doc counters ไม่ถูก reset
  - ประวัติอีเมลที่เคยอ่านแล้วไม่ถูกล้าง
- UI wording pass ล่าสุดเปลี่ยนคำเทคนิคในหน้าหลักให้เป็นภาษาพนักงานทั่วไปแล้ว เช่น `Reset UAT` → `ล้างข้อมูลทดสอบ`, `UAT Snapshot` → `สรุปข้อมูลทดสอบ`, `AI Control Center` → `การใช้งาน AI`.
- Sidebar ล่าสุดแยก `งานฝั่งซื้อ` และ `งานฝั่งขาย` แล้ว:
  - งานฝั่งซื้อ: `ใบสั่งซื้อ`
  - งานฝั่งขาย: `ใบสั่งขาย`, `ขายสินค้าและบริการ`
- Sidebar badge ล่าสุดนับแยกเมนู ไม่ใช้เลข pending รวมทั้งระบบ:
  - `ใบสั่งซื้อ`: source `shopee_shipped`, bill_type `purchase`
  - `ใบสั่งขาย`: source `shopee`, bill_type `sale`, document_route `saleorder`
  - `ขายสินค้าและบริการ`: source `shopee`, bill_type `sale`, document_route `saleinvoice`
  - นับเฉพาะ `pending + needs_review + failed`
- ทั้ง 3 หน้าเอกสารมีปุ่ม `ส่ง SML ทั้งหมด` สำหรับสถานะ `pending` พร้อม bulk preview/validation ก่อนส่งจริง.
- Shopee Excel import ล่าสุดรองรับปลายทาง SML ทั้ง `saleorder` และ `saleinvoice`; เมื่อ channel default เปลี่ยน endpoint เมนูและข้อความจะเปลี่ยนตาม.
- `ขายสินค้าและบริการ` (`saleinvoice`) ใช้ endpoint ใหม่ `POST /SMLJavaRESTService/saleinvoice/v4`; deploy แล้วเฉพาะ BillFlow main และ BillFlow Henna.
- Shopee SKU ถูกเก็บแยกเป็น `bill_items.source_sku`; ถ้า SKU ไม่มีใน SML Catalog จะไม่เอาไปใส่เป็น `item_code`.
- REST SML retry ตรวจซ้ำว่า `item_code` มีอยู่ใน Catalog จริงก่อนส่ง.
- Saleinvoice test ล่าสุด: `BF-INV26050001` ส่ง payload มี `doc_ref_date: "2026-03-10"` แล้ว ถ้า SML UI ไม่แสดงให้ dev SML ตรวจ API mapping.
- UX hardening ล่าสุดสำหรับ Phase 1+:
  - หน้า Dashboard และหน้าเอกสารแสดง empty state พร้อมปุ่มไปงานถัดไปเมื่อยังไม่มีบิลหลังล้างข้อมูลทดสอบ
  - `/settings/channels` ซ่อน API path เป็น `รายละเอียดขั้นสูง` เพื่อไม่ให้พนักงานทั่วไปสับสน
  - dialog ส่ง SML ทั้งแบบรายใบและแบบส่งทั้งหมดแสดง field ที่ยังขาด เช่น ลูกค้า/ผู้ขาย, คลัง, พื้นที่เก็บ, ภาษี, เวลาเอกสาร ก่อนกดส่ง
  - bulk send แสดงข้อความเตือนเมื่อมีเอกสารพร้อมส่งเกิน 100 รายการ และแสดง error จาก backend/SML ในแถวที่ส่งไม่สำเร็จ
  - `/settings/email` แสดง `ผู้ส่ง Shopee ที่ยอมรับ` ในตาราง และ backend จะบันทึกคำเตือนภาษาไทยเมื่ออีเมลถูกข้ามเพราะผู้ส่งไม่ตรง
  - `/logs` แสดงคำแนะนำว่า error นั้นผู้ใช้แก้เองได้หรือควรส่งให้ทีมดูแลระบบ/SML API
  - `/api/bills` รองรับทั้ง `per_page` และ `page_size`, และคืน `data: []` แทน `null` เมื่อไม่มีข้อมูล
- Email accepted-sender update:
  - ช่องเดิม `shopee_domains` ใน DB ยังใช้ชื่อเดิมเพื่อเลี่ยง migration เสี่ยง แต่ UI แสดงเป็น `ผู้ส่งที่ยอมรับ`
  - ใส่ได้ทั้งโดเมนและอีเมลเต็ม เช่น `shopee.co.th`, `mail.shopee.co.th`, `billing@example.com`
  - เว้นว่าง = รับทุกผู้ส่งที่ผ่านคำกรองหัวข้อ
  - backend ลด warning ซ้ำจากการ poll อีเมล เพื่อไม่ให้ `/settings/email` แสดง error ยาวเกินจำเป็น
- AI model update:
  - production log ยืนยันว่า `google/gemma-4-26b-a4b-it:free` fail แล้ว fallback ไป `google/gemini-2.5-flash-lite`
  - BillFlow main จึงเปลี่ยน model หลักเป็น `google/gemini-2.5-flash-lite` และใช้ `google/gemini-2.5-flash` เป็น fallback สำหรับงานที่ต้องการความเสถียรกว่า
- SML party cache reliability:
  - ตอน backend start ระบบ retry ดึงรายชื่อลูกค้า/ผู้ขายจาก SML หลายรอบแบบ backoff แทนการ fail ครั้งเดียว
  - `/api/sml/parties/last-sync` และ party picker ส่ง/แสดง `status`, `last_attempt`, `last_sync`, `error` เพื่อให้ผู้ใช้รู้ว่าควรกดรีเฟรชหรือตรวจ SML API
  - ตรวจ production ล่าสุดแล้ว `status=ok`, ลูกค้า 1,004 รายการ, ผู้ขาย 500 รายการ

## Deployment

| Instance | Server folder | Frontend | Backend | PostgreSQL | Health |
|---|---|---:|---:|---:|---|
| BillFlow main | `/home/bosscatdog/billflow` | `3010` | `8090` | `5438` | ✅ |
| Thaisunsport | `/home/bosscatdog/billflow-thaisunsport` | `3020` | `8100` | `5448` | ✅ |
| Henna | `/home/bosscatdog/billflow-henna` | `3030` | `8110` | `5458` | ✅ |

The server folders are deployed copies, not git checkouts. Deploy/update commands should target the correct folder and must not assume `git status` works there.

Detailed instance registry: [deploy-instances.md](deploy-instances.md).

## Server `.env` Snapshot

Secrets are intentionally omitted.

| Key | Production value observed |
|---|---|
| `OPENROUTER_MODEL` | `google/gemini-2.5-flash-lite` |
| `OPENROUTER_FALLBACK_MODEL` | `google/gemini-2.5-flash` |
| `OPENROUTER_AUDIO_MODEL` | `openai/whisper-1` |
| `SML_BASE_URL` | `http://192.168.2.213:3248` |
| `SHOPEE_SML_URL` | `http://192.168.2.248:8080` |
| `SHOPEE_SML_GUID` | `smlx` |
| `SHOPEE_SML_PROVIDER` | `SMLGOH` |
| `SHOPEE_SML_CONFIG_FILE` | `SMLConfigSMLGOH.xml` |
| `SHOPEE_SML_DATABASE` | `SML1_2026` |
| `SHOPEE_SML_DOC_FORMAT` | `INV` |
| `SHIPPED_SML_DOC_FORMAT` | `PO` |
| `SHOPEE_SML_WH_CODE` / `SHELF_CODE` / `UNIT_CODE` | `WH-01` / `SH-01` / `ถุง` |
| `PUBLIC_BASE_URL` | Cloudflare Quick Tunnel URL, currently configured on server |
| `VITE_API_URL` | `http://192.168.2.109:8090` |
| `VITE_PHASE` | `1` on server frontend build config |

Docker Compose overrides backend `ENV=production`, so `/health` correctly reports production even though `.env` contains `ENV=development`.

## Current Product Behavior

| Area | Current behavior |
|---|---|
| LINE OA | Human inbox at `/messages`, multi-OA CRUD at `/settings/line-oa`, webhook supports `/webhook/line/:oaId` and legacy `/webhook/line`. Old chatbot/cart flow was removed in migration/session 13. |
| Admin chat reply | Uses cached LINE `replyToken` first when available, then falls back to Push API. `delivery_method` records `reply` or `push`. |
| Admin media reply | Uses signed `/public/media/:mediaID` URLs and requires `PUBLIC_BASE_URL` to be reachable by LINE servers. |
| Email | Multi-account IMAP configured in DB via `/settings/email`; no `IMAP_*` env singleton. One goroutine runs per enabled account. |
| Shopee Excel | `/api/import/shopee/preview` parses/dedups and `/api/import/shopee/confirm` creates local bills. SML send happens through bill Retry routing; default sale route is SML 248 `saleorder`, unless channel endpoint explicitly selects `saleinvoice` (`POST /SMLJavaRESTService/saleinvoice/v4`). |
| Shopee SKU handling | Source SKU from Excel is stored separately as `bill_items.source_sku`. It only becomes SML `item_code` when the same code exists in local SML Catalog; otherwise the row remains needs review. |
| Shopee shipped email | Routes to purchase bill and SML 248 `purchaseorder`. |
| Bill Retry | 4-way dispatch: `sale_reserve`, `saleorder`, `saleinvoice`, `purchaseorder`, selected by source/bill type plus `channel_defaults.endpoint`. Phase 1 purchase send uses the Bill Detail confirmation dialog for supplier, warehouse, shelf, VAT, document time, branch/sale code, and remark. |
| Bulk SML send | `/bills`, `/sales-orders`, and `/sale-invoices` have `ส่ง SML ทั้งหมด` for `pending` documents. It loads a preview, validates each bill, skips invalid rows, and sends ready bills one-by-one using shared dialog values. |
| หน้าเริ่มต้นใช้งาน | `/setup` checks required setup steps, shows shop/system counters, and provides an admin-only test-data reset dialog that preserves settings/catalog/mappings/AI usage by default. |
| Sidebar navigation | Sidebar groups document work by purchase/sale: `งานฝั่งซื้อ` and `งานฝั่งขาย`. Badges are per-document-route queue counts, not global pending count. |
| Bill detail | Shows route preview, blocks send when item validation fails, supports artifacts preview/download, stores optional `bills.remark`, and summarizes the latest SML request/response before raw JSON. |
| Logs | `/logs` shows action-specific summaries. Expanding a row shows key facts first (bill, doc_no, route, trace, error) and keeps raw JSON as a secondary technical view. |
| UX guardrails | Empty queues guide users to import/email setup, channel API details are collapsed, email sender mismatch is surfaced in Thai, and logs classify common SML failures into user-fixable vs support-needed actions. |
| Catalog | SML 248 catalog sync, CSV import, product create, per-row refresh/delete, embeddings, and in-memory cosine index. |
| SSE | `/api/admin/events` streams inbox/admin events with HMAC token from `/api/admin/events/token`. |
| Background jobs | Daily insight, daily backup, disk monitor, LINE token checker, hourly reply-token cleanup, daily Cloudflare tunnel drift monitor, IMAP coordinator. |

## Database Notes

Local migrations currently run through:

- `001_init.sql` through `026_document_route_and_source_sku.sql`
- Important recent additions: `bill_artifacts`, `chat_conversations.status`, CRM phone/notes/tags, cached reply token, per-OA mark-as-read toggle, `bills.remark`, `app_settings`, document route defaults, processed email keys, AI usage logs, Shopee import runs, `bills.document_route`, and `bill_items.source_sku`.

Production PostgreSQL also contains `system_settings` and `sml_settings`. These tables are not present in the current local migrations and are not referenced by the current codebase, so treat them as legacy leftovers until a future migration either formalizes or removes them.

## Current Document Menus

| Menu | URL | Backend filter | SML route |
|---|---|---|---|
| ใบสั่งซื้อ | `/bills` | source `shopee_shipped`, bill_type `purchase` | `purchaseorder` |
| ใบสั่งขาย | `/sales-orders` | source `shopee`, bill_type `sale`, document_route `saleorder` | `saleorder` |
| ขายสินค้าและบริการ | `/sale-invoices` | source `shopee`, bill_type `sale`, document_route `saleinvoice` | `saleinvoice` v4 |

## Phase 1 Purchase Flow

Phase 1 initially focused on Shopee purchase bills from email. The same review/send pattern is now also used by Shopee Excel sale documents.

1. IMAP account receives Shopee payment/confirmation email.
2. Email coordinator routes the message to `shopee_shipped`.
3. Backend extracts order reference, order date, items, quantities, prices, and source artifacts.
4. Bill is created as purchase bill and appears in `/bills`.
5. Admin reviews item rows, maps or creates SML products when needed.
6. Admin clicks send from Bill Detail.
7. Confirmation dialog requires supplier, warehouse, shelf, VAT type, and VAT rate. Branch code and sale code may be empty and are sent as empty strings.
8. Backend posts to SML REST:

```text
POST http://192.168.2.248:8080/SMLJavaRESTService/v3/api/purchaseorder
```

Required SML headers are read from production config:

| Header | Current value |
|---|---|
| `guid` | `smlx` |
| `provider` | `SMLGOH` |
| `configFileName` | `SMLConfigSMLGOH.xml` |
| `databaseName` | `SML1_2026` |
| `Content-Type` | `application/json; charset=utf-8` |

Purchase payload shape now follows SML v3 transaction attributes:

- Header includes `doc_no`, `doc_date`, `doc_time`, `doc_ref`, `doc_ref_date`, `doc_format_code`, `cust_code`, `supplier_name`, `branch_code`, `sale_code`, `wh_code`, `shelf_code`, `wh_from`, `location_from`, VAT totals, and `items`.
- `user_request` is sent as an empty string for Phase 1 purchaseorder.
- Item lines include `doc_ref`, `item_code`, `item_name`, `unit_code`, `qty`, `price`, `wh_code`, `shelf_code`, `wh_code_2`, `shelf_code_2`, VAT fields, and line totals.
- `branch_code` is included even when empty, because the confirmation dialog is the source of truth.

Known verified result: sending a Shopee purchase bill to SML `purchaseorder` succeeds and creates a document number such as `BF-PO26050002` / `BF-PO26050003`.
