package handlers

import (
	"net/http/httptest"
	"testing"

	"billflow/internal/middleware"
	"github.com/gin-gonic/gin"
)

func TestEmailAlertsAdminOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, role := range []string{"staff", "viewer"} {
		r := gin.New()
		r.Use(func(c *gin.Context) { c.Set("user_role", role) })
		h := &EmailAlertHandler{}
		g := r.Group("/settings/email-alerts", middleware.RequireRole("admin"))
		g.GET("", h.Settings)
		g.PUT("", h.Save)
		g.POST("/test", h.Test)
		g.GET("/history", h.History)
		g.POST("/history/:id/retry", h.Retry)
		for _, route := range []struct{ method, path string }{{"GET", ""}, {"PUT", ""}, {"POST", "/test"}, {"GET", "/history"}, {"POST", "/history/00000000-0000-0000-0000-000000000001/retry"}} {
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(route.method, "/settings/email-alerts"+route.path, nil))
			if w.Code != 403 {
				t.Fatalf("role=%s %s %s status=%d", role, route.method, route.path, w.Code)
			}
		}
	}
}
