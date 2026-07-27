package handlers

import (
	"fmt"
	"strings"
	"testing"

	"billflow/internal/services/ai"
)

func shopeeMojibakeFixture(raw string) string {
	var out strings.Builder
	for _, b := range []byte(raw) {
		if b < 0x80 {
			out.WriteByte(b)
			continue
		}
		fmt.Fprintf(&out, "&#%d;", b)
	}
	return out.String()
}

func TestDecodeShopeeHTMLTextRepairsThaiMojibake(t *testing.T) {
	encoded := shopeeMojibakeFixture("ตัวเลือกสินค้า จำนวน ราคา")
	if got := decodeShopeeHTMLText(encoded); got != "ตัวเลือกสินค้า จำนวน ราคา" {
		t.Fatalf("decodeShopeeHTMLText() = %q", got)
	}
}

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

func TestMatchShopeeItemImagesMatchesDuplicateNamesByQuantityAndPrice(t *testing.T) {
	name := "ชุดเซทแก้วน้ำ-กระบอกน้ำ 500 ml. กระบอกน้ำเก็บอุณหภูมิ กระบอกน้ำสแตนเลส แก้วน้ำกระบอกน้ำสแตนเลส"
	price60 := 60.0
	price63 := 63.0
	price66 := 66.0
	items := []ai.ExtractedItem{
		{RawName: name, Qty: 3, Price: &price60},
		{RawName: name, Qty: 2, Price: &price63},
		{RawName: name, Qty: 2, Price: &price66},
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

	got, decisions := MatchShopeeItemImages(items, html, "26072071GA0FY0")
	want := []string{
		"https://cf.shopee.co.th/file/th-11134207-7rase-black",
		"https://cf.shopee.co.th/file/th-11134207-7rase-pink",
		"https://cf.shopee.co.th/file/th-11134207-7rase-green",
	}
	for i := range want {
		if got[i].ImageURL != want[i] {
			t.Fatalf("item %d ImageURL = %q, want %q", i, got[i].ImageURL, want[i])
		}
		if decisions[i].Reason != ShopeeItemImageReasonBlock {
			t.Fatalf("item %d reason = %q, want block", i, decisions[i].Reason)
		}
	}
}

func TestMatchShopeeItemImagesKeepsRepeatedURLForSeparateSourceLines(t *testing.T) {
	name := "ตะกร้าพลาสติก"
	price67 := 67.0
	price68 := 68.0
	imageURL := "https://cf.shopee.co.th/file/th-11134207-81ztp-mnzimihz0nwo63"
	items := []ai.ExtractedItem{
		{RawName: name, Qty: 5, Price: &price67},
		{RawName: name, Qty: 1, Price: &price68},
	}
	html := `
		<div>#260725KGHQSU9U</div>
		<section>
			<img src="https://cf.shopee.co.th/file/th-11134207-81ztp-mnzimihz0nwo63">
			<div>ตะกร้าพลาสติก</div>
			<div>ตัวเลือกสินค้า: M-YD 2542</div>
			<div>จำนวน: 5</div>
			<div>ราคา: ฿67</div>
		</section>
		<section>
			<img src="https://cf.shopee.co.th/file/th-11134207-81ztp-mnzimihz0nwo63">
			<div>ตะกร้าพลาสติก</div>
			<div>ตัวเลือกสินค้า: M-YD 2542</div>
			<div>จำนวน: 1</div>
			<div>ราคา: ฿68</div>
		</section>
	`

	got, decisions := MatchShopeeItemImages(items, html, "260725KGHQSU9U")
	for i := range got {
		if got[i].ImageURL != imageURL {
			t.Fatalf("item %d ImageURL = %q, want %q", i, got[i].ImageURL, imageURL)
		}
		if decisions[i].SourceVariant != "M-YD 2542" {
			t.Fatalf("item %d SourceVariant = %q, want M-YD 2542", i, decisions[i].SourceVariant)
		}
		if decisions[i].SourceLineNo != i+1 {
			t.Fatalf("item %d SourceLineNo = %d, want %d", i, decisions[i].SourceLineNo, i+1)
		}
	}
}

func TestMatchShopeeItemImagesMapsRepeatedExistingURLToDistinctSourceLines(t *testing.T) {
	name := "ตะกร้าพลาสติก"
	price67 := 67.0
	price68 := 68.0
	imageURL := "https://cf.shopee.co.th/file/th-11134207-81ztp-mnzimihz0nwo63"
	items := []ai.ExtractedItem{
		{RawName: name, Qty: 5, Price: &price67, ImageURL: imageURL},
		{RawName: name, Qty: 1, Price: &price68, ImageURL: imageURL},
	}
	html := `
		<section>
			<img src="https://cf.shopee.co.th/file/th-11134207-81ztp-mnzimihz0nwo63">
			<div>ตะกร้าพลาสติก</div><div>ตัวเลือกสินค้า: M-YD 2542</div><div>จำนวน: 5</div><div>ราคา: ฿67</div>
		</section>
		<section>
			<img src="https://cf.shopee.co.th/file/th-11134207-81ztp-mnzimihz0nwo63">
			<div>ตะกร้าพลาสติก</div><div>ตัวเลือกสินค้า: M-YD 2542</div><div>จำนวน: 1</div><div>ราคา: ฿68</div>
		</section>
	`

	_, decisions := MatchShopeeItemImages(items, html, "")
	for i := range decisions {
		if decisions[i].Reason != ShopeeItemImageReasonExisting {
			t.Fatalf("item %d reason = %q, want existing", i, decisions[i].Reason)
		}
		if decisions[i].SourceLineNo != i+1 {
			t.Fatalf("item %d SourceLineNo = %d, want %d", i, decisions[i].SourceLineNo, i+1)
		}
	}
}

func TestMatchShopeeItemImagesMatchesEqualDuplicateVariantsFromMojibakeHTML(t *testing.T) {
	name := "Homsmart รถเข็นพับได้ 45L/65L เข็นลื่น รถเข็นแคมป์ปิ้ง ช้อปปิ้ง นั่งได้ พับเก็บง่าย พกพาสะดวก"
	price669 := 669.0
	price679 := 679.0
	existingURL := "https://cf.shopee.co.th/file/th-11134207-7r98p-lm8m19jq0y6f43"
	items := []ai.ExtractedItem{
		{RawName: name, Qty: 1, Price: &price669},
		{RawName: name, Qty: 1, Price: &price669},
		{RawName: name, Qty: 1, Price: &price679, ImageURL: existingURL},
	}
	html := shopeeMojibakeFixture(`
		<div>#2607219G32VR3V</div>
		<section>
			<img src="https://cf.shopee.co.th/file/th-11134207-7r98p-lm8m19jq0y6f43">
			<div>Homsmart รถเข็นพับได้ 45L/65L เข็นลื่น รถเข็นแคมป์ปิ้ง ช้อปปิ้ง นั่งได้ พับเก็บง่าย พกพาสะดวก</div>
			<div>ตัวเลือกสินค้า: O14 ชมพู - 65L</div>
			<div>จำนวน: 1</div>
			<div>ราคา: ฿679</div>
		</section>
		<section>
			<img src="https://cf.shopee.co.th/file/th-11134207-7r98y-lm8lga05y1vrc3">
			<div>Homsmart รถเข็นพับได้ 45L/65L เข็นลื่น รถเข็นแคมป์ปิ้ง ช้อปปิ้ง นั่งได้ พับเก็บง่าย พกพาสะดวก</div>
			<div>ตัวเลือกสินค้า: S-15 ชมพู - 45L</div>
			<div>จำนวน: 1</div>
			<div>ราคา: ฿669</div>
		</section>
		<section>
			<img src="https://cf.shopee.co.th/file/th-11134207-7r98x-lm8lga05wnbbbf">
			<div>Homsmart รถเข็นพับได้ 45L/65L เข็นลื่น รถเข็นแคมป์ปิ้ง ช้อปปิ้ง นั่งได้ พับเก็บง่าย พกพาสะดวก</div>
			<div>ตัวเลือกสินค้า: S-15 มิ้นต์ - 45L</div>
			<div>จำนวน: 1</div>
			<div>ราคา: ฿669</div>
		</section>
	`)

	got, decisions := MatchShopeeItemImages(items, html, "2607219G32VR3V")
	want := []string{
		"https://cf.shopee.co.th/file/th-11134207-7r98y-lm8lga05y1vrc3",
		"https://cf.shopee.co.th/file/th-11134207-7r98x-lm8lga05wnbbbf",
		existingURL,
	}
	for i := range want {
		if got[i].ImageURL != want[i] {
			t.Fatalf("item %d ImageURL = %q, want %q", i, got[i].ImageURL, want[i])
		}
	}
	wantVariants := []string{"S-15 ชมพู - 45L", "S-15 มิ้นต์ - 45L", "O14 ชมพู - 65L"}
	wantLines := []int{2, 3, 1}
	wantReasons := []string{
		ShopeeItemImageReasonDuplicateGroup,
		ShopeeItemImageReasonDuplicateGroup,
		ShopeeItemImageReasonExisting,
	}
	for i := range decisions {
		if decisions[i].SourceVariant != wantVariants[i] {
			t.Fatalf("item %d SourceVariant = %q, want %q", i, decisions[i].SourceVariant, wantVariants[i])
		}
		if decisions[i].SourceLineNo != wantLines[i] {
			t.Fatalf("item %d SourceLineNo = %d, want %d", i, decisions[i].SourceLineNo, wantLines[i])
		}
		if decisions[i].Reason != wantReasons[i] {
			t.Fatalf("item %d Reason = %q, want %q", i, decisions[i].Reason, wantReasons[i])
		}
	}
}

func TestMatchShopeeItemImagesLeavesEqualDuplicateGroupAmbiguousWhenCountsDiffer(t *testing.T) {
	price := 669.0
	items := []ai.ExtractedItem{
		{RawName: "รถเข็นพับได้", Qty: 1, Price: &price},
		{RawName: "รถเข็นพับได้", Qty: 1, Price: &price},
	}
	html := `
		<img src="https://cf.shopee.co.th/file/th-11134207-one-image">
		<div>รถเข็นพับได้</div>
		<div>ตัวเลือกสินค้า: สีชมพู</div>
		<div>จำนวน: 1</div>
		<div>ราคา: ฿669</div>
	`

	got, decisions := MatchShopeeItemImages(items, html, "")
	for i := range got {
		if got[i].ImageURL != "" {
			t.Fatalf("item %d ImageURL = %q, want blank", i, got[i].ImageURL)
		}
		if decisions[i].Reason != ShopeeItemImageReasonAmbiguous {
			t.Fatalf("item %d Reason = %q, want ambiguous", i, decisions[i].Reason)
		}
	}
}

func TestMatchShopeeItemImagesLeavesDuplicateNamesBlankWithoutNumericMatch(t *testing.T) {
	items := []ai.ExtractedItem{
		{RawName: "สินค้าชื่อซ้ำ"},
		{RawName: "สินค้าชื่อซ้ำ"},
	}
	html := `
		<section>
			<img src="https://cf.shopee.co.th/file/th-11134207-7rase-first">
			<div>สินค้าชื่อซ้ำ</div>
		</section>
		<section>
			<img src="https://cf.shopee.co.th/file/th-11134207-7rase-second">
			<div>สินค้าชื่อซ้ำ</div>
		</section>
	`

	got, decisions := MatchShopeeItemImages(items, html, "")
	for i := range got {
		if got[i].ImageURL != "" {
			t.Fatalf("item %d ImageURL = %q, want blank when duplicate names have no qty/price evidence", i, got[i].ImageURL)
		}
		if decisions[i].Reason != ShopeeItemImageReasonAmbiguous {
			t.Fatalf("item %d reason = %q, want ambiguous", i, decisions[i].Reason)
		}
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
