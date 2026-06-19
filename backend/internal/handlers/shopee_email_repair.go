package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
	"go.uber.org/zap"

	"billflow/internal/config"
	"billflow/internal/models"
	"billflow/internal/repository"
	"billflow/internal/services/ai"
	"billflow/internal/services/artifact"
	"billflow/internal/services/catalog"
	emailservice "billflow/internal/services/email"
)

type ShopeeEmailRepairPreview struct {
	BillID                 string   `json:"bill_id"`
	MessageID              string   `json:"message_id"`
	ArtifactID             string   `json:"artifact_id"`
	Subject                string   `json:"subject"`
	DetectedOrderCount     int      `json:"detected_order_count"`
	ExistingCount          int      `json:"existing_count"`
	MissingCount           int      `json:"missing_count"`
	DetectedOrderIDs       []string `json:"detected_order_ids"`
	ExistingOrderIDs       []string `json:"existing_order_ids"`
	MissingOrderIDs        []string `json:"missing_order_ids"`
	EmailTotal             float64  `json:"email_total"`
	HasStaleTombstones     bool     `json:"has_stale_tombstones"`
	StaleTombstoneOrderIDs []string `json:"stale_tombstone_order_ids,omitempty"`
	Warnings               []string `json:"warnings,omitempty"`
	CanRepair              bool     `json:"can_repair"`
}

type ShopeeEmailRepairJob struct {
	ID              string                   `json:"id"`
	BillID          string                   `json:"bill_id"`
	Source          string                   `json:"source"`
	MessageID       string                   `json:"message_id"`
	Status          string                   `json:"status"`
	Snapshot        ShopeeEmailRepairPreview `json:"snapshot"`
	Result          ShopeeEmailRepairResult  `json:"result"`
	Error           string                   `json:"error,omitempty"`
	CreatedByEmail  string                   `json:"created_by_email,omitempty"`
	StartedAt       *time.Time               `json:"started_at,omitempty"`
	FinishedAt      *time.Time               `json:"finished_at,omitempty"`
	CreatedAt       time.Time                `json:"created_at"`
	UpdatedAt       time.Time                `json:"updated_at"`
	CreatedCount    int                      `json:"created_count"`
	SkippedCount    int                      `json:"skipped_count"`
	MissingCount    int                      `json:"missing_count"`
	CreatedBillIDs  []string                 `json:"created_bill_ids,omitempty"`
	CreatedOrderIDs []string                 `json:"created_order_ids,omitempty"`
	MissingOrderIDs []string                 `json:"missing_order_ids,omitempty"`
}

type ShopeeEmailRepairResult struct {
	CreatedCount    int      `json:"created_count"`
	SkippedCount    int      `json:"skipped_count"`
	MissingCount    int      `json:"missing_count"`
	CreatedBillIDs  []string `json:"created_bill_ids,omitempty"`
	CreatedOrderIDs []string `json:"created_order_ids,omitempty"`
	MissingOrderIDs []string `json:"missing_order_ids,omitempty"`
	StaleCleared    []string `json:"stale_tombstone_order_ids,omitempty"`
	OutcomeKind     string   `json:"outcome_kind,omitempty"`
	OutcomeCode     string   `json:"outcome_code,omitempty"`
}

type createShopeeEmailRepairJobRequest struct {
	ExpectedOrderCount      int      `json:"expected_order_count"`
	ExpectedTotal           float64  `json:"expected_total"`
	ExpectedMissingOrderIDs []string `json:"expected_missing_order_ids"`
}

type shopeeRepairTarget struct {
	BillID    string
	Subject   string
	FromAddr  string
	MessageID string
	Raw       map[string]interface{}
}

type shopeeRepairEmailBody struct {
	Text       string
	HTML       string
	ArtifactID string
}

// GET /api/bills/:id/shopee-email-repair/preview
func (h *BillHandler) PreviewShopeeEmailRepair(c *gin.Context) {
	svc := h.newShopeeEmailRepairService()
	if svc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "email repair service not configured"})
		return
	}
	preview, err := svc.Preview(c.Param("id"))
	if err != nil {
		writeShopeeRepairError(c, err)
		return
	}
	h.auditShopeeEmailRepair(c, "shopee_email_repair_previewed", c.Param("id"), "info", map[string]interface{}{
		"message_id":           preview.MessageID,
		"detected_order_count": preview.DetectedOrderCount,
		"existing_count":       preview.ExistingCount,
		"missing_count":        preview.MissingCount,
		"missing_order_ids":    preview.MissingOrderIDs,
		"email_total":          preview.EmailTotal,
		"stale_tombstones":     preview.StaleTombstoneOrderIDs,
	})
	c.JSON(http.StatusOK, gin.H{"data": preview})
}

// POST /api/bills/:id/shopee-email-repair/jobs
func (h *BillHandler) CreateShopeeEmailRepairJob(c *gin.Context) {
	svc := h.newShopeeEmailRepairService()
	if svc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "email repair service not configured"})
		return
	}
	var req createShopeeEmailRepairJobRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	job, err := svc.CreateJob(c.Param("id"), req, c.GetString("user_id"), c.GetString("user_email"))
	if err != nil {
		writeShopeeRepairError(c, err)
		return
	}
	h.auditShopeeEmailRepair(c, "shopee_email_repair_started", c.Param("id"), "info", map[string]interface{}{
		"job_id":                 job.ID,
		"message_id":             job.MessageID,
		"expected_order_count":   req.ExpectedOrderCount,
		"expected_total":         req.ExpectedTotal,
		"expected_missing_ids":   normalizeShopeeOrderIDs(req.ExpectedMissingOrderIDs),
		"active_existing_status": job.Status,
	})
	c.JSON(http.StatusAccepted, gin.H{"data": job})
}

// GET /api/bills/:id/shopee-email-repair/jobs/:job_id
func (h *BillHandler) GetShopeeEmailRepairJob(c *gin.Context) {
	job, err := h.getShopeeEmailRepairJob(c.Param("job_id"), c.Param("id"))
	if err != nil {
		writeShopeeRepairError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": job})
}

func (h *BillHandler) newShopeeEmailRepairService() *ShopeeEmailRepairService {
	if h == nil || h.billRepo == nil || h.artifactSvc == nil || h.cfg == nil {
		return nil
	}
	return &ShopeeEmailRepairService{
		db:              h.billRepo.DB(),
		cfg:             h.cfg,
		billRepo:        h.billRepo,
		artifactSvc:     h.artifactSvc,
		auditRepo:       h.auditRepo,
		catalogRepo:     h.catalogRepo,
		channelDefaults: h.channelDefaults,
		logger:          h.log,
		handler:         h,
	}
}

type ShopeeEmailRepairService struct {
	db              *sql.DB
	cfg             *config.Config
	billRepo        *repository.BillRepo
	artifactSvc     *artifact.Service
	auditRepo       *repository.AuditLogRepo
	catalogRepo     *repository.SMLCatalogRepo
	channelDefaults *repository.ChannelDefaultRepo
	logger          *zap.Logger
	handler         *BillHandler
}

func (s *ShopeeEmailRepairService) Preview(billID string) (ShopeeEmailRepairPreview, error) {
	target, err := s.loadTarget(billID)
	if err != nil {
		return ShopeeEmailRepairPreview{}, err
	}
	body, err := s.loadEmailBody(target)
	if err != nil {
		return ShopeeEmailRepairPreview{}, err
	}
	return s.inspectTarget(target, body)
}

func (s *ShopeeEmailRepairService) CreateJob(billID string, req createShopeeEmailRepairJobRequest, userID, userEmail string) (*ShopeeEmailRepairJob, error) {
	preview, err := s.Preview(billID)
	if err != nil {
		return nil, err
	}
	if preview.MissingCount == 0 {
		return nil, badShopeeRepairRequest("อีเมลนี้มีบิลครบแล้ว")
	}
	if preview.EmailTotal <= 0 {
		return nil, badShopeeRepairRequest("ยังคำนวณยอดรวมจากอีเมลไม่ได้ กรุณาตรวจหลักฐานต้นฉบับก่อนซ่อม")
	}
	if req.ExpectedOrderCount != preview.DetectedOrderCount {
		return nil, badShopeeRepairRequest("ข้อมูลอีเมลเปลี่ยนไป กรุณากดตรวจใหม่ก่อนซ่อม")
	}
	if math.Abs(req.ExpectedTotal-preview.EmailTotal) > 0.01 {
		return nil, badShopeeRepairRequest("ยอดรวมอีเมลเปลี่ยนไป กรุณากดตรวจใหม่ก่อนซ่อม")
	}
	if !sameStringSet(normalizeShopeeOrderIDs(req.ExpectedMissingOrderIDs), preview.MissingOrderIDs) {
		return nil, badShopeeRepairRequest("รายการที่ตกหล่นเปลี่ยนไป กรุณากดตรวจใหม่ก่อนซ่อม")
	}

	job, active, err := s.createRepairJob(preview, userID, userEmail)
	if err != nil {
		return nil, err
	}
	if active {
		return job, nil
	}
	go s.runJob(job.ID, preview.BillID)
	return job, nil
}

func (s *ShopeeEmailRepairService) runJob(jobID, billID string) {
	start := time.Now()
	if err := s.markJobRunning(jobID); err != nil {
		s.logWarn("shopee_email_repair: mark running failed", zap.String("job_id", jobID), zap.Error(err))
		return
	}
	result, err := s.applyRepair(billID)
	if err != nil {
		msg := truncateRepairError(err.Error())
		_ = s.markJobFailed(jobID, msg)
		s.auditJob("shopee_email_repair_failed", billID, jobID, "error", map[string]interface{}{
			"error":       msg,
			"duration_ms": int(time.Since(start).Milliseconds()),
		})
		return
	}
	if err := s.markJobSucceeded(jobID, result); err != nil {
		s.logWarn("shopee_email_repair: mark succeeded failed", zap.String("job_id", jobID), zap.Error(err))
		return
	}
	s.auditJob("shopee_email_repair_completed", billID, jobID, "info", map[string]interface{}{
		"created_count":     result.CreatedCount,
		"skipped_count":     result.SkippedCount,
		"missing_count":     result.MissingCount,
		"created_bill_ids":  result.CreatedBillIDs,
		"created_order_ids": result.CreatedOrderIDs,
		"stale_tombstones":  result.StaleCleared,
		"duration_ms":       int(time.Since(start).Milliseconds()),
	})
}

func (s *ShopeeEmailRepairService) applyRepair(billID string) (ShopeeEmailRepairResult, error) {
	before, err := s.Preview(billID)
	if err != nil {
		return ShopeeEmailRepairResult{}, err
	}
	if before.MissingCount == 0 {
		return ShopeeEmailRepairResult{SkippedCount: before.ExistingCount}, nil
	}
	cleared, err := s.clearStaleTombstones(before.MessageID, before.MissingOrderIDs)
	if err != nil {
		return ShopeeEmailRepairResult{}, fmt.Errorf("clear stale processed keys: %w", err)
	}
	target, err := s.loadTarget(billID)
	if err != nil {
		return ShopeeEmailRepairResult{}, err
	}
	body, err := s.loadEmailBody(target)
	if err != nil {
		return ShopeeEmailRepairResult{}, err
	}
	emailHandler, err := s.newEmailHandler()
	if err != nil {
		return ShopeeEmailRepairResult{}, err
	}
	outcome, err := emailHandler.ProcessShopeeShippedEmailBody(target.Subject, target.FromAddr, body.Text, body.HTML, target.MessageID, mailSourceFromRepairRaw(target.Raw))
	if err != nil {
		var skip *emailservice.MessageSkipError
		if !errors.As(err, &skip) || skip.Code != "duplicate_or_empty" {
			return ShopeeEmailRepairResult{}, err
		}
	}
	after, err := s.Preview(billID)
	if err != nil {
		return ShopeeEmailRepairResult{}, err
	}
	if after.MissingCount > 0 {
		return ShopeeEmailRepairResult{}, fmt.Errorf("repair incomplete; missing: %s", strings.Join(after.MissingOrderIDs, ", "))
	}
	createdBills, err := s.createdBillsForOrders(before.MissingOrderIDs)
	if err != nil {
		return ShopeeEmailRepairResult{}, err
	}
	result := ShopeeEmailRepairResult{
		CreatedCount:    len(createdBills),
		SkippedCount:    after.ExistingCount - len(createdBills),
		MissingCount:    after.MissingCount,
		CreatedBillIDs:  mapCreatedBillIDs(createdBills),
		CreatedOrderIDs: mapCreatedOrderIDs(createdBills),
		MissingOrderIDs: after.MissingOrderIDs,
		StaleCleared:    cleared,
		OutcomeKind:     string(outcome.Kind),
		OutcomeCode:     outcome.Code,
	}
	if result.SkippedCount < 0 {
		result.SkippedCount = 0
	}
	return result, nil
}

func (s *ShopeeEmailRepairService) newEmailHandler() (*EmailHandler, error) {
	if s.cfg.OpenRouterAPIKey == "" {
		return nil, fmt.Errorf("OPENROUTER_API_KEY is required")
	}
	if s.catalogRepo == nil {
		return nil, fmt.Errorf("catalog repository not configured")
	}
	catalogSvc := catalog.NewSMLCatalogService(s.catalogRepo, s.cfg.ShopeeSMLURL, shopeeRepairSMLHeaders(s.cfg), s.logger)
	aiClient := ai.NewClient(
		s.cfg.OpenRouterAPIKey,
		s.cfg.OpenRouterModel,
		s.cfg.OpenRouterFallback,
		s.cfg.OpenRouterAudioModel,
	).WithAppAttribution(s.cfg.OpenRouterAppTitle, s.cfg.OpenRouterAppReferer)
	h := NewEmailHandler(aiClient, nil, nil, nil, s.billRepo, s.auditRepo, nil, s.cfg.AutoConfirmThreshold, s.logger)
	h.SetCatalogServices(catalogSvc, nil, nil, s.catalogRepo)
	h.SetChannelDefaults(s.channelDefaults)
	h.SetArtifactService(s.artifactSvc)
	return h, nil
}

func (s *ShopeeEmailRepairService) loadTarget(billID string) (shopeeRepairTarget, error) {
	bill, err := s.billRepo.FindByID(strings.TrimSpace(billID))
	if err != nil {
		return shopeeRepairTarget{}, err
	}
	if bill == nil {
		return shopeeRepairTarget{}, notFoundShopeeRepair("ไม่พบบิลที่ต้องการ")
	}
	if bill.Source != "shopee_shipped" || bill.BillType != "purchase" {
		return shopeeRepairTarget{}, badShopeeRepairRequest("เครื่องมือนี้ใช้กับบิลซื้อ Shopee จากอีเมลเท่านั้น")
	}
	if bill.ArchivedAt != nil {
		return shopeeRepairTarget{}, badShopeeRepairRequest("บิลนี้ถูกเก็บแล้ว จึงซ่อมจากอีเมลไม่ได้")
	}
	raw := map[string]interface{}{}
	if len(bill.RawData) > 0 {
		_ = json.Unmarshal(bill.RawData, &raw)
	}
	target := shopeeRepairTarget{
		BillID:    bill.ID,
		Subject:   repairStringField(raw, "subject"),
		FromAddr:  repairStringField(raw, "from"),
		MessageID: repairStringField(raw, "email_message_id"),
		Raw:       raw,
	}
	if target.MessageID == "" {
		return shopeeRepairTarget{}, badShopeeRepairRequest("บิลนี้ไม่มี email message id สำหรับย้อนอ่านอีเมลเดิม")
	}
	eventType, _, _, ok := shopeeOrderEventFromSubject(target.Subject)
	if !ok || eventType != shopeeEventPaymentConfirmed {
		return shopeeRepairTarget{}, badShopeeRepairRequest("ซ่อมได้เฉพาะอีเมล Shopee ยืนยันการชำระเงินเท่านั้น")
	}
	return target, nil
}

func (s *ShopeeEmailRepairService) loadEmailBody(target shopeeRepairTarget) (shopeeRepairEmailBody, error) {
	artifacts, err := s.artifactSvc.ListByBill(target.BillID)
	if err != nil {
		return shopeeRepairEmailBody{}, err
	}
	for _, preferKind := range []string{"email_html", "email_text"} {
		for _, a := range artifacts {
			if a.Kind != preferKind {
				continue
			}
			if target.MessageID != "" && artifactRepairMessageID(a) != "" && artifactRepairMessageID(a) != target.MessageID {
				continue
			}
			data, _, err := s.artifactSvc.Read(a.ID)
			if err != nil || len(data) == 0 {
				continue
			}
			if a.Kind == "email_html" {
				return shopeeRepairEmailBody{HTML: string(data), ArtifactID: a.ID}, nil
			}
			return shopeeRepairEmailBody{Text: string(data), ArtifactID: a.ID}, nil
		}
	}
	for _, preferKind := range []string{"email_html", "email_text"} {
		for _, a := range artifacts {
			if a.Kind != preferKind {
				continue
			}
			data, _, err := s.artifactSvc.Read(a.ID)
			if err != nil || len(data) == 0 {
				continue
			}
			if a.Kind == "email_html" {
				return shopeeRepairEmailBody{HTML: string(data), ArtifactID: a.ID}, nil
			}
			return shopeeRepairEmailBody{Text: string(data), ArtifactID: a.ID}, nil
		}
	}
	text := repairStringField(target.Raw, "body_text")
	html := repairStringField(target.Raw, "body_html")
	if strings.TrimSpace(text) == "" && strings.TrimSpace(html) == "" {
		return shopeeRepairEmailBody{}, badShopeeRepairRequest("ไม่พบ email artifact หรือ raw body สำหรับซ่อม")
	}
	return shopeeRepairEmailBody{Text: text, HTML: html, ArtifactID: "raw_data"}, nil
}

func (s *ShopeeEmailRepairService) inspectTarget(target shopeeRepairTarget, body shopeeRepairEmailBody) (ShopeeEmailRepairPreview, error) {
	plainText := htmlToText(body.Text)
	if strings.TrimSpace(plainText) == "" {
		plainText = htmlToText(body.HTML)
	}
	orderIDs := DetectShopeeBodyOrderIDs(plainText, body.HTML)
	if len(orderIDs) == 0 {
		return ShopeeEmailRepairPreview{}, badShopeeRepairRequest("อ่านเลขคำสั่งซื้อจากอีเมลต้นฉบับไม่ได้")
	}
	existing, err := s.existingShopeeBills(orderIDs)
	if err != nil {
		return ShopeeEmailRepairPreview{}, err
	}
	missing := []string{}
	existingOrderIDs := []string{}
	for _, orderID := range orderIDs {
		if existing[orderID] == "" {
			missing = append(missing, orderID)
		} else {
			existingOrderIDs = append(existingOrderIDs, orderID)
		}
	}
	stale, err := s.staleTombstones(target.MessageID, missing)
	if err != nil {
		return ShopeeEmailRepairPreview{}, err
	}
	total := 0.0
	warnings := []string{}
	for _, orderID := range orderIDs {
		amount, ok := repository.ExtractShopeeMoneyLabel(plainText, body.HTML, orderID, "ยอดที่ต้องชำระทั้งหมด")
		if !ok {
			warnings = append(warnings, "อ่านยอดรวมของคำสั่งซื้อ "+orderID+" ไม่ได้")
			continue
		}
		total += amount
	}
	total = math.Round(total*100) / 100
	if len(stale) > 0 {
		warnings = append(warnings, "พบประวัติ processed เก่าของรายการที่ยังไม่มีบิล ระบบจะล้างเฉพาะรายการที่ตกหล่นตอนซ่อม")
	}
	return ShopeeEmailRepairPreview{
		BillID:                 target.BillID,
		MessageID:              target.MessageID,
		ArtifactID:             body.ArtifactID,
		Subject:                target.Subject,
		DetectedOrderCount:     len(orderIDs),
		ExistingCount:          len(existingOrderIDs),
		MissingCount:           len(missing),
		DetectedOrderIDs:       orderIDs,
		ExistingOrderIDs:       existingOrderIDs,
		MissingOrderIDs:        missing,
		EmailTotal:             total,
		HasStaleTombstones:     len(stale) > 0,
		StaleTombstoneOrderIDs: stale,
		Warnings:               warnings,
		CanRepair:              len(missing) > 0 && total > 0,
	}, nil
}

func (s *ShopeeEmailRepairService) existingShopeeBills(orderIDs []string) (map[string]string, error) {
	out := map[string]string{}
	if len(orderIDs) == 0 {
		return out, nil
	}
	rows, err := s.db.Query(
		`SELECT id::text,
		        UPPER(TRIM(LEADING '#' FROM COALESCE(NULLIF(raw_data->>'order_id',''), sml_order_id, ''))) AS order_id
		   FROM bills
		  WHERE source = 'shopee_shipped'
		    AND bill_type = 'purchase'
		    AND archived_at IS NULL
		    AND UPPER(TRIM(LEADING '#' FROM COALESCE(NULLIF(raw_data->>'order_id',''), sml_order_id, ''))) = ANY($1)`,
		pq.Array(orderIDs),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var billID, orderID string
		if err := rows.Scan(&billID, &orderID); err != nil {
			return nil, err
		}
		out[orderID] = billID
	}
	return out, rows.Err()
}

func (s *ShopeeEmailRepairService) staleTombstones(messageID string, missingOrderIDs []string) ([]string, error) {
	out := []string{}
	if strings.TrimSpace(messageID) == "" || len(missingOrderIDs) == 0 {
		return out, nil
	}
	rows, err := s.db.Query(
		`SELECT DISTINCT UPPER(TRIM(LEADING '#' FROM COALESCE(order_id, ''))) AS order_id
		   FROM processed_email_keys
		  WHERE source = 'shopee_shipped'
		    AND message_id = $1
		    AND UPPER(TRIM(LEADING '#' FROM COALESCE(order_id, ''))) = ANY($2)
		  ORDER BY order_id`,
		messageID, pq.Array(missingOrderIDs),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var orderID string
		if err := rows.Scan(&orderID); err != nil {
			return nil, err
		}
		if orderID != "" {
			out = append(out, orderID)
		}
	}
	return out, rows.Err()
}

func (s *ShopeeEmailRepairService) clearStaleTombstones(messageID string, missingOrderIDs []string) ([]string, error) {
	out := []string{}
	if strings.TrimSpace(messageID) == "" || len(missingOrderIDs) == 0 {
		return out, nil
	}
	rows, err := s.db.Query(
		`DELETE FROM processed_email_keys pek
		  WHERE pek.source = 'shopee_shipped'
		    AND pek.message_id = $1
		    AND UPPER(TRIM(LEADING '#' FROM COALESCE(pek.order_id, ''))) = ANY($2)
		    AND NOT EXISTS (
		      SELECT 1 FROM bills b
		       WHERE b.source = 'shopee_shipped'
		         AND b.bill_type = 'purchase'
		         AND b.archived_at IS NULL
		         AND UPPER(TRIM(LEADING '#' FROM COALESCE(NULLIF(b.raw_data->>'order_id',''), b.sml_order_id, ''))) =
		             UPPER(TRIM(LEADING '#' FROM COALESCE(pek.order_id, '')))
		    )
		  RETURNING UPPER(TRIM(LEADING '#' FROM COALESCE(order_id, ''))) AS order_id`,
		messageID, pq.Array(missingOrderIDs),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var orderID string
		if err := rows.Scan(&orderID); err != nil {
			return nil, err
		}
		if orderID != "" {
			out = append(out, orderID)
		}
	}
	return out, rows.Err()
}

type createdRepairBill struct {
	ID      string
	OrderID string
}

func (s *ShopeeEmailRepairService) createdBillsForOrders(orderIDs []string) ([]createdRepairBill, error) {
	out := []createdRepairBill{}
	if len(orderIDs) == 0 {
		return out, nil
	}
	rows, err := s.db.Query(
		`SELECT id::text,
		        UPPER(TRIM(LEADING '#' FROM COALESCE(NULLIF(raw_data->>'order_id',''), sml_order_id, ''))) AS order_id
		   FROM bills
		  WHERE source = 'shopee_shipped'
		    AND bill_type = 'purchase'
		    AND archived_at IS NULL
		    AND UPPER(TRIM(LEADING '#' FROM COALESCE(NULLIF(raw_data->>'order_id',''), sml_order_id, ''))) = ANY($1)
		  ORDER BY created_at, id`,
		pq.Array(orderIDs),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var b createdRepairBill
		if err := rows.Scan(&b.ID, &b.OrderID); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (s *ShopeeEmailRepairService) createRepairJob(preview ShopeeEmailRepairPreview, userID, userEmail string) (*ShopeeEmailRepairJob, bool, error) {
	if err := s.expireStaleActiveJobs(preview.MessageID); err != nil {
		return nil, false, err
	}
	snapshot, _ := json.Marshal(preview)
	var createdBy interface{}
	if strings.TrimSpace(userID) != "" {
		createdBy = strings.TrimSpace(userID)
	}
	var jobID string
	err := s.db.QueryRow(
		`INSERT INTO email_repair_jobs
		   (bill_id, source, message_id, status, snapshot, created_by, created_by_email)
		 VALUES ($1::uuid, 'shopee_shipped', $2, 'queued', $3::jsonb, $4::uuid, $5)
		 RETURNING id::text`,
		preview.BillID, preview.MessageID, string(snapshot), createdBy, strings.TrimSpace(userEmail),
	).Scan(&jobID)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			active, getErr := s.findActiveJob(preview.MessageID)
			if getErr != nil {
				return nil, false, getErr
			}
			if active != nil {
				return active, true, nil
			}
		}
		return nil, false, err
	}
	job, err := s.getJob(jobID, preview.BillID)
	return job, false, err
}

func (s *ShopeeEmailRepairService) expireStaleActiveJobs(messageID string) error {
	_, err := s.db.Exec(
		`UPDATE email_repair_jobs
		    SET status = 'failed',
		        error = 'repair job timed out before completion; please preview and start again',
		        finished_at = now(),
		        updated_at = now()
		  WHERE source = 'shopee_shipped'
		    AND message_id = $1
		    AND status IN ('queued','running')
		    AND created_at < now() - interval '30 minutes'`,
		messageID,
	)
	return err
}

func (s *ShopeeEmailRepairService) findActiveJob(messageID string) (*ShopeeEmailRepairJob, error) {
	var id string
	err := s.db.QueryRow(
		`SELECT id::text
		   FROM email_repair_jobs
		  WHERE source = 'shopee_shipped'
		    AND message_id = $1
		    AND status IN ('queued','running')
		  ORDER BY created_at DESC
		  LIMIT 1`,
		messageID,
	).Scan(&id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return s.getJob(id, "")
}

func (s *ShopeeEmailRepairService) getJob(jobID, billID string) (*ShopeeEmailRepairJob, error) {
	where := "id = $1"
	args := []interface{}{jobID}
	if strings.TrimSpace(billID) != "" {
		where += " AND bill_id = $2::uuid"
		args = append(args, billID)
	}
	row := s.db.QueryRow(
		`SELECT id::text, COALESCE(bill_id::text,''), source, message_id, status,
		        snapshot, result, error, created_by_email,
		        started_at, finished_at, created_at, updated_at
		   FROM email_repair_jobs
		  WHERE `+where,
		args...,
	)
	return scanShopeeEmailRepairJob(row)
}

func (h *BillHandler) getShopeeEmailRepairJob(jobID, billID string) (*ShopeeEmailRepairJob, error) {
	if h == nil || h.billRepo == nil {
		return nil, fmt.Errorf("bill repository not configured")
	}
	return (&ShopeeEmailRepairService{db: h.billRepo.DB()}).getJob(jobID, billID)
}

func scanShopeeEmailRepairJob(row interface {
	Scan(dest ...interface{}) error
}) (*ShopeeEmailRepairJob, error) {
	var job ShopeeEmailRepairJob
	var snapshotRaw, resultRaw []byte
	err := row.Scan(
		&job.ID, &job.BillID, &job.Source, &job.MessageID, &job.Status,
		&snapshotRaw, &resultRaw, &job.Error, &job.CreatedByEmail,
		&job.StartedAt, &job.FinishedAt, &job.CreatedAt, &job.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, notFoundShopeeRepair("ไม่พบ repair job")
	}
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(snapshotRaw, &job.Snapshot)
	_ = json.Unmarshal(resultRaw, &job.Result)
	job.CreatedCount = job.Result.CreatedCount
	job.SkippedCount = job.Result.SkippedCount
	job.MissingCount = job.Result.MissingCount
	job.CreatedBillIDs = job.Result.CreatedBillIDs
	job.CreatedOrderIDs = job.Result.CreatedOrderIDs
	job.MissingOrderIDs = job.Result.MissingOrderIDs
	return &job, nil
}

func (s *ShopeeEmailRepairService) markJobRunning(jobID string) error {
	_, err := s.db.Exec(
		`UPDATE email_repair_jobs
		    SET status = 'running', started_at = COALESCE(started_at, now()), updated_at = now()
		  WHERE id = $1::uuid AND status = 'queued'`,
		jobID,
	)
	return err
}

func (s *ShopeeEmailRepairService) markJobSucceeded(jobID string, result ShopeeEmailRepairResult) error {
	raw, _ := json.Marshal(result)
	_, err := s.db.Exec(
		`UPDATE email_repair_jobs
		    SET status = 'succeeded', result = $2::jsonb, error = '',
		        finished_at = now(), updated_at = now()
		  WHERE id = $1::uuid`,
		jobID, string(raw),
	)
	return err
}

func (s *ShopeeEmailRepairService) markJobFailed(jobID, message string) error {
	_, err := s.db.Exec(
		`UPDATE email_repair_jobs
		    SET status = 'failed', error = $2, finished_at = now(), updated_at = now()
		  WHERE id = $1::uuid`,
		jobID, message,
	)
	return err
}

func (s *ShopeeEmailRepairService) auditJob(action, billID, jobID, level string, detail map[string]interface{}) {
	if s.handler == nil || s.auditRepo == nil {
		return
	}
	if detail == nil {
		detail = map[string]interface{}{}
	}
	detail["job_id"] = jobID
	var targetID *string
	if billID != "" {
		targetID = &billID
	}
	_ = s.auditRepo.Log(models.AuditEntry{
		Action:   action,
		TargetID: targetID,
		Source:   "shopee_shipped",
		Level:    level,
		Detail:   detail,
	})
}

func (h *BillHandler) auditShopeeEmailRepair(c *gin.Context, action, billID, level string, detail map[string]interface{}) {
	if h == nil || h.auditRepo == nil {
		return
	}
	var userID *string
	if uid := c.GetString("user_id"); uid != "" {
		userID = &uid
	}
	targetID := billID
	_ = h.auditRepo.Log(models.AuditEntry{
		Action:   action,
		TargetID: &targetID,
		UserID:   userID,
		Source:   "shopee_shipped",
		Level:    level,
		TraceID:  c.GetString("trace_id"),
		Detail:   detail,
	})
}

type shopeeRepairHTTPError struct {
	status  int
	message string
}

func (e shopeeRepairHTTPError) Error() string { return e.message }

func badShopeeRepairRequest(message string) error {
	return shopeeRepairHTTPError{status: http.StatusBadRequest, message: message}
}

func notFoundShopeeRepair(message string) error {
	return shopeeRepairHTTPError{status: http.StatusNotFound, message: message}
}

func writeShopeeRepairError(c *gin.Context, err error) {
	var httpErr shopeeRepairHTTPError
	if errors.As(err, &httpErr) {
		c.JSON(httpErr.status, gin.H{"error": httpErr.message})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": truncateRepairError(err.Error())})
}

func artifactRepairMessageID(a models.BillArtifact) string {
	if len(a.SourceMeta) == 0 {
		return ""
	}
	var meta struct {
		MessageID string `json:"message_id"`
	}
	if err := json.Unmarshal(a.SourceMeta, &meta); err != nil {
		return ""
	}
	return strings.TrimSpace(meta.MessageID)
}

func mailSourceFromRepairRaw(raw map[string]interface{}) emailservice.MailSource {
	return emailservice.MailSource{
		AccountID:   repairStringField(raw, "imap_account_id"),
		AccountName: repairStringField(raw, "imap_account_name"),
		Username:    repairStringField(raw, "imap_username"),
		Mailbox:     repairStringField(raw, "imap_mailbox"),
		EmailDate:   repairStringField(raw, "email_date"),
	}
}

func repairStringField(raw map[string]interface{}, key string) string {
	if raw == nil {
		return ""
	}
	switch v := raw[key].(type) {
	case string:
		return strings.TrimSpace(v)
	default:
		return ""
	}
}

func normalizeShopeeOrderIDs(orderIDs []string) []string {
	out := []string{}
	seen := map[string]struct{}{}
	for _, orderID := range orderIDs {
		id := normalizeShopeeOrderID(orderID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func sameStringSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := map[string]int{}
	for _, v := range a {
		seen[v]++
	}
	for _, v := range b {
		if seen[v] == 0 {
			return false
		}
		seen[v]--
	}
	return true
}

func mapCreatedBillIDs(bills []createdRepairBill) []string {
	out := make([]string, 0, len(bills))
	for _, b := range bills {
		out = append(out, b.ID)
	}
	return out
}

func mapCreatedOrderIDs(bills []createdRepairBill) []string {
	out := make([]string, 0, len(bills))
	for _, b := range bills {
		out = append(out, b.OrderID)
	}
	return out
}

func truncateRepairError(s string) string {
	s = strings.TrimSpace(s)
	runes := []rune(s)
	if len(runes) > 500 {
		return string(runes[:500]) + "..."
	}
	if s == "" {
		return "repair failed"
	}
	return s
}

func shopeeRepairSMLHeaders(cfg *config.Config) map[string]string {
	return map[string]string{
		"guid":           cfg.ShopeeSMLGUID,
		"provider":       cfg.ShopeeSMLProvider,
		"configFileName": cfg.ShopeeSMLConfigFile,
		"databaseName":   cfg.ShopeeSMLDatabase,
	}
}

func (s *ShopeeEmailRepairService) logWarn(msg string, fields ...zap.Field) {
	if s.logger != nil {
		s.logger.Warn(msg, fields...)
	}
}
