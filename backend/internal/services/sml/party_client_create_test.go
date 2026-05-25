package sml

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"
)

func TestPartyClientCreateCustomer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/ar/customers" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("X-Tenant"); got != "sml1_2026" {
			t.Fatalf("X-Tenant = %q", got)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"success":true,"data":{"code":"ARNEW","name_1":"ลูกค้าใหม่","status":0}}`))
	}))
	defer srv.Close()

	client := NewPartyClient(PartyConfig{BaseURL: srv.URL, GUID: "smlx", Database: "sml1_2026"}, zap.NewNop())
	status, party, err := client.CreateCustomer(context.Background(), " ARNEW ", " ลูกค้าใหม่ ")
	if err != nil {
		t.Fatal(err)
	}
	if status != http.StatusCreated {
		t.Fatalf("status = %d", status)
	}
	if party.Code != "ARNEW" || party.Name != "ลูกค้าใหม่" {
		t.Fatalf("party = %+v", party)
	}
}

func TestPartyClientCreateSupplierDuplicate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"success":false,"error":{"code":"duplicate_supplier_code","message":"supplier code 'V001' already exists"}}`))
	}))
	defer srv.Close()

	client := NewPartyClient(PartyConfig{BaseURL: srv.URL, GUID: "smlx", Database: "sml1_2026"}, zap.NewNop())
	status, party, err := client.CreateSupplier(context.Background(), "V001", "ผู้ขายเดิม")
	if err == nil {
		t.Fatal("expected duplicate error")
	}
	if status != http.StatusConflict {
		t.Fatalf("status = %d", status)
	}
	if party != nil {
		t.Fatalf("party = %+v, want nil", party)
	}
}
