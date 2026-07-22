// shopee_item_images backfills bill_items.source_image_url for Shopee purchase
// bills from stored email HTML artifacts. It is intentionally conservative:
// dry-run first, then --apply after reviewing summary counts.
//
// Usage:
//
//	go run ./cmd/backfill/shopee_item_images --dry-run
//	go run ./cmd/backfill/shopee_item_images --apply
//	go run ./cmd/backfill/shopee_item_images --dry-run --bill-id <uuid>
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
	DiscountAmount float64
	Mapped         bool
	ItemCode       string
	UnitCode       string
	SourceImageURL string
	SourceVariant  string
	SourceLineNo   int
}

type shopeeItemImageUpdate struct {
	ItemID        string
	RawName       string
	ImageURL      string
	SourceVariant string
	SourceLineNo  int
	Reason        string
}

type shopeeItemImageBackfillSummary struct {
	WouldUpdate           int
	Updated               int
	AlreadyHasImage       int
	MatchedByURL          int
	MatchedExact          int
	MatchedDuplicateGroup int
	NoArtifact            int
	NoMatch               int
	Ambiguous             int
	ManualReview          int
	ReadErrors            int
}

func main() {
	dryRun := flag.Bool("dry-run", false, "print what would change without writing to DB")
	apply := flag.Bool("apply", false, "write matched Shopee item image URLs to DB")
	billID := flag.String("bill-id", "", "limit processing to one bill UUID")
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

	targets, err := listShopeeItemImageTargets(db, strings.TrimSpace(*billID))
	if err != nil {
		log.Fatalf("list targets: %v", err)
	}
	log.Printf("found %d active shopee_shipped purchase bills", len(targets))

	var summary shopeeItemImageBackfillSummary
	for _, target := range targets {
		processShopeeItemImageTarget(db, artifactSvc, artifactRepo, auditRepo, target, *dryRun, &summary)
	}

	if *dryRun {
		fmt.Printf("\ndry-run summary: would_update=%d already_has_image=%d matched_by_url=%d matched_exact=%d matched_duplicate_group=%d no_artifact=%d no_match=%d ambiguous=%d manual_review=%d read_errors=%d\n",
			summary.WouldUpdate, summary.AlreadyHasImage, summary.MatchedByURL, summary.MatchedExact, summary.MatchedDuplicateGroup,
			summary.NoArtifact, summary.NoMatch, summary.Ambiguous, summary.ManualReview, summary.ReadErrors)
		return
	}
	log.Printf("done: updated=%d already_has_image=%d matched_by_url=%d matched_exact=%d matched_duplicate_group=%d no_artifact=%d no_match=%d ambiguous=%d manual_review=%d read_errors=%d",
		summary.Updated, summary.AlreadyHasImage, summary.MatchedByURL, summary.MatchedExact, summary.MatchedDuplicateGroup,
		summary.NoArtifact, summary.NoMatch, summary.Ambiguous, summary.ManualReview, summary.ReadErrors)
}

func listShopeeItemImageTargets(db *sql.DB, billID string) ([]shopeeItemImageTarget, error) {
	rows, err := db.Query(`
		SELECT b.id::text,
		       COALESCE(NULLIF(b.raw_data->>'order_id',''), NULLIF(b.raw_data->>'shopee_order_id',''), b.sml_order_id, '') AS order_id,
		       COALESCE(b.raw_data->>'email_message_id', '') AS email_message_id,
		       COALESCE(b.raw_data, '{}'::jsonb) AS raw_data,
		       bi.id::text,
		       bi.raw_name,
		       bi.qty,
		       bi.price,
		       COALESCE(bi.discount_amount, 0) AS discount_amount,
		       bi.mapped,
		       COALESCE(bi.item_code, '') AS item_code,
		       COALESCE(bi.unit_code, '') AS unit_code,
		       COALESCE(bi.source_image_url, '') AS source_image_url,
		       COALESCE(bi.source_variant, '') AS source_variant,
		       COALESCE(bi.source_line_no, 0) AS source_line_no
		  FROM bills b
		  JOIN bill_items bi ON bi.bill_id = b.id
		 WHERE b.source = 'shopee_shipped'
		   AND b.bill_type = 'purchase'
		   AND b.archived_at IS NULL
		   AND b.sent_at IS NULL
		   AND COALESCE(b.sml_doc_no, '') = ''
		   AND b.status IN ('pending', 'needs_review', 'failed')
		   AND COALESCE(bi.source_sku, '') <> $2
		   AND (NULLIF($1, '') IS NULL OR b.id = NULLIF($1, '')::uuid)
		 ORDER BY b.created_at ASC, b.id ASC, bi.id ASC`, billID, models.ShopeeShippingSourceSKU)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	byBill := map[string]*shopeeItemImageTarget{}
	order := []string{}
	for rows.Next() {
		var billID, orderID, messageID, itemID, rawName, itemCode, unitCode, sourceImageURL, sourceVariant string
		var qty, discountAmount float64
		var mapped bool
		var sourceLineNo int
		var price sql.NullFloat64
		var rawData []byte
		if err := rows.Scan(
			&billID, &orderID, &messageID, &rawData, &itemID, &rawName, &qty, &price,
			&discountAmount, &mapped, &itemCode, &unitCode, &sourceImageURL, &sourceVariant, &sourceLineNo,
		); err != nil {
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
			DiscountAmount: discountAmount,
			Mapped:         mapped,
			ItemCode:       itemCode,
			UnitCode:       unitCode,
			SourceImageURL: sourceImageURL,
			SourceVariant:  sourceVariant,
			SourceLineNo:   sourceLineNo,
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
	needsMetadata := 0
	for _, item := range target.Items {
		if strings.TrimSpace(item.SourceImageURL) == "" ||
			strings.TrimSpace(item.SourceVariant) == "" || item.SourceLineNo <= 0 {
			needsMetadata++
		}
	}
	if needsMetadata == 0 {
		summary.AlreadyHasImage += len(target.Items)
		return
	}

	bodyHTML, artifactID, ok, err := loadShopeeItemImageHTML(artifactSvc, artifactRepo, target)
	if err != nil {
		summary.ReadErrors += needsMetadata
		log.Printf("read artifact bill=%s order=%s: %v", target.ID, target.OrderID, err)
		return
	}
	if !ok {
		summary.NoArtifact += needsMetadata
		if dryRun {
			fmt.Printf("[dry-run] bill %s order %s: no email_html artifact/raw body_html\n", target.ID, target.OrderID)
		}
		return
	}

	updates, delta := planShopeeItemImageUpdates(target, bodyHTML)
	mergeShopeeItemImageSummary(summary, delta)
	if dryRun {
		summary.WouldUpdate += len(updates)
		for _, update := range updates {
			fmt.Printf("[dry-run] bill %s order %s item %s: image_url=%s variant=%q line=%d reason=%s\n",
				target.ID, target.OrderID, update.ItemID, update.ImageURL, update.SourceVariant, update.SourceLineNo, update.Reason)
		}
		return
	}

	tx, err := db.Begin()
	if err != nil {
		summary.ReadErrors += len(updates)
		log.Printf("begin item image update bill=%s: %v", target.ID, err)
		return
	}
	updatedItems := []string{}
	for _, update := range updates {
		changed, err := updateShopeeItemImageMetadata(tx, update)
		if err != nil {
			_ = tx.Rollback()
			summary.ReadErrors += len(updates)
			log.Printf("update item image bill=%s item=%s: %v", target.ID, update.ItemID, err)
			return
		}
		if changed {
			updatedItems = append(updatedItems, update.ItemID)
		}
	}
	if err := tx.Commit(); err != nil {
		summary.ReadErrors += len(updates)
		log.Printf("commit item image update bill=%s: %v", target.ID, err)
		return
	}
	summary.Updated += len(updatedItems)
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
			"order_id":                target.OrderID,
			"artifact_id":             artifactID,
			"updated_count":           len(updatedItems),
			"matched_by_url":          delta.MatchedByURL,
			"matched_exact":           delta.MatchedExact,
			"matched_duplicate_group": delta.MatchedDuplicateGroup,
			"item_ids":                updatedItems,
		},
	})
}

func mergeShopeeItemImageSummary(dst *shopeeItemImageBackfillSummary, delta shopeeItemImageBackfillSummary) {
	dst.AlreadyHasImage += delta.AlreadyHasImage
	dst.MatchedByURL += delta.MatchedByURL
	dst.MatchedExact += delta.MatchedExact
	dst.MatchedDuplicateGroup += delta.MatchedDuplicateGroup
	dst.NoMatch += delta.NoMatch
	dst.Ambiguous += delta.Ambiguous
	dst.ManualReview += delta.ManualReview
	dst.ReadErrors += delta.ReadErrors
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
	unsafeDuplicateGroups := unsafeShopeeDuplicateBackfillGroups(target.Items)

	var summary shopeeItemImageBackfillSummary
	updates := []shopeeItemImageUpdate{}
	for i, item := range target.Items {
		if strings.TrimSpace(item.SourceImageURL) != "" {
			summary.AlreadyHasImage++
		}
		decision := decisions[i]
		if decision.Reason == handlers.ShopeeItemImageReasonDuplicateGroup {
			if key := shopeeBackfillIdentityKey(item); key != "" && unsafeDuplicateGroups[key] {
				summary.ManualReview++
				continue
			}
		}
		if shopeeBackfillMetadataConflicts(item, decision) {
			summary.ManualReview++
			continue
		}
		switch decision.Reason {
		case handlers.ShopeeItemImageReasonExisting:
			if !shopeeItemImageNeedsUpdate(item, decision) {
				continue
			}
			summary.MatchedByURL++
		case handlers.ShopeeItemImageReasonDuplicateGroup:
			summary.MatchedDuplicateGroup++
		case handlers.ShopeeItemImageReasonBlock, handlers.ShopeeItemImageReasonNearest, handlers.ShopeeItemImageReasonSingleFallback:
			summary.MatchedExact++
		case handlers.ShopeeItemImageReasonAmbiguous:
			summary.Ambiguous++
			continue
		default:
			summary.NoMatch++
			continue
		}

		if !shopeeItemImageNeedsUpdate(item, decision) {
			continue
		}
		imageURL := strings.TrimSpace(matched[i].ImageURL)
		if imageURL == "" {
			imageURL = strings.TrimSpace(decision.ImageURL)
		}
		if imageURL == "" && strings.TrimSpace(decision.SourceVariant) == "" && decision.SourceLineNo <= 0 {
			summary.NoMatch++
			continue
		}
		updates = append(updates, shopeeItemImageUpdate{
			ItemID:        item.ID,
			RawName:       item.RawName,
			ImageURL:      imageURL,
			SourceVariant: strings.TrimSpace(decision.SourceVariant),
			SourceLineNo:  decision.SourceLineNo,
			Reason:        decision.Reason,
		})
	}
	return updates, summary
}

func shopeeItemImageNeedsUpdate(item shopeeItemImageTargetItem, decision handlers.ShopeeItemImageDecision) bool {
	return (strings.TrimSpace(item.SourceImageURL) == "" && strings.TrimSpace(decision.ImageURL) != "") ||
		(strings.TrimSpace(item.SourceVariant) == "" && strings.TrimSpace(decision.SourceVariant) != "") ||
		(item.SourceLineNo <= 0 && decision.SourceLineNo > 0)
}

func shopeeBackfillMetadataConflicts(item shopeeItemImageTargetItem, decision handlers.ShopeeItemImageDecision) bool {
	return (strings.TrimSpace(item.SourceImageURL) != "" && strings.TrimSpace(decision.ImageURL) != "" && !strings.EqualFold(strings.TrimSpace(item.SourceImageURL), strings.TrimSpace(decision.ImageURL))) ||
		(strings.TrimSpace(item.SourceVariant) != "" && strings.TrimSpace(decision.SourceVariant) != "" && strings.TrimSpace(item.SourceVariant) != strings.TrimSpace(decision.SourceVariant)) ||
		(item.SourceLineNo > 0 && decision.SourceLineNo > 0 && item.SourceLineNo != decision.SourceLineNo)
}

func unsafeShopeeDuplicateBackfillGroups(items []shopeeItemImageTargetItem) map[string]bool {
	groups := map[string][]shopeeItemImageTargetItem{}
	for _, item := range items {
		if key := shopeeBackfillIdentityKey(item); key != "" {
			groups[key] = append(groups[key], item)
		}
	}
	unsafe := map[string]bool{}
	for key, group := range groups {
		if len(group) < 2 {
			continue
		}
		first := group[0]
		for _, item := range group[1:] {
			if !sameBackfillNumber(first.DiscountAmount, item.DiscountAmount) ||
				first.Mapped != item.Mapped || strings.TrimSpace(first.ItemCode) != strings.TrimSpace(item.ItemCode) ||
				strings.TrimSpace(first.UnitCode) != strings.TrimSpace(item.UnitCode) {
				unsafe[key] = true
				break
			}
		}
	}
	return unsafe
}

func shopeeBackfillIdentityKey(item shopeeItemImageTargetItem) string {
	if strings.TrimSpace(item.RawName) == "" || item.Qty <= 0 || item.Price == nil {
		return ""
	}
	return strings.ToLower(strings.Join(strings.Fields(item.RawName), " ")) + "\x1f" +
		fmt.Sprintf("%.4f\x1f%.4f", item.Qty, *item.Price)
}

func sameBackfillNumber(a, b float64) bool {
	if a > b {
		return a-b < 0.01
	}
	return b-a < 0.01
}

type shopeeItemImageExecutor interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
}

func updateShopeeItemImageMetadata(exec shopeeItemImageExecutor, update shopeeItemImageUpdate) (bool, error) {
	res, err := exec.Exec(`
		UPDATE bill_items bi
		   SET source_image_url = CASE
		         WHEN COALESCE(bi.source_image_url, '') = '' AND $1 <> '' THEN $1
		         ELSE bi.source_image_url
		       END,
		       source_variant = CASE
		         WHEN COALESCE(bi.source_variant, '') = '' AND $2 <> '' THEN $2
		         ELSE bi.source_variant
		       END,
		       source_line_no = CASE
		         WHEN COALESCE(bi.source_line_no, 0) <= 0 AND $3 > 0 THEN $3
		         ELSE bi.source_line_no
		       END
		  FROM bills b
		 WHERE bi.id = $4::uuid
		   AND bi.bill_id = b.id
		   AND b.source = 'shopee_shipped'
		   AND b.bill_type = 'purchase'
		   AND b.archived_at IS NULL
		   AND b.sent_at IS NULL
		   AND COALESCE(b.sml_doc_no, '') = ''
		   AND b.status IN ('pending', 'needs_review', 'failed')
		   AND (
		     (COALESCE(bi.source_image_url, '') = '' AND $1 <> '') OR
		     (COALESCE(bi.source_variant, '') = '' AND $2 <> '') OR
		     (COALESCE(bi.source_line_no, 0) <= 0 AND $3 > 0)
		   )`,
		update.ImageURL, update.SourceVariant, update.SourceLineNo, update.ItemID,
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
