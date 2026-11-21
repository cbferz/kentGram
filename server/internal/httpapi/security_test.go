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
