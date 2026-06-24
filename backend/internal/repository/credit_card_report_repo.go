package repository

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"billflow/internal/models"
	"github.com/lib/pq"
)

const (
	creditCardReportMaxGroups              = 500
	creditCardReportMaxOrders              = 5000
	creditCardReportSmallDiffThreshold     = 2.0
	creditCardReportMaxDiagnosticGroups    = 80
	creditCardReportMaxDiagnosticFileBytes = 512 * 1024
)

var bangkokLocation = mustBangkokLocation()
var creditCardShopeeOrderIDPattern = regexp.MustCompile(`#([0-9A-Za-z]{8,})`)

type CreditCardReportRepo struct {
	db           *sql.DB
	artifactRoot string
}

func NewCreditCardReportRepo(db *sql.DB) *CreditCardReportRepo {
	return &CreditCardReportRepo{db: db}
}

func (r *CreditCardReportRepo) SetArtifactRoot(root string) {
	if r == nil {
		return
	}
	r.artifactRoot = strings.TrimSpace(root)
}

type creditCardReportRow struct {
	BillID                      string
	Source                      string
	Status                      string
	SMLDocNo                    string
	PrintPaymentMethod          string
	EffectivePrintPaymentMethod string
	OrderID                     string
	SellerName                  string
	DocDate                     string
	EmailDate                   string
	LazadaConfirmedAt           string
	EmailMessageID              string
	LazadaGroupKey              string
	ShopeeChargeAmount          *float64
	LazadaPaidAmount            *float64
	OrderTotal                  float64
	DocRef                      string
	CreatedAt                   time.Time
}

func (r *CreditCardReportRepo) Preview(f models.CreditCardReportFilter) (*models.CreditCardReportPreview, error) {
	f = normalizeCreditCardReportFilter(f)
	rows, err := r.fetchCreditCardReportRows(f)
	if err != nil {
		return nil, err
	}
	groups := buildCreditCardReportGroups(rows, f)
	groupCount := len(groups)
	orderCount := 0
	for _, group := range groups {
		orderCount += len(group.Orders)
	}
	truncated := groupCount > creditCardReportMaxGroups || orderCount > creditCardReportMaxOrders
	if truncated {
		limited := make([]models.CreditCardReportGroup, 0, min(groupCount, creditCardReportMaxGroups))
		orders := 0
		for _, group := range groups {
			if len(limited) >= creditCardReportMaxGroups || orders+len(group.Orders) > creditCardReportMaxOrders {
				break
			}
			limited = append(limited, group)
			orders += len(group.Orders)
		}
		groups = limited
	}
	if err := r.attachCreditCardReportArtifacts(groups); err != nil {
		return nil, err
	}
	if err := r.diagnoseCreditCardReportGroups(groups); err != nil {
		return nil, err
	}
	evaluateCreditCardReportPrintReadiness(groups)
	return &models.CreditCardReportPreview{
		Filters:     f,
		Groups:      groups,
		Summary:     summarizeCreditCardReportGroups(groups),
		Limit:       creditCardReportMaxGroups,
		Truncated:   truncated,
		GeneratedAt: time.Now(),
	}, nil
}

func (r *CreditCardReportRepo) CreateRun(f models.CreditCardReportFilter, reportName string, selectedGroupIDs []string, userID, userEmail string) (*models.CreditCardReportRun, error) {
	if len(selectedGroupIDs) == 0 {
		return nil, fmt.Errorf("กรุณาเลือกรายการอย่างน้อย 1 ยอดรูดบัตร")
	}
	preview, err := r.Preview(f)
	if err != nil {
		return nil, err
	}
	selected := map[string]bool{}
	for _, id := range selectedGroupIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			selected[id] = true
		}
	}
	if len(selected) == 0 {
		return nil, fmt.Errorf("กรุณาเลือกรายการอย่างน้อย 1 ยอดรูดบัตร")
	}
	allowed := map[string]models.CreditCardReportGroup{}
	for _, group := range preview.Groups {
		allowed[group.GroupID] = group
	}
	outGroups := make([]models.CreditCardReportGroup, 0, len(selected))
	for _, id := range selectedGroupIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		group, ok := allowed[id]
		if !ok {
			return nil, fmt.Errorf("ยอดรูดบัตร %s ไม่อยู่ในผล preview ปัจจุบัน กรุณาโหลดข้อมูลใหม่", id)
		}
		outGroups = append(outGroups, group)
	}
	snapshot := *preview
	snapshot.Groups = outGroups
	snapshot.SelectedGroup = normalizeSelectedGroupIDs(selectedGroupIDs)
	snapshot.Summary = summarizeCreditCardReportGroups(outGroups)
	snapshot.Summary.SelectedCount = len(outGroups)

	snapshotJSON, err := json.Marshal(snapshot)
	if err != nil {
		return nil, err
	}
	summaryJSON, err := json.Marshal(snapshot.Summary)
	if err != nil {
		return nil, err
	}
	run := &models.CreditCardReportRun{}
	row := r.db.QueryRow(`
		INSERT INTO credit_card_report_runs
		  (report_name, date_from, date_to, payment_method, source, include_incomplete,
		   selected_group_ids, snapshot, summary, created_by, created_by_email)
		VALUES
		  ($1, $2::date, $3::date, $4, $5, $6, $7, $8, $9, NULLIF($10, '')::uuid, $11)
		RETURNING id::text, report_name, date_from::text, date_to::text, payment_method, source,
		          include_incomplete, selected_group_ids, snapshot, summary,
		          COALESCE(created_by::text, ''), created_by_email, exported_at, printed_at,
		          print_summary, created_at, updated_at`,
		strings.TrimSpace(reportName),
		snapshot.Filters.DateFrom,
		snapshot.Filters.DateTo,
		snapshot.Filters.PaymentMethod,
		snapshot.Filters.Source,
		snapshot.Filters.IncludeIncomplete,
		pq.Array(snapshot.SelectedGroup),
		snapshotJSON,
		summaryJSON,
		userID,
		strings.TrimSpace(userEmail),
	)
	return scanCreditCardReportRun(row, run)
}

func (r *CreditCardReportRepo) FindRun(id string) (*models.CreditCardReportRun, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("run id is required")
	}
	run := &models.CreditCardReportRun{}
	row := r.db.QueryRow(`
		SELECT id::text, report_name, date_from::text, date_to::text, payment_method, source,
		       include_incomplete, selected_group_ids, snapshot, summary,
		       COALESCE(created_by::text, ''), created_by_email, exported_at, printed_at,
		       print_summary, created_at, updated_at
		  FROM credit_card_report_runs
		 WHERE id = $1`, id)
	run, err := scanCreditCardReportRun(row, run)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return run, err
}

func (r *CreditCardReportRepo) ListRuns(limit int) ([]models.CreditCardReportRun, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	rows, err := r.db.Query(`
		SELECT id::text, report_name, date_from::text, date_to::text, payment_method, source,
		       include_incomplete, selected_group_ids, snapshot, summary,
		       COALESCE(created_by::text, ''), created_by_email, exported_at, printed_at,
		       print_summary, created_at, updated_at
		  FROM credit_card_report_runs
		 ORDER BY created_at DESC, id DESC
		 LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.CreditCardReportRun{}
	for rows.Next() {
		run, err := scanCreditCardReportRun(rows, &models.CreditCardReportRun{})
		if err != nil {
			return nil, err
		}
		out = append(out, *run)
	}
	return out, rows.Err()
}

func (r *CreditCardReportRepo) MarkExported(id, userID string) error {
	_, err := r.db.Exec(`
		UPDATE credit_card_report_runs
		   SET exported_at = now(), exported_by = NULLIF($2, '')::uuid, updated_at = now()
		 WHERE id = $1`, id, userID)
	return err
}

func (r *CreditCardReportRepo) MarkPrinted(id, userID string, summary map[string]interface{}) error {
	raw, _ := json.Marshal(summary)
	_, err := r.db.Exec(`
		UPDATE credit_card_report_runs
		   SET printed_at = now(), printed_by = NULLIF($2, '')::uuid,
		       print_summary = $3, updated_at = now()
		 WHERE id = $1`, id, userID, raw)
	return err
}

func (r *CreditCardReportRepo) fetchCreditCardReportRows(f models.CreditCardReportFilter) ([]creditCardReportRow, error) {
	args := []interface{}{}
	sourceWhere := ""
	dateWhere := ""
	reportDateExpr := `COALESCE(NULLIF(LEFT(COALESCE(NULLIF(b.raw_data->>'lazada_confirmed_at', ''), NULLIF(b.raw_data->>'email_date', ''), NULLIF(b.raw_data->>'doc_date', '')), 10), ''), to_char(b.created_at AT TIME ZONE 'Asia/Bangkok', 'YYYY-MM-DD'))`
	source := strings.TrimSpace(f.Source)
	if source == "shopee_shipped" || source == "lazada_email" {
		sourceWhere = " AND b.source = $1"
		args = append(args, source)
	}
	if strings.TrimSpace(f.DateFrom) != "" {
		args = append(args, strings.TrimSpace(f.DateFrom))
		dateWhere += fmt.Sprintf(" AND %s >= $%d", reportDateExpr, len(args))
	}
	if strings.TrimSpace(f.DateTo) != "" {
		args = append(args, strings.TrimSpace(f.DateTo))
		dateWhere += fmt.Sprintf(" AND %s <= $%d", reportDateExpr, len(args))
	}
	rows, err := r.db.Query(`
		WITH item_totals AS (
			SELECT bill_id,
			       ROUND(SUM((qty * COALESCE(price, 0)) - COALESCE(discount_amount, 0))::numeric, 2) AS order_total
			  FROM bill_items
			 GROUP BY bill_id
		)
		SELECT b.id::text,
		       b.source,
		       b.status,
		       COALESCE(b.sml_doc_no, '') AS sml_doc_no,
		       COALESCE(b.print_payment_method, '') AS print_payment_method,
		       `+effectivePrintPaymentMethodExpr("b")+` AS effective_print_payment_method,
		       COALESCE(NULLIF(b.raw_data->>'order_id', ''), NULLIF(b.raw_data->>'lazada_order_id', ''), '') AS order_id,
		       COALESCE(NULLIF(b.raw_data->>'seller_name', ''), NULLIF(b.sml_payload->>'remark', ''), '') AS seller_name,
		       COALESCE(b.raw_data->>'doc_date', '') AS doc_date,
		       COALESCE(b.raw_data->>'email_date', '') AS email_date,
		       COALESCE(b.raw_data->>'lazada_confirmed_at', '') AS lazada_confirmed_at,
		       COALESCE(NULLIF(b.raw_data->>'email_message_id', ''), NULLIF(b.raw_data->>'message_id', ''), '') AS email_message_id,
		       COALESCE(b.raw_data->>'lazada_charge_group_key', '') AS lazada_group_key,
		       NULLIF(regexp_replace(COALESCE(b.raw_data#>>'{payment_summary,doc_ref_amount}', b.raw_data#>>'{payment_summary,payment_paid_amount}', ''), '[^0-9\.-]', '', 'g'), '')::double precision AS shopee_charge_amount,
		       NULLIF(regexp_replace(COALESCE(b.raw_data->>'paid_total_amount', ''), '[^0-9\.-]', '', 'g'), '')::double precision AS lazada_paid_amount,
		       COALESCE(it.order_total, 0)::double precision AS order_total,
		       COALESCE(b.sml_payload->>'doc_ref', '') AS doc_ref,
		       b.created_at
		  FROM bills b
		  LEFT JOIN item_totals it ON it.bill_id = b.id
		 WHERE b.bill_type = 'purchase'
		   AND b.source IN ('shopee_shipped', 'lazada_email')
		   AND b.archived_at IS NULL`+sourceWhere+`
		   `+dateWhere+`
		 ORDER BY b.created_at ASC, b.id ASC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []creditCardReportRow{}
	for rows.Next() {
		var row creditCardReportRow
		var shopee, lazada sql.NullFloat64
		if err := rows.Scan(
			&row.BillID, &row.Source, &row.Status, &row.SMLDocNo,
			&row.PrintPaymentMethod, &row.EffectivePrintPaymentMethod,
			&row.OrderID, &row.SellerName, &row.DocDate, &row.EmailDate,
			&row.LazadaConfirmedAt, &row.EmailMessageID, &row.LazadaGroupKey,
			&shopee, &lazada, &row.OrderTotal, &row.DocRef, &row.CreatedAt,
		); err != nil {
			return nil, err
		}
		if shopee.Valid {
			v := shopee.Float64
			row.ShopeeChargeAmount = &v
		}
		if lazada.Valid {
			v := lazada.Float64
			row.LazadaPaidAmount = &v
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func buildCreditCardReportGroups(rows []creditCardReportRow, f models.CreditCardReportFilter) []models.CreditCardReportGroup {
	groupRows := map[string][]creditCardReportRow{}
	for _, row := range rows {
		groupID := creditCardReportGroupID(row)
		if groupID == "" {
			if !f.IncludeIncomplete {
				continue
			}
			groupID = row.Source + ":missing:" + row.BillID
		}
		if row.Source == "shopee_shipped" && row.ShopeeChargeAmount == nil && !f.IncludeIncomplete {
			continue
		}
		if row.Source == "lazada_email" && strings.TrimSpace(row.LazadaGroupKey) == "" && !f.IncludeIncomplete {
			continue
		}
		groupRows[groupID] = append(groupRows[groupID], row)
	}
	groups := make([]models.CreditCardReportGroup, 0, len(groupRows))
	for groupID, rows := range groupRows {
		group := buildCreditCardReportGroup(groupID, rows)
		if !creditCardGroupInDateRange(group, f.DateFrom, f.DateTo) {
			continue
		}
		if !creditCardGroupMatchesPaymentMethod(group, f.PaymentMethod) {
			continue
		}
		if !f.IncludeIncomplete && group.ChargeAmount == nil {
			continue
		}
		groups = append(groups, group)
	}
	sort.SliceStable(groups, func(i, j int) bool {
		if !groups[i].SortTime.Equal(groups[j].SortTime) {
			return groups[i].SortTime.Before(groups[j].SortTime)
		}
		if groups[i].Source != groups[j].Source {
			return groups[i].Source < groups[j].Source
		}
		ai, aj := math.Inf(1), math.Inf(1)
		if groups[i].ChargeAmount != nil {
			ai = *groups[i].ChargeAmount
		}
		if groups[j].ChargeAmount != nil {
			aj = *groups[j].ChargeAmount
		}
		if ai != aj {
			return ai < aj
		}
		return groups[i].GroupID < groups[j].GroupID
	})
	return groups
}

func buildCreditCardReportGroup(groupID string, rows []creditCardReportRow) models.CreditCardReportGroup {
	source := ""
	if len(rows) > 0 {
		source = rows[0].Source
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].SMLDocNo != rows[j].SMLDocNo {
			return rows[i].SMLDocNo < rows[j].SMLDocNo
		}
		return rows[i].OrderID < rows[j].OrderID
	})
	methodSet := map[string]bool{}
	var charge *float64
	orderTotal := 0.0
	polCount := 0
	sentCount := 0
	var sortTime time.Time
	orders := make([]models.CreditCardReportOrder, 0, len(rows))
	for _, row := range rows {
		method := strings.TrimSpace(row.EffectivePrintPaymentMethod)
		if method == "" {
			method = strings.TrimSpace(row.PrintPaymentMethod)
		}
		if method != "" {
			methodSet[method] = true
		}
		if row.SMLDocNo != "" {
			polCount++
		}
		if row.Status == "sent" {
			sentCount++
		}
		if row.Source == "lazada_email" && row.LazadaPaidAmount != nil {
			v := 0.0
			if charge != nil {
				v = *charge
			}
			v += *row.LazadaPaidAmount
			charge = &v
		}
		if row.Source == "shopee_shipped" && row.ShopeeChargeAmount != nil && charge == nil {
			v := *row.ShopeeChargeAmount
			charge = &v
		}
		t := creditCardReportChargeTime(row)
		if sortTime.IsZero() || (!t.IsZero() && t.Before(sortTime)) {
			sortTime = t
		}
		orderTotal += row.OrderTotal
		orders = append(orders, models.CreditCardReportOrder{
			BillID:                      row.BillID,
			OrderID:                     row.OrderID,
			SellerName:                  row.SellerName,
			SMLDocNo:                    row.SMLDocNo,
			Status:                      row.Status,
			PrintPaymentMethod:          row.PrintPaymentMethod,
			EffectivePrintPaymentMethod: row.EffectivePrintPaymentMethod,
			OrderTotal:                  round2(row.OrderTotal),
			DocRef:                      row.DocRef,
			EmailMessageID:              row.EmailMessageID,
			CreatedAt:                   row.CreatedAt.Format(time.RFC3339),
		})
	}
	if sortTime.IsZero() && len(rows) > 0 {
		sortTime = rows[0].CreatedAt
	}
	methods := make([]string, 0, len(methodSet))
	for method := range methodSet {
		methods = append(methods, method)
	}
	sort.Strings(methods)
	orderTotal = round2(orderTotal)
	var diff *float64
	if charge != nil {
		v := round2(*charge - orderTotal)
		diff = &v
		c := round2(*charge)
		charge = &c
	}
	group := models.CreditCardReportGroup{
		GroupID:        groupID,
		Source:         source,
		SourceLabel:    creditCardReportSourceLabel(source),
		ChargeTime:     formatBangkokTime(sortTime),
		ChargeDate:     formatBangkokDate(sortTime),
		SortTime:       sortTime,
		PaymentMethods: methods,
		ChargeAmount:   charge,
		OrderTotal:     orderTotal,
		Diff:           diff,
		OrderCount:     len(orders),
		POLCount:       polCount,
		SentCount:      sentCount,
		Orders:         orders,
	}
	group.Issues = creditCardReportIssues(group, rows)
	diagnoseCreditCardReportGroupBase(&group)
	return group
}

type creditCardDiagnosticArtifact struct {
	MessageID   string
	BillID      string
	Kind        string
	SizeBytes   int64
	StoragePath string
	Subject     string
	CreatedAt   time.Time
}

func (r *CreditCardReportRepo) diagnoseCreditCardReportGroups(groups []models.CreditCardReportGroup) error {
	for i := range groups {
		diagnoseCreditCardReportGroupBase(&groups[i])
	}
	if r == nil || strings.TrimSpace(r.artifactRoot) == "" {
		return nil
	}
	messageIDs := []string{}
	seen := map[string]bool{}
	candidateCount := 0
	for i := range groups {
		group := groups[i]
		if group.Source != "shopee_shipped" || !hasCreditCardReportIssue(group, "amount_mismatch") {
			continue
		}
		if group.Diff == nil || math.Abs(*group.Diff) <= creditCardReportSmallDiffThreshold {
			continue
		}
		if candidateCount >= creditCardReportMaxDiagnosticGroups {
			continue
		}
		messageID := creditCardReportGroupMessageID(group)
		if messageID == "" || seen[messageID] {
			continue
		}
		seen[messageID] = true
		messageIDs = append(messageIDs, messageID)
		candidateCount++
	}
	if len(messageIDs) == 0 {
		return nil
	}
	artifacts, err := r.creditCardDiagnosticArtifacts(messageIDs)
	if err != nil {
		return err
	}
	for i := range groups {
		group := &groups[i]
		if group.Source != "shopee_shipped" || !hasCreditCardReportIssue(*group, "amount_mismatch") {
			continue
		}
		if group.Diff == nil || math.Abs(*group.Diff) <= creditCardReportSmallDiffThreshold {
			continue
		}
		messageID := creditCardReportGroupMessageID(*group)
		if messageID == "" {
			continue
		}
		detected := r.detectShopeeOrdersFromDiagnosticArtifacts(artifacts[messageID])
		if detected <= group.OrderCount {
			continue
		}
		group.DiagnosisCategory = "repair_candidate"
		group.DiagnosisTitle = "คำสั่งซื้อตกหล่นจากอีเมล"
		group.DetectedEmailOrderCount = detected
		group.ActiveBillOrderCount = group.OrderCount
		group.EstimatedMissingOrderCount = detected - group.OrderCount
		group.RepairBillID = creditCardReportFirstOrderBillID(*group)
		group.DiagnosisDetail = fmt.Sprintf("อีเมลต้นฉบับมี %d คำสั่งซื้อ แต่ BillFlow มี %d ใบในยอดรูดนี้", detected, group.OrderCount)
		group.RecommendedAction = "กดตรวจ/ซ่อมจากอีเมลต้นฉบับ แล้วกลับมากด Preview ใหม่"
	}
	return nil
}

func diagnoseCreditCardReportGroupBase(group *models.CreditCardReportGroup) {
	if group == nil {
		return
	}
	group.ActiveBillOrderCount = group.OrderCount
	if group.DiagnosisCategory == "repair_candidate" {
		return
	}
	if group.ChargeAmount == nil {
		group.DiagnosisCategory = "incomplete_only"
		group.DiagnosisTitle = "ข้อมูลยอดรูดไม่ครบ"
		group.DiagnosisDetail = "กลุ่มนี้ยังไม่มียอดรูดบัตรจากอีเมล จึงยังเทียบ statement ไม่ได้"
		group.RecommendedAction = "ตรวจอีเมลต้นฉบับหรือข้อมูลยอดรูดก่อน export"
		return
	}
	if group.Diff != nil && math.Abs(*group.Diff) > 0.01 {
		if math.Abs(*group.Diff) <= creditCardReportSmallDiffThreshold {
			group.DiagnosisCategory = "small_diff"
			group.DiagnosisTitle = "ยอดต่างเล็กน้อย"
			group.DiagnosisDetail = fmt.Sprintf("ยอดต่าง %.2f บาท ควรตรวจส่วนลด ค่าส่ง หรือ rounding จากข้อมูลเก่า", *group.Diff)
			group.RecommendedAction = "ตรวจรายการย่อยในกลุ่มนี้ก่อนส่งต่อบัญชี"
			return
		}
		group.DiagnosisCategory = "amount_mismatch"
		group.DiagnosisTitle = "ยอดรวมบิลไม่ตรงกับยอดรูด"
		group.DiagnosisDetail = "ยอดรวมบิลใน BillFlow ยังไม่เท่ากับยอดรูดบัตรจากอีเมล"
		group.RecommendedAction = "ตรวจว่ามีคำสั่งซื้อตกหล่น หรือยอดสินค้า/ส่วนลด/ค่าส่งจากข้อมูลเก่าผิดหรือไม่"
		return
	}
	if len(group.Issues) > 0 {
		group.DiagnosisCategory = "incomplete_only"
		group.DiagnosisTitle = "ข้อมูลยังไม่ครบแต่ยอดตรง"
		group.DiagnosisDetail = issueMessagesForDiagnosis(group.Issues)
		group.RecommendedAction = "เติมข้อมูลที่ระบบเตือน เช่น POL หรือวิธีชำระเงิน"
		return
	}
	group.DiagnosisCategory = "ok"
	group.DiagnosisTitle = "ยอดตรง"
	group.DiagnosisDetail = "ยอดรูดบัตรตรงกับยอดรวมบิลใน BillFlow"
	group.RecommendedAction = ""
}

func (r *CreditCardReportRepo) creditCardDiagnosticArtifacts(messageIDs []string) (map[string][]creditCardDiagnosticArtifact, error) {
	out := map[string][]creditCardDiagnosticArtifact{}
	rows, err := r.db.Query(`
		SELECT message_id, bill_id, kind, size_bytes, storage_path, subject, created_at
		  FROM (
		    SELECT COALESCE(NULLIF(ba.source_meta->>'message_id', ''), NULLIF(b.raw_data->>'email_message_id', ''), NULLIF(b.raw_data->>'message_id', '')) AS message_id,
		           b.id::text AS bill_id,
		           ba.kind,
		           ba.size_bytes,
		           ba.storage_path,
		           COALESCE(ba.source_meta->>'subject', '') AS subject,
		           ba.created_at
		      FROM bills b
		      JOIN bill_artifacts ba ON ba.bill_id = b.id
		     WHERE COALESCE(NULLIF(ba.source_meta->>'message_id', ''), NULLIF(b.raw_data->>'email_message_id', ''), NULLIF(b.raw_data->>'message_id', '')) = ANY($1)
		       AND ba.kind IN ('email_html', 'email_text')
		       AND ba.size_bytes <= $2
		  ) x
		 WHERE message_id <> ''
		 ORDER BY message_id, created_at ASC`,
		pq.Array(messageIDs),
		creditCardReportMaxDiagnosticFileBytes,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var a creditCardDiagnosticArtifact
		if err := rows.Scan(&a.MessageID, &a.BillID, &a.Kind, &a.SizeBytes, &a.StoragePath, &a.Subject, &a.CreatedAt); err != nil {
			return nil, err
		}
		out[a.MessageID] = append(out[a.MessageID], a)
	}
	return out, rows.Err()
}

func (r *CreditCardReportRepo) detectShopeeOrdersFromDiagnosticArtifacts(artifacts []creditCardDiagnosticArtifact) int {
	best := 0
	for _, a := range artifacts {
		if !creditCardReportIsShopeePaymentSubject(a.Subject) && hasAnyPaymentArtifact(artifacts) {
			continue
		}
		data, ok := r.readCreditCardDiagnosticArtifact(a.StoragePath)
		if !ok {
			continue
		}
		count := len(creditCardReportUniqueShopeeOrderIDs(string(data)))
		if count > best {
			best = count
		}
	}
	return best
}

func hasAnyPaymentArtifact(artifacts []creditCardDiagnosticArtifact) bool {
	for _, a := range artifacts {
		if creditCardReportIsShopeePaymentSubject(a.Subject) {
			return true
		}
	}
	return false
}

func (r *CreditCardReportRepo) readCreditCardDiagnosticArtifact(storagePath string) ([]byte, bool) {
	root := strings.TrimSpace(r.artifactRoot)
	storagePath = strings.TrimSpace(storagePath)
	if root == "" || storagePath == "" || filepath.IsAbs(storagePath) {
		return nil, false
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return nil, false
	}
	abs := filepath.Join(rootAbs, filepath.Clean(storagePath))
	rel, err := filepath.Rel(rootAbs, abs)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return nil, false
	}
	info, err := os.Stat(abs)
	if err != nil || info.IsDir() || info.Size() > creditCardReportMaxDiagnosticFileBytes {
		return nil, false
	}
	data, err := os.ReadFile(abs)
	if err != nil || len(data) == 0 || len(data) > creditCardReportMaxDiagnosticFileBytes {
		return nil, false
	}
	return data, true
}

func creditCardReportIsShopeePaymentSubject(subject string) bool {
	subject = strings.TrimSpace(subject)
	return strings.Contains(subject, "ยืนยันการชำระเงิน") && strings.Contains(subject, "คำสั่งซื้อ")
}

func creditCardReportUniqueShopeeOrderIDs(text string) []string {
	matches := creditCardShopeeOrderIDPattern.FindAllStringSubmatch(text, -1)
	out := []string{}
	seen := map[string]bool{}
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		id := strings.ToUpper(strings.TrimLeft(strings.TrimSpace(match[1]), "#"))
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

func creditCardReportGroupMessageID(group models.CreditCardReportGroup) string {
	for _, order := range group.Orders {
		if strings.TrimSpace(order.EmailMessageID) != "" {
			return strings.TrimSpace(order.EmailMessageID)
		}
	}
	return ""
}

func creditCardReportFirstOrderBillID(group models.CreditCardReportGroup) string {
	for _, order := range group.Orders {
		if strings.TrimSpace(order.BillID) != "" {
			return strings.TrimSpace(order.BillID)
		}
	}
	return ""
}

func issueMessagesForDiagnosis(issues []models.CreditCardReportIssue) string {
	if len(issues) == 0 {
		return ""
	}
	out := make([]string, 0, len(issues))
	for _, issue := range issues {
		if strings.TrimSpace(issue.Message) != "" {
			out = append(out, issue.Message)
		}
	}
	return strings.Join(out, "; ")
}

func (r *CreditCardReportRepo) attachCreditCardReportArtifacts(groups []models.CreditCardReportGroup) error {
	messageIDs := []string{}
	seen := map[string]bool{}
	for _, group := range groups {
		for _, order := range group.Orders {
			messageID := strings.TrimSpace(order.EmailMessageID)
			if messageID != "" && !seen[messageID] {
				seen[messageID] = true
				messageIDs = append(messageIDs, messageID)
			}
		}
	}
	if len(messageIDs) == 0 {
		return nil
	}
	rows, err := r.db.Query(`
		SELECT DISTINCT ON (message_id)
		       message_id, bill_id, artifact_id, filename
		  FROM (
		    SELECT COALESCE(NULLIF(ba.source_meta->>'message_id', ''), NULLIF(b.raw_data->>'email_message_id', ''), NULLIF(b.raw_data->>'message_id', '')) AS message_id,
		           b.id::text AS bill_id,
		           ba.id::text AS artifact_id,
		           ba.filename,
		           b.created_at,
		           ba.created_at AS artifact_created_at
		      FROM bills b
		      JOIN bill_artifacts ba ON ba.bill_id = b.id
		     WHERE COALESCE(NULLIF(ba.source_meta->>'message_id', ''), NULLIF(b.raw_data->>'email_message_id', ''), NULLIF(b.raw_data->>'message_id', '')) = ANY($1)
		       AND ba.kind IN ('email_html', 'email_text')
		  ) x
		 WHERE message_id <> ''
		 ORDER BY message_id, created_at ASC, artifact_created_at ASC`,
		pq.Array(messageIDs),
	)
	if err != nil {
		return err
	}
	defer rows.Close()
	type artifact struct {
		MessageID  string
		BillID     string
		ArtifactID string
		Filename   string
	}
	byMessage := map[string]artifact{}
	for rows.Next() {
		var a artifact
		if err := rows.Scan(&a.MessageID, &a.BillID, &a.ArtifactID, &a.Filename); err != nil {
			return err
		}
		byMessage[a.MessageID] = a
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for i := range groups {
		messageOrderContexts := map[string][]models.CreditCardReportPrintOrderContext{}
		for _, order := range groups[i].Orders {
			messageID := strings.TrimSpace(order.EmailMessageID)
			if messageID == "" {
				continue
			}
			paymentMethod := strings.TrimSpace(order.EffectivePrintPaymentMethod)
			if paymentMethod == "" {
				paymentMethod = strings.TrimSpace(order.PrintPaymentMethod)
			}
			messageOrderContexts[messageID] = append(messageOrderContexts[messageID], models.CreditCardReportPrintOrderContext{
				OrderID:       order.OrderID,
				SMLDocNo:      order.SMLDocNo,
				PartyName:     order.SellerName,
				PaymentMethod: paymentMethod,
			})
		}
		orderedMessages := make([]string, 0, len(messageOrderContexts))
		for messageID := range messageOrderContexts {
			orderedMessages = append(orderedMessages, messageID)
		}
		sort.Strings(orderedMessages)
		for _, messageID := range orderedMessages {
			a, ok := byMessage[messageID]
			if !ok {
				continue
			}
			groups[i].PrintArtifacts = append(groups[i].PrintArtifacts, models.CreditCardReportPrintArtifact{
				MessageID:  messageID,
				BillID:     a.BillID,
				ArtifactID: a.ArtifactID,
				Filename:   a.Filename,
				Orders:     messageOrderContexts[messageID],
			})
		}
		groups[i].PrintableCount = len(groups[i].PrintArtifacts)
	}
	return nil
}

func evaluateCreditCardReportPrintReadiness(groups []models.CreditCardReportGroup) {
	for i := range groups {
		group := &groups[i]
		if group.PrintableCount == 0 {
			group.PrintReady = false
			group.PrintBlockReason = "ไม่มีอีเมลต้นฉบับสำหรับพิมพ์"
			continue
		}
		if group.POLCount != group.OrderCount {
			group.PrintReady = false
			group.PrintBlockReason = "ยังไม่มีเลข POL ครบทุกคำสั่งซื้อ"
			continue
		}
		if hasCreditCardReportIssue(*group, "missing_payment_method") || hasCreditCardReportIssue(*group, "non_tt_payment_method") {
			group.PrintReady = false
			group.PrintBlockReason = "ยังไม่มีวิธีชำระเงิน TT ครบทุกคำสั่งซื้อ"
			continue
		}
		group.PrintReady = true
	}
}

func creditCardReportIssues(group models.CreditCardReportGroup, rows []creditCardReportRow) []models.CreditCardReportIssue {
	issues := []models.CreditCardReportIssue{}
	add := func(code, severity, message string) {
		issues = append(issues, models.CreditCardReportIssue{Code: code, Severity: severity, Message: message})
	}
	if group.Source == "lazada_email" {
		for _, row := range rows {
			if strings.TrimSpace(row.LazadaGroupKey) == "" {
				add("missing_group_key", "warn", "Lazada ยังไม่มี charge group key")
				break
			}
		}
	}
	if group.ChargeAmount == nil {
		add("missing_charge_amount", "warn", "ข้อมูลยอดรูดบัตรไม่ครบ")
	}
	if group.ChargeAmount != nil && group.Diff != nil && math.Abs(*group.Diff) > 0.01 {
		add("amount_mismatch", "warn", "ยอดรวมบิลใน BillFlow ไม่ตรงกับยอดรูดบัตร")
	}
	if group.POLCount != group.OrderCount {
		add("missing_pol", "warn", "ยังไม่มีเลข POL ครบทุกคำสั่งซื้อ")
	}
	missingMethod := false
	nonTTMethod := false
	for _, row := range rows {
		method := strings.TrimSpace(row.EffectivePrintPaymentMethod)
		if method == "" {
			method = strings.TrimSpace(row.PrintPaymentMethod)
		}
		if method == "" {
			missingMethod = true
		} else if !strings.HasPrefix(strings.ToUpper(method), "TT") {
			nonTTMethod = true
		}
	}
	if missingMethod {
		add("missing_payment_method", "warn", "บางคำสั่งซื้อยังไม่มีวิธีชำระเงิน")
	}
	if nonTTMethod {
		add("non_tt_payment_method", "warn", "บางคำสั่งซื้อไม่ใช่วิธีชำระเงิน TT")
	}
	if len(group.PaymentMethods) > 1 {
		add("mixed_payment_method", "info", "ยอดรูดนี้มีหลายวิธีชำระเงินในกลุ่มเดียว")
	}
	return issues
}

func summarizeCreditCardReportGroups(groups []models.CreditCardReportGroup) models.CreditCardReportSummary {
	var s models.CreditCardReportSummary
	s.GroupCount = len(groups)
	for _, group := range groups {
		s.OrderCount += group.OrderCount
		s.OrderTotal = round2(s.OrderTotal + group.OrderTotal)
		if group.ChargeAmount != nil {
			s.ChargeTotal = round2(s.ChargeTotal + *group.ChargeAmount)
		} else {
			s.MissingCharge++
		}
		if len(group.Issues) > 0 {
			s.IssueGroupCount++
		}
		category := strings.TrimSpace(group.DiagnosisCategory)
		if category == "" {
			category = creditCardReportDiagnosisCategory(group)
		}
		if category == "amount_mismatch" {
			s.AmountMismatchCount++
		}
		switch category {
		case "repair_candidate":
			s.RepairCandidateCount++
		case "incomplete_only":
			s.IncompleteOnlyCount++
		case "small_diff":
			s.SmallDiffCount++
		}
		if group.POLCount != group.OrderCount {
			s.MissingPOLCount += group.OrderCount - group.POLCount
		}
		if group.PrintReady {
			s.ReadyPrintGroups++
		}
	}
	return s
}

func creditCardReportDiagnosisCategory(group models.CreditCardReportGroup) string {
	if strings.TrimSpace(group.DiagnosisCategory) != "" {
		return strings.TrimSpace(group.DiagnosisCategory)
	}
	if group.ChargeAmount == nil {
		return "incomplete_only"
	}
	if group.Diff != nil && math.Abs(*group.Diff) > 0.01 {
		if math.Abs(*group.Diff) <= creditCardReportSmallDiffThreshold {
			return "small_diff"
		}
		return "amount_mismatch"
	}
	if len(group.Issues) > 0 {
		return "incomplete_only"
	}
	return "ok"
}

func normalizeCreditCardReportFilter(f models.CreditCardReportFilter) models.CreditCardReportFilter {
	f.DateFrom = strings.TrimSpace(f.DateFrom)
	f.DateTo = strings.TrimSpace(f.DateTo)
	f.PaymentMethod = strings.TrimSpace(f.PaymentMethod)
	f.Source = strings.TrimSpace(f.Source)
	if f.Source == "" {
		f.Source = "all"
	}
	return f
}

func creditCardReportGroupID(row creditCardReportRow) string {
	switch row.Source {
	case "shopee_shipped":
		messageID := strings.TrimSpace(row.EmailMessageID)
		if messageID == "" {
			return ""
		}
		return "shopee:" + messageID
	case "lazada_email":
		groupKey := strings.TrimSpace(row.LazadaGroupKey)
		if groupKey == "" {
			return ""
		}
		return "lazada:" + groupKey
	default:
		return ""
	}
}

func creditCardReportChargeTime(row creditCardReportRow) time.Time {
	candidates := []string{}
	if row.Source == "lazada_email" {
		candidates = append(candidates, row.LazadaConfirmedAt, row.EmailDate, row.DocDate)
	} else {
		candidates = append(candidates, row.EmailDate, row.DocDate)
	}
	for _, candidate := range candidates {
		if t, ok := parseReportTime(candidate); ok {
			return t
		}
	}
	return row.CreatedAt
}

func creditCardGroupInDateRange(group models.CreditCardReportGroup, from, to string) bool {
	if from == "" && to == "" {
		return true
	}
	date := group.SortTime.In(bangkokLocation)
	if from != "" {
		t, err := time.ParseInLocation("2006-01-02", from, bangkokLocation)
		if err == nil && date.Before(t) {
			return false
		}
	}
	if to != "" {
		t, err := time.ParseInLocation("2006-01-02", to, bangkokLocation)
		if err == nil && !date.Before(t.AddDate(0, 0, 1)) {
			return false
		}
	}
	return true
}

func creditCardGroupMatchesPaymentMethod(group models.CreditCardReportGroup, method string) bool {
	method = strings.TrimSpace(method)
	if method == "" || method == "all" {
		return true
	}
	for _, current := range group.PaymentMethods {
		if strings.EqualFold(strings.TrimSpace(current), method) {
			return true
		}
	}
	return false
}

func parseReportTime(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
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
			t, err = time.ParseInLocation(layout, value, bangkokLocation)
		} else {
			t, err = time.Parse(layout, value)
		}
		if err == nil {
			return t.In(bangkokLocation), true
		}
	}
	return time.Time{}, false
}

func formatBangkokDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.In(bangkokLocation).Format("2006-01-02")
}

func formatBangkokTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.In(bangkokLocation).Format(time.RFC3339)
}

func creditCardReportSourceLabel(source string) string {
	switch source {
	case "shopee_shipped":
		return "Shopee"
	case "lazada_email":
		return "Lazada"
	default:
		return source
	}
}

func hasCreditCardReportIssue(group models.CreditCardReportGroup, code string) bool {
	for _, issue := range group.Issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}

func normalizeSelectedGroupIDs(ids []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

func mustBangkokLocation() *time.Location {
	loc, err := time.LoadLocation("Asia/Bangkok")
	if err != nil {
		return time.FixedZone("Asia/Bangkok", 7*60*60)
	}
	return loc
}

type creditCardRunScanner interface {
	Scan(dest ...interface{}) error
}

func scanCreditCardReportRun(scanner creditCardRunScanner, run *models.CreditCardReportRun) (*models.CreditCardReportRun, error) {
	var (
		dateFrom, dateTo string
		selected         []string
		snapshotRaw      []byte
		summaryRaw       []byte
		printSummaryRaw  []byte
		exportedAt       sql.NullTime
		printedAt        sql.NullTime
	)
	err := scanner.Scan(
		&run.ID, &run.ReportName, &dateFrom, &dateTo, &run.Filters.PaymentMethod,
		&run.Filters.Source, &run.Filters.IncludeIncomplete, pq.Array(&selected),
		&snapshotRaw, &summaryRaw, &run.CreatedBy, &run.CreatedByEmail,
		&exportedAt, &printedAt, &printSummaryRaw, &run.CreatedAt, &run.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	run.Filters.DateFrom = dateFrom
	run.Filters.DateTo = dateTo
	run.SelectedGroupIDs = selected
	if err := json.Unmarshal(snapshotRaw, &run.Snapshot); err != nil {
		return nil, fmt.Errorf("decode report snapshot: %w", err)
	}
	if len(summaryRaw) > 0 {
		_ = json.Unmarshal(summaryRaw, &run.Summary)
	}
	if exportedAt.Valid {
		run.ExportedAt = &exportedAt.Time
	}
	if printedAt.Valid {
		run.PrintedAt = &printedAt.Time
	}
	run.PrintSummary = map[string]interface{}{}
	if len(printSummaryRaw) > 0 {
		_ = json.Unmarshal(printSummaryRaw, &run.PrintSummary)
	}
	return run, nil
}
