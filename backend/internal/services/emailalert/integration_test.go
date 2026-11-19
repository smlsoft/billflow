package emailalert

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/lib/pq"
	"go.uber.org/zap"
)

type fakeMailer struct {
	messages []Message
	err      error
}

func (f *fakeMailer) Send(_ context.Context, _ Config, _ string, m Message) error {
	f.messages = append(f.messages, m)
	return f.err
}
func TestEmailAlertIntegration(t *testing.T) {
	dsn := os.Getenv("BILLFLOW_EMAIL_ALERT_TEST_DSN")
	if dsn == "" {
		t.Skip("set disposable database DSN")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	// Use one connection so all queries share the disposable schema.
	db.SetMaxOpenConns(1)
	schema := pq.QuoteIdentifier(fmt.Sprintf("alert_test_%d", time.Now().UnixNano()))
	if _, err = db.Exec("CREATE SCHEMA " + schema); err != nil {
		t.Fatal(err)
	}
	defer db.Exec("DROP SCHEMA " + schema + " CASCADE")
	if _, err = db.Exec("SET search_path TO " + schema); err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE app_settings(key text PRIMARY KEY,value text NOT NULL,is_secret bool,updated_by uuid,updated_at timestamptz DEFAULT now());
 CREATE TABLE audit_logs(action text,user_id uuid,target_id uuid,source text,detail jsonb);
 CREATE TABLE imap_accounts(id uuid PRIMARY KEY,channel text);
 CREATE TABLE bills(id uuid PRIMARY KEY,bill_type text,source text,status text,archived_at timestamptz,raw_data jsonb,sml_payload jsonb);
 CREATE TABLE marketplace_email_groups(id uuid PRIMARY KEY,source text,imap_account_id uuid,status text,last_seen_at timestamptz);`)
	if err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile("../../database/migrations/075_email_alerts.sql")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err = db.Exec(string(migration)); err != nil {
			t.Fatal(err)
		}
	}
	ctx := context.Background()
	store := NewStore(db, "synthetic-test-key")
	user := "00000000-0000-0000-0000-000000000001"
	bill := "00000000-0000-0000-0000-000000000002"
	c := validConfig()
	c.Recipients = []string{"owner@example.com", "review@example.com"}
	c.Kinds = []string{"shipping_mismatch", "shipping_unverifiable", "email_incomplete", "sml_failed"}
	c.Password = "synthetic-password"
	c.Enabled = false
	c, err = store.Save(ctx, c, user)
	if err != nil {
		t.Fatal(err)
	}
	if c.Password != "" || !c.HasPassword {
		t.Fatal("password leaked/not preserved")
	}
	var audit string
	if err = db.QueryRow(`SELECT detail::text FROM audit_logs LIMIT 1`).Scan(&audit); err != nil || strings.Contains(audit, "synthetic-password") {
		t.Fatal("secret in audit", err)
	}
	// Saved credentials cannot be silently redirected to a different SMTP identity.
	changedSender := c
	changedSender.Host = "other.example.com"
	if _, err = store.Save(ctx, changedSender, user); err == nil {
		t.Fatal("existing credential reused for changed SMTP host")
	}
	changedSender = c
	changedSender.Username = "other@example.com"
	if _, err = store.Save(ctx, changedSender, user); err == nil {
		t.Fatal("existing credential reused for changed SMTP username")
	}
	missingAccount := c
	missingAccount.AccountIDs = []string{"00000000-0000-0000-0000-000000000099"}
	if _, err = store.Save(ctx, missingAccount, user); err == nil {
		t.Fatal("unknown inbox accepted")
	}
	c.Enabled = true
	c, err = store.Save(ctx, c, user)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO bills VALUES($1,'purchase','lazada_email','pending',NULL,'{"order_id":"TEST","shipping_check_summary":{"status":"mismatch"}}',NULL);
 `, bill)
	if err == nil {
		_, err = db.Exec(`INSERT INTO marketplace_email_groups VALUES('00000000-0000-0000-0000-000000000003','shopee_shipped',NULL,'attention',now())`)
	}
	if err != nil {
		t.Fatal(err)
	}
	count := func(query string, want int) {
		t.Helper()
		var got int
		if e := db.QueryRow(query).Scan(&got); e != nil || got != want {
			t.Fatalf("%s => %d want %d err %v", query, got, want, e)
		}
	}
	for i := 0; i < 2; i++ {
		if err = store.Scan(ctx, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	count(`SELECT count(*) FROM email_alert_outbox`, 4)
	mailer := &fakeMailer{}
	service := NewService(store, mailer, zap.NewNop())
	service.Tick(ctx)
	if len(mailer.messages) != 2 {
		t.Fatalf("expected combined email per recipient, got %d", len(mailer.messages))
	}
	count(`SELECT count(*) FROM email_alert_outbox WHERE status='sent'`, 4)
	service.Tick(ctx)
	if len(mailer.messages) != 2 {
		t.Fatal("repeat poll duplicated notifications")
	}
	if _, err = db.Exec(`UPDATE bills SET raw_data=jsonb_set(raw_data,'{shipping_check_summary,status}','"match"');UPDATE marketplace_email_groups SET status='complete'`); err != nil {
		t.Fatal(err)
	}
	if err = store.Scan(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	count(`SELECT count(*) FROM email_alert_incidents WHERE active`, 0)
	if _, err = db.Exec(`UPDATE bills SET raw_data=jsonb_set(raw_data,'{shipping_check_summary,status}','"mismatch"')`); err != nil {
		t.Fatal(err)
	}
	service.Tick(ctx)
	if len(mailer.messages) != 4 {
		t.Fatal("reopened issue not notified")
	}
	count(`SELECT max(occurrence) FROM email_alert_incidents`, 2)
	// An uncertain SMTP result must never auto-send again.
	if _, err = db.Exec(`UPDATE bills SET raw_data=jsonb_set(raw_data,'{shipping_check_summary,status}','"unverifiable"')`); err != nil {
		t.Fatal(err)
	}
	mailer.err = &SendError{Stage: "DATA", Uncertain: true}
	service.Tick(ctx)
	before := len(mailer.messages)
	service.Tick(ctx)
	if len(mailer.messages) != before {
		t.Fatal("uncertain acceptance retried automatically")
	}
	var attempt string
	if err = db.QueryRow(`SELECT id FROM email_alert_attempts WHERE status='uncertain' LIMIT 1`).Scan(&attempt); err != nil {
		t.Fatal(err)
	}
	if err = store.Retry(ctx, attempt, user, false); err == nil {
		t.Fatal("uncertain retry missing confirmation")
	}
	if err = store.Retry(ctx, attempt, user, true); err != nil {
		t.Fatal(err)
	}
	mailer.err = nil
	service.Tick(ctx)
	if len(mailer.messages) != before+1 {
		t.Fatal("manual retry failed")
	}
	// Disabling delivery or removing recipients must cancel queued automatic work.
	if _, err = db.Exec(`UPDATE bills SET raw_data=jsonb_set(raw_data,'{shipping_check_summary,status}','"mismatch"')`); err != nil {
		t.Fatal(err)
	}
	if err = store.Scan(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	count(`SELECT count(*) FROM email_alert_outbox WHERE status='queued'`, 2)
	c.Enabled = false
	c, err = store.Save(ctx, c, user)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.QueueTest(ctx, "00000000-0000-0000-0000-000000000004", user); err != nil {
		t.Fatal(err)
	}
	if err = store.QueueTest(ctx, "00000000-0000-0000-0000-000000000004", user); err != nil {
		t.Fatal("test retry not idempotent", err)
	}
	if err = store.QueueTest(ctx, "00000000-0000-0000-0000-000000000005", user); err != ErrRateLimited {
		t.Fatalf("test rate limit: %v", err)
	}
	before = len(mailer.messages)
	service.Tick(ctx)
	if len(mailer.messages) != before+2 {
		t.Fatal("explicit tests should send while automation disabled")
	}
	count(`SELECT count(*) FROM email_alert_outbox WHERE status='cancelled'`, 2)
	// Daily summaries are once per recipient/day, with a fixed Bangkok hour.
	c.Enabled = true
	c.Mode = "daily"
	c.DailyHour = 9
	c, err = store.Save(ctx, c, user)
	if err != nil {
		t.Fatal(err)
	}
	day := time.Date(2026, 11, 19, 8, 59, 0, 0, bangkok)
	if err = store.Scan(ctx, day); err != nil {
		t.Fatal(err)
	}
	count(`SELECT count(*) FROM email_alert_outbox WHERE period_key LIKE 'daily:%'`, 0)
	for i := 0; i < 2; i++ {
		if err = store.Scan(ctx, day.Add(time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	count(`SELECT count(*) FROM email_alert_outbox WHERE period_key='daily:2026-11-19'`, 2)
	if err = store.Scan(ctx, day.Add(24*time.Hour+time.Minute)); err != nil {
		t.Fatal(err)
	}
	count(`SELECT count(*) FROM email_alert_outbox WHERE period_key='daily:2026-11-20'`, 2)
	if h, e := store.History(ctx, 1); e != nil || len(h.Data) == 0 {
		t.Fatal("history unavailable", e)
	}
	// Definite pre-DATA failures back off; the same delivery rows are then retried.
	c.Mode = "immediate"
	c, err = store.Save(ctx, c, user)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO bills VALUES('00000000-0000-0000-0000-000000000006','purchase','shopee_shipped','pending',NULL,'{"order_id":"RETRY","shipping_check_summary":{"status":"unverifiable"}}',NULL)`)
	if err != nil {
		t.Fatal(err)
	}
	mailer.err = &SendError{Stage: "connect"}
	before = len(mailer.messages)
	service.Tick(ctx)
	if len(mailer.messages) <= before {
		t.Fatal("retry fixture did not reach SMTP")
	}
	count(`SELECT count(*) FROM email_alert_outbox WHERE status='queued' AND period_key='event' AND next_attempt_at>now()`, 4)
	before = len(mailer.messages)
	service.Tick(ctx)
	if len(mailer.messages) != before {
		t.Fatal("backoff ignored")
	}
	if _, err = db.Exec(`UPDATE email_alert_outbox SET next_attempt_at=now()-interval '1 second' WHERE status='queued' AND period_key='event'`); err != nil {
		t.Fatal(err)
	}
	mailer.err = nil
	service.Tick(ctx)
	count(`SELECT count(*) FROM email_alert_outbox WHERE status='queued' AND period_key='event'`, 0)
	// Two processes may claim concurrently, but no row can belong to both sends.
	if _, err = db.Exec(`UPDATE bills SET raw_data=jsonb_set(raw_data,'{shipping_check_summary,status}','"mismatch"') WHERE id='00000000-0000-0000-0000-000000000006'`); err != nil {
		t.Fatal(err)
	}
	if err = store.Scan(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	concurrent, e := sql.Open("postgres", dsn+" search_path="+strings.Trim(schema, "\""))
	if e != nil {
		t.Fatal(e)
	}
	defer concurrent.Close()
	results := make(chan *batch, 2)
	failures := make(chan error, 2)
	for j := 0; j < 2; j++ {
		go func() { b, e := NewStore(concurrent, "synthetic-test-key").claim(ctx); results <- b; failures <- e }()
	}
	seen := map[string]bool{}
	claimed := 0
	for j := 0; j < 2; j++ {
		b := <-results
		if e := <-failures; e != nil {
			t.Fatal(e)
		}
		if b == nil {
			continue
		}
		for _, d := range b.Items {
			if seen[d.ID] {
				t.Fatal("concurrent duplicate claim")
			}
			seen[d.ID] = true
			claimed++
		}
	}
	if claimed == 0 {
		t.Fatal("nothing claimed")
	}
	// Crash recovery never automatically repeats a potentially accepted message.
	if _, err = db.Exec(`UPDATE email_alert_outbox SET started_at=now()-interval '3 minutes' WHERE status='sending';UPDATE email_alert_attempts SET created_at=now()-interval '3 minutes' WHERE status='sending'`); err != nil {
		t.Fatal(err)
	}
	if err = store.recoverStale(ctx); err != nil {
		t.Fatal(err)
	}
	count(`SELECT count(*) FROM email_alert_outbox WHERE status='sending'`, 0)

}
