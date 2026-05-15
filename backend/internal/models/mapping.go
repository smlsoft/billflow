package models

import "time"

type Mapping struct {
	ID                string     `json:"id"`
	RawName           string     `json:"raw_name"`
	ItemCode          string     `json:"item_code"`
	UnitCode          string     `json:"unit_code"`
	Confidence        float64    `json:"confidence"`
	Source            string     `json:"source"`
	UsageCount        int        `json:"usage_count"`
	LastUsedAt        *time.Time `json:"last_used_at,omitempty"`
	LearnedFromBillID *string    `json:"learned_from_bill_id,omitempty"`
	CreatedBy         *string    `json:"created_by,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
}

type MappingFeedback struct {
	ID            string    `json:"id"`
	BillItemID    string    `json:"bill_item_id"`
	OriginalMatch *string   `json:"original_match,omitempty"`
	CorrectedTo   string    `json:"corrected_to"`
	CorrectedBy   string    `json:"corrected_by"`
	CreatedAt     time.Time `json:"created_at"`
}

type CreateMappingRequest struct {
	RawName  string `json:"raw_name" binding:"required"`
	ItemCode string `json:"item_code" binding:"required"`
	UnitCode string `json:"unit_code" binding:"required"`
}

type MatchResult struct {
	Mapping     *Mapping
	Score       float64
	NeedsReview bool
	Unmapped    bool
}

type MarketplaceItemAlias struct {
	ID            string     `json:"id"`
	Source        string     `json:"source"`
	SourceSKU     string     `json:"source_sku"`
	RawName       string     `json:"raw_name"`
	NormalizedKey string     `json:"normalized_key"`
	ItemCode      string     `json:"item_code"`
	UnitCode      string     `json:"unit_code"`
	Confidence    float64    `json:"confidence"`
	ConfirmedBy   *string    `json:"confirmed_by,omitempty"`
	UsageCount    int        `json:"usage_count"`
	LastUsedAt    *time.Time `json:"last_used_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

type MarketplaceAliasReviewGroup struct {
	GroupKey       string         `json:"group_key"`
	Source         string         `json:"source"`
	BillType       string         `json:"bill_type"`
	SourceSKU      string         `json:"source_sku"`
	RawName        string         `json:"raw_name"`
	NormalizedKey  string         `json:"normalized_key"`
	BillCount      int            `json:"bill_count"`
	ItemCount      int            `json:"item_count"`
	SuggestedMatch *CatalogMatch  `json:"suggested_match,omitempty"`
	Candidates     []CatalogMatch `json:"candidates,omitempty"`
}

// PlatformColumnMapping maps a logical field name to the actual Excel column name
// per platform (lazada | shopee). Stored in platform_column_mappings table.
type PlatformColumnMapping struct {
	ID         string    `json:"id"`
	Platform   string    `json:"platform"`
	FieldName  string    `json:"field_name"`
	ColumnName string    `json:"column_name"`
	UpdatedAt  time.Time `json:"updated_at"`
}
