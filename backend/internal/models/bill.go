package models

import (
	"encoding/json"
	"time"
)

type Bill struct {
	ID           string          `json:"id"`
	BillType     string          `json:"bill_type"`
	Source       string          `json:"source"`
	Status       string          `json:"status"`
	RawData      json.RawMessage `json:"raw_data,omitempty"`
	SMLDocNo     *string         `json:"sml_doc_no,omitempty"`
	SMLOrderID   string          `json:"sml_order_id,omitempty"`
	SMLPayload   json.RawMessage `json:"sml_payload,omitempty"`
	SMLResponse  json.RawMessage `json:"sml_response,omitempty"`
	AIConfidence *float64        `json:"ai_confidence,omitempty"`
	Anomalies    json.RawMessage `json:"anomalies"`
	ErrorMsg     *string         `json:"error_msg,omitempty"`
	CreatedBy    *string         `json:"created_by,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	SentAt       *time.Time      `json:"sent_at,omitempty"`
	TotalAmount  *float64        `json:"total_amount,omitempty"`
	Remark       string          `json:"remark"`
	Items        []BillItem      `json:"items,omitempty"`
}

type DailyInsight struct {
	ID        string    `json:"id"`
	Date      string    `json:"date"`
	StatsJSON string    `json:"stats_json,omitempty"`
	Insight   string    `json:"insight"`
	CreatedAt time.Time `json:"created_at"`
}

type BillItem struct {
	ID         string          `json:"id"`
	BillID     string          `json:"bill_id"`
	RawName    string          `json:"raw_name"`
	ItemCode   *string         `json:"item_code,omitempty"`
	Qty        float64         `json:"qty"`
	UnitCode   *string         `json:"unit_code,omitempty"`
	Price      *float64        `json:"price,omitempty"`
	Mapped     bool            `json:"mapped"`
	MappingID  *string         `json:"mapping_id,omitempty"`
	Candidates json.RawMessage `json:"candidates,omitempty"` // top-5 catalog matches
}

type BillListFilter struct {
	Status   string `form:"status"`
	Source   string `form:"source"`
	BillType string `form:"bill_type"`
	Search   string `form:"search"`
	Page     int    `form:"page,default=1"`
	PageSize int    `form:"page_size,default=20"`
}

type Anomaly struct {
	Code     string `json:"code"`
	Severity string `json:"severity"` // "block" | "warn"
	Message  string `json:"message"`
}
