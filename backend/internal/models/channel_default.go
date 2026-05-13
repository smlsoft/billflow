package models

import "time"

// ChannelDefault is the per-(channel, bill_type) SML route and document
// numbering config. Party/inventory/VAT columns remain for backward
// compatibility and per-bill send dialogs can override them at retry time.
type ChannelDefault struct {
	Channel          string `json:"channel"`
	BillType         string `json:"bill_type"`
	PartyCode        string `json:"party_code"`
	PartyName        string `json:"party_name"`
	PartyPhone       string `json:"party_phone"`
	PartyAddress     string `json:"party_address"`
	PartyTaxID       string `json:"party_tax_id"`
	DocFormatCode    string `json:"doc_format_code"`
	Endpoint         string `json:"endpoint"`
	DocPrefix        string `json:"doc_prefix"`
	DocRunningFormat string `json:"doc_running_format"`
	BranchCode       string `json:"branch_code"`
	SaleCode         string `json:"sale_code"`
	UnitCode         string `json:"unit_code"`
	DocTime          string `json:"doc_time"`
	// Inventory + VAT overrides (sentinel: empty / -1 = "use server default")
	WHCode    string    `json:"wh_code"`
	ShelfCode string    `json:"shelf_code"`
	VATType   int       `json:"vat_type"`
	VATRate   float64   `json:"vat_rate"`
	UpdatedBy *string   `json:"updated_by,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ChannelDefaultUpsert is the admin-supplied payload for PUT. The frontend
// selects Endpoint from tested SML destinations; blank still falls back to
// source/bill_type routing in bills.go for older rows.
type ChannelDefaultUpsert struct {
	Channel          string `json:"channel" binding:"required,oneof=line email shopee shopee_email shopee_shipped lazada tiktok manual"`
	BillType         string `json:"bill_type" binding:"required,oneof=sale purchase"`
	PartyCode        string `json:"party_code"`
	PartyName        string `json:"party_name"`
	PartyPhone       string `json:"party_phone"`
	PartyAddress     string `json:"party_address"`
	PartyTaxID       string `json:"party_tax_id"`
	DocFormatCode    string `json:"doc_format_code"`
	Endpoint         string `json:"endpoint"`
	DocPrefix        string `json:"doc_prefix"`
	DocRunningFormat string `json:"doc_running_format"`
	BranchCode       string `json:"branch_code"`
	SaleCode         string `json:"sale_code"`
	UnitCode         string `json:"unit_code"`
	DocTime          string `json:"doc_time"`
	// Inventory + VAT overrides; empty / -1 = "use server default"
	WHCode    string  `json:"wh_code"`
	ShelfCode string  `json:"shelf_code"`
	VATType   int     `json:"vat_type"`
	VATRate   float64 `json:"vat_rate"`
}
