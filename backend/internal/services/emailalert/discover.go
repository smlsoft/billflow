package emailalert

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"time"
)

var ErrRateLimited = fmt.Errorf("กรุณารอ 1 นาทีก่อนส่งอีเมลทดสอบอีกครั้ง")

var bangkok = time.FixedZone("Asia/Bangkok", 7*60*60)

// Only current actionable states; scanning does not modify bills or shipping history.
const issueSQL = `SELECT CASE WHEN b.raw_data->'shipping_check_summary'->>'status'='mismatch' THEN 'shipping_mismatch' ELSE 'shipping_unverifiable' END,
 'bill:'||b.id::text,b.source,COALESCE(b.raw_data->>'imap_account_id',''),COALESCE(b.raw_data->>'order_id',b.raw_data->>'shopee_order_id',b.raw_data->>'lazada_order_id',''),
 '/bills/'||b.id::text,'ชื่อผู้รับหรือที่อยู่จัดส่งต้องตรวจสอบ'
 FROM bills b WHERE b.bill_type='purchase' AND b.source IN ('shopee_shipped','lazada_email') AND b.archived_at IS NULL AND b.status IN ('pending','needs_review','failed')
 AND b.raw_data->'shipping_check_summary'->>'status' IN ('mismatch','unverifiable')
 UNION ALL
 SELECT 'email_incomplete','email:'||g.id::text,g.source,COALESCE(g.imap_account_id::text,''),'','',
 'อีเมลคำสั่งซื้ออ่านไม่ครบ โปรดตรวจใน BillFlow'
 FROM marketplace_email_groups g WHERE g.status='attention' OR (g.status='processing' AND g.last_seen_at<$1::timestamptz-interval '5 minutes')
 UNION ALL
 SELECT 'sml_failed','bill:'||b.id::text,b.source,COALESCE(b.raw_data->>'imap_account_id',''),COALESCE(b.raw_data->>'order_id',b.raw_data->>'shopee_order_id',''),
 '/bills/'||b.id::text,'การส่ง SML ล้มเหลว โปรดเปิดรายละเอียดบิล'
 FROM bills b WHERE b.status='failed' AND b.archived_at IS NULL AND (b.sml_payload IS NOT NULL OR EXISTS(SELECT 1 FROM audit_logs a WHERE a.target_id=b.id AND a.action='sml_failed'))`

type issueQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func currentIssues(ctx context.Context, q issueQueryer, now time.Time) ([]Issue, error) {
	rows, err := q.QueryContext(ctx, issueSQL, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	issues := []Issue{}
	for rows.Next() {
		var i Issue
		if err = rows.Scan(&i.Kind, &i.EntityKey, &i.Source, &i.AccountID, &i.OrderID, &i.Path, &i.Summary); err != nil {
			return nil, err
		}
		if i.Path == "" {
			i.Path = "/bills?email_completeness=attention"
			if i.AccountID != "" {
				i.Path += "&email_account_id=" + url.QueryEscape(i.AccountID)
			}
		}
		issues = append(issues, i)
	}
	return issues, rows.Err()
}

type incident struct {
	ID         string
	Occurrence int
	Issue      Issue
}

func (s *Store) Scan(ctx context.Context, now time.Time) error {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var lock bool
	if err = tx.QueryRowContext(ctx, `SELECT pg_try_advisory_xact_lock(75261119,1)`).Scan(&lock); err != nil {
		return err
	}
	if !lock {
		return nil
	}
	// Serialize settings changes against queue production, without holding locks for SMTP.
	var raw string
	err = tx.QueryRowContext(ctx, `SELECT value FROM app_settings WHERE key=$1 FOR UPDATE`, configKey).Scan(&raw)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	c, _, err := loadConfig(ctx, tx)
	if err != nil {
		return err
	}
	if !c.Enabled {
		return nil
	}
	if err = c.Validate(true); err != nil {
		return err
	}
	issues, err := currentIssues(ctx, tx, now)
	if err != nil {
		return err
	}
	var token string
	if err = tx.QueryRowContext(ctx, `SELECT gen_random_uuid()::text`).Scan(&token); err != nil {
		return err
	}

	active := []incident{}
	payload, err := json.Marshal(issues)
	if err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, `INSERT INTO email_alert_incidents(kind,entity_key,source,account_id,payload,scan_token,first_seen_at,last_seen_at)
 SELECT x->>'kind',x->>'entity_key',x->>'source',x->>'account_id',x,$2,$3,$3 FROM jsonb_array_elements($1::jsonb) x
 ON CONFLICT(kind,entity_key) DO UPDATE SET active=true,occurrence=email_alert_incidents.occurrence+CASE WHEN email_alert_incidents.active THEN 0 ELSE 1 END,
 payload=EXCLUDED.payload,source=EXCLUDED.source,account_id=EXCLUDED.account_id,last_seen_at=EXCLUDED.last_seen_at,resolved_at=NULL,scan_token=EXCLUDED.scan_token
 RETURNING id::text,occurrence,payload`, string(payload), token, now)
	if err != nil {
		return err
	}
	for rows.Next() {
		var i incident
		var raw []byte
		if err = rows.Scan(&i.ID, &i.Occurrence, &raw); err != nil {
			rows.Close()
			return err
		}
		if err = json.Unmarshal(raw, &i.Issue); err != nil {
			rows.Close()
			return err
		}
		if c.matches(i.Issue) {
			active = append(active, i)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}

	if _, err = tx.ExecContext(ctx, `UPDATE email_alert_incidents SET active=false,resolved_at=$2 WHERE active AND scan_token<>$1`, token, now); err != nil {
		return err
	}
	period := "event"
	if c.Mode == "daily" {
		local := now.In(bangkok)
		if local.Hour() < c.DailyHour {
			return tx.Commit()
		}
		period = "daily:" + local.Format("2006-01-02")
	}
	for _, recipient := range c.Recipients {
		if c.Mode == "daily" {
			res, e := tx.ExecContext(ctx, `INSERT INTO email_alert_digest_runs(recipient,period_key,created_at) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, recipient, period, now)
			if e != nil {
				return e
			}
			n, _ := res.RowsAffected()
			if n == 0 {
				continue
			}
		}

		entries := make([]map[string]any, 0, len(active))
		for _, i := range active {
			entries = append(entries, map[string]any{"key": dedupeKey(i.ID, i.Occurrence, recipient, period), "id": i.ID, "occurrence": i.Occurrence})
		}
		raw, err := json.Marshal(entries)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO email_alert_outbox(dedupe_key,incident_id,occurrence,recipient,period_key,next_attempt_at,created_at)
 SELECT x->>'key',(x->>'id')::uuid,(x->>'occurrence')::integer,$2,$3,$4,$4 FROM jsonb_array_elements($1::jsonb) x
 ON CONFLICT(dedupe_key) DO UPDATE SET status='queued',next_attempt_at=EXCLUDED.next_attempt_at,error='' WHERE email_alert_outbox.status='cancelled'`, string(raw), recipient, period, now)
		if err != nil {
			return err
		}

	}
	return tx.Commit()
}
func (s *Store) QueueTest(ctx context.Context, requestID, user string) error {
	if !uuidRE.MatchString(requestID) {
		return fmt.Errorf("รหัสคำขอไม่ถูกต้อง")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var settingsRaw string
	if err = tx.QueryRowContext(ctx, `SELECT value FROM app_settings WHERE key=$1 FOR UPDATE`, configKey).Scan(&settingsRaw); err != nil {
		return err
	}
	c, _, err := loadConfig(ctx, tx)
	if err != nil {
		return err
	}
	if err = c.Validate(true); err != nil {
		return err
	}
	var duplicate bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM email_alert_outbox WHERE period_key=$1 AND is_test)`, "test:"+requestID).Scan(&duplicate); err != nil {
		return err
	}
	if duplicate {
		return nil
	}
	var lastTest string
	err = tx.QueryRowContext(ctx, `SELECT value FROM app_settings WHERE key='email.alerts.last_test_at'`).Scan(&lastTest)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if last, e := time.Parse(time.RFC3339Nano, lastTest); e == nil && time.Since(last) < time.Minute {
		return ErrRateLimited
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO app_settings(key,value,is_secret) VALUES('email.alerts.last_test_at',$1,false) ON CONFLICT(key) DO UPDATE SET value=EXCLUDED.value,updated_at=now()`, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return err
	}
	for _, recipient := range c.Recipients {
		if _, err = tx.ExecContext(ctx, `INSERT INTO email_alert_outbox(dedupe_key,recipient,period_key,is_test) VALUES($1,$2,$3,true) ON CONFLICT DO NOTHING`, "test:"+dedupeKey(requestID, 1, recipient, "test"), recipient, "test:"+requestID); err != nil {
			return err
		}
	}
	detail, _ := json.Marshal(map[string]any{"request_id": requestID, "recipients": c.Recipients})
	if _, err = tx.ExecContext(ctx, `INSERT INTO audit_logs(action,user_id,source,detail) VALUES('email_alert_test_queued',$1,'settings',$2)`, user, string(detail)); err != nil {
		return err
	}
	return tx.Commit()
}
