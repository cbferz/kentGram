package httpapi

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type rateEntry struct {
	window time.Time
	count  int
}

type ipRateLimiter struct {
	mu          sync.Mutex
	entries     map[string]rateEntry
	limit       int
	window      time.Duration
	lastCleanup time.Time
}

func newIPRateLimiter(limit int, window time.Duration) *ipRateLimiter {
	return &ipRateLimiter{
		entries: make(map[string]rateEntry),
		limit:   limit,
		window:  window,
	}
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return host
	}
	if r.RemoteAddr != "" {
		return strings.TrimSpace(r.RemoteAddr)
	}
	return "unknown"
}

func (l *ipRateLimiter) allow(key string, now time.Time) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.lastCleanup.IsZero() || now.Sub(l.lastCleanup) >= 5*time.Minute {
		for k, entry := range l.entries {
			if now.Sub(entry.window) >= 2*l.window {
				delete(l.entries, k)
			}
		}
		l.lastCleanup = now
	}

	entry, ok := l.entries[key]
	if !ok || now.Sub(entry.window) >= l.window {
		if !ok && len(l.entries) >= 10000 {
			return false, l.window
		}
		entry = rateEntry{window: now, count: 0}
	}

	if entry.count >= l.limit {
		retry := l.window - now.Sub(entry.window)
		if retry < time.Second {
			retry = time.Second
		}
		l.entries[key] = entry
		return false, retry
	}

	entry.count++
	l.entries[key] = entry
	return true, 0
}

func (l *ipRateLimiter) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ok, retry := l.allow(clientIP(r), time.Now())
		if !ok {
			w.Header().Set("Retry-After", strconv.Itoa(max(1, int(retry.Seconds()))))
			writeError(w, http.StatusTooManyRequests, "rate_limit_exceeded")
			return
		}
		next.ServeHTTP(w, r)
	})
}

const webClientOrigin = "https://web.kentgram.tendercheck.pro"

// webCORS grants browser access only to the production web client. Native
// Android requests without an Origin header continue to work unchanged.
func webCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := strings.TrimSpace(r.Header.Get("Origin"))
		if origin != "" {
			if origin != webClientOrigin {
				writeError(w, http.StatusForbidden, "origin_not_allowed")
				return
			}
			h := w.Header()
			h.Set("Access-Control-Allow-Origin", webClientOrigin)
			h.Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
			h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			h.Set("Access-Control-Max-Age", "600")
			h.Add("Vary", "Origin")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		h.Set("Cache-Control", "no-store")
		h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		next.ServeHTTP(w, r)
	})
}
