package repository

import (
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestMappingRepoListPageSearchesAndEscapesWildcards(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	where := ` WHERE raw_name ILIKE $1 ESCAPE '\' OR item_code ILIKE $1 ESCAPE '\' OR unit_code ILIKE $1 ESCAPE '\'`
	pattern := `%MD3\_100\%%`
	mock.ExpectQuery(`SELECT COUNT(*) FROM mappings` + where).
		WithArgs(pattern).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(26))

	now := time.Now()
	mock.ExpectQuery(`SELECT id, raw_name, item_code, unit_code, confidence, source,
	        usage_count, last_used_at, created_at
	 FROM mappings`+where+` ORDER BY usage_count DESC, raw_name LIMIT $2 OFFSET $3`).
		WithArgs(pattern, 25, 25).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "raw_name", "item_code", "unit_code", "confidence", "source", "usage_count", "last_used_at", "created_at",
		}).AddRow("mapping-1", "iPad", "MD3Y4TH/A", "เครื่อง", 1.0, "manual", 2, nil, now))

	mappings, total, err := NewMappingRepo(db).ListPage(2, 25, `MD3_100%`)
	if err != nil {
		t.Fatal(err)
	}
	if total != 26 || len(mappings) != 1 || mappings[0].ItemCode != "MD3Y4TH/A" {
		t.Fatalf("total=%d mappings=%+v", total, mappings)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
