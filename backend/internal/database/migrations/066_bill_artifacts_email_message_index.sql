CREATE INDEX IF NOT EXISTS idx_bill_artifacts_email_body_message
    ON bill_artifacts (bill_id, (source_meta->>'message_id'), created_at)
    WHERE kind IN ('email_html', 'email_text')
      AND source_meta ? 'message_id';
