package emailservice

import (
	"context"
	"strings"
	"testing"

	"billflow/internal/models"

	"github.com/emersion/go-imap/v2"
)

func TestClassifyDispatchWarning(t *testing.T) {
	tests := []struct {
		name     string
		warning  string
		wantCode string
		wantSkip bool
	}{
		{
			name:     "duplicate is user skipped",
			warning:  "เมลนี้เคยสร้างบิลแล้ว",
			wantCode: "duplicate",
			wantSkip: true,
		},
		{
			name:     "duplicate or empty is user skipped",
			warning:  "duplicate_or_empty: ไม่มีบิลใหม่จากเมลนี้ อาจซ้ำหรือไม่มีรายการสินค้าที่ใช้ได้",
			wantCode: "duplicate_or_empty",
			wantSkip: true,
		},
		{
			name:     "duplicate or empty thai label is user skipped",
			warning:  "ไม่มีบิลใหม่จากเมลนี้ อาจซ้ำหรือไม่มีรายการสินค้าที่ใช้ได้",
			wantCode: "duplicate_or_empty",
			wantSkip: true,
		},
		{
			name:     "empty items remains warning",
			warning:  "AI extract shopee email: empty items",
			wantCode: "empty_items",
			wantSkip: false,
		},
		{
			name:     "no attachment is user skipped",
			warning:  "no supported attachment",
			wantCode: "no_supported_attachment",
			wantSkip: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, label, userSkipped := classifyDispatchWarning(tt.warning)
			if code != tt.wantCode {
				t.Fatalf("code = %q, want %q", code, tt.wantCode)
			}
			if label == "" {
				t.Fatal("label should be user-readable")
			}
			if userSkipped != tt.wantSkip {
				t.Fatalf("userSkipped = %v, want %v", userSkipped, tt.wantSkip)
			}
		})
	}
}

func TestShouldBypassDuplicatePrecheckForShopeePaymentOnly(t *testing.T) {
	tests := []struct {
		name    string
		cfg     PollConfig
		subject string
		want    bool
	}{
		{
			name:    "shopee payment confirmation",
			cfg:     PollConfig{Channel: "shopee"},
			subject: "ยืนยันการชำระเงินคำสั่งซื้อหมายเลข #26061316DWD4GG",
			want:    true,
		},
		{
			name:    "shopee shipped confirmation keeps precheck",
			cfg:     PollConfig{Channel: "shopee"},
			subject: "คำสั่งซื้อ #2601AAA ถูกจัดส่งแล้ว",
			want:    false,
		},
		{
			name:    "shopee non shipped order email keeps precheck",
			cfg:     PollConfig{Channel: "shopee"},
			subject: "ยืนยันคำสั่งซื้อ Shopee",
			want:    false,
		},
		{
			name:    "lazada payment text does not bypass",
			cfg:     PollConfig{Channel: "lazada"},
			subject: "ยืนยันการชำระเงินคำสั่งซื้อหมายเลข #26061316DWD4GG",
			want:    false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldBypassDuplicatePrecheck(tt.cfg, tt.subject); got != tt.want {
				t.Fatalf("shouldBypassDuplicatePrecheck = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPollResultStatus(t *testing.T) {
	tests := []struct {
		name string
		res  PollResult
		want string
	}{
		{
			name: "duplicate only is not error",
			res: PollResult{
				MessagesFound: 3,
				Skipped:       3,
				Summary:       modelsSummary(3, 0, 3, 3, 0),
			},
			want: "no_new_mail",
		},
		{
			name: "created bill is ok",
			res: PollResult{
				MessagesFound: 1,
				Processed:     1,
				Summary:       modelsSummary(1, 1, 0, 0, 0),
			},
			want: "ok",
		},
		{
			name: "updated existing bill status is ok",
			res: PollResult{
				MessagesFound: 1,
				Processed:     1,
				Summary: models.IMAPPollSummary{
					Scanned:         1,
					UpdatedExisting: 1,
				},
			},
			want: "ok",
		},
		{
			name: "processing warning",
			res: PollResult{
				MessagesFound:   1,
				Skipped:         1,
				ProcessWarnings: []string{"empty orders"},
			},
			want: "warning",
		},
		{
			name: "shutdown cancel is interrupted",
			res: PollResult{
				Err: context.Canceled,
			},
			want: "interrupted",
		},
		{
			name: "shutdown cancel after progress is partial",
			res: PollResult{
				Err:         context.Canceled,
				Skipped:     10,
				LastSeenUID: 42,
			},
			want: "partial",
		},
		{
			name: "large mailbox is backlog",
			res: PollResult{
				MessagesFound: 200,
				Skipped:       150,
				Backlog:       50,
				Limited:       true,
				Summary:       modelsSummary(150, 0, 150, 150, 0),
			},
			want: "backlog",
		},
		{
			name: "created bills with backlog stays backlog when skips are user-level",
			res: PollResult{
				MessagesFound: 150,
				Processed:     57,
				Skipped:       93,
				Backlog:       520,
				Limited:       true,
				Summary:       modelsSummary(150, 57, 2, 91, 0),
			},
			want: "backlog",
		},
		{
			name: "created bills with true processing failure still needs attention",
			res: PollResult{
				MessagesFound:   150,
				Processed:       57,
				Skipped:         93,
				Backlog:         520,
				Limited:         true,
				Summary:         modelsSummary(150, 57, 2, 90, 1),
				ProcessWarnings: []string{"empty orders"},
			},
			want: "warning",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.res.Status(); got != tt.want {
				t.Fatalf("Status() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCompactWarningLabelTruncatesLongAIOutput(t *testing.T) {
	warning := "AI extract shopee_shipped: " + strings.Repeat("x", 2000)
	got := compactWarningLabel(warning)
	if len([]rune(got)) > 950 {
		t.Fatalf("compactWarningLabel length = %d, want truncated", len([]rune(got)))
	}
	if !strings.Contains(got, "truncated") {
		t.Fatalf("compactWarningLabel = %q, want truncation marker", got)
	}
}

func TestCandidateUIDsUsesProgressAndLimit(t *testing.T) {
	got := candidateUIDs([]imap.UID{1, 2, 3, 4}, 2, 1)
	if got.Total != 2 {
		t.Fatalf("Total = %d, want 2", got.Total)
	}
	if got.Backlog != 1 {
		t.Fatalf("Backlog = %d, want 1", got.Backlog)
	}
	if !got.Limited {
		t.Fatal("Limited = false, want true")
	}
	if len(got.Selected) != 1 || got.Selected[0] != 3 {
		t.Fatalf("Selected = %#v, want [3]", got.Selected)
	}
}

func TestClassifyLazadaEnvelope(t *testing.T) {
	tests := []struct {
		name     string
		subject  string
		from     string
		allowed  []string
		wantOK   bool
		wantCode string
	}{
		{
			name:    "order confirmation accepted",
			subject: "ยืนยันคำสั่งซื้อหมายเลข 1107473377495692",
			from:    "noreply@support.lazada.co.th",
			allowed: []string{"support.lazada.co.th"},
			wantOK:  true,
		},
		{
			name:    "shipped accepted",
			subject: "คำสั่งซื้อหมายเลข 1107071348695692 ได้รับการจัดส่งเรียบร้อยแล้ว",
			from:    "noreply@support.lazada.co.th",
			allowed: []string{"support.lazada.co.th"},
			wantOK:  true,
		},
		{
			name:     "e invoice skipped",
			subject:  "E-invoice for order 1107071348695692",
			from:     "noreply@support.lazada.co.th",
			allowed:  []string{"support.lazada.co.th"},
			wantOK:   false,
			wantCode: "lazada_noise",
		},
		{
			name:     "dispute sender skipped even when lazada domain",
			subject:  "ยืนยันคำสั่งซื้อหมายเลข 1107473377495692",
			from:     "DisputeTH@care.lazada.com",
			allowed:  []string{"care.lazada.com"},
			wantOK:   false,
			wantCode: "lazada_sender_not_allowed",
		},
		{
			name:     "broad lazada subject skipped",
			subject:  "แจ้งกำหนดการจัดส่งสินค้าใหม่!",
			from:     "noreply@support.lazada.co.th",
			allowed:  []string{"support.lazada.co.th"},
			wantOK:   false,
			wantCode: "lazada_noise",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, label, ok := classifyLazadaEnvelope(tt.subject, tt.from, tt.allowed)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v (code=%q label=%q)", ok, tt.wantOK, code, label)
			}
			if tt.wantCode != "" && code != tt.wantCode {
				t.Fatalf("code = %q, want %q", code, tt.wantCode)
			}
			if !ok && label == "" {
				t.Fatal("skipped Lazada messages should have a user-readable label")
			}
		})
	}
}

func TestConfiguredMaxMessagesPerRunForLazadaCapsGlobal(t *testing.T) {
	t.Setenv("IMAP_MAX_MESSAGES_PER_RUN", "150")
	t.Setenv("LAZADA_EMAIL_MAX_MESSAGES_PER_RUN", "12")
	if got := configuredMaxMessagesPerRunForChannel("lazada"); got != 12 {
		t.Fatalf("lazada max = %d, want 12", got)
	}
	if got := configuredMaxMessagesPerRunForChannel("shopee"); got != 150 {
		t.Fatalf("shopee max = %d, want 150", got)
	}
}

func TestConfiguredMaxMessagesPerRunForLazadaNeverExceedsGlobal(t *testing.T) {
	t.Setenv("IMAP_MAX_MESSAGES_PER_RUN", "25")
	t.Setenv("LAZADA_EMAIL_MAX_MESSAGES_PER_RUN", "100")
	if got := configuredMaxMessagesPerRunForChannel("lazada"); got != 25 {
		t.Fatalf("lazada max = %d, want global cap 25", got)
	}
}

func TestConfiguredMaxMessagesPerRunForLazadaDefault(t *testing.T) {
	t.Setenv("IMAP_MAX_MESSAGES_PER_RUN", "150")
	t.Setenv("LAZADA_EMAIL_MAX_MESSAGES_PER_RUN", "")
	if got := configuredMaxMessagesPerRunForChannel("lazada"); got != 30 {
		t.Fatalf("lazada default max = %d, want 30", got)
	}
}

func modelsSummary(scanned, created, alreadyProcessed, skippedUser, failed int) models.IMAPPollSummary {
	return models.IMAPPollSummary{
		Scanned:          scanned,
		Created:          created,
		AlreadyProcessed: alreadyProcessed,
		SkippedUser:      skippedUser,
		Failed:           failed,
	}
}
