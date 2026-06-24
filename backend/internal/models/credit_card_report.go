package models

import "time"

type CreditCardReportFilter struct {
	DateFrom          string `form:"date_from" json:"date_from"`
	DateTo            string `form:"date_to" json:"date_to"`
	PaymentMethod     string `form:"payment_method" json:"payment_method"`
	Source            string `form:"source" json:"source"`
	IncludeIncomplete bool   `form:"include_incomplete" json:"include_incomplete"`
}

type CreditCardReportPreview struct {
	Filters       CreditCardReportFilter  `json:"filters"`
	Groups        []CreditCardReportGroup `json:"groups"`
	Summary       CreditCardReportSummary `json:"summary"`
	Limit         int                     `json:"limit"`
	Truncated     bool                    `json:"truncated"`
	GeneratedAt   time.Time               `json:"generated_at"`
	SelectedGroup []string                `json:"selected_group_ids,omitempty"`
}

type CreditCardReportSummary struct {
	GroupCount           int     `json:"group_count"`
	OrderCount           int     `json:"order_count"`
	SelectedCount        int     `json:"selected_count,omitempty"`
	ChargeTotal          float64 `json:"charge_total"`
	OrderTotal           float64 `json:"order_total"`
	IssueGroupCount      int     `json:"issue_group_count"`
	MissingPOLCount      int     `json:"missing_pol_count"`
	MissingCharge        int     `json:"missing_charge_count"`
	ReadyPrintGroups     int     `json:"ready_print_groups"`
	AmountMismatchCount  int     `json:"amount_mismatch_count"`
	RepairCandidateCount int     `json:"repair_candidate_count"`
	IncompleteOnlyCount  int     `json:"incomplete_only_count"`
	SmallDiffCount       int     `json:"small_diff_count"`
}

type CreditCardReportGroup struct {
	GroupID                    string                          `json:"group_id"`
	Source                     string                          `json:"source"`
	SourceLabel                string                          `json:"source_label"`
	ChargeTime                 string                          `json:"charge_time"`
	ChargeDate                 string                          `json:"charge_date"`
	SortTime                   time.Time                       `json:"sort_time"`
	PaymentMethods             []string                        `json:"payment_methods"`
	ChargeAmount               *float64                        `json:"charge_amount,omitempty"`
	OrderTotal                 float64                         `json:"order_total"`
	Diff                       *float64                        `json:"diff,omitempty"`
	OrderCount                 int                             `json:"order_count"`
	POLCount                   int                             `json:"pol_count"`
	SentCount                  int                             `json:"sent_count"`
	PrintableCount             int                             `json:"printable_count"`
	PrintReady                 bool                            `json:"print_ready"`
	PrintBlockReason           string                          `json:"print_block_reason,omitempty"`
	DiagnosisCategory          string                          `json:"diagnosis_category,omitempty"`
	DiagnosisTitle             string                          `json:"diagnosis_title,omitempty"`
	DiagnosisDetail            string                          `json:"diagnosis_detail,omitempty"`
	RecommendedAction          string                          `json:"recommended_action,omitempty"`
	RepairBillID               string                          `json:"repair_bill_id,omitempty"`
	DetectedEmailOrderCount    int                             `json:"detected_email_order_count,omitempty"`
	ActiveBillOrderCount       int                             `json:"active_bill_order_count,omitempty"`
	EstimatedMissingOrderCount int                             `json:"estimated_missing_order_count,omitempty"`
	Issues                     []CreditCardReportIssue         `json:"issues"`
	Orders                     []CreditCardReportOrder         `json:"orders"`
	PrintArtifacts             []CreditCardReportPrintArtifact `json:"print_artifacts,omitempty"`
}

type CreditCardReportOrder struct {
	BillID                      string  `json:"bill_id"`
	OrderID                     string  `json:"order_id"`
	SellerName                  string  `json:"seller_name"`
	SMLDocNo                    string  `json:"sml_doc_no"`
	Status                      string  `json:"status"`
	PrintPaymentMethod          string  `json:"print_payment_method,omitempty"`
	EffectivePrintPaymentMethod string  `json:"effective_print_payment_method,omitempty"`
	OrderTotal                  float64 `json:"order_total"`
	DocRef                      string  `json:"doc_ref,omitempty"`
	EmailMessageID              string  `json:"email_message_id,omitempty"`
	CreatedAt                   string  `json:"created_at,omitempty"`
}

type CreditCardReportIssue struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

type CreditCardReportPrintArtifact struct {
	MessageID  string                              `json:"message_id"`
	BillID     string                              `json:"bill_id"`
	ArtifactID string                              `json:"artifact_id"`
	Filename   string                              `json:"filename"`
	Orders     []CreditCardReportPrintOrderContext `json:"orders"`
}

type CreditCardReportPrintOrderContext struct {
	OrderID       string `json:"order_id,omitempty"`
	SMLDocNo      string `json:"sml_doc_no,omitempty"`
	PartyName     string `json:"party_name,omitempty"`
	PaymentMethod string `json:"payment_method,omitempty"`
}

type CreditCardReportRun struct {
	ID               string                  `json:"id"`
	ReportName       string                  `json:"report_name"`
	Filters          CreditCardReportFilter  `json:"filters"`
	SelectedGroupIDs []string                `json:"selected_group_ids"`
	Snapshot         CreditCardReportPreview `json:"snapshot"`
	Summary          CreditCardReportSummary `json:"summary"`
	CreatedBy        string                  `json:"created_by,omitempty"`
	CreatedByEmail   string                  `json:"created_by_email,omitempty"`
	ExportedAt       *time.Time              `json:"exported_at,omitempty"`
	PrintedAt        *time.Time              `json:"printed_at,omitempty"`
	PrintSummary     map[string]interface{}  `json:"print_summary,omitempty"`
	CreatedAt        time.Time               `json:"created_at"`
	UpdatedAt        time.Time               `json:"updated_at"`
}
