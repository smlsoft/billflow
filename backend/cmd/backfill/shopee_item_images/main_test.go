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

func TestPlanShopeeItemImageUpdatesUsesQuantityAndPriceForDuplicateNames(t *testing.T) {
	price60 := 60.0
	price63 := 63.0
	price66 := 66.0
	rawName := "ชุดเซทแก้วน้ำ-กระบอกน้ำ 500 ml. กระบอกน้ำเก็บอุณหภูมิ กระบอกน้ำสแตนเลส แก้วน้ำกระบอกน้ำสแตนเลส"
	target := shopeeItemImageTarget{
		ID:      "bill-duplicate-name",
		OrderID: "26072071GA0FY0",
		Items: []shopeeItemImageTargetItem{
			{ID: "item-black", RawName: rawName, Qty: 3, Price: &price60},
			{ID: "item-pink", RawName: rawName, Qty: 2, Price: &price63},
			{ID: "item-green", RawName: rawName, Qty: 2, Price: &price66},
		},
	}
	html := `
		<div>#26072071GA0FY0</div>
		<section>
			<img src="https://cf.shopee.co.th/file/th-11134207-7rase-black">
			<div>ชุดเซทแก้วน้ำ-กระบอกน้ำ 500 ml. กระบอกน้ำเก็บอุณหภูมิ กระบอกน้ำสแตนเลส แก้วน้ำกระบอกน้ำสแตนเลส</div>
			<div>ตัวเลือกสินค้า: black</div>
			<div>จำนวน: 3</div>
			<div>ราคา: ฿60</div>
		</section>
		<section>
			<img src="https://cf.shopee.co.th/file/th-11134207-7rase-green">
			<div>ชุดเซทแก้วน้ำ-กระบอกน้ำ 500 ml. กระบอกน้ำเก็บอุณหภูมิ กระบอกน้ำสแตนเลส แก้วน้ำกระบอกน้ำสแตนเลส</div>
			<div>ตัวเลือกสินค้า: green</div>
			<div>จำนวน: 2</div>
			<div>ราคา: ฿66</div>
		</section>
		<section>
			<img src="https://cf.shopee.co.th/file/th-11134207-7rase-pink">
			<div>ชุดเซทแก้วน้ำ-กระบอกน้ำ 500 ml. กระบอกน้ำเก็บอุณหภูมิ กระบอกน้ำสแตนเลส แก้วน้ำกระบอกน้ำสแตนเลส</div>
			<div>ตัวเลือกสินค้า: pink</div>
			<div>จำนวน: 2</div>
			<div>ราคา: ฿63</div>
		</section>
	`

	updates, summary := planShopeeItemImageUpdates(target, html)
	if len(updates) != 3 {
		t.Fatalf("updates len = %d, want 3 (%+v)", len(updates), updates)
	}
	want := map[string]string{
		"item-black": "https://cf.shopee.co.th/file/th-11134207-7rase-black",
		"item-pink":  "https://cf.shopee.co.th/file/th-11134207-7rase-pink",
		"item-green": "https://cf.shopee.co.th/file/th-11134207-7rase-green",
	}
	for _, update := range updates {
		if update.ImageURL != want[update.ItemID] {
			t.Fatalf("update for %s image = %q, want %q", update.ItemID, update.ImageURL, want[update.ItemID])
		}
	}
	if summary.NoMatch != 0 || summary.Ambiguous != 0 {
		t.Fatalf("summary NoMatch=%d Ambiguous=%d, want zero", summary.NoMatch, summary.Ambiguous)
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
