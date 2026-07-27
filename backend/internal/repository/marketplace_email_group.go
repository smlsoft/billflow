package repository

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"billflow/internal/models"
	"github.com/lib/pq"
)

const (
	marketplaceEmailGroupProcessing = "processing"
	marketplaceEmailGroupComplete   = "complete"
	marketplaceEmailGroupAttention  = "attention"

	marketplaceEmailGroupOrderExpected = "expected"
	marketplaceEmailGroupOrderCreated  = "created"
	marketplaceEmailGroupOrderExisting = "existing"
	marketplaceEmailGroupOrderMissing  = "missing"
	marketplaceEmailGroupOrderFailed   = "failed"
	marketplaceEmailGroupOrderArchived = "archived"
)

type marketplaceEmailGroupOrderState struct {
	Status string
}

type marketplaceEmailGroupOrderCandidate struct {
	OrderRowID  string
	OrderID     string
	BillID      string
	Archived    bool
	SameMessage bool
}

func marketplaceEmailGroupCounts(orders []marketplaceEmailGroupOrderState) (resolved, missing int, status string) {
	for _, order := range orders {
		switch order.Status {
		case marketplaceEmailGroupOrderCreated, marketplaceEmailGroupOrderExisting:
			resolved++
		default:
			missing++
		}
	}
	if len(orders) > 0 && missing == 0 {
		return resolved, 0, marketplaceEmailGroupComplete
	}
	return resolved, missing, marketplaceEmailGroupAttention
}

type MarketplaceEmailGroupRepo struct {
	db *sql.DB
}

type MarketplaceEmailGroupInput struct {
	Source        string
	MessageID     string
	IMAPAccountID string
	IMAPMailbox   string
	Subject       string
	From          string
	OrderIDs      []string
}

type MarketplaceEmailGroupRef struct {
	Source    string
	MessageID string
}

func NewMarketplaceEmailGroupRepo(db *sql.DB) *MarketplaceEmailGroupRepo {
	return &MarketplaceEmailGroupRepo{db: db}
}

func normalizeMarketplaceOrderIDs(ids []string) []string {
	unique := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		id = strings.ToUpper(strings.TrimSpace(strings.TrimPrefix(id, "#")))
		if id != "" {
			unique[id] = struct{}{}
		}
	}
	out := make([]string, 0, len(unique))
	for id := range unique {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func (r *MarketplaceEmailGroupRepo) RegisterExpectedOrders(input MarketplaceEmailGroupInput) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("marketplace email group repository is not configured")
	}
	input.Source = strings.TrimSpace(input.Source)
	input.MessageID = strings.TrimSpace(input.MessageID)
	orderIDs := normalizeMarketplaceOrderIDs(input.OrderIDs)
	if input.Source == "" || input.MessageID == "" || len(orderIDs) == 0 {
		return fmt.Errorf("source, message id, and at least one order id are required")
	}

	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var accountID any
	if strings.TrimSpace(input.IMAPAccountID) != "" {
		accountID = input.IMAPAccountID
	}
	var groupID string
	err = tx.QueryRow(`
		INSERT INTO marketplace_email_groups
		  (source, message_id, imap_account_id, imap_mailbox, subject, from_addr, status, expected_order_count)
		VALUES ($1, $2, $3, $4, $5, $6, 'processing', $7)
		ON CONFLICT (source, message_id) DO UPDATE SET
		  imap_account_id = COALESCE(EXCLUDED.imap_account_id, marketplace_email_groups.imap_account_id),
		  imap_mailbox = EXCLUDED.imap_mailbox,
		  subject = EXCLUDED.subject,
		  from_addr = EXCLUDED.from_addr,
		  status = 'processing',
		  failure_code = '',
		  failure_detail = '{}'::jsonb,
		  attempt_count = marketplace_email_groups.attempt_count + 1,
		  last_seen_at = NOW(),
		  completed_at = NULL
		RETURNING id::text`,
		input.Source, input.MessageID, accountID, strings.TrimSpace(input.IMAPMailbox),
		strings.TrimSpace(input.Subject), strings.TrimSpace(input.From), len(orderIDs),
	).Scan(&groupID)
	if err != nil {
		return fmt.Errorf("upsert marketplace email group: %w", err)
	}

	for _, orderID := range orderIDs {
		if _, err := tx.Exec(`
			INSERT INTO marketplace_email_group_orders (group_id, order_id)
			VALUES ($1::uuid, $2)
			ON CONFLICT (group_id, order_id) DO UPDATE SET updated_at = NOW()`, groupID, orderID); err != nil {
			return fmt.Errorf("upsert marketplace email group order: %w", err)
		}
	}
	if _, err := tx.Exec(`
		UPDATE marketplace_email_groups
		   SET expected_order_count = (
		     SELECT COUNT(*) FROM marketplace_email_group_orders WHERE group_id = $1::uuid
		   )
		 WHERE id = $1::uuid`, groupID); err != nil {
		return fmt.Errorf("update expected order count: %w", err)
	}
	return tx.Commit()
}

func (r *MarketplaceEmailGroupRepo) Finalize(source, messageID, failureCode string, failureDetail map[string]interface{}) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("marketplace email group repository is not configured")
	}
	source = strings.TrimSpace(source)
	messageID = strings.TrimSpace(messageID)
	if source == "" || messageID == "" {
		return nil
	}
	detail, err := json.Marshal(failureDetail)
	if err != nil {
		return fmt.Errorf("marshal marketplace email group failure detail: %w", err)
	}

	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var groupID string
	err = tx.QueryRow(`SELECT id::text FROM marketplace_email_groups WHERE source=$1 AND message_id=$2 FOR UPDATE`, source, messageID).Scan(&groupID)
	if err == sql.ErrNoRows {
		return tx.Commit()
	}
	if err != nil {
		return fmt.Errorf("lock marketplace email group: %w", err)
	}

	rows, err := tx.Query(`
		SELECT o.id::text,
		       o.order_id,
		       COALESCE(candidate.id::text, ''),
		       COALESCE(candidate.archived_at IS NOT NULL, FALSE),
		       COALESCE(COALESCE(candidate.raw_data->>'email_message_id', candidate.raw_data->>'message_id', '') = $2, FALSE)
		  FROM marketplace_email_group_orders o
		  LEFT JOIN LATERAL (
		    SELECT b.id, b.archived_at, b.raw_data
		      FROM bills b
		     WHERE b.source = $1
		       AND UPPER(TRIM(LEADING '#' FROM COALESCE(b.raw_data->>'order_id', ''))) = o.order_id
	     ORDER BY (b.archived_at IS NULL) DESC,
	              (COALESCE(b.raw_data->>'email_message_id', b.raw_data->>'message_id', '') = $2) DESC,
		              b.created_at ASC
		     LIMIT 1
		  ) candidate ON TRUE
		 WHERE o.group_id = $3::uuid
		 ORDER BY o.order_id`, source, messageID, groupID)
	if err != nil {
		return fmt.Errorf("find marketplace email group bills: %w", err)
	}
	defer rows.Close()

	candidates := make([]marketplaceEmailGroupOrderCandidate, 0)
	for rows.Next() {
		candidate := marketplaceEmailGroupOrderCandidate{}
		if err := rows.Scan(
			&candidate.OrderRowID,
			&candidate.OrderID,
			&candidate.BillID,
			&candidate.Archived,
			&candidate.SameMessage,
		); err != nil {
			return err
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	// PostgreSQL uses one connection per transaction. Release the result set
	// before issuing UPDATE statements on that same transaction.
	if err := rows.Close(); err != nil {
		return err
	}

	states := make([]marketplaceEmailGroupOrderState, 0, len(candidates))
	for _, candidate := range candidates {
		orderStatus := marketplaceEmailGroupOrderMissing
		errorCode := failureCode
		var billIDArg any
		if candidate.BillID != "" && !candidate.Archived {
			billIDArg = candidate.BillID
			errorCode = ""
			if candidate.SameMessage {
				orderStatus = marketplaceEmailGroupOrderCreated
			} else {
				orderStatus = marketplaceEmailGroupOrderExisting
			}
		} else if candidate.Archived {
			orderStatus = marketplaceEmailGroupOrderArchived
			errorCode = "bill_archived"
			billIDArg = candidate.BillID
		} else if failureCode != "" {
			orderStatus = marketplaceEmailGroupOrderFailed
		}
		states = append(states, marketplaceEmailGroupOrderState{Status: orderStatus})
		if _, err := tx.Exec(`
			UPDATE marketplace_email_group_orders
			   SET bill_id=$2,
			       status=$3,
			       error_code=$4,
			       error_detail=CASE WHEN $4='' THEN '{}'::jsonb ELSE $5::jsonb END,
			       resolved_at=CASE WHEN $3 IN ('created','existing') THEN NOW() ELSE NULL END,
			       updated_at=NOW()
			 WHERE id=$1::uuid`, candidate.OrderRowID, billIDArg, orderStatus, errorCode, string(detail)); err != nil {
			return fmt.Errorf("update marketplace email group order %s: %w", candidate.OrderID, err)
		}
	}
	resolved, missing, groupStatus := marketplaceEmailGroupCounts(states)
	if groupStatus == marketplaceEmailGroupComplete {
		failureCode = ""
		detail = []byte("{}")
	}
	if _, err := tx.Exec(`
		UPDATE marketplace_email_groups
		   SET status=$2,
		       resolved_order_count=$3,
		       missing_order_count=$4,
		       failure_code=$5,
		       failure_detail=$6::jsonb,
		       last_seen_at=NOW(),
		       completed_at=CASE WHEN $2='complete' THEN NOW() ELSE NULL END
		 WHERE id=$1::uuid`, groupID, groupStatus, resolved, missing, failureCode, string(detail)); err != nil {
		return fmt.Errorf("finalize marketplace email group: %w", err)
	}
	return tx.Commit()
}

func (r *MarketplaceEmailGroupRepo) Get(source, messageID string) (*models.MarketplaceEmailGroup, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	group, err := scanMarketplaceEmailGroup(r.db.QueryRow(`
		SELECT id::text, source, message_id, COALESCE(imap_account_id::text, ''), imap_mailbox,
		       subject, from_addr, status, expected_order_count, resolved_order_count,
		       missing_order_count, failure_code
		  FROM marketplace_email_groups
		 WHERE source=$1 AND message_id=$2`, strings.TrimSpace(source), strings.TrimSpace(messageID)))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	orders, err := r.listOrders(group.ID)
	if err != nil {
		return nil, err
	}
	group.Orders = orders
	return group, nil
}

func (r *MarketplaceEmailGroupRepo) ListByRefs(refs []MarketplaceEmailGroupRef) (map[string]*models.MarketplaceEmailGroup, error) {
	out := map[string]*models.MarketplaceEmailGroup{}
	if r == nil || r.db == nil || len(refs) == 0 {
		return out, nil
	}
	sources := make([]string, 0, len(refs))
	messageIDs := make([]string, 0, len(refs))
	seen := map[string]bool{}
	for _, ref := range refs {
		ref.Source = strings.TrimSpace(ref.Source)
		ref.MessageID = strings.TrimSpace(ref.MessageID)
		key := marketplaceEmailGroupRefKey(ref.Source, ref.MessageID)
		if ref.Source == "" || ref.MessageID == "" || seen[key] {
			continue
		}
		seen[key] = true
		sources = append(sources, ref.Source)
		messageIDs = append(messageIDs, ref.MessageID)
	}
	if len(sources) == 0 {
		return out, nil
	}
	rows, err := r.db.Query(`
		SELECT g.id::text, g.source, g.message_id, COALESCE(g.imap_account_id::text, ''), g.imap_mailbox,
		       g.subject, g.from_addr, g.status, g.expected_order_count, g.resolved_order_count,
		       g.missing_order_count, g.failure_code
		  FROM marketplace_email_groups g
		  JOIN unnest($1::text[], $2::text[]) AS refs(source, message_id)
		    ON g.source=refs.source AND g.message_id=refs.message_id`, pq.Array(sources), pq.Array(messageIDs))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		group, err := scanMarketplaceEmailGroup(rows)
		if err != nil {
			return nil, err
		}
		out[marketplaceEmailGroupRefKey(group.Source, group.MessageID)] = group
	}
	return out, rows.Err()
}

// ListAttention returns recent source emails that have not been reconciled.
// It intentionally does not depend on a bill existing: an AI or parser failure
// can prevent every expected order in an email from creating a bill.
func (r *MarketplaceEmailGroupRepo) ListAttention(source, imapAccountID string, limit int) ([]models.MarketplaceEmailGroup, error) {
	if r == nil || r.db == nil {
		return []models.MarketplaceEmailGroup{}, nil
	}
	if limit < 1 || limit > 50 {
		limit = 20
	}
	rows, err := r.db.Query(`
		SELECT g.id::text, g.source, g.message_id, COALESCE(g.imap_account_id::text, ''), g.imap_mailbox,
		       g.subject, g.from_addr, g.status, g.expected_order_count, g.resolved_order_count,
		       g.missing_order_count, g.failure_code, COALESCE(active_bill.id::text, '')
		  FROM marketplace_email_groups g
		  LEFT JOIN LATERAL (
		    SELECT b.id
		      FROM marketplace_email_group_orders o
		      JOIN bills b ON b.id = o.bill_id
		     WHERE o.group_id = g.id
		       AND b.archived_at IS NULL
		     ORDER BY b.created_at ASC
		     LIMIT 1
		  ) active_bill ON TRUE
		 WHERE g.status IN ('processing', 'attention')
		   AND ($1 = '' OR g.source = $1)
		   AND ($2 = '' OR g.imap_account_id::text = $2)
		 ORDER BY g.last_seen_at DESC
		 LIMIT $3`, strings.TrimSpace(source), strings.TrimSpace(imapAccountID), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	groups := make([]models.MarketplaceEmailGroup, 0)
	for rows.Next() {
		group := models.MarketplaceEmailGroup{}
		if err := rows.Scan(
			&group.ID, &group.Source, &group.MessageID, &group.IMAPAccountID, &group.IMAPMailbox,
			&group.Subject, &group.From, &group.Status, &group.ExpectedOrderCount, &group.ResolvedOrderCount,
			&group.MissingOrderCount, &group.FailureCode, &group.RepresentativeBillID,
		); err != nil {
			return nil, err
		}
		groups = append(groups, group)
	}
	return groups, rows.Err()
}

type marketplaceEmailGroupScanner interface {
	Scan(dest ...any) error
}

func scanMarketplaceEmailGroup(scanner marketplaceEmailGroupScanner) (*models.MarketplaceEmailGroup, error) {
	group := &models.MarketplaceEmailGroup{}
	err := scanner.Scan(
		&group.ID, &group.Source, &group.MessageID, &group.IMAPAccountID, &group.IMAPMailbox,
		&group.Subject, &group.From, &group.Status, &group.ExpectedOrderCount, &group.ResolvedOrderCount,
		&group.MissingOrderCount, &group.FailureCode,
	)
	return group, err
}

func (r *MarketplaceEmailGroupRepo) listOrders(groupID string) ([]models.MarketplaceEmailGroupOrder, error) {
	rows, err := r.db.Query(`
		SELECT order_id, COALESCE(bill_id::text, ''), status, error_code
		  FROM marketplace_email_group_orders
		 WHERE group_id=$1::uuid
		 ORDER BY order_id`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	orders := []models.MarketplaceEmailGroupOrder{}
	for rows.Next() {
		var order models.MarketplaceEmailGroupOrder
		if err := rows.Scan(&order.OrderID, &order.BillID, &order.Status, &order.ErrorCode); err != nil {
			return nil, err
		}
		orders = append(orders, order)
	}
	return orders, rows.Err()
}

func marketplaceEmailGroupRefKey(source, messageID string) string {
	return strings.TrimSpace(source) + "\x1f" + strings.TrimSpace(messageID)
}
