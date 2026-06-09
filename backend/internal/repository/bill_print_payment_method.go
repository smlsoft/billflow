package repository

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/lib/pq"
)

type PrintPaymentMethodUpdateTarget struct {
	BillID       string `json:"bill_id"`
	OrderID      string `json:"order_id"`
	OldMethod    string `json:"old_method"`
	OldEffective string `json:"old_effective_method"`
	Status       string `json:"status"`
	SMLDocNo     string `json:"sml_doc_no"`
	Source       string `json:"source"`
	BillType     string `json:"bill_type"`
}

type PrintPaymentMethodUpdateResult struct {
	PaymentMethod     string                           `json:"payment_method"`
	ApplyToEmailGroup bool                             `json:"apply_to_email_group"`
	UpdatedCount      int                              `json:"updated_count"`
	MessageID         string                           `json:"message_id,omitempty"`
	Targets           []PrintPaymentMethodUpdateTarget `json:"targets"`
}

func (r *BillRepo) UpdatePrintPaymentMethod(id, paymentMethod string, applyToEmailGroup bool) (*PrintPaymentMethodUpdateResult, error) {
	id = strings.TrimSpace(id)
	paymentMethod = strings.TrimSpace(paymentMethod)
	if id == "" {
		return nil, fmt.Errorf("bill id is required")
	}
	if paymentMethod == "" {
		return nil, fmt.Errorf("กรุณาเลือกวิธีการชำระเงิน")
	}

	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var source, billType, status, smlDocNo, messageID string
	var archived bool
	err = tx.QueryRow(`
		SELECT b.source,
		       b.bill_type,
		       b.status,
		       COALESCE(b.sml_doc_no, '') AS sml_doc_no,
		       b.archived_at IS NOT NULL AS archived,
		       `+marketplaceEmailMessageExpr("b")+` AS message_id
		  FROM bills b
		 WHERE b.id = $1
		 FOR UPDATE`,
		id,
	).Scan(&source, &billType, &status, &smlDocNo, &archived, &messageID)
	if err != nil {
		return nil, err
	}
	if !isMarketplacePurchaseEmail(source, billType) {
		return nil, fmt.Errorf("แก้วิธีชำระเงินสำหรับปริ้นได้เฉพาะบิลซื้อ Shopee/Lazada email")
	}
	if archived {
		return nil, fmt.Errorf("บิลนี้ถูกเก็บแล้ว ไม่สามารถแก้วิธีชำระเงินสำหรับปริ้นได้")
	}
	if strings.TrimSpace(status) != "sent" || strings.TrimSpace(smlDocNo) == "" {
		return nil, fmt.Errorf("ต้องส่งเข้า SML และมีเลข POL ก่อนจึงจะแก้วิธีชำระเงินสำหรับปริ้นได้")
	}

	policy := r.channelPrintPolicy(source, billType)
	if !paymentMethodAllowedInList(paymentMethod, policy.PaymentMethods) {
		return nil, fmt.Errorf("วิธีการชำระเงินนี้ไม่อยู่ในรายการที่ตั้งค่าไว้ใน Channels")
	}

	where := "b.id = $1"
	args := []any{id}
	if applyToEmailGroup {
		where = marketplaceEmailMessageExpr("b") + " = $1 AND b.source IN ('shopee_shipped', 'lazada_email') AND b.bill_type = 'purchase' AND b.archived_at IS NULL"
		args = []any{messageID}
	}

	rows, err := tx.Query(`
		SELECT b.id::text,
		       COALESCE(NULLIF(b.raw_data->>'order_id', ''), NULLIF(b.raw_data->>'shopee_order_id', ''), b.id::text) AS order_id,
		       b.source,
		       b.bill_type,
		       b.status,
		       COALESCE(b.sml_doc_no, '') AS sml_doc_no,
		       COALESCE(b.print_payment_method, '') AS print_payment_method,
		       `+effectivePrintPaymentMethodExpr("b")+` AS effective_print_payment_method
		  FROM bills b
		 WHERE `+where+`
		 ORDER BY b.created_at ASC, b.id ASC
		 FOR UPDATE`,
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	targets := []PrintPaymentMethodUpdateTarget{}
	ids := []string{}
	for rows.Next() {
		var target PrintPaymentMethodUpdateTarget
		if err := rows.Scan(
			&target.BillID, &target.OrderID, &target.Source, &target.BillType,
			&target.Status, &target.SMLDocNo, &target.OldMethod, &target.OldEffective,
		); err != nil {
			return nil, err
		}
		targets = append(targets, target)
		ids = append(ids, target.BillID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, sql.ErrNoRows
	}

	if _, err := tx.Exec(
		`UPDATE bills SET print_payment_method = $1 WHERE id = ANY($2::uuid[])`,
		paymentMethod,
		pq.Array(ids),
	); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &PrintPaymentMethodUpdateResult{
		PaymentMethod:     paymentMethod,
		ApplyToEmailGroup: applyToEmailGroup,
		UpdatedCount:      len(ids),
		MessageID:         messageID,
		Targets:           targets,
	}, nil
}
