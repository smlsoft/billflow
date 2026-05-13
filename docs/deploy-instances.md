# BillFlow Deploy Instances

Registry สำหรับจำว่าแต่ละร้านใช้ folder, port, container และ Cloudflare tunnel ไหนบน server `192.168.2.109`.

> หมายเหตุ: ตอนนี้ใช้ Cloudflare Quick Tunnel (`trycloudflare.com`) URL จะเปลี่ยนเมื่อ process `cloudflared` ถูก restart หรือเครื่องดับ ให้ดู URL ใหม่จาก log path ของ instance นั้น

## Summary

| Instance | ร้าน / วัตถุประสงค์ | Server folder | Frontend | Backend | PostgreSQL | Cloudflare URL ล่าสุด | Tunnel log |
| --- | --- | --- | ---: | ---: | ---: | --- | --- |
| `billflow` | BillFlow ปกติ / demo หลัก | `/home/bosscatdog/billflow` | `3010` | `8090` | `5438` | ดูจาก log | `/tmp/billflow-tunnel.log` |
| `billflow-thaisunsport` | Thaisunsport demo Phase 1 ฝั่งซื้อ | `/home/bosscatdog/billflow-thaisunsport` | `3020` | `8100` | `5448` | `https://pets-mini-museums-ships.trycloudflare.com` | `/tmp/billflow-thaisunsport-tunnel.log` |
| `billflow-henna` | Henna customer trial | `/home/bosscatdog/billflow-henna` | `3030` | `8110` | `5458` | `https://aurora-enjoyed-backup-lines.trycloudflare.com` | `/tmp/billflow-henna-tunnel.log` |

## Deploy Policy

ใช้ codebase เดียวสำหรับทุก instance และแยกความต่างด้วย environment / feature flags / instance config เท่านั้น.

| Change type | Deploy targets | Notes |
| --- | --- | --- |
| ทดสอบงานใหม่หรือแก้เฉพาะ demo หลัก | `billflow` | ใช้ main เป็นพื้นที่ทดสอบก่อน |
| Phase 1+ / งานฝั่งขาย / UX ที่เปิดทั้งซื้อและขาย | `billflow`, `billflow-henna` | Henna ต้องเทียบเท่า main และเปิดงานฝั่งซื้อ + งานฝั่งขาย |
| Shared Phase 1 bug/UX/backend/email/bills/logs/settings ที่ไม่ผูกกับงานฝั่งขาย | `billflow`, `billflow-henna`, `billflow-thaisunsport` | Thaisunsport รับเฉพาะสิ่งที่ใช้ร่วมกับ Phase 1 |
| งานเฉพาะร้าน เช่น credential, SML config, tunnel URL, env เฉพาะ instance | instance นั้นเท่านั้น | ห้ามกระทบ instance อื่น |

ก่อน deploy ทุกครั้งต้องระบุ `Change type`, `Deploy targets`, และ instance ที่ตั้งใจ skip ให้ชัดเจนในข้อความสรุป.

## Next Planned Phase — Shopee API Direct

- เริ่ม development/test บน `billflow` ก่อน เพราะเป็น demo หลักและใช้ตรวจ flow ใหม่ได้เร็วที่สุด.
- เมื่อ stable แล้ว deploy ไป `billflow` + `billflow-henna` เพราะ Henna ต้องเทียบเท่า main สำหรับ Phase 1+ / งานฝั่งขาย.
- ยังไม่ deploy ไป `billflow-thaisunsport` เพราะ instance นี้ยังเป็น Phase 1 ฝั่งซื้อ และปิด sales/import channel ด้วย feature flags.
- Shopee API direct ต้อง feed เข้า review/SML retry pipeline เดิมเหมือน Shopee Excel; ห้ามสร้าง SML send flow แยกถ้าไม่จำเป็น.
- Shopee Excel ต้องคงไว้เป็น fallback/manual import ระหว่าง UAT ของ API direct.

## Container Names

| Instance | Frontend container | Backend container | PostgreSQL container |
| --- | --- | --- | --- |
| `billflow` | `billflow-frontend` | `billflow-backend` | `billflow-postgres` |
| `billflow-thaisunsport` | `billflow-thaisunsport-frontend` | `billflow-thaisunsport-backend` | `billflow-thaisunsport-postgres` |
| `billflow-henna` | `billflow-henna-frontend` | `billflow-henna-backend` | `billflow-henna-postgres` |

## Quick Commands

### Check health

```bash
curl http://192.168.2.109:8090/health   # billflow
curl http://192.168.2.109:8100/health   # thaisunsport
curl http://192.168.2.109:8110/health   # henna
```

### Check running containers

```bash
docker ps --format '{{.Names}} {{.Ports}}' | grep billflow
```

### Get current Quick Tunnel URL

```bash
grep -oE 'https://[a-z0-9-]+\.trycloudflare\.com' /tmp/billflow-tunnel.log | tail -1
grep -oE 'https://[a-z0-9-]+\.trycloudflare\.com' /tmp/billflow-thaisunsport-tunnel.log | tail -1
grep -oE 'https://[a-z0-9-]+\.trycloudflare\.com' /tmp/billflow-henna-tunnel.log | tail -1
```

### Restart a Quick Tunnel

```bash
nohup cloudflared tunnel --url http://127.0.0.1:3010 --no-autoupdate > /tmp/billflow-tunnel.log 2>&1 &
nohup cloudflared tunnel --url http://127.0.0.1:3020 --no-autoupdate > /tmp/billflow-thaisunsport-tunnel.log 2>&1 &
nohup cloudflared tunnel --url http://127.0.0.1:3030 --no-autoupdate > /tmp/billflow-henna-tunnel.log 2>&1 &
```

## Port Allocation Rule

ใช้ pattern นี้สำหรับร้านถัดไป:

| Instance order | Frontend | Backend | PostgreSQL |
| ---: | ---: | ---: | ---: |
| Main | `3010` | `8090` | `5438` |
| Customer 1 | `3020` | `8100` | `5448` |
| Customer 2 | `3030` | `8110` | `5458` |
| Customer 3 | `3040` | `8120` | `5468` |

ให้ตั้งชื่อ folder/container/volume ตาม slug ร้าน เช่น `billflow-henna-*` เพื่อไม่ชน instance อื่น.

## Henna Notes

- Latest deploy verified: 2026-05-13 17:25 +07.
- Created from current normal BillFlow version, not Thaisunsport branch/config.
- Deployed as isolated Docker Compose project in `/home/bosscatdog/billflow-henna`.
- Database is separate PostgreSQL volume `billflow-henna_billflow_henna_pgdata`.
- `PUBLIC_BASE_URL` in `/home/bosscatdog/billflow-henna/.env` is set to the latest Henna Quick Tunnel URL.
- Current feature flags:
  - `VITE_PHASE=99`
  - `VITE_ENABLE_SALES_ORDERS=true`
  - `VITE_ENABLE_SHOPEE_EXCEL=true`
  - `VITE_ENABLE_LAZADA_EXCEL=true`
  - `VITE_ENABLE_TIKTOK_EXCEL=true`
  - `VITE_ENABLE_CHAT=false`
- App settings seeded:
  - `instance.name = BillFlow Henna`
  - `instance.slug = billflowhenna`

## Thaisunsport Notes

- Latest deploy verified: 2026-05-13 16:53 +07.
- Current purpose: customer demo for Phase 1 purchase flow only.
- Keep sale features disabled until the user explicitly asks to open Phase 1+ for this customer:
  - `VITE_PHASE=1`
  - `VITE_ENABLE_SALES_ORDERS=false`
  - `VITE_ENABLE_SHOPEE_EXCEL=false`
  - `VITE_ENABLE_LAZADA_EXCEL=false`
  - `VITE_ENABLE_TIKTOK_EXCEL=false`
  - `VITE_ENABLE_CHAT=false`
- AI model config on server:
  - `OPENROUTER_MODEL=google/gemini-2.5-flash-lite`
  - `OPENROUTER_FALLBACK_MODEL=google/gemini-2.5-flash`
- Verified after latest deploy:
  - backend health on `8100` is ok
  - frontend on `3020` serves HTML
  - frontend flags remain `VITE_PHASE=1`, sales/marketplace Excel/chat disabled
  - containers `billflow-thaisunsport-frontend`, `billflow-thaisunsport-backend`, and `billflow-thaisunsport-postgres` are up

## Latest Shared Deploy

- 2026-05-13 18:00 +07: Local cleanup completed after checkpoint `97d73bf`; deploy pending.
- Scope: remove unused frontend files/symbols, remove legacy channel-default delete/quick-setup endpoints, keep per-bill SML party picker and DB compatibility columns.
- Local verification: strict TypeScript unused-symbol check passed, frontend production build passed, backend `go test ./...` passed, and `git diff --check` passed.
- 2026-05-13 17:45 +07: Removed remaining legacy `/settings/channels` UI on main + Henna.
- Scope: no delete action in the channel settings table; dialog uses tested SML destination dropdown only and no longer exposes free-form API URL/path; `doc_format_code` is derived from the selected destination.
- Deploy targets: frontend rebuild/restart for `billflow` and `billflow-henna` only; Thaisunsport skipped.
- Local verification: `npm run build` passed.
- Deploy verification: main and Henna serve `index-B-g165c4.js`; deployed source has no delete UI, free-form SML API/path input, old endpoint override state, old channel help, customer/supplier column wording, or custom-route badge in Channel Defaults; Thaisunsport remains on `index-CpcZFriL.js`; backend health ok on `8090`, `8110`, `8100`.
- 2026-05-13 17:33 +07: Removed customer/supplier controls from `/settings/channels` on main + Henna.
- Scope: channel defaults are route-only now: SML destination, `doc_format_code`, document prefix, and running format. Per-bill values remain in the SML send dialog.
- Deploy targets: frontend rebuild/restart for `billflow` and `billflow-henna` only; Thaisunsport skipped.
- Local verification: `npm run build` passed.
- Deploy verification: main and Henna serve `index-Doi86S9A.js`; deployed Channel Defaults source has no customer/supplier picker, party refresh endpoint, or party sync status references; frontend containers are up on `3010` and `3030`; backend health ok on `8090`, `8110`, `8100`.
- 2026-05-13 17:25 +07: Generic `Email` rows hidden from `/settings/channels` on main + Henna.
- Scope: visible channel defaults now show `Email บิลซื้อ Shopee` (`shopee_shipped/purchase`) instead of generic `Email` sale/purchase rows; sales routes remain Shopee/Lazada/TikTok Excel.
- Deploy targets: frontend rebuild/restart for `billflow` and `billflow-henna` only; Thaisunsport skipped.
- Local verification: `npm run build` passed.
- Deploy verification: main and Henna serve `index-CsDcDUth.js`; frontend containers are up on `3010` and `3030`; backend health ok on `8090`, `8110`, `8100`.
- 2026-05-13 17:25 +07: Corrected main/Henna frontend feature flags after deploy-policy review.
- Scope: `billflow` and `billflow-henna` are Phase 1+ (`VITE_PHASE=99`) with sales/Shopee/Lazada/TikTok Excel enabled and chat disabled; `billflow-thaisunsport` remains Phase 1 purchase-only.
- Deploy targets: frontend rebuild/restart for `billflow` and `billflow-henna` only.
- Deploy verification: main and Henna serve `index-qyZlCwSs.js`; Thaisunsport still serves Phase 1 asset `index-CpcZFriL.js`; backend health ok on `8090`, `8110`, `8100`.
- 2026-05-13 16:53 +07: `/settings/channels` visibility fix deployed to all three instances.
- Scope: channel settings table now derives visible rows from instance feature flags instead of showing every backend-supported channel slot. Thaisunsport Phase 1 shows only `Email บิลซื้อ Shopee`; main and Henna hide LINE/chat routing because chat is disabled while keeping Phase 1+ purchase/sales marketplace routes.
- Deploy targets: `billflow`, `billflow-henna`, `billflow-thaisunsport`.
- Local verification: `npm run build` passed.
- Deploy verification: frontend `/settings/channels` HTTP 200 on `3010`, `3030`, `3020`.
- 2026-05-13 16:31 +07: `/api/bills` Shopee status performance hotfix deployed to all three instances.
- Scope: normal bill list/count no longer runs a latest-status lateral join before pagination; backend now batch-enriches Shopee status after selecting rows, and status-filtered lists use batch latest-event lookup.
- Root cause: Thaisunsport purchase bill list scanned 975 bills and 979 status events through per-row lateral lookup, making the reported URL take about 7 seconds.
- Deploy targets: `billflow`, `billflow-henna`, `billflow-thaisunsport`.
- Local verification: `GOCACHE=/private/tmp/billflow-gocache go test ./...` passed.
- Deploy verification: backend health ok on `8090`, `8110`, `8100`; `/api/bills` timing after deploy: main `0.007s`, Henna `0.022s`, Thaisunsport `0.051s`; backend logs clean for panic/fatal/list-bills SQL errors.
- 2026-05-13 16:01 +07: Shopee order status filter + dedicated order-status column deployed to all three instances.
- Scope: `/api/bills?shopee_status=...` filters by the latest matched `shopee_order_events` row; `/bills` separates `สถานะบิล` and `สถานะคำสั่งซื้อ`; status chips now have distinct visual tones; removed noisy `Email บิลซื้อ Shopee · บิลซื้อ` row label.
- Deploy targets: `billflow`, `billflow-henna`, `billflow-thaisunsport`.
- Local verification: `go test ./...` passed; `npm run build` passed.
- Deploy verification: backend health ok on `8090`, `8110`, `8100`; frontend `/bills` HTTP 200 on `3010`, `3030`, `3020`; backend logs clean for panic/fatal/migration/list-bills SQL errors.
- Authenticated API verification: main `shopee_status=shipped` total `1`; Henna totals `0` because no status events exist yet; Thaisunsport `shipped=508`, `payment_confirmed=463`.
- Thaisunsport flags remain `VITE_PHASE=1`, sales/Shopee/Lazada/TikTok Excel/chat disabled.
- 2026-05-13 15:25 +07: Shopee order email status timeline deployed to all three instances.
- Scope: migration `033_shopee_order_events.sql`, backend event storage/matching, Shopee status subject handling, `/bills` latest status badge, and bill detail status timeline.
- Deploy targets: `billflow`, `billflow-henna`, `billflow-thaisunsport`.
- Local verification: `GOCACHE=/private/tmp/billflow-gocache go test ./...` passed; `npm run build` passed.
- Deploy verification: backend health ok on `8090`, `8110`, `8100`; frontend HTTP 200 on `3010`, `3030`, `3020`; migration table exists in all three DBs.
- Backfill: existing Shopee email bills were inserted into `shopee_order_events` where possible; Thaisunsport inserted 972 events and public `/api/bills` returns `shopee_status`.
- Thaisunsport flags remain `VITE_PHASE=1`, sales/Shopee/Lazada/TikTok Excel/chat disabled.
- 2026-05-13 13:20 +07: Shared Phase 1 IMAP support controls deployed to all three instances.
- Scope: backend adds `POST /api/settings/imap-accounts/:id/reset-progress` to reset one inbox cursor and optionally update `lookback_days` without clearing dedup history.
- Scope: `/settings/email` now has `ตั้งช่วงย้อนหลัง / อ่านใหม่`, plus skipped-reason summaries in the table and poll detail dialog.
- Deploy targets: `billflow`, `billflow-henna`, `billflow-thaisunsport`.
- Local verification: `npm run build` passed; `GOCACHE=... go test ./...` passed.
- Deploy verification: backend health ok on `8090`, `8110`, `8100`; Thaisunsport `pd.thaisunsport2@gmail.com` status is now `ok`, `consecutive_failures=0`, backlog cleared.
- Browser verification: Thaisunsport `/settings/email` confirmed reason summary, detail dialog, and reset lookback dialog.
- Thaisunsport flags remain `VITE_PHASE=1`, sales/Shopee/Lazada/TikTok Excel/chat disabled.
- 2026-05-13 13:05 +07: Shared Phase 1 IMAP large-mailbox reliability + email detail UX deployed to all three instances.
- Scope: migration `032_imap_poll_progress.sql` adds poll cursor/backlog fields; IMAP now reads bounded batches (`IMAP_MAX_MESSAGES_PER_RUN`, default 150) and treats `backlog`/`partial` as progress statuses instead of failures.
- Scope: Shopee duplicate precheck now happens before fetching message body/attachments when the envelope/message-id already proves it was processed.
- Scope: `/settings/email` shows backlog as “กำลังทยอยอ่าน” with `ดูรายละเอียด` / `อ่านชุดถัดไป`, and the latest poll details open in a searchable/filterable dialog for 100+ email rows.
- Deploy targets: `billflow`, `billflow-henna`, `billflow-thaisunsport`.
- Local verification: `npm run build` passed; `GOCACHE=... go test ./...` passed.
- Deploy verification: backend health ok on `8090`, `8110`, `8100`; Thaisunsport production `pd.thaisunsport2@gmail.com` now reports `backlog`, `consecutive_failures=0` instead of `fetch_failed`.
- Browser verification: Thaisunsport `/settings/email` confirmed backlog banner, detail dialog, real Shopee rows, and order-number search.
- Thaisunsport flags remain `VITE_PHASE=1`, sales/Shopee/Lazada/TikTok Excel/chat disabled.
- 2026-05-12 14:35 +07: Latest codebase snapshot deployed and verified on all three instances before starting Shopee API direct phase.
- Scope: synced backend/frontend/docs to `billflow`, `billflow-henna`, and `billflow-thaisunsport`, then rebuilt/restarted backend + frontend containers.
- Verification: backend health ok on `8090`, `8110`, `8100`; frontend HTTP 200 on `3010`, `3030`, `3020`.
- Flags verified in both `.env` and `docker-compose.yml` build args:
  - Main: `VITE_PHASE=99`, sales/Shopee/Lazada/TikTok Excel enabled, chat disabled.
  - Henna: `VITE_PHASE=99`, sales/Shopee/Lazada/TikTok Excel enabled, chat disabled.
  - Thaisunsport: `VITE_PHASE=1`, sales/Shopee/Lazada/TikTok Excel disabled, chat disabled.
- Next work: Shopee API direct starts on `billflow`; deploy to Henna only after UAT; skip Thaisunsport until sales features are explicitly enabled.
- 2026-05-12 11:34 +07: Audit actor + production log accountability deployed to all three instances.
- Scope: backend `/api/logs` now returns `actor` with user name/email/role when `user_id` exists, classifies background entries as worker/system, and supports `user_id` filtering.
- Scope: `/logs` shows the actor badge in each row, adds a `ผู้ทำรายการ` filter, removes playful emoji from action labels, and keeps DEV payload copyable for admins/devs.
- Scope: SML send success/failure, bill item add/delete, and mapping feedback logs now write the current `user_id`; backend also blocks malformed `doc_no` with hidden/Thai mark characters before sending to SML.
- Change type: Shared Phase 1+ audit/accountability/data-quality hardening.
- Local verification: `npm run build` passed; `GOCACHE=... go test ./...` passed.
- Deploy verification: backend health ok on `8090`, `8110`, `8100`; frontend `/logs` HTTP 200 on `3010`, `3030`, `3020`; main `/api/logs` verified returning `actor` for Admin entries.
- Thaisunsport flags remain `VITE_PHASE=1`, `VITE_ENABLE_SALES_ORDERS=false`, `VITE_ENABLE_SHOPEE_EXCEL=false`.
- 2026-05-12 11:11 +07: Logs Action View deployed to all three instances.
- Scope: `/logs` now has summary cards, quick filters, DEV toggle, grouped import runs, SML failure incident cards, copyable DEV payload, and data-quality warning for malformed/hidden-character `doc_no`.
- Change type: Shared Phase 1+ frontend UX/debug clarity.
- Local verification: `npm run build` passed; browser check on local `/logs` confirmed new summary/DEV/filter controls render.
- Deploy verification: backend health ok on `8090`, `8110`, `8100`; frontend `/logs` HTTP 200 on `3010`, `3030`, `3020`.
- Thaisunsport flags remain `VITE_PHASE=1`, `VITE_ENABLE_SALES_ORDERS=false`, `VITE_ENABLE_SHOPEE_EXCEL=false`.
- 2026-05-12 10:50 +07: Docs/status update for next phase; no application deploy.
- Scope: documented latest production status and Shopee API direct handoff plan.
- Current import readiness: Shopee Excel, Lazada Excel, and TikTok Excel/CSV are ready for BillFlow main + Henna UAT; Thaisunsport remains Phase 1 purchase-only.
- Next deploy policy: Shopee API direct starts on `billflow`, then `billflow` + `billflow-henna` after UAT; skip Thaisunsport until sales features are explicitly enabled.
- 2026-05-12 10:35 +07: Action Center and expanded error playbook deployed to all three instances.
- Scope: Dashboard now has `Action Center` that ranks next-best actions across setup, email errors, SML failures, mapping review, and pending SML sends.
- Scope: `/logs` guidance now classifies more failure causes: SML timeout/network, doc format, customer/supplier, VAT, warehouse/shelf, item/unit, Gmail App Password, and AI quota/credit.
- Change type: Shared frontend UX/workflow clarity.
- Local verification: `npm run build` passed; browser check confirmed Dashboard `Action Center` and Logs page render.
- Deploy verification: backend health ok on `8090`, `8110`, `8100`; frontend Dashboard ok on `3010`, `3030`, `3020`, and main `/logs` ok.
- Thaisunsport flags remain `VITE_PHASE=1`, `VITE_ENABLE_SALES_ORDERS=false`, `VITE_ENABLE_SHOPEE_EXCEL=false`.
- 2026-05-12 10:12 +07: Bulk send summary/results and mapping hotspot dashboard deployed to all three instances.
- Scope: bulk send dialog now shows a pre-send summary, clearer post-send success/fail/skip result panel, copyable SML error summary, and direct link to the first failed bill.
- Scope: `/mappings` now shows top raw product names still blocking `needs_review` bills, with counts and a link to the first affected bill for faster mapping cleanup.
- Change type: Shared frontend UX/workflow clarity.
- Local verification: `npm run build` passed; browser check confirmed `/mappings` renders the new hotspot panel and `/sale-invoices` loads.
- Deploy verification: backend health ok on `8090`, `8110`, `8100`; frontend route ok on main/Henna `/sale-invoices` and Thaisunsport `/bills`.
- Thaisunsport flags remain `VITE_PHASE=1`, `VITE_ENABLE_SALES_ORDERS=false`, `VITE_ENABLE_SHOPEE_EXCEL=false`.
- 2026-05-12 09:48 +07: Bulk Send dialog readability follow-up deployed to all three instances.
- Scope: `/bills`, `/sales-orders`, and `/sale-invoices` bulk-send dialog uses a wider modal, reduces noisy helper text, and shows ready rows as a table with send sequence, order no, expected `doc_no`, and status.
- Fix: frontend computes sequential expected `doc_no` from the backend preview so bulk rows no longer all appear to use the same next document number before the real send reserves numbers.
- Change type: Shared frontend UX/preview clarity.
- Local verification: `npm run build` passed.
- Deploy verification: backend health ok on `8090`, `8110`, `8100`; frontend route ok on main/Henna `/sale-invoices` and Thaisunsport `/bills`.
- Thaisunsport flags remain `VITE_PHASE=1`, `VITE_ENABLE_SALES_ORDERS=false`, `VITE_ENABLE_SHOPEE_EXCEL=false`.
- 2026-05-12: Bulk Send dialog UI parity deployed to all three instances.
- Scope: `/bills`, `/sales-orders`, and `/sale-invoices` bulk-send dialog now follows the same structure and visual language as the Bill Detail send dialog while keeping the bulk-only ready/skipped result list.
- Follow-up: each ready/skipped row now shows upstream order number plus `doc_no` preview so users can verify document numbers before bulk sending.
- Change type: Shared frontend UX parity.
- Local verification: `npm run build` passed.
- Deploy verification: backend health ok on `8090`, `8110`, `8100`; frontend route ok on main/Henna `/sale-invoices` and Thaisunsport `/bills`.
- Thaisunsport flags remain `VITE_PHASE=1`, `VITE_ENABLE_SALES_ORDERS=false`, `VITE_ENABLE_SHOPEE_EXCEL=false`.
- 2026-05-12 follow-up: deployed doc_no preview per row in the same bulk-send dialog; verified the same health/routes and Thaisunsport flags.
- 2026-05-12: Verified Mapping Loop backend fix deployed to all three instances.
- Scope: saving a bill item now learns mapping even when the AI-prefilled code did not change, then applies the verified mapping to open bills with the same source/bill_type/raw_name and promotes fully mapped bills from `needs_review` to `pending`.
- Change type: Shared mapping workflow bug for marketplace/import/email bill detail review.
- Local verification: `GOCACHE=... go test ./...` passed.
- Deploy verification: backend health ok on `8090`, `8110`, `8100`.
- Henna data follow-up: applied existing Shopee saleinvoice mapping to 3 open item rows and promoted 3 bills to `pending`; remaining `needs_review` bills are 3 distinct raw names/options with no confirmed mapping yet.
- 2026-05-12: Shared Bill Detail item mapping UI fix deployed to all three instances.
- Scope: when editing a bill item and selecting a new SML product, the edit row and saved item table now show the newly selected product name/score immediately without requiring a page refresh.
- Change type: Shared Bill Detail UX bug, applies to purchase, saleorder, and saleinvoice item rows.
- Local verification: `npm run build` passed.
- Deploy verification: backend health ok on `8090`, `8110`, `8100`; frontend `/bills` returns HTTP 200 on `3010`, `3030`, `3020`.
- Thaisunsport flags remain `VITE_PHASE=1`, `VITE_ENABLE_SALES_ORDERS=false`, `VITE_ENABLE_SHOPEE_EXCEL=false`.
- 2026-05-12 09:40 +07: Shopee Excel status filter updated on `billflow` and `billflow-henna`.
- Scope: `/import/shopee` now imports rows with status `ที่ต้องจัดส่ง`; only `ยกเลิกแล้ว` remains filtered out.
- Added parser test `TestParseShopeeExcelKeepsReadyToShipStatus`.
- Skipped `billflow-thaisunsport` intentionally because Shopee Excel remains disabled in Phase 1.
- 2026-05-12 09:24 +07: Shared Phase 1 IMAP poll detail UX deployed to all three instances.
- Scope: migration `031_imap_poll_details.sql` adds `imap_accounts.last_poll_details`; `/settings/email` can expand `ผลรอบล่าสุด` to show subject/from/date plus processed/skipped reason per email.
- Deploy targets: `billflow`, `billflow-henna`, `billflow-thaisunsport`.
- Also patched old source-check migrations `002_sml_catalog.sql` and `004_shopee_shipped.sql` to include `tiktok`, so boot migrations remain idempotent after TikTok bills exist.
- Verified backend health: `8090`, `8110`, `8100`.
- Verified frontend route `/settings/email`: HTTP 200 on `3010`, `3030`, `3020`.
- Verified `last_poll_details` column exists in all three PostgreSQL containers; latest poll details stored on main and Thaisunsport.
- 2026-05-11 20:19 +07: TikTok Excel/CSV import deployed to `billflow` and `billflow-henna` only.
- Scope: `/import/tiktok`, TikTok preview/confirm API, `tiktok` channel sale routing, migration `030_tiktok_import.sql`, and parser tests for real TikTok CSV shape.
- Verified backend health: `8090`, `8110`.
- Verified frontend route: `/import/tiktok` returns HTTP 200 on `3010`, `3030`.
- Verified migration `030_tiktok_import.sql` applied on main and Henna.
- Skipped `billflow-thaisunsport` intentionally because it remains Phase 1 purchase-only; backend `8100` was health-checked but containers were not restarted.
- 2026-05-11 16:42 +07: Phase 1+ Lazada Excel sales flow deployed to `billflow` and `billflow-henna` only.
- Scope: `/import/lazada`, Lazada preview/confirm API, `lazada` channel sale routing, sales queue counts including Lazada, and migration `029_lazada_import.sql`.
- Verified backend health: `8090`, `8110`.
- Verified frontend health: `3010`, `3030`.
- Verified migration `029_lazada_import.sql` applied on main and Henna.
- Skipped `billflow-thaisunsport` intentionally because it remains Phase 1 purchase-only; backend `8100` was health-checked but containers were not restarted.
- 2026-05-11 16:49 +07: Frontend-only follow-up for Lazada menu visibility on `billflow` and `billflow-henna`.
- Scope: remove `VITE_PHASE >= 2` gating from Lazada nav/command-palette items so the menu appears when sales features are enabled, matching Shopee Excel behavior.
- Verified built assets contain `Lazada Excel` and `/import/lazada` returns HTTP 200 on ports `3010` and `3030`.
- Skipped `billflow-thaisunsport`; frontend container uptime unchanged from before this follow-up.
- 2026-05-11 15:32 +07: Shared SML API payload requirement deployed to all three instances.
- Scope: all SML item/detail payloads now hardcode `is_get_price: 1` per line item for `sale_reserve`, `saleorder`, `saleinvoice`, and `purchaseorder`.
- Verified local `go test ./...`; added backend test `TestSMLLinePayloadsHardcodeIsGetPrice`.
- Verified backend health: `8090`, `8110`, `8100`.
- Verified Thaisunsport flags remain `VITE_PHASE=1`, `VITE_ENABLE_SALES_ORDERS=false`, `VITE_ENABLE_SHOPEE_EXCEL=false`.
- 2026-05-11 14:37 +07: End-of-day verification completed for all three instances.
- Scope: no new code after the Shopee product-image fix; verified latest deployed state, docs synced, and BillFlow main purchase bill `#260404V08VQU10` remains `pending` with product image URL stored.
- Verified backend health: `8090`, `8110`, `8100`.
- Verified frontend health: `3010`, `3030`, `3020`.
- Verified Thaisunsport flags remain `VITE_PHASE=1`, `VITE_ENABLE_SALES_ORDERS=false`, `VITE_ENABLE_SHOPEE_EXCEL=false`.
- 2026-05-11 12:45 +07: Shared Phase 1 UX/Logs clarity deployed to all three instances.
- Scope: `/logs` now explains `demo_test_data_reset` as `ล้างข้อมูลทดสอบ`, shows source `Setup`, and expands into user-readable facts/guidance instead of forcing users to interpret raw JSON.
- Verified backend health: `8090`, `8110`, `8100`.
- Verified frontend health: `3010`, `3030`, `3020`.
- 2026-05-11 12:59 +07: Shared Phase 1 Gmail IMAP onboarding fix deployed to all three instances.
- Scope: `/settings/email` now guides users through 2-Step Verification, Google App Password, and Gmail IMAP setup; Gmail App Passwords are normalized by removing spaces/dashes before test/save.
- Verified browser UI on BillFlow main and health on all three backend/frontend ports.
- 2026-05-11 13:10 +07: Shared Phase 1 bills email-date display deployed to all three instances.
- Scope: new email-created bills store `raw_data.email_date` from the IMAP envelope date; `/bills` shows that date with prefix `อีเมล` when available and falls back to `created_at` for older bills.
- Verified browser `/bills` on BillFlow main and health on all three backend/frontend ports.
- 2026-05-11 13:43 +07: Shared Phase 1 IMAP read/unread polling fix deployed to all three instances.
- Scope: IMAP now searches both read and unread messages within `lookback_days` instead of only unread messages, while keeping durable dedup through `processed_email_keys` to prevent duplicate bills.
- Verified health on all three backend/frontend ports. Production main logs confirmed account `fe6a36b8-7092-483a-9265-2e63e248dccb` found 6 messages and saw order email `260404V08VQU10`; it was skipped because a dedup tombstone already existed from 2026-05-08.
- 2026-05-11 14:20 +07: Shared Phase 1 IMAP poll-status clarity deployed to all three instances.
- Scope: migration `028_imap_poll_stats.sql` adds `last_poll_found`, `last_poll_processed`, `last_poll_skipped`; `/settings/email` now shows `ผลรอบล่าสุด` as `พบ / ประมวลผล / ข้าม` instead of implying `last_poll_messages` equals created bills.
- Verified health on all three backend/frontend ports and Thaisunsport Phase 1 flags. On BillFlow main, cleared dedup only for message/order `260404V08VQU10`, restarted backend, and poll created bill `67c0be5b-9247-4945-9dc0-85ad498243cf` with status `pending`.
- 2026-05-11 14:34 +07: Shared Phase 1 Shopee product-image extraction fix deployed to all three instances.
- Scope: exclude Shopee tracking/open pixel URLs from `source_image_url` and prefer product CDN URLs such as `cf.shopee.co.th/file/th-*`; existing BillFlow main item `ef15ccb5-c164-476e-84bb-3ef6657e531d` was updated to the verified product image URL.
- Verified backend health on all three instances; product image URL returned `HTTP/2 200` and `content-type: image/jpeg`.
