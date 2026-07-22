package main

import (
	"encoding/json"
	"testing"

	"billflow/internal/models"
	"billflow/internal/repository"
	"billflow/internal/services/artifact"

	"github.com/DATA-DOG/go-sqlmock"
	"go.uber.org/zap"
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

func TestPlanShopeeItemImageUpdatesSafelyMatchesEqualDuplicateVariants(t *testing.T) {
	price669 := 669.0
	price679 := 679.0
	name := "Homsmart รถเข็นพับได้ 45L/65L"
	target := shopeeItemImageTarget{
		ID:      "25dc9505-8b0d-452c-b020-f4f8e5ab4831",
		OrderID: "2607219G32VR3V",
		Items: []shopeeItemImageTargetItem{
			{ID: "item-669-a", RawName: name, Qty: 1, Price: &price669, DiscountAmount: 167.5},
			{ID: "item-669-b", RawName: name, Qty: 1, Price: &price669, DiscountAmount: 167.5},
			{
				ID: "item-679", RawName: name, Qty: 1, Price: &price679, DiscountAmount: 170,
				SourceImageURL: "https://cf.shopee.co.th/file/th-existing-679",
			},
		},
	}
	html := `
		<div>#2607219G32VR3V</div>
		<img src="https://cf.shopee.co.th/file/th-existing-679">
		<div>Homsmart รถเข็นพับได้ 45L/65L</div><div>ตัวเลือกสินค้า: O14 ชมพู - 65L</div><div>จำนวน: 1</div><div>ราคา: ฿679</div>
		<img src="https://cf.shopee.co.th/file/th-pink-669">
		<div>Homsmart รถเข็นพับได้ 45L/65L</div><div>ตัวเลือกสินค้า: S-15 ชมพู - 45L</div><div>จำนวน: 1</div><div>ราคา: ฿669</div>
		<img src="https://cf.shopee.co.th/file/th-mint-669">
		<div>Homsmart รถเข็นพับได้ 45L/65L</div><div>ตัวเลือกสินค้า: S-15 มิ้นต์ - 45L</div><div>จำนวน: 1</div><div>ราคา: ฿669</div>
	`

	updates, summary := planShopeeItemImageUpdates(target, html)
	if len(updates) != 3 {
		t.Fatalf("updates len = %d, want 3 (%+v)", len(updates), updates)
	}
	want := map[string]struct {
		image   string
		variant string
		line    int
	}{
		"item-669-a": {"https://cf.shopee.co.th/file/th-pink-669", "S-15 ชมพู - 45L", 2},
		"item-669-b": {"https://cf.shopee.co.th/file/th-mint-669", "S-15 มิ้นต์ - 45L", 3},
		"item-679":   {"https://cf.shopee.co.th/file/th-existing-679", "O14 ชมพู - 65L", 1},
	}
	for _, update := range updates {
		expected := want[update.ItemID]
		if update.ImageURL != expected.image || update.SourceVariant != expected.variant || update.SourceLineNo != expected.line {
			t.Fatalf("update for %s = %+v, want %+v", update.ItemID, update, expected)
		}
	}
	if summary.MatchedDuplicateGroup != 2 || summary.MatchedByURL != 1 || summary.ManualReview != 0 {
		t.Fatalf("summary = %+v", summary)
	}
}

func TestPlanShopeeItemImageUpdatesRejectsDuplicateGroupWithDifferentMappings(t *testing.T) {
	price := 669.0
	target := shopeeItemImageTarget{
		ID:      "bill-different-mappings",
		OrderID: "2607219G32VR3V",
		Items: []shopeeItemImageTargetItem{
			{ID: "item-a", RawName: "รถเข็นพับได้", Qty: 1, Price: &price, DiscountAmount: 100, Mapped: true, ItemCode: "CART-PINK", UnitCode: "ชิ้น"},
			{ID: "item-b", RawName: "รถเข็นพับได้", Qty: 1, Price: &price, DiscountAmount: 100, Mapped: true, ItemCode: "CART-MINT", UnitCode: "ชิ้น"},
		},
	}
	html := `
		<div>#2607219G32VR3V</div>
		<img src="https://cf.shopee.co.th/file/th-pink"><div>รถเข็นพับได้</div><div>ตัวเลือกสินค้า: ชมพู</div><div>จำนวน: 1</div><div>ราคา: ฿669</div>
		<img src="https://cf.shopee.co.th/file/th-mint"><div>รถเข็นพับได้</div><div>ตัวเลือกสินค้า: มิ้นต์</div><div>จำนวน: 1</div><div>ราคา: ฿669</div>
	`

	updates, summary := planShopeeItemImageUpdates(target, html)
	if len(updates) != 0 {
		t.Fatalf("updates = %+v, want none", updates)
	}
	if summary.ManualReview != 2 {
		t.Fatalf("ManualReview = %d, want 2", summary.ManualReview)
	}
}

func TestListShopeeItemImageTargetsFiltersBillAndUnsafeStates(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	billID := "25dc9505-8b0d-452c-b020-f4f8e5ab4831"
	columns := []string{
		"bill_id", "order_id", "email_message_id", "raw_data", "item_id", "raw_name", "qty", "price",
		"discount_amount", "mapped", "item_code", "unit_code", "source_image_url", "source_variant", "source_line_no",
	}
	mock.ExpectQuery(`(?s)FROM bills b.*b\.archived_at IS NULL.*b\.sent_at IS NULL.*COALESCE\(b\.sml_doc_no, ''\) = ''.*b\.status IN.*NULLIF\(\$1, ''\)`).
		WithArgs(billID, models.ShopeeShippingSourceSKU).
		WillReturnRows(sqlmock.NewRows(columns).AddRow(
			billID, "2607219G32VR3V", "message-id", []byte(`{"body_html":"<html></html>"}`),
			"item-1", "รถเข็น", 1.0, 669.0, 100.0, false, "", "", "", "", 0,
		))

	targets, err := listShopeeItemImageTargets(db, billID)
	if err != nil {
		t.Fatalf("listShopeeItemImageTargets: %v", err)
	}
	if len(targets) != 1 || targets[0].ID != billID || len(targets[0].Items) != 1 {
		t.Fatalf("targets = %+v", targets)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestProcessShopeeItemImageTargetDryRunDoesNotWrite(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	price := 100.0
	rawData, _ := json.Marshal(map[string]string{
		"body_html": `<img src="https://cf.shopee.co.th/file/th-product"><div>สินค้า</div><div>จำนวน: 1</div><div>ราคา: ฿100</div>`,
	})
	target := shopeeItemImageTarget{
		ID: "11111111-1111-1111-1111-111111111111", RawData: rawData,
		Items: []shopeeItemImageTargetItem{{ID: "item-1", RawName: "สินค้า", Qty: 1, Price: &price}},
	}
	mock.ExpectQuery(`FROM bill_artifacts`).
		WithArgs(target.ID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "bill_id", "kind", "filename", "content_type", "size_bytes", "sha256", "storage_path", "source_meta", "created_at",
		}))

	artifactRepo := repository.NewBillArtifactRepo(db)
	artifactSvc := artifact.New(t.TempDir(), 10<<20, artifactRepo, zap.NewNop())
	summary := shopeeItemImageBackfillSummary{}
	processShopeeItemImageTarget(db, artifactSvc, artifactRepo, repository.NewAuditLogRepo(db), target, true, &summary)

	if summary.WouldUpdate != 1 || summary.Updated != 0 {
		t.Fatalf("summary = %+v", summary)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("dry-run performed an unexpected database operation: %v", err)
	}
}

func TestUpdateShopeeItemImageMetadataScopesToUnsentShopeePurchase(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectExec(`(?s)UPDATE bill_items bi.*source_variant.*source_line_no.*b\.source = 'shopee_shipped'.*b\.bill_type = 'purchase'.*b\.sent_at IS NULL.*COALESCE\(b\.sml_doc_no, ''\) = ''`).
		WithArgs("https://cf.shopee.co.th/file/th-11134207-81zth-update", "สีดำ", 2, "11111111-1111-1111-1111-111111111111").
		WillReturnResult(sqlmock.NewResult(0, 1))

	changed, err := updateShopeeItemImageMetadata(db, shopeeItemImageUpdate{
		ItemID: "11111111-1111-1111-1111-111111111111", ImageURL: "https://cf.shopee.co.th/file/th-11134207-81zth-update",
		SourceVariant: "สีดำ", SourceLineNo: 2,
	})
	if err != nil {
		t.Fatalf("updateShopeeItemImageMetadata: %v", err)
	}
	if !changed {
		t.Fatal("changed = false, want true")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestUpdateShopeeItemImageMetadataReturnsFalseWhenNothingIsEligible(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectExec(`(?s)UPDATE bill_items bi.*b\.source = 'shopee_shipped'.*b\.sent_at IS NULL`).
		WithArgs("https://cf.shopee.co.th/file/th-11134207-81zth-update", "", 0, "22222222-2222-2222-2222-222222222222").
		WillReturnResult(sqlmock.NewResult(0, 0))

	changed, err := updateShopeeItemImageMetadata(db, shopeeItemImageUpdate{
		ItemID: "22222222-2222-2222-2222-222222222222", ImageURL: "https://cf.shopee.co.th/file/th-11134207-81zth-update",
	})
	if err != nil {
		t.Fatalf("updateShopeeItemImageMetadata: %v", err)
	}
	if changed {
		t.Fatal("changed = true, want false")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}
