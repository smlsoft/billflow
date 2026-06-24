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
				OrderCount:     2,
				POLCount:       2,
				Orders: []models.CreditCardReportOrder{{
					BillID:     "bill-1",
					OrderID:    "#1109337756759692",
					SellerName: "seller",
					SMLDocNo:   "POL26060022",
					Status:     "sent",
					OrderTotal: 3000,
					DocRef:     "7417.69",
				}, {
					BillID:     "bill-2",
					OrderID:    "#1109337756759693",
					SellerName: "seller two",
					SMLDocNo:   "POL26060023",
					Status:     "sent",
					OrderTotal: 4417.69,
					DocRef:     "7417.69",
				}},
			}},
		},
		Summary: models.CreditCardReportSummary{
			GroupCount:  1,
			OrderCount:  2,
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
	want := []string{"รายงานบัตรเครดิต", "สรุปยอด", "ต้องตรวจสอบ", "ยอดไม่ตรงจริง", "ข้อมูลยังไม่ครบ"}
	got := f.GetSheetList()
	for i, sheet := range want {
		if i >= len(got) || got[i] != sheet {
			t.Fatalf("sheets = %#v, want %#v", got, want)
		}
	}
	cell, err := f.GetCellValue("รายงานบัตรเครดิต", "F2")
	if err != nil {
		t.Fatalf("cell F2: %v", err)
	}
	if cell == "" {
		t.Fatalf("cell F2 should contain charge amount")
	}
	assertCellValue(t, f, "รายงานบัตรเครดิต", "B1", "วันที่จากอีเมล")
	assertCellValue(t, f, "รายงานบัตรเครดิต", "C1", "เวลาจากอีเมล")
	assertCellValue(t, f, "รายงานบัตรเครดิต", "B2", "11/06/2026")
	assertCellValue(t, f, "รายงานบัตรเครดิต", "C2", "16:45:00")
	assertCellValue(t, f, "รายงานบัตรเครดิต", "G1", "ยอดรวมบิลใน BillFlow")
	assertCellValue(t, f, "รายงานบัตรเครดิต", "H1", "ต่างจากยอดรูด")
	assertCellValue(t, f, "รายงานบัตรเครดิต", "J2", "1109337756759692")
	assertCellValue(t, f, "ต้องตรวจสอบ", "E1", "ประเภทปัญหา")
	assertCellValue(t, f, "ต้องตรวจสอบ", "F1", "สาเหตุที่พบ")
	assertCellValue(t, f, "ยอดไม่ตรงจริง", "E1", "ประเภทปัญหา")
	assertCellValue(t, f, "ข้อมูลยังไม่ครบ", "E1", "ประเภทปัญหา")

	dailyTitleRow := findCellRow(t, f, "สรุปยอด", "สรุปรายวันจาก BillFlow")
	if dailyTitleRow == 0 {
		t.Fatalf("daily summary title not found")
	}
	dailyHeaderRow := dailyTitleRow + 1
	assertCellValue(t, f, "สรุปยอด", cellName(t, 1, dailyHeaderRow), "วันที่จากอีเมล")
	assertCellValue(t, f, "สรุปยอด", cellName(t, 5, dailyHeaderRow), "ยอดรูดบัตรรวม")
	dailyDataRow := dailyHeaderRow + 1
	assertCellValue(t, f, "สรุปยอด", cellName(t, 1, dailyDataRow), "11/06/2026")
	assertCellValue(t, f, "สรุปยอด", cellName(t, 3, dailyDataRow), "1")
	assertCellValue(t, f, "สรุปยอด", cellName(t, 4, dailyDataRow), "2")
	assertCellValue(t, f, "สรุปยอด", cellName(t, 5, dailyDataRow), "7417.69")
	noteRow := findCellRow(t, f, "สรุปยอด", "หมายเหตุ")
	if noteRow == 0 {
		t.Fatalf("summary note row not found")
	}
	assertCellValue(t, f, "สรุปยอด", cellName(t, 2, noteRow), "รายงานนี้ยังไม่รวมยอดคืนเงิน/ยอดติดลบจาก statement และยอดไม่ตรงจริงไม่ใช่จำนวนเดียวกับกลุ่มที่ต้องตรวจสอบทั้งหมด")
}

func bytesReader(data []byte) *bytes.Reader {
	return bytes.NewReader(data)
}

func assertCellValue(t *testing.T, f *excelize.File, sheet, cell, want string) {
	t.Helper()
	got, err := f.GetCellValue(sheet, cell, excelize.Options{RawCellValue: true})
	if err != nil {
		t.Fatalf("cell %s!%s: %v", sheet, cell, err)
	}
	if got != want {
		t.Fatalf("cell %s!%s = %q, want %q", sheet, cell, got, want)
	}
}

func findCellRow(t *testing.T, f *excelize.File, sheet, want string) int {
	t.Helper()
	rows, err := f.GetRows(sheet)
	if err != nil {
		t.Fatalf("get rows %s: %v", sheet, err)
	}
	for r, row := range rows {
		for _, value := range row {
			if value == want {
				return r + 1
			}
		}
	}
	return 0
}

func cellName(t *testing.T, col, row int) string {
	t.Helper()
	cell, err := excelize.CoordinatesToCellName(col, row)
	if err != nil {
		t.Fatalf("cell name col=%d row=%d: %v", col, row, err)
	}
	return cell
}
