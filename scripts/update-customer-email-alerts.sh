#!/usr/bin/env bash
# Reuse the reviewed customer backup/build/health/rollback flow. SMTP remains
# disabled on first install; no recipient/password is seeded from IMAP accounts.
set -euo pipefail
SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
SOURCE_DIR="${SOURCE_DIR:-$(cd -- "$SCRIPT_DIR/.." && pwd)}"
DEPLOY_DIR="${DEPLOY_DIR:-/home/thaisunspot/billflow-thaisunsport}"
SOURCE_DIR="$SOURCE_DIR" DEPLOY_DIR="$DEPLOY_DIR" \
  VITE_EMAIL_ALERT_DEFAULT_URL="${VITE_EMAIL_ALERT_DEFAULT_URL:-http://thaisunsport.thddns.net:9985}" \
  bash "$SOURCE_DIR/scripts/update-customer-shipping-checks.sh"
if ! docker compose --project-directory "$DEPLOY_DIR" exec -T postgres sh -ec \
  'exec psql -X -At -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "SELECT to_regclass('\''public.email_alert_outbox'\'') IS NOT NULL AND to_regclass('\''public.email_alert_attempts'\'') IS NOT NULL"' | grep -qx t; then
  echo 'Email alert schema check failed. Use the rollback command printed above.' >&2
  exit 1
fi
printf '\nEMAIL ALERTS READY: new installations are OFF by default; existing configuration is retained.\n'
printf 'Email Settings -> Email alerts: enter Gmail + App Password, SAVE, then queue a TEST email.\n'
printf 'Only enable automation after checking history and the recipient inbox. Never paste the password into chat.\n'
