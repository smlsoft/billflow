package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"billflow/internal/repository"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func TestUpdateItemRejectsMarketplaceQuantityChange(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	billID := "294d5d92-eab8-41df-a9f0-2c32c3a55d82"
	itemID := "01HZ0000000000000000000001"
	now := time.Now()
	mock.ExpectQuery(`(?s)FROM bills.*WHERE id = \$1`).
		WithArgs(billID).
		WillReturnRows(billRows().AddRow(
			billID, "purchase", "lazada_email", "needs_review", "purchaseorder",
			[]byte(`{"order_id":"1110438913895692"}`),
			nil, nil, nil, nil, []byte("[]"), nil, nil, now, nil, nil, nil, "", "", "", "",
		))
	mock.ExpectQuery(`(?s)FROM bill_items.*WHERE bill_id = \$1`).
		WithArgs(billID).
		WillReturnRows(billItemRows().AddRow(
			itemID, billID, "โต๊ะ", "", "", nil, 1.0, nil, 690.0, 0.0, false, nil, []byte("[]"),
		))
	mock.ExpectQuery("FROM shopee_order_events").
		WithArgs(billID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "bill_id", "order_id", "event_type", "status_label", "subject",
			"from_addr", "message_id", "email_date", "raw_data", "created_at",
		}))

	h := &BillHandler{billRepo: repository.NewBillRepo(db), log: zap.NewNop()}
	router := gin.New()
	router.PUT("/api/bills/:id/items/:item_id", h.UpdateItem)

	req := httptest.NewRequest(
		http.MethodPut,
		"/api/bills/"+billID+"/items/"+itemID,
		strings.NewReader(`{"qty":2,"price":690}`),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s, expectations = %v", rec.Code, rec.Body.String(), mock.ExpectationsWereMet())
	}
	if !strings.Contains(rec.Body.String(), "ต้องใช้จำนวนจากอีเมล") {
		t.Fatalf("response does not explain locked quantity: %s", rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func billRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "bill_type", "source", "status", "document_route", "raw_data", "sml_doc_no",
		"sml_payload", "sml_response", "ai_confidence", "anomalies",
		"error_msg", "created_by", "created_at", "sent_at", "archived_at", "archived_by",
		"archive_reason", "remark", "print_payment_method", "effective_print_payment_method",
	})
}

func billItemRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "bill_id", "raw_name", "source_sku", "source_image_url", "item_code", "qty", "unit_code", "price",
		"discount_amount", "mapped", "mapping_id", "candidates",
	})
}
