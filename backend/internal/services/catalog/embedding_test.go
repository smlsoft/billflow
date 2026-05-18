package catalog

import (
	"net/http"
	"strings"
	"testing"
)

func TestEmbeddingOpenRouterAppAttributionHeaders(t *testing.T) {
	svc := NewEmbeddingService("key").
		WithAppAttribution("BillFlow", "https://example.test")
	req, err := http.NewRequest(http.MethodPost, embeddingAPIURL, nil)
	if err != nil {
		t.Fatal(err)
	}

	svc.setOpenRouterHeaders(req)

	if got := req.Header.Get("X-OpenRouter-Title"); got != "BillFlow" {
		t.Fatalf("X-OpenRouter-Title = %q, want BillFlow", got)
	}
	if got := req.Header.Get("X-Title"); got != "BillFlow" {
		t.Fatalf("X-Title = %q, want BillFlow", got)
	}
	if got := req.Header.Get("HTTP-Referer"); got != "https://example.test" {
		t.Fatalf("HTTP-Referer = %q, want https://example.test", got)
	}
}

func TestEmbeddingSessionIDIsBillFlowScoped(t *testing.T) {
	sessionID := newOpenRouterSessionID("Catalog Embed All", "Pending")
	if !strings.HasPrefix(sessionID, "billflow:catalog-embed-all:pending:") {
		t.Fatalf("sessionID = %q, want billflow scoped prefix", sessionID)
	}
}
