# Cloud Infrastructure Requirement — BillFlow

**วันที่:** 25 มิถุนายน 2568  
**เวอร์ชัน:** 1.0  
**วัตถุประสงค์:** RFQ สำหรับสอบถามราคา Cloud VPS/VM เพื่อ deploy ระบบ BillFlow บน Cloud

---

## 1. ภาพรวมระบบ

**BillFlow** คือ Web Application สำหรับธุรกิจ SME ช่วยจัดการใบสั่งซื้อและส่งข้อมูลเข้า ERP อัตโนมัติด้วย AI  
**ลักษณะการใช้งาน:** Internal business tool — ทีมงานภายในบริษัท 5–20 คน ใช้งานในเวลาทำการ ไม่ใช่ระบบ public high-traffic

### Architecture

```
Internet
   │
   ▼
Cloudflare Tunnel (HTTPS — ฟรี, ไม่ต้องเปิด port 80/443)
   │
   ▼
Cloud VM (Ubuntu + Docker Compose)
   ├── Frontend   Nginx + React SPA       port 80 (internal)
   ├── Backend    Go 1.24 API Server      port 8090 (internal)
   └── Database   PostgreSQL 16 Alpine    port 5432 (internal)
```

### External Connections (Outbound HTTPS เท่านั้น)

| ปลายทาง | วัตถุประสงค์ |
|---|---|
| OpenRouter API | AI processing (Google Gemini) |
| Mistral API | PDF OCR extraction |
| Gmail IMAP | รับ email ใบสั่งซื้อ (poll ทุก 5 นาที) |
| Cloudflare | HTTPS Tunnel endpoint |
| SML ERP (on-premise ลูกค้า) | ส่งข้อมูลบิลเข้า ERP |

---

## 2. Compute Specification

### Resource ที่ใช้จริง (วัดจาก production)

| Container | Memory Limit | RAM ใช้จริง |
|---|---|---|
| Backend (Go) | 512 MB | ~80 MB |
| Frontend (Nginx) | 256 MB | ~8 MB |
| PostgreSQL 16 | 512 MB | ~155 MB |
| OS + Docker daemon | — | ~300 MB |
| **รวม** | **1,280 MB** | **~540 MB** |

### VM Specification ที่ต้องการ

| | Minimum | **Recommended** |
|---|---|---|
| vCPU | 1 core | **2 vCPU** |
| RAM | 2 GB | **4 GB** |
| SSD Storage | 40 GB | **60 GB** |
| Bandwidth | 200 GB/เดือน | **500 GB/เดือน หรือ unmetered** |
| OS | Ubuntu 22.04 LTS | **Ubuntu 22.04 / 24.04 LTS** |

> RAM 4 GB เผื่อ Docker build ระหว่าง deploy และ PostgreSQL growth ในอนาคต

---

## 3. Storage

### การใช้งานจริงปัจจุบัน

| ส่วน | ขนาดปัจจุบัน | คาดการณ์ 1 ปี |
|---|---|---|
| Docker images (3 containers) | ~3 GB | ~3 GB |
| PostgreSQL data | 155 MB | ~1 GB |
| Daily backup pg_dump (30 วัน) | 1.2 GB | ~3 GB |
| Artifacts PDF/attachments | 549 MB | ~3 GB |
| OS + packages + logs | ~5 GB | ~8 GB |
| **รวม** | **~10 GB** | **~18 GB** |

**ต้องการ:** SSD 60 GB — เพียงพอสำหรับ 2–3 ปีโดยไม่ขยาย  
**ประเภท I/O:** Standard SSD (ไม่จำเป็นต้องเป็น NVMe)  
**ขยาย volume ได้ภายหลัง** จะเป็นประโยชน์มาก

---

## 4. Network

### Inbound Ports ที่ต้องการ

| Port | ใช้งาน | หมายเหตุ |
|---|---|---|
| 22 | SSH Management | จำกัด IP ได้ |
| 80 / 443 | HTTPS | **ไม่จำเป็น** ถ้าใช้ Cloudflare Tunnel |

> ถ้าใช้ **Cloudflare Tunnel** เปิดแค่ port 22 SSH — ไม่ต้องเปิด 80/443 เลย

### Static IP
- **ต้องการ Static IP 1 หมายเลข** — สำหรับ whitelist ใน firewall ของลูกค้า

### Location
- **Asia region** — Singapore / Japan / Thailand (latency กับ Gmail IMAP และ LINE API)

---

## 5. Availability & Backup

| หัวข้อ | ความต้องการ |
|---|---|
| Uptime SLA | 99%+ (critical ช่วงเวลาทำการ) |
| VM Snapshot | **ต้องการ** — อย่างน้อย weekly |
| Snapshot retention | 2–4 สัปดาห์ |
| Application backup | จัดการเองผ่าน pg_dump cron ทุกคืน |
| Maintenance window | แจ้งล่วงหน้าได้ ไม่ต้องเป็น 24/7 |

---

## 6. Access & Management

| หัวข้อ | ความต้องการ |
|---|---|
| Root / sudo access | **ต้องการ** — ติดตั้ง Docker เอง |
| SSH Key authentication | ต้องการ |
| Control Panel | ไม่จำเป็น (มีก็ดี) |
| Basic monitoring alert | CPU / RAM / Disk แจ้งเตือนทาง email |

### Software ที่ติดตั้งเอง (ต้องการแค่ clean OS)

```
Docker Engine 24+
Docker Compose v2
Cloudflare Tunnel (cloudflared)
```

---

## 7. สรุป Spec (TL;DR)

```
VM:        2 vCPU / 4 GB RAM / 60 GB SSD
OS:        Ubuntu 22.04 LTS หรือ 24.04 LTS (64-bit)
Network:   500 GB/เดือน หรือ unmetered + Static IP 1 หมายเลข
Location:  Asia — Singapore / Thailand preferred
Snapshot:  Weekly, retention 2–4 สัปดาห์
Access:    Root SSH
Usage:     Long-term (1 ปีขึ้นไป)
```

---

## 8. คำถามสำหรับ Provider

1. ราคา **รายเดือน** และ **รายปี** (มีส่วนลด annual?)
2. vCPU type และ storage type (SSD / NVMe / shared?)
3. Snapshot — ราคา, จำนวนที่เก็บได้, retention policy
4. Static IP — รวมในแพ็กเกจหรือคิดเพิ่ม?
5. Support SLA — response time ถ้า VM down?
6. ช่วย migrate data จาก server ปัจจุบันได้ไหม?
7. มี trial period ก่อนตัดสินใจไหม?
8. ช่องทางชำระเงิน (credit card / โอนเงิน / promptpay?)
9. สามารถ upgrade spec ได้ภายหลังโดยไม่ downtime นานไหม?

---

*ข้อมูล spec อ้างอิงจากระบบ production จริง ณ วันที่ 25 มิถุนายน 2568*
