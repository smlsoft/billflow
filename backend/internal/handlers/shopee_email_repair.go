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
	Source                 string   `json:"source"`
	MessageID              string   `json:"message_id"`
	ArtifactID             string   `json:"artifact_id"`
	Subject                string   `json:"subject"`
	InputSubject           string   `json:"input_subject,omitempty"`
	DetectedOrderCount     int      `json:"detected_order_count"`
	ExistingCount          int      `json:"existing_count"`
	MissingCount           int      `json:"missing_count"`
	RebuildCount           int      `json:"rebuild_count"`
	BlockedCount           int      `json:"blocked_count"`
	DetectedOrderIDs       []string `json:"detected_order_ids"`
	ExistingOrderIDs       []string `json:"existing_order_ids"`
	MissingOrderIDs        []string `json:"missing_order_ids"`
	RebuildOrderIDs        []string `json:"rebuild_order_ids,omitempty"`
	BlockedOrderIDs        []string `json:"blocked_order_ids,omitempty"`
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
	RebuiltCount    int                      `json:"rebuilt_count,omitempty"`
	SkippedCount    int                      `json:"skipped_count"`
	MissingCount    int                      `json:"missing_count"`
	CreatedBillIDs  []string                 `json:"created_bill_ids,omitempty"`
	CreatedOrderIDs []string                 `json:"created_order_ids,omitempty"`
	RebuiltBillIDs  []string                 `json:"rebuilt_bill_ids,omitempty"`
	RebuiltOrderIDs []string                 `json:"rebuilt_order_ids,omitempty"`
	MissingOrderIDs []string                 `json:"missing_order_ids,omitempty"`
	Progress        EmailRepairJobProgress   `json:"progress"`
}

type ShopeeEmailRepairResult struct {
	CreatedCount    int                    `json:"created_count"`
	RebuiltCount    int                    `json:"rebuilt_count,omitempty"`
	SkippedCount    int                    `json:"skipped_count"`
	MissingCount    int                    `json:"missing_count"`
	CreatedBillIDs  []string               `json:"created_bill_ids,omitempty"`
	CreatedOrderIDs []string               `json:"created_order_ids,omitempty"`
	RebuiltBillIDs  []string               `json:"rebuilt_bill_ids,omitempty"`
	RebuiltOrderIDs []string               `json:"rebuilt_order_ids,omitempty"`
	MissingOrderIDs []string               `json:"missing_order_ids,omitempty"`
	StaleCleared    []string               `json:"stale_tombstone_order_ids,omitempty"`
	OutcomeKind     string                 `json:"outcome_kind,omitempty"`
	OutcomeCode     string                 `json:"outcome_code,omitempty"`
	Progress        EmailRepairJobProgress `json:"progress,omitempty"`
}

type EmailRepairJobProgress struct {
	Percent        int    `json:"percent"`
	Stage          string `json:"stage,omitempty"`
	Label          string `json:"label,omitempty"`
	Current        int    `json:"current,omitempty"`
	Total          int    `json:"total,omitempty"`
	CurrentOrderID string `json:"current_order_id,omitempty"`
}

type createShopeeEmailRepairJobRequest struct {
	ExpectedOrderCount      int      `json:"expected_order_count"`
	ExpectedTotal           float64  `json:"expected_total"`
	ExpectedMissingOrderIDs []string `json:"expected_missing_order_ids"`
	ExpectedRebuildOrderIDs []string `json:"expected_rebuild_order_ids"`
	Subject                 string   `json:"subject"`
}

type shopeeRepairTarget struct {
	BillID       string
	Source       string
	Subject      string
	InputSubject string
	FromAddr     string
	MessageID    string
	ArtifactID   string
	Raw          map[string]interface{}
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
	preview, err := svc.Preview(c.Param("id"), c.Query("subject"))
	if err != nil {
		writeShopeeRepairError(c, err)
		return
	}
	h.auditShopeeEmailRepair(c, "shopee_email_repair_previewed", c.Param("id"), "info", map[string]interface{}{
		"source":               preview.Source,
		"message_id":           preview.MessageID,
		"detected_order_count": preview.DetectedOrderCount,
		"existing_count":       preview.ExistingCount,
		"missing_count":        preview.MissingCount,
		"rebuild_count":        preview.RebuildCount,
		"blocked_count":        preview.BlockedCount,
		"missing_order_ids":    preview.MissingOrderIDs,
		"rebuild_order_ids":    preview.RebuildOrderIDs,
		"blocked_order_ids":    preview.BlockedOrderIDs,
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
		"source":                 job.Source,
		"job_id":                 job.ID,
		"message_id":             job.MessageID,
		"expected_order_count":   req.ExpectedOrderCount,
		"expected_total":         req.ExpectedTotal,
		"expected_missing_ids":   normalizeShopeeOrderIDs(req.ExpectedMissingOrderIDs),
		"expected_rebuild_ids":   normalizeShopeeOrderIDs(req.ExpectedRebuildOrderIDs),
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

func (s *ShopeeEmailRepairService) Preview(billID string, subject string) (ShopeeEmailRepairPreview, error) {
	target, err := s.loadTarget(billID, subject)
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
	preview, err := s.Preview(billID, req.Subject)
	if err != nil {
		return nil, err
	}
	if preview.MissingCount == 0 && preview.RebuildCount == 0 {
		return nil, badShopeeRepairRequest("อีเมลนี้ไม่มีรายการที่ต้องซ่อม")
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
	if !sameStringSet(normalizeShopeeOrderIDs(req.ExpectedRebuildOrderIDs), preview.RebuildOrderIDs) {
		return nil, badShopeeRepairRequest("รายการที่จะซ่อมจากอีเมลยืนยันเปลี่ยนไป กรุณากดตรวจใหม่ก่อนซ่อม")
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
	s.updateJobProgress(jobID, EmailRepairJobProgress{Percent: 5, Stage: "started", Label: "เริ่มงานซ่อมจากอีเมลยืนยัน"})
	job, err := s.getJob(jobID, billID)
	if err != nil {
		msg := truncateRepairError(err.Error())
		_ = s.markJobFailed(jobID, msg)
		s.auditJob("shopee_email_repair_failed", "", billID, jobID, "error", map[string]interface{}{
			"error":       msg,
			"duration_ms": int(time.Since(start).Milliseconds()),
		})
		return
	}
	progress := func(p EmailRepairJobProgress) {
		s.updateJobProgress(jobID, p)
	}
	result, err := s.applyRepairWithProgress(billID, job.Snapshot, progress)
	if err != nil {
		msg := truncateRepairError(err.Error())
		_ = s.markJobFailed(jobID, msg)
		s.auditJob("shopee_email_repair_failed", job.Source, billID, jobID, "error", map[string]interface{}{
			"error":       msg,
			"duration_ms": int(time.Since(start).Milliseconds()),
		})
		return
	}
	if err := s.markJobSucceeded(jobID, result); err != nil {
		s.logWarn("shopee_email_repair: mark succeeded failed", zap.String("job_id", jobID), zap.Error(err))
		return
	}
	s.auditJob("shopee_email_repair_completed", job.Source, billID, jobID, "info", map[string]interface{}{
		"created_count":     result.CreatedCount,
		"rebuilt_count":     result.RebuiltCount,
		"skipped_count":     result.SkippedCount,
		"missing_count":     result.MissingCount,
		"created_bill_ids":  result.CreatedBillIDs,
		"created_order_ids": result.CreatedOrderIDs,
		"rebuilt_bill_ids":  result.RebuiltBillIDs,
		"rebuilt_order_ids": result.RebuiltOrderIDs,
		"stale_tombstones":  result.StaleCleared,
		"duration_ms":       int(time.Since(start).Milliseconds()),
	})
}

func (s *ShopeeEmailRepairService) applyRepair(billID string, snapshot ShopeeEmailRepairPreview) (ShopeeEmailRepairResult, error) {
	return s.applyRepairWithProgress(billID, snapshot, nil)
}

func (s *ShopeeEmailRepairService) applyRepairWithProgress(billID string, snapshot ShopeeEmailRepairPreview, progress func(EmailRepairJobProgress)) (ShopeeEmailRepairResult, error) {
	reportRepairProgress(progress, 10, "verify", "ตรวจข้อมูลอีเมลล่าสุดก่อนซ่อม", 0, 0, "")
	before, err := s.Preview(billID, snapshot.Subject)
	if err != nil {
		return ShopeeEmailRepairResult{}, err
	}
	if !sameStringSet(before.MissingOrderIDs, snapshot.MissingOrderIDs) ||
		!sameStringSet(before.RebuildOrderIDs, snapshot.RebuildOrderIDs) ||
		before.DetectedOrderCount != snapshot.DetectedOrderCount ||
		math.Abs(before.EmailTotal-snapshot.EmailTotal) > 0.01 {
		return ShopeeEmailRepairResult{}, badShopeeRepairRequest("ข้อมูลอีเมลเปลี่ยนไป กรุณากดตรวจใหม่ก่อนซ่อม")
	}
	if before.MissingCount == 0 && before.RebuildCount == 0 {
		return ShopeeEmailRepairResult{SkippedCount: before.ExistingCount}, nil
	}
	reportRepairProgress(progress, 20, "dedupe", "ตรวจและล้างสถานะอีเมลซ้ำที่ค้างอยู่", 0, 0, "")
	cleared, err := s.clearStaleTombstones(before.Source, before.MessageID, before.MissingOrderIDs)
	if err != nil {
		return ShopeeEmailRepairResult{}, fmt.Errorf("clear stale processed keys: %w", err)
	}
	reportRepairProgress(progress, 28, "load_email", "โหลดอีเมลต้นฉบับสำหรับซ่อม", 0, 0, "")
	target, err := s.loadTarget(billID, before.Subject)
	if err != nil {
		return ShopeeEmailRepairResult{}, err
	}
	body, err := s.loadEmailBody(target)
	if err != nil {
		return ShopeeEmailRepairResult{}, err
	}
	reportRepairProgress(progress, 35, "prepare", "เตรียมตัวอ่านรายการคำสั่งซื้อจากอีเมล", 0, 0, "")
	emailHandler, err := s.newEmailHandler()
	if err != nil {
		return ShopeeEmailRepairResult{}, err
	}
	var applied appliedShopeePaymentRepair
	if before.Source == lazadaEmailSource {
		applied, err = s.applyLazadaEmailRepairOrders(emailHandler, target, body, before.MissingOrderIDs, before.RebuildOrderIDs, progress)
	} else {
		applied, err = s.applyPaymentEmailRepairOrders(emailHandler, target, body, before.MissingOrderIDs, before.RebuildOrderIDs, progress)
	}
	if err != nil {
		return ShopeeEmailRepairResult{}, err
	}
	reportRepairProgress(progress, 95, "verify_result", "ตรวจผลหลังซ่อมว่าครบทุกคำสั่งซื้อ", 0, 0, "")
	after, err := s.Preview(billID, before.Subject)
	if err != nil {
		return ShopeeEmailRepairResult{}, err
	}
	if after.MissingCount > 0 {
		return ShopeeEmailRepairResult{}, fmt.Errorf("repair incomplete; missing: %s", strings.Join(after.MissingOrderIDs, ", "))
	}
	createdBills, err := s.createdBillsForOrders(before.Source, before.MissingOrderIDs)
	if err != nil {
		return ShopeeEmailRepairResult{}, err
	}
	result := ShopeeEmailRepairResult{
		CreatedCount:    len(createdBills),
		RebuiltCount:    len(applied.rebuilt),
		SkippedCount:    after.ExistingCount - len(createdBills) - len(applied.rebuilt),
		MissingCount:    after.MissingCount,
		CreatedBillIDs:  mapCreatedBillIDs(createdBills),
		CreatedOrderIDs: mapCreatedOrderIDs(createdBills),
		RebuiltBillIDs:  mapRebuiltBillIDs(applied.rebuilt),
		RebuiltOrderIDs: mapRebuiltOrderIDs(applied.rebuilt),
		MissingOrderIDs: after.MissingOrderIDs,
		StaleCleared:    cleared,
		OutcomeKind:     "repaired",
		OutcomeCode:     "payment_email_repair_applied",
		Progress:        EmailRepairJobProgress{Percent: 100, Stage: "succeeded", Label: "ซ่อมคำสั่งซื้อจากอีเมลเสร็จแล้ว", Current: after.DetectedOrderCount, Total: after.DetectedOrderCount},
	}
	if result.SkippedCount < 0 {
		result.SkippedCount = 0
	}
	return result, nil
}

type appliedShopeePaymentRepair struct {
	rebuilt []createdRepairBill
}

func (s *ShopeeEmailRepairService) applyPaymentEmailRepairOrders(
	emailHandler *EmailHandler,
	target shopeeRepairTarget,
	body shopeeRepairEmailBody,
	missingOrderIDs []string,
	rebuildOrderIDs []string,
	progress func(EmailRepairJobProgress),
) (appliedShopeePaymentRepair, error) {
	result := appliedShopeePaymentRepair{}
	plainText := htmlToText(body.Text)
	if strings.TrimSpace(plainText) == "" {
		plainText = htmlToText(body.HTML)
	}
	traceID := fmt.Sprintf("shopee-email-repair-%d", time.Now().UnixMilli())
	total := len(missingOrderIDs) + len(rebuildOrderIDs)
	reportRepairProgress(progress, 42, "extract", "อ่านข้อมูลคำสั่งซื้อจากอีเมล Shopee", 0, total, "")
	orders, err := emailHandler.extractShopeeOrdersBounded(target.Subject, plainText, body.HTML, traceID)
	if err != nil {
		return result, err
	}
	ordersByID := map[string]ai.ExtractedOrder{}
	for _, order := range orders {
		orderID := normalizeShopeeOrderID(order.OrderID)
		if orderID != "" {
			order.OrderID = orderID
			ordersByID[orderID] = order
		}
	}
	fallbackPrices := extractShopeePrices(plainText)
	source := mailSourceFromRepairRaw(target.Raw)
	start := time.Now()
	processed := 0
	for _, orderID := range missingOrderIDs {
		reportRepairProgress(progress, repairOrderPercent(processed, total), "create_order", "กำลังสร้างบิลที่ตกหล่น", processed, total, orderID)
		order, ok := ordersByID[orderID]
		if !ok {
			return result, fmt.Errorf("AI extract missing order %s", orderID)
		}
		if _, err := emailHandler.processOneShippedOrder(order, target.Subject, target.FromAddr, body.Text, body.HTML, target.MessageID, fallbackPrices, traceID, start, source); err != nil {
			return result, err
		}
		processed++
		reportRepairProgress(progress, repairOrderPercent(processed, total), "create_order", "สร้างบิลที่ตกหล่นแล้ว", processed, total, orderID)
	}
	for _, orderID := range rebuildOrderIDs {
		reportRepairProgress(progress, repairOrderPercent(processed, total), "rebuild_order", "กำลังซ่อมบิลเดิมจากอีเมลยืนยัน", processed, total, orderID)
		order, ok := ordersByID[orderID]
		if !ok {
			return result, fmt.Errorf("AI extract missing rebuild order %s", orderID)
		}
		billID, err := s.rebuildOneShopeeBillFromPaymentEmail(emailHandler, order, target, body, fallbackPrices, traceID, start, source)
		if err != nil {
			return result, err
		}
		if billID != "" {
			result.rebuilt = append(result.rebuilt, createdRepairBill{ID: billID, OrderID: orderID})
		}
		processed++
		reportRepairProgress(progress, repairOrderPercent(processed, total), "rebuild_order", "ซ่อมบิลเดิมแล้ว", processed, total, orderID)
	}
	if target.MessageID != "" {
		_ = s.billRepo.MarkProcessedEmailKey("shopee_shipped", target.MessageID, "")
	}
	return result, nil
}

type shopeeRepairItemWithCandidates struct {
	item       models.BillItem
	candidates []models.CatalogMatch
}

func (s *ShopeeEmailRepairService) rebuildOneShopeeBillFromPaymentEmail(
	emailHandler *EmailHandler,
	order ai.ExtractedOrder,
	target shopeeRepairTarget,
	body shopeeRepairEmailBody,
	fallbackPrices []float64,
	traceID string,
	startTime time.Time,
	source emailservice.MailSource,
) (string, error) {
	orderID := normalizeShopeeOrderID(order.OrderID)
	if orderID == "" {
		return "", fmt.Errorf("rebuild shopee bill: empty order id")
	}
	validItems := make([]ai.ExtractedItem, 0, len(order.Items))
	for _, extItem := range order.Items {
		extItem.RawName = strings.TrimSpace(extItem.RawName)
		if extItem.RawName == "" || extItem.Qty <= 0 {
			continue
		}
		validItems = append(validItems, extItem)
	}
	if len(validItems) == 0 {
		return "", fmt.Errorf("rebuild shopee bill %s: no usable items", orderID)
	}

	itemsWithCandidates, allHighConfidence := emailHandler.buildShopeeRepairItems(orderID, validItems, body, fallbackPrices)
	docDate := order.DocDate
	if docDate == "" {
		docDate = extractDocDate(body.Text)
	}
	rawDataMap := map[string]interface{}{
		"subject":          target.Subject,
		"from":             target.FromAddr,
		"email_message_id": target.MessageID,
		"order_id":         orderID,
		"seller_name":      order.SellerName,
		"items":            validItems,
		"flow":             "shopee_shipped",
		"doc_date":         docDate,
		"body_text":        body.Text,
		"body_html":        body.HTML,
		"repair_source":    "payment_email_rebuild",
	}
	if shippingAmount, hasShippingAmount := repository.ExtractShopeeShippingAmount(body.Text, body.HTML, orderID); hasShippingAmount {
		rawDataMap["shipping_amount"] = shippingAmount
	}
	if discountSummary := repository.ExtractShopeeDiscountSummary(body.Text, body.HTML, orderID); discountSummary.HasAny() {
		rawDataMap["discount_summary"] = discountSummary
	}
	if paymentSummary := repository.ExtractShopeePaymentSummary(body.Text, body.HTML, orderID); paymentSummary.HasAny() {
		rawDataMap["payment_summary"] = paymentSummary
	}
	if coin := extractShopeeCoinForRepair(body.Text, body.HTML, orderID); coin > 0 {
		rawDataMap["shopee_coin_amount"] = coin
	}
	applyMailSource(rawDataMap, source)
	rawDataBytes, _ := json.Marshal(rawDataMap)

	status := "needs_review"
	if allHighConfidence && len(itemsWithCandidates) > 0 {
		status = "pending"
	}
	conf := order.Confidence
	billID, err := s.replaceShopeeRepairBill(orderID, status, conf, rawDataBytes, itemsWithCandidates)
	if err != nil {
		return "", err
	}
	emailHandler.recordShopeeOrderEvent(billID, target.Subject, target.FromAddr, target.MessageID, source, orderID)
	emailHandler.linkShopeeOrphanEventsToBill(billID, orderID)
	emailHandler.saveShopeeShippedEmailArtifacts(billID, target.Subject, target.FromAddr, body.Text, body.HTML, target.MessageID)
	if target.MessageID != "" {
		_ = s.billRepo.MarkProcessedEmailKey("shopee_shipped", target.MessageID, orderID)
	}
	if s.auditRepo != nil {
		durMs := int(time.Since(startTime).Milliseconds())
		_ = s.auditRepo.Log(models.AuditEntry{
			Action:     "shopee_email_repair_rebuilt_bill",
			TargetID:   &billID,
			Source:     "shopee_shipped",
			Level:      "info",
			TraceID:    traceID,
			DurationMs: &durMs,
			Detail: map[string]interface{}{
				"message_id":  target.MessageID,
				"order_id":    orderID,
				"seller_name": order.SellerName,
				"items_count": len(itemsWithCandidates),
				"status":      status,
			},
		})
	}
	return billID, nil
}

func (h *EmailHandler) buildShopeeRepairItems(orderID string, validItems []ai.ExtractedItem, body shopeeRepairEmailBody, fallbackPrices []float64) ([]shopeeRepairItemWithCandidates, bool) {
	const topK = 5
	const highConfThreshold = 0.85
	itemsWithCandidates := []shopeeRepairItemWithCandidates{}
	allHighConfidence := true
	for i, extItem := range validItems {
		var matches []models.CatalogMatch
		if h.catalogSvc != nil {
			matches, _ = h.catalogSvc.SearchByText(extItem.RawName, topK)
		}
		item := models.BillItem{
			RawName: extItem.RawName,
			Qty:     extItem.Qty,
			Mapped:  false,
		}
		if extItem.Price != nil {
			item.Price = extItem.Price
		} else if i < len(fallbackPrices) {
			p := fallbackPrices[i]
			item.Price = &p
		}
		if extItem.ImageURL != "" {
			item.SourceImageURL = extItem.ImageURL
		}
		if len(matches) > 0 && matches[0].Score >= highConfThreshold {
			item.ItemCode = &matches[0].ItemCode
			item.UnitCode = &matches[0].UnitCode
			item.Mapped = true
		} else {
			allHighConfidence = false
		}
		itemsWithCandidates = append(itemsWithCandidates, shopeeRepairItemWithCandidates{item: item, candidates: matches})
	}
	shippingAmount, hasShippingAmount := repository.ExtractShopeeShippingAmount(body.Text, body.HTML, orderID)
	if shippingItem, shippingReady := h.configuredShopeeShippingLine(orderID, shippingAmount, hasShippingAmount); shippingItem != nil {
		itemsWithCandidates = append(itemsWithCandidates, shopeeRepairItemWithCandidates{item: *shippingItem})
		if !shippingReady {
			allHighConfidence = false
		}
	}
	discountSummary := repository.ExtractShopeeDiscountSummary(body.Text, body.HTML, orderID)
	effectiveDiscount := discountSummary.TotalDiscountAmount
	if coin := extractShopeeCoinForRepair(body.Text, body.HTML, orderID); coin > 0 {
		effectiveDiscount = roundShopeeRepairMoney(effectiveDiscount + coin)
	}
	if discountSummary.HasAny() || effectiveDiscount > 0 {
		itemCopies := make([]models.BillItem, len(itemsWithCandidates))
		for i := range itemsWithCandidates {
			itemCopies[i] = itemsWithCandidates[i].item
		}
		repository.ApplyShopeeDiscountsToItems(itemCopies, effectiveDiscount)
		for i := range itemsWithCandidates {
			itemsWithCandidates[i].item.DiscountAmount = itemCopies[i].DiscountAmount
		}
	}
	return itemsWithCandidates, allHighConfidence
}

func (s *ShopeeEmailRepairService) replaceShopeeRepairBill(orderID, status string, confidence float64, rawData []byte, items []shopeeRepairItemWithCandidates) (string, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()

	var billID, currentStatus, subject, smlDocNo string
	var sent bool
	err = tx.QueryRow(
		`SELECT id::text, status, COALESCE(raw_data->>'subject',''), COALESCE(sml_doc_no,''), sent_at IS NOT NULL
		   FROM bills
		  WHERE source = 'shopee_shipped'
		    AND bill_type = 'purchase'
		    AND archived_at IS NULL
		    AND UPPER(TRIM(LEADING '#' FROM COALESCE(NULLIF(raw_data->>'order_id',''), sml_order_id, ''))) = $1
		  FOR UPDATE`,
		orderID,
	).Scan(&billID, &currentStatus, &subject, &smlDocNo, &sent)
	if err == sql.ErrNoRows {
		return "", badShopeeRepairRequest("ไม่พบบิลเดิมสำหรับซ่อม order " + orderID)
	}
	if err != nil {
		return "", err
	}
	info := shopeeExistingRepairBill{ID: billID, OrderID: orderID, Status: currentStatus, Subject: subject, SMLDocNo: smlDocNo, Sent: sent}
	if !info.CanRebuildFromPayment() {
		return "", badShopeeRepairRequest("บิล " + orderID + " ส่ง SML แล้วหรือไม่อยู่ในสถานะที่ซ่อมอัตโนมัติได้")
	}
	if _, err := tx.Exec(`DELETE FROM bill_items WHERE bill_id = $1::uuid`, billID); err != nil {
		return "", err
	}
	if _, err := tx.Exec(
		`UPDATE bills
		    SET raw_data = $2::jsonb,
		        status = $3,
		        ai_confidence = $4,
		        sml_order_id = $5,
		        error_msg = NULL
		  WHERE id = $1::uuid`,
		billID, string(rawData), status, confidence, orderID,
	); err != nil {
		return "", err
	}
	for _, iwc := range items {
		item := iwc.item
		candidatesJSON, _ := json.Marshal(iwc.candidates)
		if _, err := tx.Exec(
			`INSERT INTO bill_items (bill_id, raw_name, source_sku, source_image_url, item_code, qty, unit_code, price, discount_amount, mapped, mapping_id, candidates)
			 VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
			billID, item.RawName, item.SourceSKU, item.SourceImageURL, item.ItemCode, item.Qty,
			item.UnitCode, item.Price, item.DiscountAmount, item.Mapped, item.MappingID, candidatesJSON,
		); err != nil {
			return "", err
		}
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return billID, nil
}

func extractShopeeCoinForRepair(bodyText, bodyHTML, orderID string) float64 {
	shippingAmount, _ := repository.ExtractShopeeShippingAmount(bodyText, bodyHTML, orderID)
	discountSummary := repository.ExtractShopeeDiscountSummary(bodyText, bodyHTML, orderID)
	paidTotal, hasPaidTotal := repository.ExtractShopeeMoneyLabel("", bodyHTML, orderID, "ยอดที่ต้องชำระทั้งหมด")
	if !hasPaidTotal {
		paidTotal, hasPaidTotal = repository.ExtractShopeeMoneyLabel(bodyText, "", orderID, "ยอดที่ต้องชำระทั้งหมด")
	}
	goodsTotal, hasGoodsTotal := repository.ExtractShopeeMoneyLabel("", bodyHTML, orderID, "ยอดรวมค่าสินค้า")
	if !hasGoodsTotal {
		goodsTotal, hasGoodsTotal = repository.ExtractShopeeMoneyLabel(bodyText, "", orderID, "ยอดรวมค่าสินค้า")
	}
	if !hasGoodsTotal {
		return 0
	}
	coin, ok := repository.CalcShopeeCoinAmount(goodsTotal, shippingAmount, discountSummary.TotalDiscountAmount, paidTotal, hasPaidTotal)
	if !ok {
		return 0
	}
	return coin
}

func roundShopeeRepairMoney(v float64) float64 {
	return math.Round(v*100) / 100
}

func (s *ShopeeEmailRepairService) applyLazadaEmailRepairOrders(
	emailHandler *EmailHandler,
	target shopeeRepairTarget,
	body shopeeRepairEmailBody,
	missingOrderIDs []string,
	rebuildOrderIDs []string,
	progress func(EmailRepairJobProgress),
) (appliedShopeePaymentRepair, error) {
	result := appliedShopeePaymentRepair{}
	plainText := prepareLazadaEmailText(body.Text, body.HTML)
	if strings.TrimSpace(plainText) == "" {
		return result, badShopeeRepairRequest("ไม่พบเนื้อหาอีเมล Lazada สำหรับซ่อม")
	}
	total := len(missingOrderIDs) + len(rebuildOrderIDs)
	reportRepairProgress(progress, 42, "extract", "อ่านข้อมูลคำสั่งซื้อจากอีเมล Lazada", 0, total, "")
	releaseSlot := acquireLazadaAISlot()
	orders, err := emailHandler.aiClient.ExtractLazadaOrders(plainText)
	releaseSlot()
	if err != nil || len(orders) == 0 {
		if err == nil {
			err = fmt.Errorf("AI extract lazada_email: empty orders")
		}
		return result, err
	}
	fallbackOrderID := normalizeLazadaOrderID(extractLazadaOrderID(target.Subject + "\n" + plainText))
	ordersByID := map[string]ai.ExtractedOrder{}
	for _, order := range orders {
		if strings.TrimSpace(order.OrderID) == "" && fallbackOrderID != "" {
			order.OrderID = fallbackOrderID
		}
		orderID := normalizeLazadaOrderID(order.OrderID)
		if orderID == "" {
			continue
		}
		order.OrderID = orderID
		ordersByID[orderID] = order
	}
	source := mailSourceFromRepairRaw(target.Raw)
	traceID := fmt.Sprintf("lazada-email-repair-%d", time.Now().UnixMilli())
	start := time.Now()
	processed := 0
	for _, orderID := range missingOrderIDs {
		reportRepairProgress(progress, repairOrderPercent(processed, total), "create_order", "กำลังสร้างบิล Lazada ที่ตกหล่น", processed, total, orderID)
		order, ok := ordersByID[orderID]
		if !ok {
			return result, fmt.Errorf("AI extract missing Lazada order %s", orderID)
		}
		if _, err := emailHandler.processOneLazadaEmailOrder(order, target.Subject, target.FromAddr, plainText, body.HTML, target.MessageID, traceID, start, source); err != nil {
			return result, err
		}
		processed++
		reportRepairProgress(progress, repairOrderPercent(processed, total), "create_order", "สร้างบิล Lazada ที่ตกหล่นแล้ว", processed, total, orderID)
	}
	for _, orderID := range rebuildOrderIDs {
		reportRepairProgress(progress, repairOrderPercent(processed, total), "rebuild_order", "กำลังซ่อมบิล Lazada จากอีเมลยืนยัน", processed, total, orderID)
		order, ok := ordersByID[orderID]
		if !ok {
			return result, fmt.Errorf("AI extract missing Lazada rebuild order %s", orderID)
		}
		billID, err := s.rebuildOneLazadaBillFromConfirmation(emailHandler, order, target, plainText, body.HTML, traceID, start, source)
		if err != nil {
			return result, err
		}
		if billID != "" {
			result.rebuilt = append(result.rebuilt, createdRepairBill{ID: billID, OrderID: orderID})
		}
		processed++
		reportRepairProgress(progress, repairOrderPercent(processed, total), "rebuild_order", "ซ่อมบิล Lazada แล้ว", processed, total, orderID)
	}
	if target.MessageID != "" {
		_ = s.billRepo.MarkProcessedEmailKey(lazadaEmailSource, target.MessageID, "")
	}
	return result, nil
}

func (s *ShopeeEmailRepairService) rebuildOneLazadaBillFromConfirmation(
	emailHandler *EmailHandler,
	order ai.ExtractedOrder,
	target shopeeRepairTarget,
	plainText string,
	bodyHTML string,
	traceID string,
	startTime time.Time,
	source emailservice.MailSource,
) (string, error) {
	orderID := normalizeLazadaOrderID(order.OrderID)
	if orderID == "" {
		return "", fmt.Errorf("rebuild lazada bill: empty order id")
	}
	validItems := make([]ai.ExtractedItem, 0, len(order.Items))
	for _, extItem := range order.Items {
		extItem.RawName = strings.TrimSpace(extItem.RawName)
		if extItem.RawName == "" || extItem.Qty <= 0 {
			continue
		}
		validItems = append(validItems, extItem)
	}
	if len(validItems) == 0 {
		return "", fmt.Errorf("rebuild lazada bill %s: no usable items", orderID)
	}
	validItems = attachLazadaItemImages(validItems, bodyHTML)
	amountSummary := repository.ExtractLazadaAmountSummary(plainText, bodyHTML)
	validItems = applyLazadaEmailSummaryPrices(validItems, amountSummary)
	itemsGrossDelta := lazadaExtractedItemsGrossDelta(validItems, amountSummary)
	itemsWithCandidates, allHighConfidence := emailHandler.mapLazadaItems(validItems)
	if len(itemsWithCandidates) == 0 {
		return "", fmt.Errorf("rebuild lazada bill %s: no mapped item rows", orderID)
	}
	configReady := emailHandler.lazadaEmailChannelReady()
	feeConfigReady := emailHandler.lazadaEmailFeeConfigReady(amountSummary)
	amountReady := amountSummary.ReconciliationStatus == repository.LazadaReconciliationOK
	if amountSummary.HasGoodsTotalAmount && itemsGrossDelta != nil && absFloat(*itemsGrossDelta) > 0.01 {
		amountReady = false
	}
	applyLazadaEmailDiscounts(itemsWithCandidates, amountSummary)
	feeLineAdded := false
	feeLineReady := true
	if feeAmount := amountSummary.FeeAmount(); feeAmount > 0 {
		feeItem, ready := emailHandler.configuredLazadaFeeLine(orderID, feeAmount, true)
		if feeItem != nil {
			itemsWithCandidates = append(itemsWithCandidates, lazadaItemWithCandidates{item: *feeItem})
			feeLineAdded = true
			feeLineReady = ready
		} else {
			feeLineReady = false
		}
	}
	status := "needs_review"
	if allHighConfidence && configReady && feeConfigReady && feeLineReady && amountReady && order.Confidence >= lazadaHighConfThreshold {
		status = "pending"
	}
	docDate := normalizeLazadaEmailDocDate(order.DocDate, source)
	sellerName := resolveLazadaEmailSellerName(order.SellerName, plainText, bodyHTML)
	rawDataMap := map[string]interface{}{
		"subject":          target.Subject,
		"from":             target.FromAddr,
		"email_message_id": target.MessageID,
		"order_id":         orderID,
		"lazada_order_id":  orderID,
		"seller_name":      sellerName,
		"items":            validItems,
		"flow":             lazadaEmailFlow,
		"doc_date":         docDate,
		"body_excerpt":     truncateRunes(plainText, lazadaBodyExcerptRunes),
		"ai_text_chars":    len([]rune(plainText)),
		"config_ready":     configReady,
		"fee_config_ready": feeConfigReady,
		"fee_line_added":   feeLineAdded,
		"repair_source":    "lazada_confirmation_rebuild",
	}
	applyLazadaAmountSummaryRawData(rawDataMap, amountSummary, itemsGrossDelta)
	if amountSummary.HasPaidTotalAmount {
		rawDataMap["total_amount"] = amountSummary.PaidTotalAmount
	} else if order.TotalAmount != nil {
		rawDataMap["total_amount"] = *order.TotalAmount
	}
	applyMailSource(rawDataMap, source)
	if confirmedAt, groupKey := ExtractLazadaConfirmedAt(plainText, bodyHTML, source.AccountID); confirmedAt != "" {
		rawDataMap["lazada_confirmed_at"] = confirmedAt
		rawDataMap["lazada_charge_group_key"] = groupKey
	}
	rawDataBytes, _ := json.Marshal(rawDataMap)
	conf := order.Confidence
	billID, err := s.replaceLazadaRepairBill(orderID, status, conf, rawDataBytes, itemsWithCandidates)
	if err != nil {
		return "", err
	}
	emailHandler.saveLazadaEmailArtifacts(billID, target.Subject, target.FromAddr, plainText, bodyHTML, target.MessageID)
	if target.MessageID != "" {
		_ = s.billRepo.MarkProcessedEmailKey(lazadaEmailSource, target.MessageID, orderID)
	}
	if s.auditRepo != nil {
		durMs := int(time.Since(startTime).Milliseconds())
		_ = s.auditRepo.Log(models.AuditEntry{
			Action:     "lazada_email_repair_rebuilt_bill",
			TargetID:   &billID,
			Source:     lazadaEmailSource,
			Level:      "info",
			TraceID:    traceID,
			DurationMs: &durMs,
			Detail: map[string]interface{}{
				"message_id":     target.MessageID,
				"order_id":       orderID,
				"seller_name":    sellerName,
				"items_count":    len(itemsWithCandidates),
				"status":         status,
				"amount_status":  amountSummary.ReconciliationStatus,
				"fee_line_added": feeLineAdded,
			},
		})
	}
	return billID, nil
}

func (s *ShopeeEmailRepairService) replaceLazadaRepairBill(orderID, status string, confidence float64, rawData []byte, items []lazadaItemWithCandidates) (string, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()

	var billID, currentStatus, smlDocNo string
	var sent bool
	err = tx.QueryRow(
		`SELECT id::text, status, COALESCE(sml_doc_no,''), sent_at IS NOT NULL
		   FROM bills
		  WHERE source = $2
		    AND bill_type = 'purchase'
		    AND archived_at IS NULL
		    AND UPPER(TRIM(COALESCE(NULLIF(raw_data->>'order_id',''), NULLIF(raw_data->>'lazada_order_id',''), sml_order_id, ''))) = $1
		  FOR UPDATE`,
		orderID, lazadaEmailSource,
	).Scan(&billID, &currentStatus, &smlDocNo, &sent)
	if err == sql.ErrNoRows {
		return "", badShopeeRepairRequest("ไม่พบบิล Lazada เดิมสำหรับซ่อม order " + orderID)
	}
	if err != nil {
		return "", err
	}
	info := lazadaExistingRepairBill{ID: billID, OrderID: orderID, Status: currentStatus, SMLDocNo: smlDocNo, Sent: sent}
	if !info.CanRebuildFromConfirmation() {
		return "", badShopeeRepairRequest("บิล Lazada " + orderID + " ส่ง SML แล้วหรือไม่อยู่ในสถานะที่ซ่อมอัตโนมัติได้")
	}
	if _, err := tx.Exec(`DELETE FROM bill_items WHERE bill_id = $1::uuid`, billID); err != nil {
		return "", err
	}
	if _, err := tx.Exec(
		`UPDATE bills
		    SET raw_data = $2::jsonb,
		        status = $3,
		        ai_confidence = $4,
		        sml_order_id = $5,
		        error_msg = NULL
		  WHERE id = $1::uuid`,
		billID, string(rawData), status, confidence, orderID,
	); err != nil {
		return "", err
	}
	for _, iwc := range items {
		item := iwc.item
		candidatesJSON, _ := json.Marshal(iwc.candidates)
		if _, err := tx.Exec(
			`INSERT INTO bill_items (bill_id, raw_name, source_sku, source_image_url, item_code, qty, unit_code, price, discount_amount, mapped, mapping_id, candidates)
			 VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
			billID, item.RawName, item.SourceSKU, item.SourceImageURL, item.ItemCode, item.Qty,
			item.UnitCode, item.Price, item.DiscountAmount, item.Mapped, item.MappingID, candidatesJSON,
		); err != nil {
			return "", err
		}
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return billID, nil
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

func (s *ShopeeEmailRepairService) loadTarget(billID string, inputSubject string) (shopeeRepairTarget, error) {
	bill, err := s.billRepo.FindByID(strings.TrimSpace(billID))
	if err != nil {
		return shopeeRepairTarget{}, err
	}
	if bill == nil {
		return shopeeRepairTarget{}, notFoundShopeeRepair("ไม่พบบิลที่ต้องการ")
	}
	if (bill.Source != "shopee_shipped" && bill.Source != lazadaEmailSource) || bill.BillType != "purchase" {
		return shopeeRepairTarget{}, badShopeeRepairRequest("เครื่องมือนี้ใช้กับบิลซื้อ Shopee/Lazada จากอีเมลเท่านั้น")
	}
	if bill.ArchivedAt != nil {
		return shopeeRepairTarget{}, badShopeeRepairRequest("บิลนี้ถูกเก็บแล้ว จึงซ่อมจากอีเมลไม่ได้")
	}
	raw := map[string]interface{}{}
	if len(bill.RawData) > 0 {
		_ = json.Unmarshal(bill.RawData, &raw)
	}
	target := shopeeRepairTarget{
		BillID:       bill.ID,
		Source:       bill.Source,
		Subject:      repairStringField(raw, "subject"),
		InputSubject: strings.TrimSpace(inputSubject),
		FromAddr:     repairStringField(raw, "from"),
		MessageID:    repairStringField(raw, "email_message_id"),
		Raw:          raw,
	}
	if err := s.applyRepairArtifactTarget(&target); err != nil {
		return shopeeRepairTarget{}, err
	}
	if target.MessageID == "" {
		return shopeeRepairTarget{}, badShopeeRepairRequest("อีเมลที่เลือกไม่มี message id สำหรับป้องกัน duplicate")
	}
	if target.Source == "shopee_shipped" {
		eventType, _, _, ok := shopeeOrderEventFromSubject(target.Subject)
		if !ok || eventType != shopeeEventPaymentConfirmed {
			return shopeeRepairTarget{}, badShopeeRepairRequest("ซ่อมได้เฉพาะหัวข้ออีเมล Shopee ยืนยันการชำระเงินเท่านั้น")
		}
		return target, nil
	}
	if target.Source == lazadaEmailSource {
		if !isLazadaRepairConfirmationSubject(target.Subject) {
			return shopeeRepairTarget{}, badShopeeRepairRequest("ซ่อมได้เฉพาะหัวข้ออีเมล Lazada ยืนยันคำสั่งซื้อเท่านั้น")
		}
		return target, nil
	}
	return target, nil
}

func (s *ShopeeEmailRepairService) applyRepairArtifactTarget(target *shopeeRepairTarget) error {
	if target == nil {
		return nil
	}
	artifacts, err := s.artifactSvc.ListByBill(target.BillID)
	if err != nil {
		return err
	}
	inputSubject := strings.TrimSpace(target.InputSubject)
	if inputSubject != "" {
		for _, a := range artifacts {
			if !isRepairEmailArtifact(a) {
				continue
			}
			if !sameRepairSubject(inputSubject, artifactRepairSubject(a)) {
				continue
			}
			target.Subject = artifactRepairSubject(a)
			target.FromAddr = repairFirstNonEmpty(artifactRepairFrom(a), target.FromAddr)
			target.MessageID = repairFirstNonEmpty(artifactRepairMessageID(a), target.MessageID)
			target.ArtifactID = a.ID
			return nil
		}
		if sameRepairSubject(inputSubject, target.Subject) {
			return nil
		}
		a, err := s.findRepairArtifactBySubject(target.Source, inputSubject)
		if err != nil {
			return err
		}
		if a != nil {
			target.Subject = artifactRepairSubject(*a)
			target.FromAddr = repairFirstNonEmpty(artifactRepairFrom(*a), target.FromAddr)
			target.MessageID = repairFirstNonEmpty(artifactRepairMessageID(*a), target.MessageID)
			target.ArtifactID = a.ID
			return nil
		}
		return badShopeeRepairRequest("ไม่พบอีเมลต้นฉบับที่ตรงกับหัวข้อที่ระบุใน BillFlow")
	}
	if target.Source == "shopee_shipped" {
		eventType, _, _, ok := shopeeOrderEventFromSubject(target.Subject)
		if ok && eventType == shopeeEventPaymentConfirmed {
			return nil
		}
	} else if target.Source == lazadaEmailSource {
		if isLazadaRepairConfirmationSubject(target.Subject) {
			return nil
		}
	}
	for _, a := range artifacts {
		if !isRepairEmailArtifact(a) {
			continue
		}
		subject := artifactRepairSubject(a)
		if target.Source == "shopee_shipped" {
			eventType, _, _, ok := shopeeOrderEventFromSubject(subject)
			if !ok || eventType != shopeeEventPaymentConfirmed {
				continue
			}
		} else if target.Source == lazadaEmailSource {
			if !isLazadaRepairConfirmationSubject(subject) {
				continue
			}
		} else {
			continue
		}
		target.Subject = subject
		target.FromAddr = repairFirstNonEmpty(artifactRepairFrom(a), target.FromAddr)
		target.MessageID = repairFirstNonEmpty(artifactRepairMessageID(a), target.MessageID)
		target.ArtifactID = a.ID
		return nil
	}
	return nil
}

func (s *ShopeeEmailRepairService) findRepairArtifactBySubject(source, subject string) (*models.BillArtifact, error) {
	source = strings.TrimSpace(source)
	subject = strings.TrimSpace(subject)
	if source == "" || subject == "" {
		return nil, nil
	}
	rows, err := s.db.Query(
		`SELECT a.id, a.bill_id, a.kind, a.filename, COALESCE(a.content_type,''), a.size_bytes,
		        COALESCE(a.sha256,''), a.storage_path, COALESCE(a.source_meta, '{}'::jsonb), a.created_at
		   FROM bill_artifacts a
		   JOIN bills b ON b.id = a.bill_id
		  WHERE b.source = $1
		    AND b.bill_type = 'purchase'
		    AND b.archived_at IS NULL
		    AND a.kind IN ('email_html','email_text')
		    AND COALESCE(a.source_meta->>'subject','') = $2
		  ORDER BY a.created_at DESC, a.id DESC
		  LIMIT 2`,
		source, subject,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var found []models.BillArtifact
	for rows.Next() {
		var a models.BillArtifact
		if err := rows.Scan(&a.ID, &a.BillID, &a.Kind, &a.Filename, &a.ContentType, &a.SizeBytes, &a.SHA256, &a.StoragePath, &a.SourceMeta, &a.CreatedAt); err != nil {
			return nil, err
		}
		found = append(found, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(found) == 0 {
		return nil, nil
	}
	if len(found) > 1 {
		return nil, badShopeeRepairRequest("พบอีเมลหัวข้อนี้มากกว่า 1 ฉบับ กรุณาเปิดจากบิลที่มีหลักฐานอีเมลหรือระบุหัวข้อให้เฉพาะเจาะจงกว่าเดิม")
	}
	return &found[0], nil
}

func (s *ShopeeEmailRepairService) loadEmailBody(target shopeeRepairTarget) (shopeeRepairEmailBody, error) {
	artifacts, err := s.artifactSvc.ListByBill(target.BillID)
	if err != nil {
		return shopeeRepairEmailBody{}, err
	}
	if target.ArtifactID != "" {
		data, a, err := s.artifactSvc.Read(target.ArtifactID)
		if err == nil && a != nil && len(data) > 0 {
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
	if target.Source == lazadaEmailSource {
		return s.inspectLazadaTarget(target, body)
	}
	plainText := htmlToText(body.Text)
	if strings.TrimSpace(plainText) == "" {
		plainText = htmlToText(body.HTML)
	}
	orderIDs := DetectShopeeBodyOrderIDs(plainText, body.HTML)
	if len(orderIDs) == 0 {
		return ShopeeEmailRepairPreview{}, badShopeeRepairRequest("อ่านเลขคำสั่งซื้อจากอีเมลต้นฉบับไม่ได้")
	}
	existing, err := s.existingShopeeBillInfos(orderIDs)
	if err != nil {
		return ShopeeEmailRepairPreview{}, err
	}
	missing := []string{}
	existingOrderIDs := []string{}
	rebuildOrderIDs := []string{}
	blockedOrderIDs := []string{}
	for _, orderID := range orderIDs {
		info, ok := existing[orderID]
		if !ok || info.ID == "" {
			missing = append(missing, orderID)
		} else {
			existingOrderIDs = append(existingOrderIDs, orderID)
			if info.CreatedFromShipping() {
				if info.CanRebuildFromPayment() {
					rebuildOrderIDs = append(rebuildOrderIDs, orderID)
				} else {
					blockedOrderIDs = append(blockedOrderIDs, orderID)
				}
			}
		}
	}
	stale, err := s.staleTombstones(target.Source, target.MessageID, missing)
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
	if len(rebuildOrderIDs) > 0 {
		warnings = append(warnings, "พบบิลเดิมที่สร้างจากอีเมลจัดส่ง ระบบจะใช้ข้อมูลจากอีเมลยืนยันการชำระเงินซ่อมเฉพาะบิลที่ยังไม่ส่ง SML")
	}
	if len(blockedOrderIDs) > 0 {
		warnings = append(warnings, "มีบิลที่สร้างจากอีเมลจัดส่งแต่ส่ง SML แล้วหรือแก้ไม่ได้อัตโนมัติ: "+strings.Join(blockedOrderIDs, ", "))
	}
	return ShopeeEmailRepairPreview{
		BillID:                 target.BillID,
		Source:                 target.Source,
		MessageID:              target.MessageID,
		ArtifactID:             body.ArtifactID,
		Subject:                target.Subject,
		InputSubject:           target.InputSubject,
		DetectedOrderCount:     len(orderIDs),
		ExistingCount:          len(existingOrderIDs),
		MissingCount:           len(missing),
		RebuildCount:           len(rebuildOrderIDs),
		BlockedCount:           len(blockedOrderIDs),
		DetectedOrderIDs:       orderIDs,
		ExistingOrderIDs:       existingOrderIDs,
		MissingOrderIDs:        missing,
		RebuildOrderIDs:        rebuildOrderIDs,
		BlockedOrderIDs:        blockedOrderIDs,
		EmailTotal:             total,
		HasStaleTombstones:     len(stale) > 0,
		StaleTombstoneOrderIDs: stale,
		Warnings:               warnings,
		CanRepair:              (len(missing) > 0 || len(rebuildOrderIDs) > 0) && total > 0,
	}, nil
}

func (s *ShopeeEmailRepairService) inspectLazadaTarget(target shopeeRepairTarget, body shopeeRepairEmailBody) (ShopeeEmailRepairPreview, error) {
	plainText := prepareLazadaEmailText(body.Text, body.HTML)
	if strings.TrimSpace(plainText) == "" {
		return ShopeeEmailRepairPreview{}, badShopeeRepairRequest("ไม่พบเนื้อหาอีเมล Lazada สำหรับตรวจสอบ")
	}
	orderID := normalizeLazadaOrderID(extractLazadaOrderID(target.Subject + "\n" + plainText))
	if orderID == "" {
		return ShopeeEmailRepairPreview{}, badShopeeRepairRequest("อ่านเลขคำสั่งซื้อ Lazada จากอีเมลต้นฉบับไม่ได้")
	}
	orderIDs := []string{orderID}
	existing, err := s.existingLazadaBillInfos(orderIDs)
	if err != nil {
		return ShopeeEmailRepairPreview{}, err
	}
	missing := []string{}
	existingOrderIDs := []string{}
	rebuildOrderIDs := []string{}
	blockedOrderIDs := []string{}
	if info, ok := existing[orderID]; !ok || info.ID == "" {
		missing = append(missing, orderID)
	} else {
		existingOrderIDs = append(existingOrderIDs, orderID)
		if info.CanRebuildFromConfirmation() {
			rebuildOrderIDs = append(rebuildOrderIDs, orderID)
		} else if info.NeedsManualRepairBlock() {
			blockedOrderIDs = append(blockedOrderIDs, orderID)
		}
	}
	stale, err := s.staleTombstones(target.Source, target.MessageID, missing)
	if err != nil {
		return ShopeeEmailRepairPreview{}, err
	}
	summary := repository.ExtractLazadaAmountSummary(plainText, body.HTML)
	total := 0.0
	warnings := []string{}
	if summary.HasPaidTotalAmount {
		total = math.Round(summary.PaidTotalAmount*100) / 100
	} else {
		warnings = append(warnings, "อ่านยอดรวมที่ชำระจากอีเมล Lazada ไม่ได้")
	}
	if len(stale) > 0 {
		warnings = append(warnings, "พบประวัติ processed เก่าของรายการที่ยังไม่มีบิล ระบบจะล้างเฉพาะรายการที่ตกหล่นตอนซ่อม")
	}
	if len(rebuildOrderIDs) > 0 {
		warnings = append(warnings, "พบบิล Lazada เดิมที่ยังไม่ส่ง SML ระบบจะใช้ข้อมูลจากอีเมลยืนยันซ่อมบิลเดิม")
	}
	if len(blockedOrderIDs) > 0 {
		warnings = append(warnings, "มีบิล Lazada ที่ส่ง SML แล้วหรือแก้ไม่ได้อัตโนมัติ: "+strings.Join(blockedOrderIDs, ", "))
	}
	return ShopeeEmailRepairPreview{
		BillID:                 target.BillID,
		Source:                 target.Source,
		MessageID:              target.MessageID,
		ArtifactID:             body.ArtifactID,
		Subject:                target.Subject,
		InputSubject:           target.InputSubject,
		DetectedOrderCount:     len(orderIDs),
		ExistingCount:          len(existingOrderIDs),
		MissingCount:           len(missing),
		RebuildCount:           len(rebuildOrderIDs),
		BlockedCount:           len(blockedOrderIDs),
		DetectedOrderIDs:       orderIDs,
		ExistingOrderIDs:       existingOrderIDs,
		MissingOrderIDs:        missing,
		RebuildOrderIDs:        rebuildOrderIDs,
		BlockedOrderIDs:        blockedOrderIDs,
		EmailTotal:             total,
		HasStaleTombstones:     len(stale) > 0,
		StaleTombstoneOrderIDs: stale,
		Warnings:               warnings,
		CanRepair:              (len(missing) > 0 || len(rebuildOrderIDs) > 0) && total > 0,
	}, nil
}

type shopeeExistingRepairBill struct {
	ID       string
	OrderID  string
	Status   string
	Subject  string
	SMLDocNo string
	Sent     bool
}

func (b shopeeExistingRepairBill) CreatedFromShipping() bool {
	eventType, _, _, ok := shopeeOrderEventFromSubject(b.Subject)
	return ok && eventType == shopeeEventShipped
}

func (b shopeeExistingRepairBill) CanRebuildFromPayment() bool {
	if !b.CreatedFromShipping() {
		return false
	}
	if strings.TrimSpace(b.SMLDocNo) != "" || b.Sent {
		return false
	}
	switch b.Status {
	case "needs_review", "pending", "failed":
		return true
	default:
		return false
	}
}

func (s *ShopeeEmailRepairService) existingShopeeBillInfos(orderIDs []string) (map[string]shopeeExistingRepairBill, error) {
	out := map[string]shopeeExistingRepairBill{}
	if len(orderIDs) == 0 {
		return out, nil
	}
	rows, err := s.db.Query(
		`SELECT id::text,
		        UPPER(TRIM(LEADING '#' FROM COALESCE(NULLIF(raw_data->>'order_id',''), sml_order_id, ''))) AS order_id,
		        status,
		        COALESCE(raw_data->>'subject','') AS subject,
		        COALESCE(sml_doc_no,'') AS sml_doc_no,
		        sent_at IS NOT NULL AS sent
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
		var b shopeeExistingRepairBill
		if err := rows.Scan(&b.ID, &b.OrderID, &b.Status, &b.Subject, &b.SMLDocNo, &b.Sent); err != nil {
			return nil, err
		}
		out[b.OrderID] = b
	}
	return out, rows.Err()
}

func (s *ShopeeEmailRepairService) existingShopeeBills(orderIDs []string) (map[string]string, error) {
	infos, err := s.existingShopeeBillInfos(orderIDs)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for orderID, info := range infos {
		out[orderID] = info.ID
	}
	return out, nil
}

type lazadaExistingRepairBill struct {
	ID       string
	OrderID  string
	Status   string
	Subject  string
	SMLDocNo string
	Sent     bool
}

func (b lazadaExistingRepairBill) CanRebuildFromConfirmation() bool {
	if strings.TrimSpace(b.SMLDocNo) != "" || b.Sent {
		return false
	}
	switch b.Status {
	case "needs_review", "pending", "failed":
		return true
	default:
		return false
	}
}

func (b lazadaExistingRepairBill) NeedsManualRepairBlock() bool {
	if b.ID == "" {
		return false
	}
	return !b.CanRebuildFromConfirmation()
}

func (s *ShopeeEmailRepairService) existingLazadaBillInfos(orderIDs []string) (map[string]lazadaExistingRepairBill, error) {
	out := map[string]lazadaExistingRepairBill{}
	if len(orderIDs) == 0 {
		return out, nil
	}
	rows, err := s.db.Query(
		`SELECT id::text,
		        UPPER(TRIM(COALESCE(NULLIF(raw_data->>'order_id',''), NULLIF(raw_data->>'lazada_order_id',''), sml_order_id, ''))) AS order_id,
		        status,
		        COALESCE(raw_data->>'subject','') AS subject,
		        COALESCE(sml_doc_no,'') AS sml_doc_no,
		        sent_at IS NOT NULL AS sent
		   FROM bills
		  WHERE source = $2
		    AND bill_type = 'purchase'
		    AND archived_at IS NULL
		    AND UPPER(TRIM(COALESCE(NULLIF(raw_data->>'order_id',''), NULLIF(raw_data->>'lazada_order_id',''), sml_order_id, ''))) = ANY($1)`,
		pq.Array(orderIDs), lazadaEmailSource,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var b lazadaExistingRepairBill
		if err := rows.Scan(&b.ID, &b.OrderID, &b.Status, &b.Subject, &b.SMLDocNo, &b.Sent); err != nil {
			return nil, err
		}
		out[b.OrderID] = b
	}
	return out, rows.Err()
}

func (s *ShopeeEmailRepairService) staleTombstones(source, messageID string, missingOrderIDs []string) ([]string, error) {
	out := []string{}
	source = strings.TrimSpace(source)
	if source == "" || strings.TrimSpace(messageID) == "" || len(missingOrderIDs) == 0 {
		return out, nil
	}
	rows, err := s.db.Query(
		`SELECT DISTINCT UPPER(TRIM(LEADING '#' FROM COALESCE(order_id, ''))) AS order_id
		   FROM processed_email_keys
		  WHERE source = $3
		    AND message_id = $1
		    AND UPPER(TRIM(LEADING '#' FROM COALESCE(order_id, ''))) = ANY($2)
		  ORDER BY order_id`,
		messageID, pq.Array(missingOrderIDs), source,
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

func (s *ShopeeEmailRepairService) clearStaleTombstones(source, messageID string, missingOrderIDs []string) ([]string, error) {
	out := []string{}
	source = strings.TrimSpace(source)
	if source == "" || strings.TrimSpace(messageID) == "" || len(missingOrderIDs) == 0 {
		return out, nil
	}
	rows, err := s.db.Query(
		`DELETE FROM processed_email_keys pek
		  WHERE pek.source = $3
		    AND pek.message_id = $1
		    AND UPPER(TRIM(LEADING '#' FROM COALESCE(pek.order_id, ''))) = ANY($2)
		    AND NOT EXISTS (
		      SELECT 1 FROM bills b
		       WHERE b.source = $3
		         AND b.bill_type = 'purchase'
		         AND b.archived_at IS NULL
		         AND UPPER(TRIM(LEADING '#' FROM COALESCE(NULLIF(b.raw_data->>'order_id',''), NULLIF(b.raw_data->>'lazada_order_id',''), b.sml_order_id, ''))) =
		             UPPER(TRIM(LEADING '#' FROM COALESCE(pek.order_id, '')))
		    )
		  RETURNING UPPER(TRIM(LEADING '#' FROM COALESCE(order_id, ''))) AS order_id`,
		messageID, pq.Array(missingOrderIDs), source,
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

func (s *ShopeeEmailRepairService) createdBillsForOrders(source string, orderIDs []string) ([]createdRepairBill, error) {
	out := []createdRepairBill{}
	if len(orderIDs) == 0 {
		return out, nil
	}
	rows, err := s.db.Query(
		`SELECT id::text,
		        UPPER(TRIM(LEADING '#' FROM COALESCE(NULLIF(raw_data->>'order_id',''), NULLIF(raw_data->>'lazada_order_id',''), sml_order_id, ''))) AS order_id
		   FROM bills
		  WHERE source = $2
		    AND bill_type = 'purchase'
		    AND archived_at IS NULL
		    AND UPPER(TRIM(LEADING '#' FROM COALESCE(NULLIF(raw_data->>'order_id',''), NULLIF(raw_data->>'lazada_order_id',''), sml_order_id, ''))) = ANY($1)
		  ORDER BY created_at, id`,
		pq.Array(orderIDs), source,
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
	source := strings.TrimSpace(preview.Source)
	if source == "" {
		source = "shopee_shipped"
	}
	if err := s.expireStaleActiveJobs(source, preview.MessageID); err != nil {
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
		 VALUES ($1::uuid, $2, $3, 'queued', $4::jsonb, $5::uuid, $6)
		 RETURNING id::text`,
		preview.BillID, source, preview.MessageID, string(snapshot), createdBy, strings.TrimSpace(userEmail),
	).Scan(&jobID)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			active, getErr := s.findActiveJob(source, preview.MessageID)
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

func (s *ShopeeEmailRepairService) expireStaleActiveJobs(source, messageID string) error {
	_, err := s.db.Exec(
		`UPDATE email_repair_jobs
		    SET status = 'failed',
		        error = 'repair job timed out before completion; please preview and start again',
		        finished_at = now(),
		        updated_at = now()
		  WHERE source = $1
		    AND message_id = $2
		    AND status IN ('queued','running')
		    AND created_at < now() - interval '30 minutes'`,
		source, messageID,
	)
	return err
}

func (s *ShopeeEmailRepairService) findActiveJob(source, messageID string) (*ShopeeEmailRepairJob, error) {
	var id string
	err := s.db.QueryRow(
		`SELECT id::text
		   FROM email_repair_jobs
		  WHERE source = $1
		    AND message_id = $2
		    AND status IN ('queued','running')
		  ORDER BY created_at DESC
		  LIMIT 1`,
		source, messageID,
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
	job.RebuiltCount = job.Result.RebuiltCount
	job.SkippedCount = job.Result.SkippedCount
	job.MissingCount = job.Result.MissingCount
	job.CreatedBillIDs = job.Result.CreatedBillIDs
	job.CreatedOrderIDs = job.Result.CreatedOrderIDs
	job.RebuiltBillIDs = job.Result.RebuiltBillIDs
	job.RebuiltOrderIDs = job.Result.RebuiltOrderIDs
	job.MissingOrderIDs = job.Result.MissingOrderIDs
	job.Progress = job.Result.Progress
	if job.Progress.Percent == 0 {
		switch job.Status {
		case "queued":
			job.Progress = EmailRepairJobProgress{Percent: 0, Stage: "queued", Label: "รอเริ่มงานซ่อม"}
		case "running":
			job.Progress = EmailRepairJobProgress{Percent: 5, Stage: "started", Label: "เริ่มงานซ่อมจากอีเมลยืนยัน"}
		case "succeeded":
			job.Progress = EmailRepairJobProgress{Percent: 100, Stage: "succeeded", Label: "ซ่อมคำสั่งซื้อจากอีเมลเสร็จแล้ว"}
		case "failed":
			job.Progress = EmailRepairJobProgress{Percent: 100, Stage: "failed", Label: "งานซ่อมไม่สำเร็จ"}
		}
	}
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
	if result.Progress.Percent == 0 {
		result.Progress = EmailRepairJobProgress{Percent: 100, Stage: "succeeded", Label: "ซ่อมคำสั่งซื้อจากอีเมลเสร็จแล้ว"}
	}
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
	s.updateJobProgress(jobID, EmailRepairJobProgress{Percent: 100, Stage: "failed", Label: "งานซ่อมไม่สำเร็จ"})
	_, err := s.db.Exec(
		`UPDATE email_repair_jobs
		    SET status = 'failed', error = $2, finished_at = now(), updated_at = now()
		  WHERE id = $1::uuid`,
		jobID, message,
	)
	return err
}

func (s *ShopeeEmailRepairService) updateJobProgress(jobID string, progress EmailRepairJobProgress) {
	if s == nil || s.db == nil || strings.TrimSpace(jobID) == "" {
		return
	}
	progress.Percent = clampRepairPercent(progress.Percent)
	raw, _ := json.Marshal(map[string]interface{}{"progress": progress})
	if _, err := s.db.Exec(
		`UPDATE email_repair_jobs
		    SET result = COALESCE(result, '{}'::jsonb) || $2::jsonb,
		        updated_at = now()
		  WHERE id = $1::uuid
		    AND status IN ('queued','running')`,
		jobID, string(raw),
	); err != nil {
		s.logWarn("shopee_email_repair: update progress failed", zap.String("job_id", jobID), zap.Error(err))
	}
}

func reportRepairProgress(progress func(EmailRepairJobProgress), percent int, stage, label string, current, total int, orderID string) {
	if progress == nil {
		return
	}
	progress(EmailRepairJobProgress{
		Percent:        clampRepairPercent(percent),
		Stage:          stage,
		Label:          label,
		Current:        current,
		Total:          total,
		CurrentOrderID: orderID,
	})
}

func repairOrderPercent(current, total int) int {
	if total <= 0 {
		return 80
	}
	if current < 0 {
		current = 0
	}
	if current > total {
		current = total
	}
	return 55 + int(math.Round((float64(current)/float64(total))*35))
}

func clampRepairPercent(percent int) int {
	if percent < 0 {
		return 0
	}
	if percent > 100 {
		return 100
	}
	return percent
}

func (s *ShopeeEmailRepairService) auditJob(action, source, billID, jobID, level string, detail map[string]interface{}) {
	if s.handler == nil || s.auditRepo == nil {
		return
	}
	if detail == nil {
		detail = map[string]interface{}{}
	}
	detail["job_id"] = jobID
	source = strings.TrimSpace(source)
	if source == "" {
		source = "shopee_shipped"
	}
	var targetID *string
	if billID != "" {
		targetID = &billID
	}
	_ = s.auditRepo.Log(models.AuditEntry{
		Action:   action,
		TargetID: targetID,
		Source:   source,
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
	source := "shopee_shipped"
	if detail != nil {
		if v, ok := detail["source"].(string); ok && strings.TrimSpace(v) != "" {
			source = strings.TrimSpace(v)
		}
	}
	_ = h.auditRepo.Log(models.AuditEntry{
		Action:   action,
		TargetID: &targetID,
		UserID:   userID,
		Source:   source,
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

func artifactRepairSubject(a models.BillArtifact) string {
	if len(a.SourceMeta) == 0 {
		return ""
	}
	var meta struct {
		Subject string `json:"subject"`
	}
	if err := json.Unmarshal(a.SourceMeta, &meta); err != nil {
		return ""
	}
	return strings.TrimSpace(meta.Subject)
}

func artifactRepairFrom(a models.BillArtifact) string {
	if len(a.SourceMeta) == 0 {
		return ""
	}
	var meta struct {
		From string `json:"from"`
	}
	if err := json.Unmarshal(a.SourceMeta, &meta); err != nil {
		return ""
	}
	return strings.TrimSpace(meta.From)
}

func isRepairEmailArtifact(a models.BillArtifact) bool {
	return a.Kind == "email_html" || a.Kind == "email_text"
}

func isLazadaRepairConfirmationSubject(subject string) bool {
	subject = strings.TrimSpace(subject)
	return strings.Contains(subject, "ยืนยันคำสั่งซื้อ") && normalizeLazadaOrderID(extractLazadaOrderID(subject)) != ""
}

func sameRepairSubject(a, b string) bool {
	a = strings.Join(strings.Fields(strings.TrimSpace(a)), " ")
	b = strings.Join(strings.Fields(strings.TrimSpace(b)), " ")
	return a != "" && b != "" && strings.EqualFold(a, b)
}

func repairFirstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
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

func mapRebuiltBillIDs(bills []createdRepairBill) []string {
	return mapCreatedBillIDs(bills)
}

func mapRebuiltOrderIDs(bills []createdRepairBill) []string {
	return mapCreatedOrderIDs(bills)
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
