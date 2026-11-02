package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"billflow/internal/middleware"
	"billflow/internal/models"
	"billflow/internal/repository"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
)

func TestPaymentMethodFilterRowsUseConfigAndExposeOnlyPaymentPolicy(t *testing.T) {
	rows := paymentMethodFilterRows([]*models.ChannelDefault{
		{Channel: "shopee_shipped", BillType: "purchase", PartyCode: "PRIVATE-CODE", PrintPolicy: models.MarketplacePrintPolicy{PaymentMethods: []string{"TT9610", "TT4553"}}},
		{Channel: "lazada_email", BillType: "purchase", PrintPolicy: models.MarketplacePrintPolicy{PaymentMethods: []string{"TT3132"}}},
		{Channel: "manual", BillType: "purchase", PrintPolicy: models.MarketplacePrintPolicy{PaymentMethods: []string{"IGNORE"}}},
	})
	if len(rows) != 2 || !reflect.DeepEqual(rows[0].PrintPolicy.PaymentMethods, []string{"TT9610", "TT4553"}) || !reflect.DeepEqual(rows[1].PrintPolicy.PaymentMethods, []string{"TT3132"}) {
		t.Fatalf("rows=%+v", rows)
	}
	data, err := json.Marshal(rows)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "PRIVATE-CODE") || strings.Contains(string(data), "party_code") {
		t.Fatalf("admin settings leaked: %s", data)
	}
}

func TestListPaymentMethodOptionsReadersAndSafeFailures(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, role := range []string{"admin", "staff", "viewer"} {
		t.Run(role, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			mock.ExpectQuery("FROM channel_defaults").WillReturnRows(sqlmock.NewRows([]string{"channel"}))
			h := NewChannelDefaultsHandler(repository.NewChannelDefaultRepo(db), nil, nil)
			r := gin.New()
			r.Use(func(c *gin.Context) { c.Set("user_role", role) })
			r.GET("/options", middleware.RequireRole("admin", "staff", "viewer"), h.ListPaymentMethodOptions)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/options", nil))
			if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" || !strings.Contains(w.Body.String(), `"payment_methods"`) {
				t.Fatalf("response=%d %s", w.Code, w.Body.String())
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
	t.Run("database error does not disclose internals", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		mock.ExpectQuery("FROM channel_defaults").WillReturnError(errors.New("private database connection details"))
		h := NewChannelDefaultsHandler(repository.NewChannelDefaultRepo(db), nil, nil)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		h.ListPaymentMethodOptions(c)
		if w.Code != 500 || strings.Contains(w.Body.String(), "private") {
			t.Fatalf("response=%d %s", w.Code, w.Body.String())
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	})
}

func TestPaymentMethodFilterRowsMissingConfigUsesCanonicalFallback(t *testing.T) {
	rows := paymentMethodFilterRows(nil)
	if len(rows) != 2 || rows[0].Channel != "shopee_shipped" || rows[1].Channel != "lazada_email" {
		t.Fatalf("missing channels: %+v", rows)
	}
	if !reflect.DeepEqual(rows[0].PrintPolicy.PaymentMethods, models.DefaultMarketplacePrintPaymentMethods()) {
		t.Fatal("missing config must use shared defaults")
	}
}
