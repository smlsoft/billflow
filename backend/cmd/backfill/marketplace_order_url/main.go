// marketplace_order_url backfill extracts safe Shopee/Lazada order detail URLs
// from stored email artifacts and stores them in bills.raw_data.
//
// Usage:
//
//	go run ./cmd/backfill/marketplace_order_url --dry-run
//	go run ./cmd/backfill/marketplace_order_url --apply
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
	"billflow/internal/models"
	"billflow/internal/repository"
	"billflow/internal/services/artifact"

	_ "github.com/lib/pq"
	"go.uber.org/zap"
)

type backfillTarget struct {
	ID             string
	Source         string
	OrderID        string
	EmailMessageID string
	RawData        json.RawMessage
}

func main() {
	dryRun := flag.Bool("dry-run", false, "print what would change without writing to DB")
	apply := flag.Bool("apply", false, "write extracted marketplace order URLs to DB")
	flag.Parse()
	if *dryRun == *apply {
		log.Fatal("choose exactly one of --dry-run or --apply")
	}

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
	artifactSvc := artifact.New(artifactsDir, 50*1024*1024, artifactRepo, logger)
	auditRepo := repository.NewAuditLogRepo(db)

	targets, err := listBackfillTargets(db)
	if err != nil {
		log.Fatalf("list targets: %v", err)
	}
	log.Printf("found %d marketplace purchase bills without marketplace_order_url", len(targets))

	var wouldUpdate, updated, noArtifact, noMatch, readErrors int
	for _, target := range targets {
		bodyText, bodyHTML, artifactID, ok, err := loadEmailBody(artifactSvc, artifactRepo, target)
		if err != nil {
			readErrors++
			log.Printf("read artifact bill=%s order=%s: %v", target.ID, target.OrderID, err)
			continue
		}
		if !ok {
			noArtifact++
			if *dryRun {
				fmt.Printf("[dry-run] bill %s order %s: no email artifact/raw body\n", target.ID, target.OrderID)
			}
			continue
		}
		orderURL := repository.ExtractMarketplaceOrderURL(target.Source, bodyText, bodyHTML, target.OrderID)
		if orderURL == "" {
			noMatch++
			if *dryRun {
				fmt.Printf("[dry-run] bill %s order %s: no safe marketplace order URL found\n", target.ID, target.OrderID)
			}
			continue
		}
		if *dryRun {
			wouldUpdate++
			fmt.Printf("[dry-run] bill %s order %s: %s\n", target.ID, target.OrderID, orderURL)
			continue
		}

		patch := map[string]interface{}{
			"marketplace_order_url":        orderURL,
			"marketplace_order_url_source": "email_html",
		}
		patchBytes, _ := json.Marshal(patch)
		res, err := db.Exec(
			`UPDATE bills
			    SET raw_data = COALESCE(raw_data, '{}'::jsonb) || $1::jsonb
			  WHERE id = $2
			    AND archived_at IS NULL
			    AND COALESCE(raw_data->>'marketplace_order_url', '') = ''`,
			string(patchBytes), target.ID,
		)
		if err != nil {
			log.Printf("update bill %s order %s: %v", target.ID, target.OrderID, err)
			continue
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			continue
		}
		updated++
		billID := target.ID
		_ = auditRepo.Log(models.AuditEntry{
			Action:   "marketplace_order_url_backfill",
			TargetID: &billID,
			Source:   target.Source,
			Level:    "info",
			Detail: map[string]interface{}{
				"order_id":    target.OrderID,
				"artifact_id": artifactID,
				"url_source":  "email_html",
			},
		})
	}

	if *dryRun {
		fmt.Printf("\ndry-run summary: would_update=%d no_artifact=%d no_match=%d read_errors=%d\n",
			wouldUpdate, noArtifact, noMatch, readErrors)
	} else {
		log.Printf("done: updated=%d no_artifact=%d no_match=%d read_errors=%d",
			updated, noArtifact, noMatch, readErrors)
	}
}

func listBackfillTargets(db *sql.DB) ([]backfillTarget, error) {
	rows, err := db.Query(`
		SELECT id::text,
		       source,
		       COALESCE(NULLIF(raw_data->>'order_id',''), NULLIF(raw_data->>'shopee_order_id',''), NULLIF(raw_data->>'lazada_order_id',''), sml_order_id, '') AS order_id,
		       COALESCE(raw_data->>'email_message_id', '') AS email_message_id,
		       COALESCE(raw_data, '{}'::jsonb) AS raw_data
		  FROM bills
		 WHERE source IN ('shopee_shipped', 'lazada_email')
		   AND bill_type = 'purchase'
		   AND archived_at IS NULL
		   AND COALESCE(raw_data->>'marketplace_order_url', '') = ''
		 ORDER BY created_at ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []backfillTarget
	for rows.Next() {
		var target backfillTarget
		if err := rows.Scan(&target.ID, &target.Source, &target.OrderID, &target.EmailMessageID, &target.RawData); err != nil {
			return nil, err
		}
		target.OrderID = strings.TrimSpace(target.OrderID)
		if target.OrderID == "" {
			continue
		}
		out = append(out, target)
	}
	return out, rows.Err()
}

func loadEmailBody(
	svc *artifact.Service,
	repo *repository.BillArtifactRepo,
	target backfillTarget,
) (bodyText, bodyHTML, artifactID string, ok bool, err error) {
	artifacts, err := repo.ListByBill(target.ID)
	if err != nil {
		return "", "", "", false, err
	}

	var chosen *models.BillArtifact
	if target.EmailMessageID != "" {
		for i := range artifacts {
			a := artifacts[i]
			if !isEmailBodyArtifact(a) || artifactMessageID(a) != target.EmailMessageID {
				continue
			}
			if chosen == nil || a.Kind == "email_html" {
				chosen = &a
			}
			if a.Kind == "email_html" {
				break
			}
		}
	}
	if chosen == nil {
		for i := range artifacts {
			a := artifacts[i]
			if a.Kind == "email_html" {
				chosen = &a
				break
			}
			if chosen == nil && a.Kind == "email_text" {
				chosen = &a
			}
		}
	}
	if chosen != nil {
		data, a, err := svc.Read(chosen.ID)
		if err != nil {
			return "", "", "", false, err
		}
		if a.Kind == "email_html" {
			return "", string(data), a.ID, true, nil
		}
		return string(data), "", a.ID, true, nil
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(target.RawData, &raw); err == nil {
		text := stringField(raw, "body_text")
		html := stringField(raw, "body_html")
		if text != "" || html != "" {
			return text, html, "raw_data", true, nil
		}
	}
	return "", "", "", false, nil
}

func isEmailBodyArtifact(a models.BillArtifact) bool {
	return a.Kind == "email_html" || a.Kind == "email_text"
}

func artifactMessageID(a models.BillArtifact) string {
	if len(a.SourceMeta) == 0 {
		return ""
	}
	var meta map[string]interface{}
	if err := json.Unmarshal(a.SourceMeta, &meta); err != nil {
		return ""
	}
	return stringField(meta, "message_id")
}

func stringField(raw map[string]interface{}, key string) string {
	value, ok := raw[key]
	if !ok || value == nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}
