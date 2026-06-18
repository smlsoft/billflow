package handlers

import (
	"bytes"
	"testing"
	"time"

	"billflow/internal/models"
	"github.com/xuri/excelize/v2"
)

func TestBuildCreditCardReportWorkbookHasExpectedSheets(t *testing.T) {
	charge := 7417.69
	diff := 0.0
	run := &models.CreditCardReportRun{
		ID:         "run-1",
		ReportName: "รายงานบัตรเครดิต TT0972",
		Filters: models.CreditCardReportFilter{
			DateFrom:      "2026-06-01",
			DateTo:        "2026-06-30",
			PaymentMethod: "TT0972",
			Source:        "all",
		},
		Snapshot: models.CreditCardReportPreview{
			Groups: []models.CreditCardReportGroup{{
				GroupID:        "lazada:g1",
				Source:         "lazada_email",
				SourceLabel:    "Lazada",
				ChargeTime:     "2026-06-11T16:45:00+07:00",
				PaymentMethods: []string{"TT0972"},
				ChargeAmount:   &charge,
				OrderTotal:     7417.69,
				Diff:           &diff,
				OrderCount:     1,
				POLCount:       1,
				Orders: []models.CreditCardReportOrder{{
					BillID:     "bill-1",
					OrderID:    "1109337756759692",
					SellerName: "seller",
					SMLDocNo:   "POL26060022",
					Status:     "sent",
					OrderTotal: 7417.69,
					DocRef:     "7417.69",
				}},
			}},
		},
		Summary: models.CreditCardReportSummary{
			GroupCount:  1,
			OrderCount:  1,
			ChargeTotal: 7417.69,
			OrderTotal:  7417.69,
		},
		CreatedAt: time.Date(2026, 6, 18, 10, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 6, 18, 10, 0, 0, 0, time.UTC),
	}

	data, err := buildCreditCardReportWorkbook(run)
	if err != nil {
		t.Fatalf("build workbook: %v", err)
	}
	f, err := excelize.OpenReader(bytesReader(data))
	if err != nil {
		t.Fatalf("open workbook: %v", err)
	}
	defer f.Close()
	want := []string{"รายงานบัตรเครดิต", "สรุปยอด", "ต้องตรวจสอบ"}
	got := f.GetSheetList()
	for i, sheet := range want {
		if i >= len(got) || got[i] != sheet {
			t.Fatalf("sheets = %#v, want %#v", got, want)
		}
	}
	cell, err := f.GetCellValue("รายงานบัตรเครดิต", "E2")
	if err != nil {
		t.Fatalf("cell E2: %v", err)
	}
	if cell == "" {
		t.Fatalf("cell E2 should contain charge amount")
	}
}

func bytesReader(data []byte) *bytes.Reader {
	return bytes.NewReader(data)
}
