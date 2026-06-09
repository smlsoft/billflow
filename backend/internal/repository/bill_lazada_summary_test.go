package repository

import (
	"testing"

	"billflow/internal/models"
)

func TestExtractLazadaAmountSummarySplitHTMLLabels(t *testing.T) {
	html := `
<html><body>
  <div>วิธีการจัดส่ง:</div><div>STANDARD</div>
  <div>ช่องทางการชำระเงิน:</div><div>Credit/Debit Card</div>
  <table>
    <tr><td>ยอดรวม:</td><td>THB</td><td>946.05</td></tr>
    <tr><td>ค่าธรรมเนียมจัดส่ง:</td><td>THB</td><td>104.00</td></tr>
    <tr><td>คูปองส่วนลด:</td><td>THB</td><td>175.01</td></tr>
    <tr><td>Service fee:</td><td>THB</td><td>0.00</td></tr>
    <tr><td>ยอดรวมทั้งหมด(รวม VAT):</td><td>THB</td><td>875.04</td></tr>
  </table>
</body></html>`

	got := ExtractLazadaAmountSummary("", html)
	if got.ReconciliationStatus != LazadaReconciliationOK || got.ReconciliationDelta != 0 {
		t.Fatalf("reconciliation = %s delta=%v", got.ReconciliationStatus, got.ReconciliationDelta)
	}
	if got.GoodsTotalAmount != 946.05 || got.ShippingAmount != 104 || got.CouponDiscountAmount != 175.01 || got.PaidTotalAmount != 875.04 {
		t.Fatalf("amounts = %+v", got)
	}
	if got.ShippingMethod != "STANDARD" || got.PaymentMethod != "Credit/Debit Card" {
		t.Fatalf("text labels = shipping %q payment %q", got.ShippingMethod, got.PaymentMethod)
	}
}

func TestExtractLazadaAmountSummaryNoCouponLargeShipping(t *testing.T) {
	body := `
ยอดรวม:
THB
3,597.00
ค่าธรรมเนียมจัดส่ง:
THB
770.00
คูปองส่วนลด:
THB
0.00
Service fee:
THB
0.00
ยอดรวมทั้งหมด(รวม VAT):
THB
4,367.00
`
	got := ExtractLazadaAmountSummary(body, "")
	if got.ReconciliationStatus != LazadaReconciliationOK {
		t.Fatalf("status = %s delta=%v", got.ReconciliationStatus, got.ReconciliationDelta)
	}
	if got.FeeAmount() != 770 {
		t.Fatalf("fee = %v, want 770", got.FeeAmount())
	}
}

func TestExtractLazadaAmountSummaryServiceFee(t *testing.T) {
	body := `
ยอดรวม: THB 100.00
ค่าธรรมเนียมจัดส่ง: THB 38.00
คูปองส่วนลด: THB 10.00
Service fee: THB 5.00
ยอดรวมทั้งหมด(รวม VAT): THB 133.00
`
	got := ExtractLazadaAmountSummary(body, "")
	if got.ReconciliationStatus != LazadaReconciliationOK {
		t.Fatalf("status = %s delta=%v", got.ReconciliationStatus, got.ReconciliationDelta)
	}
	if got.ServiceFeeAmount != 5 || got.FeeAmount() != 43 {
		t.Fatalf("service/fee = %v/%v", got.ServiceFeeAmount, got.FeeAmount())
	}
}

func TestExtractLazadaAmountSummaryMismatch(t *testing.T) {
	body := `
ยอดรวม: THB 100.00
ค่าธรรมเนียมจัดส่ง: THB 38.00
คูปองส่วนลด: THB 10.00
Service fee: THB 0.00
ยอดรวมทั้งหมด(รวม VAT): THB 120.00
`
	got := ExtractLazadaAmountSummary(body, "")
	if got.ReconciliationStatus != LazadaReconciliationMismatch {
		t.Fatalf("status = %s, want mismatch", got.ReconciliationStatus)
	}
	if got.ReconciliationDelta != 8 {
		t.Fatalf("delta = %v, want 8", got.ReconciliationDelta)
	}
}

func TestExtractLazadaAmountSummaryMissing(t *testing.T) {
	got := ExtractLazadaAmountSummary("ยอดรวม: THB 100.00", "")
	if got.ReconciliationStatus != LazadaReconciliationMissing {
		t.Fatalf("status = %s, want missing", got.ReconciliationStatus)
	}
}

func TestAllocateLazadaDiscountsByLineExcludesFeeLine(t *testing.T) {
	p100 := 100.0
	p200 := 200.0
	p38 := 38.0
	items := []models.BillItem{
		{Qty: 1, Price: &p100},
		{Qty: 1, Price: &p200},
		{SourceSKU: models.LazadaFeeSourceSKU, Qty: 1, Price: &p38},
	}
	got := AllocateLazadaDiscountsByLine(items, 30)
	want := []float64{10, 20, 0}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("discount[%d] = %v, want %v (all=%v)", i, got[i], want[i], got)
		}
	}
}
