// lazada_group_key backfill — sets lazada_confirmed_at and lazada_charge_group_key
// on existing lazada_email purchase bills that predate migration 063.
//
// Usage:
//
//	go run ./cmd/backfill/lazada_group_key/main.go [--dry-run]
//
// Required env var: DATABASE_URL
// Optional env var: ARTIFACTS_DIR (default /app/artifacts)
package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"billflow/internal/database"
	"billflow/internal/handlers"
	"billflow/internal/models"
	"billflow/internal/repository"
	"billflow/internal/services/artifact"

	_ "github.com/lib/pq"
	"go.uber.org/zap"
)

type backfillTarget struct {
	ID             string
	OrderID        string
	EmailMessageID string
	IMAPAccountID  string
	IMAPUsername   string
}

func main() {
	dryRun := flag.Bool("dry-run", false, "print what would change without writing to DB")
	flag.Parse()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL is required")
	}
	artifactsDir := os.Getenv("ARTIFACTS_DIR")
	if artifactsDir == "" {
		artifactsDir = "/app/artifacts"
	}

	db, err := database.Connect(dbURL)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	defer db.Close()

	logger, _ := zap.NewProduction()
	defer logger.Sync()

	artifactRepo := repository.NewBillArtifactRepo(db)
	auditRepo := repository.NewAuditLogRepo(db)
	artifactSvc := artifact.New(artifactsDir, 50*1024*1024, artifactRepo, logger)

	targets, err := listGroupKeyBackfillTargets(db)
	if err != nil {
		log.Fatalf("list targets: %v", err)
	}
	log.Printf("found %d bills without lazada_charge_group_key", len(targets))

	var updated, noArtifact, noMatch int
	for _, t := range targets {
		plainText, bodyHTML, accountID, ok := loadArtifactText(artifactSvc, artifactRepo, t)
		if !ok {
			noArtifact++
			if *dryRun {
				fmt.Printf("[dry-run] bill %s: no artifact found\n", t.ID)
			}
			continue
		}

		confirmedAt, groupKey := handlers.ExtractLazadaConfirmedAt(plainText, bodyHTML, accountID)
		if confirmedAt == "" {
			noMatch++
			if *dryRun {
				fmt.Printf("[dry-run] bill %s (order %s): no Thai date/time match in body\n", t.ID, t.OrderID)
			}
			continue
		}

		if *dryRun {
			fmt.Printf("[dry-run] bill %s (order %s): confirmedAt=%s groupKey=%s\n",
				t.ID, t.OrderID, confirmedAt, groupKey)
			updated++
			continue
		}

		patch := map[string]interface{}{
			"lazada_confirmed_at":     confirmedAt,
			"lazada_charge_group_key": groupKey,
		}
		patchBytes, _ := json.Marshal(patch)
		if _, err := db.Exec(
			`UPDATE bills SET raw_data = raw_data || $1::jsonb WHERE id = $2`,
			string(patchBytes), t.ID,
		); err != nil {
			log.Printf("update bill %s: %v", t.ID, err)
			continue
		}

		billID := t.ID
		_ = auditRepo.Log(models.AuditEntry{
			Action:   "lazada_group_key_backfill",
			TargetID: &billID,
			Source:   "lazada_email",
			Level:    "info",
			Detail: map[string]interface{}{
				"order_id":     t.OrderID,
				"confirmed_at": confirmedAt,
				"group_key":    groupKey,
			},
		})
		updated++
	}

	if *dryRun {
		fmt.Printf("\ndry-run summary: would_update=%d no_artifact=%d no_match=%d\n",
			updated, noArtifact, noMatch)
	} else {
		log.Printf("done: updated=%d no_artifact=%d no_match=%d", updated, noArtifact, noMatch)
	}
}

// listGroupKeyBackfillTargets returns active lazada_email purchase bills
// that do not yet have lazada_charge_group_key in raw_data.
func listGroupKeyBackfillTargets(db *sql.DB) ([]backfillTarget, error) {
	rows, err := db.Query(
		`SELECT id::text,
			        COALESCE(NULLIF(raw_data->>'order_id',''), NULLIF(raw_data->>'lazada_order_id',''), id::text),
			        COALESCE(raw_data->>'email_message_id',''),
			        COALESCE(raw_data->>'imap_account_id',''),
			        COALESCE(raw_data->>'imap_username','')
		   FROM bills
		  WHERE source = 'lazada_email'
		    AND bill_type = 'purchase'
		    AND archived_at IS NULL
		    AND raw_data->>'lazada_charge_group_key' IS NULL
		  ORDER BY created_at ASC, id ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list backfill targets: %w", err)
	}
	defer rows.Close()

	var out []backfillTarget
	for rows.Next() {
		var t backfillTarget
		if err := rows.Scan(&t.ID, &t.OrderID, &t.EmailMessageID, &t.IMAPAccountID, &t.IMAPUsername); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func loadArtifactText(
	svc *artifact.Service,
	artifactRepo *repository.BillArtifactRepo,
	t backfillTarget,
) (plainText, bodyHTML, accountID string, ok bool) {
	artifacts, err := artifactRepo.ListByBill(t.ID)
	if err != nil || len(artifacts) == 0 {
		return "", "", "", false
	}
	emailMessageID := strings.TrimSpace(t.EmailMessageID)

	type candidate struct {
		id   string
		kind string
	}
	var candidates []candidate
	seen := map[string]bool{}
	if emailMessageID != "" {
		for _, a := range artifacts {
			if (a.Kind == "email_html" || a.Kind == "email_text") && lazadaArtifactMsgID(a) == emailMessageID && lazadaArtifactIsConfirmation(a) {
				candidates = append(candidates, candidate{a.ID, a.Kind})
				seen[a.ID] = true
			}
		}
	}
	for _, a := range artifacts {
		if (a.Kind == "email_html" || a.Kind == "email_text") && lazadaArtifactIsConfirmation(a) && !seen[a.ID] {
			candidates = append(candidates, candidate{a.ID, a.Kind})
			seen[a.ID] = true
		}
	}
	if emailMessageID != "" {
		for _, a := range artifacts {
			if (a.Kind == "email_html" || a.Kind == "email_text") && lazadaArtifactMsgID(a) == emailMessageID && !seen[a.ID] {
				candidates = append(candidates, candidate{a.ID, a.Kind})
				seen[a.ID] = true
			}
		}
	}
	for _, a := range artifacts {
		if (a.Kind == "email_html" || a.Kind == "email_text") && !seen[a.ID] {
			candidates = append(candidates, candidate{a.ID, a.Kind})
			seen[a.ID] = true
		}
	}
	if len(candidates) == 0 {
		return "", "", "", false
	}
	accountID = strings.TrimSpace(t.IMAPAccountID)
	if accountID == "" {
		accountID = strings.TrimSpace(t.IMAPUsername)
	}
	for _, a := range artifacts {
		if accountID == "" {
			if id := lazadaArtifactAccountID(a); id != "" {
				accountID = id
				break
			}
		}
	}
	for _, c := range candidates {
		data, _, err := svc.Read(c.id)
		if err != nil || len(data) == 0 {
			continue
		}
		switch c.kind {
		case "email_html":
			bodyHTML = string(data)
		case "email_text":
			plainText = string(data)
		}
		if plainText != "" || bodyHTML != "" {
			return plainText, bodyHTML, accountID, true
		}
	}
	return "", "", "", false
}

func lazadaArtifactIsConfirmation(a models.BillArtifact) bool {
	subject := lazadaArtifactSubject(a)
	return strings.Contains(subject, "ยืนยันคำสั่งซื้อ")
}

func lazadaArtifactSubject(a models.BillArtifact) string {
	if len(a.SourceMeta) == 0 {
		return ""
	}
	var meta struct {
		Subject string `json:"subject"`
	}
	if err := json.Unmarshal(a.SourceMeta, &meta); err != nil {
		return ""
	}
	return strings.TrimSpace(meta.Subject)
}

func lazadaArtifactMsgID(a models.BillArtifact) string {
	if len(a.SourceMeta) == 0 {
		return ""
	}
	var meta struct {
		MessageID string `json:"message_id"`
	}
	if err := json.Unmarshal(a.SourceMeta, &meta); err != nil {
		return ""
	}
	return strings.TrimSpace(meta.MessageID)
}

func lazadaArtifactAccountID(a models.BillArtifact) string {
	if len(a.SourceMeta) == 0 {
		return ""
	}
	var meta struct {
		AccountID string `json:"account_id"`
	}
	if err := json.Unmarshal(a.SourceMeta, &meta); err != nil {
		return ""
	}
	return strings.TrimSpace(meta.AccountID)
}
