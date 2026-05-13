package emailservice

import (
	"testing"

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

func TestCandidateUIDsSkipsSeenAndLimitsBacklog(t *testing.T) {
	all := []imap.UID{10, 11, 12, 13, 14}
	got := candidateUIDs(all, 11, 2)
	if got.Total != 3 {
		t.Fatalf("Total = %d, want 3", got.Total)
	}
	if !got.Limited {
		t.Fatal("Limited = false, want true")
	}
	if got.Backlog != 1 {
		t.Fatalf("Backlog = %d, want 1", got.Backlog)
	}
	if len(got.Selected) != 2 || got.Selected[0] != 12 || got.Selected[1] != 13 {
		t.Fatalf("Selected = %#v, want [12 13]", got.Selected)
	}
}

func TestPollResultStatusBacklogAndPartial(t *testing.T) {
	backlog := PollResult{Limited: true}
	if got := backlog.Status(); got != "backlog" {
		t.Fatalf("backlog status = %q, want backlog", got)
	}

	partial := PollResult{
		Err:          assertErr("closed"),
		FailureStage: "fetch",
		Skipped:      25,
		LastSeenUID:  99,
	}
	if got := partial.Status(); got != "partial" {
		t.Fatalf("partial status = %q, want partial", got)
	}
}

func TestConfiguredMaxMessagesPerRun(t *testing.T) {
	t.Setenv("IMAP_MAX_MESSAGES_PER_RUN", "10")
	if got := configuredMaxMessagesPerRun(); got != minMaxMessagesPerRun {
		t.Fatalf("low env clamped to %d, got %d", minMaxMessagesPerRun, got)
	}

	t.Setenv("IMAP_MAX_MESSAGES_PER_RUN", "999")
	if got := configuredMaxMessagesPerRun(); got != maxMaxMessagesPerRun {
		t.Fatalf("high env clamped to %d, got %d", maxMaxMessagesPerRun, got)
	}

	t.Setenv("IMAP_MAX_MESSAGES_PER_RUN", "200")
	if got := configuredMaxMessagesPerRun(); got != 200 {
		t.Fatalf("configured max = %d, want 200", got)
	}
}

type assertErr string

func (e assertErr) Error() string { return string(e) }
