package handlers

import (
	"billflow/internal/middleware"
	"billflow/internal/repository"
	"billflow/internal/services/artifact"
	"billflow/internal/services/shippingcheck"
	"database/sql/driver"
	"encoding/json"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestShippingManualCheckDeniesViewer(t *testing.T) {
	h := NewShippingCheckHandler(nil, nil, nil, zap.NewNop())
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("user_role", "viewer") })
	r.POST("/bills/:id/shipping-checks", middleware.RequireRole("admin", "staff"), h.ManualCheck)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("POST", "/bills/00000000-0000-0000-0000-000000000001/shipping-checks", nil))
	if w.Code != 403 {
		t.Fatal(w.Code)
	}
}
func TestShippingSourceCanBeReadBeforeCheckingHistoricalBill(t *testing.T) {
	db, m, _ := sqlmock.New()
	defer db.Close()
	bill := "00000000-0000-0000-0000-000000000001"
	account := "00000000-0000-0000-0000-000000000002"
	m.ExpectQuery("SELECT source,bill_type").WithArgs(bill).WillReturnRows(sqlmock.NewRows([]string{"source", "bill_type", "account", "message"}).AddRow("lazada_email", "purchase", account, "original"))
	m.ExpectQuery("SELECT a.id FROM bill_artifacts").WithArgs(bill, "original").WillReturnRows(sqlmock.NewRows([]string{"id"}))
	m.ExpectQuery("SELECT value FROM app_settings").WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(`{"profiles":[],"policies":[]}`))
	h := NewShippingCheckHandler(shippingcheck.NewStore(db), nil, nil, zap.NewNop())
	r := gin.New()
	r.GET("/bills/:id/shipping-details", h.SourceDetails)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/bills/"+bill+"/shipping-details", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"manual_check_available":false`) {
		t.Fatal(w.Code, w.Body.String())
	}
	if err := m.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
func TestManualCheckWithMissingConfigDoesNotWriteHistoricalBill(t *testing.T) {
	db, m, _ := sqlmock.New()
	defer db.Close()
	bill := "00000000-0000-0000-0000-000000000001"
	account := "00000000-0000-0000-0000-000000000002"
	m.ExpectQuery("SELECT source,bill_type").WithArgs(bill).WillReturnRows(sqlmock.NewRows([]string{"source", "bill_type", "account", "message"}).AddRow("lazada_email", "purchase", account, "original"))
	m.ExpectQuery("SELECT a.id FROM bill_artifacts").WillReturnRows(sqlmock.NewRows([]string{"id"}))
	m.ExpectQuery("SELECT value FROM app_settings").WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(`{"profiles":[],"policies":[]}`))
	h := NewShippingCheckHandler(shippingcheck.NewStore(db), nil, nil, zap.NewNop())
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("user_id", account) })
	r.POST("/bills/:id/shipping-checks", h.ManualCheck)
	req := httptest.NewRequest("POST", "/bills/"+bill+"/shipping-checks", strings.NewReader(`{"request_id":"00000000-0000-0000-0000-000000000003"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Fatal(w.Code, w.Body.String())
	}
	if err := m.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestManualShippingCheckReadsRetainedHTMLAndRecordsActor(t *testing.T) {
	db, m, _ := sqlmock.New()
	defer db.Close()
	bill := "00000000-0000-0000-0000-000000000001"
	account := "00000000-0000-0000-0000-000000000002"
	user := "00000000-0000-0000-0000-000000000003"
	art := "00000000-0000-0000-0000-000000000004"
	request := "00000000-0000-0000-0000-000000000005"
	root := t.TempDir()
	body := `<h3>รายละเอียดการส่งสินค้า</h3><p>ชื่อผู้รับ: PD01</p><p>ที่อยู่: 368 หมู่10 ตำบลคลองมะเดื่อ อำเภอกระทุ่มแบน จังหวัดสมุทรสาคร 74110</p><p>เบอร์โทร: 000</p>`
	if err := os.WriteFile(filepath.Join(root, "source.html"), []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	m.ExpectQuery("SELECT source,bill_type").WithArgs(bill).WillReturnRows(sqlmock.NewRows([]string{"source", "bill_type", "account", "message"}).AddRow("shopee_shipped", "purchase", account, "original"))
	m.ExpectQuery("SELECT a.id FROM bill_artifacts").WithArgs(bill, "original").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(art))
	m.ExpectQuery("SELECT id, bill_id, kind").WithArgs(art).WillReturnRows(sqlmock.NewRows([]string{"id", "bill_id", "kind", "filename", "content_type", "size_bytes", "sha256", "storage_path", "source_meta", "created_at"}).AddRow(art, bill, "email_html", "source.html", "text/html", len(body), "", "source.html", nil, time.Now()))
	config := `{"revision":"current-rules","profiles":[{"id":"office","name":"Office","house":"368","moo":"10","district":"คลองมะเดื่อ","amphoe":"กระทุ่มแบน","province":"สมุทรสาคร","postcode":"74110"}],"policies":[{"account_id":"` + account + `","profile_id":"office","recipients":["PD01"],"enabled":false}]}`
	m.ExpectQuery("SELECT value FROM app_settings").WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(config))
	m.ExpectBegin()
	m.ExpectExec("SELECT id FROM bills").WithArgs(bill).WillReturnResult(sqlmock.NewResult(0, 1))
	m.ExpectExec("INSERT INTO bill_shipping_checks").WithArgs(bill, "manual:"+request, "original", art, sqlmock.AnyArg(), manualShippingResultArgument{}).WillReturnResult(sqlmock.NewResult(0, 1))
	m.ExpectExec("UPDATE bills SET raw_data=jsonb_set").WithArgs(bill, "match", "").WillReturnResult(sqlmock.NewResult(0, 1))
	m.ExpectExec("INSERT INTO audit_logs").WithArgs(bill, user, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	m.ExpectCommit()
	repo := repository.NewBillArtifactRepo(db)
	h := NewShippingCheckHandler(shippingcheck.NewStore(db), repo, artifact.New(root, 1024*1024, repo, zap.NewNop()), zap.NewNop())
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("user_id", user); c.Set("user_email", "admin@example.invalid") })
	r.POST("/bills/:id/shipping-checks", h.ManualCheck)
	req := httptest.NewRequest("POST", "/bills/"+bill+"/shipping-checks", strings.NewReader(`{"request_id":"`+request+`"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if err := m.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

type manualShippingResultArgument struct{}

func (manualShippingResultArgument) Match(value driver.Value) bool {
	s, ok := value.(string)
	if !ok {
		return false
	}
	var r shippingcheck.Result
	return json.Unmarshal([]byte(s), &r) == nil && r.Status == "match" && r.Trigger == "manual" && r.CheckedBy == "admin@example.invalid" && r.Recipient == "PD01"
}
