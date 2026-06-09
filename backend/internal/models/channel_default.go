package models

import (
	"encoding/json"
	"strings"
	"time"
)

type MarketplacePrintPolicy struct {
	RequiresAllOrdersSMLDoc    bool     `json:"requires_all_orders_sml_doc"`
	PaymentMethodPrefixEnabled bool     `json:"payment_method_prefix_enabled"`
	PaymentMethodPrefixes      []string `json:"payment_method_prefixes"`
	PaymentMethods             []string `json:"payment_methods"`
}

type MarketplacePrintPolicyInput struct {
	RequiresAllOrdersSMLDoc    *bool    `json:"requires_all_orders_sml_doc"`
	PaymentMethodPrefixEnabled *bool    `json:"payment_method_prefix_enabled"`
	PaymentMethodPrefixes      []string `json:"payment_method_prefixes"`
	PaymentMethods             []string `json:"payment_methods"`
}

func SupportsMarketplacePrintPolicy(channel, billType string) bool {
	if billType != "purchase" {
		return false
	}
	return channel == "shopee_shipped" || channel == "lazada_email"
}

func DefaultMarketplacePrintPolicy(channel, billType string) MarketplacePrintPolicy {
	if !SupportsMarketplacePrintPolicy(channel, billType) {
		return MarketplacePrintPolicy{}
	}
	return MarketplacePrintPolicy{
		RequiresAllOrdersSMLDoc:    true,
		PaymentMethodPrefixEnabled: true,
		PaymentMethodPrefixes:      []string{"TT"},
		PaymentMethods:             DefaultMarketplacePrintPaymentMethods(),
	}
}

func DefaultMarketplacePrintPaymentMethods() []string {
	return []string{
		"TT2789",
		"TT9630",
		"TT0972",
		"TT9628",
		"TT5128",
		"TT5432",
		"TT3086",
		"TT8456",
		"โอน Kbank",
		"โอน TTB5074",
		"โอน KTB",
		"โอน BBL",
		"โอน TTB1135",
	}
}

func NormalizeMarketplacePrintPolicy(channel, billType string, in *MarketplacePrintPolicyInput) (MarketplacePrintPolicy, error) {
	p := DefaultMarketplacePrintPolicy(channel, billType)
	if !SupportsMarketplacePrintPolicy(channel, billType) {
		return p, nil
	}
	if in == nil {
		return p, nil
	}
	if in.RequiresAllOrdersSMLDoc != nil {
		p.RequiresAllOrdersSMLDoc = *in.RequiresAllOrdersSMLDoc
	}
	if in.PaymentMethodPrefixEnabled != nil {
		p.PaymentMethodPrefixEnabled = *in.PaymentMethodPrefixEnabled
	}
	if in.PaymentMethodPrefixes != nil {
		p.PaymentMethodPrefixes = normalizeUpperTokens(in.PaymentMethodPrefixes)
	}
	if in.PaymentMethods != nil {
		p.PaymentMethods = normalizePaymentMethods(in.PaymentMethods)
	}
	if p.PaymentMethodPrefixEnabled && len(p.PaymentMethodPrefixes) == 0 {
		return p, ErrPrintPolicyRequiresPrefix
	}
	if len(p.PaymentMethods) == 0 {
		return p, ErrPrintPolicyRequiresPaymentMethod
	}
	return p, nil
}

func NormalizeMarketplacePrintPolicyFromRaw(channel, billType string, raw json.RawMessage) MarketplacePrintPolicy {
	p := DefaultMarketplacePrintPolicy(channel, billType)
	if !SupportsMarketplacePrintPolicy(channel, billType) || len(raw) == 0 {
		return p
	}
	var in MarketplacePrintPolicyInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return p
	}
	normalized, err := NormalizeMarketplacePrintPolicy(channel, billType, &in)
	if err != nil {
		return p
	}
	return normalized
}

func MarketplacePrintPolicyNote(p MarketplacePrintPolicy) string {
	parts := []string{}
	if p.RequiresAllOrdersSMLDoc {
		parts = append(parts, "ส่งเข้า SML ครบทุกคำสั่งซื้อในอีเมลเดียวกัน")
	}
	if p.PaymentMethodPrefixEnabled {
		parts = append(parts, "วิธีการชำระเงินขึ้นต้นด้วย "+strings.Join(p.PaymentMethodPrefixes, ", "))
	}
	if len(parts) == 0 {
		return "พร้อมปริ้นตามเงื่อนไขเอกสารที่ส่งเข้า SML แล้ว"
	}
	return "พร้อมปริ้น = " + strings.Join(parts, " และ ")
}

type printPolicyError string

func (e printPolicyError) Error() string { return string(e) }

const ErrPrintPolicyRequiresPrefix = printPolicyError("print policy requires at least one payment method prefix")
const ErrPrintPolicyRequiresPaymentMethod = printPolicyError("print policy requires at least one payment method")

func normalizeUpperTokens(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, value := range values {
		v := strings.ToUpper(strings.TrimSpace(value))
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

func normalizePaymentMethods(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, value := range values {
		v := strings.TrimSpace(value)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

// ChannelDefault is the per-(channel, bill_type) party setting that decides
// which SML customer (sale) or supplier (purchase) bills route to.
//
// PartyCode is sent as cust_code in saleorder/saleinvoice/purchaseorder.
// PartyName overrides the AI-extracted contact_name so SML doesn't
// create a fresh AR row for every session.
type ChannelDefault struct {
	Channel              string `json:"channel"`
	BillType             string `json:"bill_type"`
	PartyCode            string `json:"party_code"`
	PartyName            string `json:"party_name"`
	PartyPhone           string `json:"party_phone"`
	PartyAddress         string `json:"party_address"`
	PartyTaxID           string `json:"party_tax_id"`
	DocFormatCode        string `json:"doc_format_code"`
	Endpoint             string `json:"endpoint"`
	DocPrefix            string `json:"doc_prefix"`
	DocRunningFormat     string `json:"doc_running_format"`
	BranchCode           string `json:"branch_code"`
	SaleCode             string `json:"sale_code"`
	UnitCode             string `json:"unit_code"`
	DocTime              string `json:"doc_time"`
	ShippingItemEnabled  bool   `json:"shipping_item_enabled"`
	ShippingItemCode     string `json:"shipping_item_code"`
	ShippingItemUnitCode string `json:"shipping_item_unit_code"`
	PassbookCode         string `json:"passbook_code"`
	PassbookName         string `json:"passbook_name"`
	BankCode             string `json:"bank_code"`
	BankBranch           string `json:"bank_branch"`
	ExpenseCode          string `json:"expense_code"`
	ExpenseName          string `json:"expense_name"`
	// Inventory + VAT overrides (sentinel: empty / -1 = "use server default")
	WHCode      string                 `json:"wh_code"`
	ShelfCode   string                 `json:"shelf_code"`
	VATType     int                    `json:"vat_type"`
	VATRate     float64                `json:"vat_rate"`
	InquiryType int                    `json:"inquiry_type"` // -1 = ยังไม่ได้ตั้ง (กรอกตอนส่ง)
	Remark2     string                 `json:"remark_2"`     // sentinel: '' = ไม่ระบุ
	PrintPolicy MarketplacePrintPolicy `json:"print_policy"`
	UpdatedBy   *string                `json:"updated_by,omitempty"`
	UpdatedAt   time.Time              `json:"updated_at"`
}

// ChannelDefaultUpsert is the admin-supplied payload for PUT.
// PartyName/Phone/Address/TaxID come from the SML party master (snapshot at
// save time) so the table can render code+name without a second SML lookup.
// Endpoint blank = auto-resolve by (channel, bill_type) in bills.go.
type ChannelDefaultUpsert struct {
	Channel              string `json:"channel" binding:"required,oneof=line email shopee shopee_email shopee_shipped lazada lazada_email tiktok manual shopee_settlement"`
	BillType             string `json:"bill_type" binding:"required,oneof=sale purchase ar_receipt"`
	PartyCode            string `json:"party_code"`
	PartyName            string `json:"party_name"`
	PartyPhone           string `json:"party_phone"`
	PartyAddress         string `json:"party_address"`
	PartyTaxID           string `json:"party_tax_id"`
	DocFormatCode        string `json:"doc_format_code"`
	Endpoint             string `json:"endpoint"` // free-form URL or path; bills.go detects client by keyword
	DocPrefix            string `json:"doc_prefix"`
	DocRunningFormat     string `json:"doc_running_format"`
	BranchCode           string `json:"branch_code"`
	SaleCode             string `json:"sale_code"`
	UnitCode             string `json:"unit_code"`
	DocTime              string `json:"doc_time"`
	ShippingItemEnabled  bool   `json:"shipping_item_enabled"`
	ShippingItemCode     string `json:"shipping_item_code"`
	ShippingItemUnitCode string `json:"shipping_item_unit_code"`
	PassbookCode         string `json:"passbook_code"`
	PassbookName         string `json:"passbook_name"`
	BankCode             string `json:"bank_code"`
	BankBranch           string `json:"bank_branch"`
	ExpenseCode          string `json:"expense_code"`
	ExpenseName          string `json:"expense_name"`
	// Inventory + VAT overrides; empty / -1 = "use server default"
	WHCode      string                       `json:"wh_code"`
	ShelfCode   string                       `json:"shelf_code"`
	VATType     int                          `json:"vat_type"`
	VATRate     float64                      `json:"vat_rate"`
	InquiryType int                          `json:"inquiry_type"` // -1 = ยังไม่ได้ตั้ง
	Remark2     string                       `json:"remark_2"`     // sentinel: '' = ไม่ระบุ
	PrintPolicy *MarketplacePrintPolicyInput `json:"print_policy"`
}
