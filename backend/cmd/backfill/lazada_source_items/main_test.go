package main

import (
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"billflow/internal/models"
	"billflow/internal/repository"
	"github.com/DATA-DOG/go-sqlmock"
)

func repairTestDB(t *testing.T, apply bool) (*sql.DB, sqlmock.Sqlmock, string) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	dir := t.TempDir()
	t.Setenv("ARTIFACTS_DIR", dir)
	body := `<a href="https://my.lazada.co.th/customer/order/view/?tradeOrderId=1124675871588966">คำสั่งซื้อ</a><table><tr><td><img src="https://th-live-01.slatic.net/p/white.jpg"></td><td><a href="https://www.lazada.co.th/products/i1-s11.html">นาฬิกา LED</a><div>THB 678.00</div><div>จำนวน: 2</div></td></tr></table><table><tr><td><img src="https://th-live-01.slatic.net/p/green.jpg"></td><td><a href="https://www.lazada.co.th/products/i1-s12.html">นาฬิกา LED</a><div>THB 678.00</div><div>จำนวน: 2</div></td></tr></table><div>ยอดรวม: THB 2,712.00</div><div>ค่าธรรมเนียมจัดส่ง: THB 134.00</div><div>คูปองส่วนลด: THB 288.00</div><div>ยอดรวมทั้งหมด(รวม VAT): THB 2,558.00</div>`
	if err := os.WriteFile(filepath.Join(dir, "email.html"), []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	mock.ExpectBegin()
	mock.ExpectExec("SET LOCAL lock_timeout").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT source, bill_type").WithArgs("bill-1").WillReturnRows(sqlmock.NewRows([]string{"source", "bill_type", "status", "order", "doc", "sent", "archived", "raw"}).AddRow("lazada_email", "purchase", "needs_review", "1124675871588966", "", false, false, `{"email_message_id":"msg-1","order_id":"1124675871588966"}`))
	if apply {
		mock.ExpectQuery("SELECT id FROM bill_items").WithArgs("bill-1").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("goods").AddRow("fee"))
	}
	mock.ExpectQuery("SELECT COALESCE\\(jsonb_agg").WithArgs("bill-1").WillReturnRows(sqlmock.NewRows([]string{"items"}).AddRow(`[{"id":"goods","raw_name":"นาฬิกา LED","qty":2,"price":1356,"mapped":false},{"id":"fee","source_sku":"__lazada_shipping_fee__","qty":1,"price":134,"mapped":true}]`))
	artifactRows := func() *sqlmock.Rows {
		return sqlmock.NewRows([]string{"id", "bill_id", "kind", "filename", "content_type", "size_bytes", "sha256", "storage_path", "source_meta", "created_at"}).AddRow("artifact-1", "bill-1", "email_html", "email.html", "text/html", len(body), fmt.Sprintf("%x", sha256.Sum256([]byte(body))), "email.html", `{"message_id":"msg-1"}`, time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC))
	}
	mock.ExpectQuery("FROM bill_artifacts").WithArgs("bill-1", "msg-1").WillReturnRows(artifactRows())
	mock.ExpectQuery("FROM bill_artifacts WHERE id").WithArgs("artifact-1").WillReturnRows(artifactRows())
	summary := repository.ExtractLazadaAmountSummary("", body)
	items, err := repository.ParseLazadaEmailItems(body, summary)
	if err != nil {
		t.Fatal(err)
	}
	repository.ApplyLazadaDiscountsToItems(items, summary.CouponDiscountAmount)
	hash := planHash("bill-1", "1124675871588966", "needs_review", []byte(`{"email_message_id":"msg-1","order_id":"1124675871588966"}`), []byte(`[{"id":"goods","raw_name":"นาฬิกา LED","qty":2,"price":1356,"mapped":false},{"id":"fee","source_sku":"__lazada_shipping_fee__","qty":1,"price":134,"mapped":true}]`), "artifact-1", []byte(body), items)
	return db, mock, hash
}

func TestDryRunReadsRealArtifactWithoutMutations(t *testing.T) {
	db, mock, _ := repairTestDB(t, false)
	mock.ExpectRollback()
	if err := repairBill(db, "bill-1", "1124675871588966", "", false); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestApplyRefusesChangedPlanBeforeDeletingItems(t *testing.T) {
	db, mock, _ := repairTestDB(t, true)
	mock.ExpectRollback()
	err := repairBill(db, "bill-1", "1124675871588966", "stale-plan", true)
	if err == nil || !strings.Contains(err.Error(), "plan changed") {
		t.Fatalf("err=%v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestApplyWritesSeparateVariantsAndAuditInOneTransaction(t *testing.T) {
	for _, failAudit := range []bool{false, true} {
		t.Run(fmt.Sprintf("audit_failure_%v", failAudit), func(t *testing.T) {
			db, mock, hash := repairTestDB(t, true)
			mock.ExpectExec("DELETE FROM bill_items").WithArgs("bill-1", models.LazadaFeeSourceSKU).WillReturnResult(sqlmock.NewResult(0, 1))
			for i, sku := range []string{"11", "12"} {
				mock.ExpectExec("INSERT INTO bill_items").WithArgs("bill-1", "นาฬิกา LED", sku, sqlmock.AnyArg(), i+1, 2.0, 678.0, 144.0).WillReturnResult(sqlmock.NewResult(0, 1))
			}
			mock.ExpectExec("UPDATE bills SET").WithArgs(sqlmock.AnyArg(), "bill-1").WillReturnResult(sqlmock.NewResult(0, 1))
			audit := mock.ExpectExec("INSERT INTO audit_logs").WithArgs("bill-1", sqlmock.AnyArg())
			if failAudit {
				audit.WillReturnError(errors.New("audit unavailable"))
				mock.ExpectRollback()
			} else {
				audit.WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectCommit()
			}
			err := repairBill(db, "bill-1", "1124675871588966", hash, true)
			if (err != nil) != failAudit {
				t.Fatalf("err=%v", err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestApplyRollsBackWhenSourceSnapshotReadFails(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectExec("SET LOCAL lock_timeout").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT source, bill_type").WithArgs("bill-1").WillReturnError(errors.New("source unavailable"))
	mock.ExpectRollback()
	if err := repairBill(db, "bill-1", "1124675871588966", "plan", true); err == nil {
		t.Fatal("expected failure")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestEligibilityRejectsSentArchivedWrongOrderAndActiveSend(t *testing.T) {
	if err := eligible("lazada_email", "purchase", "needs_review", "123", "123", "", false, false); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		source, kind, status, order, doc string
		sent, archived                   bool
	}{
		{"lazada_email", "purchase", "sent", "123", "", true, false},
		{"lazada_email", "purchase", "needs_review", "123", "POL26100001", false, false},
		{"lazada_email", "purchase", "needs_review", "123", "", false, true},
		{"lazada_email", "purchase", "sending", "123", "", false, false},
		{"lazada_email", "purchase", "needs_review", "456", "", false, false},
		{"shopee_shipped", "purchase", "needs_review", "123", "", false, false},
		{"lazada_email", "sale", "needs_review", "123", "", false, false},
	} {
		if eligible(tc.source, tc.kind, tc.status, tc.order, "123", tc.doc, tc.sent, tc.archived) == nil {
			t.Fatalf("unsafe target accepted: %+v", tc)
		}
	}
}

func TestRepairPreservesReviewedGoodsAndAllowsMappedFee(t *testing.T) {
	code := "SHIP_POL"
	if err := unmappedGoods([]models.BillItem{{RawName: "clock"}, {SourceSKU: models.LazadaFeeSourceSKU, Mapped: true, ItemCode: &code}}); err != nil {
		t.Fatal(err)
	}
	for _, item := range []models.BillItem{{Mapped: true}, {ItemCode: &code}, {MappingID: &code}} {
		if unmappedGoods([]models.BillItem{item}) == nil {
			t.Fatalf("reviewed goods allowed: %+v", item)
		}
	}
}
