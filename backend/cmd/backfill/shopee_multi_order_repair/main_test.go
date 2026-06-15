package main

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestInspectRepairTargetReportsExistingMissingAndTotal(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	body := `หมายเลขคำสั่งซื้อ: #26061316DWD4GG
ยอดที่ต้องชำระทั้งหมด: ฿183
หมายเลขคำสั่งซื้อ: #26061316DWD4GH
ยอดที่ต้องชำระทั้งหมด: ฿183
หมายเลขคำสั่งซื้อ: #26061316DWD4GJ
ยอดที่ต้องชำระทั้งหมด: ฿416`

	mock.ExpectQuery("FROM bills").
		WillReturnRows(sqlmock.NewRows([]string{"id", "order_id"}).
			AddRow("bill-existing", "26061316DWD4GG"))

	report, err := inspectRepairTarget(db, body, "")
	if err != nil {
		t.Fatalf("inspectRepairTarget: %v", err)
	}
	if len(report.OrderIDs) != 3 {
		t.Fatalf("detected orders = %#v", report.OrderIDs)
	}
	if report.Existing["26061316DWD4GG"] != "bill-existing" {
		t.Fatalf("existing = %#v", report.Existing)
	}
	if len(report.Missing) != 2 || report.Missing[0] != "26061316DWD4GH" || report.Missing[1] != "26061316DWD4GJ" {
		t.Fatalf("missing = %#v", report.Missing)
	}
	if report.Total != 782 {
		t.Fatalf("total = %.2f, want 782", report.Total)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func TestInspectRepairTargetFailsWhenOrderTotalMissing(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	body := `หมายเลขคำสั่งซื้อ: #26061316DWD4GG
ยอดที่ต้องชำระทั้งหมด: ฿183
หมายเลขคำสั่งซื้อ: #26061316DWD4GH
ผู้ขาย: test`

	mock.ExpectQuery("FROM bills").
		WillReturnRows(sqlmock.NewRows([]string{"id", "order_id"}))

	if _, err := inspectRepairTarget(db, body, ""); err == nil {
		t.Fatal("expected missing paid total error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}
