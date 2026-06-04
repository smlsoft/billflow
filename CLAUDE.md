# CLAUDE.md — BillFlow

> **local workspace:** `/Users/nontawatwongnuk/dev_bos/billflow`
> **server:** `192.168.2.109` | user `bosscatdog` | deploy folder `~/billflow`
> DB schema → `backend/internal/database/migrations/` | Routes → `backend/cmd/server/main.go`
> Current state / session history → `docs/current-state.md`

---

## 1. Project

**BillFlow** — ระบบ AI-assisted bill entry สำหรับ SME ลดเวลาคีย์บิลจาก 100+ บิล/วัน → เกือบ 0

**Input channels:** LINE OA (human chat) · Email IMAP · Shopee Open API · Shopee/Lazada/TikTok Excel
**Output:** สร้าง/ส่งบิลเข้า SML ERP โดยอัตโนมัติ

**Tech stack:**
```
Backend:   Go 1.24 + Gin
Frontend:  React + Vite + TypeScript + Tailwind + shadcn/ui
Database:  PostgreSQL 16
AI:        OpenRouter (gemini-2.5-flash primary / claude-3-5-haiku fallback) + Mistral OCR
Deploy:    Docker Compose + Cloudflare Tunnel
```

---

## 2. Instances on Server 192.168.2.109

| Instance | Folder | Backend | Frontend | PostgreSQL | Tunnel log |
|---|---|---|---|---|---|
| `billflow` (main/demo) | `~/billflow` | 8090 | 3010 | 5438 | `/tmp/billflow-tunnel.log` |
| `billflow-thaisunsport` | `~/billflow-thaisunsport` | 8100 | 3020 | 5448 | `/tmp/billflow-thaisunsport-tunnel.log` |

> ⚠️ **`billflow-henna` ย้ายเป็น Nexflow แล้ว (2026-05-31)**
> folder `~/billflow-henna` บน server ปัจจุบันรัน **Nexflow** ไม่ใช่ billflow
> repo แยก: `https://github.com/bosocmputer/Nexflow.git` | local: `/Users/nontawatwongnuk/dev_bos/Nexflow`
> containers: `nexflow-postgres`, `nexflow-backend`, `nexflow-frontend`
> **ห้าม deploy billflow ไปที่ `~/billflow-henna`** — ต้อง deploy จาก Nexflow repo เท่านั้น

**ห้ามกระทบ projects อื่นบน server:**
| Project | Ports |
|---|---|
| openclaw-admin | 3000, 5432 |
| tcc | 8080, 5433, 9092, 8123, 9000, 6382 |
| ledgioai | 3004, 5436, 6381 |
| centrix | 3002, 5001, 5434, 6380 |
| sml-api-bybos | 8200 |

**Quick health check:**
```bash
curl http://192.168.2.109:8090/health   # main
curl http://192.168.2.109:8100/health   # thaisunsport
# port 8110 = Nexflow (ไม่ใช่ billflow แล้ว)
```

---

## 3. Deploy Policy

- ดู `docs/deploy-instances.md` สำหรับ full policy
- **shared backend/logic fix** → deploy เฉพาะ `billflow` (main) + `billflow-thaisunsport` เท่านั้น
- **Thaisunsport** = Phase 1 purchase-only: `VITE_PHASE=1`, sales/Shopee/Lazada/TikTok/chat disabled
- ⚠️ **Henna ย้ายเป็น Nexflow แล้ว** — ถ้าต้องแก้ henna ต้อง deploy จาก repo Nexflow เท่านั้น
- deploy: `rsync` backend/ + frontend/ → `docker compose build + up -d` บน server

---

## 4. SML ERP — Critical Gotchas

### Routing ผ่าน sml-api-bybos gateway
```
URL (จาก backend container): http://172.24.0.1:8200
x-tenant header: sml1_2026 (main) | aoy (henna) | data1_test (thaisunsport)
```

### Channel routing (bills.go Retry):
| source | bill_type | SML path |
|---|---|---|
| line / email / lazada / tiktok | sale | sale_reserve (JSON-RPC) |
| shopee / shopee_email | sale | saleorder (REST v3) |
| shopee_shipped | purchase | purchaseorder (REST v3) |
| any | any | endpoint จาก channel_defaults.endpoint override ได้ |

### ⚠️ Critical bugs ที่แก้ไปแล้ว — ห้าม revert:
1. **mojibake**: SML Java อ่าน body เป็น Latin-1 ไม่ว่า charset header จะเป็นอะไร → ใช้ `marshalASCII()` ใน `sml/json_ascii.go` escape non-ASCII เป็น `\uXXXX` ในทุก POST client (6 clients) — อย่าแทน `json.Marshal` โดยตรงใน SML clients
2. **doc_no bug**: pattern `prefix-YYYY...` หรือ `prefix-YY...` → SML รับแต่ไม่แสดงใน UI → ใช้ `BF-SO` + `YYMM####` → `BF-SO260400001` ✅
3. **purchaseorder doc_no**: v3 endpoint ไม่ auto-generate → ต้องส่ง doc_no เสมอ
4. **saleinvoice fields**: ใช้ `"details"` ไม่ใช่ `"items"`, `"is_permium": 0` (int, typo intentional)
5. **sale_reserve response**: JSON ซ้อน 2 ชั้น → ต้อง parse 2 รอบ
6. **cust_code**: ต้องมาจาก `channel_defaults` table เสมอ — ไม่ hardcode .env
7. **doc_no reuse on retry**: `bills.go` บันทึก `sml_doc_no` ก่อน call SML → retry ใช้ doc_no เดิม (ไม่ increment counter)
8. **Docker networking**: SML URL จาก backend container ต้องใช้ `172.24.0.1` ไม่ใช่ `localhost`

### ⚠️ channel_defaults ต้องตั้งก่อนใช้งาน:
- ตารางว่าง → SML retry error ทันที
- ตั้งค่าที่ `/settings/channels` → ปุ่ม "ตั้งค่าอัตโนมัติ" (pair AR00001-04)
- per-channel: wh_code, shelf_code, vat_type, vat_rate (sentinel '' / -1 = ใช้ env fallback)

---

## 5. Email IMAP — Gotchas

- inbox จัดการผ่าน `/settings/email` UI → DB (`imap_accounts`) — ไม่มี IMAP_* ใน .env
- `poll_interval_seconds >= 300` (DB CHECK enforce)
- Gmail = App Password เท่านั้น (2FA ต้องเปิดก่อน)
- subject "ถูกจัดส่งแล้ว" / "ยืนยันการชำระเงิน" → ShopeeShipped → purchaseorder (SML)
- dedup ผ่าน `processed_email_keys` table (persistent ข้าม restart)
- `consecutive_failures >= 3` → LINE admin notify (throttle 1h/inbox)

---

## 6. LINE OA — Gotchas

- multi-OA: webhook URL `/webhook/line/:oaId` ต่อ OA — ต้องตั้งใน LINE Developer Console
- credentials ใน `line_oa_accounts` table (DB) — default OA seed จาก `LINE_*` .env ตอน boot ครั้งแรก
- **Reply API ฟรีไม่นับ quota** → Hybrid: try Reply token ก่อน → fallback Push
- Push quota = 200/เดือน (Free OA Light Plan)
- SSE endpoint `/api/admin/events` ใช้ HMAC token auth (EventSource ไม่ support custom headers)
- `mark_as_read_enabled` ต่อ OA (LINE Premium feature เท่านั้น — Free OA return 403)

---

## 7. Shopee Open API (Nexflow เท่านั้น — ไม่ใช่ billflow แล้ว)

> ⚠️ Henna ย้ายเป็น Nexflow แล้ว — Shopee Open API config อยู่ใน Nexflow repo

- `SHOPEE_OPEN_API_ENABLED=true` ใน Nexflow .env; main/thaisunsport = false
- OAuth callback `/api/shopee-api/callback` — state token auth
- connections ใน `shopee_api_connections` table (multi-shop)
- import flow: Shopee API → local bills → review/mapping → retry/bulk SML (เหมือน Excel flow)
- **settlement flow ≠ bulk-send**: `/shopee-settlements` → reconcile → send AR receipt (`RC`) to SML แยก pipeline

---

## 8. Feature Flags (build args)

```
VITE_PHASE=99 (all) | 1 (purchase-only — Thaisunsport)
VITE_ENABLE_SALES_ORDERS   → /sales-orders, /sale-invoices, /marketplace-aliases
VITE_ENABLE_SHOPEE_EXCEL   → /import/shopee, /shopee-settlements
VITE_ENABLE_LAZADA_EXCEL   → /import/lazada
VITE_ENABLE_TIKTOK_EXCEL   → /import/tiktok
VITE_ENABLE_CHAT           → /messages, /settings/line-oa, quick-replies, chat-tags
VITE_ENABLE_REMARK2=false  (ยังไม่เปิด)
```

---

## 9. Key Env Vars

```bash
DATABASE_URL=postgres://billflow:pass@localhost:5438/billflow
JWT_SECRET=<min 32 chars>

# LINE
LINE_CHANNEL_SECRET=  LINE_CHANNEL_ACCESS_TOKEN=  LINE_ADMIN_USER_ID=
PUBLIC_BASE_URL=      # Cloudflare tunnel URL — ต้อง reachable by LINE servers

# AI
OPENROUTER_API_KEY=  OPENROUTER_MODEL=google/gemini-2.5-flash
MISTRAL_API_KEY=     # PDF extraction

# SML (ตั้งได้ผ่าน /settings/instance UI → เก็บใน app_settings DB)
SHOPEE_SML_URL=http://172.24.0.1:8200   # Docker gateway → sml-api-bybos
SHOPEE_SML_GUID=  SHOPEE_SML_PROVIDER=  SHOPEE_SML_CONFIG_FILE=  SHOPEE_SML_DATABASE=
SHOPEE_SML_WH_CODE=WH-01  SHOPEE_SML_SHELF_CODE=SH-01  SHOPEE_SML_UNIT_CODE=ถุง

# Shopee Open API (Henna only)
SHOPEE_OPEN_API_ENABLED=false  SHOPEE_OPEN_API_PARTNER_ID=  SHOPEE_OPEN_API_PARTNER_KEY=
```

> ⚠️ `SeedFromEnv()` ถูกลบแล้ว — fresh install ต้องตั้งค่า SML/LINE/AI ผ่าน `/settings/instance` UI

---

## 10. Cloudflare Tunnel

```bash
# เริ่ม Quick Tunnel (URL เปลี่ยนทุกครั้ง restart)
nohup cloudflared tunnel --url http://127.0.0.1:3010 --no-autoupdate > /tmp/billflow-tunnel.log 2>&1 &
grep -oE 'https://[a-z0-9-]+\.trycloudflare\.com' /tmp/billflow-tunnel.log | tail -1

# หลัง restart ต้องอัป PUBLIC_BASE_URL ใน .env แล้ว restart backend
```

---

## 11. Roles

| Role | สิทธิ์ |
|---|---|
| admin | ทุกอย่าง รวม settings/users/purge |
| staff | ดู/confirm bills, import, mappings |
| viewer | read-only bills + dashboard |

---

> DB schema (54 migrations) → `backend/internal/database/migrations/`
> Full API route list → `backend/cmd/server/main.go`
> Deploy history → `docs/current-state.md` + `docs/deploy-instances.md`
