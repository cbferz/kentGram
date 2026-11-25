package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestUnknownAPIEndpointReturnsStructured404(t *testing.T) {
	api := &API{apiLimiter: newIPRateLimiter(100, time.Minute)}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/does-not-exist", nil)
	rec := httptest.NewRecorder()

	api.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not JSON: %v; body=%q", err, rec.Body.String())
	}
	if body["error"] != "endpoint_not_found" || body["method"] != http.MethodGet || body["path"] != "/api/v1/does-not-exist" {
		t.Fatalf("unexpected route diagnostic: %#v", body)
	}
}

func TestUnsupportedMethodReturnsStructured405(t *testing.T) {
	api := &API{apiLimiter: newIPRateLimiter(100, time.Minute)}
	req := httptest.NewRequest(http.MethodPost, "/health", nil)
	rec := httptest.NewRecorder()

	api.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not JSON: %v; body=%q", err, rec.Body.String())
	}
	if body["error"] != "method_not_allowed" || body["method"] != http.MethodPost || body["path"] != "/health" {
		t.Fatalf("unexpected method diagnostic: %#v", body)
	}
}
