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
		WillReturnRows(sqlmock.NewRows([]string{"id", "order_id"}).
			AddRow("11111111-1111-1111-1111-111111111111", "260609M9C4UYGC"))
	mock.ExpectQuery("FROM processed_email_keys").
		WithArgs(messageID, sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"order_id"}).
			AddRow("260609M9C4UYGD"))

	preview, err := svc.inspectTarget(
		shopeeRepairTarget{
			BillID:    "source-bill",
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
