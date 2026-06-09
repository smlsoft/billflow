package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"billflow/internal/models"
	"billflow/internal/services/sml"
)

func TestResolveEndpointUsesExplicitEndpointKeyword(t *testing.T) {
	tests := []struct {
		name         string
		endpoint     string
		wantKind     string
		wantOverride string
	}{
		{
			name:         "saleorder keyword path",
			endpoint:     "/SMLJavaRESTService/v3/api/saleorder",
			wantKind:     "saleorder",
			wantOverride: "/SMLJavaRESTService/v3/api/saleorder",
		},
		{
			name:         "saleinvoice keyword path",
			endpoint:     "/SMLJavaRESTService/saleinvoice/v4",
			wantKind:     "saleinvoice",
			wantOverride: "/SMLJavaRESTService/saleinvoice/v4",
		},
		{
			name:         "purchaseorder keyword url",
			endpoint:     "http://sml.local/SMLJavaRESTService/v3/api/purchaseorder",
			wantKind:     "purchaseorder",
			wantOverride: "http://sml.local/SMLJavaRESTService/v3/api/purchaseorder",
		},
		{
			name:         "legacy sale reserve path now falls back to saleorder",
			endpoint:     "/api/sale_reserve",
			wantKind:     "saleorder",
			wantOverride: "",
		},
		{
			name:         "bare saleinvoice token",
			endpoint:     " saleinvoice ",
			wantKind:     "saleinvoice",
			wantOverride: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			def := &models.ChannelDefault{Endpoint: tt.endpoint}
			gotKind, gotOverride := resolveEndpoint(def, "line", "sale")
			if gotKind != tt.wantKind || gotOverride != tt.wantOverride {
				t.Fatalf("resolveEndpoint() = (%q, %q), want (%q, %q)", gotKind, gotOverride, tt.wantKind, tt.wantOverride)
			}
		})
	}
}

type testSMLMessageResponse struct {
	message string
}

func (r testSMLMessageResponse) GetMessage() string {
	return r.message
}

func TestSMLSendErrorMessageExplainsEmpty404(t *testing.T) {
	got := smlSendErrorMessage(http.StatusNotFound, testSMLMessageResponse{}, nil)
	want := "HTTP 404 — ไม่พบ endpoint SML ที่ตั้งไว้ กรุณาตรวจ SML REST URL ใน /settings/instance และปลายทางใน /settings/channels"
	if got != want {
		t.Fatalf("message = %q, want %q", got, want)
	}
}

func TestValidateResolvedSendFieldsRequiresVisibleConfig(t *testing.T) {
	h := &BillHandler{}
	if err := h.validateResolvedSendFields("", "WH", "SH", "09:00", 0, 7); err == nil {
		t.Fatal("missing doc_format should be rejected")
	}
	if err := h.validateResolvedSendFields("PO", "WH", "SH", "09:00", 0, 7); err != nil {
		t.Fatalf("complete visible config rejected: %v", err)
	}
}

func TestExtractSMLERPLogWarningFromNativeResponse(t *testing.T) {
	raw := []byte(`{"success":true,"data":{"doc_no":"PO26050001","log_status":"warning","log_warning":"ไม่พบฐานข้อมูล data1_test_logs"}}`)
	got := extractSMLERPLogWarning(raw)
	if got != "ไม่พบฐานข้อมูล data1_test_logs" {
		t.Fatalf("warning = %q", got)
	}
}

func TestResolveEndpointFallsBackBySourceAndBillType(t *testing.T) {
	tests := []struct {
		name     string
		source   string
		billType string
		wantKind string
	}{
		{name: "shopee excel sale defaults to saleorder", source: "shopee", billType: "sale", wantKind: "saleorder"},
		{name: "shopee email sale defaults to saleorder", source: "shopee_email", billType: "sale", wantKind: "saleorder"},
		{name: "lazada excel sale defaults to saleorder", source: "lazada", billType: "sale", wantKind: "saleorder"},
		{name: "lazada email purchase defaults to purchaseorder", source: "lazada_email", billType: "purchase", wantKind: "purchaseorder"},
		{name: "tiktok excel sale defaults to saleorder", source: "tiktok", billType: "sale", wantKind: "saleorder"},
		{name: "shopee shipped defaults to purchaseorder", source: "shopee_shipped", billType: "purchase", wantKind: "purchaseorder"},
		{name: "purchase bill defaults to purchaseorder", source: "email", billType: "purchase", wantKind: "purchaseorder"},
		{name: "line sale defaults to saleorder", source: "line", billType: "sale", wantKind: "saleorder"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotKind, gotOverride := resolveEndpoint(nil, tt.source, tt.billType)
			if gotKind != tt.wantKind || gotOverride != "" {
				t.Fatalf("resolveEndpoint() = (%q, %q), want (%q, \"\")", gotKind, gotOverride, tt.wantKind)
			}
		})
	}
}

func TestMapSourceToChannelMatchesRetryLookupKey(t *testing.T) {
	tests := []struct {
		source string
		want   string
	}{
		{source: "shopee", want: "shopee"},
		{source: "shopee_email", want: "shopee_email"},
		{source: "shopee_shipped", want: "shopee_shipped"},
		{source: "lazada", want: "lazada"},
		{source: "lazada_email", want: "lazada_email"},
		{source: "tiktok", want: "tiktok"},
		{source: "email", want: "email"},
		{source: "line", want: "line"},
		{source: "manual", want: "line"},
	}

	for _, tt := range tests {
		t.Run(tt.source, func(t *testing.T) {
			if got := mapSourceToChannel(tt.source); got != tt.want {
				t.Fatalf("mapSourceToChannel(%q) = %q, want %q", tt.source, got, tt.want)
			}
		})
	}
}

func TestValidateBulkBillIDsGuardsProductionBatch(t *testing.T) {
	validA := "11111111-1111-1111-1111-111111111111"
	validB := "22222222-2222-2222-2222-222222222222"
	if err := validateBulkBillIDs([]string{validA, validB}); err != nil {
		t.Fatalf("valid ids rejected: %v", err)
	}
	if err := validateBulkBillIDs(nil); err == nil {
		t.Fatal("empty batch should be rejected")
	}
	if err := validateBulkBillIDs([]string{"not-a-uuid"}); err == nil {
		t.Fatal("invalid UUID should be rejected")
	}
	if err := validateBulkBillIDs([]string{validA, validA}); err == nil {
		t.Fatal("duplicate bill id should be rejected")
	}
	tooMany := make([]string, 101)
	for i := range tooMany {
		tooMany[i] = "11111111-1111-1111-1111-111111111111"
	}
	if err := validateBulkBillIDs(tooMany); err == nil {
		t.Fatal("batch over 100 should be rejected")
	}
}

func TestAppendRetryOfJobKeepsFilterSnapshot(t *testing.T) {
	raw := json.RawMessage(`{"source":"shopee_shipped","page":3}`)
	out := appendRetryOfJob(raw, "job-123")
	var got map[string]interface{}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("unmarshal retry filter: %v", err)
	}
	if got["source"] != "shopee_shipped" || got["retry_of_job_id"] != "job-123" {
		t.Fatalf("filter snapshot = %#v", got)
	}
}

func TestBulkJobMatchesSnapshotFilterScopesShopeeShop(t *testing.T) {
	snapshot := json.RawMessage(`{"source":"shopee","shopee_shop_id":"1029622928"}`)
	if !bulkJobMatchesSnapshotFilter(snapshot, "shopee_shop_id", "1029622928") {
		t.Fatal("expected matching shopee_shop_id to resume active job")
	}
	if bulkJobMatchesSnapshotFilter(snapshot, "shopee_shop_id", "999") {
		t.Fatal("different shopee_shop_id should not resume another shop's active job")
	}
	if !bulkJobMatchesSnapshotFilter(snapshot, "shopee_shop_id", "") {
		t.Fatal("empty filter should keep legacy active-job behavior")
	}
	if bulkJobMatchesSnapshotFilter(json.RawMessage(`{"source":"shopee"}`), "shopee_shop_id", "1029622928") {
		t.Fatal("missing shopee_shop_id should not match a shop-specific filter")
	}
}

func TestValidBulkJobStatus(t *testing.T) {
	for _, status := range []string{"queued", "running", "completed", "completed_with_errors", "failed"} {
		if !validBulkJobStatus(status) {
			t.Fatalf("expected %q to be valid", status)
		}
	}
	for _, status := range []string{"", "sent", "pending", "bad"} {
		if validBulkJobStatus(status) {
			t.Fatalf("expected %q to be invalid", status)
		}
	}
}

func TestValidateRemark2AllowsOnlyDocumentStatusCodes(t *testing.T) {
	for _, value := range []string{"", "tax", "notax", "re"} {
		if err := validateRemark2(value); err != nil {
			t.Fatalf("validateRemark2(%q) rejected: %v", value, err)
		}
	}
	for _, value := range []string{"vat", "taxinvoice", " tax "} {
		if err := validateRemark2(value); err == nil {
			t.Fatalf("validateRemark2(%q) should reject invalid code", value)
		}
	}
}

func TestValidateBulkSendPayloadChecksRemark2ForSaleAndPurchase(t *testing.T) {
	if err := validateBulkSendPayload("sale", "saleorder", RetryRequest{Remark2: "vat"}); err == nil {
		t.Fatal("sale bulk payload with invalid remark_2 should be rejected")
	}
	if err := validateBulkSendPayload("purchase", "purchaseorder", RetryRequest{Remark2: "tax"}); err == nil {
		t.Fatal("purchase bulk still requires inquiry_type")
	}
	inquiryType := 1
	if err := validateBulkSendPayload("purchase", "purchaseorder", RetryRequest{
		Remark2:     "tax",
		InquiryType: &inquiryType,
	}); err != nil {
		t.Fatalf("valid purchase bulk payload rejected: %v", err)
	}
}

func TestPurchaseOrderHeaderFromBillUsesMarketplaceSellerRemark(t *testing.T) {
	tests := []struct {
		name           string
		bill           *models.Bill
		requestRemark  string
		wantRemark     string
		wantRemark5    string
		wantDocRef     string
		wantDocRefDate string
	}{
		{
			name: "lazada non-card moves order id to remark5 and clears doc ref",
			bill: &models.Bill{
				Source:   "lazada_email",
				BillType: "purchase",
				RawData:  json.RawMessage(`{"seller_name":"Lazada Shop","order_id":"1107473377495692","payment_method":"Cash on Delivery","paid_total_amount":1015.75}`),
			},
			requestRemark: "user typed wrong remark",
			wantRemark:    "Lazada Shop",
			wantRemark5:   "1107473377495692",
		},
		{
			name: "lazada card puts paid total in doc ref and order id in remark5",
			bill: &models.Bill{
				Source:   "lazada_email",
				BillType: "purchase",
				RawData: json.RawMessage(`{
					"seller_name":"Lucky Store*",
					"order_id":"1108153962788966",
					"payment_method":"Credit or Debit Card",
					"paid_total_amount":1015.75
				}`),
			},
			requestRemark: "user typed wrong remark",
			wantRemark:    "Lucky Store*",
			wantRemark5:   "1108153962788966",
			wantDocRef:    "1015.75",
		},
		{
			name: "lazada card with string paid total formats doc ref",
			bill: &models.Bill{
				Source:   "lazada_email",
				BillType: "purchase",
				RawData: json.RawMessage(`{
					"seller_name":"Lazada Shop",
					"order_id":"1107000000000000",
					"payment_method":"Credit or Debit Card",
					"paid_total_amount":"1,015.00"
				}`),
			},
			requestRemark: "user typed wrong remark",
			wantRemark:    "Lazada Shop",
			wantRemark5:   "1107000000000000",
			wantDocRef:    "1015",
		},
		{
			name: "lazada card without paid total keeps doc ref empty",
			bill: &models.Bill{
				Source:   "lazada_email",
				BillType: "purchase",
				RawData:  json.RawMessage(`{"seller_name":"Lazada Shop","order_id":"1107000000000001","payment_method":"Credit or Debit Card"}`),
			},
			requestRemark: "user typed wrong remark",
			wantRemark:    "Lazada Shop",
			wantRemark5:   "1107000000000001",
		},
		{
			name: "shopee preserves remark5 and card doc ref behavior",
			bill: &models.Bill{
				Source:   "shopee_shipped",
				BillType: "purchase",
				RawData: json.RawMessage(`{
					"seller_name":"Shopee Seller",
					"order_id":"2605211KR3XK1G",
					"payment_summary":{"is_credit_debit_card":true,"doc_ref_amount":"7275"}
				}`),
			},
			requestRemark: "user typed wrong remark",
			wantRemark:    "Shopee Seller",
			wantRemark5:   "2605211KR3XK1G",
			wantDocRef:    "7275",
		},
		{
			name: "normal purchase still accepts manual remark",
			bill: &models.Bill{
				Source:   "email",
				BillType: "purchase",
				RawData:  json.RawMessage(`{"order_id":"PO-REF-1"}`),
			},
			requestRemark:  "manual remark",
			wantRemark:     "manual remark",
			wantDocRef:     "PO-REF-1",
			wantDocRefDate: "2026-06-05",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			docRef, docRefDate, remark, remark5 := purchaseOrderHeaderFromBill(tt.bill, RetryRequest{Remark: tt.requestRemark}, "2026-06-05")
			if remark != tt.wantRemark || remark5 != tt.wantRemark5 || docRef != tt.wantDocRef || docRefDate != tt.wantDocRefDate {
				t.Fatalf(
					"header = docRef:%q docRefDate:%q remark:%q remark5:%q, want docRef:%q docRefDate:%q remark:%q remark5:%q",
					docRef, docRefDate, remark, remark5,
					tt.wantDocRef, tt.wantDocRefDate, tt.wantRemark, tt.wantRemark5,
				)
			}
		})
	}
}

func TestValidatePurchaseCreditorUpdateBillGuardsScope(t *testing.T) {
	docNo := "PO26060011"
	archivedAt := time.Now()
	tests := []struct {
		name       string
		bill       *models.Bill
		wantStatus int
	}{
		{
			name: "sent lazada marketplace purchase can update",
			bill: &models.Bill{
				Source:   "lazada_email",
				BillType: "purchase",
				Status:   "sent",
				SMLDocNo: &docNo,
			},
			wantStatus: 0,
		},
		{
			name: "sent shopee marketplace purchase can update",
			bill: &models.Bill{
				Source:   "shopee_shipped",
				BillType: "purchase",
				Status:   "sent",
				SMLDocNo: &docNo,
			},
			wantStatus: 0,
		},
		{
			name: "normal purchase rejected",
			bill: &models.Bill{
				Source:   "email",
				BillType: "purchase",
				Status:   "sent",
				SMLDocNo: &docNo,
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "pending marketplace rejected",
			bill: &models.Bill{
				Source:   "lazada_email",
				BillType: "purchase",
				Status:   "pending",
				SMLDocNo: &docNo,
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "archived marketplace rejected",
			bill: &models.Bill{
				Source:     "shopee_shipped",
				BillType:   "purchase",
				Status:     "sent",
				SMLDocNo:   &docNo,
				ArchivedAt: &archivedAt,
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "missing sml doc number rejected",
			bill: &models.Bill{
				Source:   "lazada_email",
				BillType: "purchase",
				Status:   "sent",
			},
			wantStatus: http.StatusBadRequest,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotStatus, _ := validatePurchaseCreditorUpdateBill(tt.bill)
			if gotStatus != tt.wantStatus {
				t.Fatalf("status = %d, want %d", gotStatus, tt.wantStatus)
			}
		})
	}
}

func TestSMLPayloadStringTrimsKnownKeys(t *testing.T) {
	raw := json.RawMessage(`{"cust_code":" AF00007 ","supplier_name":" TT3086 "}`)
	if got := smlPayloadString(raw, "cust_code"); got != "AF00007" {
		t.Fatalf("cust_code = %q", got)
	}
	if got := smlPayloadString(raw, "supplier_name"); got != "TT3086" {
		t.Fatalf("supplier_name = %q", got)
	}
	if got := smlPayloadString(json.RawMessage(`not-json`), "cust_code"); got != "" {
		t.Fatalf("invalid payload = %q, want empty", got)
	}
}

func TestPurchaseCreditorUpdateErrorMessageExplainsSMLAPIVersionAndConflict(t *testing.T) {
	got404 := purchaseCreditorUpdateErrorMessage(http.StatusNotFound, &sml.PurchaseOrderCreditorUpdateResponse{}, nil)
	if !strings.Contains(got404, "sml-api-bybos") || !strings.Contains(got404, "PATCH /api/v1/ic/purchase-orders/:doc_no/creditor") {
		t.Fatalf("404 message = %q", got404)
	}

	got409 := purchaseCreditorUpdateErrorMessage(http.StatusConflict, &sml.PurchaseOrderCreditorUpdateResponse{
		Error: map[string]interface{}{"code": "creditor_changed", "message": "changed elsewhere"},
	}, nil)
	if !strings.Contains(got409, "reload") {
		t.Fatalf("409 message = %q", got409)
	}
}
