# BillFlow

ระบบช่วยพนักงานลดเวลาคีย์บิลจาก **วันละ 100+ บิล** ลงเหลือ **เกือบ 0**  
โดยใช้ AI extract ข้อมูลจากหลาย channel แล้วส่งเข้า ERP (SML) โดยอัตโนมัติ

---

## สารบัญ

1. [Overview](#1-overview)
2. [Tech Stack](#2-tech-stack)
3. [Server Info](#3-server-info)
4. [Architecture](#4-architecture)
5. [Quick Start (Local Dev)](#5-quick-start-local-dev)
6. [Environment Variables](#6-environment-variables)
7. [Database Schema](#7-database-schema)
8. [API Routes](#8-api-routes)
9. [Input Channels](#9-input-channels)
10. [AI Extraction](#10-ai-extraction)
11. [F1 — AI Learning Loop](#11-f1--ai-learning-loop)
12. [F2 — Anomaly Detection](#12-f2--anomaly-detection)
13. [F3 — Voice Input](#13-f3--voice-input)
14. [F4 — Daily AI Insights](#14-f4--daily-ai-insights)
15. [SML ERP Integration](#15-sml-erp-integration)
16. [File Import (Lazada/Shopee)](#16-file-import-lazadashopee)
17. [Background Jobs](#17-background-jobs)
18. [Role & Permissions](#18-role--permissions)
19. [Backup Strategy](#19-backup-strategy)
20. [Cloudflare Tunnel](#20-cloudflare-tunnel)
21. [Build Phases & Status](#21-build-phases--status)

---

## 1. Overview

| Input Channel | รายละเอียด | ประเภทบิล | Phase | สถานะ |
|---|---|---|---|---|
| LINE OA | รูปภาพ, PDF, text, voice | บิลขาย | Phase 3 | text ✅ / รูป+PDF+voice deployed ยังไม่ test |
| Email IMAP | attachment PDF/Excel/รูป | บิลขาย | Phase 5 | deployed, กำลัง test |
| Shopee Excel | Export จาก Shopee Seller Center | บิลขาย | Phase 4a | ✅ deployed (รอ SML 248 เปิด) |
| Lazada Excel | Export จาก Lazada | บิลขาย + บิลซื้อ | Phase 4b | รอไฟล์จากลูกค้า |


**Output:** สร้างบิลใน SML ERP ผ่าน JSON-RPC API + บันทึก log ลง PostgreSQL + แจ้ง admin ผ่าน LINE เมื่อเกิด error

---

## 2. Tech Stack

```
Backend:   Go 1.24  (Gin framework)
Frontend:  React + Vite + TypeScript
Database:  PostgreSQL 16
AI:        OpenRouter API (google/gemini-2.5-flash primary, gemini-flash-1.5 fallback)
LINE:      line-bot-sdk-go v8 (official)
Email:     IMAP polling (go-imap/v2)
Excel:     excelize v2.10.1
Deploy:    Docker Compose + Cloudflare Tunnel
```

---

## 3. Server Info

```
OS:      Ubuntu 24.04.4 LTS
Server:  192.168.2.109  (user: bosscatdog)
Docker:  29.3.0
SML API: http://192.168.2.213:3248
```

**Ports ที่ BillFlow ใช้ (ไม่ชนกับ project อื่นบน server)**

| Service | Port |
|---|---|
| billflow-backend | 8090 |
| billflow-frontend | 3010 |
| billflow-postgres | 5438 |

**Projects อื่นบน server ที่ห้ามกระทบ:** openclaw-admin (3000/5432), tcc (8080/5433), ledgioai (3004/5436), centrix (3002/5434)

---

## 4. Architecture

```
Cloudflare Tunnel
  api.your-domain.com → :8090  (backend)
  app.your-domain.com → :3010  (frontend)
         │
Go Backend (Gin) :8090
  ├── POST /webhook/line                    ← LINE OA events
  ├── POST /api/auth/login
  ├── GET  /api/bills                       ← รายการบิล
  ├── GET  /api/bills/:id
  ├── POST /api/bills/:id/retry             ← retry pending + failed
  ├── GET  /api/mappings
  ├── POST /api/mappings
  ├── PUT  /api/mappings/:id
  ├── POST /api/mappings/feedback           ← F1 human feedback
  ├── GET  /api/mappings/stats
  ├── GET  /api/dashboard/stats
  ├── GET  /api/dashboard/insights
  ├── POST /api/dashboard/insights/generate
  ├── GET  /api/logs                        ← Activity Log
  ├── POST /api/import/upload               ← Lazada Excel
  ├── GET  /api/settings/shopee-config      ← Shopee SML defaults
  ├── POST /api/import/shopee/preview       ← parse + dedup check
  ├── POST /api/import/shopee/confirm       ← ส่ง SML 248
  ├── GET  /api/settings/column-mappings/:platform
  └── PUT  /api/settings/column-mappings/:platform
         │
    ┌────┴────┐
PostgreSQL   External APIs
  :5438        OpenRouter, SML :3248, LINE API, IMAP
```

---

## 5. Quick Start (Local Dev)

### Prerequisites
- Docker + Docker Compose
- Go 1.24+ (สำหรับ local build)
- Node.js 22+ (สำหรับ frontend dev)

### 1. Clone & configure

```bash
git clone <repo>
cd billflow
cp .env.example .env
# แก้ไข .env ใส่ credentials จริง
```

### 2. Start with Docker Compose

```bash
# Production mode
docker compose up -d

# Development mode (hot reload)
docker compose -f docker-compose.yml -f docker-compose.dev.yml up
```

### 3. Verify

```bash
curl http://localhost:8090/health
# → {"status":"ok","env":"development"}

curl http://localhost:3010
# → React app
```

### Default admin account

```
Email:    admin@billflow.local
Password: admin1234
```

### Run integration tests

```bash
bash scripts/test.sh all localhost:8090
```

---

## 6. Environment Variables

```bash
# Server
PORT=8090
ENV=development

# Database
DATABASE_URL=postgres://billflow:password@localhost:5438/billflow
DB_USER=billflow
DB_PASSWORD=changeme_strong_password

# JWT
JWT_SECRET=your-secret-key-min-32-chars
JWT_EXPIRE_HOURS=24

# LINE OA — reissue ก่อนใช้ทุกครั้ง
LINE_CHANNEL_SECRET=
LINE_CHANNEL_ACCESS_TOKEN=
LINE_ADMIN_USER_ID=

# Email IMAP
# Gmail: IMAP_HOST=imap.gmail.com, IMAP_PORT=993, IMAP_PASSWORD=<App Password 16 หลัก>
# Outlook: IMAP_HOST=imap-mail.outlook.com, IMAP_PORT=993
# ⚠️ IMAP_POLL_INTERVAL ต้องไม่น้อยกว่า 5m (Gmail rate limit)
IMAP_HOST=imap.gmail.com
IMAP_PORT=993
IMAP_USER=billing@company.com
IMAP_PASSWORD=
IMAP_FILTER_FROM=
IMAP_FILTER_SUBJECT=PO,Purchase Order,ใบสั่งซื้อ
IMAP_POLL_INTERVAL=5m

# OpenRouter
OPENROUTER_API_KEY=sk-or-xxx
OPENROUTER_MODEL=google/gemini-2.5-flash
OPENROUTER_FALLBACK_MODEL=google/gemini-flash-1.5
OPENROUTER_AUDIO_MODEL=openai/whisper-1

# SML ERP #1 (LINE/Email — JSON-RPC)
SML_BASE_URL=http://192.168.2.213:3248
SML_ACCESS_MODE=sales

# SML ERP #2 (Shopee — REST saleinvoice)
SHOPEE_SML_URL=http://192.168.2.248:8080
SHOPEE_SML_GUID=SMLX
SHOPEE_SML_PROVIDER=SML1
SHOPEE_SML_CONFIG_FILE=SMLConfigSML1.xml
SHOPEE_SML_DATABASE=SMLPLOY
SHOPEE_SML_DOC_FORMAT=IV
SHOPEE_SML_CUST_CODE=       # รหัสลูกค้า Shopee ใน SML
SHOPEE_SML_SALE_CODE=       # รหัสพนักงานขาย
SHOPEE_SML_WH_CODE=         # รหัสคลัง (fallback)
SHOPEE_SML_SHELF_CODE=      # รหัสชั้นวาง (fallback)
SHOPEE_SML_UNIT_CODE=ถุง     # หน่วย fallback ⚠️ ต้องไม่ว่าง — SML reject unit_code=""
SHOPEE_SML_VAT_TYPE=0       # 0=แยกนอก, 1=รวมใน, 2=ศูนย์%
SHOPEE_SML_VAT_RATE=7
SHOPEE_SML_DOC_TIME=09:00

# Auto-confirm
AUTO_CONFIRM_THRESHOLD=0.85

# Cron
INSIGHT_CRON_HOUR=8
BACKUP_CRON_HOUR=0
INSIGHT_LINE_NOTIFY=true
DISK_WARN_PERCENT=90

# Frontend
VITE_API_URL=http://localhost:8090
```

---

## 7. Database Schema

```sql
users                  -- auth + roles
bills                  -- บิลทุกใบ (status: pending/confirmed/sent/failed/skipped)
bill_items             -- รายการสินค้าในบิล
mappings               -- raw_name → item_code/unit_code mapping
mapping_feedback       -- F1: human corrections
item_price_history     -- F2: avg/min/max price per item_code
daily_insights         -- F4: AI-generated daily summaries
platform_column_mappings -- Lazada/Shopee column name config (admin-configurable)
audit_logs             -- action log
```

Migration file: [backend/internal/database/migrations/001_init.sql](backend/internal/database/migrations/001_init.sql)

---

## 8. API Routes

### Auth

| Method | Path | Auth | Description |
|---|---|---|---|
| POST | `/api/auth/login` | — | Login, returns JWT |
| GET | `/api/auth/me` | JWT | Current user info |

### Bills

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/api/bills` | JWT | List bills (filter: status, source, date) |
| GET | `/api/bills/:id` | JWT | Bill detail with items |
| POST | `/api/bills/:id/retry` | admin/staff | Retry failed bill |

### Mappings

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/api/mappings` | JWT | List all mappings |
| POST | `/api/mappings` | admin/staff | Create mapping |
| PUT | `/api/mappings/:id` | admin/staff | Update mapping |
| DELETE | `/api/mappings/:id` | admin | Delete mapping |
| GET | `/api/mappings/stats` | JWT | F1 accuracy stats |
| POST | `/api/mappings/feedback` | admin/staff | Human correction (F1) |

### Import

| Method | Path | Auth | Description |
|---|---|---|---|
| POST | `/api/import/upload` | admin/staff | Upload Lazada Excel → preview bills |
| GET | `/api/settings/shopee-config` | JWT | Shopee SML defaults (pre-fill dialog) |
| POST | `/api/import/shopee/preview` | admin/staff | Parse Shopee Excel + dedup check |
| POST | `/api/import/shopee/confirm` | admin/staff | ส่ง orders → SML 248 saleinvoice |

### Logs

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/api/logs` | admin | Activity log (audit_logs table) |

### Settings

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/api/settings/status` | JWT | Connection status |
| GET | `/api/settings/column-mappings/:platform` | JWT | Column mapping config |
| PUT | `/api/settings/column-mappings/:platform` | admin | Update column mapping |

### Dashboard

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/api/dashboard/stats` | JWT | Stats overview |
| GET | `/api/dashboard/insights` | JWT | F4 AI insights list |
| POST | `/api/dashboard/insights/generate` | admin | Generate insight on-demand |

### Webhook

| Method | Path | Auth | Description |
|---|---|---|---|
| POST | `/webhook/line` | LINE Signature | LINE OA events |

---

## 9. Input Channels

### LINE OA — น้องบิล Chatbot (Phase 3)

```
Mode 1 — Conversational Sales (น้องบิล)  ← ✅ TESTED
────────────────────────────────────────────────────────────
ลูกค้าพูดคุยใน LINE → LINE webhook → verify signature → async
  AI วิเคราะห์ intent:
  - inquiry  → smartSearch SML catalog → แสดง 1-5 รายการ
  - เลือกรายการ → ถามจำนวน
  - พิมพ์จำนวน (รับ "3", "10 ถุง", "สิบถุง" ผ่าน AI ParseQty)
  - view_cart → แสดงตะกร้า
  - checkout  → ขอชื่อ+เบอร์ → สรุป → รอยืนยัน
  - ยืนยัน   → ส่ง SML → bill created → แจ้ง LINE
  Cart edit:
  - "ลบรายการที่ 2"           → ลบสินค้าชิ้นที่ 2 ออกจากตะกร้า ✅
  - "แก้จำนวนรายการที่ 1 เป็น 5" → เปลี่ยน qty ✅

Mode 2 — ส่งรูป/PDF PO โดยตรง  ← code deployed, ยังไม่ test
────────────────────────────────────────────────────────────
ลูกค้าส่งรูป/PDF → download LINE Content API → AI extract → SML

Mode 3 — Voice message (F3)  ← code deployed, ยังไม่ test
────────────────────────────────────────────────────────────
ลูกค้าส่ง voice → Whisper transcribe → text → Mode 1 pipeline
confidence ลด 0.1 อัตโนมัติ
```

### Email IMAP (Phase 5)

```
Background goroutine poll ทุก 5 นาที
→ filter ตาม IMAP_FILTER_FROM / IMAP_FILTER_SUBJECT
→ download attachment → AI pipeline (เหมือน LINE)
→ mark_read หลัง process (กัน process ซ้ำ)
→ error → LINE admin notify
```

**รองรับ email provider:**
- Gmail (ใช้ App Password — ดู [Section 22](#22-gmail-imap-setup-สำหรับติดตั้งที่ร้านลูกค้า))
- Outlook/Hotmail (`imap-mail.outlook.com:993`)
- IMAP ทั่วไป (port 993 TLS หรือ 143 STARTTLS)

**ข้อจำกัด Gmail:**
- poll ถี่เกินไป → Gmail block connection (`unexpected EOF`)
- ใช้ `IMAP_POLL_INTERVAL=5m` ขั้นต่ำ (อย่าใช้ 1m)

### File Import — Lazada/Shopee (Phase 4)

ดูรายละเอียดที่ [Section 16](#16-file-import-lazadashopee)

---

## 10. AI Extraction

**Model:** `google/gemini-2.5-flash` (fallback: `google/gemini-flash-1.5`)

**System Prompt Output Format:**
```json
{
  "doc_type": "sale | purchase",
  "customer_name": "string",
  "customer_phone": "string | null",
  "items": [
    { "raw_name": "string", "qty": 0, "unit": "string", "price": null }
  ],
  "total_amount": null,
  "note": null,
  "confidence": 0.95
}
```

- ถ้าข้อมูลไม่ชัดเจน → `confidence < 0.5`
- voice transcription → `confidence - 0.1`
- ไม่ใช่ JSON valid → retry ด้วย fallback model

---

## 11. F1 — AI Learning Loop

```
Match(rawName):
  1. Exact match      → confidence 1.0
  2. Fuzzy match (Levenshtein)
     + boost ถ้า usage_count สูง
     score >= 0.85  → auto map
     score 0.60-0.84 → needs_review
     score < 0.60   → unmapped

LearnFromFeedback(feedback):
  → INSERT/UPDATE mappings (source='ai_learned', confidence=1.0)
  → increment usage_count + update last_used_at

UpdatePriceHistory:
  → อัปเดต item_price_history ทุกครั้งที่ status='sent'
```

**Mapping ดู/แก้ได้ที่:** `/mappings` (Web UI)

---

## 12. F2 — Anomaly Detection

| Rule | Severity | เงื่อนไข |
|---|---|---|
| `price_zero` | block | ราคา = 0 |
| `qty_zero` | block | qty = 0 |
| `duplicate_bill` | block | same customer+items วันเดียวกัน |
| `price_too_high` | warn | > avg × 1.5 |
| `price_too_low` | warn | < avg × 0.5 |
| `qty_suspicious` | warn | > max_ever × 2 |
| `new_customer` | warn | ลูกค้าใหม่ |
| `new_item` | warn | สินค้าใหม่ |

**Auto-confirm ผ่านเมื่อ:**
- `final_confidence >= AUTO_CONFIRM_THRESHOLD` (default 0.85)
- ไม่มี block anomaly
- warn ไม่เกิน 1 รายการ

---

## 13. F3 — Voice Input

- LINE ส่ง audio message → download ทันที (มี expiry)
- ส่งไป OpenRouter Whisper → transcribe เป็น text
- ส่ง text ต่อไป AI extract pipeline
- voice > 60 วินาที → แจ้ง user ให้ส่งสั้นกว่า
- confidence ลด 0.1 อัตโนมัติสำหรับ voice

---

## 14. F4 — Daily AI Insights

**Cron 08:00 ทุกวัน** → generate + push LINE admin

```
ตัวอย่าง output:
📈 ยอดบิลสัปดาห์นี้สูงกว่าปกติ 23%
🏆 ปูนซีเมนต์ยังคงเป็นสินค้าขายดีอันดับ 1
⚠️ พบบิลราคาผิดปกติ 3 รายการรอ review
💡 ควรเพิ่ม stock CEM001 — ใช้ไปแล้ว 78%
```

สร้าง on-demand ได้ที่ `/dashboard` (admin) หรือ `POST /api/dashboard/insights/generate`

---

## 15. SML ERP Integration

### SML #1 — JSON-RPC (LINE OA + Email)

```
POST http://192.168.2.213:3248/api/sale_reserve
Headers:
  Content-Type: application/json
  mcp-access-mode: sales

Request body (JSON-RPC 2.0):
{
  "jsonrpc": "2.0", "id": 1, "method": "tools/call",
  "params": {
    "name": "create_sale_reserve",
    "arguments": {
      "contact_name": "...",
      "contact_phone": "...",
      "items": [{ "item_code": "...", "qty": 1, "unit_code": "...", "price": 100 }]
    }
  }
}

⚠️ response text เป็น JSON ซ้อนกัน → ต้อง parse 2 ชั้น
Success: {"success":true,"doc_no":"BS20260422XXXX","message":"create success"}
```

**Retry policy:** max 3 ครั้ง, backoff 1s/3s/5s  
**หลัง fail 3 ครั้ง:** `status='failed'` + LINE admin push notify

### SML #2 — REST API (Shopee)

```
Base URL: http://192.168.2.248:8080
Auth headers (ทุก request): guid, provider, configFileName, databaseName

1. Product lookup  ← ✅ CONFIRMED WORKING
GET /SMLJavaRESTService/v3/api/product/{sku}
Response (flat — ไม่มี nested):
  {"success":true,"data":{"code":"...","unit_standard":"ถุง","start_sale_unit":"ถุง",
                           "start_sale_wh":"WH-01","start_sale_shelf":"SH-01"}}
  {"success":true,"data":null}  ← ถ้าไม่พบ SKU ใน SML
⚠️ ถ้า data=null → ใช้ fallback SHOPEE_SML_UNIT_CODE / WH_CODE / SHELF_CODE จาก .env
⚠️ ต้องตั้ง SHOPEE_SML_UNIT_CODE ไว้เสมอ (เช่น "ถุง") เพราะ SML reject unit_code=""

2. Create saleinvoice  ← ✅ CONFIRMED WORKING
POST /SMLJavaRESTService/restapi/saleinvoice
{
  "doc_no": "<shopee_order_id>",   ← ใช้ order_id เป็น doc_no
  "doc_format_code": "INV",
  "doc_date": "2026-04-24",
  "cust_code": "AR00004",
  "is_permium": 0,        ← int, typo intentional (matches SML)
  "vat_type": 0,          ← 0=แยกนอก, 1=รวมใน, 2=ศูนย์%
  "details": [{           ← key ต้องเป็น "details" ไม่ใช่ "items"
    "item_code": "...",
    "unit_code": "ถุง",
    "wh_code": "WH-01",
    "shelf_code": "SH-01",
    "price_exclude_vat": ...,
    "sum_amount_exclude_vat": ...
  }]
}
```

**SML 248 config ที่ใช้งานจริง (confirmed 2026-04-24):**
```
guid=smlx  provider=SMLGOH  configFileName=SMLConfigSMLGOH.xml  databaseName=SML1_2026
doc_format=INV  cust_code=AR00004  wh_code=WH-01  shelf_code=SH-01
```

**SKU จริงใน SML 248:** CON-xxxxx (ถุง), STEEL-xxxxx (เส้น), PLUMB-xxxxx (ท่อน), ROOF-xxxxx (แผ่น)

**ดูรายละเอียด Shopee import:** [docs/shopee-import.md](docs/shopee-import.md)  
⚠️ SML 248 (192.168.2.248) ต้องเปิดเครื่องก่อนใช้งาน

---

## 16. File Import

### Shopee Excel Import (Phase 4a) ✅ Deployed

```
URL: /import/shopee

1. กด "เลือกไฟล์ Shopee" → ระบบ GET /api/settings/shopee-config
2. Config dialog popup (pre-filled จาก env) → ผู้ใช้ยืนยัน
3. เปิด file picker → เลือก .xlsx จาก Shopee Seller Center
4. POST /api/import/shopee/preview
   - parse Excel (columns ภาษาไทย hardcoded)
   - ตรวจ duplicate: SELECT FROM bills WHERE source='shopee' AND order_id=?
   - exclude สถานะ: "ที่ต้องจัดส่ง", "ยกเลิกแล้ว"
5. Preview table: เห็นทุก order, duplicate badge สีเหลือง
6. เลือก orders (non-duplicate pre-checked) → ยืนยัน
7. POST /api/import/shopee/confirm
   - pre-fetch product info จาก SML 248
   - POST saleinvoice ต่อ order (retry max 3)
   - save bill ลง DB
8. แสดง results: สำเร็จ X / ล้มเหลว Y
```

**Shopee column names (hardcoded — ไฟล์ Shopee Seller Center คงที่):**

| Field | Column Name |
|---|---|
| order_id | หมายเลขคำสั่งซื้อ |
| status | สถานะการสั่งซื้อ |
| order_date | วันที่ทำการสั่งซื้อ |
| product_name | ชื่อสินค้า |
| sku | เลขอ้างอิง SKU (SKU Reference No.) |
| price | ราคาขาย |
| qty | จำนวน |

**ดูรายละเอียด:** [docs/shopee-import.md](docs/shopee-import.md)

### Lazada Excel Import (Phase 4b) ⏳ รอไฟล์จากลูกค้า

Column mapping เก็บใน DB → admin แก้ได้จาก `/settings`  
(ไม่ hardcode เพราะ Lazada format อาจต่างกันตาม seller)

---

## 17. Background Jobs

| Job | Schedule | Description |
|---|---|---|
| Email Poller | ทุก 5 นาที | Poll IMAP + process attachments |
| Daily Insight | Cron 08:00 | F4 generate + LINE notify |
| Backup | Cron 00:00 | `pg_dump` → `backups/YYYYMMDD.sql` |
| Token Checker | ทุกอาทิตย์ | ตรวจ LINE token — แจ้งล่วงหน้า 7 วัน |
| Disk Monitor | ทุกวัน | แจ้ง admin ถ้า disk > `DISK_WARN_PERCENT` (90%) |

---

## 18. Role & Permissions

| Permission | admin | staff | viewer |
|---|---|---|---|
| ดู bills ทั้งหมด | ✅ | ✅ | ✅ |
| confirm pending bills | ✅ | ✅ | — |
| retry failed bills | ✅ | ✅ | — |
| import Excel files | ✅ | ✅ | — |
| จัดการ mappings | ✅ | ✅ | — |
| ดู dashboard + insights | ✅ | ✅ | ✅ |
| generate insight on-demand | ✅ | — | — |
| settings (LINE/IMAP/SML/columns) | ✅ | — | — |
| จัดการ users | ✅ | — | — |
| ดู audit log | ✅ | — | — |

---

## 19. Backup Strategy

```bash
# สร้าง backup ทุกวัน 00:00
docker exec billflow-postgres \
  pg_dump -U billflow billflow \
  > ~/billflow/backups/$(date +%Y%m%d).sql

# เก็บ 30 วัน
find ~/billflow/backups -mtime +30 -delete
```

Backup files อยู่ที่: `~/billflow/backups/`

---

## 20. Cloudflare Tunnel

```bash
# cloudflared binary อยู่ที่ ~/cloudflared

# 1. Install
sudo cp ~/cloudflared/cloudflared /usr/local/bin/
sudo chmod +x /usr/local/bin/cloudflared

# 2. Login + สร้าง tunnel
cloudflared tunnel login
cloudflared tunnel create billflow

# 3. Config: ~/.cloudflared/config.yml
tunnel: <TUNNEL_ID>
credentials-file: /home/bosscatdog/.cloudflared/<TUNNEL_ID>.json
ingress:
  - hostname: api.your-domain.com
    service: http://localhost:8090
  - hostname: app.your-domain.com
    service: http://localhost:3010
  - service: http_status:404

# 4. systemd service
cloudflared service install
sudo systemctl enable cloudflared
sudo systemctl start cloudflared

# LINE Webhook URL:
# https://api.your-domain.com/webhook/line
```

---

## 21. Build Phases & Status

| Phase | Description | Status |
|---|---|---|
| 0 | Server prep (Go install, disk cleanup, cloudflared) | ✅ Done |
| 1 | Foundation: Docker, DB migrations, JWT auth, Login UI | ✅ Done |
| 2 | Core AI pipeline: OpenRouter, MapperService (F1), AnomalyService (F2), SML client, WorkerPool | ✅ Done |
| 3 | LINE integration: chatbot text ✅, cart edit ✅, image/PDF/voice (deployed untested) | ✅ Partial |
| 4a | Shopee import: saleinvoice_client, shopee_import handler, ShopeeImport.tsx, routes wired | ✅ Deployed |
| 4b | Lazada import: Excel parser + Web UI | ⏳ รอไฟล์จากลูกค้า |
| 4 | File import: Lazada/Shopee Excel, column mapping editor | ⏳ Pending (รอไฟล์จากลูกค้า) |
| 5 | Email IMAP polling + attachment pipeline (Mistral OCR + retry handler) | ✅ Done |
| 6 | Web UI: Dashboard, Bills, BillDetail, Import, Mappings, Settings | ⏳ Not started |
| 7 | Background jobs: insight cron, backup cron, token checker, disk monitor | ⏳ Not started |
| 8 | Production: Cloudflare Tunnel, structured logging, health check | ⏳ Not started |

### Latest Test Results (2026-04-24)

```
LINE OA text flow (chatbot น้องบิล):
✅ ค้นหาสินค้า → เลือก → ใส่จำนวน → checkout → SML
✅ bill created: BS20260423101501-UELM (4,603.19 บาท)
✅ SML fail → LINE admin notify
✅ retry handler: re-map + re-send SML

Email IMAP pipeline:
✅ SASL PLAIN auth (Gmail App Password)
✅ Poll ทันทีตอน start + ทุก 5 นาที
✅ Inline PDF attachment detected (Mistral OCR)
✅ AI extract → mapper → anomaly → DB → SML
✅ Bill sent: BS20260423152412-6YRU
✅ Unmapped items → pending review + LINE notify
✅ เพิ่ม mapping ใหม่ + retry → SML สำเร็จ

Shopee Import (Phase 4a) — SML 248 ที่ 192.168.2.248:
✅ SML 248 REST API confirmed working (saleinvoice + product lookup)
✅ Product lookup: GET /SMLJavaRESTService/v3/api/product/{sku} (flat response)
✅ Create invoice: POST /SMLJavaRESTService/restapi/saleinvoice (key=details)
✅ Config: guid=smlx / SMLGOH / SMLConfigSMLGOH.xml / SML1_2026
✅ cust_code=AR00004, wh=WH-01, shelf=SH-01, doc_format=INV
⚠️ test file ใช้ SKU=REST-00002 ซึ่งไม่มีในSML 248
   → ต้องแก้ไฟล์ทดสอบให้ใช้ SKU จริง (CON-xxxxx / STEEL-xxxxx ฯลฯ)
⚠️ SHOPEE_SML_UNIT_CODE ต้องไม่ว่าง (ตั้งเป็น "ถุง" เป็น fallback)
⬜ end-to-end test กับ SKU จริง — ยังไม่เสร็จ

DB (2026-04-24): cleared สำหรับ clean test
Last successful bill: BS20260423101501-UELM (LINE OA, 4,603.19 บาท)

ยังไม่ test:
⬜ LINE: รูป/PDF/voice
⬜ Shopee import end-to-end (ต้องใช้ SKU จริงในไฟล์ Excel)
⬜ Lazada Excel import
```

---

## 22. Gmail IMAP Setup สำหรับติดตั้งที่ร้านลูกค้า

### ทำไมต้องใช้ App Password แทน password จริง

Google บังคับใช้ **2-Step Verification** สำหรับ IMAP โดยตรง → ต้องสร้าง App Password แยกต่างหาก  
ไม่ต้องผ่าน OAuth2 consent screen — เหมาะสำหรับ server-side automation

---

### ขั้นตอนสมัคร/ตั้งค่า Gmail สำหรับลูกค้า

#### ขั้นที่ 1 — เปิด 2-Step Verification (ถ้ายังไม่ได้เปิด)

1. เข้า [https://myaccount.google.com](https://myaccount.google.com)
2. เมนู **Security** → **2-Step Verification** → **Get started**
3. ทำตามขั้นตอน (ใช้ SMS หรือ Authenticator app)
4. ยืนยันว่า Status = **On**

#### ขั้นที่ 2 — สร้าง App Password

1. กลับไปที่ [https://myaccount.google.com/security](https://myaccount.google.com/security)
2. ค้นหา **App passwords** (หรือเข้าตรง [https://myaccount.google.com/apppasswords](https://myaccount.google.com/apppasswords))
3. กด **Create** → ตั้งชื่อ เช่น `BillFlow IMAP`
4. Google จะแสดง **password 16 หลัก** (ไม่มีช่องว่าง) เช่น `abcd efgh ijkl mnop`
5. **Copy ทันที** — แสดงครั้งเดียว ถ้าปิดหน้าต้อง generate ใหม่

#### ขั้นที่ 3 — เปิด IMAP ใน Gmail Settings

1. เปิด Gmail → **Settings** (ไอคอนฟัน) → **See all settings**
2. Tab **Forwarding and POP/IMAP**
3. **IMAP access** → เลือก **Enable IMAP**
4. กด **Save Changes**

#### ขั้นที่ 4 — ใส่ค่าใน `.env` บน server

```bash
IMAP_HOST=imap.gmail.com
IMAP_PORT=993
IMAP_USER=ชื่ออีเมล@gmail.com
IMAP_PASSWORD=abcdefghijklmnop    # App Password 16 หลัก ไม่มีช่องว่าง
IMAP_FILTER_FROM=                  # ใส่ email ผู้ส่งถ้าต้องการ filter
IMAP_FILTER_SUBJECT=PO,Purchase Order,ใบสั่งซื้อ
IMAP_POLL_INTERVAL=5m              # ห้ามน้อยกว่า 5m
```

```bash
# ทดสอบ connection จาก server
curl -v --ssl-reqd 'imaps://imap.gmail.com:993/INBOX' \
  --user 'ชื่ออีเมล@gmail.com:apppassword16หลัก' 2>&1 | head -20
# ต้องเห็น "* OK Gimap ready"
```

```bash
# restart backend หลังแก้ .env
cd ~/billflow && docker compose up -d backend

# ดู logs ยืนยัน
docker logs billflow-backend --tail=20 2>&1
# ไม่มี "IMAP poll failed" = OK
```

---

### ข้อควรระวัง Gmail IMAP

| ปัญหา | สาเหตุ | วิธีแก้ |
|---|---|---|
| `unexpected EOF` | poll ถี่เกินไป / Gmail rate limit | เพิ่ม `IMAP_POLL_INTERVAL=5m` |
| `IMAP login failed` | password ผิด / 2FA ไม่ได้เปิด | สร้าง App Password ใหม่ |
| `IMAP login failed` | ใส่ password จริงแทน App Password | ต้องใช้ App Password เท่านั้น |
| อ่านเมลซ้ำ | เมล mark as read ไม่สำเร็จ | ตรวจ IMAP permission ใน Google account |
| ไม่เจอเมล | `IMAP_FILTER_FROM` หรือ `IMAP_FILTER_SUBJECT` ไม่ตรง | ลอง clear filter ก่อน แล้วค่อย filter |

---

### ใช้ email อื่นแทน Gmail

| Provider | IMAP Host | Port | หมายเหตุ |
|---|---|---|---|
| Gmail | `imap.gmail.com` | 993 | ต้องใช้ App Password |
| Outlook/Hotmail | `imap-mail.outlook.com` | 993 | ใช้ password ปกติ หรือ App Password |
| Yahoo Mail | `imap.mail.yahoo.com` | 993 | ต้องสร้าง App Password |
| Zoho Mail | `imap.zoho.com` | 993 | ใช้ password ปกติ |
| บริษัทมี mail server | `mail.company.com` | 993 | ถาม IT admin |

---

## 23. เอกสารเพิ่มเติม

| ไฟล์ | เนื้อหา |
|---|---|
| [docs/overview.md](docs/overview.md) | ภาพรวมการทำงานทั้งระบบ (flow diagram + component map) |
| [docs/line-oa.md](docs/line-oa.md) | LINE OA workflow: chatbot, cart edit, รูป/PDF, voice |
| [docs/email.md](docs/email.md) | Email IMAP workflow: poll, dedup, OCR, extract, SML |
| [docs/shopee-import.md](docs/shopee-import.md) | Shopee Excel import → SML 248 saleinvoice |

---

## Project Structure

```
billflow/
├── backend/
│   ├── cmd/server/main.go
│   ├── internal/
│   │   ├── config/
│   │   ├── database/migrations/001_init.sql
│   │   ├── handlers/         (line, email, import, shopee_import, bills, mappings, dashboard, auth, log_handler)
│   │   ├── middleware/        (auth JWT, logger)
│   │   ├── models/            (bill, mapping, user, audit_log)
│   │   ├── repository/        (bill_repo [+DB()], mapping, user, audit_log_repo, insight, platform_mapping)
│   │   ├── services/
│   │   │   ├── ai/            (openrouter client + prompts)
│   │   │   ├── mapper/        (F1 fuzzy matching + learning)
│   │   │   ├── anomaly/       (F2 detection rules)
│   │   │   ├── sml/           (client.go JSON-RPC #1, saleinvoice_client.go REST #2)
│   │   │   ├── line/          (reply + push notify)
│   │   │   ├── email/         (IMAP polling + dedup by Message-ID)
│   │   │   └── insight/       (F4 daily AI summary)
│   │   ├── worker/pool.go     (semaphore rate limiting)
│   │   └── jobs/              (cron + background jobs)
│   ├── go.mod
│   └── Dockerfile
├── frontend/
│   └── src/
│       ├── pages/             (Login, Dashboard, Bills, BillDetail, Import, ShopeeImport, Logs, Mappings, Settings)
│       ├── components/
│       ├── hooks/
│       ├── api/client.ts
│       └── types/index.ts
├── scripts/test.sh
├── docker-compose.yml
├── docker-compose.dev.yml
├── .env.example
└── CLAUDE.md
```

---

## Deploy to Server

```bash
# Sync code
rsync -av --exclude 'node_modules' --exclude 'dist' --exclude '.git' \
  /Users/nontawatwongnuk/dev_bos/billflow/ \
  bosscatdog@192.168.2.109:~/billflow/

# Also sync go.sum (do not exclude)
rsync -av backend/go.sum bosscatdog@192.168.2.109:~/billflow/backend/go.sum

# Build + restart
ssh bosscatdog@192.168.2.109 \
  "cd ~/billflow && docker compose build backend frontend && docker compose up -d"

# Verify
curl http://192.168.2.109:8090/health
bash scripts/test.sh all 192.168.2.109:8090
```

---

*BillFlow v0.2.0 — Last updated: 2026-04-24 | Server: 192.168.2.109 | Ports: backend:8090 / frontend:3010 / postgres:5438*
