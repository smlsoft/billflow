package handlers

import (
	"encoding/json"
	"strings"
	"testing"

	"billflow/internal/models"
)

func TestDocDateFromBillForSMLMarketplacePurchase(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr string
	}{
		{
			name: "uses doc_date first",
			raw:  `{"order_id":"1101","doc_date":"2026-06-10","order_datetime":"09 มิ.ย. 2026 16:47:47"}`,
			want: "2026-06-10",
		},
		{
			name: "falls back to thai order datetime",
			raw:  `{"order_id":"260610PT6XWJ9U","order_datetime":"10 มิ.ย. 2026 16:47:47"}`,
			want: "2026-06-10",
		},
		{
			name: "normalizes buddhist year",
			raw:  `{"order_id":"260610PT6XWJ9U","order_datetime":"10/06/2569"}`,
			want: "2026-06-10",
		},
		{
			name:    "blocks missing marketplace order date",
			raw:     `{"order_id":"260610PT6XWJ9U"}`,
			wantErr: "raw_data.doc_date หรือ raw_data.order_datetime",
		},
		{
			name:    "blocks invalid marketplace order date",
			raw:     `{"order_id":"260610PT6XWJ9U","doc_date":"not-a-date","order_datetime":"also-not-a-date"}`,
			wantErr: "260610PT6XWJ9U",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := docDateFromBillForSML(marketplacePurchaseBillForDocDateTest(tt.raw))
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("docDateFromBillForSML error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("docDateFromBillForSML = %q, want %q", got, tt.want)
			}
		})
	}
}

func marketplacePurchaseBillForDocDateTest(raw string) *models.Bill {
	var compact map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &compact); err != nil {
		panic(err)
	}
	rawBytes, _ := json.Marshal(compact)
	return &models.Bill{
		ID:       "bill-doc-date-test",
		Source:   "shopee_shipped",
		BillType: "purchase",
		RawData:  rawBytes,
	}
}
