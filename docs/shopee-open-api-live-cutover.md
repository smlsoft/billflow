# Shopee Open API Live Cutover

> Updated: 2026-05-20
> Goal: make BillFlow ready to connect a real Shopee shop immediately after Shopee approves Go-Live.

## Current State

- App ID: `231867`
- Current Shopee status: `Application to go live is under review`
- Current server public URL: `https://animal-galvanize-tameness.ngrok-free.dev`
- BillFlow has Shopee Open API code, OAuth callback, token tables, preview-only import, readiness status, and user-facing error UX ready to deploy.
- Sandbox test account creation returned `Create failed`, so sandbox OAuth is blocked by Shopee console state.
- Until Shopee approves Go-Live, the Open API card intentionally blocks live connection/fetch and keeps Excel import as the fallback path.

## Readiness Gate

`GET /api/settings/shopee-api/status` returns a checklist used by `/import/shopee`:

- Open API enabled on the server.
- Partner ID and Partner Key configured.
- Redirect URL is HTTPS and ends with `/api/shopee-api/callback`.
- Base URL matches environment:
  - `sandbox` -> `https://partner.test-stable.shopeemobile.com`
  - `live` -> `https://partner.shopeemobile.com`
- Live mode is approved before letting the admin connect a real shop.
- OAuth shop connection exists before allowing API fetch.
- Token state is usable, refreshable, or blocked.
- Last sync error is shown as a warning, not hidden in logs.

The UI derives two separate gates:

- `can_connect`: admin can start Shopee OAuth.
- `can_fetch`: admin can fetch order preview from Shopee API.

This keeps the system safe during review: configuration can be checked now, but real API fetch waits for Shopee approval and shop authorization.

## Error UX Contract

Shopee API failures return structured JSON:

```json
{
  "error": "อ่านง่ายสำหรับ admin",
  "error_code": "RATE_LIMIT",
  "retryable": true
}
```

Known mapped cases include token expiry, rate limit, duplicate/in-flight request, timeout, and Shopee business errors. The frontend displays the action the admin should take instead of a raw Go/Shopee error.

## Preconditions Before Live Cutover

1. Shopee Go-Live is approved.
2. Console shows Live Partner ID and Live Partner Key.
3. Shopee Console `Live Redirect URL Domain` matches the current public BillFlow URL.
4. If ngrok/cloudflare quick tunnel changed URL, update both Shopee Console and BillFlow `.env` before connecting.
5. Keep Shopee Excel/email import enabled as rollback path.

## Already Verified With Mock Tests

- Backend readiness blocks misconfigured live base URL.
- Backend allows `refresh_required` token state when refresh token is still valid.
- Shopee API error mapper returns friendly rate-limit messaging.
- Shopee client signs shop API requests correctly.
- Shopee client handles business errors and malformed token response.
- Shopee client rejects order detail requests over Shopee's 50-order limit before making HTTP calls.
- Frontend build and browser smoke test confirm the readiness checklist renders and blocks live actions while review is pending.

## Server Cutover

Run this on `192.168.2.109` after approval:

```bash
cd /home/bosscatdog/billflow
python3 scripts/shopee-live-cutover.py --partner-id LIVE_PARTNER_ID
docker compose up -d backend
```

The script reads the Live Partner Key via hidden prompt, writes a timestamped `.env` backup, and sets:

```dotenv
SHOPEE_OPEN_API_ENABLED=true
SHOPEE_OPEN_API_ENV=live
SHOPEE_OPEN_API_BASE_URL=https://partner.shopeemobile.com
SHOPEE_OPEN_API_PARTNER_ID=<live partner id>
SHOPEE_OPEN_API_PARTNER_KEY=<live partner key>
SHOPEE_OPEN_API_REDIRECT_URL=<PUBLIC_BASE_URL>/api/shopee-api/callback
```

## Validation

1. Backend health:

```bash
curl -sS http://localhost:8090/health
```

Expected:

```json
{"database":"ok","env":"production","status":"ok"}
```

2. In BillFlow `/import/shopee`, Shopee Open API card should show:

- Environment: `live`
- Configured: complete
- Connected: not yet connected

3. Click `เชื่อมต่อ Shopee API`, login/authorize the real shop, then return to BillFlow.

4. Fetch a small date range first, preferably 1 day. Confirm that preview shows expected order count.

5. Create BillFlow bills only after preview looks correct. Do not enable auto-send to SML for the first live run.

## Rollback

If live OAuth or API fetch fails:

```bash
cd /home/bosscatdog/billflow
cp .env.bak.YYYYMMDD-HHMMSS .env
docker compose up -d backend
```

Fast disable without restoring everything:

```dotenv
SHOPEE_OPEN_API_ENABLED=false
```

Then restart backend. Shopee Excel/email flows are independent and should keep working.

## Production Notes

- Do not paste Partner Key into chat, tickets, screenshots, or command history.
- OAuth tokens are stored in `shopee_api_connections`; treat the DB backup as sensitive.
- The first live import is preview-only by design. Bill creation still requires explicit confirmation.
- If public URL changes, old OAuth links and Shopee redirect validation will fail until Console and `.env` match again.
