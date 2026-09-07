package httpserver_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/actuponit/telegram-jira-integration/internal/platform/httpserver"
)

func TestHealthCheck_ReturnsOK(t *testing.T) {
	mux := httpserver.NewMux()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}
