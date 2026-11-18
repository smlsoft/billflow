package handlers

import (
	"billflow/internal/repository"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAttentionCountAPI(t *testing.T) {
	for _, failed := range []bool{false, true} {
		db, m, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		if failed {
			m.ExpectQuery("SELECT COUNT").WillReturnError(errors.New("unavailable"))
		} else {
			m.ExpectQuery("SELECT COUNT").WillReturnRows(sqlmock.NewRows([]string{"shipping", "email"}).AddRow(2, 3))
		}
		h := &BillHandler{billRepo: repository.NewBillRepo(db), log: zap.NewNop()}
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/api/bills/attention-counts?search=ignored&page=99", nil)
		h.AttentionCounts(c)
		if failed {
			if w.Code != 500 || strings.Contains(w.Body.String(), `"shipping":0`) {
				t.Fatalf("%d %s", w.Code, w.Body)
			}
		} else if w.Code != 200 || !strings.Contains(w.Body.String(), `"shipping":2`) {
			t.Fatalf("%d %s", w.Code, w.Body)
		}
		if err = m.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
		db.Close()
	}
}
