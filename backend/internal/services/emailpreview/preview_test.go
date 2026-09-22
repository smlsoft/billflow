package emailpreview

import (
	"strings"
	"testing"
)

func TestPrepareHTMLMatchesMarketplacePreviewContract(t *testing.T) {
	input := `<html><head><title>Email</title></head><body><table><tr><td>ยอดที่ต้องชำระทั้งหมด:</td><td>฿216</td></tr><tr><td>จำนวนเงินที่จ่าย:</td><td>฿216</td></tr></table></body></html>`
	got := PrepareHTML(input)
	for _, want := range []string{
		`id="billflow-email-preview-reset"`,
		`data-billflow-print-highlight="true"`,
		`background:#fef3c7 !important`,
		`background:#dcfce7 !important`,
		`img{display:block;max-width:100%}`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("prepared HTML missing %q:\n%s", want, got)
		}
	}
}

func TestPrepareHTMLWrapsFragmentAndIsIdempotent(t *testing.T) {
	first := PrepareHTML(`<p>สวัสดี</p>`)
	second := PrepareHTML(first)
	if strings.Count(second, `id="billflow-email-preview-reset"`) != 1 {
		t.Fatalf("expected exactly one reset style:\n%s", second)
	}
	if !strings.Contains(second, `<meta charset="utf-8">`) {
		t.Fatalf("expected UTF-8 wrapper:\n%s", second)
	}
}

func TestPreparePrintableHTMLAddsEveryPOLAndOneCardStamp(t *testing.T) {
	input := `<html><body><table>
		<tr><td>หมายเลขคำสั่งซื้อ:</td><td><a href="/one">#260820TRCU7Q7G</a></td></tr>
		<tr><td>หมายเลขคำสั่งซื้อ:</td><td><a href="/two">#260820TRCU7Q7H</a></td></tr>
	</table></body></html>`

	got := PreparePrintableHTML(input, PrintContext{
		SourceChannel: "Shopee",
		Orders: []PrintOrder{
			{OrderID: "260820TRCU7Q7G", SMLDocNo: "POL26080483", PaymentMethod: "tt9630"},
			{OrderID: "#260820TRCU7Q7H", SMLDocNo: "POL26080496", PaymentMethod: "TT9630"},
		},
	})

	for _, want := range []string{
		`#260820TRCU7Q7G</a><span data-billflow-order-label="true"`,
		`→ POL26080483`,
		`→ POL26080496`,
		`จ่ายบัตรเครดิต`,
		`TT9630`,
		`position:fixed;right:10mm;top:8mm`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("prepared printable HTML missing %q:\n%s", want, got)
		}
	}
	if strings.Count(got, `data-billflow-order-label="true"`) != 2 {
		t.Fatalf("POL labels = %d, want 2:\n%s", strings.Count(got, `data-billflow-order-label="true"`), got)
	}
	if strings.Count(got, `data-billflow-print="true"`) != 1 {
		t.Fatalf("card stamps = %d, want 1:\n%s", strings.Count(got, `data-billflow-print="true"`), got)
	}
}
