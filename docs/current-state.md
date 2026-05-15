# BillFlow — Current State

> Updated: 2026-05-15 14:40 +07
> Source of truth checked: local tests/build, deployed Docker Compose services on `192.168.2.109`, backend health, frontend route checks, migration logs, PostgreSQL schema, and instance feature flags.

## New Chat Handoff

อ่าน 3 ไฟล์นี้ก่อนเริ่มงานทุกครั้ง:

1. `AGENTS.md`
2. `docs/current-state.md`
3. `docs/deploy-instances.md`

ห้ามเดาจาก context เก่า ถ้าเอกสารอื่นขัดกับ 3 ไฟล์นี้ ให้ยึด 3 ไฟล์นี้เป็นหลัก.

## Latest Deploy

- Latest deployed app commit: `be7734e Add catalog lifecycle and review queue UX`
- Branch: `checkpoint-channel-defaults-cleanup`
- Deploy time verified: 2026-05-15 14:40 +07
- Deploy targets: `billflow`, `billflow-henna`, `billflow-thaisunsport`
- Thaisunsport still remains Phase 1 purchase-only through its `.env` feature flags.

### Verification Passed

- Local before deploy:
  - `go test ./...`
  - `npm run build`
  - `git diff --check`
  - `PYTHONPYCACHEPREFIX=/private/tmp/billflow-pycache python3 -m py_compile scripts/deploy.py`
- Backend health:
  - main `8090`: ok
  - Henna `8110`: ok
  - Thaisunsport `8100`: ok
- Frontend routes:
  - main `3010`: `/sale-invoices`, `/marketplace-aliases`, `/settings/catalog`, `/settings/old-data` all HTTP 200
  - Henna `3030`: `/sale-invoices`, `/marketplace-aliases`, `/settings/catalog`, `/settings/old-data` all HTTP 200
  - Thaisunsport `3020`: `/bills`, `/settings/catalog`, `/settings/old-data` all HTTP 200
- Migration `036_sml_catalog_lifecycle.sql` applied on all three instances.
- `sml_catalog` columns exist on all three DBs:
  - `is_active`
  - `missing_at`
  - `last_seen_at`
- Frontend assets after deploy:
  - main/Henna: `assets/index-ZO1PU287.js`
  - Thaisunsport: `assets/index-PGhS5G_e.js`

## Instance Status

| Instance | Purpose | Frontend | Backend | PostgreSQL | Phase / flags |
| --- | --- | ---: | ---: | ---: | --- |
| `billflow` | demo หลัก / ทดสอบงานใหม่ | `3010` | `8090` | `5438` | Phase 2, sales + Shopee/Lazada/TikTok Excel enabled, chat disabled |
| `billflow-henna` | Henna customer trial | `3030` | `8110` | `5458` | Phase 2, sales + Shopee/Lazada/TikTok Excel enabled, chat disabled |
| `billflow-thaisunsport` | Thaisunsport demo | `3020` | `8100` | `5448` | Phase 1 purchase-only, sales/marketplace Excel/chat disabled |

Detailed registry: `docs/deploy-instances.md`.

## Current Feature Set

- `/bills`, `/sales-orders`, `/sale-invoices`
  - Compact header UX.
  - Bulk send includes `pending + needs_review` but sends only rows passing validation.
  - Low-confidence/unconfirmed item matches are blocked until user confirms mapping.
  - Sales tables hide purchase-only order-status column.
- Marketplace matching
  - SKU-first remains when marketplace SKU equals real SML item code.
  - Marketplace alias learning stores confirmed Shopee/Lazada/TikTok SKU/name/variant to SML item mapping.
  - `/marketplace-aliases` is a dense review queue with search/filter/sort/pagination.
  - Sidebar shows red badge for urgent queues including `สินค้ารอยืนยัน`.
- Catalog sync
  - `/settings/catalog` is manual-first.
  - Full sync marks products missing from SML as inactive instead of hard-delete.
  - Matching/search hides inactive products.
  - Before SML send, BillFlow verifies only item codes used in that bill and uses short cache for bulk send.
- Old data management
  - `/settings/old-data` supports `เก็บบิล`, `บิลที่เก็บแล้ว`, `กู้คืน`, `ลบบิล`, `ลบถาวร`.
  - Permanent delete remains admin-only.
  - Logs keep important snapshots even after permanent delete.
- TikTok import
  - TikTok Excel/CSV now imports `ที่จะจัดส่ง` in addition to `จัดส่งแล้ว`, `shipped`, `delivered`, `completed`, `to ship`, `ready to ship`, and `awaiting shipment`.
  - Cancelled/non-sale-ready rows are still skipped.

## Sidebar / Menu Model

- `งานฝั่งซื้อ`
  - `ใบสั่งซื้อ`
- `งานฝั่งขาย`
  - `ใบสั่งขาย`
  - `ขายสินค้าและบริการ`
- `งานที่ต้องตรวจ`
  - `สินค้ารอยืนยัน`
- `ช่องทางรับข้อมูล`
  - email and marketplace import screens
- `ข้อมูลหลัก`
  - `ตารางจับคู่สินค้า`
  - `สินค้าใน SML`
- `ตั้งค่าระบบ`
  - SML route settings, old-data management, AI usage, users, instance, general settings

Urgent badges are red for purchase/sales document queues and marketplace alias review.

## Deploy Policy

- Shared backend/frontend/data safety changes deploy to all three instances.
- Phase 2 sales/import work deploys to main + Henna; Thaisunsport receives shared code but remains protected by Phase 1 flags unless explicitly upgraded.
- Do not change customer-specific credentials/config across instances unless the user explicitly asks.
- Server folders are deployed copies, not git checkouts.

## Git / Local State

- App commit deployed: `be7734e`
- Intentionally untracked sample files left out of commits:
  - `Order.all.20260401_20260430.xlsx`
  - `lazada.xlsx`
  - `tiktok_csv.csv`
  - `tiktok_excel.xlsx`

## Next Phase

Next planned work remains Shopee Open Platform API direct sync.

Default path:

1. Build/test on `billflow` first.
2. When stable, deploy to `billflow` + `billflow-henna`.
3. Skip `billflow-thaisunsport` for sales/API-direct features until the user explicitly enables Phase 2 for that customer.
4. Feed Shopee API orders into the same local bill review/SML retry pipeline as Shopee Excel.
