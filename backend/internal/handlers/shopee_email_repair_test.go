package handlers

import (
	"math"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestShopeeEmailRepairPreviewDetectsMissingAndStaleTombstone(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	svc := &ShopeeEmailRepairService{db: db}
	messageID := "shopee-repair@example.test"
	body := strings.Join([]string{
		"หมายเลขคำสั่งซื้อ: #260609M9C4UYGC",
		"ผู้ขาย: existing_shop",
		"ยอดที่ต้องชำระทั้งหมด ฿100",
		"หมายเลขคำสั่งซื้อ: #260609M9C4UYGD",
		"ผู้ขาย: missing_shop",
		"ยอดที่ต้องชำระทั้งหมด ฿50",
	}, "\n")

	mock.ExpectQuery("FROM bills").
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "order_id", "status", "subject", "sml_doc_no", "sent"}).
			AddRow("11111111-1111-1111-1111-111111111111", "260609M9C4UYGC", "needs_review", "ยืนยันการชำระเงินคำสั่งซื้อหมายเลข #260609M9C4UYGC", "", false))
	mock.ExpectQuery("FROM processed_email_keys").
		WithArgs(messageID, sqlmock.AnyArg(), "shopee_shipped").
		WillReturnRows(sqlmock.NewRows([]string{"order_id"}).
			AddRow("260609M9C4UYGD"))

	preview, err := svc.inspectTarget(
		shopeeRepairTarget{
			BillID:    "source-bill",
			Source:    "shopee_shipped",
			MessageID: messageID,
			Subject:   "ยืนยันการชำระเงินคำสั่งซื้อหมายเลข #260609M9C4UYGC",
		},
		shopeeRepairEmailBody{Text: body, ArtifactID: "artifact-1"},
	)
	if err != nil {
		t.Fatalf("inspectTarget: %v", err)
	}
	if preview.DetectedOrderCount != 2 || preview.ExistingCount != 1 || preview.MissingCount != 1 {
		t.Fatalf("counts = detected:%d existing:%d missing:%d", preview.DetectedOrderCount, preview.ExistingCount, preview.MissingCount)
	}
	if got := preview.MissingOrderIDs; len(got) != 1 || got[0] != "260609M9C4UYGD" {
		t.Fatalf("missing ids = %#v", got)
	}
	if math.Abs(preview.EmailTotal-150) > 0.01 {
		t.Fatalf("email total = %.2f, want 150.00", preview.EmailTotal)
	}
	if !preview.HasStaleTombstones || len(preview.StaleTombstoneOrderIDs) != 1 || preview.StaleTombstoneOrderIDs[0] != "260609M9C4UYGD" {
		t.Fatalf("stale tombstones = %#v", preview.StaleTombstoneOrderIDs)
	}
	if !preview.CanRepair {
		t.Fatalf("CanRepair = false, want true")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func TestShopeeEmailRepairPreviewClassifiesShippingCreatedBills(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	svc := &ShopeeEmailRepairService{db: db}
	messageID := "payment-message@example.test"
	body := strings.Join([]string{
		"หมายเลขคำสั่งซื้อ: #260608HPC8A42A",
		"ยอดที่ต้องชำระทั้งหมด ฿100",
		"หมายเลขคำสั่งซื้อ: #260608HPC8A42B",
		"ยอดที่ต้องชำระทั้งหมด ฿200",
		"หมายเลขคำสั่งซื้อ: #260608HPC8A42C",
		"ยอดที่ต้องชำระทั้งหมด ฿300",
	}, "\n")

	mock.ExpectQuery("FROM bills").
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "order_id", "status", "subject", "sml_doc_no", "sent"}).
			AddRow("11111111-1111-1111-1111-111111111111", "260608HPC8A42A", "needs_review", "คำสั่งซื้อ #260608HPC8A42A ถูกจัดส่งแล้ว", "", false).
			AddRow("22222222-2222-2222-2222-222222222222", "260608HPC8A42B", "sent", "คำสั่งซื้อ #260608HPC8A42B ถูกจัดส่งแล้ว", "POL26060014", true))
	mock.ExpectQuery("FROM processed_email_keys").
		WithArgs(messageID, sqlmock.AnyArg(), "shopee_shipped").
		WillReturnRows(sqlmock.NewRows([]string{"order_id"}))

	preview, err := svc.inspectTarget(
		shopeeRepairTarget{
			BillID:    "source-bill",
			Source:    "shopee_shipped",
			MessageID: messageID,
			Subject:   "ยืนยันการชำระเงินคำสั่งซื้อหมายเลข #260608HPC8A42A",
		},
		shopeeRepairEmailBody{Text: body, ArtifactID: "artifact-1"},
	)
	if err != nil {
		t.Fatalf("inspectTarget: %v", err)
	}
	if preview.MissingCount != 1 || len(preview.MissingOrderIDs) != 1 || preview.MissingOrderIDs[0] != "260608HPC8A42C" {
		t.Fatalf("missing = %d %#v", preview.MissingCount, preview.MissingOrderIDs)
	}
	if preview.RebuildCount != 1 || len(preview.RebuildOrderIDs) != 1 || preview.RebuildOrderIDs[0] != "260608HPC8A42A" {
		t.Fatalf("rebuild = %d %#v", preview.RebuildCount, preview.RebuildOrderIDs)
	}
	if preview.BlockedCount != 1 || len(preview.BlockedOrderIDs) != 1 || preview.BlockedOrderIDs[0] != "260608HPC8A42B" {
		t.Fatalf("blocked = %d %#v", preview.BlockedCount, preview.BlockedOrderIDs)
	}
	if !preview.CanRepair {
		t.Fatalf("CanRepair = false, want true")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func TestShopeeEmailRepairPreviewSupportsLazadaConfirmationSubject(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	svc := &ShopeeEmailRepairService{db: db}
	messageID := "lazada-payment@example.test"
	body := strings.Join([]string{
		"ยืนยันคำสั่งซื้อหมายเลข 1094738208195692",
		"ยอดรวม: THB 700.00",
		"ค่าธรรมเนียมจัดส่ง: THB 50.00",
		"คูปองส่วนลด: THB 0.00",
		"ยอดรวมทั้งหมด(รวม VAT): THB 750.00",
	}, "\n")

	mock.ExpectQuery("FROM bills").
		WithArgs(sqlmock.AnyArg(), lazadaEmailSource).
		WillReturnRows(sqlmock.NewRows([]string{"id", "order_id", "status", "subject", "sml_doc_no", "sent"}).
			AddRow("33333333-3333-3333-3333-333333333333", "1094738208195692", "needs_review", "ยืนยันคำสั่งซื้อหมายเลข 1094738208195692", "", false))
	preview, err := svc.inspectTarget(
		shopeeRepairTarget{
			BillID:    "source-bill",
			Source:    lazadaEmailSource,
			MessageID: messageID,
			Subject:   "ยืนยันคำสั่งซื้อหมายเลข 1094738208195692",
		},
		shopeeRepairEmailBody{Text: body, ArtifactID: "artifact-1"},
	)
	if err != nil {
		t.Fatalf("inspectTarget: %v", err)
	}
	if preview.Source != lazadaEmailSource {
		t.Fatalf("source = %q", preview.Source)
	}
	if preview.DetectedOrderCount != 1 || preview.ExistingCount != 1 || preview.RebuildCount != 1 || preview.MissingCount != 0 {
		t.Fatalf("counts = detected:%d existing:%d rebuild:%d missing:%d", preview.DetectedOrderCount, preview.ExistingCount, preview.RebuildCount, preview.MissingCount)
	}
	if got := preview.RebuildOrderIDs; len(got) != 1 || got[0] != "1094738208195692" {
		t.Fatalf("rebuild ids = %#v", got)
	}
	if math.Abs(preview.EmailTotal-750) > 0.01 {
		t.Fatalf("email total = %.2f, want 750.00", preview.EmailTotal)
	}
	if !preview.CanRepair {
		t.Fatalf("CanRepair = false, want true")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}
