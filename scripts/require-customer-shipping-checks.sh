#!/usr/bin/env bash
# Explicit customer activation. Other instances retain warning-only defaults.
set -euo pipefail
DEPLOY_DIR="${DEPLOY_DIR:-/home/thaisunspot/billflow-thaisunsport}"
[[ -f "$DEPLOY_DIR/docker-compose.yml" ]]
docker compose --project-directory "$DEPLOY_DIR" exec -T postgres sh -ec \
  'exec psql -X -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB"' <<'SQL'
BEGIN;
DO $activate$
DECLARE
  previous jsonb;
  next_config jsonb;
  invalid_inboxes integer;
BEGIN
  SELECT value::jsonb INTO previous FROM app_settings
  WHERE key='email.shipping_checks' FOR UPDATE;
  IF previous IS NULL OR jsonb_typeof(previous->'profiles') IS DISTINCT FROM 'array'
     OR jsonb_typeof(previous->'policies') IS DISTINCT FROM 'array' THEN
    RAISE EXCEPTION 'Shipping config missing or invalid; configure company inboxes before activation';
  END IF;
  IF NOT EXISTS (SELECT 1 FROM imap_accounts WHERE enabled AND channel IN ('shopee','lazada')) THEN
    RAISE EXCEPTION 'No active marketplace inboxes; no changes applied';
  END IF;
  SELECT count(*) INTO invalid_inboxes FROM imap_accounts a
  WHERE a.enabled AND a.channel IN ('shopee','lazada') AND (
    (SELECT count(*) FROM jsonb_array_elements(previous->'policies') p
      WHERE p->>'account_id'=a.id::text) <> 1
    OR NOT EXISTS (
      SELECT 1 FROM jsonb_array_elements(previous->'policies') p
      JOIN jsonb_array_elements(previous->'profiles') f ON f->>'id'=p->>'profile_id'
      WHERE p->>'account_id'=a.id::text
      AND jsonb_typeof(p->'recipients')='array' AND jsonb_array_length(p->'recipients')>0
      AND NOT EXISTS (SELECT 1 FROM jsonb_array_elements_text(p->'recipients') n WHERE btrim(n)='')
      AND btrim(COALESCE(f->>'house',''))<>'' AND btrim(COALESCE(f->>'moo',''))<>''
      AND btrim(COALESCE(f->>'district',''))<>'' AND btrim(COALESCE(f->>'amphoe',''))<>''
      AND btrim(COALESCE(f->>'province',''))<>'' AND COALESCE(f->>'postcode','') ~ '^[0-9]{5}$'
    )
  );
  IF invalid_inboxes>0 THEN
    RAISE EXCEPTION '% active marketplace inboxes lack complete shipping rules; no changes applied',invalid_inboxes;
  END IF;
  IF previous->>'require_manual_pass' IS DISTINCT FROM 'true' THEN
    next_config := previous || jsonb_build_object('require_manual_pass',true,'revision',clock_timestamp()::text);
    UPDATE app_settings SET value=next_config::text,updated_at=now(),updated_by=NULL
    WHERE key='email.shipping_checks';
    INSERT INTO audit_logs(action,source,detail)
    VALUES ('shipping_check_config_updated','settings',jsonb_build_object(
      'operation','server_require_manual_shipping_pass','actor','server_operator',
      'before',previous,'after',next_config));
  END IF;
END;
$activate$;
COMMIT;
SELECT value::jsonb->>'require_manual_pass' AS require_manual_pass
FROM app_settings WHERE key='email.shipping_checks';
SQL
printf '\nSUCCESS: manual shipping check + latest passing result required before single/bulk SML send.\n'
printf 'Applies to unsent Shopee/Lazada purchase email bills, including old bills. No bills were sent or reprocessed.\n'
