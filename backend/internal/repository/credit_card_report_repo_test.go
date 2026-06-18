package repository

import (
	"testing"
	"time"

	"billflow/internal/models"
)

func TestCreditCardReportShopeeMultiOrderGroupsByEmailMessage(t *testing.T) {
	charge := 11172.0
	rows := []creditCardReportRow{
		{
			BillID:                      "bill-1",
			Source:                      "shopee_shipped",
			Status:                      "sent",
			SMLDocNo:                    "POL26060001",
			EffectivePrintPaymentMethod: "TT0972",
			OrderID:                     "260613AAA",
			SellerName:                  "shop one",
			EmailDate:                   "2026-06-13T16:14:29+07:00",
			EmailMessageID:              "msg-shopee",
			ShopeeChargeAmount:          &charge,
			OrderTotal:                  1000,
			DocRef:                      "11172",
			CreatedAt:                   reportTestTime("2026-06-13T16:15:00+07:00"),
		},
		{
			BillID:             "bill-2",
			Source:             "shopee_shipped",
			Status:             "needs_review",
			OrderID:            "260613BBB",
			SellerName:         "shop two",
			EmailDate:          "2026-06-13T16:14:29+07:00",
			EmailMessageID:     "msg-shopee",
			ShopeeChargeAmount: &charge,
			OrderTotal:         10172,
			DocRef:             "11172",
			CreatedAt:          reportTestTime("2026-06-13T16:16:00+07:00"),
		},
	}

	groups := buildCreditCardReportGroups(rows, models.CreditCardReportFilter{
		DateFrom:      "2026-06-13",
		DateTo:        "2026-06-13",
		PaymentMethod: "TT0972",
	})

	if len(groups) != 1 {
		t.Fatalf("groups = %d, want 1", len(groups))
	}
	group := groups[0]
	if group.GroupID != "shopee:msg-shopee" {
		t.Fatalf("group id = %q, want shopee:msg-shopee", group.GroupID)
	}
	if group.ChargeAmount == nil || *group.ChargeAmount != 11172 {
		t.Fatalf("charge = %#v, want 11172", group.ChargeAmount)
	}
	if group.OrderCount != 2 || group.OrderTotal != 11172 {
		t.Fatalf("orders=%d total=%v, want 2/11172", group.OrderCount, group.OrderTotal)
	}
}

func TestCreditCardReportExcludesShopeeMissingPaymentSummaryByDefault(t *testing.T) {
	rows := []creditCardReportRow{{
		BillID:         "legacy-shipping-only",
		Source:         "shopee_shipped",
		Status:         "needs_review",
		OrderID:        "260613LEGACY",
		EmailDate:      "2026-06-13T16:14:29+07:00",
		EmailMessageID: "msg-legacy",
		OrderTotal:     183,
		CreatedAt:      reportTestTime("2026-06-13T16:15:00+07:00"),
	}}

	groups := buildCreditCardReportGroups(rows, models.CreditCardReportFilter{
		DateFrom: "2026-06-13",
		DateTo:   "2026-06-13",
	})
	if len(groups) != 0 {
		t.Fatalf("groups = %d, want 0 for default preview", len(groups))
	}

	groups = buildCreditCardReportGroups(rows, models.CreditCardReportFilter{
		DateFrom:          "2026-06-13",
		DateTo:            "2026-06-13",
		IncludeIncomplete: true,
	})
	if len(groups) != 1 {
		t.Fatalf("include incomplete groups = %d, want 1", len(groups))
	}
	if !hasCreditCardReportIssue(groups[0], "missing_charge_amount") {
		t.Fatalf("issues = %#v, want missing_charge_amount", groups[0].Issues)
	}
}

func TestCreditCardReportLazadaChargeGroupSumsPaidAmountsAndFiltersAtGroupLevel(t *testing.T) {
	paidA := 1092.68
	paidB := 6325.01
	rows := []creditCardReportRow{
		{
			BillID:                      "lazada-1",
			Source:                      "lazada_email",
			Status:                      "sent",
			SMLDocNo:                    "POL26060022",
			EffectivePrintPaymentMethod: "TT2789",
			OrderID:                     "1109337756759692",
			SellerName:                  "N.I.C.nickfurniture",
			LazadaConfirmedAt:           "2026-06-11 16:45:00",
			EmailMessageID:              "lazada-msg-1",
			LazadaGroupKey:              "lazada-20260611-1645",
			LazadaPaidAmount:            &paidA,
			OrderTotal:                  1092.68,
			DocRef:                      "7417.69",
			CreatedAt:                   reportTestTime("2026-06-11T16:46:00+07:00"),
		},
		{
			BillID:                      "lazada-2",
			Source:                      "lazada_email",
			Status:                      "needs_review",
			EffectivePrintPaymentMethod: "TT0972",
			OrderID:                     "1109337756759693",
			SellerName:                  "Tok Lae Dee Thailand",
			LazadaConfirmedAt:           "2026-06-11 16:45:00",
			EmailMessageID:              "lazada-msg-2",
			LazadaGroupKey:              "lazada-20260611-1645",
			LazadaPaidAmount:            &paidB,
			OrderTotal:                  6325.01,
			DocRef:                      "7417.69",
			CreatedAt:                   reportTestTime("2026-06-11T16:46:30+07:00"),
		},
	}

	groups := buildCreditCardReportGroups(rows, models.CreditCardReportFilter{
		DateFrom:      "2026-06-11",
		DateTo:        "2026-06-11",
		PaymentMethod: "TT2789",
	})

	if len(groups) != 1 {
		t.Fatalf("groups = %d, want 1", len(groups))
	}
	group := groups[0]
	if group.GroupID != "lazada:lazada-20260611-1645" {
		t.Fatalf("group id = %q, want Lazada group key", group.GroupID)
	}
	if group.ChargeAmount == nil || *group.ChargeAmount != 7417.69 {
		t.Fatalf("charge = %#v, want 7417.69", group.ChargeAmount)
	}
	if group.OrderCount != 2 || group.OrderTotal != 7417.69 {
		t.Fatalf("orders=%d total=%v, want 2/7417.69", group.OrderCount, group.OrderTotal)
	}
	if !hasCreditCardReportIssue(group, "mixed_payment_method") {
		t.Fatalf("issues = %#v, want mixed_payment_method", group.Issues)
	}
}

func reportTestTime(value string) time.Time {
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		panic(err)
	}
	return t
}
