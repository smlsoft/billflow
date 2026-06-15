# BillFlow — Current State

> Updated: 2026-06-15 +07
> Source of truth checked: local code/tests, frontend production build, Docker Compose deploy on `192.168.2.109`, thaisunsport DB/SML repair queries, container health checks, git tag `v1.0.0` (commit `ac06ac3`), and post-v1.0 marketplace purchase updates deployed to `billflow-thaisunsport`.

## Latest Handoff For New Chat

ถ้าเปิดแชทใหม่ ให้เริ่มจากสถานะนี้:

- BillFlow ปกติยังอยู่ที่ `http://192.168.2.109:3010` / backend `8090`.
- **Production release v1.0.0 deployed 2026-06-04** — git tag `v1.0.0`, commit `ac06ac3`.
- **Post-v1.0 marketplace purchase work deployed through 2026-06-15 on thaisunsport** — not tagged as `v1.0.0`; next feature release should be `v1.1.0` or later.
- Instances ที่ active: `billflow` (main) + `billflow-thaisunsport` เท่านั้น. `billflow-henna` → ย้ายเป็น **Nexflow** แล้ว (deploy จาก repo แยก).
- Migration ล่าสุดใน code/deployed main + thaisunsport: `063_lazada_charge_group_key.sql`.
- Lazada email purchase is live on thaisunsport with review-first flow; 3 Lazada IMAP accounts enabled, `lookback_days=1`, `poll_interval_seconds=600`.
- Marketplace purchase print readiness now depends on POL completeness plus BillFlow-only payment method rules, not creditor prefix.
- Single-bill and bulk SML send dialogs do **not** require `วิธีการชำระเงิน`; if supplier code/name starts with `TTxxxx`, BillFlow auto-syncs and locks the method to that `TTxxxx`.
- **TT payment auto-sync**: เมื่อเลือก supplier ที่ code หรือ name ขึ้นต้น TT จะ auto-fill วิธีการชำระเงินทันที (ตรวจทั้ง code และ name เพราะ SML อาจใช้ code AF00001 แต่ name = TT2789); ถ้า supplier ไม่ใช่ TT จะ clear ค่าและ user เลือกเองได้.
- **Lazada group key**: Lazada card doc_ref uses `raw_data.lazada_charge_group_key` parsed from the email body confirmation minute; legacy `email_date` fallback is blocked for send because envelope seconds can split one card charge.
- **sml-api-bybos**: เพิ่มและ deploy `PATCH /api/v1/ic/purchase-orders/:doc_no/doc-ref` + BillFlow `PATCH /api/bills/:id/sml-doc-ref` (admin) สำหรับแก้ doc_ref ย้อนหลัง
- sml-api-bybos ล่าสุด: เพิ่ม tenant `smlerpmaindata` (thaisunsport.thddns.net:9983) + endpoint `GET /api/v1/erp/sml-user-list` + `user_request` field ใน `ic_trans` INSERT.

## Latest Deploy 2026-06-15 — TT Auto-Sync + Lazada Group Key + SML Doc-Ref Patch

- Deploy targets: `billflow-thaisunsport` (`3020/8100`) + shared `sml-api-bybos` gateway (`8200`). `billflow` main was not redeployed in this round.
- Commit deployed to BillFlow thaisunsport: `691d01b fix: harden TT payments and Lazada doc ref grouping`.
- Earlier related commits: `a2b4927` (feat), `9341cc0` (fix Radix Select visual), `f329cac` (fix TT auto-sync root cause).
- **TT payment auto-sync** — `SendPurchaseDialog` + `BulkSendDialog`: เมื่อเลือก supplier ที่ code หรือ name ขึ้นต้น `TT` จะ auto-fill วิธีการชำระเงินทันที; ตรวจ name ด้วยเพราะ SML อาจ code = `AF00001`, name = `TT2789`
- **Lazada confirmed-at group key** — email ใหม่จะเก็บ `lazada_charge_group_key = "lazada_card_<accountID>_YYYYMMDD_HHMM"` ใน `raw_data`; SML send now requires this key for Lazada card purchases and blocks missing-key rows until backfill/repair runs.
- Migration `063_lazada_charge_group_key.sql` — CONCURRENTLY partial index on group key
- Backfill `cmd/backfill/lazada_group_key` (idempotent `--dry-run`) is packaged into the backend Docker image as `./lazada_group_key`; run inside the container so it can see `/app/artifacts` and `DATABASE_URL`.
- Production audit found Lazada HTML artifacts on thaisunsport; backfill must prefer the matching confirmation email artifact and use `raw_data.imap_account_id` when artifact `source_meta.account_id` is absent.
- **PATCH `/api/bills/:id/sml-doc-ref`** (admin) — proxy to sml-api-bybos + sync `sml_payload.doc_ref` and item/detail row `doc_ref` + audit.
- sml-api-bybos: `PATCH /api/v1/ic/purchase-orders/:doc_no/doc-ref` optimistic lock + `dry_run`; production gateway was rebuilt/recreated on 2026-06-15 because the route was missing in the previously deployed image.
- Pre-repair DB backup: `/home/bosscatdog/billflow-thaisunsport/backups/manual-backups/pre-tt-lazada-group-key-20260615-103944.sql`.
- Repair manifest: `/home/bosscatdog/billflow-thaisunsport/backups/manual-backups/lazada-doc-ref-repair-manifest-20260615-104241.csv`.
- Backfill result on thaisunsport: `updated=28`, `no_artifact=0`, `no_match=0`.
- Lazada target group `lazada_card_1e3ba826-f525-42b5-a5c0-79eafb318d79_20260611_1645` contains 8 active orders and totals `7417.69`.
- Historical SML doc_ref repair applied for 7 already-sent POL docs in that group: `POL26060022`, `POL26060023`, `POL26060024`, `POL26060025`, `POL26060026`, `POL26060027`, `POL26060028`.
- Post-repair verification: BillFlow local `sml_payload.doc_ref` and item row `doc_ref` are `7417.69` for the 7 sent bills; SML dry-run recheck returned `actual_old=7417.69`, `changed=false` for all 7.
- The remaining 1 order in that 8-order group is still `needs_review`; when sent later, current send logic should use the same group `doc_ref=7417.69`.
- Verification: `go test ./...` ✅, `npm run build` ✅, `sml-api-bybos go test ./...` ✅, health `8100`, `3020`, and `8200` ok.

## Latest Marketplace Purchase Rollout 2026-06-09

- Deploy targets: `billflow` main (`3010/8090`) and `billflow-thaisunsport` (`3020/8100`).
- No automatic source backup was taken in this round.
- Migrations deployed through `060_marketplace_print_perf_indexes.sql`.
- Local verification before deploy:
  - `npm run build`
  - `go test ./internal/repository ./internal/handlers`
- Post-deploy verification:
  - main backend `8090/health` ok; frontend `3010` HTTP 200
  - thaisunsport backend `8100/health` ok; frontend `3020` HTTP 200

### Customer-facing changes
- `/bills` list is more compact; row action noise moved into a secondary menu.
- `/api/bills` list enrichment batches print readiness by email group instead of per-row JSON scans.
- Single-bill send dialog and bulk send dialog include `วิธีการชำระเงิน` for Shopee/Lazada purchase email.
- Payment method is saved in `bills.print_payment_method` before SML send.
- Payment method is not sent to SML and is not part of `RetryBillPayload`.
- Backend allows saving payment method before POL exists, while still restricting the endpoint to Shopee/Lazada purchase email and configured method values.
- Print readiness uses configured policy in `/settings/channels`:
  - every order in the same email group has POL
  - effective payment method starts with `TT` by default
- Row print, detail print, and bulk print still call backend guards before recording a print event.

Runbook: [Marketplace Purchase Print And Payment Method](marketplace-purchase-print-and-payment.md).

## Latest Thaisunsport Lazada Email State 2026-06-15

### Current production data
- Initial manual poll/backfill created 7 `source='lazada_email'`, `bill_type='purchase'` bills and reconciled `ok`.
- Customer confirmed numbers were correct and one Lazada PO was sent to SML successfully.
- As of the latest DB check, active Lazada email purchase bills exist across `needs_review`, `pending`, and `sent` statuses as testing continues.
- Latest charge-group backfill updated 28 Lazada rows with `raw_data.lazada_charge_group_key`; no stored email artifacts were missing and no parse misses occurred.
- The 8-order group confirmed in email body at `2026-06-11 16:45` totals `7417.69`.
- Already-sent docs `POL26060022` through `POL26060028` in that group were patched in SML and BillFlow local payload to `doc_ref=7417.69`.
- Pre-repair backup: `/home/bosscatdog/billflow-thaisunsport/backups/manual-backups/pre-tt-lazada-group-key-20260615-103944.sql`.
- Repair manifest: `/home/bosscatdog/billflow-thaisunsport/backups/manual-backups/lazada-doc-ref-repair-manifest-20260615-104241.csv`.
- Lazada fee config is now set in `channel_defaults/lazada_email/purchase`:
  - `shipping_item_enabled=true`
  - `shipping_item_code=SHIP_CUS`
  - `shipping_item_unit_code=บาท`
- Lazada IMAP accounts:
  - `Lazada - pd.thaisunsport@gmail.com`
  - `Lazada - pd.thaisunsport2@gmail.com`
  - `Lazada - pd.thaisunsport03@gmail.com`
  - all enabled with `lookback_days=1`, `poll_interval_seconds=600`
- Fee line behavior:
  - Bill detail auto-adds `__lazada_shipping_fee__`/`SHIP_CUS` when a user opens a Lazada bill and config is ready.

### Production behavior
- Lazada email purchase uses deterministic HTML summary parsing for amounts; AI is not trusted for money totals.
- Raw data includes `goods_total_amount`, `shipping_amount`, `coupon_discount_amount`, `service_fee_amount`, `paid_total_amount`, `shipping_method`, `payment_method`, `lazada_charge_group_key`, `amount_reconciliation_status`, and `amount_reconciliation_delta`.
- `bill_items.discount_amount` stores Lazada coupon allocation, proportional by goods gross and excluding fee/shipping lines.
- Sending to SML is blocked if Lazada amount reconciliation is not `ok`.
- Sending to SML is blocked if a Lazada paid total needs shipping/fee but the fee item config is missing.
- Lazada card send is blocked if `lazada_charge_group_key` is missing; envelope `email_date` is not a safe fallback for card-charge grouping.
- Admin still reviews/maps items manually; no Lazada email bill is auto-sent to SML.

Runbook: [Lazada Email Purchase Intake](lazada-email-purchase.md).

## Production Release 2026-06-04 (v1.0.0)

### Features เพิ่มใหม่
- **SML user_request in PO**: admin ตั้ง SML user_code ให้แต่ละ BillFlow user ที่ `/settings/users` → ส่ง PO เข้า SML จะมี `user_request` = รหัสพนักงาน ดูใน `ic_trans.user_request` ได้เลย
- **AI visibility**: Dashboard card "AI ทำงานวันนี้" (บิลที่อ่าน/ความมั่นใจ/นาทีที่ประหยัด) + AI confidence badge สีต่างกันตาม % ใน bill list
- **DateRangePicker visual calendar**: คลิกเลือกวันที่ได้โดยไม่ต้องพิมพ์ YYYY-MM-DD พร้อมชื่อเดือนภาษาไทย ปีพุทธศักราช
- **Bills page layout redesign**: status + archive pills รวมแถวเดียว, bulk send button แยกแถวชัดเจน
- **Print: order_id ↔ doc_no pair**: กดพิมพ์ email artifact จะแสดง `POL... → คำสั่งซื้อ #...` ทุก order ใน email

### Bug fixes
- **doc_no collision (log_status: skipped)**: SML return skipped → ระบบ clear doc_no, fetch ใหม่, retry อัตโนมัติ 1 ครั้ง ก่อน mark failed
- **Shopee Coin discount allocation**: `shopee_coin_amount` ถูกรวมใน `discount_amount` ตอน re-apply แล้ว (เคย bug เฉพาะ existing bill update/backfill)
- **remark_2 in bulk send**: channel default ของ `remark_2` ถูก map เข้า BulkSendDialog payload แล้ว
- **Send button label**: ปุ่มส่งใน BillDetail แสดง route label จาก channel config แทน hardcode "(บิลซื้อ)"

### Production hardening
- **Global ErrorBoundary**: component crash จะเห็น "เกิดข้อผิดพลาด กรุณารีเฟรช" แทนหน้าขาว
- **Session timeout toast**: แสดง "Session หมดอายุ กรุณาเข้าสู่ระบบใหม่" 1.5 วินาทีก่อน redirect `/login`
- **404 catch-all**: URL ที่ไม่รู้จัก redirect ไป `/dashboard` แทน 404 หน้าขาว
- **Pilot → Production language**: Dashboard card ใช้คำ production แล้ว (ไม่มี "Pilot" ในหน้าใดๆ)
- **branch_code/sale_code visible**: ฟิลด์ในหน้าส่ง SML ไม่ซ่อนใน accordion แล้ว

### Deploy verification 2026-06-04
- `go build ./...` ✅
- `npx tsc --noEmit` ✅
- Backend health: main `8090` = ok, thaisunsport `8100` = ok
- Migration `056` applied on both instances
- sml-api-bybos `/api/v1/erp/sml-user-list` returns 18 users from `smlerpmaindata`
- AI today stats API: `ai_today_bills=57, avg_confidence=0.92, minutes_saved=228` (thaisunsport)

### Files changed (v1.0.0 commit)
- `backend/internal/handlers/bills.go` — skipped doc_no retry, user_request lookup, branch/sale_code always visible
- `backend/internal/repository/bill_repo.go` — ai_today stats query, shopee coin fix
- `backend/internal/repository/bill_shopee_summary.go` — `floatField()` helper
- `backend/internal/models/user.go` + `handlers/users.go` + `repository/user_repo.go` — SMLUserCode field
- `backend/internal/services/sml/purchaseorder_client.go` + `saleorder_client.go` + `saleinvoice_client.go` — IsSkipped(), UserRequest field
- `backend/internal/handlers/sml_party.go` + `main.go` — sml-user-list proxy, smlerpmaindata tenant override
- `backend/internal/database/migrations/056_user_sml_code.sql` — NEW
- `frontend/src/components/ErrorBoundary.tsx` — NEW
- `frontend/src/api/client.ts` — session expiry toast
- `frontend/src/App.tsx` — ErrorBoundary wrap, 404 catch-all
- `frontend/src/pages/Dashboard.tsx` — AITodayCard, pilot→production wording
- `frontend/src/components/BillTable.tsx` — AI confidence badge
- `frontend/src/components/common/DateRangePicker.tsx` — visual calendar
- `frontend/src/pages/Bills.tsx` — layout redesign
- `frontend/src/pages/BillDetail/hooks/useArtifacts.ts` — print order_id/doc_no pairs
- `frontend/src/pages/UserSettings.tsx` — SML user picker

---


- Latest deploy (ทั้ง 3 instances), 2026-05-27:
  - Commit deployed: `4e20c33 fix: align main ui readiness states`.
  - Scope: UI consistency audit fixes across Login/Setup/Dashboard/Logs/Bills/Shopee Settlement/Settings/Instance config.
  - `/setup` now treats IMAP `last_poll_status='no_new_mail'` as "ทดสอบสำเร็จ แต่ไม่มีอีเมลใหม่" instead of not-ready.
  - `/settings/email` shows instance context so admins know which shop/channel an inbox belongs to.
  - Shopee API disabled state no longer looks connected/usable; `/settings/instance`, `/import/shopee`, and `/shopee-settlements` show disabled/readiness warnings.
  - Sidebar and Command Palette now share one navigation source; menus such as `/shopee-settlements`, `/marketplace-aliases`, `/settings/channels`, `/settings/old-data`, `/settings/users`, and `/settings/instance` stay consistent.
  - Bills/Sale Invoice list wording now separates "เอกสารสถานะพร้อมส่ง" from dialog preflight "พร้อมส่งจริง".
  - `/settings/channels` makes Shopee settlement passbook/expense saved values clearer, and `/shopee-settlements` uses the same "ยังไม่ตั้งค่า" readiness language.
  - MAIN cleanup performed after deploy:
    - `PUBLIC_BASE_URL` and Shopee redirect updated to `https://edt-surfaces-graph-pension.trycloudflare.com`.
    - Thaisun inbox row in MAIN was disabled, not deleted: `Shopee Inbox THAISUNSPORT (ปิดใน MAIN)`.
    - Henna Shopee connection row in MAIN was disabled, not deleted: `Henna.milkford (ปิดใน MAIN)`.
    - Audit log action `main_config_cleanup` recorded with `reversible=true`.
  - Validation before deploy: `go test ./...`, `npm run build`, and `git diff --check` passed.
  - Deploy verification after restart:
    - Backend health: main `8090`, Henna `8110`, Thaisun `8100` all returned `{"database":"ok","env":"production","status":"ok"}`.
    - Frontend route smoke: main `/settings/channels`, main `/settings/instance`, Henna `/shopee-settlements`, Thaisun `/bills` all HTTP 200.
    - Public URL smoke: main, Henna, and Thaisun login pages all HTTP 200.
    - Built frontend bundles contain current patch markers such as `เอกสารสถานะพร้อมส่ง`, `พร้อมส่งจริง`, and `Shopee API ปิดใช้งาน`.
  - Current public URLs:
    - main: `https://edt-surfaces-graph-pension.trycloudflare.com`
    - henna: `https://tops-twins-oasis-philip.trycloudflare.com` (active quick tunnel) and `https://animal-galvanize-tameness.ngrok-free.dev` (configured `PUBLIC_BASE_URL`/Shopee redirect)
    - thaisunsport: `https://pets-mini-museums-ships.trycloudflare.com`
- Latest frontend deploy (ทั้ง 3 instances), 2026-05-25:
  - **Email preview dialog (ArtifactList)** — แก้ช่องว่างด้านบนใน dialog ดูตัวอย่างอีเมล:
    - ใช้ React `createPortal` mount modal ตรงที่ `document.body` แทน render ใน DOM tree — หลุดออกจาก `space-y-4` parent ที่ inject `margin-top` ผ่าน Tailwind sibling selector
    - inject CSS reset ใน iframe HTML: `html,body { margin:0; padding:0 }` + `img { display:block }` เพื่อตัด tracking pixel baseline gap
    - links ใน email เปิด new tab ได้ผ่าน `allow-popups-to-escape-sandbox` sandbox attribute
  - deploy verified: main `3010` ✅, henna `3030` ✅, thaisunsport `3020` ✅
  - Tunnel URLs at that time (superseded by 2026-05-27 snapshot above):
    - main: `https://edt-surfaces-graph-pension.trycloudflare.com`
    - thaisunsport: `https://pets-mini-museums-ships.trycloudflare.com`
    - henna: `https://animal-galvanize-tameness.ngrok-free.dev`
- Latest frontend deploy (ทั้ง 3 instances), 2026-05-22 end of day:
  - `/settings/channels` EditDialog — doc_format_code dropdown ดึงจาก `GET /api/sml/doc-formats` (sml-api-bybos)
  - รหัสขึ้นต้น (prefix) = `code` ของ doc_format ที่เลือก; รูปแบบเลขรัน = `format` field ตัด `@` นำหน้าออก
  - ทั้ง 2 field แสดงแบบ read-only — ดึงจาก SML อัตโนมัติ ไม่ให้ admin กรอกเอง
  - `doc_format_code` ใน channel_defaults ไหลผ่าน bills.go → SML client → sml-api-bybos payload ครบ loop แล้ว ไม่มี hardcode
  - deploy verified: main `3010` ✅, henna `3030` ✅, thaisunsport `3020` ✅
  - sml-api-bybos เพิ่ม endpoint `GET /api/v1/ic/doc-formats?screen_code=PO|SI|SR` + แก้ `calc_flag` ให้คำนวณจาก transType (ขาย=-1, ซื้อ=1) ไม่ hardcode
  - docs อัปเดตแล้ว: `sml-api-migration.md`, `deploy-instances.md` (เพิ่มส่วน sml-api-bybos multi-tenant)
- Latest deploy parity checkpoint, 2026-05-22:
  - Main (`3010/8090`) และ Henna (`3030/8110`) ถูก deploy จาก local source snapshot เดียวกันแล้ว; `billflow-thaisunsport` intentionally ไม่ได้ restart ในรอบนี้.
  - Health/smoke ผ่านทั้ง 2 instance: backend `/health`, frontend `/login`, login API, `/api/bills`, `/api/settings/channel-defaults`.
  - `scripts/preflight-main.sh` ผ่านสำหรับ main ค่า default และผ่านสำหรับ Henna ด้วย override `BF_HOST=192.168.2.109 BACKEND_PORT=8110 FRONTEND_PORT=3030 SML_API_PORT=8200 SML_TENANT=aoy`.
  - Schema features ล่าสุดตรงกันทั้ง main + Henna: `channel_defaults.shipping_item_enabled|shipping_item_code|shipping_item_unit_code` และ `bill_items.discount_amount`.
  - Runtime SML REST ของทั้ง main + Henna ใช้ `sml.rest_base_url=http://172.24.0.1:8200` และ channel sale routes ใช้ `/api/v1/ic/sale-invoices`.
- BillFlow main latest code checkpoint, 2026-05-21:
  - Latest functional change: Bill Detail now treats low-confidence suggested matches as unconfirmed until `mapped=true`; Send SML and Bulk Send block these rows, and each row has a quick confirm button that reuses the existing F1 learning/update path.
  - Latest validation before this docs update: `go test ./...`, `npm run build`, `git diff --check`, and real-data Shopee API discovery for Henna.milkford passed locally; deploy/preflight/smoke should be rerun after any follow-up patch.
- BillFlow main Shopee API runtime checkpoint, originally deployed 2026-05-21:
  - Shopee Open API readiness is deployed on `/import/shopee`: status checklist, OAuth URL generation, callback/token tables, preview-only API import, structured error UX, and live-cutover script/docs.
  - As of 2026-05-27 cleanup, BillFlow main has `SHOPEE_OPEN_API_ENABLED=false`; the stale Henna shop connection in MAIN is disabled rather than deleted, so MAIN no longer presents Henna as ready-to-use.
  - MAIN `PUBLIC_BASE_URL` and Shopee redirect now point to `https://edt-surfaces-graph-pension.trycloudflare.com`.
  - Henna is the active Shopee API/settlement test instance (`shop_id=264993963`, `Henna.milkford`).
  - Real Shopee API discovery for Henna.milkford over `2026-05-07` to `2026-05-21`: `create_time=28` orders, `update_time=38` orders. Shopee rejects `pay_time` for `get_order_list`; BillFlow now rejects it with a readable error.
  - API preview defaults to ready-to-bill statuses (`SHIPPED`, `TO_CONFIRM_RECEIVE`, `COMPLETED`), shows shipping/package/carrier/COD, includes shipping in mismatch detection, and blocks confirm when Shopee reports more pages.
  - Shopee Excel/email stays the fallback while the direct API path is tested with real date windows.
  - `/settings/instance` now shows runtime/default/env-aware values, lets admin test draft SML REST URL + tenant before saving, and reminds that product images use the matching `{database}_images` DB.
  - Email polling UX and backend state are hardened with poll summaries/details and indexes for message-id/dedup paths.
  - `/bills`, `/sales-orders`, and `/sale-invoices` now use DB-backed async SML bulk jobs instead of a single long frontend request. The dialog shows progress, can be closed/reopened, and supports retrying only failed rows.
  - `/bulk-send-jobs` now provides a read-only history page for admin/staff with status/route filters, progress, actor email, per-bill result dialog, and links back to source bills.
  - Bulk send runbook: [sml-bulk-send-jobs.md](sml-bulk-send-jobs.md).
  - Live SML smoke test passed with one Shopee shipped purchase bill: bill `20275aed-fe5f-402f-9160-a93a3f5b2ccb` created SML purchaseorder `BF-PO26050001` through bulk job `128ceffe-5055-4863-8944-c6ce52301d26`; sent `1`, failed `0`, skipped `0`.
  - Latest API smoke: `/api/bills/bulk-send-jobs?page=1&per_page=20` returns the completed job; invalid `status=bad` returns HTTP 400; detail endpoint returns `item_count=1`.
- SML product images on BillFlow main are now lazy-loaded through `sml-api-bybos`; BillFlow keeps only image metadata in `sml_catalog`.
  - Active tenant: `SML1_2026`; image DB: `sml1_2026_images`.
  - Index/runbook for moving SML PostgreSQL or changing tenant: [sml-image-db-maintenance.md](sml-image-db-maintenance.md).
  - Script to re-apply the expression index: [../scripts/apply-sml-image-index.sh](../scripts/apply-sml-image-index.sh).
  - Current image index `images_trim_image_id_order_roworder_file_idx` was verified on `sml1_2026_images`; `scripts/apply-sml-image-index.sh` supports host `psql` and Docker-container `psql`.
  - `/settings/catalog` and the bill item SML picker show fixed-size thumbnails, lazy-loaded images, fallback states, and full image preview/gallery.
- BillFlow main production data lifecycle deployed 2026-05-18:
  - Commit `ba2145f` adds migration `037_data_lifecycle.sql`, summary tables, indexes, cursor pagination, bill archive/restore/delete routes, and the daily lifecycle job.
  - `/api/logs` now supports `limit`, `cursor`, `has_more`, `next_cursor`; it does not run `COUNT(*)` unless `include_total=true`.
  - `/api/bills` supports `archived`, `date_from`, `date_to`, cursor pagination, and legacy `page/per_page`; default lists hide archived rows.
  - `/api/bills/counts` returns queue counts in one request for the `/bills`, `/sales-orders`, and `/sale-invoices` list pages.
  - SML audit logs no longer duplicate full `sml_payload` / `sml_response` into every audit row; they keep compact support fields (`doc_no`, route, error code, message, request/trace id).
  - `/settings/old-data` shows row counts, table sizes, oldest rows, retention policy, and dry-run purge summaries. Purge is manual, batch-safe, and no purge option is selected by default.
  - Verified on main after deploy: `go test ./...`, `npm run build`, `scripts/preflight-main.sh`, smoke checks for `/api/logs`, `/api/bills`, `/api/bills/counts`, and `/api/bills/old-data/summary`.
  - Residual risk: current production workload passed, but the planned 1M audit log / 100k bill performance seed test is still recommended before claiming full one-year high-volume proof.
- Thaisunsport แยก instance อยู่ที่ frontend `3020`, backend `8100`, postgres `5448`.
  - ล่าสุด deploy แล้วเมื่อ 2026-05-27 จาก codebase ปัจจุบัน แต่ยังคง feature flags Phase 1 ฝั่งซื้อเท่านั้น.
  - Public URL: `https://pets-mini-museums-ships.trycloudflare.com/login`
  - Health checked: backend `8100` = `ok`, frontend `3020` serve HTML ได้, containers up.
  - Frontend build flags: `VITE_PHASE=1`, `VITE_ENABLE_SALES_ORDERS=false`, `VITE_ENABLE_SHOPEE_EXCEL=false`, `VITE_ENABLE_LAZADA_EXCEL=false`, `VITE_ENABLE_TIKTOK_EXCEL=false`, `VITE_ENABLE_CHAT=false`.
  - AI model ใน `.env`: `OPENROUTER_MODEL=google/gemini-2.5-flash-lite`, fallback `google/gemini-2.5-flash`.
  - สำหรับ demo Thaisunsport ตอนนี้อย่าเปิด Phase 1+ / Shopee Excel / ใบสั่งขาย จนกว่าลูกค้าจะผ่าน demo ฝั่งซื้อ.
- Henna customer trial ถูกสร้างใหม่จาก BillFlow ปกติ ไม่ใช่ Thaisunsport:
  - Public URL: `https://tops-twins-oasis-philip.trycloudflare.com/login` (active quick tunnel); `https://animal-galvanize-tameness.ngrok-free.dev/login` also works and remains configured for `PUBLIC_BASE_URL`/Shopee redirect.
  - Frontend `3030`, backend `8110`, postgres `5440`
  - Server folder `/home/bosscatdog/billflow-henna`
  - Containers `billflow-henna-frontend`, `billflow-henna-backend`, `billflow-henna-postgres`
  - Latest parity deploy 2026-05-27 13:35 +07:
    - Sync backend/frontend จาก local source เดียวกับ BillFlow main แล้ว build/restart Henna services.
    - Backend health `8110` = `{"database":"ok","env":"production","status":"ok"}` และ frontend `/shopee-settlements` HTTP 200.
    - Feature flags remain Phase 1+ for purchase + sales + marketplace/Shopee settlement, with chat disabled.
    - Shopee API stays enabled for Henna (`SHOPEE_OPEN_API_ENABLED=true`).
  - Latest QA fix deploy 2026-05-12 13:05 +07:
    - Henna frontend rebuilt as Phase 1+ (`VITE_PHASE=99`, sales/Shopee/Lazada/TikTok Excel enabled) but chat is explicitly disabled with `VITE_ENABLE_CHAT=false`.
    - Policy clarification: Henna Phase 1+ means purchase + sales + marketplace Excel, not LINE/chat features.
    - Backend health `8110` = `{"database":"ok","env":"production","status":"ok"}`.
    - `shopee/sale` SML defaults updated to current SML cache: `wh_code=AB-2`, `shelf_code=002`, `vat_type=0`, `vat_rate=7`, `doc_time=09:00`.
    - Auto saleinvoice send retest succeeded without passing WH/Shelf/VAT/time in request; SML doc `BF-INV26050008`.
    - `doc_counters` for `BF-INV/202605` = `8`; local duplicate `BF-INV2605%` count = `0`.
    - `/api/logs` now admin/staff only; viewer gets 403.
    - `/api/settings/users` admin-only CRUD added and retested with temporary QA user.
    - `/api/settings/imap-accounts` returns `data: []` when empty.
    - Hidden-character `doc_no` guardrail now returns HTTP 400 with the clean suggested number.
    - Full report: [qa-full-system-audit-2026-05-12.md](qa-full-system-audit-2026-05-12.md).
- Instance/port registry อยู่ที่ [deploy-instances.md](deploy-instances.md). ใช้ไฟล์นี้เป็น source of truth เมื่อต้องจำ port/tunnel ของแต่ละร้าน.
- Local Codex skill `buddhist-method` ติดตั้งและอัปเดตแล้วที่ `/Users/nontawatwongnuk/.codex/skills/buddhist-method`; skill ใช้งานได้ใน session นี้และ session ใหม่ควรเห็นจาก skill list.
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
  - `ใบสั่งซื้อ`: source `shopee_shipped` และ `lazada_email`, bill_type `purchase`
  - `ใบสั่งขาย`: bill_type `sale`, document_route `saleorder` รวม Shopee/Lazada/TikTok Marketplace Excel
  - `ขายสินค้าและบริการ`: bill_type `sale`, document_route `saleinvoice` รวม Shopee/Lazada/TikTok Marketplace Excel
  - นับเฉพาะ `pending + needs_review + failed`
- ทั้ง 3 หน้าเอกสารมีปุ่ม `ส่ง SML ทั้งหมด` สำหรับสถานะ `pending` พร้อม bulk preview/validation ก่อนส่งจริง.
  - Dialog ล่าสุดจัดรายการเอกสารเป็นตารางอ่านง่ายขึ้น แสดงลำดับส่ง, order no, สถานะ, และ `doc_no` ที่คาดว่าจะได้ต่อแถว.
  - เมื่อ backend preview คืนเลขเริ่มต้นเดียวกันหลายบิล frontend จะคำนวณเลขคาดการณ์ตามลำดับส่ง เช่น `...001`, `...002`, `...003`; backend ยังเป็นผู้จองเลขจริงตอนกดส่ง.
  - เพิ่ม `สรุปก่อนส่ง` ให้ตรวจลูกค้า/ผู้ขาย, ปลายทาง, ช่วง doc_no, คลัง/พื้นที่เก็บ, VAT, เวลาเอกสาร ก่อนกดส่ง.
  - Bulk send now creates a server-side job (`sml_bulk_jobs` + `sml_bulk_job_items`) and polls progress every second; closing/reopening the dialog resumes the active or recent job.
  - Worker sends serially with concurrency `1`, revalidates each bill before send, skips bills that are no longer sendable, suppresses per-bill LINE spam during bulk, and writes audit details with `via=bulk_job`.
  - หลัง bulk send จบ dialog แสดงผลสำเร็จ/ไม่สำเร็จ/ข้าม พร้อมปุ่มคัดลอก error summary, retry failed, และลิงก์ไปบิลแรกที่ส่งไม่สำเร็จ.
- Shopee Excel import ล่าสุดรองรับปลายทาง SML ทั้ง `saleorder` และ `saleinvoice`; เมื่อ channel default เปลี่ยน endpoint เมนูและข้อความจะเปลี่ยนตาม.
- Shopee Excel status filter ล่าสุดนำเข้าแถวสถานะ `ที่ต้องจัดส่ง` แล้ว; filter ออกเฉพาะ `ยกเลิกแล้ว`.
- Lazada Excel import deployed on BillFlow main + Henna:
  - เพิ่ม `/import/lazada`, `/api/settings/lazada-config`, `/api/import/lazada/preview`, `/api/import/lazada/confirm`, และ `/api/import/lazada/runs`
  - parser อ่านไฟล์ Lazada export จาก `lazada.xlsx`, group ตาม `orderNumber`, ใช้ `paidPrice`, รวมรายการซ้ำเป็น qty, ใช้ `createTime` เป็น `doc_date`, และกรองเฉพาะ `confirmed`, `shipped`, `delivered`
  - สร้าง local bills เป็น `source='lazada'`, `bill_type='sale'`, `document_route='saleorder'` หรือ `saleinvoice` ตาม `/settings/channels`
  - sales queue/badge/setup counts ปรับให้รวม Shopee + Lazada แล้ว
  - deploy แล้วเฉพาะ BillFlow main + BillFlow Henna; Thaisunsport ถูก skip เพราะยังเป็น Phase 1 ฝั่งซื้อ
- TikTok Excel/CSV import deployed on BillFlow main + Henna:
  - เพิ่ม `/import/tiktok`, `/api/settings/tiktok-config`, `/api/import/tiktok/preview`, `/api/import/tiktok/confirm`, และ `/api/import/tiktok/runs`
  - parser รองรับ `.csv` และ `.xlsx` ที่มี header TikTok Seller Center; ไฟล์ตัวอย่างจริงที่ใช้ตรวจคือ `tiktok_csv.csv`
  - ไฟล์ `tiktok_excel.xlsx` ที่ให้มาเป็น workbook ว่าง/template มีเฉพาะ header `Order ID`; parser จะรองรับ XLSX จริงเมื่อ export มี columns ครบ
  - group ตาม `Order ID`, ใช้ `SKU ID` เป็น source SKU fallback เพราะตัวอย่างไม่มี `Seller SKU`, ใช้ `SKU Subtotal After Discount / Quantity` เป็นราคาต่อหน่วย และใช้ `Order Amount` แบบไม่บวกซ้ำเมื่อ order มีหลายแถว
  - กรองเฉพาะสถานะ `จัดส่งแล้ว`/`shipped`/`delivered`; ข้าม `ยกเลิกแล้ว` และ `ที่จะจัดส่ง`
  - สร้าง local bills เป็น `source='tiktok'`, `bill_type='sale'`, `document_route='saleorder'` หรือ `saleinvoice` ตาม `/settings/channels`
  - deploy แล้วเฉพาะ BillFlow main + BillFlow Henna; Thaisunsport ถูก skip เพราะยังเป็น Phase 1 ฝั่งซื้อ
  - Verified after deploy: backend health `8090`, `8110`; frontend routes `/import/tiktok` return HTTP 200 on ports `3010`, `3030`; migration `030_tiktok_import.sql` applied on both
- `ขายสินค้าและบริการ` (`saleinvoice`) ใช้ endpoint ใหม่ `POST /SMLJavaRESTService/saleinvoice/v4`; เปิดใช้งานบน BillFlow main และ BillFlow Henna ส่วน Thaisunsport ยังปิดด้วย Phase 1 feature flags.
- Shopee SKU ถูกเก็บแยกเป็น `bill_items.source_sku`; ถ้า SKU ไม่มีใน SML Catalog จะไม่เอาไปใส่เป็น `item_code`.
- REST SML retry ตรวจซ้ำว่า `item_code` มีอยู่ใน Catalog จริงก่อนส่ง.
- Saleinvoice test ล่าสุด: `BF-INV26050001` ส่ง payload มี `doc_ref_date: "2026-03-10"` แล้ว ถ้า SML UI ไม่แสดงให้ dev SML ตรวจ API mapping.
- UX hardening ล่าสุดสำหรับ Phase 1+:
  - Dashboard เพิ่ม `Action Center` จัดลำดับงานถัดไปให้ user: setup ไม่ครบ, email มีปัญหา, SML fail, mapping ค้าง, และเอกสารพร้อมส่ง
  - หน้า Dashboard และหน้าเอกสารแสดง empty state พร้อมปุ่มไปงานถัดไปเมื่อยังไม่มีบิลหลังล้างข้อมูลทดสอบ
  - `/settings/channels` ซ่อน API path เป็น `รายละเอียดขั้นสูง` เพื่อไม่ให้พนักงานทั่วไปสับสน
  - dialog ส่ง SML ทั้งแบบรายใบและแบบส่งทั้งหมดแสดง field ที่ยังขาด เช่น ลูกค้า/ผู้ขาย, คลัง, พื้นที่เก็บ, ภาษี, เวลาเอกสาร ก่อนกดส่ง
  - bulk send แสดงข้อความเตือนเมื่อมีเอกสารพร้อมส่งเกิน 100 รายการ และแสดง error จาก backend/SML ในแถวที่ส่งไม่สำเร็จ
  - `/settings/email` แสดง `ผู้ส่ง Shopee ที่ยอมรับ` ในตาราง และ backend จะบันทึกคำเตือนภาษาไทยเมื่ออีเมลถูกข้ามเพราะผู้ส่งไม่ตรง
  - `/logs` แสดงคำแนะนำว่า error นั้นผู้ใช้แก้เองได้หรือควรส่งให้ทีมดูแลระบบ/SML API
  - `/logs` error playbook แยกสาเหตุละเอียดขึ้น เช่น SML timeout/network, doc format, ลูกค้า/ผู้ขาย, VAT, คลัง/พื้นที่เก็บ, สินค้า/หน่วย, Gmail App Password, และ AI quota
  - `/logs` ล่าสุดปรับเป็น Action View: มี summary cards, quick filters, DEV mode, import grouping, incident card สำหรับ SML fail, และ data-quality warning เมื่อ `doc_no` มีอักขระซ่อน/ผิดรูปแบบ
  - `/logs` ล่าสุดแสดงผู้ทำรายการจริงจาก backend แล้ว: user action จะเห็นชื่อ/อีเมล/role, background job จะแสดงเป็น Email worker/System, และ filter `ผู้ทำรายการ` ส่งค่า `user_id` เข้า `/api/logs`
  - audit log สำคัญตอนส่ง SML สำเร็จ/ล้มเหลว, เพิ่ม/ลบรายการสินค้า, และยืนยัน mapping จะบันทึก `user_id` ของผู้กดใช้งาน ทำให้ตรวจสอบย้อนหลังได้ว่าใครทำอะไร
  - backend เพิ่ม guardrail ก่อนส่ง SML: ถ้า `doc_no` มี hidden character หรือ Thai mark แปลก ๆ เช่นเลขเอกสารขึ้นต้นผิดเป็น `ฺBF-...` ระบบจะหยุดก่อนยิง SML และแนะนำเลขที่สะอาดแทน เพื่อลด duplicate/error จาก SML API
  - guardrail ล่าสุดคืน HTTP 400 พร้อมข้อความ user-correctable ไม่ใช่ 500 แล้ว
  - `/logs` แสดง `demo_test_data_reset` เป็น `ล้างข้อมูลทดสอบ` พร้อม badge `Setup`, summary ภาษาไทย, และคำอธิบายว่าเป็นการล้างข้อมูลทดสอบจากหน้า `/setup` ไม่ใช่ error
  - `/api/bills` รองรับทั้ง `per_page` และ `page_size`, และคืน `data: []` แทน `null` เมื่อไม่มีข้อมูล
- Email accepted-sender update:
  - ช่องเดิม `shopee_domains` ใน DB ยังใช้ชื่อเดิมเพื่อเลี่ยง migration เสี่ยง แต่ UI แสดงเป็น `ผู้ส่งที่ยอมรับ`
  - ใส่ได้ทั้งโดเมนและอีเมลเต็ม เช่น `shopee.co.th`, `mail.shopee.co.th`, `billing@example.com`
  - เว้นว่าง = รับทุกผู้ส่งที่ผ่านคำกรองหัวข้อ
  - backend ลด warning ซ้ำจากการ poll อีเมล เพื่อไม่ให้ `/settings/email` แสดง error ยาวเกินจำเป็น
- Gmail IMAP onboarding update:
  - `/settings/email` แสดง checklist 3 ขั้นตอน: เปิด 2-Step Verification, สร้าง Google App Password, เปิด Gmail IMAP
  - มีลิงก์ตรงไป Google Security, App Passwords, และ Gmail POP/IMAP settings
  - ช่องรหัสใน dialog แสดงเป็น `App Password จาก Google` สำหรับ Gmail และอธิบายว่าไม่ใช่รหัสผ่าน Gmail ปกติ
  - Frontend และ backend normalize Gmail App Password โดยลบช่องว่าง/ขีดกลางก่อน test/save/list folders เช่น `qzqq vwqb zydo dtsi` → `qzqqvwqbzydodtsi`
  - ถ้า Gmail App Password หลัง normalize ไม่ครบ 16 ตัว ระบบเตือนก่อนบันทึก/ทดสอบ และ backend reject เพื่อกัน user error
  - `AUTHENTICATIONFAILED` ถูกแปลเป็นคำแนะนำว่าต้องตรวจ App Password, 2-Step Verification, และ IMAP
- Bills email-date display update:
  - อีเมลใหม่ที่ poll หลัง deploy จะเก็บวันที่จาก IMAP envelope header เป็น `raw_data.email_date`
  - ตาราง `/bills` แสดงวันที่อีเมลพร้อม prefix `อีเมล` เมื่อมี `email_date`
  - บิลเก่าที่ไม่มี `email_date` ยัง fallback ไปใช้ `created_at` หรือวันที่เข้าระบบ เพื่อไม่ให้รายการว่างหรือเสีย layout
- IMAP read/unread polling update:
  - IMAP poll ดึงทั้งอีเมลที่ยังไม่อ่านและอ่านแล้วภายใน `lookback_days` ไม่กรองเฉพาะ unread อีกต่อไป
  - ยังใช้ `processed_email_keys` และ bill-level dedup เพื่อกันสร้างบิลซ้ำจากอีเมลเดิมหรือ order เดิม
  - หลัง deploy main poll พบเมล `Fwd: คำสั่งซื้อ #260404V08VQU10 ถูกจัดส่งแล้ว` แล้ว แต่ข้ามเพราะมี dedup tombstone ตั้งแต่ 2026-05-08; หากต้องการทดลองดึงซ้ำหลังล้างข้อมูลทดสอบ ต้องเลือกล้างประวัติอีเมลที่เคยอ่านแล้ว หรือเคลียร์ dedup เฉพาะรายการอย่างตั้งใจ
- IMAP poll-status clarity update:
  - `imap_accounts` มี runtime fields เพิ่ม: `last_poll_found`, `last_poll_processed`, `last_poll_skipped`
  - `/settings/email` เปลี่ยนคอลัมน์จาก `สร้างบิล` เป็น `ผลรอบล่าสุด` และแสดง `พบ / ประมวลผล / ข้าม`
  - `last_poll_messages` ยังเก็บไว้เพื่อ compatibility แต่ความหมายจริงคือจำนวนที่ processor รับไปทำงาน ไม่ใช่จำนวนบิลที่สร้างเสมอ
  - เพิ่ม `last_poll_details` ผ่าน migration `031_imap_poll_details.sql`; UI กดดูรายละเอียดแต่ละเมลได้ว่า subject/from/date คืออะไร, ประมวลผลหรือข้าม, และข้ามเพราะอะไร เช่น เคยประมวลผลแล้ว, ผู้ส่งไม่อยู่ในรายชื่อที่ยอมรับ, หัวข้อไม่ตรงคำกรอง, ไม่พบไฟล์แนบ, หรืออ่านแล้วไม่พบรายการสินค้า
  - Deploy แล้วทั้ง `billflow`, `billflow-henna`, `billflow-thaisunsport`; verified health `8090`, `8110`, `8100` และ frontend `/settings/email` HTTP 200 ทั้ง `3010`, `3030`, `3020`
  - แก้ migration เก่า `002_sml_catalog.sql` และ `004_shopee_shipped.sql` ให้รวม `tiktok` ใน `bills_source_check` ด้วย เพื่อให้ idempotent เมื่อ re-run หลังมีข้อมูล TikTok แล้ว
  - บน BillFlow main เคลียร์ `processed_email_keys` เฉพาะ message/order `260404V08VQU10` แล้ว poll ใหม่สร้างบิล `67c0be5b-9247-4945-9dc0-85ad498243cf` สถานะ `pending`, source `shopee_shipped`, order `#260404V08VQU10`
- Bill Detail item mapping UI update:
  - ในหน้า detail ของบิล เมื่อกดแก้ไขรายการสินค้าแล้วเลือกสินค้าใหม่จาก SML ระบบแสดงรหัส/ชื่อ/คะแนนของสินค้าที่เลือกทันทีในแถวแก้ไข
  - หลังบันทึก ตารางรายการสินค้าจะอัปเดตชื่อสินค้า SML จากตัวเลือกใหม่ทันที ไม่ต้อง refresh หน้า
  - ใช้ร่วมกันกับ purchase, saleorder, และ saleinvoice เพราะเป็น component รายการสินค้าเดียวกัน
  - Deploy แล้วทั้ง `billflow`, `billflow-henna`, `billflow-thaisunsport`; verified health `8090`, `8110`, `8100` และ frontend `/bills` HTTP 200 ทั้ง `3010`, `3030`, `3020`
- Verified Mapping Loop update:
  - เมื่อ user กดบันทึกรายการสินค้า ระบบจะถือเป็นการยืนยัน mapping แม้ `item_code` เป็น code เดิมที่ AI เติมไว้แล้ว แต่แถวนั้นยัง `mapped=false` หรือยังไม่มี `mapping_id`
  - หลังเรียน mapping แล้ว backend จะ apply mapping ไปยังบิลค้าง source/bill_type เดียวกันที่มี `raw_name` ตรงกัน และเลื่อนบิลจาก `needs_review` เป็น `pending` เมื่อทุก item mapped แล้ว
  - แก้ปัญหา Shopee/Lazada/TikTok ที่ชื่อ marketplace ไม่ตรงกับ SML และไฟล์ไม่มี SKU: user ควรจับคู่ครั้งเดียวต่อชื่อสินค้า/ตัวเลือก ไม่ต้องไล่เลือกซ้ำทุกบิล
  - Deploy แล้วทั้ง `billflow`, `billflow-henna`, `billflow-thaisunsport`; verified health `8090`, `8110`, `8100`
  - Henna production data follow-up: applied existing confirmed Shopee mapping to 3 open item rows; promoted 3 saleinvoice bills to `pending`; remaining `needs_review` = 3 raw names/options that still need their first user confirmation
- Bulk Send dialog UX update:
  - Dialog `ส่ง SML ทั้งหมด` ในหน้า `/bills`, `/sales-orders`, และ `/sale-invoices` ปรับ layout ให้สอดคล้องกับ dialog ส่ง SML ใน Bill Detail
  - ใช้กล่องปลายทาง SML, party picker, กล่องตั้งค่าเอกสาร, helper text, advanced Branch/Sale code, หมายเหตุ, และ warning field ที่ขาดในรูปแบบเดียวกัน
  - แสดง `doc_no` preview ต่อบิลในรายการพร้อมส่ง พร้อมเลข order ต้นทาง เพื่อให้ user ตรวจเลขเอกสารก่อนกดส่ง SML ทั้งหมด
  - ส่วนตรวจรายการพร้อมส่งยังคงอยู่เฉพาะ bulk dialog เพราะต้องแสดงจำนวนรายการที่พร้อมส่ง/ต้องข้าม/ผลส่งต่อบิล
  - Deploy แล้วทั้ง `billflow`, `billflow-henna`, `billflow-thaisunsport`; verified health `8090`, `8110`, `8100`; verified frontend routes main/Henna `/sale-invoices` และ Thaisunsport `/bills`; Thaisunsport flags ยังเป็น Phase 1
- Shopee product image update:
  - Email extractor ตัด tracking/open pixel URL ของ Shopee ออก และจัดลำดับให้ product CDN เช่น `cf.shopee.co.th/file/th-*` มาก่อน logo/app/social assets
  - เพิ่ม backend test สำหรับเคสที่อีเมลมี tracking pixel ก่อน product image
  - บิล `67c0be5b-9247-4945-9dc0-85ad498243cf` item `ef15ccb5-c164-476e-84bb-3ef6657e531d` ถูกอัปเดตให้ใช้รูป `https://cf.shopee.co.th/file/th-11134207-81zth-mimxd9980lc477`; verified `HTTP 200 image/jpeg`
- SML API line-item update:
  - ทุก payload รายการสินค้าที่ส่งเข้า SML hardcode `is_get_price: 1` ที่ระดับ line item แล้ว
  - ครอบคลุม `sale_reserve` (SML 213 JSON-RPC), `saleorder`, `saleinvoice`, และ `purchaseorder`
  - เพิ่ม backend test `TestSMLLinePayloadsHardcodeIsGetPrice` เพื่อยืนยัน JSON key `is_get_price` มีค่า `1` ในทุก route
- AI model update:
  - production log ยืนยันว่า `google/gemma-4-26b-a4b-it:free` fail แล้ว fallback ไป `google/gemini-2.5-flash-lite`
  - BillFlow main จึงเปลี่ยน model หลักเป็น `google/gemini-2.5-flash-lite` และใช้ `google/gemini-2.5-flash` เป็น fallback สำหรับงานที่ต้องการความเสถียรกว่า
- SML party cache reliability:
  - ตอน backend start ระบบ retry ดึงรายชื่อลูกค้า/ผู้ขายจาก SML หลายรอบแบบ backoff แทนการ fail ครั้งเดียว
  - `/api/sml/parties/last-sync` และ party picker ส่ง/แสดง `status`, `last_attempt`, `last_sync`, `error` เพื่อให้ผู้ใช้รู้ว่าควรกดรีเฟรชหรือตรวจ SML API
  - ตรวจ production ล่าสุดแล้ว `status=ok`, ลูกค้า 1,004 รายการ, ผู้ขาย 500 รายการ
- Admin users update:
  - เพิ่ม `/settings/users` และ `/api/settings/users` สำหรับ admin จัดการผู้ใช้ระบบ
  - รองรับ create/update/delete, เปลี่ยน role `admin/staff/viewer`, reset password โดยกรอกรหัสใหม่, ป้องกันลบตัวเองและป้องกันระบบไม่มี admin เหลือ
  - ทุก create/update/delete เขียน audit log source `settings`
- Logs permission update:
  - `/api/logs` เปิดเฉพาะ `admin` และ `staff`
  - `viewer` ยังอ่านข้อมูลธุรกิจที่อนุญาตได้ แต่ไม่เห็น activity/error payload ใน logs แล้ว

## Next Phase — Shopee API Direct

สถานะก่อนเริ่ม phase ถัดไป:

- Marketplace Excel ทั้ง 3 ช่องทางพร้อมสำหรับ UAT บน BillFlow main + Henna:
  - Shopee Excel
  - Lazada Excel
  - TikTok Excel/CSV
- Thaisunsport ยังเป็น Phase 1 ฝั่งซื้อเท่านั้น และยังไม่ควรเปิด sales/import channel.
- Shopee Open Platform API readiness พร้อมบน BillFlow main แล้ว, Shopee Go-Live approved/Online แล้ว, เชื่อมร้านจริงหลายร้านผ่าน OAuth สำเร็จแล้ว (`shop_id=264993963`, `shop_id=1029622928`), และพร้อมทดสอบ confirm แบบเลือก 1-2 orders หลัง preview ตรวจครบ.

แนวทางที่ควรรักษาไว้:

- Shopee API direct ควรสร้าง local bills เข้า pipeline เดิมเหมือน Shopee Excel:
  - `source='shopee'`
  - `bill_type='sale'`
  - `document_route='saleorder'` หรือ `saleinvoice` ตาม `/settings/channels`
  - ใช้ mapping/catalog/review/retry SML flow เดิมทั้งหมด
- Shopee Excel ควรยังคงอยู่เป็น fallback/manual import จนกว่า API direct จะ stable.
- เริ่ม implement/test บน BillFlow main ก่อน แล้วค่อย deploy main + Henna เมื่อผ่าน UAT.
- ไม่ deploy ไป Thaisunsport เว้นแต่ผู้ใช้สั่งเปิด Phase 1+ / งานฝั่งขายให้ร้านนี้.

สิ่งที่เหลือก่อน live Shopee API:

- Shopee Console ต้องเปลี่ยนเป็น live key หลัง approve และ Redirect URL Domain ต้องตรงกับ public URL ปัจจุบัน.
- ใช้ [shopee-open-api-live-cutover.md](shopee-open-api-live-cutover.md) และ `scripts/shopee-live-cutover.py` เพื่อใส่ live partner id/key โดยไม่พิมพ์ secret ลง chat/log.
- หลังเชื่อมร้านจริง ให้ดึงช่วงสั้นก่อน, ตรวจ preview count/order detail/logistics/no-SKU, แล้วค่อยกด confirm สร้าง local bills 1-2 orders. ห้าม auto-send SML รอบแรก.

## Latest Deploy Notes

### 2026-06-15 — Thaisunsport TT Payment + Lazada Group-Key Doc Ref Repair

- Deploy targets: `billflow-thaisunsport` only, plus shared `sml-api-bybos` gateway. `billflow` main was intentionally not redeployed.
- Commit: `691d01b fix: harden TT payments and Lazada doc ref grouping`.
- Scope:
  - Single and bulk purchase send dialogs auto-sync and lock BillFlow-only payment method when the selected supplier code/name starts with `TT`.
  - Non-TT suppliers clear the BillFlow-only payment method and can remain blank; the field is not required to send SML.
  - Lazada card `doc_ref` now depends on `raw_data.lazada_charge_group_key` parsed from the confirmation time in the email body. Missing group key blocks SML send instead of falling back to envelope `email_date`.
  - Backend Docker image packages `/app/lazada_group_key` for production dry-run/apply backfill.
  - Admin repair endpoint syncs sent Lazada PO `doc_ref` in both SML and BillFlow local payload with expected-value guards.
- Production actions:
  - Pre-repair backup: `/home/bosscatdog/billflow-thaisunsport/backups/manual-backups/pre-tt-lazada-group-key-20260615-103944.sql`.
  - Backfill `lazada_charge_group_key`: `updated=28`, `no_artifact=0`, `no_match=0`.
  - Target Lazada group `2026-06-11 16:45` has 8 active orders and total `7417.69`.
  - Patched already-sent docs `POL26060022` through `POL26060028` in that group to `doc_ref=7417.69`.
  - Repair manifest: `/home/bosscatdog/billflow-thaisunsport/backups/manual-backups/lazada-doc-ref-repair-manifest-20260615-104241.csv`.
- Verification:
  - Local BillFlow `go test ./...`, `npm run build`, and `sml-api-bybos go test ./...` passed before deploy.
  - Thaisunsport backend `8100/health`, frontend `3020`, and SML gateway `8200/health` passed after deploy.
  - SML dry-run recheck after repair returned `actual_old=7417.69`, `changed=false` for all 7 sent POL docs.

### 2026-06-09 — Marketplace Purchase Payment/Print + Bills List Performance

- Deploy targets: `billflow` main and `billflow-thaisunsport`.
- Scope:
  - Added `วิธีการชำระเงิน` dropdown to single-bill and bulk SML send dialogs for Shopee/Lazada purchase email.
  - Payment method is persisted in BillFlow before SML send and is not sent to SML.
  - Print readiness now uses email-group POL completeness plus payment-method prefix rule from `/settings/channels`.
  - Bills list row layout was compacted and marketplace print readiness enrichment was batched by email group.
  - Backend guard for payment method update now allows pre-POL updates while keeping source/type/config guards.
- Verification:
  - Local `npm run build` passed.
  - Local `go test ./internal/repository ./internal/handlers` passed.
  - Docker build/restart on both instances passed.
  - Main backend `8090` and thaisunsport backend `8100` returned healthy JSON.
  - Main frontend `3010` and thaisunsport frontend `3020` returned HTTP 200.

### BillFlow Main — Async Bulk SML Jobs

- Change type: SML send reliability / long-running admin workflow.
- Deploy target: `billflow` main only in this session.
- Commit: `871e8f5 feat: add async SML bulk send jobs` pushed to `main`.
- Scope:
  - Added migration `044_sml_bulk_jobs.sql`.
  - Added server-side job persistence for `ส่ง SML ทั้งหมด`.
  - Added duplicate `client_request_id` handling, active-job conflict guard, startup recovery for interrupted jobs, serial worker execution, progress polling, close/reopen resume, copyable errors, and retry failed only.
  - Single-bill `/api/bills/:id/retry` still uses the same core send path and remains available.
- Verification:
  - `go test ./...` passed.
  - `npm --prefix frontend run build` passed. Vite still warns about the existing large bundle/sonner dynamic import; build succeeds.
  - `scripts/preflight-main.sh` passed after deploy and after live smoke.
  - Live smoke sent one real Shopee shipped purchase bill through the async job and created SML purchaseorder `BF-PO26050001`.
- Next safe test:
  - Use 5-10 pending bills first, inspect progress/results/logs, then test up to the 100-bill cap.

### Main + Henna — Lazada Excel Phase 1+ Sales Flow

- Change type: Phase 1+ / งานฝั่งขาย / Marketplace Excel import.
- Deploy targets: `billflow`, `billflow-henna`.
- Skipped: `billflow-thaisunsport` เพราะยัง Phase 1 ฝั่งซื้อและ sales/Shopee Excel disabled.
- Scope:
  - Lazada Excel preview/confirm endpoints and `/import/lazada` frontend page.
  - Lazada sale bills created as local review items, then sent through existing Bill Detail Retry route.
  - `/settings/channels` and sales queue counters now include Lazada sale routes.
  - Migration `029_lazada_import.sql` adds duplicate guard for `source='lazada'` + `raw_data.order_id`.
- Verification at 2026-05-11 16:42 +07:
  - BillFlow main backend `8090`: `{"database":"ok","env":"production","status":"ok"}`
  - BillFlow Henna backend `8110`: `{"database":"ok","env":"production","status":"ok"}`
  - Frontend `3010` and `3030`: HTTP 200
  - Migration `029_lazada_import.sql` applied on both main and Henna.
  - Thaisunsport backend `8100` remains healthy and its containers were not restarted by this deploy.
- Follow-up at 2026-05-11 16:49 +07:
  - Fixed sidebar/command-palette visibility: Lazada menu no longer depends on `VITE_PHASE >= 2`; it follows `ENABLE_SALES_ORDERS` like Shopee.
  - Rebuilt/restarted frontend only for BillFlow main + Henna.
  - Verified `Lazada Excel` exists in built assets for both frontends and `/import/lazada` returns HTTP 200 on ports `3010` and `3030`.
  - Thaisunsport frontend was not restarted.

### End-of-Day Snapshot

- All three instances are deployed with the latest local backend/frontend/docs state from this session.
- Verified health at 2026-05-11 14:37 +07:
  - BillFlow main backend `8090`: `ok`; frontend `3010`: `HTTP/1.1 200 OK`
  - BillFlow Henna backend `8110`: `ok`; frontend `3030`: `HTTP/1.1 200 OK`
  - BillFlow Thaisunsport backend `8100`: `ok`; frontend `3020`: `HTTP/1.1 200 OK`
- Thaisunsport remains Phase 1 only:
  - `VITE_PHASE=1`
  - `VITE_ENABLE_SALES_ORDERS=false`
  - `VITE_ENABLE_SHOPEE_EXCEL=false`
- BillFlow main verification:
  - Purchase bill `67c0be5b-9247-4945-9dc0-85ad498243cf` exists in `/bills`, status `pending`, `sml_order_id=#260404V08VQU10`.
  - Item `ef15ccb5-c164-476e-84bb-3ef6657e531d` has verified product image URL `https://cf.shopee.co.th/file/th-11134207-81zth-mimxd9980lc477`.

### All Instances — SML `is_get_price` Line Item Flag

- Change type: Shared SML API payload requirement.
- Deploy targets: `billflow`, `billflow-henna`, `billflow-thaisunsport`; skipped instances: none.
- Added `is_get_price: 1` to every item/detail sent to SML:
  - SML 213 `sale_reserve` JSON-RPC `items`
  - SML 248 REST `saleorder` `items`
  - SML 248 REST `saleinvoice` `details`
  - SML 248 REST `purchaseorder` `items`
- Verification:
  - `go test ./...` passed locally.
  - New test `TestSMLLinePayloadsHardcodeIsGetPrice` asserts the JSON key exists with value `1` for all four routes.
  - Backend deployed and restarted on all three instances.
  - Health checked: `8090`, `8110`, `8100` all return `{"database":"ok","env":"production","status":"ok"}`.
  - Thaisunsport still has `VITE_PHASE=1`, `VITE_ENABLE_SALES_ORDERS=false`, `VITE_ENABLE_SHOPEE_EXCEL=false`.

### All Instances — Shared Phase 1 UX/Logs Clarity

- Change type: Shared Phase 1 UX/Logs clarity.
- Deploy targets: `billflow`, `billflow-henna`, `billflow-thaisunsport`; skipped instances: none.
- Deployed local `frontend/` and `backend/` to:
  - `/home/bosscatdog/billflow`
  - `/home/bosscatdog/billflow-henna`
  - `/home/bosscatdog/billflow-thaisunsport`
- Verified health after restart:
  - `8090`, `8110`, `8100` all return `{"database":"ok","env":"production","status":"ok"}`.
  - Frontend ports `3010`, `3030`, `3020` all return `HTTP/1.1 200 OK`.
  - Thaisunsport still has `VITE_PHASE=1`, `VITE_ENABLE_SALES_ORDERS=false`, `VITE_ENABLE_SHOPEE_EXCEL=false`.
- Browser check on BillFlow main `/logs` confirmed `ล้างข้อมูลทดสอบ`, `Setup`, and expanded guidance/facts render correctly.

### All Instances — Gmail IMAP Onboarding

- Change type: Shared Phase 1 email onboarding UX/bug.
- Deploy targets: `billflow`, `billflow-henna`, `billflow-thaisunsport`; skipped instances: none.
- Added Gmail checklist and direct setup links in `/settings/email`.
- Added frontend/backend Gmail App Password normalization and validation.
- Added backend unit tests for Gmail App Password separator normalization and short-password rejection.
- Verified after restart:
  - `8090`, `8110`, `8100` all return healthy JSON.
  - Frontend ports `3010`, `3030`, `3020` all return `HTTP/1.1 200 OK`.
  - Thaisunsport still has `VITE_PHASE=1`, `VITE_ENABLE_SALES_ORDERS=false`, `VITE_ENABLE_SHOPEE_EXCEL=false`.
  - Browser check on BillFlow main `/settings/email` confirmed checklist, dialog guidance, and `16/16` normalized password hint.

### All Instances — Bills Email Date Display

- Change type: Shared Phase 1 bills/email UX.
- Deploy targets: `billflow`, `billflow-henna`, `billflow-thaisunsport`; skipped instances: none.
- IMAP poll now stores `raw_data.email_date` from the email envelope date for new email-created bills.
- `/bills` displays `email_date` with the prefix `อีเมล` when present, falling back to `created_at` for older bills.
- Verified after restart:
  - `8090`, `8110`, `8100` all return healthy JSON.
  - Frontend ports `3010`, `3030`, `3020` all return `HTTP/1.1 200 OK`.
  - Thaisunsport still has `VITE_PHASE=1`, `VITE_ENABLE_SALES_ORDERS=false`, `VITE_ENABLE_SHOPEE_EXCEL=false`.
  - Browser check on BillFlow main `/bills` confirmed the page loads and table/empty state renders.

### All Instances — IMAP Read/Unread Polling

- Change type: Shared Phase 1 email ingestion bug.
- Deploy targets: `billflow`, `billflow-henna`, `billflow-thaisunsport`; skipped instances: none.
- IMAP search now uses the account `lookback_days` window without `NotFlag: Seen`, so both read and unread messages are considered.
- Dedup remains active through `processed_email_keys` and existing bills to prevent duplicate bill creation after reprocessing old mailboxes.
- Verified after restart:
  - `8090`, `8110`, `8100` all return healthy JSON.
  - Frontend ports `3010`, `3030`, `3020` all return `HTTP/1.1 200 OK`.
  - Thaisunsport still has `VITE_PHASE=1`, `VITE_ENABLE_SALES_ORDERS=false`, `VITE_ENABLE_SHOPEE_EXCEL=false`.
  - Production log on BillFlow main showed account `fe6a36b8-7092-483a-9265-2e63e248dccb` found 6 messages and received subject `Fwd: คำสั่งซื้อ #260404V08VQU10 ถูกจัดส่งแล้ว`.
  - The same message was skipped by dedup because `processed_email_keys` already had `shopee_shipped` entries for message id `CALTxiT_9pZFi7kCBMgT_2V6FCS8ghNNxkQaVmUbrpaAwULyvbg@mail.gmail.com` from 2026-05-08.

### All Instances — IMAP Poll Status Clarity

- Change type: Shared Phase 1 email settings UX/backend status clarity.
- Deploy targets: `billflow`, `billflow-henna`, `billflow-thaisunsport`; skipped instances: none.
- Added migration `028_imap_poll_stats.sql` with `last_poll_found`, `last_poll_processed`, and `last_poll_skipped`.
- `/settings/email` now shows `ผลรอบล่าสุด` as found/processed/skipped instead of labeling `last_poll_messages` as created bills.
- Verified after restart:
  - `8090`, `8110`, `8100` all return healthy JSON.
  - Frontend ports `3010`, `3030`, `3020` all return `HTTP/1.1 200 OK`.
  - Thaisunsport remains Phase 1 with sales/Shopee Excel disabled.
- BillFlow main verification:
  - Removed only the two `processed_email_keys` rows for message id `CALTxiT_9pZFi7kCBMgT_2V6FCS8ghNNxkQaVmUbrpaAwULyvbg@mail.gmail.com` / order `#260404V08VQU10`.
  - Restarted main backend to force immediate poll.
  - Poll created bill `67c0be5b-9247-4945-9dc0-85ad498243cf`, source `shopee_shipped`, status `pending`, `sml_order_id=#260404V08VQU10`.
  - Last poll stats for account `fe6a36b8-7092-483a-9265-2e63e248dccb`: found `6`, processed `4`, skipped `2`, status `ok`.

### All Instances — Shopee Product Image Extraction

- Change type: Shared Phase 1 email ingestion bug.
- Deploy targets: `billflow`, `billflow-henna`, `billflow-thaisunsport`; skipped instances: none.
- Fixed `extractShopeeImageURLs` so Shopee tracking/open pixels are excluded and product image CDN URLs are preferred over Shopee logo/app/social assets.
- Added backend test `TestExtractShopeeImageURLsPrefersProductImage`.
- Verified after restart:
  - `8090`, `8110`, `8100` all return healthy JSON.
  - Existing BillFlow main item `ef15ccb5-c164-476e-84bb-3ef6657e531d` now stores `https://cf.shopee.co.th/file/th-11134207-81zth-mimxd9980lc477`.
  - Server `curl -I` to that URL returned `HTTP/2 200` with `content-type: image/jpeg`.

### Thaisunsport

- Deployed from local BillFlow code to `/home/bosscatdog/billflow-thaisunsport`.
- Scope intentionally limited to Phase 1 purchase flow for same-day customer demo.
- Verified values in server `.env`:
  - `OPENROUTER_MODEL=google/gemini-2.5-flash-lite`
  - `OPENROUTER_FALLBACK_MODEL=google/gemini-2.5-flash`
  - `VITE_PHASE=1`
  - `VITE_ENABLE_SALES_ORDERS=false`
  - `VITE_ENABLE_SHOPEE_EXCEL=false`
- Verified endpoints:
  - `curl http://localhost:8100/health` returns healthy JSON.
  - `curl http://localhost:3020/` returns frontend HTML.
  - Quick Tunnel URL from `/tmp/billflow-thaisunsport-tunnel.log`: `https://pets-mini-museums-ships.trycloudflare.com`
- Backend logs after restart show IMAP polling activity and skipped messages by subject/sender filters; no critical backend startup failure was observed.

### Local Workspace

- Git branch: `main`.
- Last committed code change before this docs update: `4e20c33 fix: align main ui readiness states`.
- Worktree was clean after commit/deploy before this docs update.
- Codex skill `buddhist-method` is installed locally and updated with principles 7-16; current session can use it.

## Deployment

| Instance | Server folder | Frontend | Backend | PostgreSQL | Health |
|---|---|---:|---:|---:|---|
| BillFlow main | `/home/bosscatdog/billflow` | `3010` | `8090` | `5438` | ✅ |
| Thaisunsport | `/home/bosscatdog/billflow-thaisunsport` | `3020` | `8100` | `5448` | ✅ |
| Henna | `/home/bosscatdog/billflow-henna` | `3030` | `8110` | `5440` | ✅ |

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
| Shopee API direct | `/api/import/shopee/api/preview` fetches selected-shop orders with `create_time`/`update_time`, default ready-to-bill statuses, page-more guard, shipping/package/COD preview, and shop-scoped duplicate detection. Confirm still uses the same `/api/import/shopee/confirm` review-first bill creation path; no auto-send to SML. |
| Lazada Excel | `/api/import/lazada/preview` parses/dedups Lazada export by `orderNumber`, and `/api/import/lazada/confirm` creates local sale bills for the same review/retry flow as Shopee. |
| TikTok Excel/CSV | Deployed main + Henna: `/api/import/tiktok/preview` parses/dedups TikTok `.csv`/`.xlsx` exports by `Order ID`, keeps `Order Amount` order-level to avoid double-counting multi-row orders, and `/api/import/tiktok/confirm` creates local sale bills for the same review/retry flow. |
| Shopee SKU handling | Source SKU from Excel is stored separately as `bill_items.source_sku`. It only becomes SML `item_code` when the same code exists in local SML Catalog; otherwise the row remains needs review. |
| Shopee shipped email | Routes to purchase bill and SML 248 `purchaseorder`. |
| Bill Retry | 4-way dispatch: `sale_reserve`, `saleorder`, `saleinvoice`, `purchaseorder`, selected by source/bill type plus `channel_defaults.endpoint`. Phase 1 purchase send uses the Bill Detail confirmation dialog for supplier, warehouse, shelf, VAT, document time, branch/sale code, and remark. |
| Bulk SML send | `/bills`, `/sales-orders`, and `/sale-invoices` have `ส่ง SML ทั้งหมด` for `pending` documents. It loads a preview, validates each bill, shows expected sequential `doc_no` per ready row, then creates a DB-backed async job capped at 100 bills. The worker sends serially, stores progress/results, supports close/reopen resume, and can retry failed rows only. |
| Mapping dashboard | `/mappings` shows the saved mapping table plus a sidebar hotspot panel for raw product names still appearing in `needs_review` bills, so admins can fix repeated names before they cause more manual review. |
| หน้าเริ่มต้นใช้งาน | `/setup` checks required setup steps, shows shop/system counters, and provides an admin-only test-data reset dialog that preserves settings/catalog/mappings/AI usage by default. |
| Sidebar navigation | Sidebar groups document work by purchase/sale: `งานฝั่งซื้อ` and `งานฝั่งขาย`. Badges are per-document-route queue counts, not global pending count. |
| Bill detail | Shows route preview, blocks send when item validation fails, treats `item_code + mapped=false` as “ต้องยืนยัน” instead of send-ready, supports quick confirm per row, supports artifacts preview/download, stores optional `bills.remark`, and summarizes the latest SML request/response before raw JSON. |
| Logs | `/logs` shows action-specific summaries. Expanding a row shows key facts first (bill, doc_no, route, trace, error) and keeps raw JSON as a secondary technical view. |
| UX guardrails | Empty queues guide users to import/email setup, channel API details are collapsed, email sender mismatch is surfaced in Thai, and logs classify common SML failures into user-fixable vs support-needed actions. |
| Catalog | SML 248 catalog sync, product create, per-row refresh/delete, embeddings, in-memory cosine index, and SML product image thumbnails/gallery. |
| SSE | `/api/admin/events` streams inbox/admin events with HMAC token from `/api/admin/events/token`. |
| Background jobs | Daily insight, daily backup, disk monitor, LINE token checker, hourly reply-token cleanup, daily Cloudflare tunnel drift monitor, IMAP coordinator. |

## Database Notes

Local migrations currently run through:

- `001_init.sql` through `048_bill_item_discount_amount.sql`
- Important recent additions: `bill_artifacts`, `chat_conversations.status`, CRM phone/notes/tags, cached reply token, per-OA mark-as-read toggle, `bills.remark`, `app_settings`, document route defaults, processed email keys, AI usage logs, Shopee/Lazada/TikTok import runs, Shopee Open API tables/events, `bills.document_route`, `bill_items.source_sku`, `bill_items.source_image_url`, `imap_accounts.last_poll_details`, IMAP poll summary fields, SML catalog image metadata, async SML bulk job tables, Shopee multi-shop metadata, Shopee order search indexes, channel shipping item defaults, and per-line bill item discounts.

Async SML bulk job tables:
- `sml_bulk_jobs`: job header, status, counts, payload/filter snapshots, `client_request_id`, creator, timestamps.
- `sml_bulk_job_items`: ordered bill rows, item status, attempts, attempted/final `doc_no`, error, timestamps.
- On backend startup, stale queued/running jobs are marked failed with `server interrupted` so users can retry failed safely.

Production PostgreSQL also contains `system_settings` and `sml_settings`. These tables are not present in the current local migrations and are not referenced by the current codebase, so treat them as legacy leftovers until a future migration either formalizes or removes them.

## Current Document Menus

| Menu | URL | Backend filter | SML route |
|---|---|---|---|
| ใบสั่งซื้อ | `/bills` | source `shopee_shipped` หรือ `lazada_email`, bill_type `purchase` | `purchaseorder` |
| ใบสั่งขาย | `/sales-orders` | source `shopee`, bill_type `sale`, document_route `saleorder` | `saleorder` |
| ขายสินค้าและบริการ | `/sale-invoices` | source `shopee`, bill_type `sale`, document_route `saleinvoice` | `saleinvoice` v4 |

## Marketplace Purchase Print/Payment Rules

- Applies only to `shopee_shipped` and `lazada_email` purchase email bills.
- `bills.print_payment_method` is BillFlow-only data and is not sent to SML.
- If `print_payment_method` is blank and `sml_payload.supplier_name` starts with `TT`, that supplier name becomes the effective payment method.
- Single-bill and bulk SML send dialogs do not require selecting `วิธีการชำระเงิน`; TT suppliers auto-sync/lock the method and non-TT suppliers start blank.
- Current default print policy:
  - all active orders in the same email group must have `sml_doc_no`
  - effective payment method must start with `TT`
- `/bills?print_ready=1`, row print, bulk print, and Bill Detail print use backend readiness.
- `recordArtifactPrint` rejects stale/non-ready print requests even if the frontend button looked enabled.

Detailed runbook: [Marketplace Purchase Print And Payment Method](marketplace-purchase-print-and-payment.md).

## Phase 1 Purchase Flow

Phase 1 initially focused on Shopee purchase bills from email. The same review/send pattern is now also used by Lazada email purchase bills and marketplace Excel sale documents.

1. IMAP account receives Shopee or Lazada marketplace purchase email.
2. Email coordinator routes the message to `shopee_shipped` or `lazada_email` after channel/subject/sender guards.
3. Backend extracts order reference, order date, items, quantities, prices, and source artifacts. Lazada amounts are reconciled from HTML summary before send.
4. Bill is created as purchase bill and appears in `/bills`.
5. Admin reviews item rows, maps or creates SML products when needed.
6. Admin clicks send from Bill Detail or starts bulk send from `/bills`.
7. Confirmation dialog requires supplier, warehouse, shelf, VAT type, VAT rate, and inquiry type. For marketplace purchase email, `วิธีการชำระเงิน` is optional; when the selected supplier code/name is `TTxxxx`, it auto-syncs and locks to that TT value.
8. VAT rate is locked/read-only in purchase send dialogs and must come from channel config/preview.
9. Purchase send dialogs do not expose free-form `remark`; backend uses source seller/supplier name for SML `remark`.
10. Payment method is saved in BillFlow before SML send and is not sent to SML.
11. Backend posts to SML through the configured route. Current thaisunsport purchase flow uses the shared SML gateway:

```text
POST http://192.168.2.109:8200/api/v1/ic/purchase-orders
```

Required SML headers are read from production config. For thaisunsport, BillFlow sends `guid` plus tenant/database `data1_test`; main still uses its own SML tenant/config.

| Header | Current value |
|---|---|
| `guid` | `smlx` |
| `provider` | `SMLGOH` where legacy/direct route still needs it |
| `configFileName` | `SMLConfigSMLGOH.xml` where legacy/direct route still needs it |
| `databaseName` / tenant | thaisunsport: `data1_test`; main: `SML1_2026` / `sml1_2026` |
| `Content-Type` | `application/json; charset=utf-8` |

Purchase payload shape now follows SML v3 transaction attributes:

- Header includes `doc_no`, `doc_date`, `doc_time`, `doc_ref`, `doc_ref_date`, `doc_format_code`, `cust_code`, `supplier_name`, `branch_code`, `sale_code`, `wh_code`, `shelf_code`, `wh_from`, `location_from`, VAT totals, and `items`.
- `user_request` is sent as an empty string for Phase 1 purchaseorder.
- Item lines include `doc_ref`, `item_code`, `item_name`, `unit_code`, `qty`, `price`, `wh_code`, `shelf_code`, `wh_code_2`, `shelf_code_2`, VAT fields, and line totals.
- `branch_code` is included even when empty, because the confirmation dialog is the source of truth.

Known verified result: sending a Shopee purchase bill to SML `purchaseorder` succeeds and creates a document number such as `BF-PO26050002` / `BF-PO26050003`.
