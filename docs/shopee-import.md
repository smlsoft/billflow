# Shopee Excel Import — การทำงาน

> อัพเดตล่าสุด: 2026-05-06
> สถานะ: ✅ Deployed — SML 248 (`192.168.2.248`) configured on server

---

## ภาพรวม

พนักงานนำ Excel export จาก Shopee Seller Center มา upload ที่ `/import/shopee`
ระบบ parse orders → ตรวจ duplicate → แสดง preview → พนักงาน confirm → สร้าง bills ใน BillFlow แล้วให้ admin ตรวจ/Retry ส่งเข้า SML 248
default sale route ปัจจุบันคือ `saleorder`; `saleinvoice` ใช้ได้เมื่อ admin ตั้ง `channel_defaults.endpoint` ให้ชี้ `/SMLJavaRESTService/saleinvoice/v4`

---

## Flow ทั้งหมด

```
พนักงาน
  │
  ▼
/import/shopee (ShopeeImport.tsx)
  │
  ├── 1. กด "เลือกไฟล์ Shopee"
  │         │
  │         ▼
  │   GET /api/settings/shopee-config
  │   ← config pre-filled จาก env vars
  │
  ├── 2. Preflight config
  │   - ต้องมี channel_defaults สำหรับ Shopee sale ก่อน
  │   - UI block file picker ถ้า config ยังไม่พร้อม
  │
  ├── 3. เลือกไฟล์ .xlsx จาก Shopee Seller Center
  │
  ├── 4. POST /api/import/shopee/preview
  │         │
  │         ▼
  │   Backend:
  │   - parse Excel (column names ภาษาไทย hardcoded)
  │   - exclude สถานะ: "ที่ต้องจัดส่ง", "ยกเลิกแล้ว"
  │   - dedup check: SELECT FROM bills WHERE source='shopee' AND order_id=?
  │   - ไม่ write DB ใน preview
  │         │
  │         ▼
  │   Preview table: แสดงทุก order
  │   - order_id, วันที่, ชื่อสินค้า, qty, ราคา
  │   - badge เหลือง "ซ้ำ" ถ้า order นั้น import แล้ว
  │   - non-duplicate pre-checked อัตโนมัติ
  │
  ├── 5. พนักงานเลือก orders ที่ต้องการ → กด "ยืนยัน Import"
  │
  └── 6. POST /api/import/shopee/confirm
            │
            ▼
      Backend (ต่อ order):
      - GET /SMLJavaRESTService/v3/api/product/{sku}
        → flat response: start_sale_unit/wh/shelf
        → data=null ถ้าไม่พบ SKU → ใช้ config defaults
      - บันทึก bill + items ลง DB พร้อม candidates/artifacts
      - ยังไม่ส่ง SML ทันที
      - admin เปิด bill → route preview → Retry
      - Retry default: POST /SMLJavaRESTService/v3/api/saleorder
            │
            ▼
      แสดง results: created / skipped / failed
```

---

## Column Names (Hardcoded)

ไฟล์ Excel จาก Shopee Seller Center ใช้ชื่อ column ภาษาไทยคงที่ — ไม่ต้อง configure:

| Field ในระบบ | Column Name ใน Excel |
|---|---|
| order_id | หมายเลขคำสั่งซื้อ |
| status | สถานะการสั่งซื้อ |
| order_date | วันที่ทำการสั่งซื้อ |
| product_name | ชื่อสินค้า |
| sku | เลขอ้างอิง SKU (SKU Reference No.) |
| price | ราคาขาย |
| qty | จำนวน |

**หมายเหตุ:** Lazada ใช้ column mapping จาก DB (admin config ได้จาก `/settings`) เพราะ format อาจต่างกัน

---

## สถานะที่ Exclude

orders ที่มีสถานะเหล่านี้จะถูกข้ามโดยอัตโนมัติ (ไม่แสดงใน preview):

- `ที่ต้องจัดส่ง`
- `ยกเลิกแล้ว`

---

## Dedup Logic

ก่อน confirm แต่ละ order ระบบตรวจสอบ:

```sql
SELECT COUNT(*) FROM bills
WHERE source = 'shopee'
AND raw_data->>'order_id' = $1
```

ถ้า > 0 → แสดง badge สีเหลือง "ซ้ำ" ใน preview table และ uncheck อัตโนมัติ

---

## SML 248 Connection

```
Base URL:  http://192.168.2.248:8080
Headers (ทุก request):
  guid:             SHOPEE_SML_GUID        (smlx)
  provider:         SHOPEE_SML_PROVIDER    (SMLGOH)
  configFileName:   SHOPEE_SML_CONFIG_FILE  (SMLConfigSMLGOH.xml)
  databaseName:     SHOPEE_SML_DATABASE    (SML1_2026)
```

**Config บน server ที่ตรวจเมื่อ 2026-05-06:**
```bash
SHOPEE_SML_URL=http://192.168.2.248:8080
SHOPEE_SML_DOC_FORMAT=INV
SHIPPED_SML_DOC_FORMAT=PO
```

ค่า `party_code`, endpoint, doc prefix/running format, WH/Shelf/VAT override อยู่ใน `channel_defaults` และจัดการผ่าน `/settings/channels`

**ทดสอบ connection:**
```bash
curl "http://192.168.2.248:8080/SMLJavaRESTService/v3/api/product/CON-01000" \
  -H "guid: smlx" \
  -H "provider: SMLGOH" \
  -H "configFileName: SMLConfigSMLGOH.xml" \
  -H "databaseName: SML1_2026"
```

**SKU จริงใน SML 248 (ic_inventory):**

| Series | ตัวอย่าง SKU | หน่วย |
|---|---|---|
| CON-xxxxx | CON-01000 | ถุง |
| STEEL-xxxxx | STEEL-01001 | เส้น |
| PLUMB-xxxxx | PLUMB-01002 | ท่อน |
| ROOF-xxxxx | ROOF-01006 | แผ่น |

⚠️ **ไฟล์ Excel ทดสอบต้องใช้ SKU ที่มีอยู่จริงใน ic_inventory** — REST-00002 ไม่มีใน SML 248

---

## Product Lookup

```
GET /SMLJavaRESTService/v3/api/product/{sku}

Response (flat — ไม่มี nested object):
  {"success":true,"data":{"code":"CON-01000","unit_standard":"ถุง",
                           "start_sale_unit":"ถุง","start_sale_wh":"WH-01",
                           "start_sale_shelf":"SH-01"}}

  {"success":true,"data":null}  ← ไม่พบ SKU ใน SML

Response fields ที่ใช้ (priority):
  data.start_sale_unit   → unit_code  (ก่อน)
  data.unit_standard     → unit_code  (fallback ถ้า start_sale_unit ว่าง)
  data.start_sale_wh     → wh_code
  data.start_sale_shelf  → shelf_code
```

ถ้า data=null → ใช้ค่า config defaults (WHCode, ShelfCode, UnitCode จาก env)

⚠️ **SHOPEE_SML_UNIT_CODE ต้องไม่ว่าง** — SML reject เมื่อ `unit_code=""`
ตั้ง fallback เช่น `SHOPEE_SML_UNIT_CODE=ถุง`

---

## VAT Types

| vat_type | ความหมาย |
|---|---|
| 0 | แยกนอก (ราคาก่อน VAT + VAT แยก) |
| 1 | รวมใน (ราคารวม VAT แล้ว) |
| 2 | ศูนย์% (ไม่มี VAT) |

---

## Saleorder Payload (default retry route)

Shopee sale bills ส่งผ่าน `saleorder_client.go` โดย default endpoint เป็น SML 248 `v3/api/saleorder` และใช้ `items` array

```json
{
  "doc_no": "BF-SO2605xxxx",
  "doc_format_code": "INV",
  "doc_date": "2026-05-06",
  "cust_code": "AR00004",
  "items": [
    {
      "item_code": "CON-01000",
      "unit_code": "ถุง",
      "wh_code": "WH-01",
      "shelf_code": "SH-01",
      "qty": 2,
      "price": 100
    }
  ]
}
```

## Saleinvoice Payload (v4 endpoint)

```json
{
  "doc_no": "250424SHOPEE001",
  "doc_format_code": "INV",
  "doc_date": "2026-04-24",
  "cust_code": "AR00004",
  "is_permium": 0,
  "vat_type": 0,
  "details": [
    {
      "item_code": "CON-01000",
      "unit_code": "ถุง",
      "wh_code": "WH-01",
      "shelf_code": "SH-01",
      "qty": 2,
      "price_exclude_vat": 93.46,
      "sum_amount_exclude_vat": 186.92
    }
  ]
}
```

⚠️ หมายเหตุสำคัญ:
- key ต้องเป็น **`"details"`** ไม่ใช่ `"items"`
- `is_permium` เป็น **int** (0/1) ไม่ใช่ bool — typo ตาม SML API จริง
- ใช้ path `POST /SMLJavaRESTService/saleinvoice/v4` เมื่อ `channel_defaults.endpoint` มีคำว่า `saleinvoice`
- ไม่มี `qty` field แยก (ราคาคำนวณไว้ใน `price_exclude_vat` และ `sum_amount_exclude_vat`)

---

## Retry Policy

- max 3 ครั้ง
- ถ้า fail ทั้ง 3 ครั้ง → bill `status='failed'` บันทึกลง DB
- สามารถ retry ด้วยตนเองผ่าน `POST /api/bills/:id/retry` ใน Web UI

---

## สิ่งที่ต้องทำก่อน Go-Live (Phase 4a)

1. ตั้ง `SHOPEE_SML_UNIT_CODE=ถุง` (หรือหน่วยที่เหมาะสม) ใน `/home/bosscatdog/billflow/.env`
2. ใช้ไฟล์ Excel ที่มี SKU จริง (CON-xxxxx / STEEL-xxxxx ฯลฯ) — ไม่ใช่ REST-00002
3. `docker compose up -d backend` (ไม่ต้อง build ใหม่)
4. ทดสอบที่ `http://192.168.2.109:3010/import/shopee`

**ตรวจสอบ SKU ที่มีใน SML 248:**
```bash
docker run --rm postgres:16-alpine psql \
  'postgresql://postgres:sml@192.168.2.248:5432/sml1_2026' \
  -c "SELECT code, name_1, unit_standard FROM ic_inventory ORDER BY code LIMIT 20;"
```

---

## ไฟล์ที่เกี่ยวข้อง

| ไฟล์ | หน้าที่ |
|---|---|
| `backend/internal/handlers/shopee_import.go` | GetConfig, Preview, Confirm handlers |
| `backend/internal/services/sml/saleorder_client.go` | REST client default สำหรับ Shopee sale |
| `backend/internal/services/sml/saleinvoice_client.go` | REST client สำหรับ `POST /SMLJavaRESTService/saleinvoice/v4` |
| `frontend/src/pages/ShopeeImport.tsx` | UI page + config dialog + preview table |
| `backend/cmd/server/main.go` | routes: shopee-config, shopee/preview, shopee/confirm |
