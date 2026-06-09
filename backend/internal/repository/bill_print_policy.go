package repository

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"billflow/internal/models"
)

const maxEmailPrintCandidates = 100

type marketplacePrintRow struct {
	ID                          string
	OrderID                     string
	Source                      string
	BillType                    string
	Status                      string
	SMLDocNo                    string
	PartyCode                   string
	PartyName                   string
	PrintPaymentMethod          string
	EffectivePrintPaymentMethod string
}

func marketplaceEmailMessageExpr(alias string) string {
	return fmt.Sprintf("COALESCE(NULLIF(%s.raw_data->>'email_message_id', ''), NULLIF(%s.raw_data->>'message_id', ''), 'bill:' || %s.id::text)", alias, alias, alias)
}

func effectivePrintPaymentMethodExpr(alias string) string {
	return fmt.Sprintf(`COALESCE(
		NULLIF(BTRIM(%s.print_payment_method), ''),
		CASE
		  WHEN UPPER(BTRIM(COALESCE(%s.sml_payload->>'supplier_name', ''))) LIKE 'TT%%'
		    THEN BTRIM(COALESCE(%s.sml_payload->>'supplier_name', ''))
		  ELSE ''
		END
	)`, alias, alias, alias)
}

func marketplacePrintReadySQL(alias string) string {
	msg := marketplaceEmailMessageExpr(alias)
	relatedMsg := marketplaceEmailMessageExpr("rb")
	effectivePayment := effectivePrintPaymentMethodExpr("rb")
	return fmt.Sprintf(`
		%s.source IN ('shopee_shipped', 'lazada_email')
		AND %s.bill_type = 'purchase'
		AND %s.status = 'sent'
		AND COALESCE(%s.sml_doc_no, '') <> ''
		AND EXISTS (
			SELECT 1 FROM bill_artifacts ba
			 WHERE ba.bill_id = %s.id
			   AND ba.kind IN ('email_html', 'email_text')
		)
		AND NOT EXISTS (
			SELECT 1
			  FROM bills rb
			  LEFT JOIN channel_defaults cd
			    ON cd.channel = rb.source AND cd.bill_type = rb.bill_type
			  CROSS JOIN LATERAL (
			    SELECT
			      CASE
			        WHEN LOWER(COALESCE(cd.print_policy->>'requires_all_orders_sml_doc', '')) IN ('true', 'false')
			          THEN (cd.print_policy->>'requires_all_orders_sml_doc')::boolean
			        ELSE TRUE
			      END AS requires_all_orders_sml_doc,
			      CASE
			        WHEN LOWER(COALESCE(cd.print_policy->>'payment_method_prefix_enabled', '')) IN ('true', 'false')
			          THEN (cd.print_policy->>'payment_method_prefix_enabled')::boolean
			        ELSE TRUE
			      END AS payment_method_prefix_enabled,
			      CASE
			        WHEN jsonb_typeof(cd.print_policy->'payment_method_prefixes') = 'array'
			          AND jsonb_array_length(cd.print_policy->'payment_method_prefixes') > 0
			          THEN cd.print_policy->'payment_method_prefixes'
			        ELSE '["TT"]'::jsonb
			      END AS payment_method_prefixes,
			      CASE
			        WHEN jsonb_typeof(cd.print_policy->'payment_methods') = 'array'
			          AND jsonb_array_length(cd.print_policy->'payment_methods') > 0
			          THEN cd.print_policy->'payment_methods'
			        ELSE '["TT2789","TT9630","TT0972","TT9628","TT5128","TT5432","TT3086","TT8456","โอน Kbank","โอน TTB5074","โอน KTB","โอน BBL","โอน TTB1135"]'::jsonb
			      END AS payment_methods,
			      %s AS effective_payment_method
			  ) pol
			 WHERE %s = %s
			   AND rb.source IN ('shopee_shipped', 'lazada_email')
			   AND rb.bill_type = 'purchase'
			   AND (
			     (
			       pol.requires_all_orders_sml_doc
			       AND (rb.status <> 'sent' OR COALESCE(rb.sml_doc_no, '') = '')
			     )
			     OR (
			       pol.effective_payment_method = ''
			       OR NOT EXISTS (
			         SELECT 1
			           FROM jsonb_array_elements_text(pol.payment_methods) AS method(value)
			          WHERE method.value = pol.effective_payment_method
			       )
			     )
			     OR (
			       pol.payment_method_prefix_enabled
			       AND NOT EXISTS (
			         SELECT 1
			           FROM jsonb_array_elements_text(pol.payment_method_prefixes) AS prefix(value)
			          WHERE UPPER(pol.effective_payment_method) LIKE UPPER(prefix.value) || '%%'
			       )
			     )
			   )
		)`, alias, alias, alias, alias, alias, effectivePayment, relatedMsg, msg)
}

func (r *BillRepo) channelPrintPolicy(channel, billType string) models.MarketplacePrintPolicy {
	if !models.SupportsMarketplacePrintPolicy(channel, billType) {
		return models.DefaultMarketplacePrintPolicy(channel, billType)
	}
	var raw json.RawMessage
	err := r.db.QueryRow(
		`SELECT COALESCE(print_policy, '{}'::jsonb) FROM channel_defaults WHERE channel=$1 AND bill_type=$2`,
		channel, billType,
	).Scan(&raw)
	if err == sql.ErrNoRows {
		return models.DefaultMarketplacePrintPolicy(channel, billType)
	}
	if err != nil {
		return models.DefaultMarketplacePrintPolicy(channel, billType)
	}
	return models.NormalizeMarketplacePrintPolicyFromRaw(channel, billType, raw)
}

func (r *BillRepo) listMarketplacePrintRows(messageID string) ([]marketplacePrintRow, error) {
	rows, err := r.db.Query(`
		SELECT b.id::text,
		       COALESCE(NULLIF(b.raw_data->>'order_id', ''), NULLIF(b.raw_data->>'shopee_order_id', ''), b.id::text) AS order_id,
		       b.source,
		       b.bill_type,
		       b.status,
		       COALESCE(b.sml_doc_no, '') AS sml_doc_no,
		       COALESCE(NULLIF(b.sml_payload->>'cust_code', ''), '') AS party_code,
		       COALESCE(NULLIF(b.sml_payload->>'supplier_name', ''), '') AS party_name,
		       COALESCE(b.print_payment_method, '') AS print_payment_method,
		       `+effectivePrintPaymentMethodExpr("b")+` AS effective_print_payment_method
		  FROM bills b
		 WHERE COALESCE(NULLIF(b.raw_data->>'email_message_id', ''), NULLIF(b.raw_data->>'message_id', ''), 'bill:' || b.id::text) = $1
		   AND b.source IN ('shopee_shipped', 'lazada_email')
		   AND b.bill_type = 'purchase'
		 ORDER BY b.created_at ASC, b.id ASC`,
		messageID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []marketplacePrintRow{}
	for rows.Next() {
		var row marketplacePrintRow
		if err := rows.Scan(&row.ID, &row.OrderID, &row.Source, &row.BillType, &row.Status, &row.SMLDocNo, &row.PartyCode, &row.PartyName, &row.PrintPaymentMethod, &row.EffectivePrintPaymentMethod); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func rowMatchesPaymentPolicy(row marketplacePrintRow, policy models.MarketplacePrintPolicy) bool {
	paymentMethod := strings.TrimSpace(row.EffectivePrintPaymentMethod)
	if paymentMethod == "" || !paymentMethodAllowedInList(paymentMethod, policy.PaymentMethods) {
		return false
	}
	if !policy.PaymentMethodPrefixEnabled {
		return true
	}
	prefixes := policy.PaymentMethodPrefixes
	if len(prefixes) == 0 {
		prefixes = []string{"TT"}
	}
	upperPayment := strings.ToUpper(paymentMethod)
	for _, prefix := range prefixes {
		p := strings.ToUpper(strings.TrimSpace(prefix))
		if p != "" && strings.HasPrefix(upperPayment, p) {
			return true
		}
	}
	return false
}

func paymentMethodAllowedInList(paymentMethod string, allowed []string) bool {
	paymentMethod = strings.TrimSpace(paymentMethod)
	if paymentMethod == "" {
		return false
	}
	if len(allowed) == 0 {
		allowed = models.DefaultMarketplacePrintPaymentMethods()
	}
	for _, method := range allowed {
		if strings.TrimSpace(method) == paymentMethod {
			return true
		}
	}
	return false
}

func rowOrderLabel(row marketplacePrintRow) string {
	if strings.TrimSpace(row.OrderID) != "" {
		return strings.TrimSpace(row.OrderID)
	}
	return row.ID
}

func (r *BillRepo) ListEmailPrintCandidates(f models.BillListFilter, limit int) ([]models.EmailPrintCandidate, bool, error) {
	if limit <= 0 || limit > maxEmailPrintCandidates {
		limit = maxEmailPrintCandidates
	}
	f.Status = ""
	f.PrintReady = true
	f.Page = 1
	f.PageSize = limit
	where, args, argN := billWhere(f)
	query := `
		SELECT ` + marketplaceEmailMessageExpr("b") + ` AS message_id,
		       MAX(b.created_at) AS latest_at
		  FROM bills b
		  ` + where + `
		 GROUP BY message_id
		 ORDER BY latest_at DESC
		 LIMIT $` + fmt.Sprint(argN)
	args = append(args, limit+1)

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()

	messageIDs := []string{}
	for rows.Next() {
		var messageID string
		var ignored sql.NullTime
		if err := rows.Scan(&messageID, &ignored); err != nil {
			return nil, false, err
		}
		messageID = strings.TrimSpace(messageID)
		if messageID != "" {
			messageIDs = append(messageIDs, messageID)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	truncated := len(messageIDs) > limit
	if truncated {
		messageIDs = messageIDs[:limit]
	}

	out := []models.EmailPrintCandidate{}
	for _, messageID := range messageIDs {
		candidate, err := r.emailPrintCandidateByMessageID(messageID)
		if err != nil {
			return nil, false, err
		}
		if candidate != nil {
			out = append(out, *candidate)
		}
	}
	return out, truncated, nil
}

func (r *BillRepo) emailPrintCandidateByMessageID(messageID string) (*models.EmailPrintCandidate, error) {
	var candidate models.EmailPrintCandidate
	var source, billType string
	err := r.db.QueryRow(`
		SELECT b.id::text,
		       ba.id::text,
		       ba.filename,
		       COALESCE(NULLIF(ba.source_meta->>'subject', ''), b.raw_data->>'subject', '') AS subject,
		       COALESCE(NULLIF(ba.source_meta->>'from', ''), NULLIF(b.raw_data->>'from', ''), NULLIF(b.raw_data->>'from_addr', ''), '') AS from_addr,
		       b.source,
		       b.bill_type
		  FROM bills b
		  JOIN bill_artifacts ba ON ba.bill_id = b.id
		 WHERE `+marketplaceEmailMessageExpr("b")+` = $1
		   AND b.source IN ('shopee_shipped', 'lazada_email')
		   AND b.bill_type = 'purchase'
		   AND ba.kind IN ('email_html', 'email_text')
		 ORDER BY b.created_at ASC, ba.created_at ASC
		 LIMIT 1`,
		messageID,
	).Scan(&candidate.ArtifactBillID, &candidate.ArtifactID, &candidate.ArtifactFilename, &candidate.Subject, &candidate.From, &source, &billType)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	candidate.MessageID = messageID
	candidate.GroupKey = emailGroupKey(messageID)
	candidate.PrintPolicy = r.channelPrintPolicy(source, billType)
	candidate.PrintPolicyNote = models.MarketplacePrintPolicyNote(candidate.PrintPolicy)

	rows, err := r.listMarketplacePrintRows(messageID)
	if err != nil {
		return nil, err
	}
	candidate.Orders = make([]models.EmailPrintCandidateOrder, 0, len(rows))
	for _, row := range rows {
		candidate.Orders = append(candidate.Orders, models.EmailPrintCandidateOrder{
			BillID:                      row.ID,
			OrderID:                     row.OrderID,
			SMLDocNo:                    row.SMLDocNo,
			PartyCode:                   row.PartyCode,
			PartyName:                   row.PartyName,
			PrintPaymentMethod:          row.PrintPaymentMethod,
			EffectivePrintPaymentMethod: row.EffectivePrintPaymentMethod,
			Source:                      row.Source,
			Status:                      row.Status,
		})
	}
	return &candidate, nil
}

type EmailPrintEventRequest struct {
	BillID     string `json:"bill_id"`
	ArtifactID string `json:"artifact_id"`
}

func (r *BillRepo) RecordEmailPrintEventsBulk(requests []EmailPrintEventRequest, userID, userEmail string) ([]models.EmailPrintEvent, error) {
	if len(requests) == 0 {
		return []models.EmailPrintEvent{}, nil
	}
	if len(requests) > maxEmailPrintCandidates {
		return nil, fmt.Errorf("พิมพ์ได้สูงสุด %d ชุดอีเมลต่อครั้ง", maxEmailPrintCandidates)
	}
	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	events := []models.EmailPrintEvent{}
	for _, req := range requests {
		event, err := r.recordEmailPrintEventWithExecutor(tx, req.BillID, req.ArtifactID, userID, userEmail)
		if err != nil {
			return nil, err
		}
		if event == nil {
			return nil, fmt.Errorf("artifact not found for bill %s", req.BillID)
		}
		events = append(events, *event)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return events, nil
}
