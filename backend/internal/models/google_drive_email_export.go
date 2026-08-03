package models

import "time"

// GoogleDriveEmailExport is one immutable file-copy task. The target naming
// fields are snapshotted at queue time so later edits cannot silently move an
// already approved document to another Drive location.
type GoogleDriveEmailExport struct {
	ID                 string     `json:"id"`
	BillID             string     `json:"bill_id"`
	SourceArtifactID   string     `json:"source_artifact_id,omitempty"`
	SourceSHA256       string     `json:"source_sha256,omitempty"`
	SourceContentType  string     `json:"source_content_type,omitempty"`
	SourceFilename     string     `json:"source_filename,omitempty"`
	SourceChannel      string     `json:"source_channel"`
	OrderDate          string     `json:"order_date"`
	PaymentToken       string     `json:"payment_token"`
	SMLDocNo           string     `json:"sml_doc_no"`
	MarketplaceOrderID string     `json:"marketplace_order_id"`
	ChargeAmount       string     `json:"charge_amount"`
	RemotePath         string     `json:"remote_path"`
	Status             string     `json:"status"`
	Priority           int        `json:"priority"`
	AttemptCount       int        `json:"attempt_count"`
	NextAttemptAt      time.Time  `json:"next_attempt_at"`
	LastAttemptAt      *time.Time `json:"last_attempt_at,omitempty"`
	StartedAt          *time.Time `json:"started_at,omitempty"`
	UploadedAt         *time.Time `json:"uploaded_at,omitempty"`
	LastError          string     `json:"last_error,omitempty"`
	CreatedBy          *string    `json:"created_by,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

type GoogleDriveEmailExportCounts struct {
	Queued    int `json:"queued"`
	Running   int `json:"running"`
	Succeeded int `json:"succeeded"`
	Failed    int `json:"failed"`
	Conflict  int `json:"conflict"`
}
