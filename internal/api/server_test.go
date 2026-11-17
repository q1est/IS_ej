package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAttendancePreflightAllowsPUT(t *testing.T) {
	handler := securityMiddleware(map[string]struct{}{"https://school.test": {}}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("preflight must not reach a protected handler")
	}))
	request := httptest.NewRequest(http.MethodOptions, "/attendance", nil)
	request.Header.Set("Origin", "https://school.test")
	request.Header.Set("Access-Control-Request-Method", "PUT")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent || !strings.Contains(response.Header().Get("Access-Control-Allow-Methods"), "PUT") {
		t.Fatalf("attendance preflight failed: status=%d headers=%v", response.Code, response.Header())
	}
	if response.Header().Get("Access-Control-Allow-Origin") != "https://school.test" {
		t.Fatal("missing allowed origin")
	}
}

func TestPreflightRejectsUnknownOrigin(t *testing.T) {
	handler := securityMiddleware(map[string]struct{}{}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Fatal("denied origin reached handler") }))
	request := httptest.NewRequest(http.MethodOptions, "/attendance", nil)
	request.Header.Set("Origin", "https://other.test")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status=%d", response.Code)
	}
}
