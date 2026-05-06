# BillFlow Phase 1 — คู่มือการใช้งาน

> ระบบดึง Email สร้างใบสั่งซื้อ (Purchase Order) อัตโนมัติ

---

## Phase 1 ทำอะไรได้บ้าง?

| ความสามารถ | รายละเอียด |
|---|---|
| ดึง Email อัตโนมัติ | ดึง Email จาก inbox ตาม subject / ผู้ส่งที่กำหนด ทุก 5 นาที |
| AI อ่าน Email | สกัดรายการสินค้า, จำนวน, ราคา, ผู้ขาย จาก body และ attachment (PDF, รูป) |
| จับคู่สินค้า | AI จับคู่ชื่อสินค้าจาก Email กับรหัสสินค้าใน SML Master |
| สร้างสินค้าใหม่ inline | ถ้าไม่พบใน SML สามารถสร้างสินค้าใหม่จากหน้าบิลได้ทันที |
| ตรวจสอบก่อนส่ง | Admin ตรวจสอบ แก้ไขรายการ และยืนยันก่อนส่งเข้า SML |
| ส่งใบสั่งซื้อ SML | สร้าง Purchase Order ใน SML ERP ผ่าน REST API อัตโนมัติ |
| Retry อัตโนมัติ | กรณีส่งไม่สำเร็จ กด Retry ได้จากหน้าบิล |

---

## หน้าต่างๆ ในระบบ

```
Dashboard        → ภาพรวมบิลวันนี้ — รอตรวจสอบ / ล้มเหลว / Email มีปัญหา
บิลทั้งหมด       → รายการบิลทั้งหมด (filter ตามสถานะ / source)
บิลรายการ        → แก้ไข รายการ / ราคา / ผู้ขาย + ส่ง SML
ตารางจับคู่สินค้า → ดู/จัดการ mapping ชื่อสินค้า ↔ รหัส SML
สินค้าใน SML     → catalog สินค้าทั้งหมดจาก SML (ค้นหา, sync, embed)
ผู้ขาย default    → ตั้งค่า Supplier default ต่อ channel (email / shopee_shipped)
อีเมลรับบิล      → จัดการ IMAP inbox (เพิ่ม / แก้ไข / ทดสอบ)
ตั้งค่าทั่วไป     → สถานะระบบ (AI / Email / SML)
ประวัติการทำงาน  → audit log ทุก action ในระบบ
```

---

## การตั้งค่าครั้งแรก

### 1. เพิ่ม Email Inbox

1. เข้าเมนู **ตั้งค่าระบบ → อีเมลรับบิล**
2. กด **เพิ่ม Inbox ใหม่**
3. กรอกข้อมูล:

| ฟิลด์ | ตัวอย่าง | หมายเหตุ |
|---|---|---|
| ชื่อ Inbox | "อีเมลจัดซื้อ" | ชื่อสำหรับ admin |
| Host | `imap.gmail.com` | หรือ `imap-mail.outlook.com` |
| Port | `993` | TLS/SSL เสมอ |
| Username | `purchase@company.com` | |
| Password | App Password | ดูหมายเหตุด้านล่าง |
| Mailbox | `INBOX` | หรือ folder ที่ต้องการ |
| กรอง subject | `ใบสั่งซื้อ, PO-` | คั่นด้วยคอมม่า |
| กรอง ผู้ส่ง | `supplier@example.com` | ว่างไว้ = ทุกคน |
| Channel | `general` | สำหรับ PO ทั่วไป |
| ดึงย้อนหลัง | `30` วัน | 1–90 วัน |
| Interval | `300` วินาที | ขั้นต่ำ 5 นาที |

4. กด **ทดสอบการเชื่อมต่อ** ก่อนบันทึก
5. กด **บันทึก** — ระบบเริ่มดึง Email ทันที

> **Gmail App Password:** ต้องเปิด 2-Step Verification ก่อน → ไปที่  
> myaccount.google.com → Security → App passwords → สร้างใหม่ → copy มาวาง  
> ห้ามใช้รหัสผ่านจริง — Gmail จะ block

> **Outlook:** ใช้ `imap-mail.outlook.com:993` + App Password เช่นกัน

---

### 2. ตั้งค่า Supplier Default

ระบบต้องรู้ว่า Supplier (ผู้ขาย) default ของแต่ละ channel คือใคร ก่อนส่ง SML

1. เข้าเมนู **ข้อมูลตั้งต้น → ผู้ขาย default**
2. กด **ตั้งค่าอัตโนมัติ** (Quick Setup) → ระบบจะดึงรายชื่อ Supplier จาก SML ให้เลือก
3. หรือกด **แก้ไข** ต่อ channel แล้วเลือก Supplier จาก dropdown

> ถ้ายังไม่ตั้งค่า → ปุ่ม "ส่งไปยัง SML" จะ error ทันที

---

### 3. Sync สินค้าจาก SML

เพื่อให้ AI จับคู่สินค้าได้แม่นยำ ควร sync catalog จาก SML ก่อน

1. เข้าเมนู **ข้อมูลตั้งต้น → สินค้าใน SML**
2. กด **Sync จาก SML** → ดึงสินค้าทั้งหมดจาก SML เข้าระบบ
3. กด **Embed All** → สร้าง AI embedding (ใช้เวลาเป็นนาที ปิดหน้าได้)

> หลัง Embed เสร็จ AI จะจับคู่ชื่อสินค้าได้แม่นยำขึ้นมาก

---

## Workflow หลัก

```
Email เข้า inbox
      ↓
ระบบดึงอัตโนมัติ (ทุก 5 นาที)
      ↓
AI อ่าน body / PDF / รูปภาพ
      ↓
สร้างบิล status = "รอตรวจสอบ"
      ↓
Admin เปิด /bills → คลิกบิล
      ↓
ตรวจสอบ แก้ไขรายการ / ราคา / ผู้ขาย
      ↓ (ถ้า AI จับคู่สินค้าไม่ได้)
กด Map บนรายการ → เลือกรหัสสินค้า SML
หรือ กด "+ สร้างสินค้าใหม่" ถ้ายังไม่มีใน SML
      ↓
กด "ส่งไปยัง SML"
      ↓
SML สร้าง Purchase Order ✓
```

---

## สถานะบิล

| สถานะ | ความหมาย | ต้องทำอะไร |
|---|---|---|
| รอดำเนินการ | AI ดึงมาแล้ว รอ admin ตรวจ | เปิดบิล ตรวจสอบ ส่ง SML |
| รอตรวจสอบ | มีสินค้าที่ AI จับคู่ไม่ได้ หรือข้อมูลผิดปกติ | Map สินค้า แล้วส่ง |
| ส่งแล้ว | SML รับบิลเรียบร้อย | ไม่ต้องทำอะไร |
| ล้มเหลว | SML ตอบ error | แก้ปัญหา แล้วกด Retry |
| ข้าม | บิลซ้ำ หรือ admin ข้ามไป | — |

---

## การจับคู่สินค้า (Catalog Matching)

### วิธี AI จับคู่

1. **Exact match** — ชื่อตรงกัน 100% → map อัตโนมัติ
2. **Fuzzy match** — คล้ายกัน ≥ 85% → map อัตโนมัติ
3. **คล้ายกัน 60–84%** → status "รอตรวจสอบ" ต้อง confirm
4. **ไม่พบ** → admin เลือก หรือสร้างสินค้าใหม่ใน SML

### ระบบเรียนรู้ (F1)

เมื่อ admin confirm การ map → ระบบบันทึกไว้ → ครั้งต่อไปสินค้าเดียวกันจะ map อัตโนมัติ  
ดูความคืบหน้าได้ที่ **ตารางจับคู่สินค้า → Learning Progress**

---

## การสร้างสินค้าใหม่ใน SML

ถ้าสินค้าใน Email ยังไม่มีใน SML:

1. เปิดบิล → คลิกรายการที่ยังไม่ได้ map
2. กด **+ สร้างสินค้าใหม่**
3. กรอก: รหัสสินค้า, ชื่อ, หน่วย, ราคา
4. กด **สร้างใน SML** → ระบบสร้างสินค้าใน SML และ map ให้อัตโนมัติ

---

## Troubleshoot เบื้องต้น

### Email ไม่เข้าระบบ

1. เข้า **อีเมลรับบิล** → ดูคอลัมน์ "สถานะล่าสุด"
2. ถ้า error → กด **Poll ตอนนี้** เพื่อดู error message
3. ตรวจสอบ: App Password ถูกต้อง, IMAP เปิดอยู่ใน Gmail settings
4. Gmail: ต้องไปที่ Gmail Settings → See all settings → Forwarding and POP/IMAP → Enable IMAP

### บิล status "ล้มเหลว"

1. เปิดบิล → ดู Error card (กล่องแดง) — ระบุ route และ error message
2. สาเหตุบ่อย:
   - `party_code` ไม่ถูกต้อง → แก้ที่ **ผู้ขาย default**
   - สินค้าไม่ครบ (`item_code` ว่าง) → Map สินค้าก่อน
   - SML server ไม่ตอบสนอง → รอแล้ว Retry
3. กด **Retry** หลังแก้ปัญหา

### สินค้า map ผิด

1. เปิดบิล → คลิกรายการ → กด **แก้ไข**
2. เลือก item_code ที่ถูกต้องจาก dropdown
3. กดบันทึก → ระบบจำสำหรับครั้งต่อไป

### Dashboard แสดง "Email มีปัญหา"

- มี inbox ที่ poll ล้มเหลวติดต่อกัน 3 ครั้งขึ้นไป
- เข้า **อีเมลรับบิล** → ดู error → แก้แล้วกด Poll

---

## การเปิด Feature เพิ่มเติม (Phase ถัดไป)

ระบบ BillFlow รองรับ Phase ถัดไปโดยไม่ต้อง deploy ใหม่ เพียงแก้ค่า `VITE_PHASE` ใน `.env`:

```bash
# Phase 1 — Email → PO อัตโนมัติ (ปัจจุบัน)
VITE_PHASE=1

# Phase 2+ — เปิดทุก feature (LINE chat, Shopee, Lazada)
VITE_PHASE=99
```

แล้ว rebuild frontend:

```bash
docker compose build frontend
docker compose up -d frontend
```

---

## ข้อมูลการเชื่อมต่อ

| ระบบ | ที่อยู่ | หมายเหตุ |
|---|---|---|
| BillFlow Admin UI | `http://192.168.2.109:3010` | เปิดจาก browser ในวง LAN |
| BillFlow API | `http://192.168.2.109:8090` | backend |
| SML ERP | `http://192.168.2.213:3248` | (LAN only) |
| PostgreSQL | `192.168.2.109:5438` | DB ของ BillFlow |

---

*BillFlow Phase 1 — ระบบดึง Email สร้างบิลซื้ออัตโนมัติ*  
*สอบถามทีม Dev: bos.catdog@gmail.com*
