package handlers

import (
	"strings"
	"testing"
)

func TestDecorateMarketplaceEmailPreviewHTMLShopeeRows(t *testing.T) {
	input := `<html><body><table><tbody>` +
		`<tr><td style="color:#000">ยอดที่ต้องชำระทั้งหมด:</td><td style="color:#000">฿216</td></tr>` +
		`<tr><td style="color:#000">จำนวนเงินที่จ่าย:</td><td style="color:#000">฿216</td></tr>` +
		`</tbody></table></body></html>`

	got := decorateMarketplaceEmailPreviewHTML(input)

	for _, want := range []string{
		`data-billflow-print-highlight="true"`,
		`data-billflow-print-highlight-cell="true"`,
		`background:#fef3c7 !important`,
		`background:#dcfce7 !important`,
		`box-shadow:inset 0 0 0 9999px`,
		`ยอดที่ต้องชำระทั้งหมด`,
		`จำนวนเงินที่จ่าย`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("decorated html missing %q:\n%s", want, got)
		}
	}
}

func TestDecorateMarketplaceEmailPreviewHTMLLazadaVATRow(t *testing.T) {
	input := `<html><body><table><tbody>` +
		`<tr><td>ยอดรวมทั้งหมด(รวม VAT):</td><td>THB</td><td>3,904.10</td></tr>` +
		`</tbody></table></body></html>`

	got := decorateMarketplaceEmailPreviewHTML(input)

	if strings.Count(got, `data-billflow-print-highlight-cell="true"`) != 3 {
		t.Fatalf("expected every Lazada total row cell to be highlighted:\n%s", got)
	}
	if !strings.Contains(got, `background:#fef3c7 !important`) {
		t.Fatalf("expected yellow highlight:\n%s", got)
	}
}

