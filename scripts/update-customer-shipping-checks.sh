#!/usr/bin/env bash
# Customer-side warning-only shipping checks release; no historical reprocessing.
# Run from a release checkout pinned with EXPECTED_COMMIT.
set -euo pipefail
umask 077

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
SOURCE_DIR="${SOURCE_DIR:-$(cd -- "$SCRIPT_DIR/.." && pwd)}"
DEPLOY_DIR="${DEPLOY_DIR:-/home/thaisunspot/billflow-thaisunsport}"
BACKEND_PORT="${BACKEND_PORT:-8100}"
FRONTEND_PORT="${FRONTEND_PORT:-3020}"
STAMP="$(date +%Y%m%d-%H%M%S)"
BACKUP_DIR="$DEPLOY_DIR/backups/shipping-checks-$STAMP"

[[ -f "$DEPLOY_DIR/docker-compose.yml" && -d "$SOURCE_DIR/backend" && -d "$SOURCE_DIR/frontend" ]]
[[ "$(cd "$SOURCE_DIR" && pwd)" != "$(cd "$DEPLOY_DIR" && pwd)" ]]
[[ -n "${EXPECTED_COMMIT:-}" ]] || { echo 'EXPECTED_COMMIT must be the reviewed full release commit' >&2; exit 1; }
[[ "$(git -C "$SOURCE_DIR" rev-parse HEAD)" == "$EXPECTED_COMMIT" ]]
[[ -z "$(git -C "$SOURCE_DIR" status --porcelain --untracked-files=no)" ]]
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
  rollback="billflow-shipping-$service-rollback:$STAMP"
  docker image tag "$image" "$rollback"
  printf '%s image: %s; rollback: %s\n' "$service" "$tag" "$rollback"
  rsync -a --exclude='.env*' --exclude='*.key' --exclude='*.pem' \
    --exclude='*.db' --exclude='*.sqlite' --exclude=node_modules --exclude=dist \
    "$DEPLOY_DIR/$service/" "$BACKUP_DIR/$service/"
  if [[ "$service" == backend ]]; then BACKEND_IMAGE="$tag"; BACKEND_ROLLBACK="$rollback"; else FRONTEND_IMAGE="$tag"; FRONTEND_ROLLBACK="$rollback"; fi
done
compose exec -T postgres sh -ec 'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB"' | gzip -c > "$BACKUP_DIR/database.sql.gz"
gzip -t "$BACKUP_DIR/database.sql.gz"
[[ -s "$BACKUP_DIR/database.sql.gz" ]]
printf 'Backup: %s\n' "$BACKUP_DIR"
# Standalone rollback restores the previous images and source, never the live DB.
# Additive migration 074 stays in place so post-deploy customer data is retained.
{
  printf '#!/usr/bin/env bash\nset -euo pipefail\n'
  printf 'DEPLOY_DIR=%q\nBACKUP_DIR=%q\n' "$DEPLOY_DIR" "$BACKUP_DIR"
  printf 'BACKEND_IMAGE=%q\nFRONTEND_IMAGE=%q\n' "$BACKEND_IMAGE" "$FRONTEND_IMAGE"
  printf 'BACKEND_ROLLBACK=%q\nFRONTEND_ROLLBACK=%q\n' "$BACKEND_ROLLBACK" "$FRONTEND_ROLLBACK"
  printf 'BACKEND_PORT=%q\nFRONTEND_PORT=%q\n' "$BACKEND_PORT" "$FRONTEND_PORT"
  cat <<'ROLLBACK'
[[ -f "$DEPLOY_DIR/docker-compose.yml" && -d "$BACKUP_DIR/backend" && -d "$BACKUP_DIR/frontend" ]]
docker image inspect "$BACKEND_ROLLBACK" "$FRONTEND_ROLLBACK" >/dev/null
for service in backend frontend; do
  rsync -a --delete --exclude='.env*' --exclude='*.key' --exclude='*.pem' \
    --exclude='*.db' --exclude='*.sqlite' --exclude=node_modules --exclude=dist \
    "$BACKUP_DIR/$service/" "$DEPLOY_DIR/$service/"
done
docker image tag "$BACKEND_ROLLBACK" "$BACKEND_IMAGE"
docker image tag "$FRONTEND_ROLLBACK" "$FRONTEND_IMAGE"
docker compose --project-directory "$DEPLOY_DIR" up -d --no-deps --force-recreate backend frontend
for attempt in $(seq 1 120); do
  if curl -fsS --max-time 5 "http://127.0.0.1:$BACKEND_PORT/health" | grep -Fq '"status":"ok"' \
    && curl -fsS --max-time 5 "http://127.0.0.1:$FRONTEND_PORT/health" >/dev/null; then
    echo 'ROLLBACK SUCCESS: previous images running; live database retained.'
    exit 0
  fi
  sleep 2
done
echo 'Rollback health check failed; inspect docker compose logs.' >&2
exit 1
ROLLBACK
} > "$BACKUP_DIR/rollback.sh"
chmod 700 "$BACKUP_DIR/rollback.sh"
printf 'Rollback command: bash %q\n' "$BACKUP_DIR/rollback.sh"
printf '%s\n' "$EXPECTED_COMMIT" > "$BACKUP_DIR/release-commit.txt"


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
    printf 'Refresh BillFlow. Shipping checks are OFF by default. Configure and preview under Email Settings before enabling.\n'
    compose ps backend frontend
    exit 0
  fi
  sleep 2
done
compose logs --tail=100 backend frontend >&2
printf 'Health check failed. Roll back with: bash %q\n' "$BACKUP_DIR/rollback.sh" >&2
exit 1
