package handlers

import (
	"bytes"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"billflow/internal/models"
	"billflow/internal/repository"
	"github.com/gin-gonic/gin"
	"github.com/xuri/excelize/v2"
	"go.uber.org/zap"
)

type CreditCardReportHandler struct {
	repo      *repository.CreditCardReportRepo
	billRepo  *repository.BillRepo
	auditRepo *repository.AuditLogRepo
	log       *zap.Logger
}

var creditCardReportExportLocation = mustCreditCardReportBangkokLocation()

func NewCreditCardReportHandler(repo *repository.CreditCardReportRepo, billRepo *repository.BillRepo, auditRepo *repository.AuditLogRepo, log *zap.Logger) *CreditCardReportHandler {
	return &CreditCardReportHandler{repo: repo, billRepo: billRepo, auditRepo: auditRepo, log: log}
}

func (h *CreditCardReportHandler) Preview(c *gin.Context) {
	var f models.CreditCardReportFilter
	if err := c.ShouldBindQuery(&f); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := validateCreditCardReportFilter(f); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	preview, err := h.repo.Preview(f)
	if err != nil {
		h.log.Error("credit card report preview", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "โหลดรายงานไม่สำเร็จ"})
		return
	}
	c.JSON(http.StatusOK, preview)
}

type createCreditCardReportRunRequest struct {
	ReportName       string                        `json:"report_name"`
	Filters          models.CreditCardReportFilter `json:"filters"`
	SelectedGroupIDs []string                      `json:"selected_group_ids"`
}

func (h *CreditCardReportHandler) CreateRun(c *gin.Context) {
	var req createCreditCardReportRunRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := validateCreditCardReportFilter(req.Filters); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	run, err := h.repo.CreateRun(req.Filters, req.ReportName, req.SelectedGroupIDs, c.GetString("user_id"), c.GetString("user_email"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	h.logAudit(c, "credit_card_report_run_created", run.ID, map[string]interface{}{
		"report_name":       run.ReportName,
		"date_from":         run.Filters.DateFrom,
		"date_to":           run.Filters.DateTo,
		"payment_method":    run.Filters.PaymentMethod,
		"source":            run.Filters.Source,
		"group_count":       run.Summary.GroupCount,
		"order_count":       run.Summary.OrderCount,
		"charge_total":      run.Summary.ChargeTotal,
		"issue_group_count": run.Summary.IssueGroupCount,
	})
	c.JSON(http.StatusOK, run)
}

func (h *CreditCardReportHandler) GetRun(c *gin.Context) {
	run, err := h.repo.FindRun(c.Param("id"))
	if err != nil {
		h.log.Error("credit card report get run", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "โหลดรอบรายงานไม่สำเร็จ"})
		return
	}
	if run == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "ไม่พบรอบรายงาน"})
		return
	}
	c.JSON(http.StatusOK, run)
}

func (h *CreditCardReportHandler) ListRuns(c *gin.Context) {
	runs, err := h.repo.ListRuns(20)
	if err != nil {
		h.log.Error("credit card report list runs", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "โหลดประวัติรอบรายงานไม่สำเร็จ"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": runs})
}

func (h *CreditCardReportHandler) ExportXLSX(c *gin.Context) {
	run, err := h.repo.FindRun(c.Param("id"))
	if err != nil {
		h.log.Error("credit card report export load", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "โหลดรอบรายงานไม่สำเร็จ"})
		return
	}
	if run == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "ไม่พบรอบรายงาน"})
		return
	}
	data, err := buildCreditCardReportWorkbook(run)
	if err != nil {
		h.log.Error("credit card report export xlsx", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "สร้างไฟล์ Excel ไม่สำเร็จ"})
		return
	}
	_ = h.repo.MarkExported(run.ID, c.GetString("user_id"))
	h.logAudit(c, "credit_card_report_exported", run.ID, map[string]interface{}{
		"report_name":    run.ReportName,
		"group_count":    run.Summary.GroupCount,
		"order_count":    run.Summary.OrderCount,
		"charge_total":   run.Summary.ChargeTotal,
		"payment_method": run.Filters.PaymentMethod,
	})
	filename := creditCardReportFilename(run)
	c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", data)
}

func (h *CreditCardReportHandler) RecordPrintEvents(c *gin.Context) {
	run, err := h.repo.FindRun(c.Param("id"))
	if err != nil {
		h.log.Error("credit card report print load", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "โหลดรอบรายงานไม่สำเร็จ"})
		return
	}
	if run == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "ไม่พบรอบรายงาน"})
		return
	}
	type skipped struct {
		GroupID string `json:"group_id"`
		Reason  string `json:"reason"`
	}
	events := []models.EmailPrintEvent{}
	skippedGroups := []skipped{}
	for _, group := range run.Snapshot.Groups {
		if !group.PrintReady {
			reason := strings.TrimSpace(group.PrintBlockReason)
			if reason == "" {
				reason = "รายการนี้ยังไม่พร้อมพิมพ์"
			}
			skippedGroups = append(skippedGroups, skipped{GroupID: group.GroupID, Reason: reason})
			continue
		}
		for _, artifact := range group.PrintArtifacts {
			event, err := h.billRepo.RecordEmailPrintEvent(artifact.BillID, artifact.ArtifactID, c.GetString("user_id"), c.GetString("user_email"))
			if err != nil {
				skippedGroups = append(skippedGroups, skipped{GroupID: group.GroupID, Reason: err.Error()})
				continue
			}
			if event != nil {
				events = append(events, *event)
			}
		}
	}
	summary := map[string]interface{}{
		"event_count":   len(events),
		"skipped_count": len(skippedGroups),
		"group_count":   len(run.Snapshot.Groups),
	}
	if len(events) > 0 {
		_ = h.repo.MarkPrinted(run.ID, c.GetString("user_id"), summary)
	}
	h.logAudit(c, "credit_card_report_print_requested", run.ID, summary)
	c.JSON(http.StatusOK, gin.H{"data": events, "skipped": skippedGroups, "summary": summary})
}

func validateCreditCardReportFilter(f models.CreditCardReportFilter) error {
	if strings.TrimSpace(f.DateFrom) == "" || strings.TrimSpace(f.DateTo) == "" {
		return fmt.Errorf("กรุณาเลือกวันที่เริ่มต้นและวันที่สิ้นสุด")
	}
	from, err := time.Parse("2006-01-02", strings.TrimSpace(f.DateFrom))
	if err != nil {
		return fmt.Errorf("วันที่เริ่มต้นไม่ถูกต้อง")
	}
	to, err := time.Parse("2006-01-02", strings.TrimSpace(f.DateTo))
	if err != nil {
		return fmt.Errorf("วันที่สิ้นสุดไม่ถูกต้อง")
	}
	if to.Before(from) {
		return fmt.Errorf("วันที่สิ้นสุดต้องไม่น้อยกว่าวันที่เริ่มต้น")
	}
	source := strings.TrimSpace(f.Source)
	if source != "" && source != "all" && source != "shopee_shipped" && source != "lazada_email" {
		return fmt.Errorf("ช่องทางไม่ถูกต้อง")
	}
	return nil
}

func (h *CreditCardReportHandler) logAudit(c *gin.Context, action, targetID string, detail map[string]interface{}) {
	if h.auditRepo == nil {
		return
	}
	var userID *string
	if uid := strings.TrimSpace(c.GetString("user_id")); uid != "" {
		userID = &uid
	}
	var target *string
	if targetID = strings.TrimSpace(targetID); targetID != "" {
		target = &targetID
	}
	_ = h.auditRepo.Log(models.AuditEntry{
		Action:   action,
		TargetID: target,
		UserID:   userID,
		Source:   "credit_card_report",
		Level:    "info",
		TraceID:  c.GetString("trace_id"),
		Detail:   detail,
	})
}

func buildCreditCardReportWorkbook(run *models.CreditCardReportRun) ([]byte, error) {
	f := excelize.NewFile()
	defer f.Close()
	detailSheet := "รายงานบัตรเครดิต"
	summarySheet := "สรุปยอด"
	issueSheet := "ต้องตรวจสอบ"
	f.SetSheetName("Sheet1", detailSheet)
	_, _ = f.NewSheet(summarySheet)
	_, _ = f.NewSheet(issueSheet)

	headerStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "1F2937"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"EAF4F5"}, Pattern: 1},
		Alignment: &excelize.Alignment{Vertical: "center"},
		Border: []excelize.Border{
			{Type: "bottom", Color: "BFD7DB", Style: 1},
		},
	})
	moneyStyle, _ := f.NewStyle(&excelize.Style{NumFmt: 4})
	warnStyle, _ := f.NewStyle(&excelize.Style{Fill: excelize.Fill{Type: "pattern", Color: []string{"FFF4CE"}, Pattern: 1}})

	writeReportDetails(f, detailSheet, run, headerStyle, moneyStyle, warnStyle)
	writeReportSummary(f, summarySheet, run, headerStyle, moneyStyle)
	writeReportIssues(f, issueSheet, run, headerStyle, moneyStyle)
	f.SetActiveSheet(0)
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeReportDetails(f *excelize.File, sheet string, run *models.CreditCardReportRun, headerStyle, moneyStyle, warnStyle int) {
	headers := []string{
		"ลำดับยอดรูดบัตร", "วันที่จากอีเมล", "เวลาจากอีเมล", "ช่องทาง", "วิธีชำระเงิน", "ยอดรูดบัตร",
		"ยอดรวมบิลใน BillFlow", "ต่างจากยอดรูด", "POL", "เลขคำสั่งซื้อ", "ผู้ขาย", "ยอดบิล", "สถานะ SML", "doc_ref", "หมายเหตุ",
	}
	writeHeader(f, sheet, headers, headerStyle)
	row := 2
	for i, group := range run.Snapshot.Groups {
		note := issueMessages(group.Issues)
		paymentMethod := strings.Join(group.PaymentMethods, ", ")
		chargeDate, chargeTime := creditCardReportExportDateTime(group.ChargeTime)
		for _, order := range group.Orders {
			values := []interface{}{
				i + 1,
				chargeDate,
				chargeTime,
				group.SourceLabel,
				paymentMethod,
				nullableFloat(group.ChargeAmount),
				group.OrderTotal,
				nullableFloat(group.Diff),
				order.SMLDocNo,
				creditCardReportDisplayOrderID(order.OrderID),
				order.SellerName,
				order.OrderTotal,
				order.Status,
				order.DocRef,
				note,
			}
			writeRow(f, sheet, row, values)
			if len(group.Issues) > 0 {
				_ = f.SetCellStyle(sheet, fmt.Sprintf("A%d", row), fmt.Sprintf("O%d", row), warnStyle)
			}
			row++
		}
	}
	setReportWidths(f, sheet)
	_ = f.SetCellStyle(sheet, "F2", fmt.Sprintf("H%d", max(row-1, 2)), moneyStyle)
	_ = f.SetCellStyle(sheet, "L2", fmt.Sprintf("L%d", max(row-1, 2)), moneyStyle)
	_ = f.AutoFilter(sheet, fmt.Sprintf("A1:O%d", max(row-1, 1)), nil)
}

func writeReportSummary(f *excelize.File, sheet string, run *models.CreditCardReportRun, headerStyle, moneyStyle int) {
	rows := [][]interface{}{
		{"ชื่อรอบ", run.ReportName},
		{"วันที่", run.Filters.DateFrom + " ถึง " + run.Filters.DateTo},
		{"วิธีชำระเงิน", emptyDash(run.Filters.PaymentMethod)},
		{"ช่องทาง", emptyDash(run.Filters.Source)},
		{"จำนวนยอดรูดบัตร", run.Summary.GroupCount},
		{"จำนวนคำสั่งซื้อ", run.Summary.OrderCount},
		{"ยอดรูดบัตรรวม", run.Summary.ChargeTotal},
		{"ยอดรวมบิลใน BillFlow", run.Summary.OrderTotal},
		{"กลุ่มที่ต้องตรวจสอบ", run.Summary.IssueGroupCount},
		{"สร้างเมื่อ", run.CreatedAt.Format(time.RFC3339)},
	}
	for i, row := range rows {
		writeRow(f, sheet, i+1, row)
	}
	_ = f.SetCellStyle(sheet, "A1", "A10", headerStyle)
	_ = f.SetCellStyle(sheet, "B7", "B8", moneyStyle)
	_ = f.SetColWidth(sheet, "A", "A", 28)
	_ = f.SetColWidth(sheet, "B", "B", 34)

	noteRow := len(rows) + 2
	_ = f.SetCellValue(sheet, fmt.Sprintf("A%d", noteRow), "หมายเหตุ")
	_ = f.SetCellValue(sheet, fmt.Sprintf("B%d", noteRow), "รายงานนี้ยังไม่รวมยอดคืนเงิน/ยอดติดลบจาก statement")
	_ = f.SetCellStyle(sheet, fmt.Sprintf("A%d", noteRow), fmt.Sprintf("A%d", noteRow), headerStyle)

	platformHeaderRow := noteRow + 2
	_ = f.SetCellValue(sheet, fmt.Sprintf("A%d", platformHeaderRow), "ช่องทาง")
	_ = f.SetCellValue(sheet, fmt.Sprintf("B%d", platformHeaderRow), "จำนวนยอดรูดบัตร")
	_ = f.SetCellValue(sheet, fmt.Sprintf("C%d", platformHeaderRow), "จำนวนคำสั่งซื้อ")
	_ = f.SetCellValue(sheet, fmt.Sprintf("D%d", platformHeaderRow), "ยอดรูดบัตรรวม")
	_ = f.SetCellValue(sheet, fmt.Sprintf("E%d", platformHeaderRow), "ยอดรวมบิลใน BillFlow")
	_ = f.SetCellStyle(sheet, fmt.Sprintf("A%d", platformHeaderRow), fmt.Sprintf("E%d", platformHeaderRow), headerStyle)
	type agg struct {
		Groups int
		Orders int
		Charge float64
		Total  float64
	}
	bySource := map[string]agg{}
	for _, group := range run.Snapshot.Groups {
		a := bySource[group.SourceLabel]
		a.Groups++
		a.Orders += group.OrderCount
		if group.ChargeAmount != nil {
			a.Charge += *group.ChargeAmount
		}
		a.Total += group.OrderTotal
		bySource[group.SourceLabel] = a
	}
	keys := make([]string, 0, len(bySource))
	for key := range bySource {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for i, key := range keys {
		a := bySource[key]
		writeRow(f, sheet, platformHeaderRow+1+i, []interface{}{key, a.Groups, a.Orders, a.Charge, a.Total})
	}
	if len(keys) > 0 {
		_ = f.SetCellStyle(sheet, fmt.Sprintf("D%d", platformHeaderRow+1), fmt.Sprintf("E%d", platformHeaderRow+len(keys)), moneyStyle)
	}

	dailyTitleRow := platformHeaderRow + len(keys) + 3
	_ = f.SetCellValue(sheet, fmt.Sprintf("A%d", dailyTitleRow), "สรุปรายวันจาก BillFlow")
	_ = f.SetCellStyle(sheet, fmt.Sprintf("A%d", dailyTitleRow), fmt.Sprintf("A%d", dailyTitleRow), headerStyle)
	dailyHeaderRow := dailyTitleRow + 1
	dailyHeaders := []string{
		"วันที่จากอีเมล", "วิธีชำระเงิน", "จำนวนยอดรูดบัตร", "จำนวนคำสั่งซื้อ",
		"ยอดรูดบัตรรวม", "ยอดรวมบิลใน BillFlow", "ต่างจากยอดรูด", "จำนวนกลุ่มที่ต้องตรวจ",
	}
	for i, header := range dailyHeaders {
		cell, _ := excelize.CoordinatesToCellName(i+1, dailyHeaderRow)
		_ = f.SetCellValue(sheet, cell, header)
	}
	lastDailyHeader, _ := excelize.CoordinatesToCellName(len(dailyHeaders), dailyHeaderRow)
	_ = f.SetCellStyle(sheet, fmt.Sprintf("A%d", dailyHeaderRow), lastDailyHeader, headerStyle)
	dailyRows := buildCreditCardReportDailySummaries(run.Snapshot.Groups)
	for i, row := range dailyRows {
		writeRow(f, sheet, dailyHeaderRow+1+i, []interface{}{
			row.Date,
			row.PaymentMethod,
			row.GroupCount,
			row.OrderCount,
			row.ChargeTotal,
			row.OrderTotal,
			row.Diff,
			row.IssueGroupCount,
		})
	}
	if len(dailyRows) > 0 {
		_ = f.SetCellStyle(sheet, fmt.Sprintf("E%d", dailyHeaderRow+1), fmt.Sprintf("G%d", dailyHeaderRow+len(dailyRows)), moneyStyle)
	}
	_ = f.SetColWidth(sheet, "C", "H", 18)
}

func writeReportIssues(f *excelize.File, sheet string, run *models.CreditCardReportRun, headerStyle, moneyStyle int) {
	headers := []string{"ลำดับยอดรูดบัตร", "วันที่จากอีเมล", "เวลาจากอีเมล", "ช่องทาง", "ยอดรูดบัตร", "ยอดรวมบิลใน BillFlow", "ต่างจากยอดรูด", "จำนวนคำสั่งซื้อ", "หมายเหตุ"}
	writeHeader(f, sheet, headers, headerStyle)
	row := 2
	for i, group := range run.Snapshot.Groups {
		if len(group.Issues) == 0 {
			continue
		}
		chargeDate, chargeTime := creditCardReportExportDateTime(group.ChargeTime)
		writeRow(f, sheet, row, []interface{}{
			i + 1,
			chargeDate,
			chargeTime,
			group.SourceLabel,
			nullableFloat(group.ChargeAmount),
			group.OrderTotal,
			nullableFloat(group.Diff),
			group.OrderCount,
			issueMessages(group.Issues),
		})
		row++
	}
	_ = f.SetColWidth(sheet, "A", "I", 18)
	_ = f.SetColWidth(sheet, "I", "I", 48)
	_ = f.SetCellStyle(sheet, "E2", fmt.Sprintf("G%d", max(row-1, 2)), moneyStyle)
}

func writeHeader(f *excelize.File, sheet string, headers []string, style int) {
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue(sheet, cell, h)
	}
	last, _ := excelize.CoordinatesToCellName(len(headers), 1)
	_ = f.SetCellStyle(sheet, "A1", last, style)
}

func writeRow(f *excelize.File, sheet string, row int, values []interface{}) {
	for i, value := range values {
		cell, _ := excelize.CoordinatesToCellName(i+1, row)
		_ = f.SetCellValue(sheet, cell, value)
	}
}

func setReportWidths(f *excelize.File, sheet string) {
	widths := map[string]float64{
		"A": 12, "B": 14, "C": 12, "D": 12, "E": 18, "F": 14, "G": 14, "H": 12,
		"I": 16, "J": 24, "K": 28, "L": 14, "M": 14, "N": 16, "O": 36,
	}
	for col, width := range widths {
		_ = f.SetColWidth(sheet, col, col, width)
	}
}

func issueMessages(issues []models.CreditCardReportIssue) string {
	if len(issues) == 0 {
		return ""
	}
	out := make([]string, 0, len(issues))
	for _, issue := range issues {
		out = append(out, issue.Message)
	}
	return strings.Join(out, "; ")
}

type creditCardReportDailySummary struct {
	Date            string
	sortDate        string
	PaymentMethod   string
	GroupCount      int
	OrderCount      int
	ChargeTotal     float64
	OrderTotal      float64
	Diff            float64
	IssueGroupCount int
}

func buildCreditCardReportDailySummaries(groups []models.CreditCardReportGroup) []creditCardReportDailySummary {
	type key struct {
		date   string
		method string
	}
	byKey := map[key]creditCardReportDailySummary{}
	for _, group := range groups {
		dateKey := creditCardReportGroupDateKey(group)
		date := creditCardReportDisplayDate(dateKey)
		method := strings.Join(group.PaymentMethods, ", ")
		if strings.TrimSpace(method) == "" {
			method = "-"
		}
		k := key{date: dateKey, method: method}
		row := byKey[k]
		row.Date = date
		row.sortDate = dateKey
		row.PaymentMethod = method
		row.GroupCount++
		row.OrderCount += group.OrderCount
		if group.ChargeAmount != nil {
			row.ChargeTotal = roundCreditCardReportAmount(row.ChargeTotal + *group.ChargeAmount)
		}
		row.OrderTotal = roundCreditCardReportAmount(row.OrderTotal + group.OrderTotal)
		row.Diff = roundCreditCardReportAmount(row.ChargeTotal - row.OrderTotal)
		if len(group.Issues) > 0 {
			row.IssueGroupCount++
		}
		byKey[k] = row
	}
	out := make([]creditCardReportDailySummary, 0, len(byKey))
	for _, row := range byKey {
		out = append(out, row)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].sortDate != out[j].sortDate {
			return out[i].sortDate < out[j].sortDate
		}
		return out[i].PaymentMethod < out[j].PaymentMethod
	})
	return out
}

func creditCardReportGroupDateKey(group models.CreditCardReportGroup) string {
	if v := strings.TrimSpace(group.ChargeDate); v != "" {
		return v
	}
	if v := strings.TrimSpace(group.ChargeTime); len(v) >= 10 {
		return v[:10]
	}
	return "-"
}

func creditCardReportExportDateTime(value string) (string, string) {
	t, ok := parseCreditCardReportExportTime(value)
	if !ok {
		if len(strings.TrimSpace(value)) >= 10 {
			date := creditCardReportDisplayDate(strings.TrimSpace(value)[:10])
			return date, ""
		}
		return "", ""
	}
	t = t.In(creditCardReportBangkokLocation())
	return t.Format("02/01/2006"), t.Format("15:04:05")
}

func parseCreditCardReportExportTime(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
	loc := creditCardReportBangkokLocation()
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05-07",
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		"2006-01-02",
	}
	for _, layout := range layouts {
		var (
			t   time.Time
			err error
		)
		if layout == "2006-01-02 15:04:05" || layout == "2006-01-02 15:04" || layout == "2006-01-02" {
			t, err = time.ParseInLocation(layout, value, loc)
		} else {
			t, err = time.Parse(layout, value)
		}
		if err == nil {
			return t.In(loc), true
		}
	}
	return time.Time{}, false
}

func creditCardReportDisplayDate(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "-" {
		return "-"
	}
	if t, err := time.ParseInLocation("2006-01-02", value, creditCardReportBangkokLocation()); err == nil {
		return t.Format("02/01/2006")
	}
	if t, ok := parseCreditCardReportExportTime(value); ok {
		return t.In(creditCardReportBangkokLocation()).Format("02/01/2006")
	}
	return value
}

func creditCardReportBangkokLocation() *time.Location {
	return creditCardReportExportLocation
}

func mustCreditCardReportBangkokLocation() *time.Location {
	loc, err := time.LoadLocation("Asia/Bangkok")
	if err != nil {
		return time.FixedZone("Asia/Bangkok", 7*60*60)
	}
	return loc
}

func creditCardReportDisplayOrderID(value string) string {
	return strings.TrimLeft(strings.TrimSpace(value), "#")
}

func roundCreditCardReportAmount(v float64) float64 {
	return math.Round(v*100) / 100
}

func nullableFloat(v *float64) interface{} {
	if v == nil {
		return ""
	}
	return *v
}

func emptyDash(v string) string {
	v = strings.TrimSpace(v)
	if v == "" || v == "all" {
		return "-"
	}
	return v
}

func creditCardReportFilename(run *models.CreditCardReportRun) string {
	method := strings.TrimSpace(run.Filters.PaymentMethod)
	if method == "" {
		method = "all"
	}
	name := fmt.Sprintf("credit-card-report_%s_%s_%s.xlsx", method, run.Filters.DateFrom, run.Filters.DateTo)
	name = strings.ReplaceAll(name, "/", "-")
	name = strings.ReplaceAll(name, " ", "_")
	return name
}
