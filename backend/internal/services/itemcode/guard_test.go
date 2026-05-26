package itemcode

import "testing"

func TestInspectHiddenItemCode(t *testing.T) {
	tests := []struct {
		name  string
		code  string
		want  bool
		clean string
	}{
		{name: "clean", code: "BILLS002", want: false, clean: ""},
		{name: "thai combining mark", code: "\u0E3ABILLS002", want: true, clean: "BILLS002"},
		{name: "bom", code: "\uFEFFBILLS002", want: true, clean: "BILLS002"},
		{name: "zero width", code: "BILLS\u200B002", want: true, clean: "BILLS002"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Inspect(tt.code)
			if got.HasHiddenChars != tt.want {
				t.Fatalf("HasHiddenChars = %v, want %v", got.HasHiddenChars, tt.want)
			}
			if got.CleanItemCode != tt.clean {
				t.Fatalf("CleanItemCode = %q, want %q", got.CleanItemCode, tt.clean)
			}
		})
	}
}
