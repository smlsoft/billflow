package handlers

import (
	"billflow/internal/middleware"
	"billflow/internal/services/shippingcheck"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestShippingSettingsAdminOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, role := range []string{"staff", "viewer"} {
		db, m, _ := sqlmock.New()
		h := NewShippingCheckHandler(shippingcheck.NewStore(db), nil, nil, zap.NewNop())
		r := gin.New()
		r.Use(func(c *gin.Context) { c.Set("user_role", role) })
		r.GET("/settings", middleware.RequireRole("admin"), h.Settings)
		r.PUT("/settings", middleware.RequireRole("admin"), h.Save)
		r.POST("/preview", middleware.RequireRole("admin"), h.Preview)
		for _, req := range []struct{ method, path string }{{"GET", "/settings"}, {"PUT", "/settings"}, {"POST", "/preview"}} {
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(req.method, req.path, nil))
			if w.Code != 403 {
				t.Fatalf("role=%s status=%d", role, w.Code)
			}
		}
		if err := m.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
		db.Close()
	}
}
func TestShippingSettingsReadDoesNotExposeMailboxSecrets(t *testing.T) {
	db, m, _ := sqlmock.New()
	defer db.Close()
	m.ExpectQuery("SELECT value FROM app_settings").WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(`{"profiles":[],"policies":[],"revision":"r1"}`))
	h := NewShippingCheckHandler(shippingcheck.NewStore(db), nil, nil, zap.NewNop())
	r := gin.New()
	r.GET("/", h.Settings)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != 200 || strings.Contains(w.Body.String(), "password") {
		t.Fatal(w.Body.String())
	}
	if err := m.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
func TestShippingPreviewRejectsInvalidBillWithoutWrites(t *testing.T) {
	db, m, _ := sqlmock.New()
	defer db.Close()
	h := NewShippingCheckHandler(shippingcheck.NewStore(db), nil, nil, zap.NewNop())
	r := gin.New()
	r.POST("/", h.Preview)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/", strings.NewReader(`{"bill_id":"not-a-uuid"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Fatal(w.Code)
	}
	if err := m.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestShippingPreviewUsesDraftWithoutWritingBill(t *testing.T) {
	db, m, _ := sqlmock.New()
	defer db.Close()
	bill := "00000000-0000-0000-0000-000000000001"
	account := "00000000-0000-0000-0000-000000000002"
	m.ExpectQuery("SELECT source,bill_type").WithArgs(bill).WillReturnRows(sqlmock.NewRows([]string{"source", "bill_type", "account", "message"}).AddRow("lazada_email", "purchase", account, "original"))
	m.ExpectQuery("SELECT a.id FROM bill_artifacts").WithArgs(bill, "original").WillReturnRows(sqlmock.NewRows([]string{"id"}))
	h := NewShippingCheckHandler(shippingcheck.NewStore(db), nil, nil, zap.NewNop())
	r := gin.New()
	r.POST("/", h.Preview)
	body := `{"account_id":"` + account + `","bill_id":"` + bill + `","config":{"profiles":[{"id":"office","name":"Office","house":"368","moo":"10","district":"D","amphoe":"A","province":"P","postcode":"74110"}],"policies":[{"account_id":"` + account + `","profile_id":"office","recipients":["PD01"],"enabled":false}]}}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "unverifiable") {
		t.Fatal(w.Code, w.Body.String())
	}
	if err := m.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
