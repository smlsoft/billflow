package handlers

import (
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"go.uber.org/zap"

	"billflow/internal/models"
	"billflow/internal/repository"
	"billflow/internal/services/ai"
	"billflow/internal/services/artifact"
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

	h := &EmailHandler{
		billRepo:    repository.NewBillRepo(db),
		artifactSvc: artifact.New(t.TempDir(), 10<<20, repository.NewBillArtifactRepo(db), zap.NewNop()),
		logger:      zap.NewNop(),
	}
	messageID := "shipped-message@example.test"
	orderID := "#2604294EP99PKT"
	existingBillID := "768a0068-cad3-4b6e-b229-a5d2ce2ede73"
	htmlBody := "<html></html>"

	mock.ExpectQuery("SELECT").
		WithArgs(messageID, orderID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("FROM bills").
		WithArgs("2604294EP99PKT").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(existingBillID))
	mock.ExpectExec("INSERT INTO shopee_order_events").
		WithArgs(existingBillID, "2604294EP99PKT", shopeeEventShipped, "ถูกจัดส่งแล้ว", "คำสั่งซื้อ #2604294EP99PKT ถูกจัดส่งแล้ว", "info@mail.shopee.co.th", messageID, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery("INSERT INTO bill_artifacts").
		WithArgs(existingBillID, "email_html", "shopee-shipped.html", "text/html; charset=utf-8", int64(len(htmlBody)), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", time.Now()))
	mock.ExpectQuery("INSERT INTO bill_artifacts").
		WithArgs(existingBillID, "email_envelope", "envelope.json", "application/json", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), nil).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb", time.Now()))
	mock.ExpectExec("INSERT INTO processed_email_keys").
		WithArgs("shopee_shipped", messageID, orderID).
		WillReturnResult(sqlmock.NewResult(1, 1))

	created, err := h.processOneShippedOrder(
		aiExtractedOrderForTest(orderID),
		"คำสั่งซื้อ #2604294EP99PKT ถูกจัดส่งแล้ว",
		"info@mail.shopee.co.th",
		"body",
		htmlBody,
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

func TestConfiguredShopeeShippingLineDisabledDoesNothing(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	h := &EmailHandler{
		channelDefaults: repository.NewChannelDefaultRepo(db),
		logger:          zap.NewNop(),
	}
	mock.ExpectQuery("FROM channel_defaults").
		WithArgs("shopee_shipped", "purchase").
		WillReturnRows(channelDefaultRows().AddRow(
			"shopee_shipped", "purchase", "", "", "", "", "", "PO", "/api/v1/ic/purchase-orders",
			"BF-PO", "YYMM####", "", "", "", "", false, "", "", "", "", -1, -1.0, nil, time.Now(),
		))

	item, ready := h.configuredShopeeShippingLine("#2601AAA", 38, true)
	if item != nil || ready {
		t.Fatalf("item=%+v ready=%v, want disabled nil false", item, ready)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func TestConfiguredShopeeShippingLineUsesConfiguredItem(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	h := &EmailHandler{
		channelDefaults: repository.NewChannelDefaultRepo(db),
		logger:          zap.NewNop(),
	}
	mock.ExpectQuery("FROM channel_defaults").
		WithArgs("shopee_shipped", "purchase").
		WillReturnRows(channelDefaultRows().AddRow(
			"shopee_shipped", "purchase", "", "", "", "", "", "PO", "/api/v1/ic/purchase-orders",
			"BF-PO", "YYMM####", "", "", "", "", true, "SHIP_TEST", "ครั้ง", "", "", -1, -1.0, nil, time.Now(),
		))

	item, ready := h.configuredShopeeShippingLine("#2601AAA", 38, true)
	if item == nil {
		t.Fatal("expected shipping item")
	}
	if !ready {
		t.Fatal("expected ready shipping item")
	}
	if item.SourceSKU != models.ShopeeShippingSourceSKU {
		t.Fatalf("source_sku = %q, want sentinel", item.SourceSKU)
	}
	if item.ItemCode == nil || *item.ItemCode != "SHIP_TEST" {
		t.Fatalf("item_code = %v, want SHIP_TEST", item.ItemCode)
	}
	if item.UnitCode == nil || *item.UnitCode != "ครั้ง" {
		t.Fatalf("unit_code = %v, want ครั้ง", item.UnitCode)
	}
	if item.Price == nil || *item.Price != 38 || item.Qty != 1 || !item.Mapped {
		t.Fatalf("item = %+v, want qty=1 price=38 mapped=true", item)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func channelDefaultRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"channel", "bill_type", "party_code", "party_name", "party_phone",
		"party_address", "party_tax_id", "doc_format_code", "endpoint",
		"doc_prefix", "doc_running_format",
		"branch_code", "sale_code", "unit_code", "doc_time",
		"shipping_item_enabled", "shipping_item_code", "shipping_item_unit_code",
		"wh_code", "shelf_code", "vat_type", "vat_rate",
		"updated_by", "updated_at",
	})
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
