package handlers

import (
	"testing"

	"billflow/internal/services/ai"
)

func TestExtractShopeeImageURLsPrefersProductImage(t *testing.T) {
	html := `
		<img src="https://tracking.mail.shopee.co.th/tracking/1/open/abc">
		<img src="https://cf.shopee.sg/file/0cd023d64f04491f3dc8076d6932dfdc">
		<img src="https://cf.shopee.co.th/file/th-11134207-81zth-mimxd9980lc477">
	`

	got := extractShopeeImageURLs(html)
	if len(got) == 0 {
		t.Fatal("extractShopeeImageURLs() returned no URLs")
	}
	if got[0] != "https://cf.shopee.co.th/file/th-11134207-81zth-mimxd9980lc477" {
		t.Fatalf("first URL = %q, want product image", got[0])
	}
	for _, u := range got {
		if u == "https://tracking.mail.shopee.co.th/tracking/1/open/abc" {
			t.Fatal("tracking pixel URL should be excluded")
		}
	}
}

func TestMatchShopeeItemImagesPreservesExistingImage(t *testing.T) {
	items := []ai.ExtractedItem{{
		RawName:  "เสื้อกีฬา สีดำ",
		Qty:      1,
		ImageURL: "https://already.example/product.jpg",
	}}
	html := `<img src="https://cf.shopee.co.th/file/th-new-product">เสื้อกีฬา สีดำ`

	got, decisions := MatchShopeeItemImages(items, html, "")
	if got[0].ImageURL != "https://already.example/product.jpg" {
		t.Fatalf("ImageURL = %q, want existing image", got[0].ImageURL)
	}
	if decisions[0].Reason != ShopeeItemImageReasonExisting {
		t.Fatalf("reason = %q, want existing", decisions[0].Reason)
	}
}

func TestMatchShopeeItemImagesUsesSingleProductFallback(t *testing.T) {
	items := []ai.ExtractedItem{{RawName: "อ่านชื่อจาก HTML ไม่เจอ", Qty: 1}}
	html := `
		<img src="https://tracking.mail.shopee.co.th/open/abc">
		<img src="https://cf.shopee.co.th/file/th-11134207-81zth-product-a">
	`

	got, decisions := MatchShopeeItemImages(items, html, "")
	if got[0].ImageURL != "https://cf.shopee.co.th/file/th-11134207-81zth-product-a" {
		t.Fatalf("ImageURL = %q, want single product fallback", got[0].ImageURL)
	}
	if decisions[0].Reason != ShopeeItemImageReasonSingleFallback {
		t.Fatalf("reason = %q, want single fallback", decisions[0].Reason)
	}
}

func TestMatchShopeeItemImagesMatchesNearestName(t *testing.T) {
	items := []ai.ExtractedItem{
		{RawName: "รองเท้าวิ่ง รุ่น A", Qty: 1},
		{RawName: "ถุงเท้ากีฬา รุ่น B", Qty: 2},
	}
	html := `
		<section>
			<img src="https://cf.shopee.co.th/file/th-11134207-81zth-shoe">
			<div>รองเท้าวิ่ง รุ่น A</div>
		</section>
		<section>
			<img src="https://f.shopee.co.th/file/th-11134207-81zth-sock">
			<div>ถุงเท้ากีฬา รุ่น B</div>
		</section>
	`

	got, decisions := MatchShopeeItemImages(items, html, "")
	if got[0].ImageURL != "https://cf.shopee.co.th/file/th-11134207-81zth-shoe" {
		t.Fatalf("item 0 ImageURL = %q", got[0].ImageURL)
	}
	if got[1].ImageURL != "https://f.shopee.co.th/file/th-11134207-81zth-sock" {
		t.Fatalf("item 1 ImageURL = %q", got[1].ImageURL)
	}
	if decisions[0].Reason != ShopeeItemImageReasonNearest || decisions[1].Reason != ShopeeItemImageReasonNearest {
		t.Fatalf("reasons = %q, %q; want nearest", decisions[0].Reason, decisions[1].Reason)
	}
}

func TestMatchShopeeItemImagesLeavesAmbiguousMissingNameBlank(t *testing.T) {
	items := []ai.ExtractedItem{{RawName: "ชื่อที่ไม่มีใน HTML", Qty: 1}}
	html := `
		<img src="https://cf.shopee.co.th/file/th-11134207-81zth-product-a">
		<img src="https://cf.shopee.co.th/file/th-11134207-81zth-product-b">
	`

	got, decisions := MatchShopeeItemImages(items, html, "")
	if got[0].ImageURL != "" {
		t.Fatalf("ImageURL = %q, want blank for ambiguous fallback", got[0].ImageURL)
	}
	if decisions[0].Reason != ShopeeItemImageReasonAmbiguous {
		t.Fatalf("reason = %q, want ambiguous", decisions[0].Reason)
	}
}

func TestMatchShopeeItemImagesScopesByOrderID(t *testing.T) {
	items := []ai.ExtractedItem{{RawName: "สินค้าเป้าหมาย", Qty: 1}}
	html := `
		<div>#260700000001</div>
		<img src="https://cf.shopee.co.th/file/th-11134207-81zth-first-order">
		<div>สินค้าออเดอร์แรก</div>
		<div>#260700000002</div>
		<img src="https://cf.shopee.co.th/file/th-11134207-81zth-target-order">
		<div>สินค้าเป้าหมาย</div>
	`

	got, decisions := MatchShopeeItemImages(items, html, "260700000002")
	if got[0].ImageURL != "https://cf.shopee.co.th/file/th-11134207-81zth-target-order" {
		t.Fatalf("ImageURL = %q, want target order image", got[0].ImageURL)
	}
	if decisions[0].Reason != ShopeeItemImageReasonNearest {
		t.Fatalf("reason = %q, want nearest", decisions[0].Reason)
	}
}
