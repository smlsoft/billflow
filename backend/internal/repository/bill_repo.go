package repository

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"billflow/internal/models"
	"github.com/lib/pq"
)

type BillRepo struct {
	db *sql.DB
}

type BillListResult struct {
	Bills      []models.Bill
	Total      *int
	HasMore    bool
	NextCursor string
	Page       int
	PageSize   int
}

type BillQueueCounts struct {
	NeedsReview int `json:"needs_review"`
	Pending     int `json:"pending"`
	Sent        int `json:"sent"`
	Failed      int `json:"failed"`
	Skipped     int `json:"skipped"`
	Total       int `json:"total"`
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
		`INSERT INTO bills (bill_type, source, status, document_route, raw_data, ai_confidence, anomalies, created_by, sml_order_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 RETURNING id, created_at`,
		b.BillType, b.Source,
		coalesceStatus(b.Status, "pending"),
		b.DocumentRoute, raw, b.AIConfidence, anomalies, b.CreatedBy, orderID,
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
		`SELECT id, bill_type, source, status, document_route, raw_data, sml_doc_no,
		        sml_payload, sml_response, ai_confidence, anomalies,
		        error_msg, created_by, created_at, sent_at, archived_at, archived_by,
		        archive_reason, remark
		 FROM bills WHERE id = $1`, id,
	).Scan(
		&b.ID, &b.BillType, &b.Source, &b.Status, &b.DocumentRoute, &b.RawData,
		&b.SMLDocNo, &smlPayloadRaw, &smlResponseRaw, &b.AIConfidence,
		&anomaliesRaw, &b.ErrorMsg, &b.CreatedBy, &b.CreatedAt, &b.SentAt,
		&b.ArchivedAt, &b.ArchivedBy, &b.ArchiveReason, &b.Remark,
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
	enrichShopeeBillRawData(b, len(items), false)
	return b, nil
}

func (r *BillRepo) List(f models.BillListFilter) (*BillListResult, error) {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 || f.PageSize > 100 {
		f.PageSize = 20
	}
	if f.Limit < 1 || f.Limit > 100 {
		f.Limit = f.PageSize
	}

	where, args, argN := billWhere(f)
	var total *int
	legacyOffset := !f.CursorMode && !f.IncludeTotal
	if f.IncludeTotal || legacyOffset {
		var t int
		if err := r.db.QueryRow("SELECT COUNT(*) FROM bills b "+where, args...).Scan(&t); err != nil {
			return nil, fmt.Errorf("count: %w", err)
		}
		total = &t
	}

	useCursor := f.CursorMode
	if f.Cursor != "" {
		cursorTime, cursorID, err := decodeTimeIDCursor(f.Cursor)
		if err != nil {
			return nil, err
		}
		where += fmt.Sprintf(" AND (b.created_at, b.id) < ($%d::timestamptz, $%d::uuid)", argN, argN+1)
		args = append(args, cursorTime, cursorID)
		argN += 2
	}
	limit := f.PageSize
	if useCursor {
		limit = f.Limit
	}
	queryLimit := limit
	if useCursor {
		queryLimit = limit + 1
	}
	query := `SELECT b.id, b.bill_type, b.source, b.status, b.document_route, b.raw_data, b.sml_doc_no, b.ai_confidence,
	                 b.anomalies, b.error_msg, b.created_at, b.sent_at,
	                 b.archived_at, b.archived_by, b.archive_reason,
	                 COALESCE(SUM(bi.qty * bi.price), 0) AS total_amount,
	                 COUNT(bi.id) AS item_count
	          FROM bills b
	          LEFT JOIN bill_items bi ON bi.bill_id = b.id
	          ` + where + `
	          GROUP BY b.id, b.bill_type, b.source, b.status, b.document_route, b.raw_data, b.sml_doc_no, b.ai_confidence,
	                   b.anomalies, b.error_msg, b.created_at, b.sent_at, b.archived_at, b.archived_by, b.archive_reason
	          ORDER BY b.created_at DESC, b.id DESC` +
		fmt.Sprintf(" LIMIT $%d", argN)
	args = append(args, queryLimit)
	if !useCursor {
		query += fmt.Sprintf(" OFFSET $%d", argN+1)
		args = append(args, (f.Page-1)*f.PageSize)
	}

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("List bills: %w", err)
	}
	defer rows.Close()

	var bills []models.Bill
	for rows.Next() {
		var b models.Bill
		var anomaliesRaw []byte
		var itemCount int
		if err := rows.Scan(
			&b.ID, &b.BillType, &b.Source, &b.Status, &b.DocumentRoute, &b.RawData, &b.SMLDocNo, &b.AIConfidence,
			&anomaliesRaw, &b.ErrorMsg, &b.CreatedAt, &b.SentAt, &b.ArchivedAt, &b.ArchivedBy, &b.ArchiveReason,
			&b.TotalAmount, &itemCount,
		); err != nil {
			return nil, err
		}
		b.Anomalies = anomaliesRaw
		enrichShopeeBillRawData(&b, itemCount, true)
		bills = append(bills, b)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	hasMore := len(bills) > limit
	if hasMore {
		bills = bills[:limit]
	}
	nextCursor := ""
	if hasMore && len(bills) > 0 {
		last := bills[len(bills)-1]
		nextCursor = encodeTimeIDCursor(last.CreatedAt, last.ID)
	}
	return &BillListResult{
		Bills:      bills,
		Total:      total,
		HasMore:    hasMore,
		NextCursor: nextCursor,
		Page:       f.Page,
		PageSize:   limit,
	}, nil
}

func billWhere(f models.BillListFilter) (string, []interface{}, int) {
	where := "WHERE 1=1"
	args := []interface{}{}
	argN := 1

	switch f.Archived {
	case "include":
	case "only":
		where += " AND b.archived_at IS NOT NULL"
	default:
		where += " AND b.archived_at IS NULL"
	}
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
	if f.DocumentRoute != "" {
		where += fmt.Sprintf(" AND b.document_route = $%d", argN)
		args = append(args, f.DocumentRoute)
		argN++
	}
	if f.EmailAccountID != "" {
		where += fmt.Sprintf(" AND b.raw_data->>'imap_account_id' = $%d", argN)
		args = append(args, f.EmailAccountID)
		argN++
	}
	if f.ShopeeStatus != "" {
		where += fmt.Sprintf(` AND EXISTS (
			SELECT 1 FROM shopee_order_events soe
			WHERE soe.bill_id = b.id AND soe.event_type = $%d
		)`, argN)
		args = append(args, f.ShopeeStatus)
		argN++
	}
	if f.DateFrom != "" {
		where += fmt.Sprintf(" AND b.created_at >= $%d::date", argN)
		args = append(args, f.DateFrom)
		argN++
	}
	if f.DateTo != "" {
		where += fmt.Sprintf(" AND b.created_at < ($%d::date + INTERVAL '1 day')", argN)
		args = append(args, f.DateTo)
		argN++
	}
	if f.Search != "" {
		where += fmt.Sprintf(
			` AND (
			 b.sml_doc_no ILIKE $%d
			 OR b.raw_data->>'customer_name' ILIKE $%d
			 OR b.raw_data->>'order_id' ILIKE $%d
			 OR b.raw_data->>'shopee_order_id' ILIKE $%d
			 OR b.raw_data->>'seller_name' ILIKE $%d
			)`,
			argN, argN, argN, argN, argN,
		)
		args = append(args, "%"+f.Search+"%")
		argN++
	}
	return where, args, argN
}

func (r *BillRepo) QueueCounts(f models.BillListFilter) (BillQueueCounts, error) {
	f.Status = ""
	where, args, _ := billWhere(f)
	var c BillQueueCounts
	err := r.db.QueryRow(`
		SELECT
		  COUNT(*) FILTER (WHERE b.status='needs_review'),
		  COUNT(*) FILTER (WHERE b.status='pending'),
		  COUNT(*) FILTER (WHERE b.status='sent'),
		  COUNT(*) FILTER (WHERE b.status='failed'),
		  COUNT(*) FILTER (WHERE b.status='skipped'),
		  COUNT(*)
		FROM bills b `+where, args...).Scan(&c.NeedsReview, &c.Pending, &c.Sent, &c.Failed, &c.Skipped, &c.Total)
	return c, err
}

func (r *BillRepo) Archive(id, userID, reason string) error {
	if reason == "" {
		reason = "manual archive"
	}
	_, err := r.db.Exec(`
		UPDATE bills
		   SET archived_at = COALESCE(archived_at, NOW()),
		       archived_by = NULLIF($2, '')::UUID,
		       archive_reason = $3
		 WHERE id = $1`, id, userID, reason)
	return err
}

func (r *BillRepo) Restore(id string) error {
	_, err := r.db.Exec(`
		UPDATE bills
		   SET archived_at = NULL,
		       archived_by = NULL,
		       archive_reason = ''
		 WHERE id = $1`, id)
	return err
}

func (r *BillRepo) Delete(id string) error {
	_, err := r.db.Exec(`DELETE FROM bills WHERE id = $1`, id)
	return err
}

func billCursorForTest(t time.Time, id string) string {
	return encodeTimeIDCursor(t, id)
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
		`SELECT id, bill_id, raw_name, COALESCE(source_sku, ''), COALESCE(source_image_url, ''), item_code, qty, unit_code, price, mapped, mapping_id,
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
			&item.ID, &item.BillID, &item.RawName, &item.SourceSKU, &item.SourceImageURL,
			&item.ItemCode, &item.Qty, &item.UnitCode, &item.Price, &item.Mapped, &item.MappingID,
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
		`INSERT INTO bill_items (bill_id, raw_name, source_sku, source_image_url, item_code, qty, unit_code, price, mapped, mapping_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		 RETURNING id`,
		item.BillID, item.RawName, item.SourceSKU, item.SourceImageURL, item.ItemCode, item.Qty,
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

// ApplyVerifiedMappingToOpenItems applies a human-confirmed raw_name mapping to
// other open bills from the same source/bill_type. It also promotes any
// needs_review bill to pending once all of its rows are mapped.
func (r *BillRepo) ApplyVerifiedMappingToOpenItems(source, billType, rawName, itemCode, unitCode string) (int, int, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = tx.Rollback() }()

	itemResult, err := tx.Exec(
		`UPDATE bill_items bi
		 SET item_code = $1,
		     unit_code = $2,
		     mapped = TRUE,
		     mapping_id = (SELECT id FROM mappings WHERE raw_name = $3 LIMIT 1)
		 FROM bills b
		 WHERE bi.bill_id = b.id
		   AND b.source = $4
		   AND b.bill_type = $5
		   AND b.status IN ('pending', 'needs_review')
		   AND bi.raw_name = $3
		   AND (
		     COALESCE(bi.item_code, '') IS DISTINCT FROM $1 OR
		     COALESCE(bi.unit_code, '') IS DISTINCT FROM $2 OR
		     bi.mapped IS DISTINCT FROM TRUE OR
		     bi.mapping_id IS DISTINCT FROM (SELECT id FROM mappings WHERE raw_name = $3 LIMIT 1)
		   )`,
		itemCode, unitCode, rawName, source, billType,
	)
	if err != nil {
		return 0, 0, err
	}
	applied64, _ := itemResult.RowsAffected()

	readyResult, err := tx.Exec(
		`UPDATE bills b
		 SET status = 'pending',
		     error_msg = NULL
		 WHERE b.source = $1
		   AND b.bill_type = $2
		   AND b.status = 'needs_review'
		   AND EXISTS (
		     SELECT 1
		     FROM bill_items bi
		     WHERE bi.bill_id = b.id
		       AND bi.raw_name = $3
		   )
		   AND NOT EXISTS (
		     SELECT 1
		     FROM bill_items bi
		     WHERE bi.bill_id = b.id
		       AND (COALESCE(bi.item_code, '') = '' OR bi.mapped IS DISTINCT FROM TRUE)
		   )`,
		source, billType, rawName,
	)
	if err != nil {
		return 0, 0, err
	}
	ready64, _ := readyResult.RowsAffected()

	if err := tx.Commit(); err != nil {
		return 0, 0, err
	}
	return int(applied64), int(ready64), nil
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

	// Work queues used by the Phase 1+ dashboard. These mirror the two
	// first-class document menus so the dashboard can show where the work is
	// waiting instead of presenting one blended bill count.
	type queueStat struct {
		key      string
		sources  []string
		billType string
	}
	queues := []queueStat{
		{key: "purchase", sources: []string{"shopee_shipped"}, billType: "purchase"},
		{key: "sales", sources: []string{"shopee", "lazada", "tiktok"}, billType: "sale"},
	}
	for _, q := range queues {
		var totalQ, pendingQ, needsReviewQ, sentQ, failedQ int
		_ = r.db.QueryRow(`
			SELECT
			  COUNT(*),
			  COUNT(*) FILTER (WHERE status = 'pending'),
			  COUNT(*) FILTER (WHERE status = 'needs_review'),
			  COUNT(*) FILTER (WHERE status = 'sent'),
			  COUNT(*) FILTER (WHERE status = 'failed')
			FROM bills
			WHERE source = ANY($1) AND bill_type = $2`,
			pq.Array(q.sources),
			q.billType,
		).Scan(&totalQ, &pendingQ, &needsReviewQ, &sentQ, &failedQ)
		stats[q.key+"_total"] = totalQ
		stats[q.key+"_pending"] = pendingQ
		stats[q.key+"_needs_review"] = needsReviewQ
		stats[q.key+"_sent"] = sentQ
		stats[q.key+"_failed"] = failedQ
	}

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
		`SELECT
		   (SELECT COUNT(*) FROM bills WHERE raw_data->>'email_message_id' = $1) +
		   (SELECT COUNT(*) FROM processed_email_keys WHERE message_id = $1)`,
		messageID,
	).Scan(&count)
	return count > 0, err
}

// FindByShopeeOrderID returns true if a Shopee email bill for this order already exists
func (r *BillRepo) FindByShopeeOrderID(orderID string) (bool, error) {
	var count int
	err := r.db.QueryRow(
		`SELECT
		   (SELECT COUNT(*) FROM bills
		     WHERE source = 'shopee_email' AND (sml_order_id = $1 OR raw_data->>'shopee_order_id' = $1)) +
		   (SELECT COUNT(*) FROM processed_email_keys
		     WHERE source = 'shopee_email' AND order_id = $1)`,
		orderID,
	).Scan(&count)
	return count > 0, err
}

// HasProcessedEmailKey returns true when a durable email/order tombstone exists.
// It is used by IMAP processors so old mailbox messages do not recreate bills
// after a UAT cleanup deletes the bills table rows.
func (r *BillRepo) HasProcessedEmailKey(source, messageID, orderID string) (bool, error) {
	var count int
	err := r.db.QueryRow(
		`SELECT COUNT(*) FROM processed_email_keys
		  WHERE source = $1 AND message_id = $2 AND order_id = $3`,
		source, messageID, orderID,
	).Scan(&count)
	return count > 0, err
}

func (r *BillRepo) MarkProcessedEmailKey(source, messageID, orderID string) error {
	if messageID == "" {
		return nil
	}
	_, err := r.db.Exec(
		`INSERT INTO processed_email_keys (source, message_id, order_id)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (source, message_id, order_id) DO NOTHING`,
		source, messageID, orderID,
	)
	return err
}

// InsertItemWithCandidates inserts a bill item including top-5 catalog candidates
func (r *BillRepo) InsertItemWithCandidates(item *models.BillItem, candidatesJSON []byte) error {
	return r.db.QueryRow(
		`INSERT INTO bill_items (bill_id, raw_name, source_sku, source_image_url, item_code, qty, unit_code, price, mapped, mapping_id, candidates)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		 RETURNING id`,
		item.BillID, item.RawName, item.SourceSKU, item.SourceImageURL, item.ItemCode, item.Qty,
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
