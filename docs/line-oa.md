# LINE OA — การทำงานของ น้องบิล

> อัพเดตล่าสุด: 2026-04-24  
> สถานะ: ✅ Mode 1 (text chatbot + cart edit) ทดสอบแล้ว | ⚠️ Mode 2/3 code พร้อม ยังไม่ test

---

## ภาพรวม

ลูกค้าติดต่อผ่าน LINE Official Account → "น้องบิล" ตอบอัตโนมัติ  
รองรับ 3 รูปแบบ:

| Mode | วิธีส่ง | สถานะ |
|---|---|---|
| 1 | พิมพ์ข้อความสั่งซื้อ (conversational) | ✅ Tested |
| 2 | ส่งรูปภาพ / PDF ใบ PO | ⚠️ Code deployed, ยังไม่ test |
| 3 | ส่ง voice message (F3) | ⚠️ Code deployed, ยังไม่ test |

---

## Mode 1 — Conversational Chatbot

### Flow ทั้งหมด

```
ลูกค้าพิมพ์ข้อความใน LINE
        │
        ▼
LINE Platform ส่ง HTTP POST → /webhook/line
        │
        ▼
Backend: verify X-Line-Signature (HMAC-SHA256)
  ผิด → return 403
  ถูก → return 200 ทันที (ต้องตอบใน 1 วินาที)
        │
        ▼ (async goroutine)
WorkerPool.Submit(job)  ← semaphore จำกัด 5 concurrent
        │
        ▼
AI วิเคราะห์ intent (Gemini 2.5 Flash)
        │
        ├── "inquiry" / "มีปูนขายไหม" / "ราคา..." ?
        │         │
        │         ▼
        │    MCPClient.SearchProduct(keyword)
        │    → SML catalog (/call endpoint)
        │    → แสดงรายการ 1-5 ชิ้น ให้เลือก
        │
        ├── ลูกค้าเลือก "รายการที่ 2" ?
        │         │
        │         ▼
        │    บันทึกสินค้าที่เลือก (item_code, unit_code, price)
        │    → ถาม "ต้องการกี่ถุงครับ?"
        │
        ├── ลูกค้าพิมพ์จำนวน "3" / "10 ถุง" / "สิบถุง" ?
        │         │
        │         ▼
        │    AI ParseQty → แปลงเป็นตัวเลข
        │    → เพิ่มลงตะกร้า (session state ใน memory)
        │    → "เพิ่ม 3 ถุง เรียบร้อยครับ 🛒 ต้องการอะไรเพิ่มไหม?"
        │
        ├── "ดูตะกร้า" / "view_cart" ?
        │         │
        │         ▼
        │    แสดง Flex Message สรุปตะกร้า
        │    [รายการ] [จำนวน] [ราคา]
        │    ปุ่ม: ยืนยันสั่งซื้อ | สั่งต่อ
        │
        ├── "checkout" / "สั่งซื้อ" ?
        │         │
        │         ▼
        │    ขอชื่อและเบอร์โทร
        │    → สรุปใบสั่งซื้อ
        │    → รอ "ยืนยัน" หรือ "ยกเลิก"
        │
        └── "ยืนยัน" ?
                  │
                  ▼
             สร้าง SaleReserveRequest
             → SML CreateSaleReserve (retry max 3)
                  │
                  ├── สำเร็จ → bill status = 'sent'
                  │           LINE ตอบ "สั่งซื้อเรียบร้อย เลขที่ BS..."
                  │
                  └── ล้มเหลว → bill status = 'failed'
                               LINE admin push notify ⚠️
```

### Session State (Cart)

cart เก็บใน memory (Go map) ตาม LINE User ID:

```go
// ใน handlers/line.go
sessions map[string]*ChatSession  // key = userID

type ChatSession struct {
    Cart      []CartItem
    State     string  // "idle" | "selecting" | "asking_qty" | "checkout" | ...
    LastItems []ProductResult  // รายการที่ค้นหาล่าสุด
}
```

### Cart Edit ✅ Implemented

ลูกค้าสามารถแก้ไขตะกร้าได้สองวิธี:

| คำสั่ง | ผลลัพธ์ |
|---|---|
| "ลบรายการที่ 2" | ลบสินค้าชิ้นที่ 2 ออกจากตะกร้า |
| "แก้จำนวนรายการที่ 1 เป็น 5" | เปลี่ยน qty ของสินค้าชิ้นที่ 1 เป็น 5 |

AI วิเคราะห์คำสั่งด้วย intent `cart_delete` / `cart_edit` และ extract ตัวเลข

⚠️ ยังไม่รองรับ: ลบทั้งตะกร้า (ใช้ "ยกเลิก" แทน)

---

## Mode 2 — รูปภาพ / PDF ใบ PO

### Flow

```
ลูกค้าส่งรูปภาพ/PDF → LINE webhook (MessageTypeImage / MessageTypeFile)
        │
        ▼
LineService.DownloadContent(messageID)
← LINE Content API (มี expiry ~1 วัน)
        │
        ▼
ถ้าเป็นรูปภาพ:
  AIClient.ExtractImage(base64, "image/jpeg")
  → Gemini วิเคราะห์รูป → JSON

ถ้าเป็น PDF:
  MistralOCR.ExtractTextFromPDF(base64)
  → markdown text
  → AIClient.ExtractText(markdownText)
  → JSON
        │
        ▼
  ← AI Pipeline เหมือน Mode 1 (Map → Anomaly → Auto-confirm หรือ Pending)
```

> ⚠️ **ยังไม่ได้ test** — code deployed แต่รอ test case จริง

---

## Mode 3 — Voice Message (F3)

### Flow

```
ลูกค้าส่ง voice message → LINE webhook (MessageTypeAudio)
        │
        ▼
LineService.DownloadContent(messageID)  ← ต้อง download ทันที! audio มี expiry สั้น
        │
        ▼
AIClient.TranscribeAudio(audioData)
→ OpenRouter Whisper (openai/whisper-1)
→ text transcription
        │
        ▼
ตรวจสอบความยาว:
  > 60 วินาที → LINE ตอบ "ขอโทษค่ะ ส่ง voice สั้นกว่า 60 วินาทีได้เลยค่ะ"
        │
        ▼
ส่ง text ไป AI Extract pipeline (เหมือน Mode 1)
confidence ลด 0.1 อัตโนมัติ (voice มีโอกาสผิดพลาดสูงกว่า text)
```

> ⚠️ **ยังไม่ได้ test** — code deployed แต่รอ test case จริง

---

## Admin Notifications

LINE push message ไปหา `LINE_ADMIN_USER_ID` ในกรณี:

| กรณี | ข้อความ |
|---|---|
| SML fail หลัง retry 3 ครั้ง | ⚠️ LINE SML failed\nBill: ...\nError: ... |
| Bill pending (unmapped/anomaly) | 📋 Bill pending review\nBill: ...\nConfidence: ... |
| Disk usage > 90% | ⚠️ Disk warning: X% |
| LINE token จะหมดอายุใน 7 วัน | ⚠️ LINE token expiring in 7 days |
| F4 Daily insight สร้างแล้ว | รายงาน AI insights ประจำวัน |

---

## Security

- Signature ทุก webhook ต้องผ่าน HMAC-SHA256 กับ `LINE_CHANNEL_SECRET`
- ถ้า signature ไม่ตรง → return 403 ทันที ไม่ process ต่อ
- Response 200 ต้องส่งภายใน 1 วินาที → ใช้ async goroutine เสมอ

---

## Config ที่เกี่ยวข้อง

```bash
LINE_CHANNEL_SECRET=          # สำหรับ verify webhook signature
LINE_CHANNEL_ACCESS_TOKEN=    # สำหรับ reply / push message
LINE_ADMIN_USER_ID=           # LINE User ID ของ admin รับ push notify

AUTO_CONFIRM_THRESHOLD=0.85   # confidence ขั้นต่ำสำหรับ auto-confirm
OPENROUTER_MODEL=google/gemini-2.5-flash
OPENROUTER_AUDIO_MODEL=openai/whisper-1
SML_BASE_URL=http://192.168.2.213:3248
```

---

## ไฟล์ที่เกี่ยวข้อง

| ไฟล์ | หน้าที่ |
|---|---|
| `backend/internal/handlers/line.go` | webhook handler + chatbot state machine |
| `backend/internal/services/line/service.go` | reply / push / download content |
| `backend/internal/services/ai/openrouter.go` | ExtractImage, ExtractPDF, ExtractText, TranscribeAudio |
| `backend/internal/services/sml/mcp.go` | SearchProduct (catalog lookup) |
| `backend/internal/services/mapper/mapper.go` | F1 fuzzy matching |
| `backend/internal/services/anomaly/detector.go` | F2 anomaly rules |
| `backend/internal/worker/pool.go` | semaphore rate limiting |
