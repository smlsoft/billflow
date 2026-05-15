# AGENTS.md — BillFlow

Blueprint สำหรับ Codex/AI agents ใน repo นี้.

## Read First

ก่อนเริ่ม code ทุกครั้งให้อ่านตามลำดับนี้:

1. `AGENTS.md`
2. `docs/current-state.md`
3. `docs/deploy-instances.md`

ถ้าเอกสารอื่นหรือ context เก่าขัดกับ 3 ไฟล์นี้ ให้ยึด 3 ไฟล์นี้เป็น source of truth.

## Workspace

- Local workspace: `/Users/nontawatwongnuk/dev_bos/billflow`
- Server: `192.168.2.109`
- Server deploy folders are copied deployments, not git checkouts:
  - main: `/home/bosscatdog/billflow`
  - Henna: `/home/bosscatdog/billflow-henna`
  - Thaisunsport: `/home/bosscatdog/billflow-thaisunsport`
- Git branch currently used for this workstream: `checkpoint-channel-defaults-cleanup`

## Product Summary

BillFlow ลดงานคีย์บิลเข้า SML โดยรับข้อมูลจาก Email, Shopee/Lazada/TikTok Excel, LINE OA code path, และต่อไป Shopee API direct. ระบบสร้าง local bills ให้พนักงานตรวจสินค้า/route/party/ภาษีก่อนส่งเข้า SML.

Core rule: **manual-confirm first**. อย่าสร้าง flow ใหม่ที่ auto-send เข้า SML โดยไม่มี validation/review guard.

## Current Instances

| Instance | Frontend | Backend | Phase |
| --- | ---: | ---: | --- |
| `billflow` | `3010` | `8090` | Phase 2, sales/import enabled, chat disabled |
| `billflow-henna` | `3030` | `8110` | Phase 2, sales/import enabled, chat disabled |
| `billflow-thaisunsport` | `3020` | `8100` | Phase 1 purchase-only, sales/import/chat disabled |

ดู registry เต็มที่ `docs/deploy-instances.md`.

## Stack

- Backend: Go 1.24, Gin
- Frontend: React + Vite + TypeScript
- DB: PostgreSQL 16
- AI: OpenRouter
- Deploy: Docker Compose + Cloudflare Quick Tunnel
- SML:
  - SML #1 JSON-RPC: `192.168.2.213:3248`
  - SML #2 REST: `192.168.2.248:8080`

## Important Routes

- Documents:
  - `/bills` → purchase orders
  - `/sales-orders` → sales orders
  - `/sale-invoices` → sales invoices
- Operational review:
  - `/marketplace-aliases` → `สินค้ารอยืนยัน`
- Settings/data:
  - `/settings/channels`
  - `/settings/catalog`
  - `/settings/old-data`
  - `/settings/email`
  - `/logs`

## Current Behavior To Preserve

- Bill detail and bulk send must block SML send when:
  - item is unmapped/unconfirmed
  - `item_code`/`unit_code` missing
  - qty/price invalid
  - SML product no longer exists
- Marketplace matching order:
  - exact real SML SKU when marketplace SKU equals `sml_catalog.item_code`
  - confirmed marketplace alias by source + SKU/name
  - learned raw-name mapping
  - catalog candidates as suggestions only when uncertain
- User confirmation on item mapping should save marketplace alias and apply it to matching open bills.
- Catalog sync is manual-first. Do not add frequent background full sync by default.
- Missing SML products are marked inactive in BillFlow, not hard-deleted.
- Old bills use user-facing lifecycle terms:
  - `เก็บบิล`
  - `บิลที่เก็บแล้ว`
  - `กู้คืน`
  - `ลบบิล`
  - `ลบถาวร`

## Deploy Policy

- Shared backend/frontend guardrails can deploy to all three instances.
- Phase 2 sales/import UX deploys to main + Henna.
- Thaisunsport must remain Phase 1 unless the user explicitly asks to enable sales/import.
- Always verify feature flags after deploy.
- Do not edit customer credentials/config unless explicitly requested.

## Local Verification

Run before commit/deploy when relevant:

```bash
cd backend && go test ./...
cd frontend && npm run build
git diff --check
PYTHONPYCACHEPREFIX=/private/tmp/billflow-pycache python3 -m py_compile scripts/deploy.py
```

## Git Hygiene

- Do not revert user changes you did not make.
- Do not commit sample import files unless user explicitly asks:
  - `Order.all.20260401_20260430.xlsx`
  - `lazada.xlsx`
  - `tiktok_csv.csv`
  - `tiktok_excel.xlsx`
- Prefer two commits for release closeout:
  - app/code commit deployed to server
  - docs verification commit after deploy

## Next Phase

Shopee Open Platform API direct is the next planned phase. It should create local bills through the same review/SML retry pipeline as Shopee Excel. Start on `billflow`; deploy to main + Henna after UAT; skip Thaisunsport until Phase 2 is explicitly enabled.
