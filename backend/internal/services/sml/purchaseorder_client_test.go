package sml

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBuildPurchaseOrderPayloadAppliesLineDiscounts(t *testing.T) {
	payload := BuildPurchaseOrderPayload(
		"PO-1",
		"2026-05-21",
		"#2605211KR3XK1G",
		"2026-05-21",
		[]POItem{
			{ItemCode: "ITEM-1", ItemName: "สินค้า 1", Qty: 1, Price: 100, DiscountAmount: 10, UnitCode: "ชิ้น"},
			{ItemCode: "ITEM-2", ItemName: "สินค้า 2", Qty: 2, Price: 50, DiscountAmount: 5, UnitCode: "ชิ้น"},
		},
		PurchaseOrderConfig{DocFormat: "PO", CustCode: "V001", VATType: 2, VATRate: 0, UnitCode: "ชิ้น"},
		"",
	)

	if len(payload.Items) != 2 {
		t.Fatalf("items = %d, want 2", len(payload.Items))
	}
	if payload.Items[0].DiscountAmount != 10 || payload.Items[0].SumAmount != 90 {
		t.Fatalf("first line = %+v, want discount=10 sum=90", payload.Items[0])
	}
	if payload.Items[1].DiscountAmount != 5 || payload.Items[1].SumAmount != 95 {
		t.Fatalf("second line = %+v, want discount=5 sum=95", payload.Items[1])
	}
	if payload.TotalValue != 185 || payload.TotalDiscount != 0 || payload.TotalAmount != 185 {
		t.Fatalf("totals value=%v discount=%v amount=%v, want 185/0/185",
			payload.TotalValue, payload.TotalDiscount, payload.TotalAmount)
	}
}

func TestBuildPurchaseOrderPayloadCapsLineDiscountAtGross(t *testing.T) {
	payload := BuildPurchaseOrderPayload(
		"PO-1",
		"2026-05-21",
		"#2605211KR3XK1G",
		"2026-05-21",
		[]POItem{{ItemCode: "ITEM-1", Qty: 1, Price: 100, DiscountAmount: 150, UnitCode: "ชิ้น"}},
		PurchaseOrderConfig{DocFormat: "PO", CustCode: "V001", VATType: 2, UnitCode: "ชิ้น"},
		"",
	)

	if payload.Items[0].DiscountAmount != 100 || payload.Items[0].SumAmount != 0 {
		t.Fatalf("line = %+v, want capped discount=100 sum=0", payload.Items[0])
	}
	if payload.TotalValue != 0 || payload.TotalDiscount != 0 || payload.TotalAmount != 0 {
		t.Fatalf("totals value=%v discount=%v amount=%v, want 0/0/0",
			payload.TotalValue, payload.TotalDiscount, payload.TotalAmount)
	}
}

func TestBuildPurchaseOrderPayloadIncludesHeaderFields(t *testing.T) {
	payload := BuildPurchaseOrderPayload(
		"PO-1",
		"2026-05-21",
		"7275",
		"",
		[]POItem{{ItemCode: "ITEM-1", Qty: 1, Price: 100, UnitCode: "ชิ้น"}},
		PurchaseOrderConfig{DocFormat: "PO", CustCode: "V001", VATType: 2, UnitCode: "ชิ้น", WHCode: "WH-01", ShelfCode: "SH-01"},
		"alove922",
		PurchaseOrderHeaderOptions{
			Remark:      "alove922",
			Remark2:     "tax",
			Remark5:     "2605211KR3XK1G",
			InquiryType: 1,
		},
	)

	if payload.Remark != "alove922" || payload.Remark2 != "tax" || payload.Remark5 != "2605211KR3XK1G" {
		t.Fatalf("remark fields = %q/%q/%q", payload.Remark, payload.Remark2, payload.Remark5)
	}
	if payload.DocRef != "7275" || payload.Items[0].DocRef != "7275" {
		t.Fatalf("doc_ref payload=%q item=%q, want 7275", payload.DocRef, payload.Items[0].DocRef)
	}
	if payload.InquiryType != 1 {
		t.Fatalf("inquiry_type = %d, want 1", payload.InquiryType)
	}
	if payload.Items[0].WHCode2 != "WH-01" || payload.Items[0].ShelfCode2 != "SH-01" {
		t.Fatalf("secondary warehouse/shelf = %q/%q, want WH-01/SH-01", payload.Items[0].WHCode2, payload.Items[0].ShelfCode2)
	}
}

func TestPurchaseOrderPayloadJSONPreservesTwoDecimalMoney(t *testing.T) {
	payload := BuildPurchaseOrderPayload(
		"POL26060044",
		"2026-06-18",
		"6597.10",
		"2026-06-18",
		[]POItem{
			{ItemCode: "CH0002BK", ItemName: "เก้าอี้", Qty: 5, Price: 699, DiscountAmount: 69.9, UnitCode: "ชิ้น"},
			{ItemCode: "SHIP_CUS", ItemName: "ค่าบริการส่งสินค้า", Qty: 1, Price: 479, UnitCode: "บาท"},
		},
		PurchaseOrderConfig{DocFormat: "POL", CustCode: "AF00001", VATType: 2, VATRate: 7, UnitCode: "ชิ้น"},
		"Thaidi Bear",
	)

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range [][]byte{
		[]byte(`"total_amount":3904.10`),
		[]byte(`"total_value":3904.10`),
		[]byte(`"discount_amount":69.90`),
		[]byte(`"sum_amount":3425.10`),
		[]byte(`"price":479.00`),
	} {
		if !bytes.Contains(body, want) {
			t.Fatalf("payload JSON missing %s: %s", want, body)
		}
	}

	asciiBody, err := marshalASCII(payload)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(asciiBody, []byte(`"total_amount":3904.10`)) {
		t.Fatalf("ASCII payload lost two-decimal amount: %s", asciiBody)
	}
}

func TestSaleAndInvoicePayloadIncludeRemark2WhenProvided(t *testing.T) {
	saleOrder := BuildSaleOrderPayload(
		"SO-1",
		"2026-05-25",
		"",
		"",
		[]SOItem{{ItemCode: "ITEM-1", Qty: 1, Price: 100}},
		SaleOrderConfig{DocFormat: "SO", CustCode: "AR001", VATType: 2, UnitCode: "ชิ้น"},
		"",
		SaleOrderHeaderOptions{Remark2: "notax"},
	)
	if saleOrder.Remark2 != "notax" {
		t.Fatalf("saleorder remark_2 = %q, want notax", saleOrder.Remark2)
	}

	invoice := BuildInvoicePayload(
		"INV-1",
		"2026-05-25",
		"",
		"",
		[]ShopeeOrderItem{{SKU: "ITEM-1", ProductName: "สินค้า", Qty: 1, Price: 100}},
		InvoiceConfig{DocFormat: "INV", CustCode: "AR001", VATType: 2, UnitCode: "ชิ้น"},
		map[string]*ProductInfo{},
		"",
		InvoiceHeaderOptions{Remark2: "re"},
	)
	if invoice.Remark2 != "re" {
		t.Fatalf("saleinvoice remark_2 = %q, want re", invoice.Remark2)
	}
}

func TestRemark2IsOmittedWhenEmpty(t *testing.T) {
	payload := BuildPurchaseOrderPayload(
		"PO-1",
		"2026-05-25",
		"",
		"",
		[]POItem{{ItemCode: "ITEM-1", Qty: 1, Price: 100, UnitCode: "ชิ้น"}},
		PurchaseOrderConfig{DocFormat: "PO", CustCode: "V001", VATType: 2, UnitCode: "ชิ้น"},
		"",
	)
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(body, []byte("remark_2")) {
		t.Fatalf("remark_2 should be omitted when empty: %s", string(body))
	}
}

func TestUpdatePurchaseOrderCreditorPATCHesExpectedPayload(t *testing.T) {
	var gotMethod, gotPath, gotGUID string
	var gotBody PurchaseOrderCreditorUpdateRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.EscapedPath()
		gotGUID = r.Header.Get("guid")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"success": true,
			"status": "success",
			"message": "updated",
			"data": {
				"doc_no": "PO26060011",
				"old_cust_code": "AF00001",
				"new_cust_code": "AF00007",
				"supplier_name": "TT3086",
				"updated_detail_rows": 2,
				"changed": true
			}
		}`))
	}))
	defer srv.Close()

	client := NewPurchaseOrderClient(PurchaseOrderConfig{
		BaseURL:    srv.URL,
		GUID:       "tenant-guid",
		Provider:   "provider",
		ConfigFile: "config",
		Database:   "data1",
	}, nil)

	status, resp, err := client.UpdatePurchaseOrderCreditor("PO26060011", PurchaseOrderCreditorUpdateRequest{
		CustCode:            "AF00007",
		SupplierName:        "TT3086",
		ExpectedOldCustCode: "AF00001",
	}, "")
	if err != nil {
		t.Fatalf("UpdatePurchaseOrderCreditor returned error: %v", err)
	}
	if status != http.StatusOK || resp == nil || !resp.Success {
		t.Fatalf("response = status %d %#v, want success", status, resp)
	}
	if gotMethod != http.MethodPatch {
		t.Fatalf("method = %q, want PATCH", gotMethod)
	}
	if gotPath != "/api/v1/ic/purchase-orders/PO26060011/creditor" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotGUID != "tenant-guid" {
		t.Fatalf("guid header = %q", gotGUID)
	}
	if gotBody.CustCode != "AF00007" || gotBody.SupplierName != "TT3086" || gotBody.ExpectedOldCustCode != "AF00001" {
		t.Fatalf("body = %#v", gotBody)
	}
	if !resp.Data.Changed || resp.Data.UpdatedDetailRows != 2 {
		t.Fatalf("data = %#v", resp.Data)
	}
}
