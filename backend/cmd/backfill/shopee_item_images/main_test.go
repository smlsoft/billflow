package main

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestPlanShopeeItemImageUpdatesSkipsExistingAndAmbiguous(t *testing.T) {
	target := shopeeItemImageTarget{
		ID:      "bill-1",
		OrderID: "260700000002",
		Items: []shopeeItemImageTargetItem{
			{
				ID:             "item-existing",
				RawName:        "มีรูปแล้ว",
				SourceImageURL: "https://already.example/product.jpg",
			},
			{
				ID:      "item-update",
				RawName: "สินค้าที่ควรเติมรูป",
			},
			{
				ID:      "item-ambiguous",
				RawName: "ชื่อที่ไม่มีใน HTML",
			},
		},
	}
	html := `
		<div>#260700000002</div>
		<img src="https://cf.shopee.co.th/file/th-11134207-81zth-update">
		<div>สินค้าที่ควรเติมรูป</div>
		<img src="https://cf.shopee.co.th/file/th-11134207-81zth-other">
	`

	updates, summary := planShopeeItemImageUpdates(target, html)
	if len(updates) != 1 {
		t.Fatalf("updates len = %d, want 1 (%+v)", len(updates), updates)
	}
	if updates[0].ItemID != "item-update" {
		t.Fatalf("updated item = %q, want item-update", updates[0].ItemID)
	}
	if updates[0].ImageURL != "https://cf.shopee.co.th/file/th-11134207-81zth-update" {
		t.Fatalf("image url = %q", updates[0].ImageURL)
	}
	if summary.AlreadyHasImage != 1 {
		t.Fatalf("AlreadyHasImage = %d, want 1", summary.AlreadyHasImage)
	}
	if summary.Ambiguous != 1 {
		t.Fatalf("Ambiguous = %d, want 1", summary.Ambiguous)
	}
	if summary.NoMatch != 0 {
		t.Fatalf("NoMatch = %d, want 0", summary.NoMatch)
	}
}

func TestUpdateShopeeItemImageScopesToShopeePurchaseAndBlank(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectExec(`(?s)UPDATE bill_items bi.*b\.source = 'shopee_shipped'.*b\.bill_type = 'purchase'.*COALESCE\(bi\.source_image_url, ''\) = ''`).
		WithArgs("https://cf.shopee.co.th/file/th-11134207-81zth-update", "11111111-1111-1111-1111-111111111111").
		WillReturnResult(sqlmock.NewResult(0, 1))

	changed, err := updateShopeeItemImage(db, "11111111-1111-1111-1111-111111111111", "https://cf.shopee.co.th/file/th-11134207-81zth-update")
	if err != nil {
		t.Fatalf("updateShopeeItemImage: %v", err)
	}
	if !changed {
		t.Fatal("changed = false, want true")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestUpdateShopeeItemImageReturnsFalseWhenAlreadyFilledOrNotShopee(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectExec(`(?s)UPDATE bill_items bi.*b\.source = 'shopee_shipped'.*COALESCE\(bi\.source_image_url, ''\) = ''`).
		WithArgs("https://cf.shopee.co.th/file/th-11134207-81zth-update", "22222222-2222-2222-2222-222222222222").
		WillReturnResult(sqlmock.NewResult(0, 0))

	changed, err := updateShopeeItemImage(db, "22222222-2222-2222-2222-222222222222", "https://cf.shopee.co.th/file/th-11134207-81zth-update")
	if err != nil {
		t.Fatalf("updateShopeeItemImage: %v", err)
	}
	if changed {
		t.Fatal("changed = true, want false")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}
