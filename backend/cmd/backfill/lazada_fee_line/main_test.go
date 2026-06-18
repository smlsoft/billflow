package main

import (
	"testing"
	"time"

	"billflow/internal/models"
	"billflow/internal/repository"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestListLazadaFeeLineTargets(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("WITH candidates").
		WithArgs(models.LazadaFeeSourceSKU).
		WillReturnRows(sqlmock.NewRows([]string{"bill_id", "order_id", "fee_amount", "existing_items"}).
			AddRow("294d5d92-eab8-41df-a9f0-2c32c3a55d82", "1110438913895692", 104.0, 1))

	targets, err := listLazadaFeeLineTargets(db)
	if err != nil {
		t.Fatalf("listLazadaFeeLineTargets: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("targets len = %d, want 1", len(targets))
	}
	got := targets[0]
	if got.BillID != "294d5d92-eab8-41df-a9f0-2c32c3a55d82" ||
		got.OrderID != "1110438913895692" ||
		got.FeeAmount != 104 ||
		got.ExistingItems != 1 {
		t.Fatalf("target = %+v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func TestBuildLazadaFeeLineItemFromConfiguredDefault(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("FROM channel_defaults").
		WithArgs("lazada_email", "purchase").
		WillReturnRows(channelDefaultRows().AddRow(
			"lazada_email", "purchase", "", "", "", "", "", "PO", "/api/v1/ic/purchase-orders",
			"BF-PO", "YYMM####", "", "", "", "", true, "SHIP_CUS", "บาท", "", "", "", "", "", "", "", "", -1, -1.0, -1, "", []byte("{}"), nil, time.Now(),
		))

	target := lazadaFeeLineTarget{
		BillID:        "294d5d92-eab8-41df-a9f0-2c32c3a55d82",
		OrderID:       "1110438913895692",
		FeeAmount:     104,
		ExistingItems: 1,
	}
	item, err := buildLazadaFeeLineItem(repository.NewChannelDefaultRepo(db), nil, target)
	if err != nil {
		t.Fatalf("buildLazadaFeeLineItem: %v", err)
	}
	if item.BillID != target.BillID || item.SourceSKU != models.LazadaFeeSourceSKU {
		t.Fatalf("item identity = %+v", item)
	}
	if item.ItemCode == nil || *item.ItemCode != "SHIP_CUS" {
		t.Fatalf("item code = %v, want SHIP_CUS", item.ItemCode)
	}
	if item.UnitCode == nil || *item.UnitCode != "บาท" {
		t.Fatalf("unit code = %v, want บาท", item.UnitCode)
	}
	if item.Price == nil || *item.Price != 104 || item.Qty != 1 || !item.Mapped {
		t.Fatalf("amount fields = %+v", item)
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
		"passbook_code", "passbook_name", "bank_code", "bank_branch", "expense_code", "expense_name",
		"wh_code", "shelf_code", "vat_type", "vat_rate",
		"inquiry_type", "remark_2",
		"print_policy",
		"updated_by", "updated_at",
	})
}
