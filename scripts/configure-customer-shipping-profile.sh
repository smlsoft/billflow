#!/usr/bin/env bash
# Fill the approved company address once and select it for unconfigured inboxes.
# Does not approve recipient aliases, enable checks, or reprocess existing bills.
set -euo pipefail
DEPLOY_DIR="${DEPLOY_DIR:-/home/thaisunspot/billflow-thaisunsport}"
[[ -f "$DEPLOY_DIR/docker-compose.yml" ]]
docker compose --project-directory "$DEPLOY_DIR" exec -T postgres sh -ec \
  'exec psql -X -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB"' <<'SQL'
BEGIN;
INSERT INTO app_settings(key,value,is_secret)
VALUES ('email.shipping_checks','{"profiles":[],"policies":[],"revision":""}',false)
ON CONFLICT(key) DO NOTHING;
DO $seed$
DECLARE
  previous jsonb;
  next_config jsonb;
  profiles jsonb;
  policies jsonb;
  inbox record;
  company_profile jsonb := '{
    "id":"thaisunsport-hq-368",
    "name":"ไทยซันสปอร์ต สำนักงานใหญ่",
    "house":"368","moo":"10",
    "district":"คลองมะเดื่อ","amphoe":"กระทุ่มแบน",
    "province":"สมุทรสาคร","postcode":"74110",
    "district_aliases":["Khlong Ma Duea"],
    "amphoe_aliases":["Krathum Baen"],
    "province_aliases":["Samut Sakhon"]
  }'::jsonb;
BEGIN
  SELECT value::jsonb INTO previous FROM app_settings
  WHERE key='email.shipping_checks' FOR UPDATE;
  profiles := COALESCE(previous->'profiles','[]'::jsonb);
  policies := COALESCE(previous->'policies','[]'::jsonb);
  IF jsonb_typeof(profiles)<>'array' OR jsonb_typeof(policies)<>'array' THEN
    RAISE EXCEPTION 'Invalid shipping config; no changes applied';
  END IF;
  IF NOT EXISTS (SELECT 1 FROM jsonb_array_elements(profiles) p
                 WHERE p->>'id'=company_profile->>'id') THEN
    profiles := profiles || jsonb_build_array(company_profile);
  END IF;
  FOR inbox IN SELECT id FROM imap_accounts
               WHERE channel IN ('shopee','lazada') ORDER BY id LOOP
    IF NOT EXISTS (SELECT 1 FROM jsonb_array_elements(policies) p
                   WHERE p->>'account_id'=inbox.id::text) THEN
      policies := policies || jsonb_build_array(jsonb_build_object(
        'account_id',inbox.id::text,'profile_id',company_profile->>'id',
        'recipients','[]'::jsonb,'enabled',false));
    END IF;
  END LOOP;
  next_config := previous || jsonb_build_object('profiles',profiles,'policies',policies);
  IF next_config IS DISTINCT FROM previous THEN
    next_config := next_config || jsonb_build_object('revision',clock_timestamp()::text);
    UPDATE app_settings SET value=next_config::text,updated_at=now(),updated_by=NULL
    WHERE key='email.shipping_checks';
    INSERT INTO audit_logs(action,source,detail)
    VALUES ('shipping_check_config_updated','settings',jsonb_build_object(
      'operation','server_profile_setup','actor','server_operator',
      'before',previous,'after',next_config));
  END IF;
END;
$seed$;
COMMIT;
SELECT a.name AS inbox,a.channel,
       p->>'profile_id' AS shipping_profile,
       p->>'enabled' AS checking_enabled,
       jsonb_array_length(COALESCE(p->'recipients','[]'::jsonb)) AS approved_names
FROM app_settings s
CROSS JOIN LATERAL jsonb_array_elements(s.value::jsonb->'policies') p
JOIN imap_accounts a ON a.id::text=p->>'account_id'
WHERE s.key='email.shipping_checks' ORDER BY a.channel,a.name;
SQL
printf '\nSUCCESS: company profile prepared; existing settings preserved.\n'
printf 'Refresh Email Settings, verify approved recipient names per inbox, preview, enable and save.\n'
printf 'Newly configured inboxes remain OFF. No restart or old-bill processing is required.\n'
