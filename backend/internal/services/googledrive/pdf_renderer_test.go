package googledrive

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestHTTPPDFRendererRendersPDFAndWarnings(t *testing.T) {
	warnings := url.QueryEscape(`["โหลดรูปจาก cdn.example ไม่สำเร็จ"]`)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/render" || r.Header.Get("X-BillFlow-Renderer-Token") != "test-token" {
			t.Fatalf("unexpected renderer request: %s", r.URL)
		}
		w.Header().Set("X-BillFlow-Render-Warnings", warnings)
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write([]byte("%PDF-1.7 test"))
	}))
	defer server.Close()

	got, err := newHTTPPDFRenderer(server.URL, "test-token").Render(context.Background(), "<p>สวัสดี</p>")
	if err != nil {
		t.Fatal(err)
	}
	if string(got.PDF) != "%PDF-1.7 test" || len(got.Warnings) != 1 {
		t.Fatalf("unexpected result: %#v", got)
	}
}

func TestHTTPPDFRendererRejectsInvalidResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("not a PDF"))
	}))
	defer server.Close()
	if _, err := newHTTPPDFRenderer(server.URL, "test-token").Render(context.Background(), "<p>x</p>"); err == nil {
		t.Fatal("expected invalid PDF error")
	}
}
