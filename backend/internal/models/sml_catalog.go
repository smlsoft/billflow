package models

import (
	"time"
)

// CatalogItem represents one row in sml_catalog
type CatalogItem struct {
	ItemCode        string     `json:"item_code"`
	ItemName        string     `json:"item_name"`
	ItemName2       string     `json:"item_name2"`
	UnitCode        string     `json:"unit_code"`
	WHCode          string     `json:"wh_code"`
	ShelfCode       string     `json:"shelf_code"`
	Price           *float64   `json:"price"`
	GroupCode       string     `json:"group_code"`
	BalanceQty      *float64   `json:"balance_qty"`
	EmbeddingStatus string     `json:"embedding_status"` // pending | done | error
	EmbeddedAt      *time.Time `json:"embedded_at"`
	SyncedAt        time.Time  `json:"synced_at"`
	CreatedAt       time.Time  `json:"created_at"`
}

// CatalogMatch is one similarity search result
type CatalogMatch struct {
	ItemCode  string  `json:"item_code"`
	ItemName  string  `json:"item_name"`
	ItemName2 string  `json:"item_name2"`
	UnitCode  string  `json:"unit_code"`
	WHCode    string  `json:"wh_code"`
	ShelfCode string  `json:"shelf_code"`
	Price     float64 `json:"price"`
	Score     float64 `json:"score"` // cosine similarity 0–1
}
