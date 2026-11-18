package shippingcheck

import (
	"database/sql"
	"encoding/json"
	"github.com/DATA-DOG/go-sqlmock"
	"testing"
)

func TestUnenrolledBillIsNotBackfilled(t *testing.T) {
	db, m, _ := sqlmock.New()
	defer db.Close()
	m.ExpectQuery("SELECT raw_data").WithArgs("old").WillReturnRows(sqlmock.NewRows([]string{"raw_data", "source"}).AddRow(`{"order_id":"old"}`, "lazada_email"))
	if err := NewStore(db).Record("old", "message", "<p>new email</p>", ""); err != nil {
		t.Fatal(err)
	}
	if err := m.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
func TestNoShippingFollowupDoesNotClearHistory(t *testing.T) {
	db, m, _ := sqlmock.New()
	defer db.Close()
	raw, _ := json.Marshal(map[string]any{"shipping_check_policy": Enrollment{Profile: testProfile(), Recipients: []string{"PD01"}}, "shipping_check_summary": map[string]any{"status": "mismatch"}})
	m.ExpectQuery("SELECT raw_data").WithArgs("bill").WillReturnRows(sqlmock.NewRows([]string{"raw_data", "source"}).AddRow(raw, "lazada_email"))
	m.ExpectQuery("SELECT EXISTS").WithArgs("bill").WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	if err := NewStore(db).Record("bill", "followup", "<p>พัสดุจัดส่งแล้ว</p>", ""); err != nil {
		t.Fatal(err)
	}
	if err := m.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
func TestEmptyConfigIsDisabled(t *testing.T) {
	db, m, _ := sqlmock.New()
	defer db.Close()
	m.ExpectQuery("SELECT value").WillReturnError(sql.ErrNoRows)
	raw := map[string]any{}
	if err := NewStore(db).Enroll(raw, "mail"); err != nil {
		t.Fatal(err)
	}
	if len(raw) != 0 {
		t.Fatal(raw)
	}
}

func TestDuplicateMessageDoesNotRewriteSummary(t *testing.T) {
	db, m, _ := sqlmock.New()
	defer db.Close()
	raw, _ := json.Marshal(map[string]any{"shipping_check_policy": Enrollment{Profile: testProfile(), Recipients: []string{"PD01"}}})
	m.ExpectQuery("SELECT raw_data").WithArgs("bill").WillReturnRows(sqlmock.NewRows([]string{"raw_data", "source"}).AddRow(raw, "lazada_email"))
	m.ExpectQuery("SELECT EXISTS").WithArgs("bill").WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	m.ExpectBegin()
	m.ExpectExec("SELECT id FROM bills").WithArgs("bill").WillReturnResult(sqlmock.NewResult(0, 1))
	m.ExpectExec("INSERT INTO bill_shipping_checks").WillReturnResult(sqlmock.NewResult(0, 0))
	m.ExpectCommit()
	if err := NewStore(db).Record("bill", "same-message", "<p>unsupported</p>", ""); err != nil {
		t.Fatal(err)
	}
	if err := m.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
func TestSaveRejectsStaleRevision(t *testing.T) {
	db, m, _ := sqlmock.New()
	defer db.Close()
	m.ExpectBegin()
	m.ExpectExec("INSERT INTO app_settings").WillReturnResult(sqlmock.NewResult(0, 0))
	m.ExpectQuery("SELECT value").WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(`{"revision":"new"}`))
	m.ExpectRollback()
	_, err := NewStore(db).Save(Settings{Revision: "old"}, "user")
	if err != ErrConflict {
		t.Fatalf("got %v", err)
	}
	if err = m.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
func TestPersistResultOnlyUpdatesShippingSummary(t *testing.T) {
	db, m, _ := sqlmock.New()
	defer db.Close()
	raw, _ := json.Marshal(map[string]any{"shipping_check_policy": Enrollment{Profile: testProfile(), Recipients: []string{"PD01"}}})
	m.ExpectQuery("SELECT raw_data").WillReturnRows(sqlmock.NewRows([]string{"raw_data", "source"}).AddRow(raw, "shopee_shipped"))
	m.ExpectQuery("SELECT EXISTS").WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	m.ExpectBegin()
	m.ExpectExec("SELECT id FROM bills").WillReturnResult(sqlmock.NewResult(0, 1))
	m.ExpectExec("INSERT INTO bill_shipping_checks").WillReturnResult(sqlmock.NewResult(0, 1))
	m.ExpectExec(`UPDATE bills SET raw_data=jsonb_set`).WithArgs("bill", "unverifiable", sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	m.ExpectCommit()
	if err := NewStore(db).Record("bill", "new-message", "<p>unsupported</p>", ""); err != nil {
		t.Fatal(err)
	}
	if err := m.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestManualShippingCheckDoesNotEnrolHistoricalBill(t *testing.T) {
	db, m, _ := sqlmock.New()
	defer db.Close()
	m.ExpectBegin()
	m.ExpectExec("SELECT id FROM bills").WithArgs("old").WillReturnResult(sqlmock.NewResult(0, 1))
	m.ExpectExec("INSERT INTO bill_shipping_checks").WithArgs("old", "manual:request-1", "original", nil, sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	m.ExpectExec("UPDATE bills SET raw_data=jsonb_set").WithArgs("old", "mismatch", "").WillReturnResult(sqlmock.NewResult(0, 1))
	m.ExpectExec("INSERT INTO audit_logs").WillReturnResult(sqlmock.NewResult(0, 1))
	m.ExpectCommit()
	r := Result{Status: "mismatch", Trigger: "manual", CheckedBy: "admin@example.invalid"}
	if err := NewStore(db).RecordManual("old", "request-1", "original", "", Enrollment{Profile: testProfile(), Recipients: []string{"PD01"}}, r, "user"); err != nil {
		t.Fatal(err)
	}
	if err := m.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
