package shippingcheck

import (
	"database/sql"
	"encoding/json"

	"billflow/internal/repository"
	"fmt"
	"github.com/lib/pq"
	"os"
	"strings"
	"testing"
	"time"
)

// Run only against an explicitly supplied disposable database; no deploy secrets.
func TestShippingLedgerIntegration(t *testing.T) {
	dsn := os.Getenv("BILLFLOW_SHIPPING_TEST_DSN")
	if dsn == "" {
		t.Skip("set BILLFLOW_SHIPPING_TEST_DSN to a disposable database")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	schema := fmt.Sprintf("shipping_test_%d", time.Now().UnixNano())
	if _, err = db.Exec("CREATE SCHEMA " + pq.QuoteIdentifier(schema)); err != nil {
		t.Fatal(err)
	}
	defer db.Exec("DROP SCHEMA " + pq.QuoteIdentifier(schema) + " CASCADE")
	if _, err = db.Exec("SET search_path TO " + pq.QuoteIdentifier(schema)); err != nil {
		t.Fatal(err)
	}

	_, err = db.Exec(`CREATE TABLE bills(id uuid PRIMARY KEY,bill_type text,source text,raw_data jsonb,status text,created_at timestamptz DEFAULT now());CREATE TABLE bill_artifacts(id uuid PRIMARY KEY,bill_id uuid REFERENCES bills(id),kind text,source_meta jsonb,created_at timestamptz DEFAULT now());CREATE TABLE app_settings(key text PRIMARY KEY,value text,is_secret boolean,updated_by uuid,updated_at timestamptz);CREATE TABLE imap_accounts(id uuid PRIMARY KEY,channel text);CREATE TABLE audit_logs(action text,target_id uuid,user_id uuid,source text,detail jsonb);`)
	if err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile("../../database/migrations/074_shipping_checks.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(string(migration)); err != nil {
		t.Fatal(err)
	}
	s := NewStore(db)
	account := "00000000-0000-0000-0000-000000000001"
	bill := "00000000-0000-0000-0000-000000000002"
	user := "00000000-0000-0000-0000-000000000003"
	_, err = db.Exec(`INSERT INTO imap_accounts(id,channel) VALUES($1,'shopee')`, account)
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.Save(Settings{Config: Config{Profiles: []Profile{testProfile()}, Policies: []Policy{{AccountID: account, ProfileID: "office", Recipients: []string{"PD01"}, Enabled: true}}}}, user)
	if err != nil {
		t.Fatal(err)
	}
	raw := map[string]any{}
	if err = s.Enroll(raw, account); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(raw)
	_, err = db.Exec(`INSERT INTO bills(id,bill_type,source,raw_data,status) VALUES($1,'purchase','shopee_shipped',$2,'pending')`, bill, string(b))
	if err != nil {
		t.Fatal(err)
	}
	good := `<h3>รายละเอียดการส่งสินค้า</h3><p>ชื่อผู้รับ: PD01</p><p>ที่อยู่: 368 หมู่10 ตำบลคลองมะเดื่อ อำเภอกระทุ่มแบน จังหวัดสมุทรสาคร 74110</p><p>เบอร์โทร: 000</p>`
	for _, e := range []struct{ msg, body string }{{"1", good}, {"2", strings.Replace(good, "368", "369", 1)}, {"2", good}, {"3", "<p>ส่งสินค้าแล้ว</p>"}, {"4", good}} {
		if err = s.Record(bill, e.msg, e.body, ""); err != nil {
			t.Fatal(err)
		}
	}
	var status, latest string
	var attention bool
	if err = db.QueryRow(`SELECT status,raw_data->'shipping_check_summary'->>'status',(raw_data->'shipping_check_summary'->>'attention')::boolean FROM bills WHERE id=$1`, bill).Scan(&status, &latest, &attention); err != nil {
		t.Fatal(err)
	}
	if status != "pending" || latest != "match" || !attention {
		t.Fatalf("status=%s latest=%s attention=%v", status, latest, attention)
	}
	entries, err := s.History(bill)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("history=%d", len(entries))
	}
	c.Profiles[0].House = "777"
	if _, err = s.Save(c, user); err != nil {
		t.Fatal(err)
	}
	entries, err = s.History(bill)
	if err != nil {
		t.Fatal(err)
	}
	if entries[0].Policy.Profile.House != "368" {
		t.Fatal("history changed with config")
	}

	// Reproduce a payment follow-up that read raw_data before a concurrent checker
	// committed its warning. The existing amount-update path must retain it.
	if _, err = db.Exec(`UPDATE bills SET raw_data=jsonb_set(raw_data,'{shipping_check_summary}','{"status":"match","attention":false}') WHERE id=$1`, bill); err != nil {
		t.Fatal(err)
	}
	worker, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer worker.Close()
	worker.SetMaxOpenConns(1)
	if _, err = worker.Exec("SET search_path TO " + pq.QuoteIdentifier(schema)); err != nil {
		t.Fatal(err)
	}
	locked, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer locked.Rollback()
	if _, err = locked.Exec(`SELECT id FROM bills WHERE id=$1 FOR UPDATE`, bill); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, e := repository.NewBillRepo(worker).ApplyShopeePurchasePaymentSummaryToBill(bill, repository.ShopeePaymentSummary{PaymentMethod: "card"})
		done <- e
	}()
	waiting := false
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if err = locked.QueryRow(`SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE wait_event_type='Lock' AND query LIKE '%SET raw_data = $1::jsonb%')`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !waiting {
		t.Fatal("payment update did not reach the expected row lock")
	}
	if _, err = locked.Exec(`UPDATE bills SET raw_data=jsonb_set(raw_data,'{shipping_check_summary}','{"status":"mismatch","attention":true,"has_warning":true}') WHERE id=$1`, bill); err != nil {
		t.Fatal(err)
	}
	if err = locked.Commit(); err != nil {
		t.Fatal(err)
	}
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT (raw_data->'shipping_check_summary'->>'attention')::boolean FROM bills WHERE id=$1`, bill).Scan(&attention); err != nil || !attention {
		t.Fatalf("concurrent warning lost: %v", err)
	}
	// An un-enrolled bill remains untouched even after a new email arrives.
	old := "00000000-0000-0000-0000-000000000004"
	if _, err = db.Exec(`INSERT INTO bills(id,bill_type,source,raw_data,status) VALUES($1,'purchase','shopee_shipped','{}','sent')`, old); err != nil {
		t.Fatal(err)
	}
	if err = s.Record(old, "new-followup", good, ""); err != nil {
		t.Fatal(err)
	}
	entries, err = s.History(old)
	if err != nil || len(entries) != 0 {
		t.Fatalf("historical bill changed: %v", err)
	}

	current, err := s.CurrentPolicy(account)
	if err != nil || current == nil {
		t.Fatalf("current policy: %v", err)
	}
	manualResult := Check(good, current.Profile, current.Recipients)
	manualResult.CheckedBy = "admin@example.invalid"
	for attempt := 0; attempt < 2; attempt++ {
		if err = s.RecordManual(old, "request-1", "original", "", *current, manualResult, user); err != nil {
			t.Fatal(err)
		}
	}
	var enrolled bool
	var auditCount int
	if err = db.QueryRow(`SELECT status,raw_data ? 'shipping_check_policy' FROM bills WHERE id=$1`, old).Scan(&status, &enrolled); err != nil {
		t.Fatal(err)
	}
	if status != "sent" || enrolled {
		t.Fatalf("manual check changed SML status or auto-enrolment: status=%s enrolled=%v", status, enrolled)
	}
	if err = db.QueryRow(`SELECT count(*) FROM audit_logs WHERE action='shipping_check_manual' AND target_id=$1`, old).Scan(&auditCount); err != nil || auditCount != 1 {
		t.Fatalf("manual audit: count=%d err=%v", auditCount, err)
	}
	if err = s.Record(old, "later-followup", good, ""); err != nil {
		t.Fatal(err)
	}
	entries, err = s.History(old)
	if err != nil || len(entries) != 1 {
		t.Fatalf("manual check enrolled future emails: count=%d err=%v", len(entries), err)
	}
	if entries[0].Result.Trigger != "manual" || entries[0].Result.CheckedBy != "admin@example.invalid" || entries[0].Policy.Profile.House != "777" {
		t.Fatalf("wrong manual evidence: %+v", entries[0])
	}
}
