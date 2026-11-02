#!/usr/bin/env bash
# Customer-side backend + frontend deployment. Uses configured Vite build args
# from Compose; does not open or edit secret files. No bill-data repair is run.
set -euo pipefail
umask 077

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
SOURCE_DIR="${SOURCE_DIR:-$(cd -- "$SCRIPT_DIR/.." && pwd)}"
DEPLOY_DIR="${DEPLOY_DIR:-/home/thaisunspot/billflow-thaisunsport}"
BACKEND_PORT="${BACKEND_PORT:-8100}"
FRONTEND_PORT="${FRONTEND_PORT:-3020}"
STAMP="$(date +%Y%m%d-%H%M%S)"
BACKUP_DIR="$DEPLOY_DIR/backups/payment-method-filters-$STAMP"

[[ -f "$DEPLOY_DIR/docker-compose.yml" && -d "$SOURCE_DIR/backend" && -d "$SOURCE_DIR/frontend" ]]
[[ "$(cd "$SOURCE_DIR" && pwd)" != "$(cd "$DEPLOY_DIR" && pwd)" ]]
command -v python3 >/dev/null
command -v rsync >/dev/null
compose() { docker compose --project-directory "$DEPLOY_DIR" "$@"; }
compose config --quiet
mkdir -p "$BACKUP_DIR"

# Resolve only non-secret Vite feature flags from the deployment's Compose
# build contract. Missing legacy flags use this customer's Phase 1 defaults.
# Never print or persist the full resolved Compose configuration.
BUILD_ARGS_TEXT="$(compose config --format json | python3 -c '
import json, sys
config = json.load(sys.stdin)
build = config["services"]["frontend"].get("build", {})
args = build.get("args", {}) if isinstance(build, dict) else {}
defaults = {
    "VITE_PHASE": "1",
    "VITE_ENABLE_SALES_ORDERS": "false",
    "VITE_ENABLE_SHOPEE_EXCEL": "false",
    "VITE_ENABLE_LAZADA_EXCEL": "false",
    "VITE_ENABLE_TIKTOK_EXCEL": "false",
    "VITE_ENABLE_CHAT": "false",
    "VITE_ENABLE_REMARK2": "true",
}
for key, fallback in defaults.items():
    value = str(args.get(key) or fallback)
    if key == "VITE_PHASE":
        if not value.isdigit(): raise SystemExit("Invalid VITE_PHASE build argument")
    elif value not in ("true", "false"):
        raise SystemExit("Invalid feature build argument: " + key)
    print("--build-arg")
    print(key + "=" + value)
')"
mapfile -t FRONTEND_BUILD_ARGS <<< "$BUILD_ARGS_TEXT"

for service in backend frontend; do
  container="$(compose ps -q "$service")"
  [[ -n "$container" ]]
  image="$(docker inspect --format '{{.Image}}' "$container")"
  tag="$(docker inspect --format '{{.Config.Image}}' "$container")"
  rollback="billflow-payments-$service-rollback:$STAMP"
  docker image tag "$image" "$rollback"
  printf '%s image: %s; rollback: %s\n' "$service" "$tag" "$rollback"
  rsync -a --exclude='.env*' --exclude='*.key' --exclude='*.pem' \
    --exclude='*.db' --exclude='*.sqlite' --exclude=node_modules --exclude=dist \
    "$DEPLOY_DIR/$service/" "$BACKUP_DIR/$service/"
  if [[ "$service" == backend ]]; then BACKEND_IMAGE="$tag"; else FRONTEND_IMAGE="$tag"; fi
done
compose exec -T postgres sh -ec 'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB"' | gzip -c > "$BACKUP_DIR/database.sql.gz"
gzip -t "$BACKUP_DIR/database.sql.gz"
[[ -s "$BACKUP_DIR/database.sql.gz" ]]
printf 'Backup: %s\n' "$BACKUP_DIR"

for service in backend frontend; do
  rsync -a --checksum --exclude='.env*' --exclude='*.key' --exclude='*.pem' \
    --exclude='*.db' --exclude='*.sqlite' --exclude=node_modules --exclude=dist \
    --exclude='.DS_Store' --exclude=tmp --exclude='*.test' \
    "$SOURCE_DIR/$service/" "$DEPLOY_DIR/$service/"
done
docker build --network host --pull=false -t "$BACKEND_IMAGE" -f "$DEPLOY_DIR/backend/Dockerfile" "$DEPLOY_DIR/backend"
docker build --network host --pull=false -t "$FRONTEND_IMAGE" \
  "${FRONTEND_BUILD_ARGS[@]}" -f "$DEPLOY_DIR/frontend/Dockerfile" "$DEPLOY_DIR/frontend"
compose up -d --no-deps --force-recreate backend frontend

for attempt in $(seq 1 120); do
  if curl -fsS --max-time 5 "http://127.0.0.1:$BACKEND_PORT/health" | grep -Fq '"status":"ok"' \
    && curl -fsS --max-time 5 "http://127.0.0.1:$FRONTEND_PORT/health" >/dev/null; then
    printf '\nSUCCESS: backend and frontend healthy. Backup: %s\n' "$BACKUP_DIR"
    printf 'Refresh BillFlow to load the new payment filters.\n'
    compose ps backend frontend
    exit 0
  fi
  sleep 2
done
compose logs --tail=100 backend frontend >&2
printf 'Health check failed; backup and previous images were preserved: %s\n' "$BACKUP_DIR" >&2
exit 1
