// Package googledrive exports source marketplace emails through an rclone
// remote configured by the server operator. It intentionally never receives
// OAuth credentials through the BillFlow UI or database.
package googledrive

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	stdhtml "html"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"billflow/internal/config"
	"billflow/internal/models"
	"billflow/internal/repository"
	"billflow/internal/services/artifact"
	"billflow/internal/services/emailpreview"
)

const (
	settingEnabled   = "google_drive_export.enabled"
	settingRoot      = "google_drive_export.root_folder"
	settingStartDate = "google_drive_export.start_date"
	maxAttempts      = 8
	pdfCacheDirName  = ".google-drive-pdf-cache"
)

var remoteNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
var errOrderBeforeExportStartDate = errors.New("วันที่สั่งซื้อก่อนวันเริ่มเก็บ Google Drive")

type Runner interface {
	Run(context.Context, string, ...string) ([]byte, error)
}

type commandRunner struct{}

func (commandRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	stdout, err := cmd.Output()
	if err == nil {
		return stdout, nil
	}

	// Commands such as `rclone lsjson` must keep stdout as valid JSON. Return
	// stderr only on failure so callers can surface an actionable error without
	// allowing rclone notices to corrupt JSON success output.
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
		return exitErr.Stderr, err
	}
	return stdout, err
}

type RuntimeStatus struct {
	Enabled      bool   `json:"enabled"`
	RootFolder   string `json:"root_folder"`
	StartDate    string `json:"start_date"`
	Remote       string `json:"remote"`
	OutputFormat string `json:"output_format"`
	RuntimeReady bool   `json:"runtime_ready"`
	RuntimeError string `json:"runtime_error,omitempty"`
	MaxAttempts  int    `json:"max_attempts"`
}

type BackfillPreview struct {
	DateFrom       string `json:"date_from"`
	DateTo         string `json:"date_to"`
	CandidateCount int    `json:"candidate_count"`
	Limited        bool   `json:"limited"`
	Limit          int    `json:"limit"`
}

type BackfillResult struct {
	CandidateCount int `json:"candidate_count"`
	Queued         int `json:"queued"`
	AlreadyQueued  int `json:"already_queued"`
	Skipped        int `json:"skipped"`
}

type Service struct {
	cfg          *config.Config
	settingsRepo *repository.AppSettingsRepo
	billRepo     *repository.BillRepo
	artifactRepo *repository.BillArtifactRepo
	artifactSvc  *artifact.Service
	exportRepo   *repository.GoogleDriveEmailExportRepo
	auditRepo    *repository.AuditLogRepo
	log          *zap.Logger
	runner       Runner
	pdfRenderer  PDFRenderer
	now          func() time.Time
}

func NewEmailExportService(
	cfg *config.Config,
	settingsRepo *repository.AppSettingsRepo,
	billRepo *repository.BillRepo,
	artifactRepo *repository.BillArtifactRepo,
	artifactSvc *artifact.Service,
	exportRepo *repository.GoogleDriveEmailExportRepo,
	auditRepo *repository.AuditLogRepo,
	logger *zap.Logger,
) *Service {
	var renderer PDFRenderer
	if cfg != nil {
		renderer = newHTTPPDFRenderer(cfg.EmailPDFRendererURL, cfg.EmailPDFRendererToken)
	}
	return &Service{
		cfg: cfg, settingsRepo: settingsRepo, billRepo: billRepo, artifactRepo: artifactRepo,
		artifactSvc: artifactSvc, exportRepo: exportRepo, auditRepo: auditRepo, log: logger,
		runner: commandRunner{}, pdfRenderer: renderer, now: time.Now,
	}
}

// SetRunner is used by tests. Production always uses direct exec.CommandContext
// arguments; no shell expands remote paths or filenames.
func (s *Service) SetRunner(r Runner) {
	if r != nil {
		s.runner = r
	}
}

// SetPDFRenderer is used by focused service tests. Production uses the
// private email-renderer Compose service.
func (s *Service) SetPDFRenderer(renderer PDFRenderer) {
	if renderer != nil {
		s.pdfRenderer = renderer
	}
}

func (s *Service) Status() RuntimeStatus {
	status := RuntimeStatus{MaxAttempts: maxAttempts}
	if s == nil || s.cfg == nil {
		return status
	}
	status.Remote = strings.TrimSpace(s.cfg.GoogleDriveRcloneRemote)
	status.OutputFormat, _ = s.exportFormat()
	if s.settingsRepo == nil {
		status.RuntimeReady, status.RuntimeError = s.runtimeReady()
		return status
	}
	enabled, _ := s.settingsRepo.GetValue(settingEnabled)
	root, _ := s.settingsRepo.GetValue(settingRoot)
	startDate, _ := s.settingsRepo.GetValue(settingStartDate)
	status.Enabled = enabled == "true"
	status.RootFolder = root
	status.StartDate = startDate
	status.RuntimeReady, status.RuntimeError = s.runtimeReady()
	return status
}

func (s *Service) UpdateSettings(enabled bool, rootFolder, startDate, userID string) (RuntimeStatus, error) {
	if s == nil || s.settingsRepo == nil {
		return RuntimeStatus{}, errors.New("google drive export service not configured")
	}
	root, err := validateRootFolder(rootFolder)
	if err != nil {
		return RuntimeStatus{}, err
	}
	startDate, err = validateExportStartDate(startDate)
	if err != nil {
		return RuntimeStatus{}, err
	}
	if enabled {
		if ok, reason := s.runtimeReady(); !ok {
			return RuntimeStatus{}, errors.New(reason)
		}
		if err := s.testConnection(root); err != nil {
			return RuntimeStatus{}, err
		}
	}
	if err := s.settingsRepo.UpsertMany(map[string]string{
		settingEnabled: strconv.FormatBool(enabled), settingRoot: root, settingStartDate: startDate,
	}, map[string]bool{}, userID); err != nil {
		return RuntimeStatus{}, err
	}
	status := s.Status()
	s.logAudit("google_drive_export_settings_updated", nil, userID, map[string]interface{}{
		"enabled": enabled, "root_folder": root, "start_date": startDate,
	})
	return status, nil
}

func (s *Service) TestConnection(rootFolder string) error {
	root, err := validateRootFolder(rootFolder)
	if err != nil {
		return err
	}
	if ok, reason := s.runtimeReady(); !ok {
		return errors.New(reason)
	}
	return s.testConnection(root)
}

func (s *Service) testConnection(root string) error {
	format, err := s.exportFormat()
	if err != nil {
		return err
	}
	if format == "pdf" {
		if s.pdfRenderer == nil {
			return errors.New("PDF renderer ไม่พร้อมใช้งาน")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if _, err := s.pdfRenderer.Render(ctx, "<!doctype html><html><body><p>BillFlow PDF renderer test</p></body></html>"); err != nil {
			return fmt.Errorf("ทดสอบ PDF renderer ไม่สำเร็จ: %w", err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	probe := path.Join(root, ".billflow-connection-test-"+strconv.FormatInt(s.now().UnixNano(), 10))
	remotePath := s.remotePath(probe)
	if output, err := s.run(ctx, "mkdir", remotePath); err != nil {
		return fmt.Errorf("ทดสอบ Google Drive ไม่สำเร็จ: %s", cleanCommandError(output, err))
	}
	if output, err := s.run(ctx, "rmdir", remotePath); err != nil {
		return fmt.Errorf("สร้างโฟลเดอร์ทดสอบได้ แต่ลบไม่สำเร็จ: %s", cleanCommandError(output, err))
	}
	return nil
}

// EnqueueSentBill creates only a local DB task. It never blocks or rolls back
// a completed SML send when Drive is temporarily unavailable.
func (s *Service) EnqueueSentBill(billID, userID string) (bool, error) {
	status := s.Status()
	if !status.Enabled {
		return false, nil
	}
	startDate, err := s.exportStartDate()
	if err != nil {
		return false, err
	}
	job, err := s.buildJob(billID, userID, startDate)
	if err != nil {
		if errors.Is(err, errOrderBeforeExportStartDate) {
			return false, nil
		}
		return false, err
	}
	created, err := s.exportRepo.InsertQueued(job)
	if err != nil {
		return false, err
	}
	if created {
		s.logAudit("google_drive_email_export_queued", &billID, userID, map[string]interface{}{
			"sml_doc_no": job.SMLDocNo, "marketplace_order_id": job.MarketplaceOrderID,
		})
	}
	return created, nil
}

func (s *Service) buildJob(billID, userID string, startDate time.Time) (*models.GoogleDriveEmailExport, error) {
	if s == nil || s.billRepo == nil || s.artifactRepo == nil || s.exportRepo == nil {
		return nil, errors.New("google drive export service not configured")
	}
	bill, err := s.billRepo.FindByID(billID)
	if err != nil {
		return nil, fmt.Errorf("load bill: %w", err)
	}
	if bill == nil {
		return nil, errors.New("ไม่พบบิล")
	}
	if bill.ArchivedAt != nil {
		return nil, errors.New("บิลถูกย้ายออกจากรายการแล้ว")
	}
	if bill.BillType != "purchase" || (bill.Source != "shopee_shipped" && bill.Source != "lazada_email") {
		return nil, errors.New("รองรับเฉพาะใบสั่งซื้อจาก Shopee และ Lazada")
	}
	if bill.Status != "sent" || bill.SMLDocNo == nil || strings.TrimSpace(*bill.SMLDocNo) == "" {
		return nil, errors.New("บิลยังส่ง SML ไม่สำเร็จ")
	}
	raw := rawMap(bill.RawData)
	messageID := stringRaw(raw, "email_message_id", "message_id")
	if messageID == "" {
		return nil, errors.New("ไม่พบ message id ของอีเมลต้นทาง")
	}
	sourceArtifact, err := s.artifactRepo.FindEmailBodyBySourceMessage(bill.Source, messageID)
	if err != nil {
		return nil, fmt.Errorf("ค้นหาอีเมลต้นทาง: %w", err)
	}
	if sourceArtifact == nil {
		return nil, errors.New("ไม่พบไฟล์อีเมลต้นทาง")
	}
	orderDate, err := orderDate(raw)
	if err != nil {
		return nil, err
	}
	if !orderDateMeetsExportStartDate(orderDate, startDate) {
		return nil, fmt.Errorf("%w: %s", errOrderBeforeExportStartDate, resultDate(startDate))
	}
	orderID := strings.TrimLeft(stringRaw(raw, "order_id", "shopee_order_id", "order_no"), "#")
	if orderID == "" {
		return nil, errors.New("ไม่พบเลขคำสั่งซื้อจาก marketplace")
	}
	payment := paymentToken(bill, raw)
	channel := channelName(bill.Source)
	charge := chargeAmount(bill, raw)
	ext := "txt"
	format, err := s.exportFormat()
	if err != nil {
		return nil, err
	}
	if format == "pdf" {
		ext = "pdf"
	} else if sourceArtifact.Kind == "email_html" || strings.Contains(strings.ToLower(sourceArtifact.ContentType), "html") {
		ext = "html"
	}
	fileName := strings.Join([]string{
		orderDate.Format("20060102"), channel, payment, safeComponent(*bill.SMLDocNo), safeComponent(orderID), charge,
	}, "_") + "." + ext
	root, err := s.currentRoot()
	if err != nil {
		return nil, err
	}
	remotePath := path.Join(root, orderDate.Format("2006"), orderDate.Format("01"), orderDate.Format("02"), channel, payment, fileName)
	var createdBy *string
	if userID != "" {
		createdBy = &userID
	}
	return &models.GoogleDriveEmailExport{
		BillID: bill.ID, SourceArtifactID: sourceArtifact.ID, SourceSHA256: sourceArtifact.SHA256,
		SourceContentType: sourceArtifact.ContentType, SourceFilename: sourceArtifact.Filename,
		SourceChannel: channel, OrderDate: orderDate.Format("2006-01-02"), PaymentToken: payment,
		SMLDocNo: *bill.SMLDocNo, MarketplaceOrderID: orderID, ChargeAmount: charge,
		OutputFormat: format, RemotePath: remotePath, Priority: 100, CreatedBy: createdBy,
	}, nil
}

func (s *Service) RunDue(ctx context.Context) {
	if s == nil || !s.Status().Enabled {
		return
	}
	// Chromium rendering is intentionally serialized. It avoids a burst of
	// browser processes competing with SML and the primary backend on a small
	// customer server; rclone verification still runs in the same durable job.
	jobs, err := s.exportRepo.ClaimDue(1)
	if err != nil {
		s.log.Warn("claim google drive exports", zap.Error(err))
		return
	}
	for i := range jobs {
		s.processOne(ctx, jobs[i])
	}
}

func (s *Service) processOne(parent context.Context, job models.GoogleDriveEmailExport) {
	ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
	defer cancel()
	result, message, warning := s.upload(ctx, job)
	switch result {
	case "succeeded":
		if err := s.exportRepo.MarkSucceeded(job.ID, warning); err != nil {
			s.log.Error("mark google drive export succeeded", zap.Error(err))
			return
		}
		if err := s.removeCachedPDF(job.ID); err != nil {
			s.log.Warn("remove cached google drive PDF", zap.String("job_id", job.ID), zap.Error(err))
		}
		s.logAudit("google_drive_email_export_succeeded", &job.BillID, "", map[string]interface{}{
			"sml_doc_no": job.SMLDocNo, "marketplace_order_id": job.MarketplaceOrderID,
			"attempt": job.AttemptCount, "output_format": job.OutputFormat, "render_warning": warning != "",
		})
	case "conflict":
		_ = s.exportRepo.MarkConflict(job.ID, message)
		s.logAudit("google_drive_email_export_conflict", &job.BillID, "", map[string]interface{}{
			"sml_doc_no": job.SMLDocNo, "marketplace_order_id": job.MarketplaceOrderID, "reason": message,
		})
	case "skipped":
		_ = s.exportRepo.MarkSkipped(job.ID, message)
		s.logAudit("google_drive_email_export_skipped", &job.BillID, "", map[string]interface{}{"reason": message})
	default:
		final := job.AttemptCount >= maxAttempts
		next := s.now().Add(retryDelay(job.AttemptCount))
		_ = s.exportRepo.MarkRetryOrFailed(job.ID, message, next, final)
		action := "google_drive_email_export_retry_scheduled"
		if final {
			action = "google_drive_email_export_failed"
		}
		s.logAudit(action, &job.BillID, "", map[string]interface{}{"attempt": job.AttemptCount, "reason": message, "next_attempt_at": next.Format(time.RFC3339)})
	}
}

func (s *Service) upload(ctx context.Context, job models.GoogleDriveEmailExport) (string, string, string) {
	if ok, reason := s.runtimeReady(); !ok {
		return "retry", reason, ""
	}
	if normalizedJobOutputFormat(job.OutputFormat) == "pdf" {
		if ok, reason := s.rendererReady("pdf"); !ok {
			return "retry", reason, ""
		}
	}
	if job.SourceArtifactID == "" {
		return "skipped", "ไม่พบไฟล์อีเมลต้นทางในคิว", ""
	}
	data, sourceArtifact, err := s.artifactSvc.Read(job.SourceArtifactID)
	if err != nil {
		return "retry", "อ่านไฟล์อีเมลต้นทางไม่สำเร็จ", ""
	}
	if sourceArtifact == nil || len(data) == 0 {
		return "skipped", "ไม่พบไฟล์อีเมลต้นทาง", ""
	}
	if job.SourceSHA256 != "" && sourceArtifact.SHA256 != "" && job.SourceSHA256 != sourceArtifact.SHA256 {
		return "conflict", "ไฟล์อีเมลต้นทางเปลี่ยนหลังสร้างคิว", ""
	}
	data, renderWarning, err := s.exportData(ctx, job, sourceArtifact, data)
	if err != nil {
		return "retry", err.Error(), ""
	}
	localMD5 := md5.Sum(data)
	localMD5Hex := hex.EncodeToString(localMD5[:])
	tmp, err := os.CreateTemp("", "billflow-google-drive-*")
	if err != nil {
		return "retry", "สร้างไฟล์ชั่วคราวไม่สำเร็จ", ""
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return "retry", "ตั้งค่าสิทธิ์ไฟล์ชั่วคราวไม่สำเร็จ", ""
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return "retry", "เขียนไฟล์ชั่วคราวไม่สำเร็จ", ""
	}
	if err := tmp.Close(); err != nil {
		return "retry", "ปิดไฟล์ชั่วคราวไม่สำเร็จ", ""
	}
	remote := s.remotePath(job.RemotePath)
	remoteTemp := s.remotePath(job.RemotePath + ".partial-" + safeComponent(job.ID) + "-" + strconv.Itoa(job.AttemptCount))
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_, _ = s.run(cleanupCtx, "deletefile", remoteTemp)
	}()
	if output, err := s.run(ctx, "copyto", "--checksum", "--retries", "1", "--low-level-retries", "1", tmpName, remoteTemp); err != nil {
		s.discardCachedPDF(job)
		return "retry", "อัปโหลด Google Drive ไม่สำเร็จ: " + cleanCommandError(output, err), ""
	}
	exists, same, err := s.remoteMatches(ctx, remoteTemp, int64(len(data)), localMD5Hex)
	if err != nil {
		s.discardCachedPDF(job)
		return "retry", "ตรวจไฟล์ชั่วคราวหลังอัปโหลดไม่สำเร็จ", ""
	}
	if !exists {
		s.discardCachedPDF(job)
		return "retry", "ไม่พบไฟล์ชั่วคราวหลังอัปโหลด", ""
	}
	if !same {
		s.discardCachedPDF(job)
		return "conflict", "ไฟล์ชั่วคราวบน Google Drive ไม่ตรงกับต้นฉบับ", ""
	}
	if output, err := s.run(ctx, "moveto", "--ignore-existing", remoteTemp, remote); err != nil {
		return "retry", "ย้ายไฟล์ไปชื่อปลายทางไม่สำเร็จ: " + cleanCommandError(output, err), ""
	}
	exists, same, err = s.remoteMatches(ctx, remote, int64(len(data)), localMD5Hex)
	if err != nil {
		return "retry", "ตรวจไฟล์หลังย้ายไม่สำเร็จ", ""
	}
	if !exists {
		return "retry", "ไม่พบไฟล์หลังย้าย", ""
	}
	if !same {
		return "conflict", "พบชื่อไฟล์เดิมบน Google Drive แต่เนื้อหาไม่ตรงกัน", ""
	}
	return "succeeded", "", renderWarning
}

func (s *Service) exportData(ctx context.Context, job models.GoogleDriveEmailExport, source *models.BillArtifact, data []byte) ([]byte, string, error) {
	if normalizedJobOutputFormat(job.OutputFormat) != "pdf" {
		return data, "", nil
	}
	if cachedPDF, cachedWarning, found, err := s.readCachedPDF(job.ID); err != nil {
		return nil, "", err
	} else if found {
		return cachedPDF, cachedWarning, nil
	}
	if s.pdfRenderer == nil {
		return nil, "", errors.New("PDF renderer ไม่พร้อมใช้งาน")
	}
	html := string(data)
	if source == nil || (strings.ToLower(source.Kind) != "email_html" && !strings.Contains(strings.ToLower(source.ContentType), "html")) {
		html = "<!doctype html><html><head><meta charset=\"utf-8\"></head><body><pre style=\"white-space:pre-wrap;font-family:sans-serif\">" + stdhtml.EscapeString(html) + "</pre></body></html>"
	}
	result, err := s.pdfRenderer.Render(ctx, emailpreview.PrepareHTML(html))
	if err != nil {
		return nil, "", fmt.Errorf("สร้าง PDF จากอีเมลไม่สำเร็จ: %w", err)
	}
	warning := strings.Join(result.Warnings, " · ")
	if err := s.cachePDF(job.ID, result.PDF, warning); err != nil {
		return nil, "", err
	}
	return result.PDF, warning, nil
}

// PDF bytes from Chromium include generation metadata and are not stable across
// renders. Retrying an upload must therefore reuse the first render so its MD5
// still matches a final Drive file written just before an interrupted process.
func (s *Service) readCachedPDF(jobID string) ([]byte, string, bool, error) {
	pdfPath, warningPath, err := s.cachedPDFPaths(jobID)
	if err != nil {
		return nil, "", false, err
	}
	data, err := os.ReadFile(pdfPath)
	if os.IsNotExist(err) {
		return nil, "", false, nil
	}
	if err != nil {
		return nil, "", false, fmt.Errorf("อ่าน PDF ที่เตรียมอัปโหลดไม่สำเร็จ: %w", err)
	}
	if len(data) == 0 || len(data) > maxRenderedPDFBytes || !bytes.HasPrefix(data, []byte("%PDF-")) {
		return nil, "", false, errors.New("PDF ที่เตรียมอัปโหลดไม่ถูกต้อง")
	}
	warning, err := os.ReadFile(warningPath)
	if os.IsNotExist(err) {
		return data, "", true, nil
	}
	if err != nil {
		return nil, "", false, fmt.Errorf("อ่านคำเตือน PDF ที่เตรียมอัปโหลดไม่สำเร็จ: %w", err)
	}
	return data, string(warning), true, nil
}

func (s *Service) cachePDF(jobID string, data []byte, warning string) error {
	if len(data) == 0 || len(data) > maxRenderedPDFBytes || !bytes.HasPrefix(data, []byte("%PDF-")) {
		return errors.New("PDF ที่เตรียมอัปโหลดไม่ถูกต้อง")
	}
	pdfPath, warningPath, err := s.cachedPDFPaths(jobID)
	if err != nil {
		return err
	}
	if _, _, found, err := s.readCachedPDF(jobID); err != nil {
		return err
	} else if found {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(pdfPath), 0o700); err != nil {
		return fmt.Errorf("สร้างพื้นที่เก็บ PDF สำหรับ retry ไม่สำเร็จ: %w", err)
	}
	// Write the warning first. A completed PDF cache always has its matching
	// warning metadata, even if the server stops immediately after rendering.
	if err := atomicWriteFile(warningPath, []byte(warning)); err != nil {
		return fmt.Errorf("เก็บคำเตือน PDF สำหรับ retry ไม่สำเร็จ: %w", err)
	}
	if err := atomicWriteFile(pdfPath, data); err != nil {
		return fmt.Errorf("เก็บ PDF สำหรับ retry ไม่สำเร็จ: %w", err)
	}
	return nil
}

func (s *Service) removeCachedPDF(jobID string) error {
	pdfPath, warningPath, err := s.cachedPDFPaths(jobID)
	if err != nil {
		return err
	}
	for _, filename := range []string{pdfPath, warningPath} {
		if err := os.Remove(filename); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// A cache is only required after a move has been attempted, because Drive may
// already contain the exact PDF while the process has not marked success yet.
// Earlier upload failures cannot leave the final object behind, so drop the
// cache and avoid retaining PDFs during a prolonged Drive outage.
func (s *Service) discardCachedPDF(job models.GoogleDriveEmailExport) {
	if normalizedJobOutputFormat(job.OutputFormat) != "pdf" {
		return
	}
	if err := s.removeCachedPDF(job.ID); err != nil && s.log != nil {
		s.log.Warn("discard cached google drive PDF", zap.String("job_id", job.ID), zap.Error(err))
	}
}

func (s *Service) cachedPDFPaths(jobID string) (string, string, error) {
	if s == nil || s.cfg == nil || strings.TrimSpace(s.cfg.ArtifactsDir) == "" || strings.TrimSpace(jobID) == "" {
		return "", "", errors.New("พื้นที่เก็บ PDF สำหรับ retry ไม่พร้อมใช้งาน")
	}
	sum := sha256.Sum256([]byte(jobID))
	base := hex.EncodeToString(sum[:])
	dir := filepath.Join(s.cfg.ArtifactsDir, pdfCacheDirName)
	return filepath.Join(dir, base+".pdf"), filepath.Join(dir, base+".warning"), nil
}

func atomicWriteFile(filename string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(filename), ".billflow-write-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, filename)
}

type remoteObject struct {
	Size   int64             `json:"Size"`
	Hashes map[string]string `json:"Hashes"`
}

func (s *Service) remoteMatches(ctx context.Context, remote string, size int64, localMD5 string) (bool, bool, error) {
	output, err := s.run(ctx, "lsjson", "--files-only", "--hash", remote)
	if err != nil {
		return false, false, err
	}
	var entries []remoteObject
	if err := json.Unmarshal(output, &entries); err != nil {
		return false, false, err
	}
	if len(entries) == 0 {
		return false, false, nil
	}
	if len(entries) != 1 {
		return true, false, nil
	}
	remoteMD5 := ""
	for key, value := range entries[0].Hashes {
		if strings.EqualFold(key, "md5") {
			remoteMD5 = strings.ToLower(strings.TrimSpace(value))
			break
		}
	}
	return true, entries[0].Size == size && remoteMD5 != "" && remoteMD5 == strings.ToLower(localMD5), nil
}

func (s *Service) PreviewBackfill(dateFrom, dateTo string) (BackfillPreview, error) {
	from, to, err := parseBackfillRange(dateFrom, dateTo)
	if err != nil {
		return BackfillPreview{}, err
	}
	if err := s.validateBackfillExportStart(from); err != nil {
		return BackfillPreview{}, err
	}
	ids, err := s.exportRepo.ListBackfillBillIDs(from.Format("2006-01-02"), to.Format("2006-01-02"), 501)
	if err != nil {
		return BackfillPreview{}, err
	}
	return BackfillPreview{DateFrom: from.Format("2006-01-02"), DateTo: to.Format("2006-01-02"), CandidateCount: min(len(ids), 500), Limited: len(ids) > 500, Limit: 500}, nil
}

func (s *Service) EnqueueBackfill(dateFrom, dateTo, userID string) (BackfillResult, error) {
	if !s.Status().Enabled {
		return BackfillResult{}, errors.New("กรุณาเปิดใช้งาน Google Drive ก่อน")
	}
	from, to, err := parseBackfillRange(dateFrom, dateTo)
	if err != nil {
		return BackfillResult{}, err
	}
	if err := s.validateBackfillExportStart(from); err != nil {
		return BackfillResult{}, err
	}
	ids, err := s.exportRepo.ListBackfillBillIDs(from.Format("2006-01-02"), to.Format("2006-01-02"), 501)
	if err != nil {
		return BackfillResult{}, err
	}
	if len(ids) > 500 {
		return BackfillResult{}, errors.New("พบเกิน 500 บิล กรุณาแบ่งช่วงวันที่ให้สั้นลง")
	}
	result := BackfillResult{CandidateCount: len(ids)}
	for _, id := range ids {
		created, err := s.EnqueueSentBill(id, userID)
		if err != nil {
			result.Skipped++
			continue
		}
		if created {
			result.Queued++
		} else {
			result.AlreadyQueued++
		}
	}
	s.logAudit("google_drive_email_export_backfill_queued", nil, userID, map[string]interface{}{
		"date_from": resultDate(from), "date_to": resultDate(to), "queued": result.Queued,
		"already_queued": result.AlreadyQueued, "skipped": result.Skipped,
	})
	return result, nil
}

func (s *Service) ReconcileRecentSent() {
	if s == nil || !s.Status().Enabled {
		return
	}
	ids, err := s.exportRepo.ListRecentUnqueuedSentBillIDs(500)
	if err != nil {
		s.log.Warn("list recent google drive reconciliation", zap.Error(err))
		return
	}
	for _, id := range ids {
		if _, err := s.EnqueueSentBill(id, ""); err != nil {
			s.log.Warn("queue reconciled google drive export", zap.String("bill_id", id), zap.Error(err))
		}
	}
}

func (s *Service) RecoverInterrupted() {
	if s == nil || s.exportRepo == nil {
		return
	}
	if n, err := s.exportRepo.RecoverInterrupted(); err != nil {
		s.log.Warn("recover google drive exports", zap.Error(err))
	} else if n > 0 {
		s.log.Warn("recovered google drive exports", zap.Int64("count", n))
	}
}

func (s *Service) ListJobs(limit int) ([]models.GoogleDriveEmailExport, models.GoogleDriveEmailExportCounts, error) {
	return s.exportRepo.List(limit)
}

func (s *Service) Retry(id, userID string) (bool, error) {
	ok, err := s.exportRepo.Retry(id)
	if ok {
		s.logAudit("google_drive_email_export_retried", nil, userID, map[string]interface{}{"job_id": id})
	}
	return ok, err
}

func (s *Service) RequeueAsPDF(id, userID string) (bool, error) {
	if ok, reason := s.rendererReady("pdf"); !ok {
		return false, errors.New(reason)
	}
	ok, err := s.exportRepo.RequeueAsPDF(id)
	if ok {
		s.logAudit("google_drive_email_export_pdf_queued", nil, userID, map[string]interface{}{"job_id": id})
	}
	return ok, err
}

func (s *Service) currentRoot() (string, error) {
	root, err := s.settingsRepo.GetValue(settingRoot)
	if err != nil {
		return "", err
	}
	return validateRootFolder(root)
}

func (s *Service) runtimeReady() (bool, string) {
	if s == nil || s.cfg == nil {
		return false, "Google Drive export service ไม่พร้อม"
	}
	format, err := s.exportFormat()
	if err != nil {
		return false, err.Error()
	}
	if !remoteNamePattern.MatchString(strings.TrimSpace(s.cfg.GoogleDriveRcloneRemote)) {
		return false, "ยังไม่ได้ตั้งค่า GOOGLE_DRIVE_RCLONE_REMOTE บน server"
	}
	if strings.TrimSpace(s.cfg.GoogleDriveRcloneConfig) == "" {
		return false, "ยังไม่ได้ตั้งค่า RCLONE_CONFIG บน server"
	}
	configData, err := os.ReadFile(s.cfg.GoogleDriveRcloneConfig)
	if err != nil {
		return false, "ไม่พบไฟล์ rclone.conf ที่ mount เข้า backend"
	}
	info, err := os.Stat(s.cfg.GoogleDriveRcloneConfig)
	if err != nil || info.IsDir() {
		return false, "ไม่พบไฟล์ rclone.conf ที่ mount เข้า backend"
	}
	if _, err := exec.LookPath(s.rcloneBinary()); err != nil {
		return false, "ไม่พบคำสั่ง rclone ใน backend container"
	}
	if !rcloneConfigIsEncrypted(configData) && !rcloneConfigHasRemote(s.cfg.GoogleDriveRcloneConfig, s.cfg.GoogleDriveRcloneRemote) {
		return false, "ไม่พบ remote Google Drive ที่ตั้งไว้ใน rclone.conf"
	}
	if rcloneConfigIsEncrypted(configData) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		output, err := s.run(ctx, "listremotes")
		if err != nil {
			return false, "เปิด rclone.conf ที่เข้ารหัสไม่สำเร็จ: " + cleanCommandError(output, err)
		}
		for _, line := range strings.Split(string(output), "\n") {
			if strings.TrimSuffix(strings.TrimSpace(line), ":") == strings.TrimSpace(s.cfg.GoogleDriveRcloneRemote) {
				return s.rendererReady(format)
			}
		}
		return false, "ไม่พบ remote Google Drive ที่ตั้งไว้ใน rclone.conf"
	}
	return s.rendererReady(format)
}

func (s *Service) exportFormat() (string, error) {
	if s == nil || s.cfg == nil {
		return "", errors.New("Google Drive export service ไม่พร้อม")
	}
	format := strings.ToLower(strings.TrimSpace(s.cfg.GoogleDriveExportFormat))
	if format == "" {
		format = "pdf"
	}
	if format != "pdf" && format != "html" {
		return "", errors.New("GOOGLE_DRIVE_EMAIL_EXPORT_FORMAT ต้องเป็น pdf หรือ html")
	}
	return format, nil
}

func normalizedJobOutputFormat(value string) string {
	if strings.EqualFold(strings.TrimSpace(value), "pdf") {
		return "pdf"
	}
	return "html"
}

func (s *Service) rendererReady(format string) (bool, string) {
	if format != "pdf" {
		return true, ""
	}
	if strings.TrimSpace(s.cfg.EmailPDFRendererToken) == "" {
		return false, "ยังไม่ได้ตั้งค่า EMAIL_PDF_RENDERER_TOKEN บน server"
	}
	if s.pdfRenderer == nil {
		return false, "PDF renderer ไม่พร้อมใช้งาน"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.pdfRenderer.Health(ctx); err != nil {
		return false, "PDF renderer ไม่พร้อม: " + cleanRendererError(err)
	}
	return true, ""
}

func cleanRendererError(err error) string {
	if err == nil {
		return ""
	}
	message := strings.TrimSpace(err.Error())
	if len(message) > 220 {
		return message[:220]
	}
	return message
}

func rcloneConfigIsEncrypted(data []byte) bool {
	return strings.HasPrefix(strings.TrimSpace(string(data)), "RCLONE_ENCRYPT_V0:")
}

func rcloneConfigHasRemote(configPath, remote string) bool {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return false
	}
	section := "[" + strings.TrimSpace(remote) + "]"
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == section {
			return true
		}
	}
	return false
}

func (s *Service) rcloneBinary() string {
	if s != nil && strings.TrimSpace(s.cfg.GoogleDriveRcloneBinary) != "" {
		return strings.TrimSpace(s.cfg.GoogleDriveRcloneBinary)
	}
	return "rclone"
}

func (s *Service) run(ctx context.Context, args ...string) ([]byte, error) {
	base := []string{"--config", s.cfg.GoogleDriveRcloneConfig}
	return s.runner.Run(ctx, s.rcloneBinary(), append(base, args...)...)
}

func (s *Service) remotePath(relative string) string {
	return strings.TrimSpace(s.cfg.GoogleDriveRcloneRemote) + ":" + strings.TrimPrefix(relative, "/")
}

func (s *Service) logAudit(action string, targetID *string, userID string, detail map[string]interface{}) {
	if s == nil || s.auditRepo == nil {
		return
	}
	var actor *string
	if userID != "" {
		actor = &userID
	}
	if err := s.auditRepo.Log(models.AuditEntry{Action: action, TargetID: targetID, UserID: actor, Source: "google_drive", Level: "info", Detail: detail}); err != nil {
		s.log.Warn("write google drive audit log", zap.Error(err))
	}
}

func validateRootFolder(raw string) (string, error) {
	root := strings.Trim(strings.TrimSpace(raw), "/")
	if root == "" {
		return "", errors.New("กรุณาระบุโฟลเดอร์หลักบน Google Drive")
	}
	if len(root) > 240 || strings.Contains(root, "\\") || strings.Contains(root, "..") || strings.ContainsAny(root, "\x00\r\n") {
		return "", errors.New("ชื่อโฟลเดอร์ Google Drive ไม่ถูกต้อง")
	}
	for _, segment := range strings.Split(root, "/") {
		if strings.TrimSpace(segment) == "" || segment == "." {
			return "", errors.New("ชื่อโฟลเดอร์ Google Drive ไม่ถูกต้อง")
		}
	}
	return root, nil
}

func rawMap(raw json.RawMessage) map[string]interface{} {
	data := map[string]interface{}{}
	_ = json.Unmarshal(raw, &data)
	return data
}

func stringRaw(raw map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if value, ok := raw[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func orderDate(raw map[string]interface{}) (time.Time, error) {
	for _, key := range []string{"doc_date", "order_datetime"} {
		value := stringRaw(raw, key)
		if value == "" {
			continue
		}
		for _, layout := range []string{time.RFC3339, "2006-01-02", "2006/01/02", "02/01/2006", "02-01-2006"} {
			if parsed, err := time.ParseInLocation(layout, value, time.Local); err == nil {
				year := parsed.Year()
				if year > 2400 {
					parsed = parsed.AddDate(-543, 0, 0)
				}
				if parsed.Year() >= 2000 && parsed.Year() <= 2100 {
					return parsed, nil
				}
			}
		}
	}
	return time.Time{}, errors.New("ไม่พบวันที่คำสั่งซื้อที่ใช้ตั้งชื่อไฟล์")
}

func paymentToken(bill *models.Bill, raw map[string]interface{}) string {
	method := strings.TrimSpace(bill.EffectivePrintPaymentMethod)
	if method == "" {
		method = strings.TrimSpace(bill.PrintPaymentMethod)
	}
	if method == "" {
		method = stringRaw(raw, "payment_method")
	}
	if method == "" {
		if summary, ok := raw["payment_summary"].(map[string]interface{}); ok {
			method = stringRaw(summary, "payment_method")
		}
	}
	upper := strings.ToUpper(strings.ReplaceAll(method, " ", ""))
	if regexp.MustCompile(`^TT[0-9A-Z_-]+$`).MatchString(upper) {
		return upper
	}
	lower := strings.ToLower(method)
	switch {
	case strings.Contains(lower, "cash on delivery") || strings.Contains(method, "เก็บเงินปลายทาง") || strings.Contains(lower, "cod"):
		return "COD"
	case strings.Contains(lower, "transfer") || strings.Contains(method, "โอน") || strings.Contains(lower, "bank"):
		return "TRANSFER"
	case strings.Contains(lower, "credit") || strings.Contains(lower, "debit") || strings.Contains(method, "บัตร"):
		return "CARD"
	default:
		return "OTHER"
	}
}

func chargeAmount(bill *models.Bill, raw map[string]interface{}) string {
	for _, key := range []string{"paid_total_amount", "card_charge_total", "total_amount"} {
		if value, ok := numberRaw(raw[key]); ok && value >= 0 {
			return formatAmount(value)
		}
	}
	if summary, ok := raw["payment_summary"].(map[string]interface{}); ok {
		if value, ok := numberRaw(summary["payment_paid_amount"]); ok && value >= 0 {
			return formatAmount(value)
		}
	}
	if bill != nil {
		total := 0.0
		for _, item := range bill.Items {
			if item.Price != nil {
				total += item.Qty**item.Price - item.DiscountAmount
			}
		}
		if total > 0 {
			return formatAmount(total)
		}
	}
	return "NA"
}

func numberRaw(value interface{}) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case string:
		parsed, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(v), ",", ""), 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func formatAmount(value float64) string {
	return strings.TrimRight(strings.TrimRight(strconv.FormatFloat(value, 'f', 2, 64), "0"), ".")
}

func channelName(source string) string {
	if source == "lazada_email" {
		return "Lazada"
	}
	return "Shopee"
}

func safeComponent(value string) string {
	value = strings.TrimSpace(value)
	value = regexp.MustCompile(`[^A-Za-z0-9._-]+`).ReplaceAllString(value, "-")
	value = strings.Trim(value, ".-_")
	if value == "" {
		return "NA"
	}
	if len(value) > 80 {
		return value[:80]
	}
	return value
}

func retryDelay(attempt int) time.Duration {
	switch attempt {
	case 1:
		return time.Minute
	case 2:
		return 5 * time.Minute
	case 3:
		return 15 * time.Minute
	case 4:
		return time.Hour
	default:
		return 6 * time.Hour
	}
}

func cleanCommandError(output []byte, err error) string {
	message := strings.TrimSpace(string(output))
	if message == "" {
		message = err.Error()
	}
	message = strings.ReplaceAll(message, "\n", " ")
	if len(message) > 300 {
		message = message[:300]
	}
	return message
}

func (s *Service) exportStartDate() (time.Time, error) {
	if s == nil || s.settingsRepo == nil {
		return time.Time{}, errors.New("google drive export service not configured")
	}
	raw, err := s.settingsRepo.GetValue(settingStartDate)
	if err != nil {
		return time.Time{}, fmt.Errorf("อ่านวันที่เริ่มเก็บ Google Drive: %w", err)
	}
	normalized, err := validateExportStartDate(raw)
	if err != nil || normalized == "" {
		return time.Time{}, err
	}
	return time.Parse("2006-01-02", normalized)
}

func (s *Service) validateBackfillExportStart(from time.Time) error {
	startDate, err := s.exportStartDate()
	if err != nil {
		return err
	}
	if !startDate.IsZero() && from.Before(startDate) {
		return fmt.Errorf("Google Drive ตั้งค่าให้เริ่มเก็บตั้งแต่วันที่ %s", resultDate(startDate))
	}
	return nil
}

func validateExportStartDate(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", nil
	}
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return "", errors.New("วันที่เริ่มเก็บ Google Drive ไม่ถูกต้อง")
	}
	return parsed.Format("2006-01-02"), nil
}

func orderDateMeetsExportStartDate(orderDate, startDate time.Time) bool {
	if startDate.IsZero() {
		return true
	}
	orderDay := time.Date(orderDate.Year(), orderDate.Month(), orderDate.Day(), 0, 0, 0, 0, time.UTC)
	startDay := time.Date(startDate.Year(), startDate.Month(), startDate.Day(), 0, 0, 0, 0, time.UTC)
	return !orderDay.Before(startDay)
}

func parseBackfillRange(rawFrom, rawTo string) (time.Time, time.Time, error) {
	from, err := time.Parse("2006-01-02", strings.TrimSpace(rawFrom))
	if err != nil {
		return time.Time{}, time.Time{}, errors.New("วันที่เริ่มต้นไม่ถูกต้อง")
	}
	to, err := time.Parse("2006-01-02", strings.TrimSpace(rawTo))
	if err != nil {
		return time.Time{}, time.Time{}, errors.New("วันที่สิ้นสุดไม่ถูกต้อง")
	}
	if to.Before(from) {
		return time.Time{}, time.Time{}, errors.New("วันที่สิ้นสุดต้องไม่น้อยกว่าวันที่เริ่มต้น")
	}
	if to.Sub(from) > 30*24*time.Hour {
		return time.Time{}, time.Time{}, errors.New("เลือกช่วงย้อนหลังได้ครั้งละไม่เกิน 31 วัน")
	}
	return from, to, nil
}

func resultDate(value time.Time) string { return value.Format("2006-01-02") }
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
