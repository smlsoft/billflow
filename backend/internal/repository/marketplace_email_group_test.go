package repository

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestMarketplaceEmailGroupCounts(t *testing.T) {
	tests := []struct {
		name         string
		orders       []marketplaceEmailGroupOrderState
		wantResolved int
		wantMissing  int
		wantStatus   string
	}{
		{
			name: "complete when every expected order is created or already exists",
			orders: []marketplaceEmailGroupOrderState{
				{Status: marketplaceEmailGroupOrderCreated},
				{Status: marketplaceEmailGroupOrderExisting},
			},
			wantResolved: 2,
			wantMissing:  0,
			wantStatus:   marketplaceEmailGroupComplete,
		},
		{
			name: "attention when an expected order has not been created",
			orders: []marketplaceEmailGroupOrderState{
				{Status: marketplaceEmailGroupOrderCreated},
				{Status: marketplaceEmailGroupOrderMissing},
			},
			wantResolved: 1,
			wantMissing:  1,
			wantStatus:   marketplaceEmailGroupAttention,
		},
		{
			name:       "attention for an empty expected order set",
			orders:     nil,
			wantStatus: marketplaceEmailGroupAttention,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolved, missing, status := marketplaceEmailGroupCounts(tt.orders)
			if resolved != tt.wantResolved || missing != tt.wantMissing || status != tt.wantStatus {
				t.Fatalf("marketplaceEmailGroupCounts() = (%d, %d, %q), want (%d, %d, %q)", resolved, missing, status, tt.wantResolved, tt.wantMissing, tt.wantStatus)
			}
		})
	}
}

func TestMarketplaceEmailGroupRepoListAttentionIncludesGroupWithoutBill(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("FROM marketplace_email_groups g").
		WithArgs("shopee_shipped", "", 20).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "source", "message_id", "imap_account_id", "imap_mailbox", "subject", "from_addr",
			"status", "expected_order_count", "resolved_order_count", "missing_order_count", "failure_code", "representative_bill_id",
		}).AddRow(
			"8dd5b297-829a-4d7b-a280-50dd78d37b1d", "shopee_shipped", "message@example.test", "", "INBOX", "subject", "from",
			"attention", 3, 0, 3, "ai_extract_failed", "",
		))

	groups, err := NewMarketplaceEmailGroupRepo(db).ListAttention("shopee_shipped", "", 20)
	if err != nil {
		t.Fatalf("ListAttention() error = %v", err)
	}
	if len(groups) != 1 || groups[0].RepresentativeBillID != "" || groups[0].MissingOrderCount != 3 {
		t.Fatalf("ListAttention() = %#v, want unresolved group without representative bill", groups)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}
