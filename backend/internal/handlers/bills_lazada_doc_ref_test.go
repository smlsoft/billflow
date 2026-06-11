package handlers

import (
	"encoding/json"
	"strings"
	"testing"

	"billflow/internal/models"
	"billflow/internal/repository"
	"billflow/internal/services/sml"
	"github.com/DATA-DOG/go-sqlmock"
)

func TestResolveLazadaCardChargeGroupForSendUsesEmailDateGroup(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	emailDate := "2026-06-10T17:56:30+08:00"
	mock.ExpectQuery("FROM bills b").
		WithArgs(emailDate).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "order_id", "payment_method", "paid_total_amount", "amount_reconciliation_status",
		}).
			AddRow("bill-1", "1100887409895692", "Credit or Debit Card", "162.36", "ok").
			AddRow("bill-2", "1100887410295692", "Credit or Debit Card", "187", "ok").
			AddRow("bill-3", "1100887410495692", "Credit or Debit Card", "1021.45", "ok").
			AddRow("bill-4", "1100887410695692", "Credit or Debit Card", "845.33", "ok"))

	h := &BillHandler{billRepo: repository.NewBillRepo(db)}
	bill := lazadaBillForDocRefTest(`{
		"order_id":"1100887410495692",
		"email_date":"2026-06-10T17:56:30+08:00",
		"payment_method":"Credit or Debit Card",
		"paid_total_amount":1021.45,
		"amount_reconciliation_status":"ok"
	}`)
	cache := map[string]*repository.LazadaChargeGroup{}
	group, err := h.resolveLazadaCardChargeGroupForSend(bill, retrySendOptions{LazadaChargeGroups: cache})
	if err != nil {
		t.Fatalf("resolveLazadaCardChargeGroupForSend: %v", err)
	}
	if group.GroupCount != 4 || group.GroupTotal != 2216.14 {
		t.Fatalf("group = count %d total %v, want 4/2216.14", group.GroupCount, group.GroupTotal)
	}
	if cached := cache[emailDate]; cached == nil || cached.GroupTotal != 2216.14 {
		t.Fatalf("cache = %#v", cache)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func TestValidateLazadaChargeGroupForDocRefBlocksInvalidOrders(t *testing.T) {
	group := &repository.LazadaChargeGroup{
		EmailDate:                "2026-06-09T17:46:43+08:00",
		GroupCount:               3,
		GroupTotal:               471.27,
		MissingPaidTotalOrderIDs: []string{"1108968794895692"},
		NonCardPaymentOrderIDs:   []string{"1108968795095692"},
		NotReconciledOrderIDs:    []string{"1108968795495692"},
	}
	err := validateLazadaChargeGroupForDocRef(group)
	if err == nil {
		t.Fatal("expected invalid group to be blocked")
	}
	msg := err.Error()
	for _, want := range []string{"ไม่มียอดชำระ", "ไม่ใช่บัตรเครดิต/เดบิต", "reconcile ไม่ผ่าน"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error %q missing %q", msg, want)
		}
	}
}

func TestValidateMarketplaceAmountReadyForSendRequiresLazadaCardInputs(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "missing email date",
			raw:  `{"order_id":"1101","payment_method":"Credit or Debit Card","paid_total_amount":100,"amount_reconciliation_status":"ok"}`,
			want: "ไม่มีวันที่อีเมล",
		},
		{
			name: "non card",
			raw:  `{"order_id":"1102","email_date":"2026-06-10T17:56:30+08:00","payment_method":"Cash on Delivery","paid_total_amount":100,"amount_reconciliation_status":"ok"}`,
			want: "ไม่ใช่การชำระด้วยบัตร",
		},
		{
			name: "missing paid total",
			raw:  `{"order_id":"1103","email_date":"2026-06-10T17:56:30+08:00","payment_method":"Credit or Debit Card","amount_reconciliation_status":"ok"}`,
			want: "ไม่มียอดชำระ",
		},
		{
			name: "not reconciled",
			raw:  `{"order_id":"1104","email_date":"2026-06-10T17:56:30+08:00","payment_method":"Credit or Debit Card","paid_total_amount":100,"amount_reconciliation_status":"mismatch","amount_reconciliation_delta":1.23}`,
			want: "reconcile ไม่ผ่าน",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateMarketplaceAmountReadyForSend(lazadaBillForDocRefTest(tt.raw))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestValidateLazadaPurchasePayloadTotalForSend(t *testing.T) {
	bill := lazadaBillForDocRefTest(`{"order_id":"1100887410495692","paid_total_amount":1021.45}`)
	if err := validateLazadaPurchasePayloadTotalForSend(bill, sml.PurchaseOrderPayload{TotalAmount: 1021.45}); err != nil {
		t.Fatalf("matching payload rejected: %v", err)
	}
	err := validateLazadaPurchasePayloadTotalForSend(bill, sml.PurchaseOrderPayload{TotalAmount: 1000})
	if err == nil {
		t.Fatal("mismatched payload should be blocked")
	}
	for _, want := range []string{"1100887410495692", "1021.45", "1000.00", "SHIP_CUS"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q missing %q", err.Error(), want)
		}
	}
}

func lazadaBillForDocRefTest(raw string) *models.Bill {
	var compact map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &compact); err != nil {
		panic(err)
	}
	rawBytes, _ := json.Marshal(compact)
	return &models.Bill{
		ID:       "bill-lazada-test",
		Source:   "lazada_email",
		BillType: "purchase",
		RawData:  rawBytes,
	}
}
