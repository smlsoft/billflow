package handlers

import (
	"strings"

	"billflow/internal/models"
)

type marketplaceCatalogLookup func(code string) *models.CatalogItem

func normalizeMarketplaceSKU(sku string) string {
	sku = strings.ReplaceAll(sku, "\ufeff", "")
	sku = strings.TrimSpace(sku)
	if strings.EqualFold(sku, "nan") || strings.EqualFold(sku, "null") || sku == "-" {
		return ""
	}
	return sku
}

func marketplaceBillItemFromMatch(
	rawName string,
	sourceSKU string,
	qty float64,
	price *float64,
	defaultUnit string,
	learned *models.Mapping,
	matches []models.CatalogMatch,
	lookup marketplaceCatalogLookup,
	highConfThreshold float64,
) (models.BillItem, bool) {
	sourceSKU = normalizeMarketplaceSKU(sourceSKU)
	bi := models.BillItem{
		RawName:   rawName,
		SourceSKU: sourceSKU,
		Qty:       qty,
		Price:     price,
	}

	if sourceSKU != "" && lookup != nil {
		if cat := lookup(sourceSKU); cat != nil {
			code := cat.ItemCode
			unit := cat.UnitCode
			if unit == "" {
				unit = defaultUnit
			}
			bi.ItemCode = &code
			bi.UnitCode = &unit
			bi.Mapped = true
			return bi, true
		}
	}

	switch {
	case learned != nil:
		bi.ItemCode = &learned.ItemCode
		bi.UnitCode = &learned.UnitCode
		bi.MappingID = &learned.ID
		bi.Mapped = true
		return bi, true
	case len(matches) > 0 && matches[0].Score >= highConfThreshold:
		bi.ItemCode = &matches[0].ItemCode
		unit := matches[0].UnitCode
		if unit == "" {
			unit = defaultUnit
		}
		bi.UnitCode = &unit
		bi.Mapped = true
		return bi, true
	default:
		if len(matches) > 0 {
			bi.ItemCode = &matches[0].ItemCode
			unit := matches[0].UnitCode
			if unit == "" {
				unit = defaultUnit
			}
			bi.UnitCode = &unit
		}
		bi.Mapped = false
		return bi, false
	}
}
