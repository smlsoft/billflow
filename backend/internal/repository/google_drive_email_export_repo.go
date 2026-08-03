package repository

import (
	"database/sql"
	"fmt"
	"time"

	"billflow/internal/models"
)

type GoogleDriveEmailExportRepo struct {
	db *sql.DB
}

func NewGoogleDriveEmailExportRepo(db *sql.DB) *GoogleDriveEmailExportRepo {
	return &GoogleDriveEmailExportRepo{db: db}
}

func (r *GoogleDriveEmailExportRepo) InsertQueued(job *models.GoogleDriveEmailExport) (bool, error) {
	if job == nil || job.BillID == "" {
		return false, fmt.Errorf("google drive export bill_id required")
	}
	var artifactID any
	if job.SourceArtifactID != "" {
		artifactID = job.SourceArtifactID
	}
	var createdBy any
	if job.CreatedBy != nil && *job.CreatedBy != "" {
		createdBy = *job.CreatedBy
	}
	err := r.db.QueryRow(`
		INSERT INTO google_drive_email_exports
		  (bill_id, source_artifact_id, source_sha256, source_content_type, source_filename,
		   source_channel, order_date, payment_token, sml_doc_no, marketplace_order_id,
		   charge_amount, remote_path, status, priority, next_attempt_at, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7::date,$8,$9,$10,$11,$12,'queued',$13,NOW(),$14)
		ON CONFLICT (bill_id) DO NOTHING
		RETURNING id, created_at, updated_at`,
		job.BillID, artifactID, job.SourceSHA256, job.SourceContentType, job.SourceFilename,
		job.SourceChannel, job.OrderDate, job.PaymentToken, job.SMLDocNo, job.MarketplaceOrderID,
		job.ChargeAmount, job.RemotePath, job.Priority, createdBy,
	).Scan(&job.ID, &job.CreatedAt, &job.UpdatedAt)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("insert google drive export: %w", err)
	}
	job.Status = "queued"
	job.AttemptCount = 0
	job.NextAttemptAt = job.CreatedAt
	return true, nil
}

// ClaimDue atomically reserves work for one process. SKIP LOCKED makes an
// overlapping cron tick or a second backend harmless.
func (r *GoogleDriveEmailExportRepo) ClaimDue(limit int) ([]models.GoogleDriveEmailExport, error) {
	if limit < 1 {
		limit = 1
	}
	if limit > 10 {
		limit = 10
	}
	rows, err := r.db.Query(`
		WITH due AS (
			SELECT id
			  FROM google_drive_email_exports
			 WHERE status = 'queued' AND next_attempt_at <= NOW()
			 ORDER BY priority ASC, next_attempt_at ASC, created_at ASC
			 LIMIT $1
			 FOR UPDATE SKIP LOCKED
		)
		UPDATE google_drive_email_exports e
		   SET status = 'running',
		       attempt_count = attempt_count + 1,
		       last_attempt_at = NOW(),
		       started_at = NOW(),
		       updated_at = NOW()
		  FROM due
		 WHERE e.id = due.id
		 RETURNING e.id, e.bill_id, COALESCE(e.source_artifact_id::text, ''), e.source_sha256,
		           e.source_content_type, e.source_filename, e.source_channel, e.order_date::text,
		           e.payment_token, e.sml_doc_no, e.marketplace_order_id, e.charge_amount,
		           e.remote_path, e.status, e.priority, e.attempt_count, e.next_attempt_at,
		           e.last_attempt_at, e.started_at, e.uploaded_at, e.last_error,
		           e.created_by::text, e.created_at, e.updated_at`, limit)
	if err != nil {
		return nil, fmt.Errorf("claim google drive exports: %w", err)
	}
	defer rows.Close()
	jobs := []models.GoogleDriveEmailExport{}
	for rows.Next() {
		job, err := scanGoogleDriveEmailExport(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

func (r *GoogleDriveEmailExportRepo) MarkSucceeded(id string) error {
	_, err := r.db.Exec(`
		UPDATE google_drive_email_exports
		   SET status = 'succeeded', uploaded_at = NOW(), last_error = '', updated_at = NOW()
		 WHERE id = $1`, id)
	return err
}

func (r *GoogleDriveEmailExportRepo) MarkConflict(id, message string) error {
	_, err := r.db.Exec(`
		UPDATE google_drive_email_exports
		   SET status = 'conflict', last_error = $2, updated_at = NOW()
		 WHERE id = $1`, id, message)
	return err
}

func (r *GoogleDriveEmailExportRepo) MarkSkipped(id, message string) error {
	_, err := r.db.Exec(`
		UPDATE google_drive_email_exports
		   SET status = 'skipped', last_error = $2, updated_at = NOW()
		 WHERE id = $1`, id, message)
	return err
}

func (r *GoogleDriveEmailExportRepo) MarkRetryOrFailed(id, message string, retryAt time.Time, final bool) error {
	status := "queued"
	if final {
		status = "failed"
	}
	_, err := r.db.Exec(`
		UPDATE google_drive_email_exports
		   SET status = $2, next_attempt_at = $3, last_error = $4, updated_at = NOW()
		 WHERE id = $1`, id, status, retryAt, message)
	return err
}

func (r *GoogleDriveEmailExportRepo) Retry(id string) (bool, error) {
	res, err := r.db.Exec(`
		UPDATE google_drive_email_exports
		   SET status = 'queued', attempt_count = 0, next_attempt_at = NOW(), last_error = '', updated_at = NOW()
		 WHERE id = $1 AND status IN ('failed', 'conflict', 'skipped')`, id)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

func (r *GoogleDriveEmailExportRepo) RecoverInterrupted() (int64, error) {
	res, err := r.db.Exec(`
		UPDATE google_drive_email_exports
		   SET status = 'queued', next_attempt_at = NOW(), last_error = 'server restarted while upload was running', updated_at = NOW()
		 WHERE status = 'running'`)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (r *GoogleDriveEmailExportRepo) List(limit int) ([]models.GoogleDriveEmailExport, models.GoogleDriveEmailExportCounts, error) {
	if limit < 1 || limit > 200 {
		limit = 50
	}
	counts := models.GoogleDriveEmailExportCounts{}
	if err := r.db.QueryRow(`
		SELECT COUNT(*) FILTER (WHERE status = 'queued'),
		       COUNT(*) FILTER (WHERE status = 'running'),
		       COUNT(*) FILTER (WHERE status = 'succeeded'),
		       COUNT(*) FILTER (WHERE status = 'failed'),
		       COUNT(*) FILTER (WHERE status = 'conflict')
		  FROM google_drive_email_exports`).Scan(&counts.Queued, &counts.Running, &counts.Succeeded, &counts.Failed, &counts.Conflict); err != nil {
		return nil, counts, err
	}
	rows, err := r.db.Query(`
		SELECT id, bill_id, COALESCE(source_artifact_id::text, ''), source_sha256,
		       source_content_type, source_filename, source_channel, order_date::text,
		       payment_token, sml_doc_no, marketplace_order_id, charge_amount,
		       remote_path, status, priority, attempt_count, next_attempt_at,
		       last_attempt_at, started_at, uploaded_at, last_error, created_by::text, created_at, updated_at
	  FROM google_drive_email_exports
	 ORDER BY CASE status WHEN 'running' THEN 0 WHEN 'queued' THEN 1 WHEN 'failed' THEN 2 WHEN 'conflict' THEN 3 ELSE 4 END,
	          updated_at DESC
	 LIMIT $1`, limit)
	if err != nil {
		return nil, counts, err
	}
	defer rows.Close()
	jobs := []models.GoogleDriveEmailExport{}
	for rows.Next() {
		job, err := scanGoogleDriveEmailExport(rows)
		if err != nil {
			return nil, counts, err
		}
		jobs = append(jobs, job)
	}
	return jobs, counts, rows.Err()
}

// ListBackfillBillIDs returns only current marketplace purchase POs. The date
// is taken from raw_data.doc_date/order_datetime, never from ingestion time.
func (r *GoogleDriveEmailExportRepo) ListBackfillBillIDs(dateFrom, dateTo string, limit int) ([]string, error) {
	if limit < 1 || limit > 501 {
		limit = 501
	}
	rows, err := r.db.Query(`
		SELECT id
	  FROM bills
	 WHERE source IN ('shopee_shipped', 'lazada_email')
	   AND bill_type = 'purchase'
	   AND status = 'sent'
	   AND archived_at IS NULL
	   AND COALESCE(sml_doc_no, '') <> ''
	   AND COALESCE(NULLIF(raw_data->>'doc_date', ''), LEFT(COALESCE(raw_data->>'order_datetime', ''), 10)) >= $1
	   AND COALESCE(NULLIF(raw_data->>'doc_date', ''), LEFT(COALESCE(raw_data->>'order_datetime', ''), 10)) <= $2
	 ORDER BY COALESCE(NULLIF(raw_data->>'doc_date', ''), LEFT(COALESCE(raw_data->>'order_datetime', ''), 10)), id
	 LIMIT $3`, dateFrom, dateTo, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *GoogleDriveEmailExportRepo) ListRecentUnqueuedSentBillIDs(limit int) ([]string, error) {
	if limit < 1 || limit > 500 {
		limit = 500
	}
	rows, err := r.db.Query(`
		SELECT b.id
	  FROM bills b
	  LEFT JOIN google_drive_email_exports e ON e.bill_id = b.id
	 WHERE b.source IN ('shopee_shipped', 'lazada_email')
	   AND b.bill_type = 'purchase'
	   AND b.status = 'sent'
	   AND b.archived_at IS NULL
	   AND COALESCE(b.sml_doc_no, '') <> ''
	   AND b.sent_at >= NOW() - INTERVAL '24 hours'
	   AND e.id IS NULL
	 ORDER BY b.sent_at ASC
	 LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

type googleDriveEmailExportScanner interface {
	Scan(dest ...interface{}) error
}

func scanGoogleDriveEmailExport(s googleDriveEmailExportScanner) (models.GoogleDriveEmailExport, error) {
	var job models.GoogleDriveEmailExport
	var sourceArtifactID, createdBy sql.NullString
	if err := s.Scan(
		&job.ID, &job.BillID, &sourceArtifactID, &job.SourceSHA256,
		&job.SourceContentType, &job.SourceFilename, &job.SourceChannel, &job.OrderDate,
		&job.PaymentToken, &job.SMLDocNo, &job.MarketplaceOrderID, &job.ChargeAmount,
		&job.RemotePath, &job.Status, &job.Priority, &job.AttemptCount, &job.NextAttemptAt,
		&job.LastAttemptAt, &job.StartedAt, &job.UploadedAt, &job.LastError, &createdBy,
		&job.CreatedAt, &job.UpdatedAt,
	); err != nil {
		return job, err
	}
	if sourceArtifactID.Valid {
		job.SourceArtifactID = sourceArtifactID.String
	}
	if createdBy.Valid && createdBy.String != "" {
		job.CreatedBy = &createdBy.String
	}
	return job, nil
}
