package emailservice

import "testing"

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
