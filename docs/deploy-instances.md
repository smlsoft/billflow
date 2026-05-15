# BillFlow Deploy Instances

Registry สำหรับ folder, ports, containers, tunnels และ deploy policy บน server `192.168.2.109`.

> ใช้ Cloudflare Quick Tunnel (`trycloudflare.com`) เป็นหลัก URL อาจเปลี่ยนเมื่อ restart `cloudflared`; อ่าน URL ล่าสุดจาก tunnel log ของ instance นั้น.

## Current Instances

| Instance | Purpose | Server folder | Frontend | Backend | PostgreSQL | Current frontend asset | Tunnel log |
| --- | --- | --- | ---: | ---: | ---: | --- | --- |
| `billflow` | demo หลัก / ทดสอบงานใหม่ | `/home/bosscatdog/billflow` | `3010` | `8090` | `5438` | `index-ZO1PU287.js` | `/tmp/billflow-tunnel.log` |
| `billflow-henna` | Henna customer trial | `/home/bosscatdog/billflow-henna` | `3030` | `8110` | `5458` | `index-ZO1PU287.js` | `/tmp/billflow-henna-tunnel.log` |
| `billflow-thaisunsport` | Thaisunsport Phase 1 demo | `/home/bosscatdog/billflow-thaisunsport` | `3020` | `8100` | `5448` | `index-PGhS5G_e.js` | `/tmp/billflow-thaisunsport-tunnel.log` |

## Latest Verified Deploy

- Time verified: 2026-05-15 14:40 +07
- Deployed app commit: `be7734e Add catalog lifecycle and review queue UX`
- Change type: shared backend/frontend/data safety + UX handoff cleanup
- Deploy targets: all three instances
- Verification:
  - backend health ok on `8090`, `8110`, `8100`
  - frontend key routes HTTP 200 on `3010`, `3030`, `3020`
  - migration `036_sml_catalog_lifecycle.sql` applied on all three instances
  - `sml_catalog.is_active`, `sml_catalog.missing_at`, `sml_catalog.last_seen_at` exist in all three DBs
  - main/Henna remain Phase 2 sales-enabled; Thaisunsport remains Phase 1 sales-disabled

## Feature Flags

| Instance | `VITE_PHASE` | Sales | Shopee Excel | Lazada Excel | TikTok Excel | Chat |
| --- | ---: | --- | --- | --- | --- | --- |
| `billflow` | `2` | true | true | true | true | false |
| `billflow-henna` | `2` | true | true | true | true | false |
| `billflow-thaisunsport` | `1` | false | false | false | false | false |

## Deploy Policy

- Use the same codebase for every instance.
- Keep instance differences in `.env`, Docker Compose build args, database data, and per-instance settings.
- Shared bugfixes, guardrails, `/bills`, email, logs, catalog, and old-data changes may deploy to all three instances.
- Phase 2 sales/marketplace features should be tested on `billflow`, then deployed to `billflow` + `billflow-henna`.
- `billflow-thaisunsport` must remain Phase 1 purchase-only unless the user explicitly asks to enable Phase 2.
- Server folders are deployed copies, not git checkouts.

## Quick Checks

```bash
curl http://192.168.2.109:8090/health   # billflow
curl http://192.168.2.109:8110/health   # Henna
curl http://192.168.2.109:8100/health   # Thaisunsport
```

```bash
docker ps --format '{{.Names}} {{.Status}}' | grep billflow
```

```bash
grep -oE 'https://[a-z0-9-]+\.trycloudflare\.com' /tmp/billflow-tunnel.log | tail -1
grep -oE 'https://[a-z0-9-]+\.trycloudflare\.com' /tmp/billflow-henna-tunnel.log | tail -1
grep -oE 'https://[a-z0-9-]+\.trycloudflare\.com' /tmp/billflow-thaisunsport-tunnel.log | tail -1
```

## Container Names

| Instance | Frontend | Backend | PostgreSQL |
| --- | --- | --- | --- |
| `billflow` | `billflow-frontend` | `billflow-backend` | `billflow-postgres` |
| `billflow-henna` | `billflow-henna-frontend` | `billflow-henna-backend` | `billflow-henna-postgres` |
| `billflow-thaisunsport` | `billflow-thaisunsport-frontend` | `billflow-thaisunsport-backend` | `billflow-thaisunsport-postgres` |

## Port Pattern For New Customer Instances

| Instance order | Frontend | Backend | PostgreSQL |
| ---: | ---: | ---: | ---: |
| Main | `3010` | `8090` | `5438` |
| Customer 1 | `3020` | `8100` | `5448` |
| Customer 2 | `3030` | `8110` | `5458` |
| Customer 3 | `3040` | `8120` | `5468` |

Use folder/container/volume names with the customer slug, e.g. `billflow-henna-*`.

## Notes

- Henna was created from the normal BillFlow instance, not from Thaisunsport.
- Thaisunsport is a Phase 1 purchase-flow demo and should not show sales/import menus.
- Shopee API direct is the next planned phase; start on `billflow`, then main + Henna after UAT, and skip Thaisunsport until Phase 2 is explicitly enabled.
