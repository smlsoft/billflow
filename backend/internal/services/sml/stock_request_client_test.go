package sml

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStockRequestURLAllowsBaseOrFullEndpoint(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "base url appends default endpoint",
			in:   "http://sml.local:9980",
			want: "http://sml.local:9980/SMLJavaWebService/rest/v1/processstockrequest",
		},
		{
			name: "trailing slash base url appends default endpoint once",
			in:   "http://sml.local:9980/",
			want: "http://sml.local:9980/SMLJavaWebService/rest/v1/processstockrequest",
		},
		{
			name: "full endpoint is used as-is",
			in:   "http://sml.local:9980/custom/processstockrequest",
			want: "http://sml.local:9980/custom/processstockrequest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stockRequestURL(tt.in); got != tt.want {
				t.Fatalf("stockRequestURL(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestProcessStockRequestNotFoundExplainsRequiredSMLJavaWebService(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != stockRequestPath {
			t.Fatalf("path = %q, want %q", r.URL.Path, stockRequestPath)
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	client := NewStockRequestClient(srv.URL, "provider", "database", nil)
	err := client.ProcessStockRequest(context.Background(), []string{"ITEM001"})
	if err == nil {
		t.Fatal("expected 404 error")
	}
	if !strings.Contains(err.Error(), "HTTP 404") {
		t.Fatalf("error = %q, want HTTP 404", err.Error())
	}
	if !strings.Contains(err.Error(), "SMLJavaWebService") || !strings.Contains(err.Error(), "processstockrequest") {
		t.Fatalf("error = %q, want SMLJavaWebService processstockrequest hint", err.Error())
	}
	if got := StockRequestErrorHint(err); got != stockRequestNotFoundHint {
		t.Fatalf("StockRequestErrorHint = %q, want %q", got, stockRequestNotFoundHint)
	}
}
