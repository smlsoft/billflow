package repository

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestGoogleDriveEmailExportRetryResetsAttemptCount(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	jobID := "11111111-1111-1111-1111-111111111111"
	mock.ExpectExec("UPDATE google_drive_email_exports[\\s\\S]*attempt_count = 0").
		WithArgs(jobID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	ok, err := NewGoogleDriveEmailExportRepo(db).Retry(jobID)
	if err != nil {
		t.Fatalf("Retry: %v", err)
	}
	if !ok {
		t.Fatal("Retry returned false, want true")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
