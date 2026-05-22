package repository

import (
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestRecordEmailPrintEventUsesArtifactSourceMeta(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	repo := NewBillRepo(db)
	billID := "768a0068-cad3-4b6e-b229-a5d2ce2ede73"
	artifactID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	userID := "11111111-1111-1111-1111-111111111111"

	mock.ExpectQuery("SELECT COALESCE").
		WithArgs(billID, artifactID).
		WillReturnRows(sqlmock.NewRows([]string{"message_id", "subject", "from_addr", "kind"}).
			AddRow("artifact-message@example.test", "artifact subject", "artifact sender", "email_html"))
	mock.ExpectQuery("INSERT INTO email_print_events").
		WithArgs(billID, artifactID, "artifact-message@example.test", emailGroupKey("artifact-message@example.test"), "artifact subject", "artifact sender", userID, "admin@example.test").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "bill_id", "artifact_id", "email_message_id", "email_group_key",
			"subject", "from_addr", "requested_by", "requested_by_email", "created_at",
		}).AddRow(
			"22222222-2222-2222-2222-222222222222", billID, artifactID, "artifact-message@example.test",
			emailGroupKey("artifact-message@example.test"), "artifact subject", "artifact sender", userID, "admin@example.test", time.Now(),
		))

	event, err := repo.RecordEmailPrintEvent(billID, artifactID, userID, "admin@example.test")
	if err != nil {
		t.Fatalf("RecordEmailPrintEvent: %v", err)
	}
	if event == nil {
		t.Fatal("expected print event")
	}
	if event.EmailMessageID != "artifact-message@example.test" || event.Subject != "artifact subject" || event.From != "artifact sender" {
		t.Fatalf("event used wrong metadata: %#v", event)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}
