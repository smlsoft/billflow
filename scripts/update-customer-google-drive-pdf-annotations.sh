#!/usr/bin/env bash
# Update only the backend on a customer BillFlow server so newly exported
# Google Drive PDFs include POL labels and TT (card-cut) stamps.
#
# Run this ON the customer server after copying a checked-out/release copy of
# this repository there. It creates a PostgreSQL backup before changing code;
# it does not requeue or modify existing Google Drive files.
#
# Example for Thaisunsport production:
#   SOURCE_DIR=/home/thaisunspot/billflow-release \
#   DEPLOY_DIR=/home/thaisunspot/billflow-thaisunsport \
#   BACKEND_PORT=8100 \
#   bash /home/thaisunspot/billflow-release/scripts/update-customer-google-drive-pdf-annotations.sh

set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
SOURCE_DIR="${SOURCE_DIR:-$(cd -- "${SCRIPT_DIR}/.." && pwd)}"
DEPLOY_DIR="${DEPLOY_DIR:-/home/thaisunspot/billflow-thaisunsport}"
BACKEND_PORT="${BACKEND_PORT:-8100}"
BACKUP_DIR="${DEPLOY_DIR}/backups/manual-backups"
STAMP="$(date +%Y%m%d-%H%M%S)"
BACKUP_FILE="${BACKUP_DIR}/before-google-drive-pdf-annotations-${STAMP}.sql.gz"

fail() {
  printf 'ERROR: %s\n' "$*" >&2
  exit 1
}

[[ -d "${SOURCE_DIR}/backend" ]] || fail "SOURCE_DIR must contain backend/: ${SOURCE_DIR}"
[[ -f "${DEPLOY_DIR}/docker-compose.yml" ]] || fail "DEPLOY_DIR is not a BillFlow deployment: ${DEPLOY_DIR}"
command -v docker >/dev/null || fail "docker is not installed"
command -v rsync >/dev/null || fail "rsync is not installed"
command -v gzip >/dev/null || fail "gzip is not installed"
command -v curl >/dev/null || fail "curl is not installed"

compose() {
  docker compose --project-directory "${DEPLOY_DIR}" "$@"
}

printf 'BillFlow Google Drive PDF annotation update\n'
printf 'Source : %s\nDeploy : %s\nBackend: http://127.0.0.1:%s/health\n\n' "${SOURCE_DIR}" "${DEPLOY_DIR}" "${BACKEND_PORT}"

compose config --quiet
mkdir -p "${BACKUP_DIR}"

printf '[1/5] Backing up PostgreSQL to %s\n' "${BACKUP_FILE}"
compose exec -T postgres sh -ec 'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB"' | gzip -c > "${BACKUP_FILE}"
[[ -s "${BACKUP_FILE}" ]] || fail "database backup was empty"

printf '[2/5] Syncing backend source (no delete)\n'
rsync -a --checksum \
  --exclude='.DS_Store' \
  --exclude='tmp' \
  --exclude='*.test' \
  "${SOURCE_DIR}/backend/" "${DEPLOY_DIR}/backend/"

printf '[3/5] Building backend image\n'
compose build backend

printf '[4/5] Recreating backend only\n'
compose up -d --no-deps --force-recreate backend

printf '[5/5] Checking backend health\n'
for attempt in 1 2 3 4 5 6 7 8 9 10 11 12; do
  if curl -fsS --max-time 5 "http://127.0.0.1:${BACKEND_PORT}/health" | grep -Fq '"status":"ok"'; then
    printf 'SUCCESS: backend is healthy. Backup: %s\n' "${BACKUP_FILE}"
    printf 'New Google Drive PDFs will include POL and TT card stamps; existing Drive files were not changed.\n'
    exit 0
  fi
  sleep 2
done

printf 'Backend did not become healthy. Recent logs follow:\n' >&2
compose logs --tail=120 backend >&2 || true
fail "deployment stopped after backup; inspect the logs before retrying"
