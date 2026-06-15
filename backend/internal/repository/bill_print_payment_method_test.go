package repository

import (
	"encoding/json"
	"testing"

	"billflow/internal/models"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
)

func TestUpdatePrintPaymentMethodAllowsClear(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	repo := NewBillRepo(db)
	billID := "11111111-1111-1111-1111-111111111111"

	mock.ExpectBegin()
	expectPrintPaymentHeader(mock, billID, "lazada_email", "purchase", "sent", "", false, "msg-1")
	mock.ExpectQuery("SELECT COALESCE\\(print_policy").
		WithArgs("lazada_email", "purchase").
		WillReturnRows(sqlmock.NewRows([]string{"print_policy"}).AddRow([]byte("{}")))
	expectPrintPaymentTargets(mock).
		WithArgs(billID).
		WillReturnRows(sqlmock.NewRows(printPaymentTargetColumns()).
			AddRow(billID, "1101", "lazada_email", "purchase", "sent", "", "TT2789", "TT2789"))
	mock.ExpectExec("UPDATE bills SET print_payment_method").
		WithArgs("", pq.Array([]string{billID})).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	result, err := repo.UpdatePrintPaymentMethod(billID, "", false)
	if err != nil {
		t.Fatalf("UpdatePrintPaymentMethod: %v", err)
	}
	if result.PaymentMethod != "" || result.UpdatedCount != 1 {
		t.Fatalf("result = %#v", result)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func TestUpdatePrintPaymentMethodAllowsDynamicTTPrefix(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	repo := NewBillRepo(db)
	billID := "11111111-1111-1111-1111-111111111111"
	policyJSON, _ := json.Marshal(models.MarketplacePrintPolicy{
		RequiresAllOrdersSMLDoc:    true,
		PaymentMethodPrefixEnabled: true,
		PaymentMethodPrefixes:      []string{"TT"},
		PaymentMethods:             []string{"TT2789"},
	})

	mock.ExpectBegin()
	expectPrintPaymentHeader(mock, billID, "lazada_email", "purchase", "sent", "", false, "msg-1")
	mock.ExpectQuery("SELECT COALESCE\\(print_policy").
		WithArgs("lazada_email", "purchase").
		WillReturnRows(sqlmock.NewRows([]string{"print_policy"}).AddRow(policyJSON))
	expectPrintPaymentTargets(mock).
		WithArgs(billID).
		WillReturnRows(sqlmock.NewRows(printPaymentTargetColumns()).
			AddRow(billID, "1101", "lazada_email", "purchase", "sent", "", "", ""))
	mock.ExpectExec("UPDATE bills SET print_payment_method").
		WithArgs("TT9999", pq.Array([]string{billID})).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	result, err := repo.UpdatePrintPaymentMethod(billID, "TT9999", false)
	if err != nil {
		t.Fatalf("UpdatePrintPaymentMethod: %v", err)
	}
	if result.PaymentMethod != "TT9999" || result.UpdatedCount != 1 {
		t.Fatalf("result = %#v", result)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func TestRowMatchesPaymentPolicyAllowsDynamicTTPrefix(t *testing.T) {
	row := marketplacePrintRow{
		EffectivePrintPaymentMethod: "TT9999",
		Status:                      "sent",
		SMLDocNo:                    "POL26060001",
	}
	policy := models.MarketplacePrintPolicy{
		RequiresAllOrdersSMLDoc:    true,
		PaymentMethodPrefixEnabled: true,
		PaymentMethodPrefixes:      []string{"TT"},
		PaymentMethods:             []string{"TT2789"},
	}
	if !rowMatchesPaymentPolicy(row, policy) {
		t.Fatal("dynamic TT prefix should be print-ready even when not listed")
	}
}

func TestPatchSMLPayloadDocRefUpdatesHeaderItemsAndDetails(t *testing.T) {
	got, err := patchSMLPayloadDocRef([]byte(`{
		"doc_ref":"old",
		"items":[{"item_code":"A","doc_ref":"old"},{"item_code":"B"}],
		"details":[{"item_code":"C","doc_ref":"old"}]
	}`), "7417.69")
	if err != nil {
		t.Fatalf("patchSMLPayloadDocRef: %v", err)
	}
	var payload struct {
		DocRef  string                   `json:"doc_ref"`
		Items   []map[string]interface{} `json:"items"`
		Details []map[string]interface{} `json:"details"`
	}
	if err := json.Unmarshal(got, &payload); err != nil {
		t.Fatalf("unmarshal patched payload: %v", err)
	}
	if payload.DocRef != "7417.69" {
		t.Fatalf("header doc_ref = %q", payload.DocRef)
	}
	for i, row := range append(payload.Items, payload.Details...) {
		if row["doc_ref"] != "7417.69" {
			t.Fatalf("row %d doc_ref = %#v", i, row["doc_ref"])
		}
	}
}

func expectPrintPaymentHeader(mock sqlmock.Sqlmock, billID, source, billType, status, smlDocNo string, archived bool, messageID string) {
	mock.ExpectQuery("SELECT b.source,").
		WithArgs(billID).
		WillReturnRows(sqlmock.NewRows([]string{
			"source", "bill_type", "status", "sml_doc_no", "archived", "message_id",
		}).AddRow(source, billType, status, smlDocNo, archived, messageID))
}

func expectPrintPaymentTargets(mock sqlmock.Sqlmock) *sqlmock.ExpectedQuery {
	return mock.ExpectQuery("SELECT b.id::text,")
}

func printPaymentTargetColumns() []string {
	return []string{
		"id", "order_id", "source", "bill_type", "status", "sml_doc_no",
		"print_payment_method", "effective_print_payment_method",
	}
}
