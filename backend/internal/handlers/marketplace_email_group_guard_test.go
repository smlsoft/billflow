package handlers

import (
	"net/http"
	"testing"

	"billflow/internal/config"
	"billflow/internal/models"
	"billflow/internal/repository"
	"github.com/DATA-DOG/go-sqlmock"
)

func TestGuardMarketplaceEmailGroupCompletenessBlocksStaff(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	const messageID = "message@example.test"
	const groupID = "8dd5b297-829a-4d7b-a280-50dd78d37b1d"
	mock.ExpectQuery("FROM marketplace_email_groups").
		WithArgs("shopee_shipped", messageID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "source", "message_id", "imap_account_id", "imap_mailbox", "subject", "from_addr",
			"status", "expected_order_count", "resolved_order_count", "missing_order_count", "failure_code",
		}).AddRow(groupID, "shopee_shipped", messageID, "", "INBOX", "subject", "from", "attention", 3, 2, 1, "order_processing_failed"))
	mock.ExpectQuery("FROM marketplace_email_group_orders").
		WithArgs(groupID).
		WillReturnRows(sqlmock.NewRows([]string{"order_id", "bill_id", "status", "error_code"}).
			AddRow("260725KGR6PUJ7", "", "created", "").
			AddRow("260725KGR6PUJ8", "", "created", "").
			AddRow("260725KGR6PUJ9", "", "missing", "order_processing_failed"))

	h := &BillHandler{
		cfg:            &config.Config{EnforceEmailGroups: true},
		emailGroupRepo: repository.NewMarketplaceEmailGroupRepo(db),
	}
	bill := &models.Bill{
		ID:       "b5d5b297-829a-4d7b-a280-50dd78d37b1d",
		Source:   "shopee_shipped",
		BillType: "purchase",
		EmailGroup: &models.BillEmailGroup{
			MessageID: messageID,
		},
	}

	result := h.guardMarketplaceEmailGroupCompleteness(bill, RetryRequest{}, retrySendOptions{UserRole: "staff", Via: "retry"})
	if result == nil {
		t.Fatal("guard result = nil, want block")
	}
	if result.HTTPStatus != http.StatusConflict {
		t.Fatalf("status = %d, want %d", result.HTTPStatus, http.StatusConflict)
	}
	if result.Error == "" {
		t.Fatal("expected a user-facing incomplete email group error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}
