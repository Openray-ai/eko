package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

type fakeHealthChecker struct {
	err error
}

func (f fakeHealthChecker) HealthCheck(context.Context) error {
	return f.err
}

func TestHealthHandlerLivenessDegradesButStays200(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := NewHealthHandler(fakeHealthChecker{err: errors.New("redis down")})
	router := gin.New()
	router.GET("/health", handler.HandleLiveness)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
}

func TestHealthHandlerReadinessFailsOnDependencyOutage(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := NewHealthHandler(fakeHealthChecker{err: errors.New("redis down")})
	router := gin.New()
	router.GET("/ready", handler.HandleReadiness)

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503, got %d", rec.Code)
	}
}
