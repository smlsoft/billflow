-- Keep the default mapping table view fast as learned mappings grow.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_mappings_usage_raw_name
  ON mappings (usage_count DESC, raw_name);
