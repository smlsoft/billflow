# BillFlow — ภาพรวมการทำงาน

> อัพเดตล่าสุด: 2026-05-11 20:00 +07
> ดู snapshot จาก server จริงเพิ่มที่ [current-state.md](current-state.md)

---

## ระบบทำงานยังไง

BillFlow รับบิล/ออเดอร์จาก LINE OA, Email IMAP, Shopee Excel, Lazada Excel และ TikTok Excel/CSV แล้วช่วย admin ตรวจข้อมูลก่อนส่งเข้า SML ERP อัตโนมัติ จุดสำคัญของระบบตอนนี้คือ workflow แบบ human-in-the-loop: AI ช่วยอ่านเอกสารและจับคู่สินค้า แต่ admin ยังเห็นสถานะ, route, error, source artifact และกด Retry ได้จากหน้าเว็บ

สำหรับ customer test ปัจจุบัน BillFlow รองรับทั้ง Shopee email purchase flow และ Shopee Excel sale flow: ดึงข้อมูลเข้าเป็นเอกสาร local, review รายการสินค้า, เลือกลูกค้า/ผู้ขาย/คลัง/ภาษีก่อนส่ง, และส่งเข้า SML REST ตามเส้นทางเอกสารที่ตั้งไว้.

---

## Input → Process → Output

```
LINE OA / Email / Excel Upload
        │
        ▼
Ingest
  - LINE webhook: /webhook/line/:oaId หรือ /webhook/line
  - EmailCoordinator: one goroutine per enabled imap_accounts row
  - Import handlers: Lazada generic / Shopee/Lazada/TikTok preview+confirm
        │
        ▼
AI + Matching
  - OpenRouter text/image/audio extraction
  - Mistral OCR for PDFs
  - F1 mapper + SML catalog similarity candidates
  - F2 anomaly checks
        │
        ▼
Manual Review
  - /bills, /sales-orders, /sale-invoices
  - /bills/:id, /sales-orders/:id, /sale-invoices/:id
  - edit/add/delete items
  - map item, create product, inspect artifacts/timeline
  - route preview + validation guard before send
        │
        ▼
SML Retry Dispatch
  - sale_reserve  → SML #1 JSON-RPC 213
  - saleorder     → SML #2 REST 248 default sale route
  - saleinvoice   → SML #2 REST 248 saleinvoice v4 endpoint
  - purchaseorder → SML #2 REST 248 purchase route
        │
        ▼
PostgreSQL + Audit Logs + LINE admin notifications
```

---

## Component Map

```
billflow/
├── backend/cmd/server/main.go
│   ├── routes, handlers, services, cron jobs
│   └── migrations auto-run from backend/internal/database/migrations
│
├── LINE OA Human Inbox
│   ├── handlers/line.go                 webhook, media download, conversation writes
│   ├── handlers/chat_inbox.go           /api/admin/conversations
│   ├── services/line/registry.go        multi-OA token/secret registry
│   ├── /messages                        admin inbox
│   ├── /settings/line-oa                multi-OA config
│   └── /settings/quick-replies, /settings/chat-tags
│
├── Email Pipeline
│   ├── services/email/coordinator.go    per-account pollers
│   ├── services/email/imap.go           connect/search/fetch/mark seen
│   ├── handlers/email.go                attachment AI pipeline
│   └── /settings/email                  IMAP account admin UI
│
├── Import
│   ├── handlers/import.go               generic Lazada WIP
│   └── handlers/shopee_import.go        Shopee preview/confirm into local bills
│
├── SML + Catalog
│   ├── services/sml/client.go           SML #1 JSON-RPC sale_reserve
│   ├── saleorder_client.go              SML #2 saleorder default
│   ├── saleinvoice_client.go            SML #2 saleinvoice v4
│   ├── purchaseorder_client.go          SML #2 purchaseorder
│   ├── product_client.go, party_client.go
│   └── services/catalog                 embeddings + in-memory cosine index
│
└── Web UI
    ├── /dashboard, /bills, /bills/:id, /logs
    ├── /messages
    ├── /import, /import/shopee
    └── /settings/*
```

---

## Current Routes ที่ควรรู้

| Area | Routes |
|---|---|
| Health | `GET /health` |
| Auth | `POST /api/auth/login`, `GET /api/auth/me` |
| Bills | `GET /api/bills`, `GET /api/bills/:id`, `POST /api/bills/:id/retry`, item CRUD, timeline, artifact preview/download |
| Chat inbox | `/api/admin/conversations...` |
| LINE OA | `POST /webhook/line/:oaId`, `POST /webhook/line`, `/api/settings/line-oa...` |
| SSE | `POST /api/admin/events/token`, `GET /api/admin/events?t=...` |
| Email settings | `/api/settings/imap-accounts...` |
| Channel defaults | `/api/settings/channel-defaults...` |
| Catalog | `/api/catalog...` |
| Imports | `/api/import/upload`, `/api/import/confirm`, `/api/import/shopee/preview`, `/api/import/shopee/confirm` |
| Logs | `GET /api/logs` |

---

## Background Jobs

| Job | เวลา/Trigger | หน้าที่ |
|---|---|---|
| EmailCoordinator | per `imap_accounts.poll_interval_seconds`, min 300s | poll IMAP, route general/Shopee/Shopee shipped |
| Daily Insight | `INSIGHT_CRON_HOUR`, default 08:00 | AI summary + optional LINE notify |
| Backup | `BACKUP_CRON_HOUR`, default 00:00 | `pg_dump` to `/app/backups` mounted as `~/billflow/backups` |
| Disk Monitor | daily 07:00 | LINE alert when disk usage exceeds threshold |
| LINE Token Checker | weekly | expiry reminder |
| Reply Token Cleanup | hourly | clear LINE reply tokens older than 1 hour |
| Tunnel Drift Monitor | daily 09:00 Bangkok | ping `PUBLIC_BASE_URL/health`, LINE alert if Cloudflare Quick Tunnel URL drifted |

---

## สถานะปัจจุบัน

| Channel | สถานะ |
|---|---|
| Shopee purchase email | ✅ Phase 1 customer-test focus; sends SML `purchaseorder` through `192.168.2.248:8080` |
| Email IMAP | ✅ multi-account DB-driven, Shopee email routing, artifacts, logs |
| LINE OA | ✅ code exists for human chat 2 ทาง, multi-OA, media, quick replies, status, notes, tags, create bill from chat; hidden/not central in Phase 1 |
| Shopee Excel | ✅ preview/dedup/create local bills; routes to `saleorder` or `saleinvoice` based on `/settings/channels` |
| Lazada Excel | ✅ local implementation for sale Excel: preview/dedup/create local bills; routes to `saleorder` or `saleinvoice` based on `/settings/channels`; deploy target is main + Henna |
| TikTok Excel/CSV | ✅ local-ready for sale Excel/CSV: preview/dedup/create local bills; routes to `saleorder` or `saleinvoice` based on `/settings/channels`; deploy target is main + Henna |

## Current Document Menus

| กลุ่ม Sidebar | Menu | URL | SML route |
|---|---|---|---|
| งานฝั่งซื้อ | ใบสั่งซื้อ | `/bills` | `purchaseorder` |
| งานฝั่งขาย | ใบสั่งขาย | `/sales-orders` | `saleorder` |
| งานฝั่งขาย | ขายสินค้าและบริการ | `/sale-invoices` | `saleinvoice` |

ทั้ง 3 เมนูมีปุ่ม `ส่ง SML ทั้งหมด` สำหรับเอกสารสถานะพร้อมส่ง (`pending`) โดยมี preview/validation ก่อนส่งจริง.

---

## เอกสารอื่น

| ไฟล์ | เนื้อหา |
|---|---|
| [current-state.md](current-state.md) | snapshot จาก code + server + production DB |
| [deploy-instances.md](deploy-instances.md) | registry port/folder/container/tunnel ของแต่ละร้าน |
| [phase1-test-checklist.md](phase1-test-checklist.md) | checklist สำหรับทดสอบ Phase 1 ก่อน demo/customer test |
| [line-oa.md](line-oa.md) | LINE OA human inbox |
| [email.md](email.md) | Email IMAP pipeline |
| [shopee-import.md](shopee-import.md) | Shopee Excel import |
| [phase1-guide.md](phase1-guide.md) | คู่มือใช้งาน Phase 1 |
| [README.md](../README.md) | setup, API, deploy notes |
| [AGENTS.md](../AGENTS.md) | blueprint สำหรับ Codex |
