#!/usr/bin/env bash
# Enable shipping checks for active marketplace inboxes using the company
# profile and recipient names explicitly approved by the customer on 2026-11-18.
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
  current_policy jsonb;
  patched_policy jsonb;
  approved_recipients jsonb;
  approved_names jsonb := '[
    "บริษัท ไทยซันสปอร์ต จำกัด",
    "บริษัท ไทยซันสปอร์ต จำกัด (สำนักงานใหญ่)",
    "บริษัท ไทยซันสปอร์ต จำกัด (สนญ)",
    "บริษัท ไทยซันสปอร์ต จำกัด (สำนักงานใหญ่) 0105559145296",
    "บริษัท ไทยซันสปอร์ต จำกัด (สนญ) 0105559145296",
    "บริษัท ไทยซันสปอร์ต จำกัด 05",
    "PD01"
  ]'::jsonb;
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
                 WHERE p->>'id'=company_profile->>'id'
                 AND p @> jsonb_build_object('house','368','moo','10',
                   'district','คลองมะเดื่อ','amphoe','กระทุ่มแบน',
                   'province','สมุทรสาคร','postcode','74110')) THEN
    RAISE EXCEPTION 'Approved company profile missing or changed; no changes applied';
  END IF;
  FOR inbox IN SELECT id FROM imap_accounts
               WHERE enabled=true AND channel IN ('shopee','lazada') ORDER BY id LOOP
    IF (SELECT count(*) FROM jsonb_array_elements(policies) p
        WHERE p->>'account_id'=inbox.id::text)>1 THEN
      RAISE EXCEPTION 'Duplicate mailbox policy; no changes applied';
    END IF;
    SELECT p INTO current_policy FROM jsonb_array_elements(policies) p
    WHERE p->>'account_id'=inbox.id::text;
    current_policy := COALESCE(current_policy,'{}'::jsonb);
    SELECT jsonb_agg(name ORDER BY name) INTO approved_recipients
    FROM (SELECT DISTINCT jsonb_array_elements_text(
      COALESCE(current_policy->'recipients','[]'::jsonb) || approved_names) AS name) n;
    IF jsonb_array_length(approved_recipients)>50 THEN
      RAISE EXCEPTION 'Too many recipient names; no changes applied';
    END IF;
    patched_policy := current_policy || jsonb_build_object(
      'account_id',inbox.id::text,'profile_id',company_profile->>'id',
      'recipients',approved_recipients,'enabled',true,
      'enabled_at',CASE WHEN current_policy->>'enabled'='true'
                       THEN COALESCE(NULLIF(current_policy->>'enabled_at',''),clock_timestamp()::text)
                       ELSE clock_timestamp()::text END);
    SELECT COALESCE(jsonb_agg(p),'[]'::jsonb) INTO policies
    FROM jsonb_array_elements(policies) p
    WHERE p->>'account_id' IS DISTINCT FROM inbox.id::text;
    policies := policies || jsonb_build_array(patched_policy);
  END LOOP;
  next_config := previous || jsonb_build_object('profiles',profiles,'policies',policies);
  IF next_config IS DISTINCT FROM previous THEN
    next_config := next_config || jsonb_build_object('revision',clock_timestamp()::text);
    UPDATE app_settings SET value=next_config::text,updated_at=now(),updated_by=NULL
    WHERE key='email.shipping_checks';
    INSERT INTO audit_logs(action,source,detail)
    VALUES ('shipping_check_config_updated','settings',jsonb_build_object(
      'operation','server_enable_company_mailboxes','actor','server_operator',
      'before',previous,'after',next_config));
  END IF;
END;
$seed$;
COMMIT;
SELECT a.name AS inbox,a.channel,
       p->>'profile_id' AS shipping_profile,
       a.enabled AS inbox_active,
       p->>'enabled' AS checking_enabled,
       jsonb_array_length(COALESCE(p->'recipients','[]'::jsonb)) AS approved_names
FROM app_settings s
CROSS JOIN LATERAL jsonb_array_elements(s.value::jsonb->'policies') p
JOIN imap_accounts a ON a.id::text=p->>'account_id'
WHERE s.key='email.shipping_checks' ORDER BY a.channel,a.name;
SQL
printf '\nSUCCESS: shipping checks enabled for active Shopee/Lazada inboxes.\n'
printf 'Inactive IMAP accounts were not enabled. No restart or old-bill processing was performed.\n'
printf 'Refresh BillFlow and test NEW orders/emails. Existing bills retain their frozen rules.\n'
