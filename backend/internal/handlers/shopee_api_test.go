package handlers

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"go.uber.org/zap"

	"billflow/internal/config"
	"billflow/internal/repository"
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

func TestConsumeSolePendingShopeeOAuthStateConsumesOnlyUnambiguousState(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	redirectURL := "https://example.com/api/shopee-api/callback"
	handler := &ShopeeImportHandler{
		billRepo: repository.NewBillRepo(db),
		cfg: &config.Config{
			ShopeeOpenAPIEnv:      "live",
			ShopeeOpenAPIRedirect: redirectURL,
		},
		logger: zap.NewNop(),
	}

	mock.ExpectQuery("WITH candidates AS").
		WithArgs("live", redirectURL).
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "environment", "redirect_url"}).
			AddRow("8bdb5d26-86fc-4a58-a6a9-0376a48180a1", "live", redirectURL))

	got, err := handler.consumeSolePendingShopeeOAuthState(context.Background())
	if err != nil {
		t.Fatalf("consumeSolePendingShopeeOAuthState: %v", err)
	}
	if got.UserID != "8bdb5d26-86fc-4a58-a6a9-0376a48180a1" || got.Environment != "live" || got.RedirectURL != redirectURL {
		t.Fatalf("state = %+v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func TestConsumeSolePendingShopeeOAuthStateRejectsMissingOrAmbiguousState(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	redirectURL := "https://example.com/api/shopee-api/callback"
	handler := &ShopeeImportHandler{
		billRepo: repository.NewBillRepo(db),
		cfg: &config.Config{
			ShopeeOpenAPIEnv:      "live",
			ShopeeOpenAPIRedirect: redirectURL,
		},
		logger: zap.NewNop(),
	}

	mock.ExpectQuery("WITH candidates AS").
		WithArgs("live", redirectURL).
		WillReturnError(sql.ErrNoRows)

	if _, err := handler.consumeSolePendingShopeeOAuthState(context.Background()); err == nil {
		t.Fatal("expected missing/ambiguous pending state error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func TestResolveShopeeAPIConnectionRequiresSelectionWhenMultipleActive(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	handler := &ShopeeImportHandler{
		billRepo: repository.NewBillRepo(db),
		cfg:      &config.Config{ShopeeOpenAPIEnv: "live"},
		logger:   zap.NewNop(),
	}

	now := time.Date(2026, 5, 21, 9, 0, 0, 0, time.UTC)
	mock.ExpectQuery("FROM shopee_api_connections").
		WithArgs("live").
		WillReturnRows(newShopeeConnectionRows().
			AddRow("11111111-1111-1111-1111-111111111111", int64(1001), nil, "Shop A", "Shop A", "access-a", "refresh-a", now.Add(time.Hour), now.Add(24*time.Hour), "live", nil, nil, "", "", "", now, now).
			AddRow("22222222-2222-2222-2222-222222222222", int64(1002), nil, "Shop B", "Shop B", "access-b", "refresh-b", now.Add(time.Hour), now.Add(24*time.Hour), "live", nil, nil, "", "", "", now, now))

	_, err = handler.resolveShopeeAPIConnection(context.Background(), "")
	if err == nil || !strings.Contains(err.Error(), "เลือกร้าน") {
		t.Fatalf("expected explicit shop selection error, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func TestResolveShopeeAPIConnectionByIDReturnsSelectedShop(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	handler := &ShopeeImportHandler{
		billRepo: repository.NewBillRepo(db),
		cfg:      &config.Config{ShopeeOpenAPIEnv: "live"},
		logger:   zap.NewNop(),
	}

	now := time.Date(2026, 5, 21, 9, 0, 0, 0, time.UTC)
	connectionID := "33333333-3333-3333-3333-333333333333"
	mock.ExpectQuery("WHERE id = \\$1::uuid").
		WithArgs(connectionID, "live").
		WillReturnRows(newShopeeConnectionRows().
			AddRow(connectionID, int64(1029622928), int64(555), "semicolon.con", "Semicolon Main", "access", "refresh", now.Add(time.Hour), now.Add(24*time.Hour), "live", nil, now, "ok", "", "", now, now))

	got, err := handler.resolveShopeeAPIConnection(context.Background(), connectionID)
	if err != nil {
		t.Fatalf("resolveShopeeAPIConnection: %v", err)
	}
	if got.ShopID != 1029622928 || got.Label != "Semicolon Main" {
		t.Fatalf("connection = %+v", got)
	}
	if !got.MerchantID.Valid || got.MerchantID.Int64 != 555 {
		t.Fatalf("merchant_id = %+v", got.MerchantID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func newShopeeConnectionRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id",
		"shop_id",
		"merchant_id",
		"shop_name",
		"label",
		"access_token",
		"refresh_token",
		"access_expires_at",
		"refresh_expires_at",
		"environment",
		"disabled_at",
		"last_sync_at",
		"last_sync_status",
		"last_sync_error",
		"last_error_code",
		"connected_at",
		"updated_at",
	})
}
