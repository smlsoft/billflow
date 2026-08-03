package googledrive

import (
	"context"
	"errors"
	"os"
	"reflect"
	"testing"
	"time"

	"billflow/internal/config"
	"billflow/internal/models"
)

type runnerFunc func(context.Context, string, ...string) ([]byte, error)

func (f runnerFunc) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	return f(ctx, name, args...)
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

func TestRetryDelay(t *testing.T) {
	got := []time.Duration{retryDelay(1), retryDelay(2), retryDelay(3), retryDelay(4), retryDelay(5)}
	want := []time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute, time.Hour, 6 * time.Hour}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("retry delays = %v, want %v", got, want)
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

func floatPtr(v float64) *float64 { return &v }
