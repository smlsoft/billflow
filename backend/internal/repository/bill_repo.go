package repository

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"billflow/internal/models"
)

type BillRepo struct {
	db *sql.DB
}

func NewBillRepo(db *sql.DB) *BillRepo {
	return &BillRepo{db: db}
}

// DB exposes the underlying *sql.DB for one-off queries.
func (r *BillRepo) DB() *sql.DB { return r.db }

func (r *BillRepo) Create(b *models.Bill) error {
	raw, _ := json.Marshal(b.RawData)
	anomalies, _ := json.Marshal([]models.Anomaly{})

	var orderID *string
	if b.SMLOrderID != "" {
		orderID = &b.SMLOrderID
	}

	return r.db.QueryRow(
		`INSERT INTO bills (bill_type, source, status, raw_data, ai_confidence, anomalies, created_by, sml_order_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 RETURNING id, created_at`,
		b.BillType, b.Source,
		coalesceStatus(b.Status, "pending"),
		raw, b.AIConfidence, anomalies, b.CreatedBy, orderID,
	).Scan(&b.ID, &b.CreatedAt)
}

func coalesceStatus(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

// ListByLineUserID returns recent bills tied to a LINE user, joined via
// raw_data->>'line_user_id'. Used by the chat customer-history panel
// (Phase 4.5). Capped to limit; no pagination — keep it simple.
func (r *BillRepo) ListByLineUserID(lineUserID string, limit int) ([]models.Bill, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	rows, err := r.db.Query(
		`SELECT id, bill_type, source, status, sml_doc_no, ai_confidence,
		        error_msg, created_at, sent_at
		 FROM bills
		 WHERE raw_data->>'line_user_id' = $1
		 ORDER BY created_at DESC
		 LIMIT $2`,
		lineUserID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("ListByLineUserID: %w", err)
	}
	defer rows.Close()
	var out []models.Bill
	for rows.Next() {
		b := models.Bill{}
		if err := rows.Scan(
			&b.ID, &b.BillType, &b.Source, &b.Status, &b.SMLDocNo,
			&b.AIConfidence, &b.ErrorMsg, &b.CreatedAt, &b.SentAt,
		); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (r *BillRepo) FindByID(id string) (*models.Bill, error) {
	b := &models.Bill{}
	var anomaliesRaw []byte
	var smlPayloadRaw, smlResponseRaw []byte
	err := r.db.QueryRow(
		`SELECT id, bill_type, source, status, raw_data, sml_doc_no,
		        sml_payload, sml_response, ai_confidence, anomalies,
		        error_msg, created_by, created_at, sent_at, remark
		 FROM bills WHERE id = $1`, id,
	).Scan(
		&b.ID, &b.BillType, &b.Source, &b.Status, &b.RawData,
		&b.SMLDocNo, &smlPayloadRaw, &smlResponseRaw, &b.AIConfidence,
		&anomaliesRaw, &b.ErrorMsg, &b.CreatedBy, &b.CreatedAt, &b.SentAt, &b.Remark,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("FindByID: %w", err)
	}
	b.Anomalies = anomaliesRaw
	if smlPayloadRaw != nil {
		b.SMLPayload = json.RawMessage(smlPayloadRaw)
	}
	if smlResponseRaw != nil {
		b.SMLResponse = json.RawMessage(smlResponseRaw)
	}

	items, err := r.findItems(id)
	if err != nil {
		return nil, err
	}
	b.Items = items
	return b, nil
}

func (r *BillRepo) List(f models.BillListFilter) ([]models.Bill, int, error) {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 || f.PageSize > 100 {
		f.PageSize = 20
	}

	where := "WHERE 1=1"
	args := []interface{}{}
	argN := 1

	if f.Status != "" {
		where += fmt.Sprintf(" AND b.status = $%d", argN)
		args = append(args, f.Status)
		argN++
	}
	if f.Source != "" {
		where += fmt.Sprintf(" AND b.source = $%d", argN)
		args = append(args, f.Source)
		argN++
	}
	if f.BillType != "" {
		where += fmt.Sprintf(" AND b.bill_type = $%d", argN)
		args = append(args, f.BillType)
		argN++
	}
	if f.Search != "" {
		where += fmt.Sprintf(
			" AND (b.sml_doc_no ILIKE $%d OR b.raw_data->>'customer_name' ILIKE $%d)",
			argN, argN,
		)
		args = append(args, "%"+f.Search+"%")
		argN++
	}

	countQuery := "SELECT COUNT(*) FROM bills b " + where
	var total int
	if err := r.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count: %w", err)
	}

	query := `SELECT b.id, b.bill_type, b.source, b.status, b.sml_doc_no, b.ai_confidence,
	                 b.anomalies, b.error_msg, b.created_at, b.sent_at,
	                 COALESCE(SUM(bi.qty * bi.price), 0) AS total_amount
	          FROM bills b
	          LEFT JOIN bill_items bi ON bi.bill_id = b.id
	          ` + where + `
	          GROUP BY b.id, b.bill_type, b.source, b.status, b.sml_doc_no, b.ai_confidence,
	                   b.anomalies, b.error_msg, b.created_at, b.sent_at
	          ORDER BY b.created_at DESC` +
		fmt.Sprintf(" LIMIT $%d OFFSET $%d", argN, argN+1)
	args = append(args, f.PageSize, (f.Page-1)*f.PageSize)

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("List bills: %w", err)
	}
	defer rows.Close()

	var bills []models.Bill
	for rows.Next() {
		var b models.Bill
		var anomaliesRaw []byte
		if err := rows.Scan(
			&b.ID, &b.BillType, &b.Source, &b.Status, &b.SMLDocNo, &b.AIConfidence,
			&anomaliesRaw, &b.ErrorMsg, &b.CreatedAt, &b.SentAt, &b.TotalAmount,
		); err != nil {
			return nil, 0, err
		}
		b.Anomalies = anomaliesRaw
		bills = append(bills, b)
	}
	return bills, total, rows.Err()
}

func (r *BillRepo) UpdateStatus(id, status string, smlDocNo *string, smlResponse json.RawMessage, errMsg *string) error {
	_, err := r.db.Exec(
		`UPDATE bills SET status=$1, sml_doc_no=$2, sml_response=$3,
		 error_msg=$4, sent_at=CASE WHEN $1='sent' THEN NOW() ELSE sent_at END
		 WHERE id=$5`,
		status, smlDocNo, smlResponse, errMsg, id,
	)
	return err
}

func (r *BillRepo) findItems(billID string) ([]models.BillItem, error) {
	rows, err := r.db.Query(
		`SELECT id, bill_id, raw_name, item_code, qty, unit_code, price, mapped, mapping_id,
		        COALESCE(candidates, '[]') as candidates
		 FROM bill_items WHERE bill_id = $1 ORDER BY id`, billID,
	)
	if err != nil {
		return nil, fmt.Errorf("findItems: %w", err)
	}
	defer rows.Close()

	var items []models.BillItem
	for rows.Next() {
		var item models.BillItem
		var candidatesRaw []byte
		if err := rows.Scan(
			&item.ID, &item.BillID, &item.RawName, &item.ItemCode,
			&item.Qty, &item.UnitCode, &item.Price, &item.Mapped, &item.MappingID,
			&candidatesRaw,
		); err != nil {
			return nil, err
		}
		if len(candidatesRaw) > 0 {
			item.Candidates = json.RawMessage(candidatesRaw)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *BillRepo) InsertItem(item *models.BillItem) error {
	return r.db.QueryRow(
		`INSERT INTO bill_items (bill_id, raw_name, item_code, qty, unit_code, price, mapped, mapping_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 RETURNING id`,
		item.BillID, item.RawName, item.ItemCode, item.Qty,
		item.UnitCode, item.Price, item.Mapped, item.MappingID,
	).Scan(&item.ID)
}

// DeleteItem removes a single bill_item row, scoped to the bill_id to prevent
// deleting items from a different bill via crafted item IDs.
func (r *BillRepo) DeleteItem(billID, itemID string) error {
	_, err := r.db.Exec(
		`DELETE FROM bill_items WHERE id = $1 AND bill_id = $2`,
		itemID, billID,
	)
	return err
}

// UpdateBillItem updates item_code, unit_code, mapping_id, and mapped flag for a bill item
func (r *BillRepo) UpdateBillItem(itemID, itemCode, unitCode, mappingID string, mapped bool) error {
	_, err := r.db.Exec(
		`UPDATE bill_items SET item_code=$1, unit_code=$2, mapping_id=$3, mapped=$4 WHERE id=$5`,
		itemCode, unitCode, mappingID, mapped, itemID,
	)
	return err
}

// UpdateBillItemFields applies a partial update to a bill_item row.
// Each pointer is applied only when non-nil; setting item_code also marks the row mapped.
func (r *BillRepo) UpdateBillItemFields(itemID string, itemCode, unitCode *string, qty, price *float64) error {
	sets := []string{}
	args := []interface{}{}
	idx := 1

	if itemCode != nil {
		sets = append(sets, fmt.Sprintf("item_code=$%d", idx))
		args = append(args, *itemCode)
		idx++
		sets = append(sets, fmt.Sprintf("mapped=$%d", idx))
		args = append(args, *itemCode != "")
		idx++
	}
	if unitCode != nil {
		sets = append(sets, fmt.Sprintf("unit_code=$%d", idx))
		args = append(args, *unitCode)
		idx++
	}
	if qty != nil {
		sets = append(sets, fmt.Sprintf("qty=$%d", idx))
		args = append(args, *qty)
		idx++
	}
	if price != nil {
		sets = append(sets, fmt.Sprintf("price=$%d", idx))
		args = append(args, *price)
		idx++
	}
	if len(sets) == 0 {
		return nil
	}
	args = append(args, itemID)
	query := fmt.Sprintf(`UPDATE bill_items SET %s WHERE id=$%d`, strings.Join(sets, ", "), idx)
	_, err := r.db.Exec(query, args...)
	return err
}

// DashboardStats returns aggregated counts for dashboard
func (r *BillRepo) DashboardStats() (map[string]interface{}, error) {
	stats := map[string]interface{}{}

	rows, err := r.db.Query(`SELECT status, COUNT(*) FROM bills GROUP BY status`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	total := 0
	pending, needsReview, confirmed, smlSuccess, smlFailed := 0, 0, 0, 0, 0
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		total += count
		switch status {
		case "pending":
			pending = count
		case "needs_review":
			needsReview = count
		case "confirmed":
			confirmed = count
		case "sent":
			smlSuccess = count
		case "failed":
			smlFailed = count
		}
	}
	stats["total_bills"] = total
	stats["pending"] = pending
	stats["needs_review"] = needsReview
	stats["confirmed"] = confirmed
	stats["sml_success"] = smlSuccess
	stats["sml_failed"] = smlFailed

	// Today's bill count
	var todayCount int
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM bills WHERE created_at >= CURRENT_DATE`).Scan(&todayCount)
	stats["today_bills"] = todayCount

	// Total amount from bill_items
	var totalAmount float64
	_ = r.db.QueryRow(`SELECT COALESCE(SUM(qty * price), 0) FROM bill_items WHERE price IS NOT NULL`).Scan(&totalAmount)
	stats["total_amount"] = totalAmount

	// F1: mapped vs unmapped
	var mappedCount, unmappedCount int
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM bill_items WHERE mapped = true`).Scan(&mappedCount)
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM bill_items WHERE mapped = false`).Scan(&unmappedCount)
	stats["items_mapped"] = mappedCount
	stats["items_unmapped"] = unmappedCount

	return stats, nil
}

// UpdateAnomalies stores anomaly results on a bill
func (r *BillRepo) UpdateAnomalies(id string, anomalies []models.Anomaly) error {
	data, err := json.Marshal(anomalies)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(`UPDATE bills SET anomalies = $1 WHERE id = $2`, data, id)
	return err
}

// UpdateSMLPayload saves the payload that was sent to SML
func (r *BillRepo) UpdateSMLPayload(id string, payload json.RawMessage) error {
	_, err := r.db.Exec(`UPDATE bills SET sml_payload = $1 WHERE id = $2`, payload, id)
	return err
}

func (r *BillRepo) UpdateRemark(id, remark string) error {
	_, err := r.db.Exec(`UPDATE bills SET remark = $1 WHERE id = $2`, remark, id)
	return err
}

// GetPriceHistories returns avg_price and max_price for each item code from historical data
func (r *BillRepo) GetPriceHistories(itemCodes []string) (map[string]float64, map[string]float64, error) {
	if len(itemCodes) == 0 {
		return nil, nil, nil
	}

	placeholders := make([]string, len(itemCodes))
	args := make([]interface{}, len(itemCodes))
	for i, code := range itemCodes {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = code
	}

	rows, err := r.db.Query(
		fmt.Sprintf(
			`SELECT item_code, avg_price, max_price FROM item_price_history WHERE item_code IN (%s)`,
			strings.Join(placeholders, ","),
		),
		args...,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("GetPriceHistories: %w", err)
	}
	defer rows.Close()

	avgPrices := make(map[string]float64)
	maxPrices := make(map[string]float64)
	for rows.Next() {
		var code string
		var avg, maxP float64
		if err := rows.Scan(&code, &avg, &maxP); err != nil {
			return nil, nil, err
		}
		avgPrices[code] = avg
		maxPrices[code] = maxP
	}
	return avgPrices, maxPrices, rows.Err()
}

// FindByEmailMessageID returns true if a bill with the given email Message-ID already exists.
// This prevents duplicate bills when IMAP re-processes the same email (e.g. mark-seen failed).
func (r *BillRepo) FindByEmailMessageID(messageID string) (bool, error) {
	var count int
	err := r.db.QueryRow(
		`SELECT COUNT(*) FROM bills WHERE raw_data->>'email_message_id' = $1`,
		messageID,
	).Scan(&count)
	return count > 0, err
}

// FindByShopeeOrderID returns true if a Shopee email bill for this order already exists
func (r *BillRepo) FindByShopeeOrderID(orderID string) (bool, error) {
	var count int
	err := r.db.QueryRow(
		`SELECT COUNT(*) FROM bills
		 WHERE source = 'shopee_email' AND (sml_order_id = $1 OR raw_data->>'shopee_order_id' = $1)`,
		orderID,
	).Scan(&count)
	return count > 0, err
}

// InsertItemWithCandidates inserts a bill item including top-5 catalog candidates
func (r *BillRepo) InsertItemWithCandidates(item *models.BillItem, candidatesJSON []byte) error {
	return r.db.QueryRow(
		`INSERT INTO bill_items (bill_id, raw_name, item_code, qty, unit_code, price, mapped, mapping_id, candidates)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 RETURNING id`,
		item.BillID, item.RawName, item.ItemCode, item.Qty,
		item.UnitCode, item.Price, item.Mapped, item.MappingID, candidatesJSON,
	).Scan(&item.ID)
}

// ExistsDuplicateToday checks if a bill with the same source, customer name, and item codes
// already exists today. Used by anomaly.DuplicateChecker.
func (r *BillRepo) ExistsDuplicateToday(source, customerName string, itemCodes []string) (bool, error) {
	if len(itemCodes) == 0 {
		return false, nil
	}
	placeholders := make([]string, len(itemCodes))
	args := []interface{}{source, customerName}
	for i, code := range itemCodes {
		placeholders[i] = fmt.Sprintf("$%d", i+3)
		args = append(args, code)
	}
	query := fmt.Sprintf(`
		SELECT COUNT(*) FROM bills b
		WHERE b.source = $1
		  AND b.raw_data->>'customer_name' ILIKE $2
		  AND b.created_at >= CURRENT_DATE
		  AND EXISTS (
		    SELECT 1 FROM bill_items bi
		    WHERE bi.bill_id = b.id
		      AND bi.item_code IN (%s)
		  )`, strings.Join(placeholders, ","))
	var count int
	err := r.db.QueryRow(query, args...).Scan(&count)
	return count > 0, err
}

// HasSeenCustomer returns true if any prior bill has this customer_name
// (case-insensitive). Used by anomaly.CustomerLookup for the "new_customer" warn rule.
func (r *BillRepo) HasSeenCustomer(customerName string) (bool, error) {
	if customerName == "" {
		return false, nil
	}
	var count int
	err := r.db.QueryRow(
		`SELECT COUNT(*) FROM bills WHERE raw_data->>'customer_name' ILIKE $1`,
		customerName,
	).Scan(&count)
	return count > 0, err
}

// UpdatePriceHistory updates rolling avg/min/max price statistics for each item
func (r *BillRepo) UpdatePriceHistory(items []models.BillItem) error {
	for _, item := range items {
		if item.ItemCode == nil || item.Price == nil || *item.Price <= 0 {
			continue
		}
		_, err := r.db.Exec(`
			INSERT INTO item_price_history (item_code, avg_price, min_price, max_price, sample_count, last_updated)
			VALUES ($1, $2, $2, $2, 1, NOW())
			ON CONFLICT (item_code) DO UPDATE SET
				avg_price    = (item_price_history.avg_price * item_price_history.sample_count + $2)
				              / (item_price_history.sample_count + 1),
				min_price    = LEAST(item_price_history.min_price, $2),
				max_price    = GREATEST(item_price_history.max_price, $2),
				sample_count = item_price_history.sample_count + 1,
				last_updated = NOW()
		`, *item.ItemCode, *item.Price)
		if err != nil {
			return fmt.Errorf("UpdatePriceHistory %s: %w", *item.ItemCode, err)
		}
	}
	return nil
}
