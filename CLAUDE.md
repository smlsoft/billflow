# CLAUDE.md — BillFlow
## Blueprint สำหรับ Claude Code ใน VSCode

> อ่านไฟล์นี้ให้ครบก่อนเริ่ม code ทุกครั้ง
> ห้าม assume สิ่งที่ไม่ได้ระบุ — ถามก่อนทำเสมอ
> **project folder: `~/billflow`**

---

## 1. Project Overview

**BillFlow** — ระบบช่วยพนักงานลดเวลาคีย์บิลจาก **วันละ 100+ บิล** ลงเหลือ **เกือบ 0**
โดยใช้ AI extract ข้อมูลจากหลาย channel แล้วส่งเข้า ERP (SML) โดยอัตโนมัติ

### Input Channels
| Channel | รายละเอียด | ประเภทบิล | Phase | สถานะ |
|---------|-----------|----------|-------|-------|
| LINE OA | รูปภาพ, PDF, text, voice | บิลขาย (sale) | Phase 3 | text ✅ / รูป+PDF+voice deployed แต่ยังไม่ test |
| Email (IMAP) | attachment PDF/Excel/รูป | บิลขาย (sale) | Phase 5 | deployed, กำลัง test |
| Shopee Excel | Export จาก Shopee Seller Center | บิลขาย (sale) | Phase 4a | ✅ deployed (รอ SML 224 เปิด) |
| Lazada Excel | Export จาก Lazada | บิลขาย + บิลซื้อ | Phase 4b | รอไฟล์จริงจากลูกค้า |

> ⚠️ ใช้ IMAP แทน Gmail API สำหรับ demo — ง่ายกว่า ไม่ต้องผ่าน Google OAuth2 consent
> Gmail API อาจเพิ่มใน Phase ถัดไปหลัง demo ลูกค้าแล้ว

### Output
- สร้างบิลใน SML ERP ผ่าน JSON-RPC API
- บันทึก log ทุก transaction ลง PostgreSQL
- แจ้ง admin ผ่าน LINE ทุกครั้งที่เกิด error

---

## 2. Tech Stack

```
Backend:   Go 1.22+  (framework: Gin)
Frontend:  React + Vite + TypeScript
Database:  PostgreSQL 16
AI:        OpenRouter API (google/gemini-flash-1.5 default)
LINE:      line-bot-sdk-go (official)
Email:     IMAP polling — ไม่ใช้ Gmail API สำหรับ demo
Deploy:    Docker Compose + Cloudflare Tunnel
```

---

## 3. Server Info (Ubuntu 24.04 — 192.168.2.109)

```
OS:      Ubuntu 24.04.4 LTS (kernel 6.17.0)
CPU:     4 cores
RAM:     7.6GB total / ~4GB available
Disk:    109GB (เคลียร์แล้ว < 70%)
Docker:  29.3.0
Node:    v22.22.1 ✅
Go:      1.24.0 ✅
cloudflared: ✅ installed & configured
SML #1 (LINE/Email): http://192.168.2.213:3248  ✅ JSON-RPC, /api/sale_reserve
SML #2 (Shopee):     http://192.168.2.224:8080  ⚠️ REST API — ยังไม่ได้เปิด machine
```

### Projects ที่รันอยู่บน Server — ห้ามกระทบ

| Project | Ports |
|---------|-------|
| openclaw-admin | 3000, 5432 |
| tcc | 8080, 5433, 9092, 8123, 9000, 6382 |
| ledgioai | 3004, 5436, 6381 |
| centrix | 3002, 5001, 5434, 6380 |

### Ports ที่ BillFlow ใช้ (ไม่ชนกับใคร)

```
billflow-backend   → 8090
billflow-frontend  → 3010
billflow-postgres  → 5438
```

---

## 4. Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Cloudflare Tunnel                        │
│   api.your-domain.com → :8090  (backend)                   │
│   app.your-domain.com → :3010  (frontend)                  │
└────────────────────────┬────────────────────────────────────┘
                         │
┌────────────────────────▼────────────────────────────────────┐
│                  Go Backend (Gin)  :8090                    │
│                                                             │
│  Webhook Routes:                                            │
│    POST /webhook/line              ← LINE OA events         │
│                                                             │
│  API Routes (JWT protected unless noted):                   │
│    POST /api/auth/login            ← login (no auth)        │
│    GET  /api/auth/me               ← current user           │
│                                                             │
│    Bills (manual-confirm flow):                             │
│    GET  /api/bills                 ← list (status/source/   │
│                                       bill_type filter)     │
│    GET  /api/bills/:id             ← detail + items         │
│    POST /api/bills/:id/retry       ← 3-way SML send         │
│                                       (sale_reserve /       │
│                                        saleinvoice /        │
│                                        purchaseorder)       │
│    PUT  /api/bills/:id/items/:iid  ← edit qty/price/code    │
│                                       (also fires F1 hook)  │
│    POST /api/bills/:id/items       ← add row                │
│    DEL  /api/bills/:id/items/:iid  ← delete row             │
│                                                             │
│    Mappings + F1:                                           │
│    GET  /api/mappings              ← list (manual + ai_*)   │
│    POST /api/mappings              ← add                    │
│    PUT  /api/mappings/:id          ← edit (3-field)         │
│    DEL  /api/mappings/:id          ← delete                 │
│    POST /api/mappings/feedback     ← legacy F1 endpoint     │
│    GET  /api/mappings/stats        ← F1 accuracy stats      │
│                                                             │
│    Catalog (SML 248 product catalog + embeddings):          │
│    GET  /api/catalog               ← paginated list         │
│    GET  /api/catalog/stats         ← embed progress         │
│    GET  /api/catalog/search?q=     ← embedding similarity   │
│    GET  /api/catalog/:code         ← item detail            │
│    POST /api/catalog/products      ← create new SML product │
│                                       + sync + embed        │
│    POST /api/catalog/sync          ← bulk sync from SML 248 │
│    POST /api/catalog/import-csv    ← bulk CSV upload        │
│    POST /api/catalog/embed-all     ← background batch embed │
│    POST /api/catalog/reload-index  ← rebuild memory index   │
│    POST /api/catalog/:code/embed   ← embed single           │
│    POST /api/bills/:id/items/:iid/confirm-match  ← legacy   │
│                                                             │
│    Imports:                                                 │
│    POST /api/import/upload         ← generic Lazada (WIP)   │
│    POST /api/import/confirm        ← generic Lazada confirm │
│    GET  /api/settings/shopee-config← Shopee SML defaults    │
│    POST /api/import/shopee/preview ← parse Excel + dedup    │
│    POST /api/import/shopee/confirm ← send to SML 248        │
│                                                             │
│    Dashboard / Logs / Settings:                             │
│    GET  /api/dashboard/stats                                │
│    GET  /api/dashboard/insights                             │
│    POST /api/dashboard/insights/generate (admin)            │
│    GET  /api/logs                                           │
│    GET  /api/settings/status                                │
│    GET  /api/settings/column-mappings/:platform             │
│    PUT  /api/settings/column-mappings/:platform (admin)     │
│                                                             │
│    Email Inboxes (admin only):                              │
│    GET  /api/settings/imap-accounts        ← list          │
│    POST /api/settings/imap-accounts        ← create        │
│    GET  /api/settings/imap-accounts/:id    ← detail        │
│    PUT  /api/settings/imap-accounts/:id    ← update        │
│    DEL  /api/settings/imap-accounts/:id    ← delete        │
│    POST /api/settings/imap-accounts/:id/poll ← manual poll │
│    POST /api/settings/imap-accounts/test   ← dry connect   │
│    POST /api/settings/imap-accounts/list-folders ← enum   │
│                                                             │
│    Webhook:                                                 │
│    POST /webhook/line              ← LINE OA events         │
│                                                             │
│  Background Jobs:                                           │
│    EmailCoordinator → one goroutine per imap_accounts row  │
│                    each polls its mailbox every             │
│                    poll_interval_seconds (≥ 300 s)         │
│                    per-account channel routing:             │
│                    "ถูกจัดส่งแล้ว" / "ยืนยันการชำระเงิน"  │
│                       → ShopeeShipped (purchaseorder 248)  │
│                    other Shopee → saleinvoice (248)        │
│                    general     → attachment AI pipeline     │
│                    LINE admin notify on ≥ 3 consecutive    │
│                    failures (throttled 1/hour per inbox)   │
│    Cron 08:00    → F4 daily insight + LINE notify          │
│    Cron 00:00    → pg_dump backup (in-container, gzip)     │
│    Cron Mon 09   → LINE token expiry reminder              │
│    Cron daily 07 → disk usage monitor (root fs > 90%)      │
│                                                             │
│  Services:                                                  │
│    AIService         → OpenRouter (text/image/PDF/audio)   │
│    MapperService     → F1 fuzzy match + auto-learn loop    │
│    AnomalyService    → F2 anomaly (incl. new_customer)     │
│    SML Client        → JSON-RPC sale_reserve (213)         │
│    SML Invoice       → REST saleinvoice (248)              │
│    SML PurchaseOrd   → REST purchaseorder (248)            │
│    SML Product       → REST product create/lookup (248)    │
│    LineService       → reply / flex / push notify          │
│    EmailCoordinator  → multi-account IMAP (one goroutine   │
│                        per imap_accounts row)              │
│    InsightService    → F4 daily AI summary                 │
│    Catalog           → embed (1536-dim) + cosine index     │
│    WorkerPool        → semaphore rate limiting             │
└──────┬──────────────────────────────┬───────────────────────┘
       │                              │
┌──────▼──────┐              ┌────────▼────────┐
│ PostgreSQL  │              │  External APIs  │
│   :5438     │              │                 │
│             │              │ OpenRouter API  │
│ tables:     │              │ SML :3248       │
│  bills      │              │ LINE API        │
│  bill_items │              │ IMAP server     │
│  mappings   │              └─────────────────┘
│  mapping_   │
│  feedback   │
│  users      │
│  audit_logs │
│  daily_     │
│  insights   │
│  item_price_│
│  history    │
└─────────────┘
       ▲
┌──────┴──────────────────────────────────────────────────────┐
│             React + Vite Frontend  :3010                    │
│                                                             │
│  /login            ← หน้า login                            │
│  /dashboard        ← stats + charts + F4 AI Insights       │
│  /bills            ← รายการบิล + filter + anomaly badge    │
│  /bills/:id        ← รายละเอียด + status + retry           │
│  /import           ← upload Lazada/Shopee                  │
│  /mappings         ← จัดการ mapping + F1 learning stats    │
│  /settings         ← LINE, SML, threshold, columns         │
│  /settings/email   ← Email Inboxes (admin only)            │
└─────────────────────────────────────────────────────────────┘
```

---

## 5. Database Schema (PostgreSQL)

```sql
-- Users & Auth
CREATE TABLE users (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  email         TEXT UNIQUE NOT NULL,
  name          TEXT NOT NULL,
  role          TEXT NOT NULL CHECK (role IN ('admin','staff','viewer')),
  password_hash TEXT NOT NULL,
  created_at    TIMESTAMPTZ DEFAULT NOW()
);

-- Item Mapping
CREATE TABLE mappings (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  raw_name      TEXT NOT NULL,
  item_code     TEXT NOT NULL,
  unit_code     TEXT NOT NULL,
  confidence    FLOAT DEFAULT 1.0,
  source        TEXT DEFAULT 'manual' CHECK (source IN ('manual','ai_learned')),
  usage_count   INT DEFAULT 0,
  last_used_at  TIMESTAMPTZ,
  learned_from_bill_id UUID,
  created_by    UUID REFERENCES users(id),
  created_at    TIMESTAMPTZ DEFAULT NOW(),
  UNIQUE(raw_name)
);

-- F1: Mapping Feedback
CREATE TABLE mapping_feedback (
  id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  bill_item_id   UUID REFERENCES bill_items(id),
  original_match TEXT,
  corrected_to   TEXT,
  corrected_by   UUID REFERENCES users(id),
  created_at     TIMESTAMPTZ DEFAULT NOW()
);

-- Bills
CREATE TABLE bills (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  bill_type     TEXT NOT NULL CHECK (bill_type IN ('sale','purchase')),
  source        TEXT NOT NULL CHECK (source IN ('line','email','lazada','shopee','manual')),
  status        TEXT NOT NULL DEFAULT 'pending'
                  CHECK (status IN ('pending','confirmed','sent','failed','skipped')),
  raw_data      JSONB,
  sml_doc_no    TEXT,
  sml_payload   JSONB,
  sml_response  JSONB,
  ai_confidence FLOAT,
  anomalies     JSONB DEFAULT '[]',
  error_msg     TEXT,
  created_by    UUID REFERENCES users(id),
  created_at    TIMESTAMPTZ DEFAULT NOW(),
  sent_at       TIMESTAMPTZ
);

-- Bill Items
CREATE TABLE bill_items (
  id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  bill_id    UUID REFERENCES bills(id) ON DELETE CASCADE,
  raw_name   TEXT NOT NULL,
  item_code  TEXT,
  qty        NUMERIC NOT NULL,
  unit_code  TEXT,
  price      NUMERIC,
  mapped     BOOLEAN DEFAULT FALSE,
  mapping_id UUID REFERENCES mappings(id)
);

-- Audit Log
CREATE TABLE audit_logs (
  id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id    UUID REFERENCES users(id),
  action     TEXT NOT NULL,
  target_id  UUID,
  detail     JSONB,
  created_at TIMESTAMPTZ DEFAULT NOW()
);

-- F2: Item Price History
CREATE TABLE item_price_history (
  item_code    TEXT PRIMARY KEY,
  avg_price    NUMERIC,
  min_price    NUMERIC,
  max_price    NUMERIC,
  sample_count INT DEFAULT 0,
  last_updated TIMESTAMPTZ DEFAULT NOW()
);

-- F4: Daily AI Insights
CREATE TABLE daily_insights (
  id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  date       DATE UNIQUE NOT NULL,
  stats_json JSONB,
  insight    TEXT,
  created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Platform Column Mapping (Lazada/Shopee — admin config)
CREATE TABLE platform_column_mappings (
  id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  platform     TEXT NOT NULL CHECK (platform IN ('lazada','shopee')),
  field_name   TEXT NOT NULL,   -- 'order_id', 'buyer_name', 'item_name', etc.
  column_name  TEXT NOT NULL,   -- ชื่อ column จริงในไฟล์ Excel
  updated_by   UUID REFERENCES users(id),
  updated_at   TIMESTAMPTZ DEFAULT NOW(),
  UNIQUE(platform, field_name)
);

-- Chat Sessions (LINE conversation state — persistent across restart)
CREATE TABLE chat_sessions (
  line_user_id   TEXT PRIMARY KEY,
  history        JSONB NOT NULL DEFAULT '[]',  -- last ~20 messages
  pending_order  JSONB,                        -- cart state
  last_active    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- SML Catalog (smart matching — Shopee email + manual review)
-- ทำงานเป็น in-memory cosine-similarity index (1536-dim vectors)
CREATE TABLE sml_catalog (
  item_code        TEXT PRIMARY KEY,
  item_name        TEXT NOT NULL,
  item_name2       TEXT NOT NULL DEFAULT '',
  unit_code        TEXT NOT NULL DEFAULT '',
  wh_code          TEXT NOT NULL DEFAULT '',
  shelf_code       TEXT NOT NULL DEFAULT '',
  price            NUMERIC(14,4),
  group_code       TEXT NOT NULL DEFAULT '',
  balance_qty      NUMERIC(14,4),
  embedding_status TEXT NOT NULL DEFAULT 'pending'
                   CHECK (embedding_status IN ('pending','done','error')),
  embedded_at      TIMESTAMPTZ,
  embedding        JSONB,           -- text-embedding-3-small (OpenRouter)
  embedding_model  TEXT,
  synced_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Channel Customer Defaults (default cust_code per channel)
CREATE TABLE channel_customer_defaults (
  channel     TEXT PRIMARY KEY
              CHECK (channel IN ('line','email','shopee','lazada')),
  cust_code   TEXT NOT NULL,
  cust_name   TEXT NOT NULL,
  cust_phone  TEXT NOT NULL DEFAULT '',
  updated_by  UUID,
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- IMAP Accounts — multi-account email config (replaces .env IMAP_* singleton)
-- Admin manages via /settings/email UI. One goroutine per enabled row.
CREATE TABLE imap_accounts (
  id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name                  TEXT NOT NULL,
  host                  TEXT NOT NULL,
  port                  INT NOT NULL DEFAULT 993,
  username              TEXT NOT NULL,
  password              TEXT NOT NULL,                  -- plaintext, LAN-only
  mailbox               TEXT NOT NULL DEFAULT 'INBOX',
  filter_from           TEXT NOT NULL DEFAULT '',
  filter_subjects       TEXT[] NOT NULL DEFAULT '{}',
  channel               TEXT NOT NULL DEFAULT 'general'
                        CHECK (channel IN ('general','shopee','lazada')),
  shopee_domains        TEXT[] NOT NULL DEFAULT '{}',
  lookback_days         INT NOT NULL DEFAULT 30
                        CHECK (lookback_days BETWEEN 1 AND 90),
  poll_interval_seconds INT NOT NULL DEFAULT 300
                        CHECK (poll_interval_seconds >= 300),
  enabled               BOOLEAN NOT NULL DEFAULT TRUE,
  -- runtime status (updated by coordinator after each poll)
  last_polled_at        TIMESTAMPTZ,
  last_poll_status      TEXT CHECK (last_poll_status IN ('ok','error')),
  last_poll_error       TEXT,
  last_poll_messages    INT,
  consecutive_failures  INT NOT NULL DEFAULT 0,
  last_admin_alert_at   TIMESTAMPTZ,
  created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

> **Migration files** (run in order, all idempotent):
> - [001_init.sql](backend/internal/database/migrations/001_init.sql) — initial schema (users, bills, bill_items, mappings, mapping_feedback, item_price_history, daily_insights, platform_column_mappings, audit_logs, chat_sessions)
> - [002_audit_logging.sql](backend/internal/database/migrations/002_audit_logging.sql) — audit_logs structured columns (source, level, duration_ms, trace_id) + indexes
> - [002_sml_catalog.sql](backend/internal/database/migrations/002_sml_catalog.sql) — sml_catalog table + bills.sml_order_id + bill_items.candidates + extended source/status CHECK
> - [003_channel_customer_defaults.sql](backend/internal/database/migrations/003_channel_customer_defaults.sql) — channel_customer_defaults table
> - [004_shopee_shipped.sql](backend/internal/database/migrations/004_shopee_shipped.sql) — extends bills.source CHECK to include shopee_shipped
> - [006_imap_accounts.sql](backend/internal/database/migrations/006_imap_accounts.sql) — imap_accounts table (multi-account IMAP, replaces .env singleton)

---

## 6. Use Cases (ละเอียด)

### UC1 — LINE OA รับ PO

> ✅ DEPLOYED — ทดสอบ text flow ผ่านแล้ว (2026-04-23)
> ⚠️ image/PDF/voice มี code แต่ยังไม่ได้ test

```
Mode 1 — Conversational Sales (น้องบิล chatbot) ← IMPLEMENTED & TESTED
──────────────────────────────────────────────────────────
1. ลูกค้าพูดคุย เช่น "มีปูนขายไหม" "มีเหล็กขายด้วยไหม"
2. LINE ส่ง webhook POST /webhook/line
3. verify X-Line-Signature (HMAC-SHA256)
4. respond HTTP 200 ทันที → process async
5. AI (ChatSalesV2) วิเคราะห์ intent:
   - inquiry  → smartSearch SML catalog → แสดง 1-5 รายการให้เลือก
   - ลูกค้าเลือก "รายการที่ 2" → ถามจำนวน
   - ลูกค้าพิมพ์จำนวน (รับ "3", "10 ถุง", "สิบถุง" ผ่าน AI ParseQty)
   - view_cart → แสดงตะกร้า
   - checkout  → ขอชื่อ+เบอร์ → สรุป → รอ ยืนยัน
   - ยืนยัน    → ส่ง SML → bill created → แจ้ง LINE

Mode 2 — ส่งรูป/PDF PO โดยตรง ← code มีแต่ยังไม่ test
──────────────────────────────────────────────────────────
1. ลูกค้าส่งรูปภาพ/PDF ใบสั่งซื้อ
2. download จาก LINE Content API
3. AIService (ExtractImage/ExtractPDF) → JSON
4. สร้าง bill → SML

Mode 3 — Voice message (F3) ← code มีแต่ยังไม่ test
──────────────────────────────────────────────────────────
1. ลูกค้าส่ง voice message
2. download ทันที (มี expiry)
3. TranscribeAudio → text → ส่งต่อเหมือน Mode 1
4. confidence ลดลง 0.1

Error Cases:
- SML fail → failed + แจ้ง LINE admin ทันที
- ไม่พบสินค้า → "ขอโทษค่ะ ไม่พบสินค้า \"X\" ในระบบค่ะ"
- voice > 60 วินาที → แจ้งให้ส่งสั้นกว่านี้

Open items:
- [ ] ลบรายการออกจาก cart (ยังไม่มี — ต้องพิมพ์ยกเลิกแล้วเริ่มใหม่)
- [ ] แก้ไขจำนวนใน cart
- [ ] test รูปภาพ PO
- [ ] test PDF attachment
- [ ] test voice message
```

### UC2 — Email IMAP (multi-account)

```
Config: admin จัดการ inbox ได้ที่ /settings/email (EmailCoordinator อ่าน imap_accounts จาก DB)
        ไม่มี IMAP_* ใน .env อีกต่อไป — ทุกอย่างอยู่ใน DB

Flow (ต่อ account):
1. EmailCoordinator spawn goroutine ต่อ row ที่ enabled=true
2. ทุก poll_interval_seconds → SELECT unseen messages จาก mailbox
3. filter ตาม filter_from / filter_subjects ของ account นั้น
4. per-account channel routing:
   - subject matches "ถูกจัดส่งแล้ว" หรือ "ยืนยันการชำระเงิน"
       → ProcessShopeeShippedEmailBody → purchaseorder (SML 248)
   - from domain ใน shopee_domains && channel=shopee
       → ProcessShopeeEmailBody → saleinvoice (SML 248)
   - else → download attachment → AI pipeline → pending/needs_review
5. mark_read หลัง process (กัน process ซ้ำ)
6. consecutive_failures ≥ 3 → LINE admin notify (throttle 1/hour per inbox)

Production inboxes (2026-04-28):
  "Shopee Inbox" — bos.catdog@gmail.com — channel=shopee
    filter_subjects: คำสั่งซื้อ, ถูกจัดส่งแล้ว
  "Shopee Inbox (sutee — pay-now)" — sutee.toe@gmail.com — channel=shopee
    filter_subjects: ยืนยันการชำระเงินคำสั่งซื้อหมายเลข #
```

### UC3 — Import Lazada/Shopee (บิลขาย)

```
Flow:
1. พนักงาน login → /import → upload Excel
2. Backend ดึง column mapping จาก DB (ไม่ hardcode)
3. parse + แสดง preview + anomaly warning
4. พนักงาน confirm → loop สร้างบิล → SML
5. สรุปผล: สำเร็จ X / ล้มเหลว Y + download error report

⚠️ รอไฟล์จริงจากลูกค้าก่อน Phase 4
   admin config column mapping ได้จาก /settings
```

### UC4 — Import Lazada/Shopee (บิลซื้อ)

```
เหมือน UC3 แต่ bill_type = 'purchase'
SML endpoint อาจต่างกัน — รอ confirm จาก SML doc
```

---

## 7. AI Extraction — System Prompt

```go
// backend/internal/ai/prompts.go

const ExtractPrompt = `
คุณเป็น AI ที่ช่วย extract ข้อมูลจากใบสั่งซื้อ (Purchase Order)
ตอบเป็น JSON เท่านั้น ห้ามมีข้อความอื่น ห้ามมี markdown

output format:
{
  "doc_type": "sale" | "purchase",
  "customer_name": string,
  "customer_phone": string | null,
  "items": [
    {
      "raw_name": string,
      "qty": number,
      "unit": string,
      "price": number | null
    }
  ],
  "total_amount": number | null,
  "note": string | null,
  "confidence": number
}

ถ้าข้อมูลไม่ชัดเจน ให้ confidence ต่ำ (< 0.5)
ถ้าข้อมูลมาจาก voice transcription ให้ confidence ลดลง 0.1
`
```

---

## 8. F1 — AI Learning Loop

```go
func (s *MapperService) Match(rawName string) MatchResult {
  // 1. Exact match → confidence 1.0
  // 2. Fuzzy match (levenshtein)
  //    boost score ถ้า usage_count สูง
  //    score >= 0.85 → auto map
  //    score 0.60-0.84 → needs_review
  //    score < 0.60 → unmapped
}

func (s *MapperService) LearnFromFeedback(f MappingFeedback) error {
  // human confirm → INSERT หรือ UPDATE
  // source='ai_learned', confidence=1.0
  // increment usage_count + update last_used_at
}

// อัปเดต item_price_history ทุกครั้ง status='sent'
func (s *MapperService) UpdatePriceHistory(items []BillItem) error {}
```

---

## 9. F2 — Anomaly Detection

```go
var AnomalyRules = []AnomalyRule{
  // block — บังคับ manual confirm
  {"price_zero",      "block"},  // ราคา = 0
  {"qty_zero",        "block"},  // qty = 0
  {"duplicate_bill",  "block"},  // same customer+items วันเดียวกัน

  // warn — แสดง badge แต่ไม่ block อัตโนมัติ
  {"price_too_high",  "warn"},   // > avg * 1.5
  {"price_too_low",   "warn"},   // < avg * 0.5
  {"qty_suspicious",  "warn"},   // > max_ever * 2
  {"new_customer",    "warn"},   // ลูกค้าใหม่
  {"new_item",        "warn"},   // สินค้าใหม่
}

// Auto-confirm ผ่านเมื่อ:
// final_confidence >= threshold
// AND ไม่มี block
// AND warn ไม่เกิน 1 รายการ
```

---

## 10. SML API Integration

### SML #1 — JSON-RPC (LINE OA + Email)
```go
// POST http://192.168.2.213:3248/api/sale_reserve
// Headers: Content-Type: application/json
//          mcp-access-mode: sales

// Request
{
  "jsonrpc": "2.0", "id": 1, "method": "tools/call",
  "params": {
    "name": "create_sale_reserve",
    "arguments": {
      "contact_name": string,
      "contact_phone": string,
      "items": [{ "item_code": string, "qty": number,
                  "unit_code": string, "price": number }]
    }
  }
}

// ⚠️ response text เป็น JSON string ซ้อนกัน ต้อง parse 2 ชั้น
// Success: {"success":true,"doc_no":"BS20260422XXXX","message":"create success"}

// Retry: max 3 ครั้ง, backoff 1s/3s/5s
// หลัง fail 3 ครั้ง → status='failed' + LINE admin push notify
```

### SML #2 — REST API (Shopee)
```go
// Base URL: http://192.168.2.248:8080
// Auth headers (ทุก request):
//   guid, provider, configFileName, databaseName

// 1. Product lookup  ← CONFIRMED WORKING (2026-04-24)
// GET /SMLJavaRESTService/v3/api/product/{sku}
// Response (flat — ไม่มี nested):
//   {"success":true,"data":{"code":"...","unit_standard":"ถุง",
//                           "start_sale_unit":"ถุง","start_sale_wh":"WH-01",
//                           "start_sale_shelf":"SH-01"}}
//   {"success":true,"data":null}  ← ถ้าไม่พบ SKU ใน SML
// ⚠️ ต้องตั้ง SHOPEE_SML_UNIT_CODE เป็น fallback (เช่น "ถุง") เมื่อ data=null

// 2. Create saleinvoice  ← CONFIRMED WORKING (2026-04-24)
// POST /SMLJavaRESTService/restapi/saleinvoice
// {
//   "doc_format_code": "INV",
//   "doc_date": "2026-04-24",
//   "cust_code": "AR00004",
//   "is_permium": 0,         ← int (ไม่ใช่ bool), typo intentional
//   "vat_type": 0,           // 0=แยกนอก, 1=รวมใน, 2=ศูนย์%
//   "details": [             ← key ต้องเป็น "details" ไม่ใช่ "items"
//     {
//       "item_code": string,
//       "unit_code": string,
//       "wh_code": "WH-01",
//       "shelf_code": "SH-01",
//       "price_exclude_vat": number,
//       "sum_amount_exclude_vat": number
//     }
//   ]
// }

// Client: backend/internal/services/sml/saleinvoice_client.go
// Retry: max 3 ครั้ง
// Config จริง: guid=smlx / SMLGOH / SMLConfigSMLGOH.xml / SML1_2026
// SKU จริงใน SML 248: CON-xxxxx (ถุง), STEEL-xxxxx (เส้น), PLUMB-xxxxx (ท่อน)

// 3. Create purchaseorder  ← CONFIRMED WORKING (2026-04-27)
// POST /SMLJavaRESTService/v3/api/purchaseorder
// Same payload shape as saleinvoice, except:
//   - field "buy_type" instead of "sale_type"
//   - cust_code semantically = supplier
//   - ⚠️ doc_no MUST be non-empty (unlike saleinvoice). The v3 endpoint
//     does NOT auto-generate. Sending null/empty triggers ic_trans NOT NULL.
// Client: backend/internal/services/sml/purchaseorder_client.go
// doc_no convention: "BF-PO-YYYYMMDD-{8-char bill UUID prefix}" generated
// in bills.go retryPurchaseOrder. SML response data.doc_no often empty;
// fall back to the request doc_no in that case.
// Used by: shopee_shipped emails (handlers/shipped_email.go).

// 4. Create product  ← CONFIRMED WORKING (2026-04-27)
// POST /SMLJavaRESTService/v3/api/product
// {
//   "code": "TEST-001",            ← user-supplied
//   "name": "ทดสอบ",
//   "tax_type": 0, "item_type": 0, "unit_type": 1,
//   "unit_cost": "ชิ้น", "unit_standard": "ชิ้น",
//   "purchase_point": 0,
//   "units": [{"unit_code":"ชิ้น","unit_name":"ชิ้น","stand_value":1,"divide_value":1}],
//   "price_formulas": [{"unit_code":"ชิ้น","sale_type":0,"price_0":"99.5",
//                       "tax_type":0,"price_currency":0}]
// }
// Response: {"success":true,"data":{"code":"..."}} (SML may return
// a different code than requested — use response code as canonical).
// Client: backend/internal/services/sml/product_client.go
// Wired to BillFlow: POST /api/catalog/products handler upserts into
// sml_catalog with status='pending' + triggers background embedding.
// Used by: BillDetail's MapItemModal "+ สร้างสินค้าใหม่" form.
```

---

## 11. Worker Pool

```go
// backend/internal/worker/pool.go
// จำกัด concurrent requests ป้องกัน spike

type WorkerPool struct {
  openrouterSem chan struct{}  // max 5 concurrent
  smlSem        chan struct{}  // max 3 concurrent
}
// LINE webhook respond 200 ก่อนเสมอ → job เข้า pool
```

---

## 12. LINE Admin Notifications

```go
// push message (ไม่ใช่ reply) ไปหา LINE_ADMIN_USER_ID
// กรณีที่แจ้ง:
// - SML fail หลัง retry 3 ครั้ง
// - bill anomaly block ถูก reject
// - IMAP connection fail
// - disk usage > 90%
// - LINE token จะหมดอายุใน 7 วัน
// - F4 daily insight สร้างเสร็จ (08:00)
```

---

## 13. F3 — Voice Input

```go
// handlers/line.go
case linebot.MessageTypeAudio:
  audioData, _ := s.lineService.DownloadContent(msg.ID)
  text, _ := s.aiService.TranscribeAudio(audioData)
  // ส่ง text ต่อไป extract pipeline

// ⚠️ audio มี expiry → download ทันที
// voice > 60 วินาที → แจ้ง user
// confidence ลดลง 0.1 สำหรับ voice
```

---

## 14. F4 — Daily AI Insights

```go
// Cron 08:00 ทุกวัน
const InsightPrompt = `
คุณเป็น business analyst สรุปข้อมูลธุรกิจเป็นภาษาไทย
กระชับ 3-5 ประโยค ใช้ emoji นำหน้าแต่ละประเด็น
ข้อมูลวันนี้ vs สัปดาห์ที่แล้ว: %s
สรุป: trend / สินค้าขายดี-แย่ / บิลมีปัญหา / คำแนะนำ
`
// ตัวอย่าง output:
// 📈 ยอดบิลสัปดาห์นี้สูงกว่าปกติ 23%
// 🏆 ปูนซีเมนต์ยังคงเป็นสินค้าขายดีอันดับ 1
// ⚠️ พบบิลราคาผิดปกติ 3 รายการรอ review
// 💡 ควรเพิ่ม stock CEM001 — ใช้ไปแล้ว 78%
```

---

## 15. Backup Strategy

```bash
# Cron 00:00 ทุกวัน
docker exec billflow-postgres \
  pg_dump -U billflow billflow \
  > ~/billflow/backups/$(date +%Y%m%d).sql

# เก็บ 30 วัน
find ~/billflow/backups -mtime +30 -delete

# สร้าง folder ก่อน
mkdir -p ~/billflow/backups
```

---

## 16. Project Structure

```
~/billflow/
│
├── backend/
│   ├── cmd/server/main.go
│   ├── internal/
│   │   ├── config/config.go
│   │   ├── database/
│   │   │   ├── postgres.go
│   │   │   └── migrations/001_init.sql
│   │   ├── handlers/
│   │   │   ├── line.go              ← LINE webhook + chatbot + cart edit
│   │   │   ├── email.go             ← general email handler + dedup
│   │   │   ├── shipped_email.go     ← Shopee shipped/pay-now → purchaseorder
│   │   │   ├── import.go            ← Lazada import
│   │   │   ├── shopee_import.go     ← Shopee Excel → SML 248 saleinvoice
│   │   │   ├── bills.go             ← bill CRUD + retry (3-way routed)
│   │   │   ├── mappings.go
│   │   │   ├── dashboard.go
│   │   │   ├── log_handler.go       ← GET /api/logs
│   │   │   ├── imap_settings.go     ← /api/settings/imap-accounts CRUD ✨NEW
│   │   │   └── auth.go
│   │   ├── middleware/
│   │   │   ├── auth.go
│   │   │   └── logger.go
│   │   ├── models/
│   │   │   ├── bill.go
│   │   │   ├── mapping.go
│   │   │   ├── user.go
│   │   │   ├── audit_log.go
│   │   │   └── imap_account.go      ← ImapAccount model ✨NEW
│   │   ├── services/
│   │   │   ├── ai/
│   │   │   │   ├── openrouter.go
│   │   │   │   └── prompts.go
│   │   │   ├── mapper/mapper.go        ← F1
│   │   │   ├── anomaly/detector.go     ← F2
│   │   │   ├── sml/
│   │   │   │   ├── client.go                ← SML #1 JSON-RPC
│   │   │   │   ├── saleinvoice_client.go    ← SML #2 REST saleinvoice
│   │   │   │   ├── purchaseorder_client.go  ← SML #2 REST purchaseorder
│   │   │   │   └── product_client.go        ← SML #2 REST product CRUD
│   │   │   ├── line/service.go         ← reply + push notify
│   │   │   ├── email/
│   │   │   │   ├── coordinator.go      ← starts one goroutine per account ✨NEW
│   │   │   │   ├── account.go          ← per-account poll loop ✨NEW
│   │   │   │   ├── processors.go       ← Shopee/general message routing ✨NEW
│   │   │   │   ├── folders.go          ← list-folders helper ✨NEW
│   │   │   │   └── imap.go             ← stateless PollOnce (refactored)
│   │   │   └── insight/service.go      ← F4
│   │   ├── worker/pool.go
│   │   ├── jobs/
│   │   │   ├── insight_cron.go
│   │   │   ├── backup_cron.go
│   │   │   ├── token_checker.go
│   │   │   └── disk_monitor.go
│   │   │   (email_poller.go — DELETED; replaced by EmailCoordinator)
│   │   └── repository/
│   │       ├── bill_repo.go
│   │       ├── mapping_repo.go
│   │       ├── user_repo.go
│   │       ├── audit_log_repo.go
│   │       └── imap_account_repo.go   ← CRUD + status update ✨NEW
│   │   ├── models/  (see models/ above)
│   ├── go.mod
│   ├── Dockerfile
│   └── .air.toml
│
├── frontend/
│   ├── src/
│   │   ├── pages/
│   │   │   ├── Login.tsx
│   │   │   ├── Dashboard.tsx
│   │   │   ├── Bills.tsx
│   │   │   ├── BillDetail/              ← decomposed from 1234-line monolith ✨NEW
│   │   │   │   ├── index.tsx
│   │   │   │   ├── components/          ← BillHeader, BillItems, BillTotal, etc.
│   │   │   │   ├── hooks/
│   │   │   │   └── utils/formatters.ts  ← SOURCE_LABELS, FLOW_META, etc.
│   │   │   ├── Import.tsx
│   │   │   ├── ShopeeImport.tsx
│   │   │   ├── Logs.tsx
│   │   │   ├── Mappings.tsx
│   │   │   ├── Settings.tsx
│   │   │   ├── EmailAccounts.tsx        ← /settings/email admin page ✨NEW
│   │   │   ├── EmailAccounts/
│   │   │   │   └── AccountDialog.tsx    ← add/edit inbox dialog ✨NEW
│   │   │   └── Showcase.tsx             ← dev-only component gallery
│   │   ├── components/
│   │   │   ├── layout/
│   │   │   │   ├── Sidebar.tsx          ← redesigned sidebar (shadcn) ✨NEW
│   │   │   │   └── Topbar.tsx           ← topbar + breadcrumbs ✨NEW
│   │   │   ├── common/                  ← shared primitives ✨NEW
│   │   │   │   ├── StatusDot.tsx
│   │   │   │   ├── KeyboardShortcut.tsx
│   │   │   │   ├── EmptyState.tsx
│   │   │   │   ├── PageHeader.tsx
│   │   │   │   ├── JsonViewer.tsx
│   │   │   │   ├── DataTable.tsx
│   │   │   │   ├── LoadingSkeleton.tsx
│   │   │   │   ├── ConfirmDialog.tsx
│   │   │   │   ├── ThemeToggle.tsx
│   │   │   │   └── TagInput.tsx
│   │   │   ├── CommandPalette.tsx       ← ⌘K palette ✨NEW
│   │   │   ├── ui/                      ← 25 shadcn/ui primitives ✨NEW
│   │   │   ├── BillTable.tsx
│   │   │   ├── BillStatusBadge.tsx
│   │   │   ├── AnomalyBadge.tsx
│   │   │   ├── InsightCard.tsx
│   │   │   ├── LearningProgress.tsx
│   │   │   ├── FileUploader.tsx
│   │   │   └── StatsCard.tsx
│   │   ├── hooks/
│   │   │   ├── useAuth.ts
│   │   │   ├── useBills.ts
│   │   │   └── useHotkeys.ts            ← two-key chord hotkeys ✨NEW
│   │   ├── lib/                         ← shared utilities ✨NEW
│   │   │   ├── utils.ts                 ← cn() helper (clsx + tailwind-merge)
│   │   │   ├── theme.ts                 ← Zustand dark/light theme store
│   │   │   ├── breadcrumbs.tsx          ← route → breadcrumb label map
│   │   │   └── ui-store.ts              ← command palette open state
│   │   ├── api/client.ts
│   │   ├── store/auth.ts
│   │   └── types/index.ts
│   ├── package.json
│   ├── tailwind.config.ts               ← Tailwind 3 + shadcn theme ✨NEW
│   ├── postcss.config.js                ✨NEW
│   ├── vite.config.ts
│   └── Dockerfile
│
├── backups/
├── docker-compose.yml
├── docker-compose.dev.yml
├── .env.example
├── .gitignore
└── CLAUDE.md
```

---

## 17. Environment Variables

```bash
# .env.example

PROJECT_NAME=billflow

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

# LINE OA
# ⚠️ credentials ด้านล่างนี้ถูก expose แล้ว — REISSUE ก่อนใช้
# LINE Developer Console → Messaging API → Reissue token/secret
LINE_CHANNEL_SECRET=
LINE_CHANNEL_ACCESS_TOKEN=
LINE_ADMIN_USER_ID=    ← User ID ของ admin สำหรับ push notify

# Email IMAP — managed via /settings/email (DB-driven, multi-account)
# ไม่มี IMAP_* env vars อีกต่อไป — admin เพิ่ม inbox ผ่าน UI ได้เลย

# OpenRouter
OPENROUTER_API_KEY=sk-or-xxx
OPENROUTER_MODEL=google/gemini-2.5-flash
OPENROUTER_FALLBACK_MODEL=google/gemini-flash-1.5   ← ไม่ใช้ Claude Haiku แล้ว (ราคาแพงกว่า Gemini)
OPENROUTER_AUDIO_MODEL=openai/whisper-1

# SML ERP #1 (LINE/Email — JSON-RPC)
SML_BASE_URL=http://192.168.2.213:3248
SML_ACCESS_MODE=sales

# SML ERP #2 (Shopee — REST saleinvoice)
SHOPEE_SML_URL=http://192.168.2.224:8080
SHOPEE_SML_GUID=SMLX
SHOPEE_SML_PROVIDER=SML1
SHOPEE_SML_CONFIG_FILE=SMLConfigSML1.xml
SHOPEE_SML_DATABASE=SMLPLOY
SHOPEE_SML_DOC_FORMAT=IV
SHOPEE_SML_CUST_CODE=           ← รหัสลูกค้า Shopee ใน SML
SHOPEE_SML_SALE_CODE=           ← รหัสพนักงานขาย
SHOPEE_SML_WH_CODE=             ← รหัสคลัง (fallback)
SHOPEE_SML_SHELF_CODE=          ← รหัสชั้นวาง (fallback)
SHOPEE_SML_UNIT_CODE=           ← หน่วย (fallback)
SHOPEE_SML_VAT_TYPE=0           ← 0=แยกนอก, 1=รวมใน, 2=ศูนย์%
SHOPEE_SML_VAT_RATE=7
SHOPEE_SML_DOC_TIME=09:00

# Shopee shipped → SML purchaseorder (reuses all SHOPEE_SML_* above)
# Only doc_format and cust_code differ. cust_code falls back to SHOPEE_SML_CUST_CODE if blank.
# ⚠️ For shipped emails to flow through IMAP, IMAP_FILTER_SUBJECT must include "ถูกจัดส่งแล้ว"
SHIPPED_SML_DOC_FORMAT=PO
SHIPPED_SML_CUST_CODE=

# Mistral OCR — required for PDF extraction (model: mistral-ocr-2512)
MISTRAL_API_KEY=

# Shopee email domains — now per-account in imap_accounts.shopee_domains (DB)

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

## 18. Docker Compose

```yaml
# docker-compose.yml
# ⚠️ ทุก container ชื่อ billflow-* ไม่กระทบ project อื่น

services:
  postgres:
    image: postgres:16-alpine
    container_name: billflow-postgres
    volumes:
      - billflow_pgdata:/var/lib/postgresql/data
    environment:
      POSTGRES_DB: billflow
      POSTGRES_USER: ${DB_USER}
      POSTGRES_PASSWORD: ${DB_PASSWORD}
    ports:
      - "5438:5432"
    mem_limit: 512m
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U ${DB_USER}"]
      interval: 10s
      retries: 5
    restart: unless-stopped

  backend:
    build: ./backend
    container_name: billflow-backend
    ports:
      - "8090:8090"
    env_file: .env
    depends_on:
      postgres:
        condition: service_healthy
    mem_limit: 512m
    restart: unless-stopped

  frontend:
    build: ./frontend
    container_name: billflow-frontend
    ports:
      - "3010:80"
    depends_on:
      - backend
    mem_limit: 256m
    restart: unless-stopped

volumes:
  billflow_pgdata:
```

```yaml
# docker-compose.dev.yml
services:
  backend:
    volumes:
      - ./backend:/app
    command: air
  frontend:
    volumes:
      - ./frontend:/app
    command: npm run dev
    ports:
      - "5173:5173"
```

---

## 19. Cloudflare Tunnel

```bash
# cloudflared มีอยู่แล้วใน ~/cloudflared

# 1. install binary
sudo cp ~/cloudflared/cloudflared /usr/local/bin/
sudo chmod +x /usr/local/bin/cloudflared
cloudflared --version

# 2. login + สร้าง tunnel
cloudflared tunnel login
cloudflared tunnel create billflow

# 3. ~/.cloudflared/config.yml
tunnel: <TUNNEL_ID>
credentials-file: /home/bosscatdog/.cloudflared/<TUNNEL_ID>.json
ingress:
  - hostname: api.your-domain.com
    service: http://localhost:8090
  - hostname: app.your-domain.com
    service: http://localhost:3010
  - service: http_status:404

# 4. install systemd service
cloudflared service install
sudo systemctl enable cloudflared
sudo systemctl start cloudflared

# LINE webhook URL:
# https://api.your-domain.com/webhook/line
```

---

## 20. Role & Permissions

```
admin:
  - ดู/แก้ไข bills ทั้งหมด + retry
  - จัดการ mappings + ดู F1 learning stats
  - จัดการ users
  - settings (LINE, IMAP, SML, threshold, column mapping)
  - generate F4 insight on-demand
  - ดู audit log

staff:
  - ดู bills ทั้งหมด
  - confirm pending bills
  - import files
  - เพิ่ม/แก้ไข mappings

viewer:
  - ดู bills (read-only)
  - ดู dashboard + insights
```

---

## 21. Libraries

### Backend (Go)
```
github.com/gin-gonic/gin              ← HTTP framework
github.com/golang-jwt/jwt/v5          ← JWT
github.com/lib/pq                     ← PostgreSQL driver
github.com/line/line-bot-sdk-go/v8    ← LINE official SDK
github.com/emersion/go-imap/v2        ← IMAP client
github.com/xuri/excelize/v2           ← Excel parser
github.com/lithammer/fuzzysearch      ← fuzzy matching
github.com/joho/godotenv              ← .env loader
go.uber.org/zap                       ← structured logging
github.com/robfig/cron/v3             ← cron jobs
```

### Frontend (React + Vite)
```
react-router-dom         ← routing
zustand                  ← state management (with persist middleware)
axios                    ← HTTP client (shared client.ts with 401 interceptor)
recharts                 ← charts
react-dropzone           ← file upload (Lazada Import เท่านั้น)
sonner                   ← toast notifications (replaces react-hot-toast)
dayjs                    ← date formatting
cmdk                     ← ⌘K command palette primitive
vaul                     ← drawer primitive (shadcn)
tailwindcss @^3.4        ← utility-first CSS (Linear/Vercel style)
tailwindcss-animate      ← animation utilities
class-variance-authority ← component variant helper (shadcn)
clsx + tailwind-merge    ← cn() className utility
lucide-react             ← icon set (shadcn default)
@radix-ui/*              ← 25 shadcn/ui component primitives
```
> **Styling:** Tailwind 3 + shadcn/ui + HSL design tokens (dark mode ready)
> CSS bundle: 89 KB → 44 KB after migration
> **ทุก data fetching:** manual `useState`/`useEffect` (ไม่ใช้ @tanstack/react-query)
> **Hotkeys:** two-key chord system — g d (dashboard), g b (bills), g i (import),
>   g s (settings), g m (mappings), g l (logs), g c (catalog), g x (export)

---

## 22. ข้อควรระวัง

```
1. LINE Webhook
   - verify X-Line-Signature ทุก request
   - respond HTTP 200 ใน 1 วินาที → processing async เสมอ
   - ⚠️ credentials ที่แชร์ใน chat ถูก expose → REISSUE ก่อน

2. SML API
   - 192.168.2.213 อยู่ใน internal network เท่านั้น
   - response text เป็น JSON ซ้อนกัน → parse 2 ชั้น
   - retry max 3 → failed + LINE admin notify

3. OpenRouter
   - image → base64 หรือ URL
   - PDF → plugins: file-parser
   - ไม่ใช่ JSON → retry ด้วย fallback model

4. IMAP (multi-account — DB-driven)
   - จัดการ inbox ผ่าน /settings/email (admin only) — ไม่ต้องแก้ .env
   - DB CHECK enforces poll_interval_seconds >= 300 (5 min minimum)
   - mark_read หลัง process กัน process ซ้ำ
   - consecutive_failures >= 3 → LINE admin notify (throttle 1/hour per inbox)
   - Gmail ต้องใช้ App Password (ไม่ใช่ password จริง)
     → myaccount.google.com → Security → App passwords → Create
     → ต้องเปิด 2-Step Verification ก่อน
     → AccountDialog มี App Password popover ช่วย guide ตามประเภท host
   - Outlook: imap-mail.outlook.com:993 (App Password เช่นกัน)
   - Gmail rate limit: poll ถี่กว่า 5m → unexpected EOF
   - ลูกค้าเพิ่ม inbox ใหม่ได้เองจาก UI (test-connection + list-folders buttons)

5. Disk & Memory
   - เคลียร์ disk ก่อน Phase 1 (เป้าหมาย < 70%)
   - mem_limit ใน docker-compose กัน OOM
   - cron ตรวจ disk ทุกวัน → แจ้งถ้า > 90%

6. LINE Token
   - rotate ทุก 90 วัน
   - cron check รายสัปดาห์ → แจ้งล่วงหน้า 7 วัน

7. Shopee Column Mapping (hardcoded — ไฟล์จาก Shopee Seller Center คงที่)
   - หมายเลขคำสั่งซื้อ, สถานะการสั่งซื้อ, วันที่ทำการสั่งซื้อ
   - ชื่อสินค้า, เลขอ้างอิง SKU (SKU Reference No.), ราคาขาย, จำนวน
   - exclude: "ที่ต้องจัดส่ง", "ยกเลิกแล้ว"
   - dedup: ตรวจ bills WHERE source='shopee' AND raw_data->>'order_id' = ?

8. Lazada Column Mapping
   - ❌ ห้าม hardcode column names
   - ✅ เก็บใน DB → admin แก้ได้จาก /settings
   - รอไฟล์จริงจากลูกค้าก่อน Phase 4b

9. SML 248 Machine (Shopee)
   - เครื่อง 192.168.2.248 — ✅ CONFIRMED WORKING (2026-04-24)
   - Product lookup: curl "http://192.168.2.248:8080/SMLJavaRESTService/v3/api/product/CON-01000"
     -H "guid: smlx" -H "provider: SMLGOH" -H "configFileName: SMLConfigSMLGOH.xml" -H "databaseName: SML1_2026"
   - ⚠️ SKU ในไฟล์ test ต้องมีอยู่ใน ic_inventory ของ SML 248
     REST-00002 ไม่มีใน DB → ต้องใช้ CON-xxxxx / STEEL-xxxxx ฯลฯ
   - ⚠️ SHOPEE_SML_UNIT_CODE ต้องไม่ว่าง (ตั้ง "ถุง" เป็น fallback)
   - DB check: docker run --rm postgres:16-alpine psql
     'postgresql://postgres:sml@192.168.2.248:5432/sml1_2026'
     -c "SELECT code, unit_standard FROM ic_inventory LIMIT 10"
```

---

## 23. Build Phases

```
Phase 0 — Server Prep (ทำบน server ก่อน code บรรทัดแรก)
  [x] 0.1 เคลียร์ disk ✅
  [x] 0.2 ติดตั้ง Go 1.24.0 ✅
  [x] 0.3 Setup cloudflared ✅
  [x] 0.4 Reissue LINE credentials ✅
  [x] 0.5 mkdir -p ~/billflow/backups ✅
  [x] 0.6 verify: curl http://192.168.2.213:3248 ✅

Phase 1 — Foundation
  [x] 1.1 สร้าง ~/billflow/ structure + .gitignore ✅
  [x] 1.2 docker-compose.yml (ports 8090/3010/5438 + mem_limit) ✅
  [x] 1.3 Go: config + database + migrations ✅
  [x] 1.4 Go: JWT auth endpoint ✅
  [x] 1.5 React: Vite + React Router + Zustand ✅
  [x] 1.6 React: Login page + auth flow ✅
  [x] 1.7 ✅ ทดสอบ: docker compose up → login สำเร็จ

Phase 2 — Core AI Pipeline
  [x] 2.1 Go: OpenRouter client (text + image + PDF + audio) ✅
  [x] 2.2 Go: MapperService (exact + fuzzy + F1 learning) ✅
  [x] 2.3 Go: AnomalyService (F2 rules) ✅
  [x] 2.4 Go: SML client (JSON-RPC + retry + admin notify) ✅
  [x] 2.5 Go: WorkerPool (semaphore) ✅
  [ ] 2.6 Unit tests: extract + mapper + anomaly
  [x] 2.7 ✅ ทดสอบ: text → chatbot → SML (bill created BS20260423101501-UELM)

Phase 3 — LINE Integration
  [x] 3.1 Go: LINE webhook (verify + async) ✅
  [x] 3.2 Go: image + PDF download ✅ (code deployed, untested)
  [x] 3.3 Go: voice → Whisper (F3) ✅ (code deployed, untested)
  [x] 3.4 Go: Sales chatbot "น้องบิล" (inquiry/select/qty/cart/checkout) ✅ TESTED
  [x] 3.5 Go: LINE admin push notify ✅
  [x] 3.6b Go: Cart edit — ลบรายการที่ N, แก้จำนวนรายการที่ N เป็น Y ✅
  [ ] 3.7 ทดสอบ LINE loop: รูป / PDF / voice ← ยังไม่ test

Phase 4a — Shopee Import ← ✅ DEPLOYED (SML 248 API confirmed working)
  [x] 4a.1 Go: saleinvoice_client.go (SML 248 REST) ✅
  [x] 4a.2 Go: shopee_import.go (Preview + Confirm + GetConfig) ✅
  [x] 4a.3 React: ShopeeImport.tsx (config dialog + preview + confirm) ✅
  [x] 4a.4 Routes wired in main.go ✅
  [ ] 4a.5 ทดสอบ end-to-end กับ SKU จริง ← ต้องแก้ไฟล์ Excel + ตั้ง UNIT_CODE
  [ ] 4a.6 เพิ่ม SHOPEE_SML_UNIT_CODE=ถุง ใน server .env

Phase 4b — Lazada Import (รอไฟล์จริงจากลูกค้า)
  [ ] 4b.1 Go: Excel parser (column mapping จาก DB)
  [ ] 4b.2 React: Import page + preview + anomaly badge
  [ ] 4b.3 React: Settings → column mapping editor
  [ ] 4b.4 ✅ ทดสอบ bulk import + error report

Phase 5 — Email IMAP
  [x] 5.1 Go: IMAP poller (single account, .env-driven) ✅
  [x] 5.2 Go: attachment download + parse ✅
  [x] 5.2b Go: PDF extraction ใช้ Mistral OCR (mistral-ocr-2512) ✅
  [x] 5.2c Go: dedup by Message-ID (email) ✅
  [x] 5.3 ✅ ทดสอบ email → บิล loop (3 sent on 2026-04-24)
  [x] 5.4 Shopee shipped email → purchaseorder (separate handler)
  [x] 5.5 doc_date extracted from email body (not time.Now())
  [x] 5.6 Multi-account IMAP — DB-driven (migration 006) ✅ (session 6)
           EmailCoordinator + AccountDialog + /settings/email admin UI
           "ยืนยันการชำระเงิน" subject → ShopeeShipped flow (session 6)

Phase 6 — Web UI Complete
  [x] 6.1–6.13 initial UI complete + UX cleanup pass (13 issues) ✅
  [x] 6.14 UI redesign — Tailwind 3 + shadcn/ui (Linear/Vercel style) ✅ (session 6)
            CSS bundle 89 KB → 44 KB; dark mode tokens; ⌘K palette; chord hotkeys
            14-file BillDetail decomposition; 25 shadcn primitives; Sonner toasts
  [x] 6.15 Email Inboxes page (/settings/email) ✅ (session 6)
  [x] 6.16 Shopee Excel artifact storage (.xlsx archived per bill) ✅ (session 6)
  [x] 6.17 UTF-8 charset fix for HTML artifacts (text/html; charset=utf-8) ✅ (session 6)
  [x] 6.18 PO → ใบสั่งซื้อ/สั่งจอง relabeling across all UI components ✅ (session 6)

Phase 7 — Background Jobs
  [x] 7.1 Cron 08:00 daily insight + LINE notify (F4) ✅
  [x] 7.2 Cron 00:00 pg_dump | gzip backup (.sql.gz) ✅ (verified 20 MB output)
  [x] 7.3 Cron weekly LINE token expiry check ✅
  [x] 7.4 Cron daily disk monitor → notify > 90% ✅

Phase 8 — Production Ready
  [ ] 8.1 Cloudflare Tunnel + systemd (cloudflared installed, not configured)
  [ ] 8.2 docker-compose production build
  [x] 8.3 Structured logging (zap) — used everywhere
  [x] 8.4 Health check endpoint (/health, used by deploy.py probe)
  [ ] 8.5 ✅ Demo ลูกค้า
```

---

## 24. Demo Checklist

```
Core Flow — LINE OA Text (Chatbot):
[x] ค้นหาสินค้าจาก catalog SML ผ่าน LINE ✅
[x] เลือกสินค้า + ใส่จำนวน (รับ "3", "10 ถุง", "สิบถุง") ✅
[x] ราคา/หน่วยถูกต้อง (ถุง, เส้น, บาท) ✅
[x] ตะกร้าสะสมหลายรายการ ✅
[x] Checkout → ส่ง SML → bill created ✅ (BS20260423101501-UELM)
[x] SML fail → LINE แจ้ง admin ทันที ✅

Core Flow — ยังไม่ test:
[ ] ส่งรูป PO ใน LINE → บิลสร้างอัตโนมัติ
[ ] ส่ง PDF ใน LINE → extract ถูกต้อง
[ ] ส่ง voice message ใน LINE → สร้างบิล (F3)
[x] ลบรายการที่ N จาก cart ✅
[x] แก้จำนวนรายการที่ N เป็น Y ✅
[ ] Email มี PO attachment → บิลสร้างอัตโนมัติ ← Phase 5 กำลัง test
[ ] Upload Shopee Excel → bulk import สำเร็จ ← Phase 4a (รอ SML 224 เปิด)
[ ] Upload Lazada Excel → bulk import สำเร็จ ← Phase 4b

AI Features:
[ ] item map ไม่ได้ → confirm → ระบบเรียนรู้ (F1)
[ ] Learning Progress แสดง accuracy เพิ่มขึ้น (F1)
[ ] บิลราคาผิดปกติ → anomaly warning (F2)
[ ] บิลซ้ำ → block อัตโนมัติ (F2)
[ ] Dashboard แสดง AI Insights ภาษาไทย (F4)

System:
[x] SML retry 3 ครั้ง + LINE admin notify ✅
[x] Web UI แสดง bill list + detail + status ✅
[ ] SML fail → Web UI retry ได้
[ ] Login แต่ละ role → permission ถูกต้อง
[ ] pg_dump backup ทำงานได้
[ ] disk monitor แจ้ง admin ถ้า > 90%
```

---

## 25. คำสั่งแรกสำหรับ Claude Code ใน VSCode

```
Read CLAUDE.md completely.
Then execute Phase 0 on server 192.168.2.109:
1. Clear docker disk space (target < 70%)
2. Install Go 1.22
3. Setup cloudflared from ~/billflow
4. Verify SML API health
Report disk usage before and after, then ask before Phase 1.
```

---

---

## 26. Current Status (2026-04-28 — session 6)

```
Deployed & Running on 192.168.2.109:
  backend  → billflow-backend  :8090  ✅
  frontend → billflow-frontend :3010  ✅
  postgres → billflow-postgres :5438  ✅
  health:  {"database":"ok","env":"production","status":"ok"}

AI Models (OpenRouter):
  primary  → google/gemini-2.5-flash
  fallback → google/gemini-flash-1.5
  audio    → openai/whisper-1
  embed    → openai/text-embedding-3-small  (1536-dim, in-memory cosine index)
  PDF      → Mistral OCR (mistral-ocr-2512) → markdown → ExtractText

SML servers (both confirmed working):
  SML #1 (LINE/Email JSON-RPC) — http://192.168.2.213:3248
    POST /api/sale_reserve   (line / email / lazada source)
  SML #2 (Shopee REST)        — http://192.168.2.248:8080
    POST /SMLJavaRESTService/restapi/saleinvoice    (Shopee Excel + Shopee email order)
    POST /SMLJavaRESTService/v3/api/purchaseorder   (Shopee shipped / pay-now email)
    POST /SMLJavaRESTService/v3/api/product         (create new product)
    GET  /SMLJavaRESTService/v3/api/product/{sku}   (product lookup)
  Config (SML #2): guid=smlx / SMLGOH / SMLConfigSMLGOH.xml / SML1_2026
                   cust_code=AR00004, wh=WH-01, shelf=SH-01

Multi-account IMAP (session 6 — BREAKING change from session 5):
  .env IMAP_* vars removed. Admin adds/edits inboxes via /settings/email.
  EmailCoordinator: one goroutine per enabled imap_accounts row.
  Active production inboxes:
    "Shopee Inbox"                  bos.catdog@gmail.com  channel=shopee
      subjects: คำสั่งซื้อ, ถูกจัดส่งแล้ว
    "Shopee Inbox (sutee — pay-now)" sutee.toe@gmail.com  channel=shopee
      subject: ยืนยันการชำระเงินคำสั่งซื้อหมายเลข #
  Both "ถูกจัดส่งแล้ว" AND "ยืนยันการชำระเงิน" → ShopeeShipped → purchaseorder

UI redesign (session 6):
  Tailwind 3 + shadcn/ui — Linear/Vercel aesthetic
  CSS bundle: 89 KB → 44 KB
  Dark mode: tokens in place (toggle UI shipped; production test pending)
  ⌘K command palette; two-key chord hotkeys (g d/b/i/s/m/l/c/x)
  BillDetail decomposed from 1234-line monolith → 14-file folder
  Sonner replaces react-hot-toast

PO label: "PO" → "ใบสั่งซื้อ/สั่งจอง" across all UI components

Shopee Excel artifact: original .xlsx archived per imported bill
  (pendingUploads sync.Map, SHA-256 key, 30-min TTL)

UTF-8 fix: all HTML artifacts now served with "text/html; charset=utf-8"
  Frontend Blob rebuilt with explicit type from Content-Type header

Bill flow → SML routing (bills.go Retry handler):
  source                   bill_type   endpoint
  ──────────────────────   ─────────   ──────────────────────
  line / email / lazada    sale        sale_reserve (213)
  shopee / shopee_email    sale        saleinvoice (248)
  shopee_shipped           purchase    purchaseorder (248)

Migrations applied (6 files):
  001_init.sql
  002_audit_logging.sql              (audit_logs structured columns)
  002_sml_catalog.sql                (sml_catalog + extended CHECK)
  003_channel_customer_defaults.sql
  004_shopee_shipped.sql             (bills.source shopee_shipped)
  006_imap_accounts.sql              (imap_accounts table — session 6)

Phases:
  Phase 0–7  ✅
  Phase 6    ✅ UI redesign (Tailwind/shadcn) + multi-account IMAP + artifacts
  Phase 8    ⏳ cloudflared named tunnel + systemd (need domain decision)

Pending (carry-over):
  ⏳ Phase 3 — test รูป/PDF/voice ใน LINE OA (code deployed, never tested)
  ⏳ Phase 4b — Lazada Excel import (waiting customer files)
  ⏳ Phase 8 — cloudflared + systemd

Recent commits (session 6):
  0819450 ui: sync 'PO' wording across FLOW_META + source labels
  6ac1e8d ui: friendlier email-inbox setup + Thai 'ใบสั่งซื้อ/สั่งจอง' label
  1fcf2ba fix(artifact): force Blob type to include charset=utf-8 on preview
  e72eab5 fix(artifact): force UTF-8 charset on text artifacts so Thai renders
  082f86f feat(imap): route Shopee 'ยืนยันการชำระเงิน' to ShopeeShipped flow
  8df8d23 ui(email): inline App Password help popover next to password field
  9bada07 scripts: deploy.py also wipes orphan email_poller.go after IMAP refactor
  b0c77ef feat(imap): multi-account email config with admin UI
  faf906d feat: archive Shopee Excel as artifact for every imported bill
  decb685 scripts: deploy.py also wipes orphan BillDetail.tsx after decomposition
```

---

*Last updated: 2026-04-28 (session 6)*
*Server: 192.168.2.109 | Project: billflow | Folder: ~/billflow*
*Ports: backend:8090 / frontend:3010 / postgres:5438*
*⚠️ LINE credentials ต้อง reissue ก่อนใช้ทุกครั้ง*
