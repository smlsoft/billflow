package repository

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"billflow/internal/models"
)

type AuditLogRepo struct {
	db *sql.DB
}

func NewAuditLogRepo(db *sql.DB) *AuditLogRepo {
	return &AuditLogRepo{db: db}
}

// Log writes one audit event. All fields in AuditEntry are optional except Action.
func (r *AuditLogRepo) Log(e models.AuditEntry) error {
	var detailJSON []byte
	if e.Detail != nil {
		var err error
		detailJSON, err = json.Marshal(e.Detail)
		if err != nil {
			return fmt.Errorf("audit log marshal: %w", err)
		}
	}
	level := e.Level
	if level == "" {
		level = "info"
	}
	var traceID *string
	if e.TraceID != "" {
		traceID = &e.TraceID
	}
	_, err := r.db.Exec(
		`INSERT INTO audit_logs (action, target_id, user_id, source, level, duration_ms, trace_id, detail)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		e.Action, e.TargetID, e.UserID, e.Source, level, e.DurationMs, traceID, detailJSON,
	)
	return err
}

// List returns audit logs with optional filters, newest first.
func (r *AuditLogRepo) List(f models.AuditLogFilter) ([]models.AuditLog, int, error) {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 || f.PageSize > 200 {
		f.PageSize = 50
	}

	where := "WHERE 1=1"
	args := []interface{}{}
	n := 1

	if f.Action != "" {
		where += fmt.Sprintf(" AND action = $%d", n)
		args = append(args, f.Action)
		n++
	}
	if f.Source != "" {
		where += fmt.Sprintf(" AND source = $%d", n)
		args = append(args, f.Source)
		n++
	}
	if f.Level != "" {
		where += fmt.Sprintf(" AND level = $%d", n)
		args = append(args, f.Level)
		n++
	}
	if f.DateFrom != "" {
		where += fmt.Sprintf(" AND created_at >= $%d::date", n)
		args = append(args, f.DateFrom)
		n++
	}
	if f.DateTo != "" {
		where += fmt.Sprintf(" AND created_at < ($%d::date + INTERVAL '1 day')", n)
		args = append(args, f.DateTo)
		n++
	}

	var total int
	if err := r.db.QueryRow("SELECT COUNT(*) FROM audit_logs "+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("audit count: %w", err)
	}

	query := "SELECT id, user_id, action, target_id, source, level, duration_ms, trace_id, detail, created_at FROM audit_logs " +
		where + fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d OFFSET $%d", n, n+1)
	args = append(args, f.PageSize, (f.Page-1)*f.PageSize)

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("audit list: %w", err)
	}
	defer rows.Close()

	var logs []models.AuditLog
	for rows.Next() {
		var l models.AuditLog
		var source, traceID sql.NullString
		var detailRaw []byte
		if err := rows.Scan(&l.ID, &l.UserID, &l.Action, &l.TargetID,
			&source, &l.Level, &l.DurationMs, &traceID,
			&detailRaw, &l.CreatedAt); err != nil {
			return nil, 0, err
		}
		l.Source = source.String
		l.TraceID = traceID.String
		if detailRaw != nil {
			l.Detail = json.RawMessage(detailRaw)
		}
		logs = append(logs, l)
	}
	return logs, total, rows.Err()
}

// ListByTarget returns audit_log rows whose target_id matches, oldest-first.
// Used by the BillDetail timeline view to show every event tied to one bill
// (created → confirmed → SML send → retried → ...). Caps at 200 rows so a
// pathological bill with many retries doesn't blow up the response.
func (r *AuditLogRepo) ListByTarget(targetID string) ([]models.AuditLog, error) {
	rows, err := r.db.Query(
		`SELECT id, user_id, action, target_id, source, level, duration_ms,
		        trace_id, detail, created_at
		 FROM audit_logs
		 WHERE target_id = $1
		 ORDER BY created_at ASC
		 LIMIT 200`,
		targetID,
	)
	if err != nil {
		return nil, fmt.Errorf("audit list by target: %w", err)
	}
	defer rows.Close()

	var out []models.AuditLog
	for rows.Next() {
		var l models.AuditLog
		var source, traceID sql.NullString
		var detailRaw []byte
		if err := rows.Scan(&l.ID, &l.UserID, &l.Action, &l.TargetID,
			&source, &l.Level, &l.DurationMs, &traceID,
			&detailRaw, &l.CreatedAt); err != nil {
			return nil, err
		}
		l.Source = source.String
		l.TraceID = traceID.String
		if detailRaw != nil {
			l.Detail = json.RawMessage(detailRaw)
		}
		out = append(out, l)
	}
	return out, rows.Err()
}
