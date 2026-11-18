package repository

import (
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"billflow/internal/models"
	"github.com/lib/pq"
)

func TestBillAttentionIntegration(t *testing.T) {
	dsn := os.Getenv("BILLFLOW_SHIPPING_TEST_DSN")
	if dsn == "" {
		t.Skip("set disposable database DSN")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	schema := pq.QuoteIdentifier(fmt.Sprintf("attention_test_%d", time.Now().UnixNano()))
	if _, err = db.Exec("CREATE SCHEMA " + schema); err != nil {
		t.Fatal(err)
	}
	defer db.Exec("DROP SCHEMA " + schema + " CASCADE")
	if _, err = db.Exec("SET search_path TO " + schema); err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE bills(id int,status text,bill_type text,source text,raw_data jsonb,archived_at timestamptz);
 CREATE TABLE marketplace_email_groups(source text,message_id text,status text);
 INSERT INTO bills VALUES
 (1,'pending','purchase','shopee_shipped','{"email_message_id":"group","shipping_check_summary":{"status":"mismatch","attention":true}}',NULL),
 (2,'failed','purchase','shopee_shipped','{"message_id":"group","shipping_check_summary":{"status":"unverifiable"}}',NULL),
 (3,'pending','purchase','lazada_email','{"shipping_check_summary":{"status":"match","attention":true}}',NULL),
 (4,'sent','purchase','shopee_shipped','{"email_message_id":"group","shipping_check_summary":{"status":"mismatch"}}',NULL),
 (5,'pending','purchase','shopee_shipped','{"email_message_id":"group","shipping_check_summary":{"status":"mismatch"}}',now()),
 (6,'pending','sale','shopee_shipped','{"email_message_id":"group","shipping_check_summary":{"status":"mismatch"}}',NULL),
 (7,'pending','purchase','lazada_email','{}',NULL),
 (8,'skipped','purchase','shopee_shipped','{"email_message_id":"group","shipping_check_summary":{"status":"mismatch"}}',NULL);
 INSERT INTO marketplace_email_groups VALUES ('shopee_shipped','group','attention'),('shopee_shipped','group','processing'),('lazada_email','orphan','attention');`)
	if err != nil {
		t.Fatal(err)
	}
	repo := NewBillRepo(db)
	counts, err := repo.AttentionCounts()
	if err != nil || counts.Shipping != 2 || counts.Email != 2 {
		t.Fatalf("%+v %v", counts, err)
	}
	for _, filter := range []models.BillListFilter{{ShippingCheck: "attention"}, {EmailCompleteness: "attention"}} {
		where, args, _ := billWhere(filter)
		var count int
		if err = db.QueryRow("SELECT count(*) FROM bills b "+where, args...).Scan(&count); err != nil || count != 2 {
			t.Fatalf("filter mismatch: %d %v", count, err)
		}
	}
	if _, err = db.Exec(`UPDATE bills SET raw_data=jsonb_set(raw_data,'{shipping_check_summary,status}','"match"') WHERE id=1;UPDATE marketplace_email_groups SET status='complete';`); err != nil {
		t.Fatal(err)
	}
	counts, err = repo.AttentionCounts()
	if err != nil || counts.Shipping != 1 || counts.Email != 0 {
		t.Fatalf("resolved work counted: %+v %v", counts, err)
	}
}
