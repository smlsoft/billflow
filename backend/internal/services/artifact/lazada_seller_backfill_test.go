package artifact

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"go.uber.org/zap"

	"billflow/internal/repository"
)

func TestBackfillLazadaEmailSellerNamesUpdatesMismatchAndSkipsSafeCases(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	root := t.TempDir()
	billRepo := repository.NewBillRepo(db)
	artifactRepo := repository.NewBillArtifactRepo(db)
	auditRepo := repository.NewAuditLogRepo(db)
	svc := New(root, 10*1024*1024, artifactRepo, zap.NewNop())

	billUpdate := "11111111-1111-1111-1111-111111111111"
	billSkip := "22222222-2222-2222-2222-222222222222"
	billMissing := "33333333-3333-3333-3333-333333333333"
	matched := writeArtifactFile(t, root, billUpdate, "matched.html", `<td>จัดจำหน่ายโดย: CCC Sports</td><td>วันและเวลาจัดส่ง</td>`)
	oldDuplicate := writeArtifactFile(t, root, billUpdate, "old.html", `<td>จัดจำหน่ายโดย: Wrong Store</td>`)
	skipArtifact := writeArtifactFile(t, root, billSkip, "skip.html", `<td>จัดจำหน่ายโดย: Mostna Store</td>`)

	mock.ExpectQuery("SELECT id::text,").
		WillReturnRows(sqlmock.NewRows([]string{"id", "order_id", "email_message_id", "seller_name"}).
			AddRow(billUpdate, "1100887410295692", "matched-message", "Lazada Thailand").
			AddRow(billSkip, "1100887409895692", "skip-message", "Mostna Store").
			AddRow(billMissing, "1100887410095692", "missing-message", "Lazada Thailand"))

	mock.ExpectQuery("FROM bill_artifacts WHERE bill_id =").
		WithArgs(billUpdate).
		WillReturnRows(artifactRows(
			artifactFixture{id: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", billID: billUpdate, kind: "email_html", path: oldDuplicate.path, size: oldDuplicate.size, sha: oldDuplicate.sha, messageID: "old-message"},
			artifactFixture{id: "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb", billID: billUpdate, kind: "email_html", path: matched.path, size: matched.size, sha: matched.sha, messageID: "matched-message"},
		))
	mock.ExpectQuery("FROM bill_artifacts WHERE id =").
		WithArgs("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb").
		WillReturnRows(artifactRows(
			artifactFixture{id: "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb", billID: billUpdate, kind: "email_html", path: matched.path, size: matched.size, sha: matched.sha, messageID: "matched-message"},
		))
	mock.ExpectQuery("WITH current AS").
		WithArgs(billUpdate, "CCC Sports").
		WillReturnRows(sqlmock.NewRows([]string{"old_seller"}).AddRow("Lazada Thailand"))
	mock.ExpectExec("INSERT INTO audit_logs").
		WillReturnResult(sqlmock.NewResult(1, 1))

	mock.ExpectQuery("FROM bill_artifacts WHERE bill_id =").
		WithArgs(billSkip).
		WillReturnRows(artifactRows(
			artifactFixture{id: "cccccccc-cccc-cccc-cccc-cccccccccccc", billID: billSkip, kind: "email_html", path: skipArtifact.path, size: skipArtifact.size, sha: skipArtifact.sha, messageID: "skip-message"},
		))
	mock.ExpectQuery("FROM bill_artifacts WHERE id =").
		WithArgs("cccccccc-cccc-cccc-cccc-cccccccccccc").
		WillReturnRows(artifactRows(
			artifactFixture{id: "cccccccc-cccc-cccc-cccc-cccccccccccc", billID: billSkip, kind: "email_html", path: skipArtifact.path, size: skipArtifact.size, sha: skipArtifact.sha, messageID: "skip-message"},
		))

	mock.ExpectQuery("FROM bill_artifacts WHERE bill_id =").
		WithArgs(billMissing).
		WillReturnRows(artifactRows())

	stats, err := svc.BackfillLazadaEmailSellerNames(billRepo, auditRepo)
	if err != nil {
		t.Fatalf("BackfillLazadaEmailSellerNames: %v", err)
	}
	if stats.Scanned != 3 || stats.Updated != 1 || stats.MissingArtifact != 1 || stats.MissingSeller != 0 || stats.ReadErrors != 0 {
		t.Fatalf("stats = %+v", stats)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

type artifactFile struct {
	path string
	size int64
	sha  string
}

func writeArtifactFile(t *testing.T, root, billID, name, content string) artifactFile {
	t.Helper()
	rel := filepath.Join("2026", "06", billID, name)
	abs := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatalf("mkdir artifact dir: %v", err)
	}
	data := []byte(content)
	if err := os.WriteFile(abs, data, 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}
	sum := sha256.Sum256(data)
	return artifactFile{path: rel, size: int64(len(data)), sha: hex.EncodeToString(sum[:])}
}

type artifactFixture struct {
	id        string
	billID    string
	kind      string
	path      string
	size      int64
	sha       string
	messageID string
}

func artifactRows(fixtures ...artifactFixture) *sqlmock.Rows {
	rows := sqlmock.NewRows([]string{
		"id", "bill_id", "kind", "filename", "content_type", "size_bytes", "sha256", "storage_path", "source_meta", "created_at",
	})
	for _, f := range fixtures {
		rows.AddRow(
			f.id,
			f.billID,
			f.kind,
			"lazada-email.html",
			"text/html; charset=utf-8",
			f.size,
			f.sha,
			f.path,
			fmt.Sprintf(`{"message_id":%q}`, f.messageID),
			time.Now(),
		)
	}
	return rows
}
