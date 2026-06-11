package repository

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

type LazadaChargeGroup struct {
	EmailDate                string
	GroupCount               int
	GroupTotal               float64
	OrderIDs                 []string
	MissingPaidTotalOrderIDs []string
	NonCardPaymentOrderIDs   []string
	NotReconciledOrderIDs    []string
	Orders                   []LazadaChargeGroupOrder
}

type LazadaChargeGroupOrder struct {
	BillID               string
	OrderID              string
	PaymentMethod        string
	ReconciliationStatus string
	PaidTotal            float64
	HasPaidTotal         bool
}

func (r *BillRepo) GetLazadaChargeGroupByEmailDate(emailDate string) (*LazadaChargeGroup, error) {
	emailDate = strings.TrimSpace(emailDate)
	if emailDate == "" {
		return nil, fmt.Errorf("lazada charge group email_date is empty")
	}

	rows, err := r.db.Query(
		`SELECT b.id::text,
		        COALESCE(NULLIF(b.raw_data->>'order_id', ''), NULLIF(b.raw_data->>'lazada_order_id', ''), b.id::text) AS order_id,
		        COALESCE(b.raw_data->>'payment_method', '') AS payment_method,
		        COALESCE(b.raw_data->>'paid_total_amount', '') AS paid_total_amount,
		        COALESCE(b.raw_data->>'amount_reconciliation_status', '') AS amount_reconciliation_status
		   FROM bills b
		  WHERE b.source = 'lazada_email'
		    AND b.bill_type = 'purchase'
		    AND b.archived_at IS NULL
		    AND b.raw_data->>'email_date' = $1
		  ORDER BY b.created_at ASC, b.id ASC`,
		emailDate,
	)
	if err != nil {
		return nil, fmt.Errorf("get lazada charge group: %w", err)
	}
	defer rows.Close()

	group := &LazadaChargeGroup{EmailDate: emailDate}
	for rows.Next() {
		var order LazadaChargeGroupOrder
		var paidText string
		if err := rows.Scan(&order.BillID, &order.OrderID, &order.PaymentMethod, &paidText, &order.ReconciliationStatus); err != nil {
			return nil, err
		}
		order.OrderID = strings.TrimSpace(order.OrderID)
		if order.OrderID == "" {
			order.OrderID = order.BillID
		}
		order.PaymentMethod = strings.TrimSpace(order.PaymentMethod)
		order.ReconciliationStatus = strings.TrimSpace(order.ReconciliationStatus)
		if paidTotal, ok := parseBillMoneyText(paidText); ok {
			order.PaidTotal = paidTotal
			order.HasPaidTotal = true
			group.GroupTotal = roundBillMoney(group.GroupTotal + paidTotal)
		} else {
			group.MissingPaidTotalOrderIDs = append(group.MissingPaidTotalOrderIDs, order.OrderID)
		}
		if !IsCreditDebitCardPaymentMethod(order.PaymentMethod) {
			group.NonCardPaymentOrderIDs = append(group.NonCardPaymentOrderIDs, order.OrderID)
		}
		if order.ReconciliationStatus != LazadaReconciliationOK {
			group.NotReconciledOrderIDs = append(group.NotReconciledOrderIDs, order.OrderID)
		}
		group.OrderIDs = append(group.OrderIDs, order.OrderID)
		group.Orders = append(group.Orders, order)
		group.GroupCount++
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return group, nil
}

func IsCreditDebitCardPaymentMethod(method string) bool {
	lower := strings.ToLower(strings.TrimSpace(method))
	return strings.Contains(lower, "credit") ||
		strings.Contains(lower, "debit") ||
		strings.Contains(method, "บัตรเครดิต") ||
		strings.Contains(method, "บัตรเดบิต")
}

func parseBillMoneyText(value string) (float64, bool) {
	clean := strings.TrimSpace(value)
	clean = strings.TrimPrefix(clean, "฿")
	clean = strings.ReplaceAll(clean, ",", "")
	if clean == "" {
		return 0, false
	}
	n, err := strconv.ParseFloat(clean, 64)
	if err != nil {
		return 0, false
	}
	return roundBillMoney(n), true
}

func roundBillMoney(v float64) float64 {
	return math.Round(v*100) / 100
}
