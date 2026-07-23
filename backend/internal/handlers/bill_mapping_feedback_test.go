package handlers

import (
	"testing"

	"billflow/internal/models"
)

func TestItemMappingFeedbackPlanKeepsDuplicateRawNamesLocal(t *testing.T) {
	bill := &models.Bill{
		Source:   "lazada_email",
		BillType: "purchase",
		Items: []models.BillItem{
			{ID: "first", RawName: "กระติก TITAN", Qty: 1},
			{ID: "second", RawName: "กระติก TITAN", Qty: 1},
		},
	}

	plan := itemMappingFeedbackPlan(bill, &bill.Items[0])
	if plan.Scope != itemMappingScopeItemOnly {
		t.Fatalf("scope = %q, want %q", plan.Scope, itemMappingScopeItemOnly)
	}
	if plan.LearnGlobal || plan.ApplyGlobal || plan.UseMarketplaceAlias {
		t.Fatalf("duplicate plan must not write a shared mapping: %+v", plan)
	}
}

func TestItemMappingFeedbackPlanUsesSourceSKUForDuplicateMarketplaceRows(t *testing.T) {
	bill := &models.Bill{
		Source:   "lazada_email",
		BillType: "purchase",
		Items: []models.BillItem{
			{ID: "green", RawName: "กระติก TITAN", SourceSKU: "127271408131", Qty: 1},
			{ID: "blue", RawName: "กระติก TITAN", SourceSKU: "127271408130", Qty: 1},
		},
	}

	plan := itemMappingFeedbackPlan(bill, &bill.Items[0])
	if plan.Scope != itemMappingScopeSourceSKU || !plan.UseMarketplaceAlias {
		t.Fatalf("plan = %+v, want source_sku alias", plan)
	}
	if plan.AliasSource != "lazada" {
		t.Fatalf("alias source = %q, want lazada", plan.AliasSource)
	}
}

func TestItemMappingFeedbackPlanKeepsMarketplaceRowLocalWithoutSourceSKU(t *testing.T) {
	bill := &models.Bill{
		Source:   "lazada_email",
		BillType: "purchase",
		Items: []models.BillItem{
			{ID: "row", RawName: "กระติก TITAN", Qty: 1},
		},
	}

	plan := itemMappingFeedbackPlan(bill, &bill.Items[0])
	if plan.Scope != itemMappingScopeItemOnly || plan.LearnGlobal || plan.ApplyGlobal || plan.UseMarketplaceAlias {
		t.Fatalf("plan = %+v, want an item-only marketplace update", plan)
	}
}

func TestMarketplaceDuplicateItemCodeWarningsRequireConfirmation(t *testing.T) {
	bill := &models.Bill{
		Source:   "lazada_email",
		BillType: "purchase",
		Items: []models.BillItem{
			{ID: "green", RawName: "กระติก TITAN", SourceSKU: "127271408131", ItemCode: stringPtr("KC0116-BU")},
			{ID: "blue", RawName: "กระติก TITAN", SourceSKU: "127271408130", ItemCode: stringPtr("KC0116-BU")},
			{ID: "fee", RawName: "ค่าขนส่งบิลซื้อ", SourceSKU: models.LazadaFeeSourceSKU, ItemCode: stringPtr("SHIP_POL")},
		},
	}

	warnings := marketplaceDuplicateItemCodeWarnings(bill)
	if len(warnings) != 1 {
		t.Fatalf("warnings = %+v, want one", warnings)
	}
	if warnings[0].ItemCode != "KC0116-BU" || warnings[0].Count != 2 {
		t.Fatalf("warning = %+v", warnings[0])
	}

	*bill.Items[1].ItemCode = "KC0116-GR"
	if got := marketplaceDuplicateItemCodeWarnings(bill); len(got) != 0 {
		t.Fatalf("warnings = %+v, want none when codes differ", got)
	}
}

func stringPtr(value string) *string { return &value }
