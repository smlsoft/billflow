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
		        error_msg, created_by, created_at, sent_at, archived_at, archived_by, archive_reason, remark
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
	events, err := r.ListShopeeOrderEventsForBill(id)
	if err != nil {
		return nil, err
	}
	b.ShopeeEvents = events
	if len(events) > 0 {
		b.ShopeeStatus = &events[0]
	}
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

	switch f.Archived {
	case "include":
		// no-op: include active and stored bills
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

	var total int
	if f.ShopeeStatus == "" {
		countQuery := "SELECT COUNT(*) FROM bills b " + where
		if err := r.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
			return nil, 0, fmt.Errorf("count: %w", err)
		}
	}

	query := `SELECT b.id, b.bill_type, b.source, b.status, b.document_route, b.raw_data, b.sml_doc_no, b.ai_confidence,
	                 b.anomalies, b.error_msg, b.created_at, b.sent_at, b.archived_at, b.archived_by, b.archive_reason,
	                 COALESCE(SUM(bi.qty * bi.price), 0) AS total_amount,
	                 COUNT(bi.id) AS item_count
	          FROM bills b
	          LEFT JOIN bill_items bi ON bi.bill_id = b.id
	          ` + where + `
	          GROUP BY b.id, b.bill_type, b.source, b.status, b.document_route, b.raw_data, b.sml_doc_no, b.ai_confidence,
	                   b.anomalies, b.error_msg, b.created_at, b.sent_at, b.archived_at, b.archived_by, b.archive_reason
	          ORDER BY b.created_at DESC`
	if f.ShopeeStatus == "" {
		query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", argN, argN+1)
		args = append(args, f.PageSize, (f.Page-1)*f.PageSize)
	}

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("List bills: %w", err)
	}
	defer rows.Close()

	var bills []models.Bill
	for rows.Next() {
		var b models.Bill
		var anomaliesRaw []byte
		var itemCount int
		if err := rows.Scan(
			&b.ID, &b.BillType, &b.Source, &b.Status, &b.DocumentRoute, &b.RawData, &b.SMLDocNo, &b.AIConfidence,
			&anomaliesRaw, &b.ErrorMsg, &b.CreatedAt, &b.SentAt, &b.ArchivedAt, &b.ArchivedBy,
			&b.ArchiveReason, &b.TotalAmount, &itemCount,
		); err != nil {
			return nil, 0, err
		}
		b.Anomalies = anomaliesRaw
		enrichShopeeBillRawData(&b, itemCount, true)
		bills = append(bills, b)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	latestByBill, err := r.LatestShopeeOrderEventsForBills(billIDs(bills))
	if err != nil {
		return nil, 0, err
	}
	for i := range bills {
		bills[i].ShopeeStatus = latestByBill[bills[i].ID]
	}
	if f.ShopeeStatus != "" {
		filtered := make([]models.Bill, 0, len(bills))
		for _, b := range bills {
			if b.ShopeeStatus != nil && b.ShopeeStatus.EventType == f.ShopeeStatus {
				filtered = append(filtered, b)
			}
		}
		total = len(filtered)
		start := (f.Page - 1) * f.PageSize
		if start >= len(filtered) {
			return []models.Bill{}, total, nil
		}
		end := start + f.PageSize
		if end > len(filtered) {
			end = len(filtered)
		}
		bills = filtered[start:end]
	}
	return bills, total, nil
}

func billIDs(bills []models.Bill) []string {
	ids := make([]string, 0, len(bills))
	for _, b := range bills {
		ids = append(ids, b.ID)
	}
	return ids
}

type shopeeOrderEventScan struct {
	ID          sql.NullString
	BillID      sql.NullString
	OrderID     sql.NullString
	EventType   sql.NullString
	StatusLabel sql.NullString
	Subject     sql.NullString
	FromAddr    sql.NullString
	MessageID   sql.NullString
	EmailDate   sql.NullTime
	RawData     []byte
	CreatedAt   sql.NullTime
}

func (s shopeeOrderEventScan) event() *models.ShopeeOrderEvent {
	if !s.ID.Valid {
		return nil
	}
	var billID *string
	if s.BillID.Valid {
		v := s.BillID.String
		billID = &v
	}
	var emailDate *time.Time
	if s.EmailDate.Valid {
		v := s.EmailDate.Time
		emailDate = &v
	}
	return &models.ShopeeOrderEvent{
		ID:          s.ID.String,
		BillID:      billID,
		OrderID:     s.OrderID.String,
		EventType:   s.EventType.String,
		StatusLabel: s.StatusLabel.String,
		Subject:     s.Subject.String,
		FromAddr:    s.FromAddr.String,
		MessageID:   s.MessageID.String,
		EmailDate:   emailDate,
		RawData:     json.RawMessage(s.RawData),
		CreatedAt:   s.CreatedAt.Time,
	}
}

func (r *BillRepo) UpsertShopeeOrderEvent(e *models.ShopeeOrderEvent) error {
	if e == nil {
		return nil
	}
	if strings.TrimSpace(e.MessageID) == "" {
		e.MessageID = fmt.Sprintf("local-%s-%s-%d", e.OrderID, e.EventType, time.Now().UnixNano())
	}
	e.OrderID = normalizeShopeeOrderID(e.OrderID)
	raw := e.RawData
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	if e.BillID == nil && strings.TrimSpace(e.OrderID) != "" {
		var billID string
		err := r.db.QueryRow(
			`SELECT id
			   FROM bills
			  WHERE ltrim(COALESCE(sml_order_id, ''), '#') = $1
			     OR ltrim(COALESCE(raw_data->>'order_id', ''), '#') = $1
			     OR ltrim(COALESCE(raw_data->>'shopee_order_id', ''), '#') = $1
			     OR ltrim(COALESCE(raw_data->>'order_no', ''), '#') = $1
			  ORDER BY created_at DESC
			  LIMIT 1`,
			e.OrderID,
		).Scan(&billID)
		if err != nil && err != sql.ErrNoRows {
			return fmt.Errorf("find shopee event bill: %w", err)
		}
		if billID != "" {
			e.BillID = &billID
		}
	}
	err := r.db.QueryRow(
		`INSERT INTO shopee_order_events
		   (bill_id, order_id, event_type, status_label, subject, from_addr, message_id, email_date, raw_data)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 ON CONFLICT (message_id, order_id, event_type) DO UPDATE SET
		   bill_id = COALESCE(EXCLUDED.bill_id, shopee_order_events.bill_id),
		   status_label = EXCLUDED.status_label,
		   subject = EXCLUDED.subject,
		   from_addr = EXCLUDED.from_addr,
		   email_date = EXCLUDED.email_date,
		   raw_data = EXCLUDED.raw_data
		 RETURNING id, created_at`,
		e.BillID, e.OrderID, e.EventType, e.StatusLabel, e.Subject, e.FromAddr, e.MessageID, e.EmailDate, raw,
	).Scan(&e.ID, &e.CreatedAt)
	if err != nil {
		return fmt.Errorf("upsert shopee order event: %w", err)
	}
	return nil
}

func (r *BillRepo) LatestShopeeOrderEventForBill(billID string) (*models.ShopeeOrderEvent, error) {
	events, err := r.listShopeeOrderEventsForBill(billID, 1)
	if err != nil || len(events) == 0 {
		return nil, err
	}
	return &events[0], nil
}

func (r *BillRepo) LatestShopeeOrderEventsForBills(billIDs []string) (map[string]*models.ShopeeOrderEvent, error) {
	out := map[string]*models.ShopeeOrderEvent{}
	if len(billIDs) == 0 {
		return out, nil
	}
	rows, err := r.db.Query(
		`SELECT DISTINCT ON (e.bill_id)
		        e.bill_id AS owner_bill_id,
		        e.id, e.bill_id, e.order_id, e.event_type, e.status_label,
		        e.subject, e.from_addr, e.message_id, e.email_date, e.raw_data, e.created_at
		   FROM shopee_order_events e
		  WHERE e.bill_id = ANY($1)
		  ORDER BY e.bill_id, COALESCE(e.email_date, e.created_at) DESC, e.created_at DESC`,
		pq.Array(billIDs),
	)
	if err != nil {
		return nil, fmt.Errorf("latest shopee events for bills: %w", err)
	}
	defer rows.Close()

	if err := scanLatestShopeeRows(rows, out); err != nil {
		return nil, err
	}
	missingIDs := missingBillIDs(billIDs, out)
	if len(missingIDs) == 0 {
		return out, nil
	}

	fallbackRows, err := r.db.Query(
		`WITH target AS (
		   SELECT id, COALESCE(sml_order_id, '') AS sml_order_id, raw_data
		     FROM bills
		    WHERE id = ANY($1)
		 )
		 SELECT DISTINCT ON (t.id)
		        t.id AS owner_bill_id,
		        e.id, e.bill_id, e.order_id, e.event_type, e.status_label,
		        e.subject, e.from_addr, e.message_id, e.email_date, e.raw_data, e.created_at
		   FROM target t
		   JOIN shopee_order_events e
		     ON e.bill_id IS NULL
		    AND e.order_id <> ''
		    AND e.order_id IN (
		      NULLIF(ltrim(t.sml_order_id, '#'), ''),
		      NULLIF(ltrim(COALESCE(t.raw_data->>'order_id', ''), '#'), ''),
		      NULLIF(ltrim(COALESCE(t.raw_data->>'shopee_order_id', ''), '#'), ''),
		      NULLIF(ltrim(COALESCE(t.raw_data->>'order_no', ''), '#'), ''),
		      NULLIF(ltrim(COALESCE(t.raw_data->>'doc_ref', ''), '#'), '')
		    )
		  ORDER BY t.id, COALESCE(e.email_date, e.created_at) DESC, e.created_at DESC`,
		pq.Array(missingIDs),
	)
	if err != nil {
		return nil, fmt.Errorf("fallback latest shopee events for bills: %w", err)
	}
	defer fallbackRows.Close()
	if err := scanLatestShopeeRows(fallbackRows, out); err != nil {
		return nil, err
	}
	return out, nil
}

func scanLatestShopeeRows(rows *sql.Rows, out map[string]*models.ShopeeOrderEvent) error {
	for rows.Next() {
		var ownerBillID string
		var e models.ShopeeOrderEvent
		var raw []byte
		if err := rows.Scan(
			&ownerBillID, &e.ID, &e.BillID, &e.OrderID, &e.EventType, &e.StatusLabel,
			&e.Subject, &e.FromAddr, &e.MessageID, &e.EmailDate, &raw, &e.CreatedAt,
		); err != nil {
			return err
		}
		if len(raw) > 0 {
			e.RawData = json.RawMessage(raw)
		}
		event := e
		out[ownerBillID] = &event
	}
	return rows.Err()
}

func missingBillIDs(billIDs []string, events map[string]*models.ShopeeOrderEvent) []string {
	missing := make([]string, 0)
	for _, id := range billIDs {
		if events[id] == nil {
			missing = append(missing, id)
		}
	}
	return missing
}

func (r *BillRepo) ListShopeeOrderEventsForBill(billID string) ([]models.ShopeeOrderEvent, error) {
	return r.listShopeeOrderEventsForBill(billID, 50)
}

func (r *BillRepo) listShopeeOrderEventsForBill(billID string, limit int) ([]models.ShopeeOrderEvent, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.db.Query(
		`WITH target AS (
		   SELECT id, COALESCE(sml_order_id, '') AS sml_order_id, raw_data
		     FROM bills
		    WHERE id = $1
		 )
		 SELECT e.id, e.bill_id, e.order_id, e.event_type, e.status_label,
		        e.subject, e.from_addr, e.message_id, e.email_date, e.raw_data, e.created_at
		   FROM shopee_order_events e, target t
		  WHERE e.bill_id = t.id
		     OR (
		       e.order_id <> ''
		       AND e.order_id IN (
		         NULLIF(ltrim(t.sml_order_id, '#'), ''),
		         NULLIF(ltrim(COALESCE(t.raw_data->>'order_id', ''), '#'), ''),
		         NULLIF(ltrim(COALESCE(t.raw_data->>'shopee_order_id', ''), '#'), ''),
		         NULLIF(ltrim(COALESCE(t.raw_data->>'order_no', ''), '#'), ''),
		         NULLIF(ltrim(COALESCE(t.raw_data->>'doc_ref', ''), '#'), '')
		       )
		     )
		  ORDER BY COALESCE(e.email_date, e.created_at) DESC, e.created_at DESC
		  LIMIT $2`,
		billID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list shopee order events: %w", err)
	}
	defer rows.Close()

	var out []models.ShopeeOrderEvent
	for rows.Next() {
		var e models.ShopeeOrderEvent
		var raw []byte
		if err := rows.Scan(
			&e.ID, &e.BillID, &e.OrderID, &e.EventType, &e.StatusLabel,
			&e.Subject, &e.FromAddr, &e.MessageID, &e.EmailDate, &raw, &e.CreatedAt,
		); err != nil {
			return nil, err
		}
		if len(raw) > 0 {
			e.RawData = json.RawMessage(raw)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func normalizeShopeeOrderID(orderID string) string {
	return strings.TrimLeft(strings.TrimSpace(orderID), "#")
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

func (r *BillRepo) ArchiveBill(billID, userID, reason string) error {
	_, err := r.db.Exec(
		`UPDATE bills
		    SET archived_at = COALESCE(archived_at, NOW()),
		        archived_by = COALESCE(NULLIF($2, '')::uuid, archived_by),
		        archive_reason = $3
		  WHERE id = $1`,
		billID, userID, reason,
	)
	return err
}

func (r *BillRepo) RestoreBill(billID string) error {
	_, err := r.db.Exec(
		`UPDATE bills
		    SET archived_at = NULL,
		        archived_by = NULL,
		        archive_reason = ''
		  WHERE id = $1`,
		billID,
	)
	return err
}

func (r *BillRepo) BulkArchiveSentOlderThan(days int, userID, reason string) (int, error) {
	if days < 1 {
		days = 180
	}
	res, err := r.db.Exec(
		`UPDATE bills
		    SET archived_at = NOW(),
		        archived_by = NULLIF($2, '')::uuid,
		        archive_reason = $3
		  WHERE archived_at IS NULL
		    AND status IN ('sent', 'skipped')
		    AND created_at < NOW() - ($1::int * INTERVAL '1 day')`,
		days, userID, reason,
	)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func (r *BillRepo) ArchivedIDsOlderThan(days int, limit int) ([]string, error) {
	if days < 1 {
		days = 730
	}
	if limit <= 0 || limit > 1000 {
		limit = 500
	}
	rows, err := r.db.Query(
		`SELECT id
		   FROM bills
		  WHERE archived_at IS NOT NULL
		    AND archived_at < NOW() - ($1::int * INTERVAL '1 day')
		  ORDER BY archived_at ASC
		  LIMIT $2`,
		days, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *BillRepo) DeleteBill(billID string) (int, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`
		UPDATE mapping_feedback
		   SET bill_item_id = NULL
		 WHERE bill_item_id IN (SELECT id FROM bill_items WHERE bill_id = $1)`, billID); err != nil {
		return 0, err
	}
	res, err := tx.Exec(`DELETE FROM bills WHERE id = $1`, billID)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return int(n), nil
}

type OldDataSummary struct {
	ActiveTotal           int `json:"active_total"`
	ArchivedTotal         int `json:"archived_total"`
	SentOlderThan90Days   int `json:"sent_older_than_90_days"`
	SentOlderThan180Days  int `json:"sent_older_than_180_days"`
	SentOlderThan365Days  int `json:"sent_older_than_365_days"`
	PurgeEligible730Days  int `json:"purge_eligible_730_days"`
	ArchivedArtifactCount int `json:"archived_artifact_count"`
}

func (r *BillRepo) OldDataSummary() (OldDataSummary, error) {
	var s OldDataSummary
	err := r.db.QueryRow(`
		SELECT
		  COUNT(DISTINCT b.id) FILTER (WHERE b.archived_at IS NULL),
		  COUNT(DISTINCT b.id) FILTER (WHERE b.archived_at IS NOT NULL),
		  COUNT(DISTINCT b.id) FILTER (WHERE b.archived_at IS NULL AND b.status IN ('sent', 'skipped') AND b.created_at < NOW() - INTERVAL '90 days'),
		  COUNT(DISTINCT b.id) FILTER (WHERE b.archived_at IS NULL AND b.status IN ('sent', 'skipped') AND b.created_at < NOW() - INTERVAL '180 days'),
		  COUNT(DISTINCT b.id) FILTER (WHERE b.archived_at IS NULL AND b.status IN ('sent', 'skipped') AND b.created_at < NOW() - INTERVAL '365 days'),
		  COUNT(DISTINCT b.id) FILTER (WHERE b.archived_at IS NOT NULL AND b.archived_at < NOW() - INTERVAL '730 days'),
		  COUNT(ba.id) FILTER (WHERE b.archived_at IS NOT NULL)
		FROM bills b
		LEFT JOIN bill_artifacts ba ON ba.bill_id = b.id`,
	).Scan(
		&s.ActiveTotal,
		&s.ArchivedTotal,
		&s.SentOlderThan90Days,
		&s.SentOlderThan180Days,
		&s.SentOlderThan365Days,
		&s.PurgeEligible730Days,
		&s.ArchivedArtifactCount,
	)
	return s, err
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
