package handlers

import (
	"strings"
	"testing"

	"billflow/internal/services/ai"
	emailservice "billflow/internal/services/email"
)

func TestExtractLazadaOrderID(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{
			name: "thai subject",
			text: "ยืนยันคำสั่งซื้อหมายเลข 1107473377495692",
			want: "1107473377495692",
		},
		{
			name: "shipped subject",
			text: "คำสั่งซื้อหมายเลข 1107071348695692 ได้รับการจัดส่งเรียบร้อยแล้ว",
			want: "1107071348695692",
		},
		{
			name: "fallback long number",
			text: "Order details 1107071348695692",
			want: "1107071348695692",
		},
		{
			name: "too short skipped",
			text: "Order 12345",
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractLazadaOrderID(tt.text); got != tt.want {
				t.Fatalf("extractLazadaOrderID() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPrepareLazadaEmailTextUsesTextNotFullHTML(t *testing.T) {
	html := `<html><body><div>header</div><div>ยืนยันคำสั่งซื้อหมายเลข 1107473377495692</div><div>สินค้า A</div></body></html>`
	got := prepareLazadaEmailText("", html)
	if got == "" {
		t.Fatal("prepared text is empty")
	}
	if len(got) >= len(html) {
		t.Fatalf("prepared text length = %d, want less than raw html %d", len(got), len(html))
	}
	if got == html {
		t.Fatal("prepared text should strip HTML")
	}
	if !containsAll(got, []string{"ยืนยันคำสั่งซื้อหมายเลข", "สินค้า A"}) {
		t.Fatalf("prepared text %q missing order/item content", got)
	}
}

func TestNormalizeLazadaOrderID(t *testing.T) {
	if got := normalizeLazadaOrderID(" #110-747-337-749-5692 "); got != "1107473377495692" {
		t.Fatalf("normalizeLazadaOrderID() = %q", got)
	}
}

func TestValidateLazadaExtractedOrderID(t *testing.T) {
	expected := "1107473377495692"
	if mismatch := validateLazadaExtractedOrderID(expected, []ai.ExtractedOrder{{OrderID: expected}, {}}); mismatch != nil {
		t.Fatalf("matching/source fallback orders should be accepted: %+v", mismatch)
	}
	mismatch := validateLazadaExtractedOrderID(expected, []ai.ExtractedOrder{{OrderID: "1107473377495693"}})
	if mismatch == nil {
		t.Fatal("different AI order id should be rejected")
	}
	if mismatch.Expected != expected || len(mismatch.Unexpected) != 1 || mismatch.Unexpected[0] != "1107473377495693" {
		t.Fatalf("mismatch = %+v", mismatch)
	}
}

func TestNormalizeLazadaEmailDocDatePrefersMailHeader(t *testing.T) {
	source := emailservice.MailSource{EmailDate: "2026-06-04T09:31:00+07:00"}
	if got := normalizeLazadaEmailDocDate("2024-06-04", source); got != "2026-06-04" {
		t.Fatalf("doc date = %q, want mail date", got)
	}
}

func TestNormalizeLazadaDocDateStringBuddhistYear(t *testing.T) {
	if got := normalizeLazadaDocDateString("2569-06-03"); got != "2026-06-03" {
		t.Fatalf("doc date = %q, want 2026-06-03", got)
	}
}

func TestResolveLazadaEmailSellerNamePrefersEmailLabel(t *testing.T) {
	body := "ยืนยันคำสั่งซื้อหมายเลข 1100887410295692\nจัดจำหน่ายโดย: CCC Sports\nวันและเวลาจัดส่ง 12 มิถุนายน"
	if got := resolveLazadaEmailSellerName("Lazada Thailand", body, ""); got != "CCC Sports" {
		t.Fatalf("seller = %q, want CCC Sports", got)
	}
}

func TestResolveLazadaEmailSellerNameFallsBackToAIThenDefault(t *testing.T) {
	if got := resolveLazadaEmailSellerName("Mostna Store", "no seller label", ""); got != "Mostna Store" {
		t.Fatalf("seller = %q, want AI seller", got)
	}
	if got := resolveLazadaEmailSellerName("", "no seller label", ""); got != "Lazada" {
		t.Fatalf("seller = %q, want Lazada", got)
	}
}

func TestAttachLazadaItemImagesUsesNearestProductImage(t *testing.T) {
	html := `
	  <img src="https://lzd-img-global.slatic.net/g/tps/logo.png">
	  <img style="display:none" src="https://sg.mmstat.com/track.gif">
	  <a><img src="https://sg-test-11.slatic.net/p/product-table.jpg"></a>
	  <span>【กรุงเทพพร้อมส่ง】โต๊ะพับได้ โต๊ะพับ โต๊ะพับอเนกประสงค์</span>
	  <img src="https://th-live.slatic.net/p/recommendation.jpg">
	`
	items := []ai.ExtractedItem{{
		RawName: "【กรุงเทพพร้อมส่ง】โต๊ะพับได้ โต๊ะพับ โต๊ะพับอเนกประสงค์",
		Qty:     1,
	}}

	got := attachLazadaItemImages(items, html)
	if len(got) != 1 {
		t.Fatalf("items len = %d, want 1", len(got))
	}
	if got[0].ImageURL != "https://sg-test-11.slatic.net/p/product-table.jpg" {
		t.Fatalf("ImageURL = %q", got[0].ImageURL)
	}
}

func TestAttachLazadaItemSourceMetadataSeparatesDuplicateNamesByProductSKU(t *testing.T) {
	name := "Icebase กระติกเก็บความเย็น ขนาด39ลิตร (TITAN) รุ่น CLT-039"
	html := `
	  <section>
	    <img src="https://th-live-01.slatic.net/p/green.jpg">
	    <a href="https://c.lazada.co.th/t/c.example?url=https%3A%2F%2Fwww.lazada.co.th%2Fproducts%2Fi16181655661-s127271408131.html%3FurlFlag%3Dtrue"><span>` + name + `</span></a>
	    <span>THB 3,266.00</span><span>จำนวน: 1</span>
	  </section>
	  <section>
	    <img src="https://th-live-01.slatic.net/p/blue.jpg">
	    <a href="https://c.lazada.co.th/t/c.example?url=https%3A%2F%2Fwww.lazada.co.th%2Fproducts%2Fi16181655661-s127271408130.html%3FurlFlag%3Dtrue"><span>` + name + `</span></a>
	    <span>THB 3,266.00</span><span>จำนวน: 1</span>
	  </section>
	`
	items := []ai.ExtractedItem{
		{RawName: name, Qty: 1},
		{RawName: name, Qty: 1},
	}

	got, metadata := attachLazadaItemSourceMetadata(items, html)
	if len(metadata) != 2 {
		t.Fatalf("metadata len = %d, want 2", len(metadata))
	}
	if metadata[0].SourceSKU != "127271408131" || metadata[0].SourceLineNo != 1 {
		t.Fatalf("metadata[0] = %+v", metadata[0])
	}
	if metadata[1].SourceSKU != "127271408130" || metadata[1].SourceLineNo != 2 {
		t.Fatalf("metadata[1] = %+v", metadata[1])
	}
	if got[0].ImageURL != "https://th-live-01.slatic.net/p/green.jpg" {
		t.Fatalf("first image = %q", got[0].ImageURL)
	}
	if got[1].ImageURL != "https://th-live-01.slatic.net/p/blue.jpg" {
		t.Fatalf("second image = %q", got[1].ImageURL)
	}
}

func TestLazadaDuplicateRawNamesRequireExplicitMapping(t *testing.T) {
	items := []ai.ExtractedItem{
		{RawName: "กระติก TITAN 45L", Qty: 1},
		{RawName: "กระติก TITAN 45L", Qty: 1},
		{RawName: "โต๊ะพับ", Qty: 1},
	}
	counts := lazadaDuplicateRawNameCounts(items)
	if lazadaItemCanAutoMap("กระติก TITAN 45L", counts) {
		t.Fatal("duplicate Lazada title must not be auto-mapped before a SKU alias exists")
	}
	if !lazadaItemCanAutoMap("โต๊ะพับ", counts) {
		t.Fatal("unique Lazada title should remain eligible for high-confidence auto-mapping")
	}
}

func TestAttachLazadaItemImagesKeepsExistingImage(t *testing.T) {
	items := []ai.ExtractedItem{{
		RawName:  "เก้าอี้แคมป์ปิ้ง",
		Qty:      1,
		ImageURL: "https://example.test/existing.jpg",
	}}
	got := attachLazadaItemImages(items, `<img src="https://sg-test-11.slatic.net/p/new.jpg">เก้าอี้แคมป์ปิ้ง`)
	if got[0].ImageURL != items[0].ImageURL {
		t.Fatalf("ImageURL overwritten = %q", got[0].ImageURL)
	}
}

func TestAttachLazadaItemImagesFallsBackForSingleItem(t *testing.T) {
	items := []ai.ExtractedItem{{
		RawName: "AI normalized name not found in original html",
		Qty:     1,
	}}
	got := attachLazadaItemImages(items, `<img src="https://th-live.slatic.net/p/fallback.jpg">`)
	if got[0].ImageURL != "https://th-live.slatic.net/p/fallback.jpg" {
		t.Fatalf("ImageURL = %q", got[0].ImageURL)
	}
}

func TestAttachLazadaItemImagesDoesNotFallbackForMultipleItems(t *testing.T) {
	items := []ai.ExtractedItem{
		{RawName: "missing one", Qty: 1},
		{RawName: "missing two", Qty: 1},
	}
	got := attachLazadaItemImages(items, `<img src="https://th-live.slatic.net/p/fallback.jpg">`)
	if got[0].ImageURL != "" || got[1].ImageURL != "" {
		t.Fatalf("unexpected fallback images: %#v", got)
	}
}

func TestIsLazadaProductImageURLFiltersNonProductImages(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"https://sg-test-11.slatic.net/p/6c0ec81a579646381608fd76801b05b1.jpg", true},
		{"https://th-live.slatic.net/p/2210ca43d70f507ea35cdb3f6df2e50e.png", true},
		{"https://lzd-img-global.slatic.net/g/tps/imgextra/logo.png", false},
		{"https://sg.mmstat.com/lzdmailer.module.exp?mail_id=1", false},
		{"https://example.test/p/product.jpg", false},
	}
	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			if got := isLazadaProductImageURL(tt.url); got != tt.want {
				t.Fatalf("isLazadaProductImageURL() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLazadaBatchCompletionError(t *testing.T) {
	if err := lazadaBatchCompletionError(1, 0, 0); err != nil {
		t.Fatalf("created-only batch returned error: %v", err)
	}

	if err := lazadaBatchCompletionError(0, 1, 0); err == nil {
		t.Fatal("skipped-only batch should return a skip error")
	} else if _, ok := err.(*emailservice.MessageSkipError); !ok {
		t.Fatalf("skipped-only batch returned %T, want MessageSkipError", err)
	}

	if err := lazadaBatchCompletionError(0, 0, 1); err == nil {
		t.Fatal("failed batch should return an error")
	} else if strings.Contains(err.Error(), "duplicate_or_empty") {
		t.Fatalf("failed batch should not be reported as a user skip: %v", err)
	}
}

func containsAll(text string, needles []string) bool {
	for _, needle := range needles {
		if !strings.Contains(text, needle) {
			return false
		}
	}
	return true
}
