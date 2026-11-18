package handlers

import (
	"errors"
	"testing"

	"billflow/internal/models"
	"billflow/internal/repository"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestShippingGateBlocksSingleAndBulkBeforeSML(t *testing.T) {
	for _, via := range []string{"retry", "bulk_job"} {
		t.Run(via, func(t *testing.T) {
			db, m, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			m.ExpectQuery("SELECT value FROM app_settings").WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(`{"require_manual_pass":true,"profiles":[],"policies":[]}`))
			m.ExpectQuery("SELECT DISTINCT ON").WillReturnRows(sqlmock.NewRows([]string{"bill_id", "policy", "result"}))
			h := &BillHandler{billRepo: repository.NewBillRepo(db)}
			// No SML client is wired: the central guard must exit before any external call.
			b := &models.Bill{ID: "00000000-0000-0000-0000-000000000001", BillType: "purchase", Source: "shopee_shipped", Status: "pending", RawData: []byte(`{"shipping_check_summary":{"status":"match","trigger":"manual"}}`)}
			r := h.sendBillToSML(b, RetryRequest{}, retrySendOptions{Via: via})
			if r.HTTPStatus != 409 || !r.Skipped {
				t.Fatalf("%+v", r)
			}
			if err = m.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestShippingGateDatabaseFailureClosesSend(t *testing.T) {
	db, m, _ := sqlmock.New()
	defer db.Close()
	m.ExpectQuery("SELECT value FROM app_settings").WillReturnError(errors.New("database unavailable"))
	h := &BillHandler{billRepo: repository.NewBillRepo(db)}
	b := &models.Bill{ID: "bill", BillType: "purchase", Source: "lazada_email", Status: "pending"}
	if r := h.guardShippingManualPass(b); r == nil || r.HTTPStatus != 503 {
		t.Fatalf("%+v", r)
	}
	h.attachShippingSendReadiness(nil)
	if err := m.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
func TestShippingGateLeavesOtherSourcesAlone(t *testing.T) {
	h := &BillHandler{}
	if r := h.guardShippingManualPass(&models.Bill{BillType: "sale", Source: "line"}); r != nil {
		t.Fatalf("%+v", r)
	}
}
