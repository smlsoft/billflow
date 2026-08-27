package googledrive

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"billflow/internal/config"
	"billflow/internal/models"
)

type runnerFunc func(context.Context, string, ...string) ([]byte, error)

func (f runnerFunc) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	return f(ctx, name, args...)
}

type pdfRendererFunc struct {
	health func(context.Context) error
	render func(context.Context, string) (PDFRenderResult, error)
}

func (f pdfRendererFunc) Health(ctx context.Context) error {
	if f.health == nil {
		return nil
	}
	return f.health(ctx)
}

func (f pdfRendererFunc) Render(ctx context.Context, html string) (PDFRenderResult, error) {
	return f.render(ctx, html)
}

func TestValidateRootFolder(t *testing.T) {
	tests := []struct {
		input string
		want  string
		ok    bool
	}{
		{"BillFlow Email/Thaisunsport/", "BillFlow Email/Thaisunsport", true},
		{"", "", false},
		{"../private", "", false},
		{"BillFlow\\private", "", false},
		{"BillFlow//private", "", false},
	}
	for _, tt := range tests {
		got, err := validateRootFolder(tt.input)
		if (err == nil) != tt.ok || got != tt.want {
			t.Fatalf("validateRootFolder(%q) = %q, %v; want %q, ok=%t", tt.input, got, err, tt.want, tt.ok)
		}
	}
}

func TestPaymentTokenPrefersTTAndUsesSafeFallbacks(t *testing.T) {
	tests := []struct {
		name string
		bill models.Bill
		raw  map[string]interface{}
		want string
	}{
		{"configured tt", models.Bill{EffectivePrintPaymentMethod: "tt19630"}, nil, "TT19630"},
		{"cod", models.Bill{}, map[string]interface{}{"payment_method": "Cash on Delivery"}, "COD"},
		{"transfer", models.Bill{}, map[string]interface{}{"payment_method": "โอนผ่านธนาคาร"}, "TRANSFER"},
		{"card", models.Bill{}, map[string]interface{}{"payment_method": "Credit or Debit Card"}, "CARD"},
		{"unknown", models.Bill{}, map[string]interface{}{"payment_method": "ShopeePay"}, "OTHER"},
	}
	for _, tt := range tests {
		if got := paymentToken(&tt.bill, tt.raw); got != tt.want {
			t.Errorf("%s: got %q want %q", tt.name, got, tt.want)
		}
	}
}

func TestOrderDateAndChargeAmount(t *testing.T) {
	date, err := orderDate(map[string]interface{}{"doc_date": "2569-07-23"})
	if err != nil || date.Format("2006-01-02") != "2026-07-23" {
		t.Fatalf("Thai Buddhist date = %v, %v", date, err)
	}
	bill := &models.Bill{Items: []models.BillItem{{Qty: 2, Price: floatPtr(40), DiscountAmount: 5}}}
	if got := chargeAmount(bill, map[string]interface{}{}); got != "75" {
		t.Fatalf("fallback item total = %q, want 75", got)
	}
	if got := chargeAmount(bill, map[string]interface{}{"paid_total_amount": 1234.5}); got != "1234.5" {
		t.Fatalf("raw paid amount = %q, want 1234.5", got)
	}
}

func TestValidateExportStartDate(t *testing.T) {
	for _, tt := range []struct {
		input string
		want  string
		ok    bool
	}{
		{input: "", want: "", ok: true},
		{input: " 2026-08-04 ", want: "2026-08-04", ok: true},
		{input: "04/08/2026", ok: false},
		{input: "2026-02-30", ok: false},
	} {
		got, err := validateExportStartDate(tt.input)
		if (err == nil) != tt.ok || got != tt.want {
			t.Fatalf("validateExportStartDate(%q) = %q, %v; want %q, ok=%t", tt.input, got, err, tt.want, tt.ok)
		}
	}
}

func TestOrderDateMeetsExportStartDate(t *testing.T) {
	start := time.Date(2026, time.August, 4, 0, 0, 0, 0, time.UTC)
	for _, tt := range []struct {
		name string
		date time.Time
		want bool
	}{
		{name: "before start", date: time.Date(2026, time.August, 3, 23, 59, 59, 0, time.UTC), want: false},
		{name: "at start", date: start, want: true},
		{name: "after start", date: time.Date(2026, time.August, 5, 12, 0, 0, 0, time.UTC), want: true},
		{name: "no cutoff", date: time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC), want: true},
	} {
		var cutoff time.Time
		if tt.name != "no cutoff" {
			cutoff = start
		}
		if got := orderDateMeetsExportStartDate(tt.date, cutoff); got != tt.want {
			t.Errorf("%s: orderDateMeetsExportStartDate(%s, %s) = %t, want %t", tt.name, tt.date, cutoff, got, tt.want)
		}
	}
}

func TestRetryDelay(t *testing.T) {
	got := []time.Duration{retryDelay(1), retryDelay(2), retryDelay(3), retryDelay(4), retryDelay(5)}
	want := []time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute, time.Hour, 6 * time.Hour}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("retry delays = %v, want %v", got, want)
	}
}

func TestRunDueGuardPreventsOverlappingCronTicks(t *testing.T) {
	svc := &Service{}
	if !svc.runDueActive.CompareAndSwap(false, true) {
		t.Fatal("first cron tick should acquire the guard")
	}
	if svc.runDueActive.CompareAndSwap(false, true) {
		t.Fatal("overlapping cron tick must not acquire the guard")
	}
	svc.runDueActive.Store(false)
	if !svc.runDueActive.CompareAndSwap(false, true) {
		t.Fatal("a later cron tick should acquire the released guard")
	}
}

func TestRemoteMatchesRequiresSameSizeAndMD5(t *testing.T) {
	svc := &Service{
		cfg: &config.Config{GoogleDriveRcloneConfig: "/run/secrets/rclone.conf", GoogleDriveRcloneBinary: "rclone"},
		runner: runnerFunc(func(_ context.Context, name string, args ...string) ([]byte, error) {
			if name != "rclone" || len(args) < 4 || args[2] != "lsjson" {
				t.Fatalf("unexpected command: %s %v", name, args)
			}
			return []byte(`[{"Size":3,"Hashes":{"md5":"900150983cd24fb0d6963f7d28e17f72"}}]`), nil
		}),
	}
	exists, same, err := svc.remoteMatches(context.Background(), "remote:file", 3, "900150983cd24fb0d6963f7d28e17f72")
	if err != nil || !exists || !same {
		t.Fatalf("remoteMatches = %t, %t, %v", exists, same, err)
	}
	svc.runner = runnerFunc(func(context.Context, string, ...string) ([]byte, error) { return nil, errors.New("remote unavailable") })
	if _, _, err := svc.remoteMatches(context.Background(), "remote:file", 3, "x"); err == nil {
		t.Fatal("expected remote error")
	}
}

func TestRcloneConfigHasRemote(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "rclone-*.conf")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("[thaisunsport_gdrive]\ntype = drive\n\n[other]\ntype = drive\n"); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if !rcloneConfigHasRemote(file.Name(), "thaisunsport_gdrive") {
		t.Fatal("expected configured remote")
	}
	if rcloneConfigHasRemote(file.Name(), "missing") {
		t.Fatal("unexpected missing remote")
	}
}

func TestRuntimeReadySupportsEncryptedRcloneConfig(t *testing.T) {
	configFile := t.TempDir() + "/rclone.conf"
	if err := os.WriteFile(configFile, []byte("RCLONE_ENCRYPT_V0:example"), 0o600); err != nil {
		t.Fatal(err)
	}
	svc := &Service{
		cfg: &config.Config{
			GoogleDriveRcloneRemote: "thaisunsport_gdrive",
			GoogleDriveRcloneConfig: configFile,
			GoogleDriveRcloneBinary: "sh",
			GoogleDriveExportFormat: "html",
		},
		runner: runnerFunc(func(_ context.Context, name string, args ...string) ([]byte, error) {
			if name != "sh" || len(args) != 3 || args[2] != "listremotes" {
				t.Fatalf("unexpected command: %s %v", name, args)
			}
			return []byte("thaisunsport_gdrive:\n"), nil
		}),
	}
	if ok, reason := svc.runtimeReady(); !ok {
		t.Fatalf("runtimeReady = false: %s", reason)
	}
}

func TestCommandRunnerKeepsSuccessfulStdoutClean(t *testing.T) {
	runner := commandRunner{}
	output, err := runner.Run(context.Background(), "sh", "-c", `printf '[{"Size":3}]'; printf 'notice\n' >&2`)
	if err != nil {
		t.Fatalf("successful command: %v", err)
	}
	if string(output) != `[{"Size":3}]` {
		t.Fatalf("stdout = %q, want JSON only", output)
	}

	output, err = runner.Run(context.Background(), "sh", "-c", `printf 'rclone failure' >&2; exit 1`)
	if err == nil || string(output) != "rclone failure" {
		t.Fatalf("failure output = %q, err = %v", output, err)
	}
}

func TestExportDataPDFUsesSharedDialogPreviewHTML(t *testing.T) {
	var renderedHTML string
	svc := &Service{cfg: &config.Config{ArtifactsDir: t.TempDir()}, pdfRenderer: pdfRendererFunc{render: func(_ context.Context, html string) (PDFRenderResult, error) {
		renderedHTML = html
		return PDFRenderResult{PDF: []byte("%PDF-1.7 test"), Warnings: []string{"โหลดรูปจาก cdn.example ไม่สำเร็จ"}}, nil
	}}}

	pdf, warning, err := svc.exportData(context.Background(), models.GoogleDriveEmailExport{ID: "test-pdf-job", OutputFormat: "pdf"}, &models.BillArtifact{Kind: "email_html", ContentType: "text/html"}, []byte(`<html><body><table><tr><td>ยอดที่ต้องชำระทั้งหมด:</td><td>฿216</td></tr></table></body></html>`))
	if err != nil {
		t.Fatal(err)
	}
	if string(pdf) != "%PDF-1.7 test" || warning == "" {
		t.Fatalf("unexpected PDF result: %q / %q", pdf, warning)
	}
	for _, want := range []string{`id="billflow-email-preview-reset"`, `data-billflow-print-highlight="true"`, `฿216`} {
		if !strings.Contains(renderedHTML, want) {
			t.Fatalf("renderer did not receive shared preview HTML %q:\n%s", want, renderedHTML)
		}
	}
}

func TestExportDataPDFReusesFirstRenderForRetry(t *testing.T) {
	renderCount := 0
	svc := &Service{
		cfg: &config.Config{ArtifactsDir: t.TempDir()},
		pdfRenderer: pdfRendererFunc{render: func(_ context.Context, _ string) (PDFRenderResult, error) {
			renderCount++
			return PDFRenderResult{PDF: []byte("%PDF-first-render"), Warnings: []string{"โหลดรูปจาก cdn.example ไม่สำเร็จ"}}, nil
		}},
	}
	job := models.GoogleDriveEmailExport{ID: "retry-job", OutputFormat: "pdf"}
	source := &models.BillArtifact{Kind: "email_html", ContentType: "text/html"}

	firstPDF, firstWarning, err := svc.exportData(context.Background(), job, source, []byte("<html>source</html>"))
	if err != nil {
		t.Fatal(err)
	}
	secondPDF, secondWarning, err := svc.exportData(context.Background(), job, source, []byte("<html>source</html>"))
	if err != nil {
		t.Fatal(err)
	}
	if renderCount != 1 {
		t.Fatalf("renderer calls = %d, want 1", renderCount)
	}
	if string(firstPDF) != string(secondPDF) || firstWarning != secondWarning {
		t.Fatalf("retry did not reuse cached render: %q/%q vs %q/%q", firstPDF, firstWarning, secondPDF, secondWarning)
	}
	if err := svc.removeCachedPDF(job.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, found, err := svc.readCachedPDF(job.ID); err != nil || found {
		t.Fatalf("cache after cleanup = found:%t err:%v", found, err)
	}
}

func TestExportDataLegacyHTMLDoesNotInvokePDFRenderer(t *testing.T) {
	svc := &Service{pdfRenderer: pdfRendererFunc{render: func(context.Context, string) (PDFRenderResult, error) {
		t.Fatal("legacy HTML export must not render PDF")
		return PDFRenderResult{}, nil
	}}}
	source := []byte("<html>original</html>")
	got, warning, err := svc.exportData(context.Background(), models.GoogleDriveEmailExport{OutputFormat: "html"}, nil, source)
	if err != nil || warning != "" || string(got) != string(source) {
		t.Fatalf("legacy export = %q / %q / %v", got, warning, err)
	}
}

func TestTestConnectionRendersPDFBeforeWritingGoogleDriveProbe(t *testing.T) {
	configFile := t.TempDir() + "/rclone.conf"
	if err := os.WriteFile(configFile, []byte("[thaisunsport_gdrive]\ntype = drive\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	rendered := false
	svc := &Service{
		cfg: &config.Config{
			GoogleDriveRcloneRemote: "thaisunsport_gdrive",
			GoogleDriveRcloneConfig: configFile,
			GoogleDriveRcloneBinary: "rclone",
			GoogleDriveExportFormat: "pdf",
		},
		pdfRenderer: pdfRendererFunc{render: func(_ context.Context, html string) (PDFRenderResult, error) {
			rendered = strings.Contains(html, "BillFlow PDF renderer test")
			return PDFRenderResult{PDF: []byte("%PDF-1.7 test")}, nil
		}},
		runner: runnerFunc(func(_ context.Context, name string, args ...string) ([]byte, error) {
			if name != "rclone" || len(args) < 3 {
				t.Fatalf("unexpected command: %s %v", name, args)
			}
			switch args[2] {
			case "mkdir", "rmdir":
				return nil, nil
			default:
				t.Fatalf("unexpected rclone operation: %v", args)
				return nil, nil
			}
		}),
		now: func() time.Time { return time.Unix(1, 0) },
	}
	if err := svc.testConnection("BillFlowEmail"); err != nil {
		t.Fatal(err)
	}
	if !rendered {
		t.Fatal("PDF renderer was not tested")
	}
}

func floatPtr(v float64) *float64 { return &v }
