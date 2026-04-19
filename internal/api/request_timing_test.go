package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/executor"
)

func TestRequestTimingMiddlewareSetsRequestStart(t *testing.T) {
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	engine.Use(requestTimingMiddleware())
	engine.GET("/check", func(c *gin.Context) {
		startedAt, ok := cliproxyexecutor.RequestStart(c.Request.Context())
		if !ok {
			t.Fatal("request start missing from request context")
		}
		if time.Since(startedAt) > time.Second {
			t.Fatalf("request start too old: %v", startedAt)
		}
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/check", nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestRequestTimingMiddlewarePreservesExistingRequestStart(t *testing.T) {
	gin.SetMode(gin.TestMode)

	base := time.Date(2026, 4, 16, 0, 0, 0, 0, time.UTC)
	engine := gin.New()
	engine.Use(requestTimingMiddleware())
	engine.GET("/check", func(c *gin.Context) {
		startedAt, ok := cliproxyexecutor.RequestStart(c.Request.Context())
		if !ok {
			t.Fatal("request start missing from request context")
		}
		if !startedAt.Equal(base) {
			t.Fatalf("request start = %v, want %v", startedAt, base)
		}
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/check", nil)
	req = req.WithContext(cliproxyexecutor.WithRequestStart(req.Context(), base))
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}
