-- PDF is an immutable visual snapshot of the same prepared HTML used by the
-- BillFlow email dialog. Existing completed HTML copies remain untouched;
-- queued/retryable work switches to PDF during this rollout.
ALTER TABLE google_drive_email_exports
    ADD COLUMN IF NOT EXISTS output_format TEXT NOT NULL DEFAULT 'html'
        CHECK (output_format IN ('html', 'pdf'));

ALTER TABLE google_drive_email_exports
    ADD COLUMN IF NOT EXISTS render_warning TEXT NOT NULL DEFAULT '';

UPDATE google_drive_email_exports
   SET output_format = 'pdf',
       remote_path = REGEXP_REPLACE(remote_path, '\.(html|txt)$', '.pdf', 'i'),
       render_warning = '',
       updated_at = NOW()
 WHERE status IN ('queued', 'running', 'failed', 'skipped', 'conflict')
   AND output_format = 'html';
