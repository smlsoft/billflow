# Google Drive Email Export

BillFlow can copy the original marketplace email to a customer-owned Google
Drive only after the associated purchase order has been accepted by SML.
It supports `shopee_shipped` and `lazada_email` purchase bills. The feature
uses the existing server-owned rclone remote, never a Google token entered in
the BillFlow web UI.

## Before Enabling

1. Configure the customer's Google Drive remote on the server with rclone.
   Use a customer-specific Google OAuth client ID rather than rclone's shared
   client ID; the shared Google Drive client is being retired during 2026.
2. Confirm the remote can create and delete a test folder as the account that
   starts Docker Compose.
3. In the BillFlow deploy `.env`, set:

```dotenv
GOOGLE_DRIVE_RCLONE_REMOTE=thaisunsport_gdrive
RCLONE_CONFIG_HOST_PATH=/root/.config/rclone/rclone.conf
```

`RCLONE_CONFIG_HOST_PATH` is a host path. Docker mounts it read-only at
`/run/secrets/rclone.conf` in the backend. Keep the actual `rclone.conf`
outside Git and never put its refresh token in `.env` or the UI.

If the rclone configuration is encrypted, set `RCLONE_CONFIG_PASS` only in the
deployment `.env`; do not add it to the tracked `.env.example`.

## Operator Flow

Open **ตั้งค่าระบบ → Google Drive อีเมล** as an administrator.

1. Enter the root destination folder, for example `BillFlow Email/Thaisunsport`.
2. Click the connection test. It creates and removes a uniquely named test
   folder under the selected root.
3. Enable automatic upload and save.
4. Use the historical date range preview before adding older SML-sent bills to
   the upload queue. Each request is capped at 31 days and 500 bills.

The worker runs once per minute with two concurrent uploads. It retries after
1, 5, 15, and 60 minutes, then every 6 hours, with at most 8 attempts. A
backend restart returns interrupted work to the queue. It also reconciles sent
bills from the preceding 24 hours every 10 minutes.

## Output Naming

Each purchase order gets a separate copy. Multi-order emails therefore appear
once per SML PO even though they share the original email artifact.

```text
<root>/<YYYY>/<MM>/<DD>/<Shopee|Lazada>/<payment>/
YYYYMMDD_<channel>_<payment>_<SML-PO>_<marketplace-order>_<charge>.<html|txt>
```

`payment` uses `TT...` when staff recorded one. Other methods use `TRANSFER`,
`COD`, `CARD`, or `OTHER`; the charge field is `NA` only when the source email
does not contain a usable amount.

The worker uploads a unique temporary object, verifies its size and MD5 on
Google Drive, then renames it to the final filename. It never overwrites a
different file with the same final name; such a case stays `ต้องตรวจ` for an
administrator to resolve.
