package repository

import (
	"strings"
	"testing"

	"billflow/internal/models"
)

func TestBillWhereDefaultsToActiveDocuments(t *testing.T) {
	where, _, _ := billWhere(models.BillListFilter{})
	if !strings.Contains(where, "b.archived_at IS NULL") {
		t.Fatalf("default where = %q, want active archived filter", where)
	}
}

func TestBillWhereArchivedModes(t *testing.T) {
	where, _, _ := billWhere(models.BillListFilter{Archived: "include"})
	if strings.Contains(where, "archived_at") {
		t.Fatalf("include where = %q, should not constrain archived_at", where)
	}

	where, _, _ = billWhere(models.BillListFilter{Archived: "only"})
	if !strings.Contains(where, "b.archived_at IS NOT NULL") {
		t.Fatalf("only where = %q, want archived_at IS NOT NULL", where)
	}
}

func TestBillWhereDateAndShopeeStatusFilters(t *testing.T) {
	where, args, _ := billWhere(models.BillListFilter{
		DateFrom:     "2026-05-01",
		DateTo:       "2026-05-18",
		ShopeeStatus: "shipped",
	})
	for _, want := range []string{"b.created_at >= $", "b.created_at < ($", "shopee_order_events", "soe.bill_id = b.id"} {
		if !strings.Contains(where, want) {
			t.Fatalf("where = %q, missing %q", where, want)
		}
	}
	if len(args) != 3 {
		t.Fatalf("args len = %d, want 3", len(args))
	}
}
