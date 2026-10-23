package repository

import (
	"fmt"
	"strings"
	"testing"
)

func lazadaCard(sku, image, qty, price string) string {
	return fmt.Sprintf(`<table><tr><td><img src="https://th-live-01.slatic.net/p/%s.jpg"></td><td><a href="https://www.lazada.co.th/products/i123-s%s.html"><span>นาฬิกา LED รุ่น HB-4819</span></a><div>THB %s</div><div>จำนวน: %s</div></td></tr></table>`, image, sku, price, qty)
}

func TestParseLazadaEmailItemsPreservesDuplicateNamesAndQuantities(t *testing.T) {
	body := lazadaCard("11", "white", "2", "678.00") + lazadaCard("12", "green", "2", "678.00")
	items, err := ParseLazadaEmailItems(body, LazadaAmountSummary{HasGoodsTotalAmount: true, GoodsTotalAmount: 2712})
	if err != nil || len(items) != 2 {
		t.Fatalf("items=%+v err=%v", items, err)
	}
	for i, item := range items {
		if item.Qty != 2 || item.Price == nil || *item.Price != 678 || item.SourceLineNo != i+1 {
			t.Fatalf("line %d: %+v", i, item)
		}
	}
	if items[0].SourceSKU != "11" || items[1].SourceSKU != "12" || items[0].SourceImageURL == items[1].SourceImageURL {
		t.Fatalf("variant identity lost: %+v", items)
	}
	discounts := AllocateLazadaDiscountsByLine(items, 288)
	if discounts[0] != 144 || discounts[1] != 144 {
		t.Fatalf("discounts=%v", discounts)
	}
}

func TestParseLazadaEmailItemsRejectsIncompleteOrMismatchedSource(t *testing.T) {
	for _, body := range []string{
		lazadaCard("11", "white", "2", "678.00"),
		lazadaCard("11", "white", "2", "678.00") + lazadaCard("12", "green", "2", "unknown"),
		lazadaCard("11", "white", "0", "678.00"),
		lazadaCard("11", "white", "-2", "678.00"),
	} {
		if _, err := ParseLazadaEmailItems(body, LazadaAmountSummary{HasGoodsTotalAmount: true, GoodsTotalAmount: 2712}); err == nil {
			t.Fatalf("unsafe source accepted: %s", body)
		}
	}
}

func TestParseLazadaEmailItemsSupportsMissingSKUAndIgnoresRecommendations(t *testing.T) {
	body := strings.ReplaceAll(lazadaCard("11", "white", "2", "678.00"), `https://www.lazada.co.th/products/i123-s11.html`, "https://example.test/tracking")
	body += `<div><img src="https://th-live-01.slatic.net/p/ad.jpg"><a>สินค้าแนะนำ</a>THB 100.00</div>`
	items, err := ParseLazadaEmailItems(body, LazadaAmountSummary{HasGoodsTotalAmount: true, GoodsTotalAmount: 1356})
	if err != nil || len(items) != 1 || items[0].SourceSKU != "" || items[0].Qty != 2 {
		t.Fatalf("items=%+v err=%v", items, err)
	}
}

func TestParseLazadaEmailItemsKeepsIdenticalSourceRows(t *testing.T) {
	body := lazadaCard("11", "white", "2", "678.00")
	items, err := ParseLazadaEmailItems(body+body, LazadaAmountSummary{HasGoodsTotalAmount: true, GoodsTotalAmount: 2712})
	if err != nil || len(items) != 2 {
		t.Fatalf("source rows must not be deduplicated: items=%+v err=%v", items, err)
	}
}

func TestParseLazadaEmailItemsRequiresSourceEvidence(t *testing.T) {
	items, err := ParseLazadaEmailItems(`<p>ไม่มีรายละเอียดสินค้า</p>`, LazadaAmountSummary{})
	if err != nil || len(items) != 0 {
		t.Fatalf("unsupported template should allow review fallback: items=%+v err=%v", items, err)
	}
	if _, err := ParseLazadaEmailItems(lazadaCard("11", "white", "2", "678.00"), LazadaAmountSummary{}); err == nil {
		t.Fatal("source rows without goods total must not be treated as verified")
	}
}

func TestParseLazadaEmailItemsNestedMJMLAndSplitCurrency(t *testing.T) {
	body := `<html><head><style>.noise { content: "จำนวน: 999"; }</style></head><body><table><tr><td>จัดจำหน่ายโดย: ร้านตัวอย่าง</td></tr><tr><td><table><tr><td><img src="https://th-live-01.slatic.net/p/white.jpg"></td><td><div><a href="https://c.lazada.co.th/t/x?url=https%3A%2F%2Fwww.lazada.co.th%2Fproducts%2Fi123-s11.html">นาฬิกา LED</a></div><table><tr><td>THB</td><td>678.00</td></tr></table><div><span>จำนวน:</span><span>2</span></div></td></tr></table></td></tr></table><table><tr><td>ยอดรวม:</td><td>THB 1,356.00</td></tr><tr><td>ค่าธรรมเนียมจัดส่ง:</td><td>THB 134.00</td></tr></table></body></html>`
	items, err := ParseLazadaEmailItems(body, LazadaAmountSummary{HasGoodsTotalAmount: true, GoodsTotalAmount: 1356})
	if err != nil || len(items) != 1 || items[0].SourceSKU != "11" || *items[0].Price != 678 {
		t.Fatalf("items=%+v err=%v", items, err)
	}
}
