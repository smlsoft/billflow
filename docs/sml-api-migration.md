# SML API Migration — billflow main

## เป้าหมาย
เปลี่ยนจากเรียก SML REST API (192.168.2.248:8080) โดยตรง  
→ ไปใช้ **sml-api-bybos** (localhost:8200) เป็น proxy กลาง

sml-api-bybos expose endpoints ที่ `/api/v1/` ทั้งหมด  
Swagger docs: http://192.168.2.109:8200/docs

---

## สถานะ (2026-05-16) — ✅ พร้อมทดสอบ

### การเปลี่ยนแปลงที่ทำแล้ว

| ส่วน | สิ่งที่เปลี่ยน |
|---|---|
| `services/sml/saleorder_client.go` | path: `/SMLJavaRESTService/v3/api/saleorder` → `/api/v1/ic/sale-orders` |
| `services/sml/saleinvoice_client.go` | path: `/SMLJavaRESTService/restapi/saleinvoice` → `/api/v1/ic/sale-invoices` |
| `services/sml/purchaseorder_client.go` | path: `/SMLJavaRESTService/v3/api/purchaseorder` → `/api/v1/ic/purchase-orders` |
| `services/sml/product_client.go` | path: `/SMLJavaRESTService/v3/api/product` → `/api/v1/ic/products` |
| `services/sml/party_client.go` | path: customer/supplier → `/api/v1/ar/customers`, `/api/v1/ap/suppliers` |
| `services/sml/warehouse_client.go` | path: `/SMLJavaRESTService/warehouse/v4` → `/api/v1/ic/warehouses` |
| `services/catalog/service.go` | path: `/product/v4` → `/api/v1/ic/products` |
| `config/config.go` | default `ShopeeSMLURL` ยังเป็น 192.168.2.248 แต่ .env บน server override แล้ว |
| **server `.env`** | `SHOPEE_SML_URL=http://172.24.0.1:8200` ✅ |

### Auth headers — ไม่ต้องเปลี่ยน

billflow ส่ง `guid` + `databaseName` — sml-api-bybos รับทั้งสองแบบ:
- `guid: smlx` → ผ่าน API key check (`API_KEYS=dev-key,smlx`)
- `databaseName: SML1_2026` → ผ่าน tenant check

---

## Endpoints ที่เปลี่ยน

| เดิม (SML 248 direct) | ใหม่ (sml-api-bybos) |
|---|---|
| `POST /SMLJavaRESTService/v3/api/saleorder` | `POST /api/v1/ic/sale-orders` |
| `POST /SMLJavaRESTService/restapi/saleinvoice` | `POST /api/v1/ic/sale-invoices` |
| `POST /SMLJavaRESTService/v3/api/purchaseorder` | `POST /api/v1/ic/purchase-orders` |
| `POST /SMLJavaRESTService/v3/api/product` | `POST /api/v1/ic/products` |
| `GET /SMLJavaRESTService/v3/api/product/{code}` | `GET /api/v1/ic/products/{code}` |
| `GET /SMLJavaRESTService/v3/api/customer` | `GET /api/v1/ar/customers` |
| `GET /SMLJavaRESTService/v3/api/supplier` | `GET /api/v1/ap/suppliers` |
| `GET /SMLJavaRESTService/warehouse/v4` | `GET /api/v1/ic/warehouses` |
| `GET /SMLJavaRESTService/product/v4` (catalog sync) | `GET /api/v1/ic/products` |

---

## การตั้งค่า .env บน server

```
# ~/billflow/.env (ตั้งค่าแล้ว)
SHOPEE_SML_URL=http://172.24.0.1:8200   ← sml-api-bybos ผ่าน Docker gateway จาก billflow-backend container
SHOPEE_SML_GUID=smlx
SHOPEE_SML_PROVIDER=SMLGOH
SHOPEE_SML_CONFIG_FILE=SMLConfigSMLGOH.xml
SHOPEE_SML_DATABASE=SML1_2026
```

---

## ขั้นตอนทดสอบ

1. เปิด http://192.168.2.109:3010/bills
2. เลือกบิลที่ status = `pending` หรือ `failed`
3. กด Retry → ดู log

```bash
docker logs billflow-backend --tail=50 | grep -i "sml\|retry\|error"
```

4. ตรวจว่า doc ขึ้นใน SML UI
5. ทดสอบ import Shopee/Lazada/TikTok Excel ครบ flow

---

## สิ่งที่ยังไม่แตะ

| Project | URL | สถานะ |
|---|---|---|
| billflow-henna | :3030 | ยังใช้ 192.168.2.248:8080 โดยตรง — **ห้ามแตะ** |
| billflow-thaisunsport | - | ยังใช้ 192.168.2.248:8080 โดยตรง — **ห้ามแตะ** |

จะ migrate henna/thaisunsport หลังจาก billflow main ทดสอบผ่านแล้ว

---

## sml-api-bybos reference

- **Container**: `sml-api-bybos-sml-api-1` (healthy)
- **Port**: 8200
- **Health**: `curl http://localhost:8200/health` → `{"status":"ok"}`
- **Swagger**: http://192.168.2.109:8200/docs
- **Source**: `/home/bosscatdog/sml-api-bybos`
- **API_KEYS**: `dev-key,smlx`

## Latest verification — 2026-05-18

- Deployed `sml-api-bybos` to `192.168.2.109:8200`.
- Swagger UI serves local embedded official Swagger UI assets, not CDN; OpenAPI is available at `/docs/openapi.json` and `/openapi.json`.
- Live smoke passed: `/health`, `/health/ready` with `SML1_2026`, `/docs`, `/openapi.json`.
- Master reads passed on `SML1_2026`: customers `1004`, suppliers `500`, products `3005`, warehouses `4`, shelves `4`.
- Golden writes passed:
  - SO `BF-APIQA-SO-260518-001` (`trans_flag=36`)
  - SI `BF-APIQA-SI-260518-001` (`trans_flag=44`)
  - PO `BF-APIQA-PO-260518-001` (`trans_flag=6`)
- Product create passed: `BFAPIQAPRD260518001`; duplicate returns `duplicate_product_code`.
- BillFlow main backend was rebuilt/restarted with the new response parser; startup cache shows `warehouse_cache_refreshed warehouses=4 shelves=4` and `party_cache_refreshed customers=1004 suppliers=500`.
