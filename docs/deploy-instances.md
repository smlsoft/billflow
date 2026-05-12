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

- Latest deploy verified: 2026-05-11 16:49 +07.
- Created from current normal BillFlow version, not Thaisunsport branch/config.
- Deployed as isolated Docker Compose project in `/home/bosscatdog/billflow-henna`.
- Database is separate PostgreSQL volume `billflow-henna_billflow_henna_pgdata`.
- `PUBLIC_BASE_URL` in `/home/bosscatdog/billflow-henna/.env` is set to the latest Henna Quick Tunnel URL.
- App settings seeded:
  - `instance.name = BillFlow Henna`
  - `instance.slug = billflowhenna`

## Thaisunsport Notes

- Latest deploy verified: 2026-05-11 15:32 +07.
- Current purpose: customer demo for Phase 1 purchase flow only.
- Keep sale features disabled until the user explicitly asks to open Phase 1+ for this customer:
  - `VITE_PHASE=1`
  - `VITE_ENABLE_SALES_ORDERS=false`
  - `VITE_ENABLE_SHOPEE_EXCEL=false`
- AI model config on server:
  - `OPENROUTER_MODEL=google/gemini-2.5-flash-lite`
  - `OPENROUTER_FALLBACK_MODEL=google/gemini-2.5-flash`
- Verified after latest deploy:
  - backend health on `8100` is ok
  - frontend on `3020` serves HTML
  - frontend flags remain `VITE_PHASE=1`, `VITE_ENABLE_SALES_ORDERS=false`, `VITE_ENABLE_SHOPEE_EXCEL=false`
  - containers `billflow-thaisunsport-frontend`, `billflow-thaisunsport-backend`, and `billflow-thaisunsport-postgres` are up

## Latest Shared Deploy

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
