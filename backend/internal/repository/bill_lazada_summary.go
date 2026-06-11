package repository

import (
	"math"
	"regexp"
	"strconv"
	"strings"

	"billflow/internal/models"
)

const (
	LazadaReconciliationOK       = "ok"
	LazadaReconciliationMissing  = "missing"
	LazadaReconciliationMismatch = "mismatch"
)

var lazadaAmountPattern = regexp.MustCompile(`(?i)(?:THB|฿)?\s*(-?\d{1,3}(?:,\d{3})*(?:\.\d+)?|-?\d+(?:\.\d+)?)`)

type LazadaAmountSummary struct {
	GoodsTotalAmount        float64 `json:"goods_total_amount"`
	ShippingAmount          float64 `json:"shipping_amount"`
	CouponDiscountAmount    float64 `json:"coupon_discount_amount"`
	ServiceFeeAmount        float64 `json:"service_fee_amount"`
	PaidTotalAmount         float64 `json:"paid_total_amount"`
	ShippingMethod          string  `json:"shipping_method,omitempty"`
	PaymentMethod           string  `json:"payment_method,omitempty"`
	TotalDiscountAmount     float64 `json:"total_discount_amount"`
	AllocationMethod        string  `json:"allocation_method,omitempty"`
	ReconciliationStatus    string  `json:"reconciliation_status"`
	ReconciliationDelta     float64 `json:"reconciliation_delta"`
	HasGoodsTotalAmount     bool    `json:"-"`
	HasShippingAmount       bool    `json:"-"`
	HasCouponDiscountAmount bool    `json:"-"`
	HasServiceFeeAmount     bool    `json:"-"`
	HasPaidTotalAmount      bool    `json:"-"`
}

func (s LazadaAmountSummary) FeeAmount() float64 {
	return roundMoney(s.ShippingAmount + s.ServiceFeeAmount)
}

func (s LazadaAmountSummary) HasCoreAmounts() bool {
	return s.HasGoodsTotalAmount && s.HasShippingAmount && s.HasCouponDiscountAmount && s.HasPaidTotalAmount
}

// ExtractLazadaAmountSummary parses Lazada marketplace totals from the
// deterministic email text/HTML. AI extraction must not be used for money
// reconciliation because the PO amount needs to match Lazada's paid amount.
func ExtractLazadaAmountSummary(bodyText, bodyHTML string) LazadaAmountSummary {
	text := shopeeSummaryBody(bodyText, bodyHTML)
	lines := lazadaSummaryLines(text)

	var s LazadaAmountSummary
	if v, ok := lazadaMoneyAfterLabel(lines, "ยอดรวม:"); ok {
		s.GoodsTotalAmount = v
		s.HasGoodsTotalAmount = true
	}
	if v, ok := lazadaMoneyAfterLabel(lines, "ค่าธรรมเนียมจัดส่ง:"); ok {
		s.ShippingAmount = v
		s.HasShippingAmount = true
	}
	if v, ok := lazadaMoneyAfterLabel(lines, "คูปองส่วนลด:"); ok {
		s.CouponDiscountAmount = math.Abs(v)
		s.HasCouponDiscountAmount = true
	}
	if v, ok := lazadaMoneyAfterLabel(lines, "Service fee:"); ok {
		s.ServiceFeeAmount = v
		s.HasServiceFeeAmount = true
	}
	if v, ok := lazadaMoneyAfterLabel(lines, "ยอดรวมทั้งหมด(รวม VAT):"); ok {
		s.PaidTotalAmount = v
		s.HasPaidTotalAmount = true
	}
	s.ShippingMethod = lazadaTextAfterLabel(lines, "วิธีการจัดส่ง:")
	s.PaymentMethod = lazadaTextAfterLabel(lines, "ช่องทางการชำระเงิน:")

	s.TotalDiscountAmount = roundMoney(s.CouponDiscountAmount)
	if s.TotalDiscountAmount > 0 {
		s.AllocationMethod = "proportional_by_gross_excluding_shipping"
	}
	if !s.HasCoreAmounts() {
		s.ReconciliationStatus = LazadaReconciliationMissing
		return s
	}
	expected := roundMoney(s.GoodsTotalAmount + s.ShippingAmount + s.ServiceFeeAmount - s.CouponDiscountAmount)
	s.ReconciliationDelta = roundMoney(expected - s.PaidTotalAmount)
	if math.Abs(s.ReconciliationDelta) <= 0.01 {
		s.ReconciliationStatus = LazadaReconciliationOK
		s.ReconciliationDelta = 0
	} else {
		s.ReconciliationStatus = LazadaReconciliationMismatch
	}
	return s
}

// ExtractLazadaSellerName parses the seller displayed in Lazada purchase
// emails. This field is operationally important because purchase-order SML
// remarks use the marketplace seller, and AI sometimes returns generic values
// such as "Lazada Thailand" even when the email contains the real shop name.
func ExtractLazadaSellerName(bodyText, bodyHTML string) string {
	text := shopeeSummaryBody(bodyText, bodyHTML)
	lines := lazadaSummaryLines(text)
	return lazadaSellerAfterLabel(lines)
}

func ApplyLazadaDiscountsToItems(items []models.BillItem, couponDiscount float64) {
	discounts := AllocateLazadaDiscountsByLine(items, couponDiscount)
	for i := range items {
		items[i].DiscountAmount = discounts[i]
	}
}

func AllocateLazadaDiscountsByLine(items []models.BillItem, couponDiscount float64) []float64 {
	out := make([]float64, len(items))
	couponDiscount = roundMoney(couponDiscount)
	if couponDiscount <= 0 {
		return out
	}

	totalGross := 0.0
	discountable := []int{}
	for i, item := range items {
		if models.IsMarketplaceFeeSourceSKU(item.SourceSKU) {
			continue
		}
		gross := itemGross(item)
		if gross <= 0 {
			continue
		}
		totalGross = roundMoney(totalGross + gross)
		discountable = append(discountable, i)
	}
	if totalGross <= 0 || len(discountable) == 0 {
		return out
	}

	distributed := 0.0
	for j, idx := range discountable {
		var portion float64
		if j == len(discountable)-1 {
			portion = roundMoney(couponDiscount - distributed)
		} else {
			portion = roundMoney(couponDiscount * itemGross(items[idx]) / totalGross)
		}
		capacity := itemGross(items[idx])
		if portion > capacity {
			portion = capacity
		}
		if portion < 0 {
			portion = 0
		}
		out[idx] = portion
		distributed = roundMoney(distributed + portion)
	}
	return out
}

func lazadaSummaryLines(text string) []string {
	if looksLikeHTML(text) {
		text = htmlToSummaryText(text)
	}
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		clean := strings.Join(strings.Fields(strings.TrimSpace(line)), " ")
		if clean != "" {
			out = append(out, clean)
		}
	}
	return out
}

func lazadaMoneyAfterLabel(lines []string, label string) (float64, bool) {
	for i, line := range lines {
		idx := strings.Index(line, label)
		if idx < 0 {
			continue
		}
		if v, ok := parseLazadaAmount(line[idx+len(label):]); ok {
			return v, true
		}
		for j := i + 1; j < len(lines) && j <= i+8; j++ {
			next := strings.TrimSpace(lines[j])
			if next == "" {
				continue
			}
			if lazadaLooksLikeNextLabel(next) {
				break
			}
			if v, ok := parseLazadaAmount(next); ok {
				return v, true
			}
		}
	}
	return 0, false
}

func parseLazadaAmount(value string) (float64, bool) {
	value = strings.TrimSpace(value)
	if value == "" || strings.EqualFold(value, "THB") {
		return 0, false
	}
	matches := lazadaAmountPattern.FindAllStringSubmatch(value, -1)
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		clean := strings.ReplaceAll(m[1], ",", "")
		v, err := strconv.ParseFloat(clean, 64)
		if err == nil {
			return roundMoney(v), true
		}
	}
	return 0, false
}

func lazadaTextAfterLabel(lines []string, label string) string {
	for i, line := range lines {
		idx := strings.Index(line, label)
		if idx < 0 {
			continue
		}
		value := strings.TrimSpace(strings.TrimLeft(line[idx+len(label):], " \t:-："))
		if value != "" {
			return value
		}
		for j := i + 1; j < len(lines) && j <= i+4; j++ {
			next := strings.TrimSpace(lines[j])
			if next == "" {
				continue
			}
			if lazadaLooksLikeNextLabel(next) {
				break
			}
			return next
		}
	}
	return ""
}

func lazadaSellerAfterLabel(lines []string) string {
	const label = "จัดจำหน่ายโดย"
	for i, line := range lines {
		idx := strings.Index(line, label)
		if idx < 0 {
			continue
		}
		value := trimLazadaSellerName(line[idx+len(label):])
		if value != "" {
			return value
		}
		for j := i + 1; j < len(lines) && j <= i+4; j++ {
			next := strings.TrimSpace(lines[j])
			if next == "" {
				continue
			}
			if lazadaLooksLikeNextLabel(next) || lazadaLooksLikeSellerBoundary(next) {
				break
			}
			if value := trimLazadaSellerName(next); value != "" {
				return value
			}
		}
	}
	return ""
}

func trimLazadaSellerName(value string) string {
	value = strings.TrimSpace(strings.TrimLeft(value, " \t:-："))
	if value == "" {
		return ""
	}
	value = strings.Join(strings.Fields(value), " ")
	for _, boundary := range lazadaSellerBoundaryLabels() {
		if idx := strings.Index(value, boundary); idx >= 0 {
			value = strings.TrimSpace(value[:idx])
		}
	}
	return strings.TrimSpace(strings.Trim(value, " \t\r\n:-："))
}

func lazadaLooksLikeSellerBoundary(line string) bool {
	for _, label := range lazadaSellerBoundaryLabels() {
		if strings.Contains(line, label) {
			return true
		}
	}
	return false
}

func lazadaSellerBoundaryLabels() []string {
	return []string{
		"วันและเวลาจัดส่ง",
		"การจัดส่ง",
		"รายละเอียดคำสั่งซื้อ",
		"รายละเอียดสินค้า",
		"ยอดรวม:",
		"ค่าธรรมเนียมจัดส่ง:",
		"คูปองส่วนลด:",
		"Service fee:",
		"ยอดรวมทั้งหมด(รวม VAT):",
		"วิธีการจัดส่ง:",
		"ช่องทางการชำระเงิน:",
	}
}

func lazadaLooksLikeNextLabel(line string) bool {
	for _, label := range []string{
		"ยอดรวม:",
		"ค่าธรรมเนียมจัดส่ง:",
		"คูปองส่วนลด:",
		"Service fee:",
		"ยอดรวมทั้งหมด(รวม VAT):",
		"วิธีการจัดส่ง:",
		"ช่องทางการชำระเงิน:",
		"การจัดส่ง",
		"รายละเอียดการชำระเงิน",
	} {
		if strings.Contains(line, label) {
			return true
		}
	}
	return strings.HasSuffix(line, ":") || strings.HasSuffix(line, "：")
}
