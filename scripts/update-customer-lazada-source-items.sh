#!/usr/bin/env bash
# Run on the customer server from a checked-out release. Preflight the exact
# bill's source HTML using the new image before replacing the running backend.
set -euo pipefail
umask 077

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
SOURCE_DIR="${SOURCE_DIR:-$(cd -- "$SCRIPT_DIR/.." && pwd)}"
DEPLOY_DIR="${DEPLOY_DIR:-/home/thaisunspot/billflow-thaisunsport}"
BILL_ID="${BILL_ID:-e25e856f-8cee-4765-99cb-5b597c1dc4d3}"
ORDER_ID="${ORDER_ID:-1124675871588966}"
BACKEND_PORT="${BACKEND_PORT:-8100}"
STAMP="$(date +%Y%m%d-%H%M%S)"
BACKUP_DIR="$DEPLOY_DIR/backups/lazada-source-items-$STAMP"

[[ -d "$SOURCE_DIR/backend" && -f "$DEPLOY_DIR/docker-compose.yml" ]]
[[ "$(cd "$SOURCE_DIR" && pwd)" != "$(cd "$DEPLOY_DIR" && pwd)" ]]
compose() { docker compose --project-directory "$DEPLOY_DIR" "$@"; }
compose config --quiet
CONTAINER_ID="$(compose ps -q backend)"
[[ -n "$CONTAINER_ID" ]]
PREVIOUS_IMAGE="$(docker inspect --format '{{.Image}}' "$CONTAINER_ID")"
BACKEND_IMAGE="$(docker inspect --format '{{.Config.Image}}' "$CONTAINER_ID")"
ROLLBACK_TAG="billflow-lazada-rollback:$STAMP"
docker image tag "$PREVIOUS_IMAGE" "$ROLLBACK_TAG"
mkdir -p "$BACKUP_DIR"
cp -a "$DEPLOY_DIR/backend" "$BACKUP_DIR/backend"
compose exec -T postgres sh -ec 'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB"' | gzip -c > "$BACKUP_DIR/database.sql.gz"
gzip -t "$BACKUP_DIR/database.sql.gz"
[[ -s "$BACKUP_DIR/database.sql.gz" ]]
printf 'Backup: %s\nRollback image: %s\n' "$BACKUP_DIR" "$ROLLBACK_TAG"

rsync -a --checksum --exclude='.DS_Store' --exclude=tmp --exclude='*.test' "$SOURCE_DIR/backend/" "$DEPLOY_DIR/backend/"
docker build --network host --pull=false -t "$BACKEND_IMAGE" -f "$DEPLOY_DIR/backend/Dockerfile" "$DEPLOY_DIR/backend"

# Read-only SQL and artifact reads only. On unsupported HTML or inconsistent
# source totals this exits before the running backend is recreated.
compose run --rm --no-deps -T backend /app/lazada_source_items \
  --bill-id "$BILL_ID" --order-id "$ORDER_ID" --dry-run | tee "$BACKUP_DIR/dry-run.txt"
compose up -d --no-deps --force-recreate backend
for attempt in $(seq 1 90); do
  if curl -fsS --max-time 5 "http://127.0.0.1:$BACKEND_PORT/health" | grep -Fq '"status":"ok"'; then
    printf '\nSUCCESS: backend healthy; historical bill has not been changed.\n'
    printf 'Review plan: %s/dry-run.txt\n' "$BACKUP_DIR"
    exit 0
  fi
  sleep 2
done
compose logs --tail=100 backend >&2
printf 'Health failed. Backup: %s; previous image: %s\n' "$BACKUP_DIR" "$ROLLBACK_TAG" >&2
exit 1
