package repository

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestGetLazadaChargeGroupByEmailDateAggregatesActiveCardOrders(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	repo := NewBillRepo(db)
	emailDate := "2026-06-10T17:56:30+08:00"
	mock.ExpectQuery("FROM bills b").
		WithArgs(emailDate).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "order_id", "payment_method", "paid_total_amount", "amount_reconciliation_status",
		}).
			AddRow("bill-1", "1100887409895692", "Credit or Debit Card", "162.36", "ok").
			AddRow("bill-2", "1100887410295692", "Credit or Debit Card", "187", "ok").
			AddRow("bill-3", "1100887410495692", "Credit or Debit Card", "1,021.45", "ok").
			AddRow("bill-4", "1100887410695692", "Credit or Debit Card", "845.33", "ok"))

	group, err := repo.GetLazadaChargeGroupByEmailDate(emailDate)
	if err != nil {
		t.Fatalf("GetLazadaChargeGroupByEmailDate: %v", err)
	}
	if group.GroupCount != 4 || group.GroupTotal != 2216.14 {
		t.Fatalf("group count/total = %d/%v, want 4/2216.14", group.GroupCount, group.GroupTotal)
	}
	if len(group.OrderIDs) != 4 || group.OrderIDs[0] != "1100887409895692" || group.OrderIDs[3] != "1100887410695692" {
		t.Fatalf("order ids = %#v", group.OrderIDs)
	}
	if len(group.MissingPaidTotalOrderIDs) != 0 || len(group.NonCardPaymentOrderIDs) != 0 || len(group.NotReconciledOrderIDs) != 0 {
		t.Fatalf("unexpected invalid lists: %#v", group)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func TestGetLazadaChargeGroupByEmailDateCapturesInvalidOrders(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	repo := NewBillRepo(db)
	emailDate := "2026-06-09T17:46:43+08:00"
	mock.ExpectQuery("FROM bills b").
		WithArgs(emailDate).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "order_id", "payment_method", "paid_total_amount", "amount_reconciliation_status",
		}).
			AddRow("bill-1", "1108968794895692", "Credit or Debit Card", "", "ok").
			AddRow("bill-2", "1108968795095692", "Cash on Delivery", "471.27", "ok").
			AddRow("bill-3", "1108968795495692", "Credit or Debit Card", "2325.26", "mismatch"))

	group, err := repo.GetLazadaChargeGroupByEmailDate(emailDate)
	if err != nil {
		t.Fatalf("GetLazadaChargeGroupByEmailDate: %v", err)
	}
	if group.GroupCount != 3 || group.GroupTotal != 2796.53 {
		t.Fatalf("group count/total = %d/%v, want 3/2796.53", group.GroupCount, group.GroupTotal)
	}
	if got := group.MissingPaidTotalOrderIDs; len(got) != 1 || got[0] != "1108968794895692" {
		t.Fatalf("missing paid = %#v", got)
	}
	if got := group.NonCardPaymentOrderIDs; len(got) != 1 || got[0] != "1108968795095692" {
		t.Fatalf("non-card = %#v", got)
	}
	if got := group.NotReconciledOrderIDs; len(got) != 1 || got[0] != "1108968795495692" {
		t.Fatalf("not reconciled = %#v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func TestIsCreditDebitCardPaymentMethod(t *testing.T) {
	for _, method := range []string{"Credit/Debit Card", "Credit or Debit Card", "บัตรเครดิต/บัตรเดบิต"} {
		if !IsCreditDebitCardPaymentMethod(method) {
			t.Fatalf("method %q should be treated as card", method)
		}
	}
	if IsCreditDebitCardPaymentMethod("Cash on Delivery") {
		t.Fatal("COD should not be treated as card")
	}
}
