package emailalert

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/lib/pq"
	"go.uber.org/zap"
)

type Service struct {
	store  *Store
	mailer Mailer
	log    *zap.Logger
	mu     sync.Mutex
}

func NewService(s *Store, m Mailer, log *zap.Logger) *Service {
	return &Service{store: s, mailer: m, log: log}
}
func (s *Service) Tick(ctx context.Context) {
	if !s.mu.TryLock() {
		return
	}
	defer s.mu.Unlock()
	if err := s.store.recoverStale(ctx); err != nil {
		s.log.Error("email_alert_recover_failed", zap.Error(err))
		return
	}
	if err := s.store.Scan(ctx, time.Now()); err != nil {
		s.log.Error("email_alert_scan_failed", zap.Error(err))
		return
	}
	for i := 0; i < 20 && ctx.Err() == nil; i++ {
		more, err := s.deliver(ctx)
		if err != nil {
			s.log.Error("email_alert_worker_failed", zap.Error(err))
			return
		}
		if !more {
			return
		}
	}
}
func (s *Store) recoverStale(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `UPDATE email_alert_outbox SET status='uncertain',error='กระบวนการส่งหยุดก่อนยืนยันผล โปรดตรวจผู้รับก่อนลองใหม่' WHERE status='sending' AND started_at<now()-interval '2 minutes'`); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE email_alert_attempts SET status='uncertain',finished_at=now(),error='กระบวนการส่งหยุดก่อนยืนยันผล โปรดตรวจผู้รับก่อนลองใหม่' WHERE status='sending' AND created_at<now()-interval '2 minutes'`); err != nil {
		return err
	}
	return tx.Commit()
}

type delivery struct {
	ID                   string
	Issue                Issue
	IsTest               bool
	Occurrence, Attempts int
}
type batch struct {
	ID, Recipient, Period string
	Items                 []delivery
}

func (s *Store) claim(ctx context.Context) (*batch, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	b := &batch{}
	err = tx.QueryRowContext(ctx, `SELECT recipient,period_key FROM email_alert_outbox WHERE status='queued' AND next_attempt_at<=now() ORDER BY created_at,id LIMIT 1`).Scan(&b.Recipient, &b.Period)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var locked bool
	if err = tx.QueryRowContext(ctx, `SELECT pg_try_advisory_xact_lock(75261119,hashtext($1))`, b.Recipient+"\n"+b.Period).Scan(&locked); err != nil {
		return nil, err
	}
	if !locked {
		return nil, nil
	}
	rows, err := tx.QueryContext(ctx, `UPDATE email_alert_outbox SET status='sending',started_at=now(),attempts=attempts+1 WHERE recipient=$1 AND period_key=$2 AND status='queued' AND next_attempt_at<=now()
 RETURNING id::text,is_test,occurrence,attempts,COALESCE((SELECT payload FROM email_alert_incidents WHERE id=incident_id),'{}'::jsonb)`, b.Recipient, b.Period)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var d delivery
		var raw []byte
		if err = rows.Scan(&d.ID, &d.IsTest, &d.Occurrence, &d.Attempts, &raw); err != nil {
			rows.Close()
			return nil, err
		}
		if err = json.Unmarshal(raw, &d.Issue); err != nil {
			rows.Close()
			return nil, err
		}
		b.Items = append(b.Items, d)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	if len(b.Items) == 0 {
		return nil, nil
	}
	if err = tx.QueryRowContext(ctx, `INSERT INTO email_alert_attempts(recipient,item_count,is_test,status) VALUES($1,$2,$3,'sending') RETURNING id::text`, b.Recipient, len(b.Items), b.Items[0].IsTest).Scan(&b.ID); err != nil {
		return nil, err
	}
	ids := []string{}
	for _, d := range b.Items {
		ids = append(ids, d.ID)
	}
	if _, err = tx.ExecContext(ctx, `UPDATE email_alert_outbox SET attempt_id=$2 WHERE id=ANY($1::uuid[]) AND status='sending'`, pq.Array(ids), b.ID); err != nil {
		return nil, err
	}

	return b, tx.Commit()
}
func (s *Service) deliver(ctx context.Context) (bool, error) {
	b, err := s.store.claim(ctx)
	if err != nil || b == nil {
		return false, err
	}
	c, enc, err := loadConfig(ctx, s.store.db)
	if err != nil {
		return true, err
	}
	// Load errors leave the lease uncertain, never mark a message sent speculatively.
	now := time.Now()
	current, err := currentIssues(ctx, s.store.db, now)
	if err != nil {
		return true, err
	}
	live := map[string]Issue{}
	for _, i := range current {
		live[i.Kind+"\x00"+i.EntityKey] = i
	}
	kept := []delivery{}
	for _, d := range b.Items {
		permitted := slices.Contains(c.Recipients, b.Recipient)
		if !d.IsTest {
			latest, exists := live[d.Issue.Kind+"\x00"+d.Issue.EntityKey]
			if exists {
				d.Issue = latest
			}
			periodOK := (c.Mode == "immediate" && b.Period == "event") || (c.Mode == "daily" && b.Period == "daily:"+now.In(bangkok).Format("2006-01-02"))
			permitted = permitted && c.Enabled && exists && c.matches(d.Issue) && periodOK
		}
		if permitted {
			kept = append(kept, d)
		} else {
			if _, err = s.store.db.ExecContext(ctx, `UPDATE email_alert_outbox SET status='cancelled',error='เงื่อนไขเปลี่ยนหรือปัญหาถูกแก้ไขก่อนส่ง' WHERE id=$1 AND attempt_id=$2 AND status='sending'`, d.ID, b.ID); err != nil {
				return true, err
			}
		}
	}
	b.Items = kept
	if len(kept) == 0 {
		_, err = s.store.db.ExecContext(ctx, `UPDATE email_alert_attempts SET status='cancelled',finished_at=now(),error='ยกเลิกก่อนส่งตามเงื่อนไขปัจจุบัน' WHERE id=$1`, b.ID)
		return true, err
	}

	if _, err = s.store.db.ExecContext(ctx, `UPDATE email_alert_attempts SET item_count=$2,sender=$3,config_revision=$4 WHERE id=$1`, b.ID, len(kept), c.From, c.Revision); err != nil {
		return true, err
	}
	sendErr := c.Validate(true)
	password := ""
	if sendErr == nil {
		password, sendErr = s.store.decrypt(enc)
	}
	if sendErr == nil {
		sendErr = s.mailer.Send(ctx, c, password, renderMessage(c, *b, now))
	}
	return true, s.store.finish(ctx, *b, sendErr)
}
func renderMessage(c Config, b batch, now time.Time) Message {
	subject := "BillFlow: แจ้งปัญหาที่ต้องตรวจสอบ"
	var body strings.Builder
	fmt.Fprintf(&body, "BillFlow — ตรวจพบเมื่อ %s (เวลาไทย)\n\n", now.In(bangkok).Format("02/01/2006 15:04"))
	if b.Items[0].IsTest {
		subject = "BillFlow: อีเมลทดสอบการแจ้งเตือน"
		body.WriteString("นี่คืออีเมลทดสอบจากผู้ดูแล BillFlow ไม่ได้สร้างบิลหรือส่ง SML\n")
	} else {
		if strings.HasPrefix(b.Period, "daily:") {
			subject = "BillFlow: สรุปปัญหาค้างรายวัน"
		}
		fmt.Fprintf(&body, "พบ %d รายการที่ต้องตรวจสอบ\n", len(b.Items))
		for i, d := range b.Items {
			if i >= 30 {
				fmt.Fprintf(&body, "\nและอีก %d รายการ โปรดเปิด BillFlow\n", len(b.Items)-30)
				break
			}
			order := strings.ReplaceAll(strings.ReplaceAll(d.Issue.OrderID, "\n", " "), "\r", " ")
			if runes := []rune(order); len(runes) > 100 {
				order = string(runes[:100])
			}
			reference := "คำสั่งซื้อ " + order
			if strings.HasPrefix(d.Issue.EntityKey, "email:") {
				reference = "กลุ่มอีเมล " + strings.TrimPrefix(d.Issue.EntityKey, "email:")
			}
			fmt.Fprintf(&body, "\n• %s | %s | %s\n%s%s\n", KindLabels[d.Issue.Kind], d.Issue.Source, reference, c.BaseURL, d.Issue.Path)
		}
	}
	fmt.Fprintf(&body, "\nเปิด BillFlow: %s/bills\n\nข้อมูลอาจเปลี่ยนหลังตรวจพบ กรุณาตรวจหลักฐานในระบบก่อนดำเนินการ\nการแจ้งนี้ไม่ได้ยืนยันการทุจริต และไม่ได้ยืนยันว่าผู้รับอ่านอีเมลแล้ว\n", c.BaseURL)
	return Message{ID: b.ID, To: b.Recipient, Subject: subject, Body: body.String()}
}
func (s *Store) finish(ctx context.Context, b batch, sendErr error) error {
	state := "sent"
	reason := ""
	uncertain := false
	if sendErr != nil {
		state = "failed"
		reason = sendErr.Error()
		var e *SendError
		if errors.As(sendErr, &e) {
			uncertain = e.Uncertain
		}
		if uncertain {
			state = "uncertain"
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, d := range b.Items {
		status := state
		next := time.Now()
		if state == "failed" && d.Attempts < 3 {
			status = "queued"
			next = next.Add(time.Duration(d.Attempts*d.Attempts) * time.Minute)
		}
		if _, err = tx.ExecContext(ctx, `UPDATE email_alert_outbox SET status=$3,error=$4,next_attempt_at=$5,sent_at=CASE WHEN $3='sent' THEN now() ELSE NULL END WHERE id=$1 AND attempt_id=$2 AND status='sending'`, d.ID, b.ID, status, reason, next); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE email_alert_attempts SET status=$2,error=$3,finished_at=now() WHERE id=$1 AND status='sending'`, b.ID, state, reason); err != nil {
		return err
	}
	return tx.Commit()
}
