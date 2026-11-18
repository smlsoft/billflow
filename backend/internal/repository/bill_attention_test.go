package repository

import (
	"errors"
	"strings"
	"testing"

	"billflow/internal/models"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestAttentionFiltersUseUnsentWorkAndLatestResult(t *testing.T) {
	for _, f := range []models.BillListFilter{{ShippingCheck: "attention"}, {EmailCompleteness: "attention"}} {
		where, _, _ := billWhere(f)
		for _, want := range []string{"b.status IN ('pending', 'needs_review', 'failed')", "b.bill_type = 'purchase'", "b.source IN ('shopee_shipped', 'lazada_email')"} {
			if !strings.Contains(where, want) {
				t.Fatalf("missing %s: %s", want, where)
			}
		}
		if f.ShippingCheck != "" && (!strings.Contains(where, "->>'status' IN ('mismatch', 'unverifiable')") || strings.Contains(where, "->>'attention'")) {
			t.Fatalf("historical warning still counted: %s", where)
		}
	}
}
func TestAttentionCountsAreGlobalAndDatabaseErrorsPropagate(t *testing.T) {
	db, m, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	m.ExpectQuery("SELECT COUNT").WillReturnRows(sqlmock.NewRows([]string{"shipping", "email"}).AddRow(12, 3))
	counts, err := NewBillRepo(db).AttentionCounts()
	if err != nil || counts.Shipping != 12 || counts.Email != 3 {
		t.Fatalf("%+v %v", counts, err)
	}
	if err = m.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
	m.ExpectQuery("SELECT COUNT").WillReturnError(errors.New("unavailable"))
	if _, err = NewBillRepo(db).AttentionCounts(); err == nil {
		t.Fatal("database error must not become zero")
	}
	if err = m.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}

}
