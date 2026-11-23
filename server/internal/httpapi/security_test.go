package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestIPRateLimiterBlocksAfterLimit(t *testing.T) {
	limiter := newIPRateLimiter(2, time.Minute)
	now := time.Now()

	if ok, _ := limiter.allow("192.0.2.1", now); !ok {
		t.Fatal("first request should be allowed")
	}
	if ok, _ := limiter.allow("192.0.2.1", now.Add(time.Second)); !ok {
		t.Fatal("second request should be allowed")
	}
	if ok, retry := limiter.allow("192.0.2.1", now.Add(2*time.Second)); ok || retry <= 0 {
		t.Fatal("third request should be blocked with a retry duration")
	}
	if ok, _ := limiter.allow("192.0.2.2", now.Add(2*time.Second)); !ok {
		t.Fatal("a different IP should have an independent limit")
	}
	if ok, _ := limiter.allow("192.0.2.1", now.Add(time.Minute)); !ok {
		t.Fatal("request should be allowed after the window expires")
	}
}

func TestSecurityHeaders(t *testing.T) {
	handler := securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	for _, name := range []string{
		"X-Content-Type-Options",
		"X-Frame-Options",
		"Referrer-Policy",
		"Content-Security-Policy",
		"Cache-Control",
		"Strict-Transport-Security",
	} {
		if rec.Header().Get(name) == "" {
			t.Errorf("expected %s header to be set", name)
		}
	}
}


func TestWebCORSAllowsOnlyConfiguredOrigin(t *testing.T) {
	handler := webCORS(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	allowed := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodOptions, "/api/v1/chats", nil)
	request.Header.Set("Origin", webClientOrigin)
	request.Header.Set("Access-Control-Request-Method", "POST")
	handler.ServeHTTP(allowed, request)
	if allowed.Code != http.StatusNoContent {
		t.Fatalf("allowed preflight status = %d, want %d", allowed.Code, http.StatusNoContent)
	}
	if got := allowed.Header().Get("Access-Control-Allow-Origin"); got != webClientOrigin {
		t.Fatalf("allow origin = %q, want %q", got, webClientOrigin)
	}

	denied := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	request.Header.Set("Origin", "https://untrusted.example")
	handler.ServeHTTP(denied, request)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("untrusted origin status = %d, want %d", denied.Code, http.StatusForbidden)
	}
	if got := denied.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("untrusted origin unexpectedly received CORS permission %q", got)
	}
}

func TestWebCORSKeepsNativeRequestsWorking(t *testing.T) {
	handler := webCORS(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("native request status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}
