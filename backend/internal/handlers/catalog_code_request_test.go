package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestCatalogCodeFromJSONPreservesSlash(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/catalog/embed", func(c *gin.Context) {
		code, ok := catalogCodeFromJSON(c)
		if !ok {
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": code})
	})

	req := httptest.NewRequest(http.MethodPost, "/api/catalog/embed", strings.NewReader(`{"code":"MD3Y4TH/A"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got, want := rec.Body.String(), `{"code":"MD3Y4TH/A"}`; !strings.Contains(got, want) {
		t.Fatalf("body = %s, want code %s", got, want)
	}
}

func TestCatalogCodeFromJSONRejectsControlCharacters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/catalog/embed", func(c *gin.Context) {
		_, _ = catalogCodeFromJSON(c)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/catalog/embed", strings.NewReader("{\"code\":\"MD3Y4TH/A\\n\"}"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}
