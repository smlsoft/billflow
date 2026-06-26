# Cloud Infrastructure Requirement — BillFlow

**Date:** June 25, 2025
**Version:** 1.0
**Purpose:** RFQ document for Cloud VPS/VM providers to host BillFlow in production

---

## 1. System Overview

**BillFlow** is a web application for SME businesses that automates purchase order management and ERP data entry using AI.
**Usage pattern:** Internal business tool — 5–20 staff users during business hours. Not a public-facing high-traffic system.

### Architecture

```
Internet
   │
   ▼
Cloudflare Tunnel (HTTPS — free, no need to expose port 80/443)
   │
   ▼
Cloud VM (Ubuntu + Docker Compose)
   ├── Frontend   Nginx + React SPA       port 80 (internal)
   ├── Backend    Go 1.24 API Server      port 8090 (internal)
   └── Database   PostgreSQL 16 Alpine    port 5432 (internal)
```

### External Connections (Outbound HTTPS only)

| Destination | Purpose |
|---|---|
| OpenRouter API | AI processing (Google Gemini) |
| Mistral API | PDF OCR extraction |
| Gmail IMAP | Receive purchase order emails (poll every 5 minutes) |
| Cloudflare | HTTPS Tunnel endpoint |
| SML ERP (customer on-premise) | Push bill data into ERP |

---

## 2. Compute Specification

### Actual Resource Usage (measured from production)

| Container | Memory Limit | Actual RAM Used |
|---|---|---|
| Backend (Go) | 512 MB | ~80 MB |
| Frontend (Nginx) | 256 MB | ~8 MB |
| PostgreSQL 16 | 512 MB | ~155 MB |
| OS + Docker daemon | — | ~300 MB |
| **Total** | **1,280 MB** | **~540 MB** |

### Required VM Specification

| | Minimum | **Recommended** |
|---|---|---|
| vCPU | 1 core | **2 vCPU** |
| RAM | 2 GB | **4 GB** |
| SSD Storage | 40 GB | **60 GB** |
| Bandwidth | 200 GB/month | **500 GB/month or unmetered** |
| OS | Ubuntu 22.04 LTS | **Ubuntu 22.04 / 24.04 LTS** |

> 4 GB RAM is recommended to accommodate Docker builds during deployment and future PostgreSQL growth.

---

## 3. Storage

### Current Usage (production data)

| Component | Current Size | 1-Year Estimate |
|---|---|---|
| Docker images (3 containers) | ~3 GB | ~3 GB |
| PostgreSQL data | 155 MB | ~1 GB |
| Daily pg_dump backups (30-day retention) | 1.2 GB | ~3 GB |
| Artifacts — PDFs / attachments | 549 MB | ~3 GB |
| OS + packages + logs | ~5 GB | ~8 GB |
| **Total** | **~10 GB** | **~18 GB** |

**Requirement:** 60 GB SSD — sufficient for 2–3 years without expansion
**I/O type:** Standard SSD (NVMe not required)
**Expandable volume** would be a plus

---

## 4. Network

### Required Inbound Ports

| Port | Purpose | Notes |
|---|---|---|
| 22 | SSH Management | Can be IP-restricted |
| 80 / 443 | HTTPS | **Not required** if using Cloudflare Tunnel |

> With **Cloudflare Tunnel**, only port 22 needs to be open — no 80/443 required.

### Static IP
- **1 Static IP required** — for firewall whitelisting on the customer's ERP server

### Location
- **Asia region preferred** — Singapore / Japan / Thailand (lower latency to Gmail IMAP and LINE API servers)

---

## 5. Availability & Backup

| Item | Requirement |
|---|---|
| Uptime SLA | 99%+ (critical during business hours) |
| VM Snapshot | **Required** — at least weekly |
| Snapshot retention | 2–4 weeks |
| Application-level backup | Self-managed via nightly pg_dump cron |
| Maintenance window | Advance notice acceptable; not 24/7 critical |

---

## 6. Access & Management

| Item | Requirement |
|---|---|
| Root / sudo access | **Required** — self-managed Docker installation |
| SSH Key authentication | Required |
| Control Panel | Not required (CLI is sufficient; panel is a plus) |
| Basic monitoring alerts | CPU / RAM / Disk alerts via email |

### Software installed by our team (clean OS only needed)

```
Docker Engine 24+
Docker Compose v2
Cloudflare Tunnel (cloudflared)
```

---

## 7. Specification Summary (TL;DR)

```
VM:        2 vCPU / 4 GB RAM / 60 GB SSD
OS:        Ubuntu 22.04 LTS or 24.04 LTS (64-bit)
Network:   500 GB/month or unmetered + 1 Static IP
Location:  Asia — Singapore / Thailand preferred
Snapshot:  Weekly, 2–4 week retention
Access:    Root SSH
Usage:     Long-term (1 year+)
```

---

## 8. Questions for Provider

1. Pricing — **monthly** and **annual** rates (annual discount available?)
2. vCPU type and storage type (SSD / NVMe / shared disk?)
3. Snapshot policy — cost, maximum snapshots stored, retention period
4. Static IP — included in plan or charged separately?
5. Support SLA — response time in case of VM downtime?
6. Migration assistance — can you help migrate data from an existing server?
7. Trial period — is a trial available before commitment?
8. Payment methods accepted (credit card / bank transfer / local payment?)
9. Can the VM be upgraded to a higher spec later without significant downtime?

---

*Specifications are based on actual production system data as of June 25, 2025*
