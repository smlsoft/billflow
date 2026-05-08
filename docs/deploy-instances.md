# BillFlow Deploy Instances

Registry สำหรับจำว่าแต่ละร้านใช้ folder, port, container และ Cloudflare tunnel ไหนบน server `192.168.2.109`.

> หมายเหตุ: ตอนนี้ใช้ Cloudflare Quick Tunnel (`trycloudflare.com`) URL จะเปลี่ยนเมื่อ process `cloudflared` ถูก restart หรือเครื่องดับ ให้ดู URL ใหม่จาก log path ของ instance นั้น

## Summary

| Instance | ร้าน / วัตถุประสงค์ | Server folder | Frontend | Backend | PostgreSQL | Cloudflare URL ล่าสุด | Tunnel log |
| --- | --- | --- | ---: | ---: | ---: | --- | --- |
| `billflow` | BillFlow ปกติ / demo หลัก | `/home/bosscatdog/billflow` | `3010` | `8090` | `5438` | ดูจาก log | `/tmp/billflow-tunnel.log` |
| `billflow-thaisunsport` | Thaisunsport demo | `/home/bosscatdog/billflow-thaisunsport` | `3020` | `8100` | `5448` | `https://pets-mini-museums-ships.trycloudflare.com` | `/tmp/billflow-thaisunsport-tunnel.log` |
| `billflow-henna` | Henna customer trial | `/home/bosscatdog/billflow-henna` | `3030` | `8110` | `5458` | `https://aurora-enjoyed-backup-lines.trycloudflare.com` | `/tmp/billflow-henna-tunnel.log` |

## Container Names

| Instance | Frontend container | Backend container | PostgreSQL container |
| --- | --- | --- | --- |
| `billflow` | `billflow-frontend` | `billflow-backend` | `billflow-postgres` |
| `billflow-thaisunsport` | `billflow-thaisunsport-frontend` | `billflow-thaisunsport-backend` | `billflow-thaisunsport-postgres` |
| `billflow-henna` | `billflow-henna-frontend` | `billflow-henna-backend` | `billflow-henna-postgres` |

## Quick Commands

### Check health

```bash
curl http://192.168.2.109:8090/health   # billflow
curl http://192.168.2.109:8100/health   # thaisunsport
curl http://192.168.2.109:8110/health   # henna
```

### Check running containers

```bash
docker ps --format '{{.Names}} {{.Ports}}' | grep billflow
```

### Get current Quick Tunnel URL

```bash
grep -oE 'https://[a-z0-9-]+\.trycloudflare\.com' /tmp/billflow-tunnel.log | tail -1
grep -oE 'https://[a-z0-9-]+\.trycloudflare\.com' /tmp/billflow-thaisunsport-tunnel.log | tail -1
grep -oE 'https://[a-z0-9-]+\.trycloudflare\.com' /tmp/billflow-henna-tunnel.log | tail -1
```

### Restart a Quick Tunnel

```bash
nohup cloudflared tunnel --url http://127.0.0.1:3010 --no-autoupdate > /tmp/billflow-tunnel.log 2>&1 &
nohup cloudflared tunnel --url http://127.0.0.1:3020 --no-autoupdate > /tmp/billflow-thaisunsport-tunnel.log 2>&1 &
nohup cloudflared tunnel --url http://127.0.0.1:3030 --no-autoupdate > /tmp/billflow-henna-tunnel.log 2>&1 &
```

## Port Allocation Rule

ใช้ pattern นี้สำหรับร้านถัดไป:

| Instance order | Frontend | Backend | PostgreSQL |
| ---: | ---: | ---: | ---: |
| Main | `3010` | `8090` | `5438` |
| Customer 1 | `3020` | `8100` | `5448` |
| Customer 2 | `3030` | `8110` | `5458` |
| Customer 3 | `3040` | `8120` | `5468` |

ให้ตั้งชื่อ folder/container/volume ตาม slug ร้าน เช่น `billflow-henna-*` เพื่อไม่ชน instance อื่น.

## Henna Notes

- Created from current normal BillFlow version, not Thaisunsport branch/config.
- Deployed as isolated Docker Compose project in `/home/bosscatdog/billflow-henna`.
- Database is separate PostgreSQL volume `billflow-henna_billflow_henna_pgdata`.
- `PUBLIC_BASE_URL` in `/home/bosscatdog/billflow-henna/.env` is set to the latest Henna Quick Tunnel URL.
- App settings seeded:
  - `instance.name = BillFlow Henna`
  - `instance.slug = billflowhenna`
