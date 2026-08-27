# Google Drive Email Export

BillFlow can save an immutable PDF snapshot of a marketplace email to a
customer-owned Google Drive only after the associated purchase order has been
accepted by SML. The PDF uses the same prepared HTML as the **ดูอีเมล** dialog
in BillFlow, including marketplace layout, Thai text, source product images,
and the payment-total highlight. It supports `shopee_shipped` and
`lazada_email` purchase bills. The feature uses the existing server-owned
rclone remote, never a Google token entered in the BillFlow web UI.

## Before Enabling

1. Configure the customer's Google Drive remote on the server with rclone.
   Use a customer-specific Google OAuth client ID rather than rclone's shared
   client ID; the shared Google Drive client is being retired during 2026.
2. Confirm the remote can create and delete a test folder as the account that
   starts Docker Compose.
3. In the BillFlow deploy `.env`, set:

```dotenv
GOOGLE_DRIVE_RCLONE_REMOTE=thaisunsport_gdrive
RCLONE_CONFIG_HOST_DIR=/root/.config/rclone
RCLONE_CONFIG=/run/secrets/rclone/rclone.conf
GOOGLE_DRIVE_EMAIL_EXPORT_FORMAT=pdf
EMAIL_PDF_RENDERER_URL=http://email-renderer:8080
EMAIL_PDF_RENDERER_TOKEN=<random-server-secret>
EMAIL_PDF_ALLOWED_IMAGE_HOST_SUFFIXES=shopee.co.th,shopee.sg,susercontent.com,lazada.co.th,alicdn.com,slatic.net,lazcdn.com
EMAIL_PDF_SILENT_BLOCKED_IMAGE_HOST_SUFFIXES=mmstat.com
```

Generate the renderer token on the server, then keep it only in deployment
`.env`:

```bash
openssl rand -hex 32
```

`RCLONE_CONFIG_HOST_DIR` is a root-owned directory on the host. Docker mounts
it at `/run/secrets/rclone` so rclone can atomically write refreshed Google
OAuth tokens to `rclone.conf`. Keep the actual config outside Git and never
put its refresh token in `.env` or the UI. On the host, use directory mode
`700` and file mode `600`.

If the rclone configuration is encrypted, set `RCLONE_CONFIG_PASS` only in the
deployment `.env`; do not add it to the tracked `.env.example`. The PDF
renderer is a private Docker service: it has no published port and does not
mount Google credentials or the artifact directory.

## Operator Flow

Open **ตั้งค่าระบบ → Google Drive อีเมล** as an administrator.

1. Enter the root destination folder, for example `BillFlow Email/Thaisunsport`.
2. Click the connection test. It creates a small PDF through the renderer,
   then creates and removes a uniquely named test folder under the selected
   root.
3. Enable automatic upload and save.
4. Use the historical date range preview before adding older SML-sent bills to
   the upload queue. Each request is capped at 31 days and 500 bills.

The worker runs once per minute with one job at a time. Both the worker and the
private renderer serialize PDF work, so overlapping cron ticks, manual
connection tests, and Chromium cannot compete for the renderer PID budget on
customer hardware. Docker runs the renderer beneath an init process, and a
renderer cleanup failure deliberately restarts only that container instead of
leaving Chrome children behind. It retries after 1, 5, 15, and 60 minutes,
then every 6 hours, with at most 8 attempts. A backend restart returns
interrupted work to the queue. It also reconciles sent bills from the preceding
24 hours every 10 minutes.

## Output Naming

Each purchase order gets a separate copy. Multi-order emails therefore appear
once per SML PO even though they share the original email artifact.

```text
<root>/<YYYY>/<MM>/<DD>/<Shopee|Lazada>/<payment>/
YYYYMMDD_<channel>_<payment>_<SML-PO>_<marketplace-order>_<charge>.pdf
```

A legacy `.html` record that was already uploaded remains untouched; an
administrator can click **สร้าง PDF** in the export history to create a
sibling PDF without deleting the original file.

`payment` uses `TT...` when staff recorded one. Other methods use `TRANSFER`,
`COD`, `CARD`, or `OTHER`; the charge field is `NA` only when the source email
does not contain a usable amount.

The worker uploads a unique temporary object, verifies its size and MD5 on
Google Drive, then renames it to the final filename. It never overwrites a
different file with the same final name; such a case stays `ต้องตรวจ` for an
administrator to resolve.

Chromium includes generation metadata in each PDF, so retrying a render would
otherwise produce different bytes for the same email. BillFlow keeps the first
render temporarily in the mounted artifact volume and reuses it during retry.
The cache is removed after success and also after failures before any final
Drive filename could have been created.

## Image Safety And Visual Checks

The renderer disables email JavaScript and blocks every network request except
embedded `data:image` content and HTTPS image requests to configured
marketplace/CDN suffixes. Its default list is `shopee.co.th`, `shopee.sg`,
`susercontent.com`, `lazada.co.th`, `alicdn.com`, `slatic.net`, and
`lazcdn.com`. Add a suffix through
`EMAIL_PDF_ALLOWED_IMAGE_HOST_SUFFIXES` only after verifying it appears in a
real marketplace email; do not add a wildcard or a private/internal domain.

Each source image loaded during rendering is embedded in the resulting PDF, so
the Drive copy remains readable even when a marketplace image URL later
expires. If an image is blocked or cannot be loaded, the PDF is still uploaded
but the export history shows a **ตรวจรูป** warning with the host name. That
warning is intentionally visible: it prevents a partial visual snapshot from
looking silently complete. Known transparent tracking pixels stay blocked but
are omitted from that warning through
`EMAIL_PDF_SILENT_BLOCKED_IMAGE_HOST_SUFFIXES`; its default is `mmstat.com`
for Lazada's hidden email-open tracking endpoint. This setting never permits
the domain to load.
