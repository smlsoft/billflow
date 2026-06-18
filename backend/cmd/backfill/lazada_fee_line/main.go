// lazada_fee_line backfill — adds the configured Lazada shipping/fee line to
// active unsent Lazada purchase bills that were created before fee lines were
// inserted at ingestion time.
//
// Usage:
//
//	go run ./cmd/backfill/lazada_fee_line --dry-run
//	go run ./cmd/backfill/lazada_fee_line --apply
//
// Required env var: DATABASE_URL
package main

import (
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"billflow/internal/database"
	"billflow/internal/models"
	"billflow/internal/repository"

	_ "github.com/lib/pq"
)

type lazadaFeeLineTarget struct {
	BillID        string
	OrderID       string
	FeeAmount     float64
	ExistingItems int
}

func main() {
	dryRun := flag.Bool("dry-run", false, "print what would change without writing to DB")
	apply := flag.Bool("apply", false, "write missing fee lines to DB")
	flag.Parse()

	if *dryRun == *apply {
		log.Fatal("choose exactly one of --dry-run or --apply")
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL is required")
	}

	db, err := database.Connect(dbURL)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	defer db.Close()

	billRepo := repository.NewBillRepo(db)
	channelDefaults := repository.NewChannelDefaultRepo(db)
	catalogRepo := repository.NewSMLCatalogRepo(db)
	auditRepo := repository.NewAuditLogRepo(db)

	targets, err := listLazadaFeeLineTargets(db)
	if err != nil {
		log.Fatalf("list targets: %v", err)
	}
	log.Printf("found %d active unsent Lazada bills missing fee line", len(targets))

	var inserted, skipped int
	for _, target := range targets {
		fmt.Printf("[%s] bill=%s order=%s fee_amount=%.2f existing_items=%d would_insert=true\n",
			modeLabel(*dryRun), target.BillID, target.OrderID, target.FeeAmount, target.ExistingItems)
		if *dryRun {
			continue
		}

		item, err := buildLazadaFeeLineItem(channelDefaults, catalogRepo, target)
		if err != nil {
			log.Printf("skip bill=%s order=%s: %v", target.BillID, target.OrderID, err)
			skipped++
			continue
		}
		if err := billRepo.InsertItem(item); err != nil {
			log.Printf("insert bill=%s order=%s: %v", target.BillID, target.OrderID, err)
			skipped++
			continue
		}
		billID := target.BillID
		_ = auditRepo.Log(models.AuditEntry{
			Action:   "lazada_fee_line_backfill",
			TargetID: &billID,
			Source:   "lazada_email",
			Level:    "info",
			Detail: map[string]interface{}{
				"order_id":       target.OrderID,
				"fee_amount":     target.FeeAmount,
				"source_sku":     models.LazadaFeeSourceSKU,
				"item_id":        item.ID,
				"item_code":      derefString(item.ItemCode),
				"existing_items": target.ExistingItems,
				"action":         "inserted",
			},
		})
		inserted++
	}

	if *dryRun {
		fmt.Printf("\ndry-run summary: targets=%d would_insert=%d\n", len(targets), len(targets))
		return
	}
	log.Printf("done: inserted=%d skipped=%d", inserted, skipped)
}

func modeLabel(dryRun bool) string {
	if dryRun {
		return "dry-run"
	}
	return "apply"
}

func listLazadaFeeLineTargets(db *sql.DB) ([]lazadaFeeLineTarget, error) {
	rows, err := db.Query(`
		WITH candidates AS (
			SELECT
				b.id::text AS bill_id,
				COALESCE(NULLIF(b.raw_data->>'order_id',''), NULLIF(b.raw_data->>'lazada_order_id',''), b.id::text) AS order_id,
				ROUND(
					COALESCE(
						CASE
							WHEN replace(COALESCE(b.raw_data->>'shipping_amount',''), ',', '') ~ '^-?[0-9]+(\.[0-9]+)?$'
							THEN replace(COALESCE(b.raw_data->>'shipping_amount',''), ',', '')::numeric
							ELSE 0
						END,
						0
					)
					+
					COALESCE(
						CASE
							WHEN replace(COALESCE(b.raw_data->>'service_fee_amount',''), ',', '') ~ '^-?[0-9]+(\.[0-9]+)?$'
							THEN replace(COALESCE(b.raw_data->>'service_fee_amount',''), ',', '')::numeric
							ELSE 0
						END,
						0
					),
					2
				)::float8 AS fee_amount,
				(SELECT COUNT(*) FROM bill_items bi WHERE bi.bill_id = b.id) AS existing_items
			FROM bills b
			WHERE b.source = 'lazada_email'
			  AND b.bill_type = 'purchase'
			  AND b.archived_at IS NULL
			  AND b.status IN ('failed', 'pending', 'needs_review')
			  AND NOT EXISTS (
			  	SELECT 1
			  	FROM bill_items bi
			  	WHERE bi.bill_id = b.id
			  	  AND bi.source_sku = $1
			  )
		)
		SELECT bill_id, order_id, fee_amount, existing_items
		FROM candidates
		WHERE fee_amount > 0
		ORDER BY order_id ASC, bill_id ASC
	`, models.LazadaFeeSourceSKU)
	if err != nil {
		return nil, fmt.Errorf("list lazada fee line targets: %w", err)
	}
	defer rows.Close()

	var out []lazadaFeeLineTarget
	for rows.Next() {
		var target lazadaFeeLineTarget
		if err := rows.Scan(&target.BillID, &target.OrderID, &target.FeeAmount, &target.ExistingItems); err != nil {
			return nil, err
		}
		out = append(out, target)
	}
	return out, rows.Err()
}

func buildLazadaFeeLineItem(
	channelDefaults *repository.ChannelDefaultRepo,
	catalogRepo *repository.SMLCatalogRepo,
	target lazadaFeeLineTarget,
) (*models.BillItem, error) {
	if target.FeeAmount <= 0 {
		return nil, errors.New("fee amount is not positive")
	}
	if channelDefaults == nil {
		return nil, errors.New("channel defaults repository is not configured")
	}
	def, err := channelDefaults.Get("lazada_email", "purchase")
	if err != nil {
		return nil, fmt.Errorf("load lazada channel default: %w", err)
	}
	if def == nil || !def.ShippingItemEnabled {
		return nil, errors.New("lazada_email purchase shipping item is not enabled")
	}
	code := strings.TrimSpace(def.ShippingItemCode)
	if code == "" {
		return nil, errors.New("lazada_email purchase shipping item code is empty")
	}
	unit := strings.TrimSpace(def.ShippingItemUnitCode)
	rawName := "ค่าจัดส่ง/ค่าธรรมเนียม Lazada"
	if catalogRepo != nil {
		cat, err := catalogRepo.GetOne(code)
		if err != nil {
			return nil, fmt.Errorf("load shipping catalog item %s: %w", code, err)
		}
		if cat != nil {
			if strings.TrimSpace(cat.ItemName) != "" {
				rawName = strings.TrimSpace(cat.ItemName)
			}
			if unit == "" {
				unit = strings.TrimSpace(cat.UnitCode)
			}
		}
	}
	if unit == "" {
		return nil, fmt.Errorf("lazada fee item %s has no unit_code in channel default or catalog", code)
	}

	itemCode := code
	unitCode := unit
	price := target.FeeAmount
	return &models.BillItem{
		BillID:    target.BillID,
		RawName:   rawName,
		SourceSKU: models.LazadaFeeSourceSKU,
		ItemCode:  &itemCode,
		Qty:       1,
		UnitCode:  &unitCode,
		Price:     &price,
		Mapped:    true,
	}, nil
}

func derefString(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
