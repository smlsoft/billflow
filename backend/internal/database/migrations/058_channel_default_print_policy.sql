-- 058_channel_default_print_policy.sql
-- Configurable marketplace email print readiness rules per channel.

ALTER TABLE channel_defaults
  ADD COLUMN IF NOT EXISTS print_policy JSONB NOT NULL DEFAULT '{}'::jsonb;
