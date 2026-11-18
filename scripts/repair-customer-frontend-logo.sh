#!/usr/bin/env bash
# Live permission repair for the existing public logo; no rebuild or restart.
# The Dockerfile also normalizes public file permissions in future image builds.
set -euo pipefail
DEPLOY_DIR="${DEPLOY_DIR:-/home/thaisunspot/billflow-thaisunsport}"
FRONTEND_PORT="${FRONTEND_PORT:-3020}"
[[ -f "$DEPLOY_DIR/docker-compose.yml" ]]
CONTAINER_ID="$(docker compose --project-directory "$DEPLOY_DIR" ps -q frontend)"
[[ -n "$CONTAINER_ID" ]]
docker exec -u 0 "$CONTAINER_ID" sh -ec '
  logo=/usr/share/nginx/html/billflow-mark.svg
  test -f "$logo"
  test ! -L "$logo"
  echo "Before:"
  stat -c "%a %U:%G %n" "$logo"
  chmod 644 "$logo"
  echo "After:"
  stat -c "%a %U:%G %n" "$logo"
'
RESULT="$(curl -fsS --max-time 10 -o /dev/null -w '%{http_code} %{content_type}' "http://127.0.0.1:$FRONTEND_PORT/billflow-mark.svg")"
printf 'Logo endpoint: %s\n' "$RESULT"
[[ "$RESULT" == '200 image/svg+xml'* ]] || {
  echo 'Logo did not return SVG content; inspect frontend/proxy logs.' >&2
  exit 1
}
printf 'SUCCESS: public logo is readable. Hard-refresh BillFlow.\n'
printf 'This repairs the running container. Future rebuilds must use the updated release Dockerfile.\n'
