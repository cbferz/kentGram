package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cbferz/kentgram/server/internal/config"
	"github.com/cbferz/kentgram/server/internal/store"
)

const testPassword = "correct-horse-battery"

var userCounter atomic.Int64

// newTestAPI connects to the database named by TEST_DATABASE_URL. Tests that
// need Postgres are skipped locally and run in CI, where a service container
// provides it. Limiters are loosened so unrelated tests do not trip them.
func newTestAPI(t *testing.T) *API {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set; skipping database integration test")
	}
	ctx := context.Background()
	db, err := store.Open(ctx, url)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(db.Close)
	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	a := New(db, config.Config{
		JWTSecret:  strings.Repeat("s", 40),
		MediaDir:   t.TempDir(),
		AvatarDir:  t.TempDir(),
		AccessTTL:  15 * time.Minute,
		RefreshTTL: 24 * time.Hour,
	})
	a.apiLimiter = newIPRateLimiter(1_000_000, time.Minute)
	a.authLimiter = newIPRateLimiter(1_000_000, time.Minute)
	a.loginLimiter = newIPRateLimiter(1_000_000, time.Minute)
	return a
}

func call(t *testing.T, h http.Handler, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func asMap(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("response is not a JSON object: %v\nbody: %s", err, rec.Body.String())
	}
	return m
}

func expectStatus(t *testing.T, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rec.Code != want {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, want, rec.Body.String())
	}
}

type testUser struct {
	ID       int64
	Username string
	Email    string
	Access   string
	Refresh  string
}

func newName() string {
	return fmt.Sprintf("u%d%d", time.Now().UnixNano()%1_000_000_000_000, userCounter.Add(1))
}

func registerUser(t *testing.T, h http.Handler) testUser {
	t.Helper()
	name := newName()
	rec := call(t, h, http.MethodPost, "/api/v1/auth/register", "", map[string]string{
		"username": name, "email": name + "@example.test", "password": testPassword,
		"device_name": "test-device",
	})
	expectStatus(t, rec, http.StatusCreated)
	m := asMap(t, rec)
	user := m["user"].(map[string]any)
	return testUser{
		ID:       int64(user["id"].(float64)),
		Username: name,
		Email:    name + "@example.test",
		Access:   m["access_token"].(string),
		Refresh:  m["refresh_token"].(string),
	}
}
