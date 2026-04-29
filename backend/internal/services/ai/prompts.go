package ai

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

กฎสำคัญเรื่องราคา:
- price ใน items คือราคาต่อหน่วย (unit price) ไม่ใช่ราคารวม
- ถ้าข้อความระบุราคารวมของทุก item ให้ใส่ใน total_amount และ price ของแต่ละ item เป็น null
- ถ้าข้อความระบุราคาต่อหน่วยของ item นั้นๆ ให้ใส่ใน price
- ตัวอย่าง: "ปูนซีเมนต์ 2 ถุง 300 บาท" — ถ้าไม่ชัดว่า 300 คือต่อถุงหรือรวม ให้ใส่ total_amount=300, price=null
- ตัวอย่าง: "ปูนซีเมนต์ 2 ถุง ถุงละ 150 บาท" — ใส่ price=150, total_amount=300

ถ้าข้อมูลไม่ชัดเจน ให้ confidence ต่ำ (< 0.5)
ถ้าข้อมูลมาจาก voice transcription ให้ confidence ลดลง 0.1
`

const InsightPrompt = `
คุณเป็น business analyst สรุปข้อมูลธุรกิจเป็นภาษาไทย
กระชับ 3-5 ประโยค ใช้ emoji นำหน้าแต่ละประเด็น
ข้อมูลวันนี้ vs สัปดาห์ที่แล้ว: %s
สรุป: trend / สินค้าขายดี-แย่ / บิลมีปัญหา / คำแนะนำ
`

// SalesSystemPrompt was removed in session 13 along with the AI chatbot.
// LINE conversations are now human-to-human via the /messages inbox.
