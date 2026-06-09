package repository

import (
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"billflow/internal/models"
	"github.com/lib/pq"
)

type emailGroupStats struct {
	OrderCount         int
	HasPrintableEmail  bool
	PrintCount         int
	LastPrintedAt      sql.NullTime
	LastPrintedByEmail string
	LastPrintedByName  string
}

type EmailPrintReadinessError struct {
	Message                        string
	MissingOrders                  []string
	MissingPaymentMethodOrders     []string
	NonMatchingPaymentMethodOrders []string
	RelatedOrderCount              int
	SMLDocCount                    int
	PrintPolicy                    models.MarketplacePrintPolicy
}

func (e *EmailPrintReadinessError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func (r *BillRepo) attachEmailGroups(bills []models.Bill) error {
	messageIDs := make([]string, 0, len(bills))
	seen := make(map[string]bool, len(bills))
	marketplaceCandidate := make(map[string]bool, len(bills))
	for i := range bills {
		messageID := billEmailMessageID(&bills[i])
		if messageID == "" {
			continue
		}
		isMarketplace := isMarketplacePurchaseEmail(bills[i].Source, bills[i].BillType)
		if !seen[messageID] {
			seen[messageID] = true
			messageIDs = append(messageIDs, messageID)
			marketplaceCandidate[messageID] = isMarketplace
			continue
		}
		if !isMarketplace {
			marketplaceCandidate[messageID] = false
		}
	}
	if len(messageIDs) == 0 {
		return nil
	}

	marketplaceMessageIDs := make([]string, 0, len(messageIDs))
	generalMessageIDs := make([]string, 0, len(messageIDs))
	for _, messageID := range messageIDs {
		if marketplaceCandidate[messageID] {
			marketplaceMessageIDs = append(marketplaceMessageIDs, messageID)
		} else {
			generalMessageIDs = append(generalMessageIDs, messageID)
		}
	}

	stats := make(map[string]emailGroupStats, len(messageIDs))
	rows, err := r.db.Query(`
		WITH marketplace_message_ids AS (
		  SELECT unnest($1::text[]) AS message_id
		),
		general_message_ids AS (
		  SELECT unnest($2::text[]) AS message_id
		),
		all_message_ids AS (
		  SELECT message_id FROM marketplace_message_ids
		  UNION
		  SELECT message_id FROM general_message_ids
		),
		matched_bills AS (
		  SELECT b.id, m.message_id
		    FROM marketplace_message_ids m
		    JOIN bills b ON b.raw_data->>'email_message_id' = m.message_id
		   WHERE b.raw_data ? 'email_message_id'
		     AND b.source IN ('shopee_shipped', 'lazada_email')
		     AND b.bill_type = 'purchase'
		  UNION ALL
		  SELECT b.id, m.message_id
		    FROM marketplace_message_ids m
		    JOIN bills b ON b.raw_data->>'message_id' = m.message_id
		   WHERE b.raw_data ? 'message_id'
		     AND COALESCE(b.raw_data->>'email_message_id', '') = ''
		     AND b.source IN ('shopee_shipped', 'lazada_email')
		     AND b.bill_type = 'purchase'
		  UNION ALL
		  SELECT b.id, m.message_id
		    FROM general_message_ids m
		    JOIN bills b ON b.raw_data->>'email_message_id' = m.message_id
		   WHERE b.raw_data ? 'email_message_id'
		  UNION ALL
		  SELECT b.id, m.message_id
		    FROM general_message_ids m
		    JOIN bills b ON b.raw_data->>'message_id' = m.message_id
		   WHERE b.raw_data ? 'message_id'
		     AND COALESCE(b.raw_data->>'email_message_id', '') = ''
		),
		group_stats AS (
		  SELECT message_id,
		         COUNT(DISTINCT mb.id)::int AS order_count,
		         COALESCE(BOOL_OR(kind IN ('email_html', 'email_text')), FALSE) AS has_printable_email
		    FROM matched_bills mb
		    LEFT JOIN bill_artifacts ba ON ba.bill_id = mb.id
		   WHERE message_id IS NOT NULL AND message_id <> ''
		   GROUP BY message_id
		),
		print_stats AS (
		  SELECT e.email_message_id AS message_id,
		         COUNT(e.id)::int AS print_count,
		         (ARRAY_AGG(e.created_at ORDER BY e.created_at DESC))[1] AS last_printed_at,
		         (ARRAY_AGG(e.requested_by_email ORDER BY e.created_at DESC))[1] AS last_printed_by_email,
		         (ARRAY_AGG(COALESCE(u.name, '') ORDER BY e.created_at DESC))[1] AS last_printed_by_name
		    FROM all_message_ids m
		    JOIN email_print_events e ON e.email_message_id = m.message_id
		    LEFT JOIN users u ON u.id = e.requested_by
		   GROUP BY e.email_message_id
		)
		SELECT gs.message_id,
		       gs.order_count,
		       gs.has_printable_email,
		       COALESCE(ps.print_count, 0) AS print_count,
		       ps.last_printed_at,
		       COALESCE(ps.last_printed_by_email, '') AS last_printed_by_email,
		       COALESCE(ps.last_printed_by_name, '') AS last_printed_by_name
			  FROM group_stats gs
			  LEFT JOIN print_stats ps ON ps.message_id = gs.message_id`,
		pq.Array(marketplaceMessageIDs),
		pq.Array(generalMessageIDs),
	)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var messageID string
		var stat emailGroupStats
		if err := rows.Scan(
			&messageID,
			&stat.OrderCount,
			&stat.HasPrintableEmail,
			&stat.PrintCount,
			&stat.LastPrintedAt,
			&stat.LastPrintedByEmail,
			&stat.LastPrintedByName,
		); err != nil {
			return err
		}
		stats[messageID] = stat
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for i := range bills {
		messageID := billEmailMessageID(&bills[i])
		if messageID == "" {
			continue
		}
		stat := stats[messageID]
		if stat.OrderCount == 0 {
			stat.OrderCount = 1
		}
		var lastPrintedAt *time.Time
		if stat.LastPrintedAt.Valid {
			t := stat.LastPrintedAt.Time
			lastPrintedAt = &t
		}
		bills[i].EmailGroup = &models.BillEmailGroup{
			MessageID:          messageID,
			GroupKey:           emailGroupKey(messageID),
			Subject:            billEmailSubject(&bills[i]),
			From:               billEmailFrom(&bills[i]),
			OrderCount:         stat.OrderCount,
			HasPrintableEmail:  stat.HasPrintableEmail,
			PrintCount:         stat.PrintCount,
			LastPrintedAt:      lastPrintedAt,
			LastPrintedByEmail: stat.LastPrintedByEmail,
			LastPrintedByName:  stat.LastPrintedByName,
		}
	}
	if err := r.attachPrintReadinessSummaries(bills); err != nil {
		return err
	}
	return nil
}

func (r *BillRepo) attachPrintReadinessSummary(b *models.Bill) {
	if b == nil || b.EmailGroup == nil || !isMarketplacePurchaseEmail(b.Source, b.BillType) {
		return
	}
	policy := r.channelPrintPolicy(b.Source, b.BillType)
	b.EmailGroup.PrintPolicy = policy
	b.EmailGroup.PrintPolicyNote = models.MarketplacePrintPolicyNote(policy)
	_, _, err := r.validateMarketplaceEmailPrintReady(b.ID, b.EmailGroup.MessageID)
	if err == nil {
		b.EmailGroup.PrintReady = true
		return
	}
	var readinessErr *EmailPrintReadinessError
	if errors.As(err, &readinessErr) {
		b.EmailGroup.PrintBlockReason = readinessErr.Message
		b.EmailGroup.MissingSMLOrders = readinessErr.MissingOrders
		b.EmailGroup.MissingPaymentMethodOrders = readinessErr.MissingPaymentMethodOrders
		b.EmailGroup.NonMatchingPaymentMethodOrders = readinessErr.NonMatchingPaymentMethodOrders
		return
	}
	b.EmailGroup.PrintBlockReason = "ตรวจสถานะพร้อมพิมพ์ไม่ได้"
}

func (r *BillRepo) attachPrintReadinessSummaries(bills []models.Bill) error {
	messageIDs := make([]string, 0, len(bills))
	seenMessages := map[string]bool{}
	for i := range bills {
		if bills[i].EmailGroup == nil || !isMarketplacePurchaseEmail(bills[i].Source, bills[i].BillType) {
			continue
		}
		messageID := strings.TrimSpace(bills[i].EmailGroup.MessageID)
		if messageID == "" || seenMessages[messageID] {
			continue
		}
		seenMessages[messageID] = true
		messageIDs = append(messageIDs, messageID)
	}
	if len(messageIDs) == 0 {
		return nil
	}

	rowsByMessageID, err := r.listMarketplacePrintRowsByMessageIDs(messageIDs)
	if err != nil {
		return err
	}
	policyCache := map[string]models.MarketplacePrintPolicy{}
	getPolicy := func(row marketplacePrintRow) models.MarketplacePrintPolicy {
		return r.channelPrintPolicyCached(policyCache, row.Source, row.BillType)
	}

	for i := range bills {
		if bills[i].EmailGroup == nil || !isMarketplacePurchaseEmail(bills[i].Source, bills[i].BillType) {
			continue
		}
		current := marketplacePrintRowFromBill(&bills[i])
		policy := getPolicy(current)
		bills[i].EmailGroup.PrintPolicy = policy
		bills[i].EmailGroup.PrintPolicyNote = models.MarketplacePrintPolicyNote(policy)

		_, _, err := evaluateMarketplacePrintRows(bills[i].ID, current, rowsByMessageID[bills[i].EmailGroup.MessageID], getPolicy)
		if err == nil {
			bills[i].EmailGroup.PrintReady = true
			continue
		}
		var readinessErr *EmailPrintReadinessError
		if errors.As(err, &readinessErr) {
			bills[i].EmailGroup.PrintBlockReason = readinessErr.Message
			bills[i].EmailGroup.MissingSMLOrders = readinessErr.MissingOrders
			bills[i].EmailGroup.MissingPaymentMethodOrders = readinessErr.MissingPaymentMethodOrders
			bills[i].EmailGroup.NonMatchingPaymentMethodOrders = readinessErr.NonMatchingPaymentMethodOrders
			continue
		}
		bills[i].EmailGroup.PrintBlockReason = "ตรวจสถานะพร้อมพิมพ์ไม่ได้"
	}
	return nil
}

func (r *BillRepo) attachEmailGroupDetails(b *models.Bill) error {
	if b == nil || b.EmailGroup == nil || b.EmailGroup.MessageID == "" {
		return nil
	}
	related, err := r.ListBillsByEmailMessageID(b.EmailGroup.MessageID, b.ID, 50)
	if err != nil {
		return err
	}
	prints, err := r.ListEmailPrintEvents(b.EmailGroup.MessageID, 20)
	if err != nil {
		return err
	}
	b.EmailGroup.RelatedBills = related
	b.EmailGroup.PrintEvents = prints
	return nil
}

func (r *BillRepo) ListBillsByEmailMessageID(messageID, currentBillID string, limit int) ([]models.BillEmailRelatedBill, error) {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return []models.BillEmailRelatedBill{}, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := r.db.Query(`
		SELECT b.id::text,
		       COALESCE(NULLIF(b.raw_data->>'order_id', ''), NULLIF(b.raw_data->>'shopee_order_id', ''), '') AS order_id,
		       COALESCE(NULLIF(b.sml_payload->>'cust_code', ''), '') AS party_code,
		       COALESCE(NULLIF(b.sml_payload->>'supplier_name', ''), '') AS party_name,
		       COALESCE(b.print_payment_method, '') AS print_payment_method,
		       `+effectivePrintPaymentMethodExpr("b")+` AS effective_print_payment_method,
		       b.source,
		       b.bill_type,
		       b.document_route,
		       b.status,
		       COALESCE(b.sml_doc_no, '') AS sml_doc_no,
		       b.created_at,
		       COALESCE(SUM(GREATEST(bi.qty * COALESCE(bi.price, 0) - COALESCE(bi.discount_amount, 0), 0)), 0)::float8 AS total_amount,
		       b.id::text = $2 AS is_current
		  FROM bills b
		  LEFT JOIN bill_items bi ON bi.bill_id = b.id
		 WHERE COALESCE(NULLIF(b.raw_data->>'email_message_id', ''), NULLIF(b.raw_data->>'message_id', '')) = $1
		 GROUP BY b.id, b.raw_data, b.source, b.bill_type, b.document_route, b.status,
		          b.sml_doc_no, b.sml_payload, b.print_payment_method, b.created_at
		 ORDER BY b.created_at DESC, b.id DESC
		 LIMIT $3`,
		messageID, currentBillID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []models.BillEmailRelatedBill{}
	for rows.Next() {
		var b models.BillEmailRelatedBill
		if err := rows.Scan(
			&b.ID, &b.OrderID, &b.PartyCode, &b.PartyName, &b.PrintPaymentMethod, &b.EffectivePrintPaymentMethod,
			&b.Source, &b.BillType, &b.DocumentRoute,
			&b.Status, &b.SMLDocNo, &b.CreatedAt, &b.TotalAmount, &b.IsCurrent,
		); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (r *BillRepo) ListEmailPrintEvents(messageID string, limit int) ([]models.EmailPrintEvent, error) {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return []models.EmailPrintEvent{}, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := r.db.Query(`
		SELECT e.id::text,
		       e.bill_id::text,
		       COALESCE(e.artifact_id::text, '') AS artifact_id,
		       e.email_message_id,
		       e.email_group_key,
		       e.subject,
		       e.from_addr,
		       COALESCE(e.requested_by::text, '') AS requested_by,
		       e.requested_by_email,
		       COALESCE(u.name, '') AS requested_by_name,
		       e.created_at
		  FROM email_print_events e
		  LEFT JOIN users u ON u.id = e.requested_by
		 WHERE e.email_message_id = $1
		 ORDER BY e.created_at DESC
		 LIMIT $2`,
		messageID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []models.EmailPrintEvent{}
	for rows.Next() {
		var event models.EmailPrintEvent
		if err := rows.Scan(
			&event.ID, &event.BillID, &event.ArtifactID, &event.EmailMessageID,
			&event.EmailGroupKey, &event.Subject, &event.From, &event.RequestedBy,
			&event.RequestedByEmail, &event.RequestedByName, &event.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, event)
	}
	return out, rows.Err()
}

func (r *BillRepo) RecordEmailPrintEvent(billID, artifactID, userID, userEmail string) (*models.EmailPrintEvent, error) {
	return r.recordEmailPrintEventWithExecutor(r.db, billID, artifactID, userID, userEmail)
}

type emailPrintEventExecutor interface {
	QueryRow(query string, args ...any) *sql.Row
}

func (r *BillRepo) recordEmailPrintEventWithExecutor(exec emailPrintEventExecutor, billID, artifactID, userID, userEmail string) (*models.EmailPrintEvent, error) {
	billID = strings.TrimSpace(billID)
	artifactID = strings.TrimSpace(artifactID)
	if billID == "" || artifactID == "" {
		return nil, fmt.Errorf("bill_id and artifact_id are required")
	}

	var messageID, subject, fromAddr, kind string
	err := exec.QueryRow(`
		SELECT COALESCE(NULLIF(ba.source_meta->>'message_id', ''), NULLIF(b.raw_data->>'email_message_id', ''), NULLIF(b.raw_data->>'message_id', '')) AS message_id,
		       COALESCE(NULLIF(ba.source_meta->>'subject', ''), b.raw_data->>'subject', '') AS subject,
		       COALESCE(NULLIF(ba.source_meta->>'from', ''), NULLIF(b.raw_data->>'from', ''), NULLIF(b.raw_data->>'from_addr', ''), '') AS from_addr,
		       ba.kind
		  FROM bills b
		  JOIN bill_artifacts ba ON ba.bill_id = b.id
		 WHERE b.id = $1 AND ba.id = $2`,
		billID, artifactID,
	).Scan(&messageID, &subject, &fromAddr, &kind)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !isPrintableEmailKind(kind) {
		return nil, fmt.Errorf("artifact is not a printable email")
	}
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return nil, fmt.Errorf("bill has no email message id")
	}

	relatedOrderCount, smlDocCount, err := r.validateMarketplaceEmailPrintReady(billID, messageID)
	if err != nil {
		return nil, err
	}

	requestedBy := strings.TrimSpace(userID)
	event := &models.EmailPrintEvent{}
	err = exec.QueryRow(`
		INSERT INTO email_print_events
		  (bill_id, artifact_id, email_message_id, email_group_key, subject, from_addr, requested_by, requested_by_email)
		VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, '')::uuid, $8)
		RETURNING id::text, bill_id::text, artifact_id::text, email_message_id, email_group_key,
		          subject, from_addr, COALESCE(requested_by::text, ''), requested_by_email, created_at`,
		billID, artifactID, messageID, emailGroupKey(messageID), subject, fromAddr, requestedBy, strings.TrimSpace(userEmail),
	).Scan(
		&event.ID, &event.BillID, &event.ArtifactID, &event.EmailMessageID,
		&event.EmailGroupKey, &event.Subject, &event.From, &event.RequestedBy,
		&event.RequestedByEmail, &event.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	event.RequestedByName = ""
	event.RelatedOrderCount = relatedOrderCount
	event.SMLDocCount = smlDocCount
	return event, nil
}

func (r *BillRepo) validateMarketplaceEmailPrintReady(billID, messageID string) (int, int, error) {
	var source, billType, status, currentOrderID, currentDocNo, currentPartyCode, currentPartyName, currentPrintPaymentMethod, currentEffectivePaymentMethod string
	err := r.db.QueryRow(`
		SELECT source,
		       bill_type,
		       status,
		       COALESCE(NULLIF(raw_data->>'order_id', ''), NULLIF(raw_data->>'shopee_order_id', ''), id::text) AS order_id,
		       COALESCE(sml_doc_no, '') AS sml_doc_no,
		       COALESCE(NULLIF(sml_payload->>'cust_code', ''), '') AS party_code,
		       COALESCE(NULLIF(sml_payload->>'supplier_name', ''), '') AS party_name,
		       COALESCE(print_payment_method, '') AS print_payment_method,
		       `+effectivePrintPaymentMethodExpr("bills")+` AS effective_print_payment_method
		  FROM bills
		 WHERE id = $1`,
		billID,
	).Scan(&source, &billType, &status, &currentOrderID, &currentDocNo, &currentPartyCode, &currentPartyName, &currentPrintPaymentMethod, &currentEffectivePaymentMethod)
	if err != nil {
		return 0, 0, err
	}
	if !isMarketplacePurchaseEmail(source, billType) {
		return 0, 0, nil
	}

	policyCache := map[string]models.MarketplacePrintPolicy{
		source + "/" + billType: r.channelPrintPolicy(source, billType),
	}
	getPolicy := func(row marketplacePrintRow) models.MarketplacePrintPolicy {
		return r.channelPrintPolicyCached(policyCache, row.Source, row.BillType)
	}
	rows, err := r.listMarketplacePrintRows(messageID)
	if err != nil {
		return 0, 0, err
	}
	current := marketplacePrintRow{
		ID:                          billID,
		OrderID:                     currentOrderID,
		Source:                      source,
		BillType:                    billType,
		Status:                      status,
		SMLDocNo:                    currentDocNo,
		PartyCode:                   currentPartyCode,
		PartyName:                   currentPartyName,
		PrintPaymentMethod:          currentPrintPaymentMethod,
		EffectivePrintPaymentMethod: currentEffectivePaymentMethod,
	}
	return evaluateMarketplacePrintRows(billID, current, rows, getPolicy)
}

func isMarketplacePurchaseEmail(source, billType string) bool {
	if billType != "purchase" {
		return false
	}
	switch source {
	case "shopee_shipped", "lazada_email":
		return true
	default:
		return false
	}
}

func isPrintableEmailKind(kind string) bool {
	switch kind {
	case "email_html", "email_text":
		return true
	default:
		return false
	}
}

func billEmailMessageID(b *models.Bill) string {
	if b == nil {
		return ""
	}
	for _, key := range []string{"email_message_id", "message_id"} {
		if v := billRawString(b.RawData, key); v != "" {
			return v
		}
	}
	if b.ShopeeStatus != nil && b.ShopeeStatus.MessageID != "" {
		return strings.TrimSpace(b.ShopeeStatus.MessageID)
	}
	for _, event := range b.ShopeeEvents {
		if event.MessageID != "" {
			return strings.TrimSpace(event.MessageID)
		}
	}
	return ""
}

func billEmailSubject(b *models.Bill) string {
	if b == nil {
		return ""
	}
	if v := billRawString(b.RawData, "subject"); v != "" {
		return v
	}
	if b.ShopeeStatus != nil && b.ShopeeStatus.Subject != "" {
		return strings.TrimSpace(b.ShopeeStatus.Subject)
	}
	for _, event := range b.ShopeeEvents {
		if event.Subject != "" {
			return strings.TrimSpace(event.Subject)
		}
	}
	return ""
}

func billEmailFrom(b *models.Bill) string {
	if b == nil {
		return ""
	}
	for _, key := range []string{"from", "from_addr"} {
		if v := billRawString(b.RawData, key); v != "" {
			return v
		}
	}
	if b.ShopeeStatus != nil && b.ShopeeStatus.FromAddr != "" {
		return strings.TrimSpace(b.ShopeeStatus.FromAddr)
	}
	for _, event := range b.ShopeeEvents {
		if event.FromAddr != "" {
			return strings.TrimSpace(event.FromAddr)
		}
	}
	return ""
}

func billRawString(raw json.RawMessage, key string) string {
	if len(raw) == 0 || key == "" {
		return ""
	}
	var data map[string]interface{}
	if err := json.Unmarshal(raw, &data); err != nil {
		return ""
	}
	v, ok := data[key]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	default:
		return strings.TrimSpace(strings.TrimPrefix(strings.TrimSuffix(toJSONString(t), `"`), `"`))
	}
}

func toJSONString(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

func emailGroupKey(messageID string) string {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return ""
	}
	sum := sha1.Sum([]byte(messageID))
	encoded := strings.ToUpper(hex.EncodeToString(sum[:]))
	if len(encoded) > 6 {
		return encoded[:6]
	}
	return encoded
}
