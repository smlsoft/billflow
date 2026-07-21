// shopee_item_images backfills bill_items.source_image_url for Shopee purchase
// bills from stored email HTML artifacts. It is intentionally conservative:
// dry-run first, then --apply after reviewing summary counts.
//
// Usage:
//
//	go run ./cmd/backfill/shopee_item_images --dry-run
//	go run ./cmd/backfill/shopee_item_images --apply
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
	"billflow/internal/services/ai"
	"billflow/internal/services/artifact"

	"github.com/joho/godotenv"
	"go.uber.org/zap"
)

type shopeeItemImageTarget struct {
	ID             string
	OrderID        string
	EmailMessageID string
	RawData        json.RawMessage
	Items          []shopeeItemImageTargetItem
}

type shopeeItemImageTargetItem struct {
	ID             string
	RawName        string
	Qty            float64
	Price          *float64
	SourceImageURL string
}

type shopeeItemImageUpdate struct {
	ItemID   string
	RawName  string
	ImageURL string
	Reason   string
}

type shopeeItemImageBackfillSummary struct {
	WouldUpdate     int
	Updated         int
	AlreadyHasImage int
	NoArtifact      int
	NoMatch         int
	Ambiguous       int
	ReadErrors      int
}

func main() {
	dryRun := flag.Bool("dry-run", false, "print what would change without writing to DB")
	apply := flag.Bool("apply", false, "write matched Shopee item image URLs to DB")
	flag.Parse()
	if *dryRun == *apply {
		log.Fatal("choose exactly one of --dry-run or --apply")
	}

	_ = godotenv.Load()
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

	targets, err := listShopeeItemImageTargets(db)
	if err != nil {
		log.Fatalf("list targets: %v", err)
	}
	log.Printf("found %d active shopee_shipped purchase bills", len(targets))

	var summary shopeeItemImageBackfillSummary
	for _, target := range targets {
		processShopeeItemImageTarget(db, artifactSvc, artifactRepo, auditRepo, target, *dryRun, &summary)
	}

	if *dryRun {
		fmt.Printf("\ndry-run summary: would_update=%d already_has_image=%d no_artifact=%d no_match=%d ambiguous=%d read_errors=%d\n",
			summary.WouldUpdate, summary.AlreadyHasImage, summary.NoArtifact, summary.NoMatch, summary.Ambiguous, summary.ReadErrors)
		return
	}
	log.Printf("done: updated=%d already_has_image=%d no_artifact=%d no_match=%d ambiguous=%d read_errors=%d",
		summary.Updated, summary.AlreadyHasImage, summary.NoArtifact, summary.NoMatch, summary.Ambiguous, summary.ReadErrors)
}

func listShopeeItemImageTargets(db *sql.DB) ([]shopeeItemImageTarget, error) {
	rows, err := db.Query(`
		SELECT b.id::text,
		       COALESCE(NULLIF(b.raw_data->>'order_id',''), NULLIF(b.raw_data->>'shopee_order_id',''), b.sml_order_id, '') AS order_id,
		       COALESCE(b.raw_data->>'email_message_id', '') AS email_message_id,
		       COALESCE(b.raw_data, '{}'::jsonb) AS raw_data,
		       bi.id::text,
		       bi.raw_name,
		       bi.qty,
		       bi.price,
		       COALESCE(bi.source_image_url, '') AS source_image_url
		  FROM bills b
		  JOIN bill_items bi ON bi.bill_id = b.id
		 WHERE b.source = 'shopee_shipped'
		   AND b.bill_type = 'purchase'
		   AND b.archived_at IS NULL
		 ORDER BY b.created_at ASC, b.id ASC, bi.id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	byBill := map[string]*shopeeItemImageTarget{}
	order := []string{}
	for rows.Next() {
		var billID, orderID, messageID, itemID, rawName, sourceImageURL string
		var qty float64
		var price sql.NullFloat64
		var rawData []byte
		if err := rows.Scan(&billID, &orderID, &messageID, &rawData, &itemID, &rawName, &qty, &price, &sourceImageURL); err != nil {
			return nil, err
		}
		target := byBill[billID]
		if target == nil {
			target = &shopeeItemImageTarget{
				ID:             billID,
				OrderID:        strings.TrimSpace(orderID),
				EmailMessageID: strings.TrimSpace(messageID),
				RawData:        json.RawMessage(rawData),
			}
			byBill[billID] = target
			order = append(order, billID)
		}
		var pricePtr *float64
		if price.Valid {
			v := price.Float64
			pricePtr = &v
		}
		target.Items = append(target.Items, shopeeItemImageTargetItem{
			ID:             itemID,
			RawName:        rawName,
			Qty:            qty,
			Price:          pricePtr,
			SourceImageURL: sourceImageURL,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]shopeeItemImageTarget, 0, len(order))
	for _, billID := range order {
		out = append(out, *byBill[billID])
	}
	return out, nil
}

func processShopeeItemImageTarget(
	db *sql.DB,
	artifactSvc *artifact.Service,
	artifactRepo *repository.BillArtifactRepo,
	auditRepo *repository.AuditLogRepo,
	target shopeeItemImageTarget,
	dryRun bool,
	summary *shopeeItemImageBackfillSummary,
) {
	blankItems := 0
	alreadyItems := 0
	for _, item := range target.Items {
		if strings.TrimSpace(item.SourceImageURL) != "" {
			alreadyItems++
			continue
		}
		blankItems++
	}
	summary.AlreadyHasImage += alreadyItems
	if blankItems == 0 {
		return
	}

	bodyHTML, artifactID, ok, err := loadShopeeItemImageHTML(artifactSvc, artifactRepo, target)
	if err != nil {
		summary.ReadErrors += blankItems
		log.Printf("read artifact bill=%s order=%s: %v", target.ID, target.OrderID, err)
		return
	}
	if !ok {
		summary.NoArtifact += blankItems
		if dryRun {
			fmt.Printf("[dry-run] bill %s order %s: no email_html artifact/raw body_html\n", target.ID, target.OrderID)
		}
		return
	}

	updates, delta := planShopeeItemImageUpdates(target, bodyHTML)
	summary.NoMatch += delta.NoMatch
	summary.Ambiguous += delta.Ambiguous
	if dryRun {
		summary.WouldUpdate += len(updates)
		for _, update := range updates {
			fmt.Printf("[dry-run] bill %s order %s item %s: image_url=%s reason=%s\n",
				target.ID, target.OrderID, update.ItemID, update.ImageURL, update.Reason)
		}
		return
	}

	updatedItems := []string{}
	for _, update := range updates {
		changed, err := updateShopeeItemImage(db, update.ItemID, update.ImageURL)
		if err != nil {
			summary.ReadErrors++
			log.Printf("update item image bill=%s item=%s: %v", target.ID, update.ItemID, err)
			continue
		}
		if changed {
			summary.Updated++
			updatedItems = append(updatedItems, update.ItemID)
		}
	}
	if len(updatedItems) == 0 || auditRepo == nil {
		return
	}
	billID := target.ID
	_ = auditRepo.Log(models.AuditEntry{
		Action:   "shopee_item_images_backfill",
		TargetID: &billID,
		Source:   "shopee_shipped",
		Level:    "info",
		Detail: map[string]interface{}{
			"order_id":      target.OrderID,
			"artifact_id":   artifactID,
			"updated_count": len(updatedItems),
			"item_ids":      updatedItems,
		},
	})
}

func planShopeeItemImageUpdates(target shopeeItemImageTarget, bodyHTML string) ([]shopeeItemImageUpdate, shopeeItemImageBackfillSummary) {
	items := make([]ai.ExtractedItem, 0, len(target.Items))
	for _, item := range target.Items {
		items = append(items, ai.ExtractedItem{
			RawName:  item.RawName,
			Qty:      item.Qty,
			Price:    item.Price,
			ImageURL: item.SourceImageURL,
		})
	}
	matched, decisions := handlers.MatchShopeeItemImages(items, bodyHTML, target.OrderID)

	var summary shopeeItemImageBackfillSummary
	updates := []shopeeItemImageUpdate{}
	for i, item := range target.Items {
		if strings.TrimSpace(item.SourceImageURL) != "" {
			summary.AlreadyHasImage++
			continue
		}
		decision := decisions[i]
		switch decision.Reason {
		case handlers.ShopeeItemImageReasonBlock, handlers.ShopeeItemImageReasonNearest, handlers.ShopeeItemImageReasonSingleFallback:
			imageURL := strings.TrimSpace(matched[i].ImageURL)
			if imageURL == "" {
				summary.NoMatch++
				continue
			}
			updates = append(updates, shopeeItemImageUpdate{
				ItemID:   item.ID,
				RawName:  item.RawName,
				ImageURL: imageURL,
				Reason:   decision.Reason,
			})
		case handlers.ShopeeItemImageReasonAmbiguous:
			summary.Ambiguous++
		default:
			summary.NoMatch++
		}
	}
	return updates, summary
}

func updateShopeeItemImage(db *sql.DB, itemID, imageURL string) (bool, error) {
	res, err := db.Exec(`
		UPDATE bill_items bi
		   SET source_image_url = $1
		  FROM bills b
		 WHERE bi.id = $2::uuid
		   AND bi.bill_id = b.id
		   AND b.source = 'shopee_shipped'
		   AND b.bill_type = 'purchase'
		   AND b.archived_at IS NULL
		   AND COALESCE(bi.source_image_url, '') = ''`,
		imageURL, itemID,
	)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func loadShopeeItemImageHTML(
	svc *artifact.Service,
	repo *repository.BillArtifactRepo,
	target shopeeItemImageTarget,
) (bodyHTML, artifactID string, ok bool, err error) {
	artifacts, err := repo.ListByBill(target.ID)
	if err != nil {
		return "", "", false, err
	}
	var chosen *models.BillArtifact
	if target.EmailMessageID != "" {
		for i := range artifacts {
			a := artifacts[i]
			if a.Kind != "email_html" || artifactMessageID(a) != target.EmailMessageID {
				continue
			}
			chosen = &a
			break
		}
	}
	if chosen == nil {
		for i := range artifacts {
			a := artifacts[i]
			if a.Kind == "email_html" {
				chosen = &a
				break
			}
		}
	}
	if chosen != nil {
		data, a, err := svc.Read(chosen.ID)
		if err != nil {
			return "", "", false, err
		}
		if strings.TrimSpace(string(data)) != "" {
			return string(data), a.ID, true, nil
		}
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(target.RawData, &raw); err == nil {
		html := stringField(raw, "body_html")
		if html != "" {
			return html, "raw_data", true, nil
		}
	}
	return "", "", false, nil
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
