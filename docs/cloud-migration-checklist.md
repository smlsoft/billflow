# Cloud Migration Checklist — BillFlow Thaisunsport

ใช้เมื่อย้ายจาก server dev (`192.168.2.109`) ไปยัง Cloud ใหม่

---

## ก่อนเริ่ม (Pre-Migration)

- [ ] ได้รับ IP address และ SSH credential จาก cloud provider แล้ว
- [ ] แจ้งลูกค้าว่าจะมี downtime ประมาณ 30–60 นาที
- [ ] เลือกช่วงเวลานอกเวลาทำการ (แนะนำคืนวันศุกร์)

---

## Step 1 — เตรียม Cloud VM ใหม่

```bash
# SSH เข้า VM ใหม่
ssh root@<NEW_SERVER_IP>

# อัพเดต OS
apt update && apt upgrade -y

# ติดตั้ง Docker
curl -fsSL https://get.docker.com | sh
systemctl enable docker
systemctl start docker

# ติดตั้ง Docker Compose v2
apt install -y docker-compose-plugin

# ติดตั้ง cloudflared
curl -L https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-amd64 -o /usr/local/bin/cloudflared
chmod +x /usr/local/bin/cloudflared

# ตรวจสอบ
docker --version
docker compose version
cloudflared --version
```

---

## Step 2 — Backup ข้อมูลจาก Server เดิม

```bash
# SSH เข้า server เดิม
ssh bosscatdog@192.168.2.109

# Backup PostgreSQL (thaisunsport)
docker exec billflow-thaisunsport-postgres \
  pg_dump -U billflow billflow \
  > /tmp/billflow-thaisunsport-backup-$(date +%Y%m%d-%H%M%S).sql

# ตรวจสอบขนาด backup (ต้องไม่ใช่ 0 bytes)
ls -lh /tmp/billflow-thaisunsport-backup-*.sql

# Backup artifacts (PDF/เอกสาร)
docker cp billflow-thaisunsport-backend:/app/artifacts /tmp/artifacts-backup-$(date +%Y%m%d)
```

---

## Step 3 — คัดลอกไฟล์ไปยัง Cloud ใหม่

```bash
# จาก local machine — copy backup จาก server เดิมไปยัง cloud ใหม่
scp bosscatdog@192.168.2.109:/tmp/billflow-thaisunsport-backup-*.sql ./
scp -r bosscatdog@192.168.2.109:/tmp/artifacts-backup-* ./

# copy ไปยัง cloud ใหม่
scp billflow-thaisunsport-backup-*.sql root@<NEW_SERVER_IP>:/tmp/
scp -r artifacts-backup-* root@<NEW_SERVER_IP>:/tmp/

# copy โค้ด BillFlow (จาก local dev)
rsync -avz --exclude='.git' \
  /Users/nontawatwongnuk/dev_bos/billflow/ \
  root@<NEW_SERVER_IP>:/home/billflow-thaisunsport/
```

---

## Step 4 — ตั้งค่า .env บน Cloud ใหม่

```bash
# SSH เข้า cloud ใหม่
ssh root@<NEW_SERVER_IP>

cd /home/billflow-thaisunsport
cp .env.example .env
nano .env
```

**ค่าที่ต้องตั้งใหม่:**

```bash
# Server
PORT=8100
PROJECT_NAME=billflow-thaisunsport

# Database
DATABASE_URL=postgres://billflow:CHANGE_ME@localhost:5448/billflow

# JWT (copy จากเดิม — ห้ามเปลี่ยน จะทำให้ session ทุกคน logout)
JWT_SECRET=<copy จาก server เดิม>

# Telegram alerts
TELEGRAM_BOT_TOKEN=8934103811:AAFWKsIiszPW3BoHyM13KXw-LIwAqrqmgsY
TELEGRAM_CHAT_ID=7548005041

# AI
OPENROUTER_API_KEY=<copy จาก server เดิม>
OPENROUTER_MODEL=google/gemini-2.5-flash-lite
OPENROUTER_FALLBACK_MODEL=google/gemini-2.5-flash
MISTRAL_API_KEY=<copy จาก server เดิม>

# LINE (ถ้าใช้)
LINE_CHANNEL_SECRET=<copy จาก server เดิม>
LINE_CHANNEL_ACCESS_TOKEN=<copy จาก server เดิม>
LINE_ADMIN_USER_ID=<copy จาก server เดิม>

# SML
SHOPEE_SML_URL=http://172.17.0.1:8200   # ⚠️ ต้องตรวจ Docker gateway IP ใหม่บน VM นี้
SHOPEE_SML_GUID=<copy จาก server เดิม>
SHOPEE_SML_PROVIDER=<copy จาก server เดิม>

# PUBLIC_BASE_URL — ตั้งหลังจากได้ Cloudflare Tunnel URL แล้ว
PUBLIC_BASE_URL=

# Phase 1 flags (ห้ามเปลี่ยน)
VITE_PHASE=1
VITE_ENABLE_SALES_ORDERS=false
VITE_ENABLE_SHOPEE_EXCEL=false
VITE_ENABLE_LAZADA_EXCEL=false
VITE_ENABLE_TIKTOK_EXCEL=false
VITE_ENABLE_CHAT=false
```

> ⚠️ ตรวจ Docker gateway IP ก่อน: `ip route | grep default` — อาจเป็น `172.17.0.1` หรือ `172.18.0.1` ขึ้นอยู่กับ VM

---

## Step 5 — Restore ข้อมูล

```bash
# รัน PostgreSQL ก่อน
docker compose up -d postgres

# รอ 10 วินาทีให้ DB พร้อม
sleep 10

# Restore backup
cat /tmp/billflow-thaisunsport-backup-*.sql | \
  docker exec -i billflow-thaisunsport-postgres \
  psql -U billflow billflow

# Restore artifacts
docker cp /tmp/artifacts-backup-*/. billflow-thaisunsport-backend:/app/artifacts/
```

---

## Step 6 — Build และ Start ระบบ

```bash
cd /home/billflow-thaisunsport

# Build และ start ทุก container
docker compose build --no-cache
docker compose up -d

# ดู log ว่าขึ้นปกติ
docker compose logs -f --tail=50
```

---

## Step 7 — ตรวจสอบ Health

```bash
# Backend health
curl http://localhost:8100/health
# ต้องได้: {"status":"ok","db":"ok",...}

# Frontend
curl -I http://localhost:3020
# ต้องได้: HTTP/1.1 200 OK

# ดู containers ทั้งหมด
docker ps --format '{{.Names}} {{.Status}}'
```

---

## Step 8 — ตั้ง Cloudflare Tunnel

```bash
# เริ่ม Quick Tunnel (ชั่วคราวระหว่างทดสอบ)
nohup cloudflared tunnel \
  --url http://127.0.0.1:3020 \
  --no-autoupdate \
  > /tmp/billflow-thaisunsport-tunnel.log 2>&1 &

# รอ 5 วินาที แล้วดู URL
sleep 5
grep -oE 'https://[a-z0-9-]+\.trycloudflare\.com' \
  /tmp/billflow-thaisunsport-tunnel.log | tail -1
```

```bash
# อัพเดต PUBLIC_BASE_URL ใน .env ด้วย URL ที่ได้
nano .env
# แก้: PUBLIC_BASE_URL=https://xxxx.trycloudflare.com

# Restart backend เพื่อโหลด URL ใหม่
docker compose restart backend
```

---

## Step 9 — ทดสอบ End-to-End

- [ ] เปิด Tunnel URL ใน browser → หน้า login โหลดได้
- [ ] Login ด้วย admin account → เข้าได้
- [ ] ดูหน้า `/bills` → ข้อมูลเก่าครบ
- [ ] ดูหน้า `/settings/email` → IMAP accounts ยังอยู่
- [ ] ดูหน้า `/settings/channels` → channel defaults ยังอยู่
- [ ] ดูหน้า `/settings/instance` → SML config ยังอยู่
- [ ] ทดสอบ manual email poll → ดึง email ได้ปกติ
- [ ] Telegram alert ทำงาน → ทดสอบโดย restart backend แล้วดูว่ามี log

---

## Step 10 — ตั้ง UptimeRobot (ถ้ายังไม่ได้ทำ)

1. ไปที่ uptimerobot.com → Add Monitor
2. Type: `HTTP(s)`
3. URL: `https://<TUNNEL_URL>/health`
4. Interval: 5 นาที
5. Alert: Email หรือ Telegram webhook
6. ทดสอบ: กด "Test" ต้องได้ UP

---

## Step 11 — ปิด Instance เดิม (หลังยืนยันว่า cloud ใหม่ OK)

```bash
# SSH เข้า server เดิม
ssh bosscatdog@192.168.2.109

# หยุด thaisunsport instance เดิม
cd ~/billflow-thaisunsport
docker compose down

# เก็บ backup ไว้อีก 7 วัน ก่อนลบ
```

---

## Rollback Plan

ถ้ามีปัญหาหลัง migrate ไป cloud ใหม่:

```bash
# เปิด instance เดิมกลับมา
ssh bosscatdog@192.168.2.109
cd ~/billflow-thaisunsport
docker compose up -d

# เปิด Tunnel เดิม
nohup cloudflared tunnel \
  --url http://127.0.0.1:3020 \
  --no-autoupdate \
  > /tmp/billflow-thaisunsport-tunnel.log 2>&1 &
```

> ⚠️ ข้อมูลที่บันทึกบน cloud ใหม่ระหว่าง migrate จะไม่อยู่บน server เดิม — rollback ควรทำเฉพาะกรณีฉุกเฉินก่อนลูกค้าเริ่มใช้งานจริง

---

## ข้อมูลอ้างอิง

| รายการ | ค่า |
|---|---|
| Server เดิม | `192.168.2.109` |
| Folder เดิม | `~/billflow-thaisunsport` |
| Backend port | `8100` |
| Frontend port | `3020` |
| PostgreSQL port | `5448` |
| Container prefix | `billflow-thaisunsport-*` |
| SML tenant | `data1_test` |
| SML DB host | `thaisunsport.thddns.net:9983` |

---

*จัดทำโดย BillFlow Team | 26 มิถุนายน 2568*
