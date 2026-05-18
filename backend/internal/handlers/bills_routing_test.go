package handlers

import (
	"testing"

	"billflow/internal/models"
)

func TestResolveEndpointUsesExplicitEndpointKeyword(t *testing.T) {
	tests := []struct {
		name         string
		endpoint     string
		wantKind     string
		wantOverride string
	}{
		{
			name:         "saleorder keyword path",
			endpoint:     "/SMLJavaRESTService/v3/api/saleorder",
			wantKind:     "saleorder",
			wantOverride: "/SMLJavaRESTService/v3/api/saleorder",
		},
		{
			name:         "saleinvoice keyword path",
			endpoint:     "/SMLJavaRESTService/saleinvoice/v4",
			wantKind:     "saleinvoice",
			wantOverride: "/SMLJavaRESTService/saleinvoice/v4",
		},
		{
			name:         "purchaseorder keyword url",
			endpoint:     "http://sml.local/SMLJavaRESTService/v3/api/purchaseorder",
			wantKind:     "purchaseorder",
			wantOverride: "http://sml.local/SMLJavaRESTService/v3/api/purchaseorder",
		},
		{
			name:         "legacy sale reserve path now falls back to saleorder",
			endpoint:     "/api/sale_reserve",
			wantKind:     "saleorder",
			wantOverride: "",
		},
		{
			name:         "bare saleinvoice token",
			endpoint:     " saleinvoice ",
			wantKind:     "saleinvoice",
			wantOverride: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			def := &models.ChannelDefault{Endpoint: tt.endpoint}
			gotKind, gotOverride := resolveEndpoint(def, "line", "sale")
			if gotKind != tt.wantKind || gotOverride != tt.wantOverride {
				t.Fatalf("resolveEndpoint() = (%q, %q), want (%q, %q)", gotKind, gotOverride, tt.wantKind, tt.wantOverride)
			}
		})
	}
}

func TestResolveEndpointFallsBackBySourceAndBillType(t *testing.T) {
	tests := []struct {
		name     string
		source   string
		billType string
		wantKind string
	}{
		{name: "shopee excel sale defaults to saleorder", source: "shopee", billType: "sale", wantKind: "saleorder"},
		{name: "shopee email sale defaults to saleorder", source: "shopee_email", billType: "sale", wantKind: "saleorder"},
		{name: "lazada excel sale defaults to saleorder", source: "lazada", billType: "sale", wantKind: "saleorder"},
		{name: "tiktok excel sale defaults to saleorder", source: "tiktok", billType: "sale", wantKind: "saleorder"},
		{name: "shopee shipped defaults to purchaseorder", source: "shopee_shipped", billType: "purchase", wantKind: "purchaseorder"},
		{name: "purchase bill defaults to purchaseorder", source: "email", billType: "purchase", wantKind: "purchaseorder"},
		{name: "line sale defaults to saleorder", source: "line", billType: "sale", wantKind: "saleorder"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotKind, gotOverride := resolveEndpoint(nil, tt.source, tt.billType)
			if gotKind != tt.wantKind || gotOverride != "" {
				t.Fatalf("resolveEndpoint() = (%q, %q), want (%q, \"\")", gotKind, gotOverride, tt.wantKind)
			}
		})
	}
}

func TestMapSourceToChannelMatchesRetryLookupKey(t *testing.T) {
	tests := []struct {
		source string
		want   string
	}{
		{source: "shopee", want: "shopee"},
		{source: "shopee_email", want: "shopee_email"},
		{source: "shopee_shipped", want: "shopee_shipped"},
		{source: "lazada", want: "lazada"},
		{source: "tiktok", want: "tiktok"},
		{source: "email", want: "email"},
		{source: "line", want: "line"},
		{source: "manual", want: "line"},
	}

	for _, tt := range tests {
		t.Run(tt.source, func(t *testing.T) {
			if got := mapSourceToChannel(tt.source); got != tt.want {
				t.Fatalf("mapSourceToChannel(%q) = %q, want %q", tt.source, got, tt.want)
			}
		})
	}
}
