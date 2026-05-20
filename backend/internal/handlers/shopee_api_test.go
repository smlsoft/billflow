package handlers

import (
	"strings"
	"testing"
	"time"
)

func TestParseShopeeAPIRangeRejectsInvalidDate(t *testing.T) {
	_, _, err := parseShopeeAPIRange("2026/05/01", "2026-05-02")
	if err == nil {
		t.Fatal("expected invalid date error")
	}
}

func TestParseShopeeAPIRangeRejectsInvertedRange(t *testing.T) {
	_, _, err := parseShopeeAPIRange("2026-05-03", "2026-05-02")
	if err == nil {
		t.Fatal("expected inverted range error")
	}
}

func TestParseShopeeAPIRangeRejectsMoreThan15Days(t *testing.T) {
	_, _, err := parseShopeeAPIRange("2026-05-01", "2026-05-17")
	if err == nil {
		t.Fatal("expected max range error")
	}
}

func TestParseShopeeAPIRangeAccepts15DayWindow(t *testing.T) {
	from, to, err := parseShopeeAPIRange("2026-05-01", "2026-05-15")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if from.Format("2006-01-02") != "2026-05-01" || to.Format("2006-01-02") != "2026-05-15" {
		t.Fatalf("range = %s..%s", from.Format(time.RFC3339), to.Format(time.RFC3339))
	}
}

func TestShopeeAPIReadinessBlocksBadLiveBaseURL(t *testing.T) {
	status := ShopeeAPIStatus{
		Enabled:     true,
		Configured:  true,
		Environment: "live",
		BaseURL:     "https://openplatform.sandbox.test-stable.shopee.sg",
		RedirectURL: "https://example.com/api/shopee-api/callback",
	}
	status.finalizeReadiness(time.Date(2026, 5, 20, 9, 0, 0, 0, time.UTC))
	if status.CanConnect {
		t.Fatal("CanConnect should be false when live env still points at sandbox base URL")
	}
	if !strings.Contains(status.BlockingReason, "environment=live") {
		t.Fatalf("BlockingReason = %q", status.BlockingReason)
	}
}

func TestShopeeAPIReadinessAllowsRefreshRequiredToken(t *testing.T) {
	now := time.Date(2026, 5, 20, 9, 0, 0, 0, time.UTC)
	status := ShopeeAPIStatus{
		Enabled:          true,
		Configured:       true,
		Environment:      "live",
		BaseURL:          "https://partner.shopeemobile.com",
		RedirectURL:      "https://example.com/api/shopee-api/callback",
		Connected:        true,
		ShopID:           123,
		AccessExpiresAt:  now.Add(-time.Minute).Format(time.RFC3339),
		RefreshExpiresAt: now.Add(24 * time.Hour).Format(time.RFC3339),
	}
	status.finalizeReadiness(now)
	if !status.CanFetch {
		t.Fatal("CanFetch should be true because backend can refresh the access token")
	}
	if status.TokenState != "refresh_required" {
		t.Fatalf("TokenState = %q", status.TokenState)
	}
}

func TestShopeeAPIErrorMessageMapsRateLimit(t *testing.T) {
	got := shopeeAPIErrorMessage(nil, "shopee http 429: too many requests")
	if got.Code != "rate_limited" || !got.Retryable {
		t.Fatalf("error view = %+v", got)
	}
}
