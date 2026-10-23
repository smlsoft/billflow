// lazada_source_items repairs one explicitly selected, unmapped, unsent bill
// from its matching HTML artifact. Dry-run uses a read-only transaction and
// never runs migrations. Apply requires the exact dry-run plan fingerprint.
package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"strings"
	"time"

	"billflow/internal/models"
	"billflow/internal/repository"
	"billflow/internal/services/artifact"
	_ "github.com/lib/pq"
)

func main() {
	dry := flag.Bool("dry-run", false, "read and print a repair plan without writes or migrations")
	apply := flag.Bool("apply", false, "apply the reviewed plan atomically")
	billID := flag.String("bill-id", "", "required bill UUID")
	orderID := flag.String("order-id", "", "required source order ID")
	expected := flag.String("expected-plan", "", "plan hash printed by dry-run; required for apply")
	flag.Parse()
	if *dry == *apply || *billID == "" || *orderID == "" || (*apply && *expected == "") {
		log.Fatal("choose --dry-run or --apply, specify --bill-id and --order-id; apply also requires --expected-plan")
	}
	if err := run(*billID, *orderID, *expected, *apply); err != nil {
		log.Fatal(err)
	}
}

func run(billID, orderID, expected string, apply bool) error {
	if os.Getenv("DATABASE_URL") == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	db, err := sql.Open("postgres", os.Getenv("DATABASE_URL"))
	if err != nil {
		return err
	}
	defer db.Close()
	return repairBill(db, billID, orderID, expected, apply)
}

func repairBill(db *sql.DB, billID, orderID, expected string, apply bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: !apply, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`SET LOCAL lock_timeout = '5s'`); err != nil {
		return err
	}
	var rawJSON []byte
	var source, billType, status, storedOrder, docNo string
	var sent, archived bool
	query := `SELECT source, bill_type, status, COALESCE(raw_data->>'order_id',raw_data->>'lazada_order_id',''), COALESCE(sml_doc_no,''), sent_at IS NOT NULL, archived_at IS NOT NULL, raw_data FROM bills WHERE id=$1`
	if apply {
		query += ` FOR UPDATE`
	}
	if err := tx.QueryRow(query, billID).Scan(&source, &billType, &status, &storedOrder, &docNo, &sent, &archived, &rawJSON); err != nil {
		return err
	}
	if err := eligible(source, billType, status, storedOrder, orderID, docNo, sent, archived); err != nil {
		return err
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(rawJSON, &raw); err != nil {
		return err
	}
	if raw["source_items_repair_version"] == "html-v1" {
		fmt.Println("already repaired: no changes")
		return nil
	}
	var oldJSON []byte
	if apply {
		rows, err := tx.Query(`SELECT id FROM bill_items WHERE bill_id=$1 FOR UPDATE`, billID)
		if err != nil {
			return err
		}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
	}
	if err := tx.QueryRow(`SELECT COALESCE(jsonb_agg(to_jsonb(i) ORDER BY i.id), '[]'::jsonb) FROM bill_items i WHERE bill_id=$1`, billID).Scan(&oldJSON); err != nil {
		return err
	}
	var old []models.BillItem
	if err := json.Unmarshal(oldJSON, &old); err != nil {
		return err
	}
	if err := unmappedGoods(old); err != nil {
		return err
	}
	messageID, _ := raw["email_message_id"].(string)
	if messageID == "" {
		return fmt.Errorf("missing source email message ID")
	}
	repo := repository.NewBillArtifactRepo(db)
	a, err := repo.FindEmailBodyByBillMessage(billID, messageID)
	if err != nil {
		return err
	}
	if a == nil || a.Kind != "email_html" {
		return fmt.Errorf("no matching HTML artifact; refusing another email's content")
	}
	dir := os.Getenv("ARTIFACTS_DIR")
	if dir == "" {
		dir = "/app/artifacts"
	}
	data, _, err := artifact.New(dir, 50*1024*1024, repo, nil).Read(a.ID)
	if err != nil {
		return err
	}
	if url := repository.ExtractLazadaMarketplaceOrderURL("", string(data), orderID); url == "" {
		return fmt.Errorf("source HTML does not prove requested order ID")
	}
	summary := repository.ExtractLazadaAmountSummary("", string(data))
	if summary.ReconciliationStatus != repository.LazadaReconciliationOK {
		return fmt.Errorf("email totals do not reconcile")
	}
	items, err := repository.ParseLazadaEmailItems(string(data), summary)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		return fmt.Errorf("no verified source items")
	}
	fee := 0.0
	for _, item := range old {
		if item.SourceSKU == models.LazadaFeeSourceSKU && item.Price != nil {
			fee += item.Qty**item.Price - item.DiscountAmount
		}
	}
	if math.Abs(fee-summary.FeeAmount()) > .01 {
		return fmt.Errorf("existing fee %.2f differs from email %.2f; repair fee separately", fee, summary.FeeAmount())
	}
	repository.ApplyLazadaDiscountsToItems(items, summary.CouponDiscountAmount)
	hash := planHash(billID, orderID, status, rawJSON, oldJSON, a.ID, data, items)
	fmt.Printf("bill=%s order=%s source_rows=%d goods=%.2f coupon=%.2f fee=%.2f paid=%.2f\n", billID, orderID, len(items), summary.GoodsTotalAmount, summary.CouponDiscountAmount, fee, summary.PaidTotalAmount)
	for i, item := range items {
		fmt.Printf("line=%d sku=%s qty=%g unit_price=%.2f discount=%.2f name=%s\n", i+1, item.SourceSKU, item.Qty, *item.Price, item.DiscountAmount, item.RawName)
	}
	fmt.Printf("plan=%s\n", hash)
	if !apply {
		fmt.Println("dry-run: no writes; use this plan hash with --expected-plan on apply")
		return nil
	}
	if hash != expected {
		return fmt.Errorf("plan changed since dry-run; run dry-run and review again")
	}
	if _, err := tx.Exec(`DELETE FROM bill_items WHERE bill_id=$1 AND COALESCE(source_sku,'') <> $2`, billID, models.LazadaFeeSourceSKU); err != nil {
		return err
	}
	for _, item := range items {
		if _, err := tx.Exec(`INSERT INTO bill_items (bill_id,raw_name,source_sku,source_image_url,source_line_no,qty,price,discount_amount,mapped) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,false)`, billID, item.RawName, item.SourceSKU, item.SourceImageURL, item.SourceLineNo, item.Qty, item.Price, item.DiscountAmount); err != nil {
			return err
		}
	}
	rawItems := make([]map[string]interface{}, len(items))
	for i, item := range items {
		rawItems[i] = map[string]interface{}{"raw_name": item.RawName, "qty": item.Qty, "price": item.Price, "image_url": item.SourceImageURL, "source_sku": item.SourceSKU, "source_line_no": item.SourceLineNo}
	}
	raw["items"] = rawItems
	raw["source_item_count"] = len(items)
	raw["source_items_status"] = "verified_html"
	raw["source_items_repair_version"] = "html-v1"
	raw["source_items_artifact_id"] = a.ID
	raw["items_gross_delta"] = 0
	raw["goods_total_amount"] = summary.GoodsTotalAmount
	raw["shipping_amount"] = summary.ShippingAmount
	raw["service_fee_amount"] = summary.ServiceFeeAmount
	raw["lazada_fee_amount"] = summary.FeeAmount()
	raw["coupon_discount_amount"] = summary.CouponDiscountAmount
	raw["paid_total_amount"] = summary.PaidTotalAmount
	raw["total_amount"] = summary.PaidTotalAmount
	raw["amount_reconciliation_delta"] = 0
	raw["discount_summary"] = map[string]interface{}{"platform": "lazada", "coupon_discount_amount": summary.CouponDiscountAmount, "total_discount_amount": summary.TotalDiscountAmount, "allocation_method": summary.AllocationMethod}
	raw["amount_reconciliation_status"] = repository.LazadaReconciliationOK
	delete(raw, "amount_reconciliation_warning")
	raw["item_count"] = len(items)
	for _, item := range old {
		if item.SourceSKU == models.LazadaFeeSourceSKU {
			raw["item_count"] = raw["item_count"].(int) + 1
		}
	}
	updated, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE bills SET raw_data=$1::jsonb, status='needs_review', error_msg=NULL WHERE id=$2`, string(updated), billID); err != nil {
		return err
	}
	audit, _ := json.Marshal(map[string]interface{}{"order_id": orderID, "artifact_id": a.ID, "plan": hash, "previous_raw_data": json.RawMessage(rawJSON), "previous_items": json.RawMessage(oldJSON), "new_items": items})
	if _, err := tx.Exec(`INSERT INTO audit_logs (action,target_id,source,level,detail) VALUES ('lazada_source_items_repaired',$1,'lazada_email','info',$2::jsonb)`, billID, string(audit)); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	fmt.Println("applied: source items restored, fee preserved, status=needs_review; map each variant before SML")
	return nil
}

func eligible(source, billType, status, storedOrder, orderID, docNo string, sent, archived bool) error {
	if source != "lazada_email" || billType != "purchase" || storedOrder != orderID || sent || archived || strings.TrimSpace(docNo) != "" || (status != "needs_review" && status != "pending" && status != "failed") {
		return fmt.Errorf("bill is not an eligible unsent Lazada purchase for this order")
	}
	return nil
}

func planHash(billID, orderID, status string, rawJSON, oldJSON []byte, artifactID string, data []byte, items []models.BillItem) string {
	planData, _ := json.Marshal([]interface{}{billID, orderID, status, json.RawMessage(rawJSON), json.RawMessage(oldJSON), artifactID, fmt.Sprintf("%x", sha256.Sum256(data)), items})
	return fmt.Sprintf("%x", sha256.Sum256(planData))
}

func unmappedGoods(items []models.BillItem) error {
	for _, item := range items {
		if item.SourceSKU != models.LazadaFeeSourceSKU && (item.Mapped || item.ItemCode != nil || item.MappingID != nil) {
			return fmt.Errorf("goods line %s has mapping; refusing to overwrite reviewed work", item.ID)
		}
	}
	return nil
}
