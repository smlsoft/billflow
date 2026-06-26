# Cloud Server Request for Quotation
## Thaisunsport Co., Ltd.

**Date:** June 26, 2025

---

## Overview

Thaisunsport requires **1 Cloud VM** to host BillFlow — a purchase order management system used by 5–20 internal staff members during business hours.

The system runs 24/7 to automatically receive and process purchase orders via Email.

**System Administrator:** BillFlow team (handles all installation and configuration — no technical action required from the customer)

---

## Server Specification Required

| Item | Requirement |
|---|---|
| CPU | **2 vCPU** |
| RAM | **4 GB** |
| Storage | **60 GB SSD** |
| Bandwidth | **500 GB/month or more** (unmetered preferred) |
| Operating System | **Ubuntu Linux 22.04 or 24.04** (64-bit) |
| Datacenter Location | **Thailand** |
| IP Address | **1 Static IP** (fixed, non-changing) |
| Access Level | **Root access** (required for software installation) |
| Number of VMs | **1 VM** |

---

## Software Running on the Server

All software is installed via **Docker** (managed entirely by the BillFlow team — no setup required from the provider).

| Application | Purpose | Memory Usage |
|---|---|---|
| API Server (Go) | Core processing, AI integration, Email polling | ~80 MB |
| Web Frontend (React) | User interface | ~8 MB |
| Database (PostgreSQL) | Bills, customers, history | ~155 MB |
| OS + Docker | Base runtime | ~300 MB |
| **Total** | | **~540 MB of 4,000 MB** |

**Storage Usage:**

| Item | Current | Est. 1 Year |
|---|---|---|
| Application (Docker images) | ~3 GB | ~3 GB |
| Database | ~0.2 GB | ~1 GB |
| PDF files / attachments | ~0.5 GB | ~3 GB |
| Daily backups | ~1.2 GB | ~3 GB |
| OS + logs | ~5 GB | ~8 GB |
| **Total** | **~10 GB** | **~18 GB** |

---

## Questions Required in Quotation

Please include answers to the following in your quotation:

1. **Pricing** — Monthly and annual rate? Any discount for longer commitment?
2. **VAT** — Is pricing inclusive or exclusive of 7% VAT?
3. **Backup** — Is automated backup available? Price? How many days of retention?
4. **Static IP** — Included in the package or charged separately?
5. **SLA** — What uptime is guaranteed? Is there a credit/refund policy if the SLA is breached?
6. **Support** — Hours of availability? Channels (chat / phone / ticket)? Response time?
7. **Migration assistance** — Can you assist with migrating data from an existing server? Is there a cost?
8. **Payment methods** — Credit card / bank transfer / PromptPay accepted?
9. **Scaling** — Can CPU/RAM be upgraded later? How long is the downtime during upgrade?
10. **Trial period** — Is a trial period or refund policy available?

---

## Notes for Sales Team

- **No Managed Service required** — the BillFlow team manages all software independently
- **Clean OS only** (Ubuntu) with root SSH access is sufficient
- For any technical questions, please contact the BillFlow team directly

---

**Technical Contact:**
Nontawat Wongnuk
bos.catdog@gmail.com

*Document prepared by BillFlow Team | June 26, 2025*
