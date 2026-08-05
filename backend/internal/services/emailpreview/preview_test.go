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
