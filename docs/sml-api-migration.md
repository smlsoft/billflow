# SML API Migration — billflow main

## เป้าหมาย
เปลี่ยนจากเรียก SML REST API (192.168.2.248:8080) โดยตรง  
→ ไปใช้ **sml-api-bybos** (192.168.2.109:8200) เป็น proxy กลาง

sml-api-bybos expose endpoints ที่ `/api/v1/` ทั้งหมด  
docs: http://192.168.2.109:8200/docs

---

## สถานะ (2026-05-16)

### ✅ เสร็จแล้ว — SML clients (services/sml/)

| ไฟล์ | path เดิม | path ใหม่ |
|---|---|---|
| `saleorder_client.go` | `POST /SMLJavaRESTService/v3/api/saleorder` | `POST /api/v1/ic/sale-orders` |
| `saleinvoice_client.go` | `POST /SMLJavaRESTService/restapi/saleinvoice` | `POST /api/v1/ic/sale-invoices` |
| `purchaseorder_client.go` | `POST /SMLJavaRESTService/v3/api/purchaseorder` | `POST /api/v1/ic/purchase-orders` |
| `product_client.go` | `POST /SMLJavaRESTService/v3/api/product` | `POST /api/v1/ic/products` |
| `party_client.go` | `GET /SMLJavaRESTService/v3/api/customer\|supplier` | `GET /api/v1/ar/customers`, `/api/v1/ap/suppliers` |
| `warehouse_client.go` | `GET /SMLJavaRESTService/warehouse/v4` | `GET /api/v1/ic/warehouses` |

`BaseURL` ของ clients เหล่านี้ตอนนี้รับค่าจาก `cfg.ShopeeSMLURL`  
ซึ่งใน `.env` ต้องตั้งเป็น `http://192.168.2.109:8200` แทน `http://192.168.2.248:8080`

---

### ⚠️ ยังไม่เสร็จ — ส่วนที่ยังชี้ตรงไป SML 248

ส่วนเหล่านี้ยังใช้ `cfg.ShopeeSMLURL` แต่ส่งแบบ old compat path ผ่าน struct ที่ build URL เอง:

| ไฟล์ | หน้าที่ | หมายเหตุ |
|---|---|---|
| `handlers/bills.go` | Retry บิล (sale/purchase) | ใช้ clients ที่ migrate แล้ว ✅ ผ่าน BaseURL |
| `handlers/shopee_import.go` | Import Shopee Excel | ใช้ clients ที่ migrate แล้ว ✅ ผ่าน BaseURL |
| `handlers/lazada_import.go` | Import Lazada Excel | ใช้ clients ที่ migrate แล้ว ✅ ผ่าน BaseURL |
| `handlers/tiktok_import.go` | Import TikTok Excel | ใช้ clients ที่ migrate แล้ว ✅ ผ่าน BaseURL |
| `services/catalog/service.go` | Sync catalog จาก SML | **⚠️ เรียก SML 248 โดยตรง** ต้องแก้ |
| `config/config.go` | default URL | default ยังเป็น 192.168.2.248 ต้องเปลี่ยนใน .env |

---

## วิธีเปิดใช้งาน sml-api-bybos

### 1. แก้ `.env` บน server

```bash
# เดิม
SHOPEE_SML_URL=http://192.168.2.248:8080

# ใหม่
SHOPEE_SML_URL=http://192.168.2.109:8200
```

> **Auth headers** (GUID, provider, configFileName, databaseName) ยังส่งเหมือนเดิม  
> sml-api-bybos รับทั้ง `X-Api-Key` และ `guid` header — billflow ส่ง `guid` อยู่แล้ว ✅

### 2. Restart backend

```bash
cd ~/billflow && docker compose up -d --build backend
```

### 3. ตรวจสอบ

```bash
# ดู log ว่า retry บิลไปที่ไหน
docker logs billflow-backend --tail=50 | grep "sml\|POST\|BaseURL"

# ทดสอบ retry บิล 1 ใบ จาก /bills UI
```

---

## สิ่งที่ยังต้องทำก่อน production

- [ ] **catalog/service.go** — ยัง sync catalog จาก 192.168.2.248 โดยตรง  
  ต้องเปลี่ยนให้ใช้ `GET /api/v1/ic/products` ของ sml-api-bybos แทน
- [ ] ทดสอบ retry บิลจริงผ่าน UI ว่า doc ขึ้นใน SML ครบ
- [ ] ทดสอบ import Shopee/Lazada/TikTok Excel ครบ flow

---

## sml-api-bybos

- **URL**: http://192.168.2.109:8200  
- **Swagger**: http://192.168.2.109:8200/docs  
- **Branch**: `feature/v1-unified-api` (local: `~/dev/sml-api-bybos`)  
- **Auth**: header `guid` หรือ `X-Api-Key` + header `databaseName` หรือ `X-Tenant`

### Endpoints ที่ billflow ใช้

```
POST /api/v1/ic/sale-orders        ← saleorder
POST /api/v1/ic/sale-invoices      ← saleinvoice
POST /api/v1/ic/purchase-orders    ← purchaseorder
POST /api/v1/ic/products           ← create product
GET  /api/v1/ic/products/:code     ← lookup product
GET  /api/v1/ar/customers          ← party master customers
GET  /api/v1/ap/suppliers          ← party master suppliers
GET  /api/v1/ic/warehouses         ← warehouses
```

---

## henna / thaisunsport

**ห้ามแตะ** — ยังใช้ `SHOPEE_SML_URL=http://192.168.2.248:8080` โดยตรง  
จะ migrate หลังจาก billflow main ทดสอบผ่านแล้ว
