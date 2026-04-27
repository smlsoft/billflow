# BillFlow — ภาพรวมการทำงาน

> อัพเดตล่าสุด: 2026-04-24

---

## ระบบทำงานยังไง (ในภาษาคน)

พนักงานร้านค้าส่งใบสั่งซื้อมาหลายช่องทาง — LINE, Email, หรือ upload Excel  
BillFlow รับข้อมูล → ให้ AI อ่านและ extract → จับคู่รหัสสินค้า → ส่งสร้างบิลใน SML โดยอัตโนมัติ  
พนักงานแทบไม่ต้องคีย์บิลเองเลย

---

## Input → Process → Output

```
┌──────────────┐    ┌─────────────────────────────────────┐    ┌────────────┐
│   INPUT      │    │            PROCESSING               │    │   OUTPUT   │
│              │    │                                     │    │            │
│  LINE OA     ├───►│  1. รับข้อมูล (webhook/poll/upload) │    │  SML ERP   │
│  (text/รูป/  │    │                    │                │    │  บิลสร้าง  │
│   PDF/voice) │    │  2. AI Extract     │                ├───►│  อัตโนมัติ │
│              │    │     (Gemini/OCR)   │                │    │            │
│  Email IMAP  ├───►│                    │                │    │  LINE      │
│  (PDF/รูป/   │    │  3. Item Mapping   │                │    │  แจ้ง admin│
│   Excel)     │    │     (F1 fuzzy)     │                ├───►│  ทุก event │
│              │    │                    │                │    │            │
│  File Upload ├───►│  4. Anomaly Check  │                │    │  PostgreSQL│
│  (Lazada/    │    │     (F2 rules)     │                ├───►│  log ทุก   │
│   Shopee     │    │                    │                │    │  บิล       │
│   Excel)     │    │  5. Auto-confirm   │                │    │            │
│              │    │     หรือ Pending   │                │    │            │
└──────────────┘    └─────────────────────────────────────┘    └────────────┘
```

---

## Component Map

```
billflow/
│
├── LINE OA Webhook (/webhook/line)
│     handlers/line.go
│     ├── Mode 1: Chatbot น้องบิล (text → cart → SML)
│     ├── Mode 2: รูป/PDF PO → AI extract → SML
│     └── Mode 3: Voice → Whisper → Mode 1 pipeline
│
├── Email Poller (background goroutine)
│     jobs/email_poller.go → services/email/imap.go
│     └── handlers/email.go (AttachmentProcessor)
│
│  File Import
│     handlers/import.go              ← Lazada (Phase 4b — รอไฟล์)
│     handlers/shopee_import.go       ← Shopee Preview + Confirm + GetConfig ✅
│
├── Logs / Audit
│     handlers/log_handler.go         ← GET /api/logs
│     repository/audit_log_repo.go    ← Log() + List()
│     models/audit_log.go
│
├── AI Pipeline (ใช้ร่วมกันทุก channel)
│     services/ai/openrouter.go          ← Gemini 2.5 Flash
│     services/mistral/ocr.go            ← Mistral OCR สำหรับ PDF
│     services/mapper/mapper.go          ← F1: fuzzy match + learning
│     services/anomaly/detector.go       ← F2: anomaly rules
│     services/sml/client.go             ← JSON-RPC → SML #1 (LINE/Email)
│     services/sml/saleinvoice_client.go ← REST → SML #2 (Shopee)
│     services/sml/mcp.go                ← catalog search
│
└── Web UI (React :3010)
      /login → /dashboard → /bills → /bills/:id
      → /import → /import/shopee → /logs → /mappings → /settings
```

---

## AI Pipeline ทำงานยังไง (Step by step)

```
Input (text/รูป/PDF/voice)
        │
        ▼
┌─────────────────┐
│  Step 1: Extract │  ← AI อ่านและแปลงเป็น JSON
│                  │
│  text  → Gemini  │    output: {customer_name, items[{raw_name, qty, unit, price}], confidence}
│  รูป   → Gemini  │
│  PDF   → Mistral │    Mistral OCR → markdown text → Gemini ExtractText
│           OCR    │
│  voice → Whisper │    transcribe → text → Gemini ExtractText
└────────┬─────────┘
         │
         ▼
┌─────────────────┐
│  Step 2: Map     │  ← จับคู่ raw_name → item_code/unit_code
│  (F1 Mapper)     │
│                  │    1. Exact match (confidence 1.0)
│                  │    2. Fuzzy match (Levenshtein)
│                  │       ≥ 0.85 → auto map
│                  │       0.60-0.84 → needs_review
│                  │       < 0.60 → unmapped
│                  │
│  allMapped?      │    ถ้า unmapped → pending + แจ้ง admin
│  → ต้องครบทุกชิ้น │    พนักงานเพิ่ม mapping → retry
└────────┬─────────┘
         │
         ▼
┌─────────────────┐
│  Step 3: Anomaly │  ← F2 ตรวจความผิดปกติ
│  (F2 Detector)   │
│                  │    block: ราคา=0, qty=0, บิลซ้ำ
│                  │    warn:  ราคาผิดปกติ, qty สูงผิดปกติ, ลูกค้าใหม่
│                  │
│                  │    block → ต้อง manual confirm เสมอ
│                  │    warn > 1 รายการ → pending
└────────┬─────────┘
         │
         ▼
┌─────────────────────────────┐
│  Step 4: Auto-confirm?       │
│                              │
│  allMapped = true            │
│  AND confidence ≥ 0.85       │  YES →  ส่ง SML ทันที
│  AND ไม่มี block             │ ──────► status = 'sent'
│  AND warn ≤ 1                │         doc_no = BS...
│                              │
│                              │  NO  →  status = 'pending'
│                              │ ──────► LINE แจ้ง admin review
└──────────────────────────────┘
```

---

## Item Mapping (F1) — หัวใจของระบบ

ปัญหา: ลูกค้าพิมพ์ชื่อสินค้าหลายแบบ เช่น:
- "ปูนกาว" / "ปูนกาวปูกระเบื้อง" / "Beger ปูนกาวปูกระเบื้อง 50 มม. Eco-Series"

ระบบต้องรู้ว่าทั้งหมดนี้คือ `CON-01140` (รหัสในSML)

```
Flow:
1. ครั้งแรก: ชื่อไม่ match → unmapped → พนักงาน add mapping ใหม่
2. ครั้งถัดไป: exact/fuzzy match → auto map
3. ยิ่งใช้บ่อย → usage_count สูง → fuzzy score boost → match ง่ายขึ้น

Table: mappings
  raw_name = "Beger ปูนกาวปูกระเบื้อง 50 มม. Eco-Series"
  item_code = "CON-01140"
  unit_code = "ถุง"
  confidence = 1.0
  usage_count = 5
  source = "manual" | "ai_learned"
```

---

## Retry Flow

เมื่อบิล `status = 'failed'` หรือ `'pending'` (ยังส่ง SML ไม่ได้):

```
POST /api/bills/:id/retry
        │
        ▼
  Re-run mapper บน items ทุกชิ้น
  (pick up mappings ใหม่ที่เพิ่งเพิ่ม)
        │
        ├── allMapped? NO → status = 'pending', แจ้ง user
        │
        └── allMapped? YES → ส่ง SML → status = 'sent' + doc_no
```

---

## Anomaly Detection (F2) — ป้องกันบิลผิดพลาด

```
ตัวอย่าง:
  ราคาปกติ: 189 บาท/ถุง
  บิลใหม่มา: 1,000 บาท/ถุง → "price_too_high" (warn)
  บิลใหม่มา: 0 บาท → "price_zero" (BLOCK → ต้อง manual confirm)

  ลูกค้า A สั่งของวันนี้แล้ว สั่งอีกรายการเดิม → "duplicate_bill" (BLOCK)
```

---

## Background Jobs

| Job | เวลา | หน้าที่ |
|---|---|---|
| Email Poller | ทุก 5 นาที | poll IMAP → process → SML |
| Daily Insight (F4) | 08:00 ทุกวัน | AI สรุปยอดขาย → LINE admin |
| Backup | 00:00 ทุกวัน | pg_dump → backups/ |
| Token Checker | ทุกอาทิตย์ | ตรวจ LINE token หมดอายุ |
| Disk Monitor | ทุกวัน | แจ้งถ้า disk > 90% |

---

## สถานะปัจจุบัน (2026-04-24)

| Channel | สถานะ | หมายเหตุ |
|---|---|---|
| LINE OA text chatbot | ✅ ทดสอบแล้ว | cart edit ✅ |
| LINE OA รูป/PDF/voice | ⚠️ code deployed ยังไม่ test | — |
| Email IMAP (PDF) | ✅ deployed + tested | dedup by Message-ID ✅ |
| Shopee Excel → SML 248 | ✅ deployed, SML 248 API confirmed | ต้องแก้ไฟล์ Excel ใช้ SKU จริง |
| Lazada Excel | ⏳ รอไฟล์จากลูกค้า | Phase 4b |

**DB ปัจจุบัน:** 0 bills (cleared สำหรับ clean Shopee test)  
**Last successful bill:** BS20260423101501-UELM (LINE OA, 4,603.19 บาท)

---

## ไฟล์เอกสารอื่นๆ

| ไฟล์ | เนื้อหา |
|---|---|
| [docs/line-oa.md](line-oa.md) | LINE OA workflow รายละเอียด |
| [docs/email.md](email.md) | Email IMAP workflow รายละเอียด |
| [README.md](../README.md) | คู่มือ deploy + API reference |
| [CLAUDE.md](../CLAUDE.md) | Blueprint สำหรับ AI coding assistant |
