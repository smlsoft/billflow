-- Additive email alert ledger/outbox. Older images ignore these tables.
CREATE TABLE IF NOT EXISTS email_alert_incidents (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 kind text NOT NULL, entity_key text NOT NULL,
 source text NOT NULL DEFAULT '', account_id text NOT NULL DEFAULT '',
 payload jsonb NOT NULL DEFAULT '{}', active boolean NOT NULL DEFAULT true,
 occurrence integer NOT NULL DEFAULT 1 CHECK(occurrence>0),
 first_seen_at timestamptz NOT NULL DEFAULT now(), last_seen_at timestamptz NOT NULL DEFAULT now(),
 resolved_at timestamptz, scan_token text NOT NULL,
 UNIQUE(kind,entity_key)
);
CREATE TABLE IF NOT EXISTS email_alert_attempts (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), recipient text NOT NULL,
 item_count integer NOT NULL CHECK(item_count>0), is_test boolean NOT NULL DEFAULT false,
 sender text NOT NULL DEFAULT '', config_revision text NOT NULL DEFAULT '',
 status text NOT NULL CHECK(status IN ('sending','sent','failed','uncertain','cancelled')),
 error text NOT NULL DEFAULT '', created_at timestamptz NOT NULL DEFAULT now(), finished_at timestamptz
);
CREATE TABLE IF NOT EXISTS email_alert_outbox (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), dedupe_key text NOT NULL UNIQUE,
 incident_id uuid REFERENCES email_alert_incidents(id) ON DELETE SET NULL,
 occurrence integer NOT NULL DEFAULT 1, recipient text NOT NULL,
 period_key text NOT NULL, is_test boolean NOT NULL DEFAULT false,
 status text NOT NULL DEFAULT 'queued' CHECK(status IN ('queued','sending','sent','failed','cancelled','uncertain')),
 attempts integer NOT NULL DEFAULT 0 CHECK(attempts>=0),
 attempt_id uuid REFERENCES email_alert_attempts(id),
 next_attempt_at timestamptz NOT NULL DEFAULT now(), started_at timestamptz,
 sent_at timestamptz, error text NOT NULL DEFAULT '', created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS email_alert_outbox_ready_idx ON email_alert_outbox(next_attempt_at,created_at) WHERE status='queued';
CREATE INDEX IF NOT EXISTS email_alert_attempts_history_idx ON email_alert_attempts(created_at DESC,id DESC);
CREATE INDEX IF NOT EXISTS email_alert_outbox_attempt_idx ON email_alert_outbox(attempt_id);
CREATE TABLE IF NOT EXISTS email_alert_digest_runs (
 recipient text NOT NULL, period_key text NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(recipient,period_key)
);
