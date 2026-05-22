package handlers

import (
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"go.uber.org/zap"

	"billflow/internal/repository"
	"billflow/internal/services/ai"
	emailservice "billflow/internal/services/email"
)

func TestFindExistingShopeeShippedBillIDNormalizesHashPrefix(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	h := &EmailHandler{billRepo: repository.NewBillRepo(db), logger: zap.NewNop()}
	mock.ExpectQuery("FROM bills").
		WithArgs("2604294EP99PKT").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).
			AddRow("92b142e9-19bc-432b-8d8e-67d4e984e3ef"))

	got, exists, err := h.findExistingShopeeShippedBillID("#2604294EP99PKT")
	if err != nil {
		t.Fatalf("findExistingShopeeShippedBillID: %v", err)
	}
	if !exists || got != "92b142e9-19bc-432b-8d8e-67d4e984e3ef" {
		t.Fatalf("got id=%q exists=%v", got, exists)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func TestFindExistingShopeeShippedBillIDMissingReturnsFalse(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	h := &EmailHandler{billRepo: repository.NewBillRepo(db), logger: zap.NewNop()}
	mock.ExpectQuery("FROM bills").
		WithArgs("2604294EP99PKT").
		WillReturnError(sql.ErrNoRows)

	got, exists, err := h.findExistingShopeeShippedBillID("2604294EP99PKT")
	if err != nil {
		t.Fatalf("findExistingShopeeShippedBillID: %v", err)
	}
	if exists || got != "" {
		t.Fatalf("got id=%q exists=%v, want missing", got, exists)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func TestProcessOneShippedOrderRecordsEventOnExistingBill(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	h := &EmailHandler{billRepo: repository.NewBillRepo(db), logger: zap.NewNop()}
	messageID := "shipped-message@example.test"
	orderID := "#2604294EP99PKT"
	existingBillID := "768a0068-cad3-4b6e-b229-a5d2ce2ede73"

	mock.ExpectQuery("SELECT").
		WithArgs(messageID, orderID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("FROM bills").
		WithArgs("2604294EP99PKT").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(existingBillID))
	mock.ExpectExec("INSERT INTO shopee_order_events").
		WithArgs(existingBillID, "2604294EP99PKT", shopeeEventShipped, "ถูกจัดส่งแล้ว", "คำสั่งซื้อ #2604294EP99PKT ถูกจัดส่งแล้ว", "info@mail.shopee.co.th", messageID, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO processed_email_keys").
		WithArgs("shopee_shipped", messageID, orderID).
		WillReturnResult(sqlmock.NewResult(1, 1))

	created, err := h.processOneShippedOrder(
		aiExtractedOrderForTest(orderID),
		"คำสั่งซื้อ #2604294EP99PKT ถูกจัดส่งแล้ว",
		"info@mail.shopee.co.th",
		"body",
		"<html></html>",
		messageID,
		nil,
		nil,
		"trace-1",
		time.Now(),
		mailSourceForTest(),
	)
	if err != nil {
		t.Fatalf("processOneShippedOrder: %v", err)
	}
	if created {
		t.Fatal("expected existing shipped event to skip creating a new bill")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func aiExtractedOrderForTest(orderID string) ai.ExtractedOrder {
	price := 131.0
	return ai.ExtractedOrder{
		OrderID:    orderID,
		Confidence: 0.9,
		Items: []ai.ExtractedItem{{
			RawName: "SPIN MOP",
			Qty:     1,
			Price:   &price,
		}},
	}
}

func mailSourceForTest() emailservice.MailSource {
	return emailservice.MailSource{
		AccountID: "imap-account-id",
		Username:  "pd.thaisunsport2@gmail.com",
		EmailDate: time.Date(2026, 5, 3, 12, 10, 3, 0, time.UTC).Format(time.RFC3339),
	}
}
