package emailalert

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

type Attempt struct {
	Sender       string     `json:"sender"`
	ID           string     `json:"id"`
	Recipient    string     `json:"recipient"`
	ItemCount    int        `json:"item_count"`
	IsTest       bool       `json:"is_test"`
	Status       string     `json:"status"`
	Error        string     `json:"error"`
	CreatedAt    time.Time  `json:"created_at"`
	FinishedAt   *time.Time `json:"finished_at,omitempty"`
	Retryable    bool       `json:"retryable"`
	RetryPending bool       `json:"retry_pending"`
}
type History struct {
	Data    []Attempt `json:"data"`
	HasMore bool      `json:"has_more"`
	Queued  int       `json:"queued"`
}

func (s *Store) History(ctx context.Context, page int) (History, error) {
	result := History{Data: []Attempt{}}
	rows, err := s.db.QueryContext(ctx, `SELECT a.id::text,a.recipient,a.item_count,a.is_test,a.status,a.error,a.created_at,a.finished_at,a.sender,
 EXISTS(SELECT 1 FROM email_alert_outbox o WHERE o.attempt_id=a.id AND o.status IN ('failed','uncertain')),
 EXISTS(SELECT 1 FROM email_alert_outbox o WHERE o.attempt_id=a.id AND o.status='queued')
 FROM email_alert_attempts a ORDER BY a.created_at DESC,a.id DESC LIMIT 51 OFFSET $1`, (page-1)*50)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var a Attempt
		if err = rows.Scan(&a.ID, &a.Recipient, &a.ItemCount, &a.IsTest, &a.Status, &a.Error, &a.CreatedAt, &a.FinishedAt, &a.Sender, &a.Retryable, &a.RetryPending); err != nil {
			rows.Close()
			return result, err
		}
		result.Data = append(result.Data, a)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	if len(result.Data) > 50 {
		result.HasMore = true
		result.Data = result.Data[:50]
	}
	err = s.db.QueryRowContext(ctx, `SELECT count(*) FROM email_alert_outbox WHERE status IN ('queued','sending')`).Scan(&result.Queued)
	return result, err
}
func (s *Store) Retry(ctx context.Context, id, user string, confirm bool) error {
	if !uuidRE.MatchString(id) {
		return RequestError("รหัสประวัติไม่ถูกต้อง")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var status string
	if err = tx.QueryRowContext(ctx, `SELECT status FROM email_alert_attempts WHERE id=$1 FOR UPDATE`, id).Scan(&status); err == sql.ErrNoRows {
		return RequestError("ไม่พบประวัติการส่ง")
	}
	if err != nil {
		return err
	}
	if status == "uncertain" && !confirm {
		return RequestError("ต้องยืนยันว่าได้ตรวจผู้รับแล้ว การลองใหม่อาจส่งซ้ำ")
	}
	if status != "failed" && status != "uncertain" {
		return RequestError("รายการนี้ไม่สามารถลองส่งใหม่ได้")
	}
	res, err := tx.ExecContext(ctx, `UPDATE email_alert_outbox SET status='queued',next_attempt_at=now(),error='' WHERE attempt_id=$1 AND status IN ('failed','uncertain')`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return RequestError("รายการถูกลองใหม่หรือเปลี่ยนสถานะแล้ว กรุณาโหลดประวัติใหม่")
	}
	detail, _ := json.Marshal(map[string]any{"attempt_id": id, "confirmed_duplicate_risk": confirm})
	if _, err = tx.ExecContext(ctx, `INSERT INTO audit_logs(action,user_id,source,detail) VALUES('email_alert_retry_queued',$1,'settings',$2)`, user, string(detail)); err != nil {
		return err
	}
	return tx.Commit()
}
