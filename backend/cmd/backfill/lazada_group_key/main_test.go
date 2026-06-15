package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"billflow/internal/repository"
	"billflow/internal/services/artifact"
	"github.com/DATA-DOG/go-sqlmock"
	"go.uber.org/zap"
)

func TestLoadArtifactTextPrefersMatchingConfirmationArtifactAndRawAccount(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	root := t.TempDir()
	billID := "11111111-1111-1111-1111-111111111111"
	shippingPath := filepath.Join("2026", "06", billID, "shipping.html")
	confirmPath := filepath.Join("2026", "06", billID, "confirm.html")
	for _, rel := range []string{shippingPath, confirmPath} {
		if err := os.MkdirAll(filepath.Join(root, filepath.Dir(rel)), 0o755); err != nil {
			t.Fatalf("mkdir artifact dir: %v", err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, shippingPath), []byte("คำสั่งซื้อหมายเลข 1109337756795692 ได้รับการจัดส่งเรียบร้อยแล้ว"), 0o644); err != nil {
		t.Fatalf("write shipping artifact: %v", err)
	}
	confirmHTML := "เราได้รับหมายเลขคำสั่งซื้อ <b>1109337756795692</b> ของคุณเมื่อ 11 มิถุนายน 2569 เวลา 16:45ผ่านช่องทาง"
	if err := os.WriteFile(filepath.Join(root, confirmPath), []byte(confirmHTML), 0o644); err != nil {
		t.Fatalf("write confirmation artifact: %v", err)
	}

	artifactRows := sqlmock.NewRows([]string{
		"id", "bill_id", "kind", "filename", "content_type", "size_bytes", "sha256", "storage_path", "source_meta", "created_at",
	}).
		AddRow("ship-artifact", billID, "email_html", "shipping.html", "text/html", int64(64), "", shippingPath, `{"subject":"คำสั่งซื้อหมายเลข 1109337756795692 ได้รับการจัดส่งเรียบร้อยแล้ว","message_id":"msg-shipping","account_id":"artifact-account"}`, time.Now()).
		AddRow("confirm-artifact", billID, "email_html", "confirm.html", "text/html", int64(len(confirmHTML)), "", confirmPath, `{"subject":"ยืนยันคำสั่งซื้อหมายเลข 1109337756795692","message_id":"msg-confirm","account_id":"artifact-account"}`, time.Now())
	mock.ExpectQuery("FROM bill_artifacts WHERE bill_id").
		WithArgs(billID).
		WillReturnRows(artifactRows)
	mock.ExpectQuery("FROM bill_artifacts WHERE id").
		WithArgs("confirm-artifact").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "bill_id", "kind", "filename", "content_type", "size_bytes", "sha256", "storage_path", "source_meta", "created_at",
		}).AddRow("confirm-artifact", billID, "email_html", "confirm.html", "text/html", int64(len(confirmHTML)), "", confirmPath, `{"subject":"ยืนยันคำสั่งซื้อหมายเลข 1109337756795692","message_id":"msg-confirm","account_id":"artifact-account"}`, time.Now()))

	repo := repository.NewBillArtifactRepo(db)
	svc := artifact.New(root, 50*1024*1024, repo, zap.NewNop())
	plainText, bodyHTML, accountID, ok := loadArtifactText(svc, repo, backfillTarget{
		ID:             billID,
		OrderID:        "1109337756795692",
		EmailMessageID: "msg-confirm",
		IMAPAccountID:  "raw-account",
	})
	if !ok {
		t.Fatal("expected artifact text")
	}
	if plainText != "" {
		t.Fatalf("plainText = %q, want empty", plainText)
	}
	if !strings.Contains(bodyHTML, "เวลา 16:45") {
		t.Fatalf("bodyHTML = %q, want confirmation body", bodyHTML)
	}
	if accountID != "raw-account" {
		t.Fatalf("accountID = %q, want raw-account", accountID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}
