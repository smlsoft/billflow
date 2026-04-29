package models

import "time"

// ChannelDefault is the per-(channel, bill_type) party setting that decides
// which SML customer (sale) or supplier (purchase) bills route to.
//
// For SML 248 channels (shopee*, lazada), PartyCode is sent as cust_code in
// saleinvoice/purchaseorder. For SML 213 channels (line, email), PartyName
// overrides the AI-extracted contact_name on sale_reserve so SML doesn't
// create a fresh AR row for every chatbot session.
type ChannelDefault struct {
	Channel          string    `json:"channel"`
	BillType         string    `json:"bill_type"`
	PartyCode        string    `json:"party_code"`
	PartyName        string    `json:"party_name"`
	PartyPhone       string    `json:"party_phone"`
	PartyAddress     string    `json:"party_address"`
	PartyTaxID       string    `json:"party_tax_id"`
	DocFormatCode    string    `json:"doc_format_code"`
	Endpoint         string    `json:"endpoint"`
	DocPrefix        string    `json:"doc_prefix"`
	DocRunningFormat string    `json:"doc_running_format"`
	// Inventory + VAT overrides (sentinel: empty / -1 = "use server default")
	WHCode    string  `json:"wh_code"`
	ShelfCode string  `json:"shelf_code"`
	VATType   int     `json:"vat_type"`
	VATRate   float64 `json:"vat_rate"`
	UpdatedBy *string   `json:"updated_by,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ChannelDefaultUpsert is the admin-supplied payload for PUT.
// PartyName/Phone/Address/TaxID come from the SML party master (snapshot at
// save time) so the table can render code+name without a second SML lookup.
// Endpoint blank = auto-resolve by (channel, bill_type) in bills.go.
type ChannelDefaultUpsert struct {
	Channel       string `json:"channel" binding:"required,oneof=line email shopee shopee_email shopee_shipped lazada manual"`
	BillType      string `json:"bill_type" binding:"required,oneof=sale purchase"`
	PartyCode     string `json:"party_code" binding:"required"`
	PartyName     string `json:"party_name" binding:"required"`
	PartyPhone    string `json:"party_phone"`
	PartyAddress  string `json:"party_address"`
	PartyTaxID    string `json:"party_tax_id"`
	DocFormatCode    string `json:"doc_format_code"`
	Endpoint         string `json:"endpoint"` // free-form URL or path; bills.go detects client by keyword
	DocPrefix        string `json:"doc_prefix"`
	DocRunningFormat string `json:"doc_running_format"`
	// Inventory + VAT overrides; empty / -1 = "use server default"
	WHCode    string  `json:"wh_code"`
	ShelfCode string  `json:"shelf_code"`
	VATType   int     `json:"vat_type"`
	VATRate   float64 `json:"vat_rate"`
}
