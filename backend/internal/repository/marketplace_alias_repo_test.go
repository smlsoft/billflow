package repository

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestMarketplaceAliasMatchesRowDoesNotFallbackToRawNameWhenSourceSKUPresent(t *testing.T) {
	if marketplaceAliasMatchesRow("127271408131", "", "กระติก TITAN", "127271408130", "กระติก TITAN") {
		t.Fatal("different source SKU with same raw name must not match")
	}
	if !marketplaceAliasMatchesRow("127271408131", "", "กระติก TITAN", "127271408131", "กระติก TITAN") {
		t.Fatal("same source SKU should match")
	}
}

func TestMarketplaceAliasFindWithSourceSKUNeverFallsBackToRawName(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	repo := NewMarketplaceAliasRepo(db)
	rawName := "กระติก TITAN"
	sourceSKU := "127271408131"
	mock.ExpectQuery("WHERE source = \\$1 AND source_sku = \\$2").
		WithArgs("lazada", sourceSKU).
		WillReturnRows(sqlmock.NewRows(aliasColumns()))

	alias, err := repo.Find("lazada", sourceSKU, rawName)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if alias != nil {
		t.Fatalf("alias = %+v, want nil", alias)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func aliasColumns() []string {
	return []string{
		"id", "source", "source_sku", "raw_name", "normalized_key", "item_code", "unit_code",
		"confidence", "confirmed_by", "usage_count", "last_used_at", "created_at", "updated_at",
	}
}
