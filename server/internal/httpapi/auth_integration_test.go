package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestRegisterAndLogin(t *testing.T) {
	a := newTestAPI(t)
	h := a.Routes()
	name := newName()
	creds := map[string]string{"username": name, "email": name + "@example.test", "password": testPassword}

	rec := call(t, h, http.MethodPost, "/api/v1/auth/register", "", creds)
	expectStatus(t, rec, http.StatusCreated)
	if strings.Contains(strings.ToLower(rec.Body.String()), "password") {
		t.Fatalf("registration response must never contain password material: %s", rec.Body.String())
	}
	m := asMap(t, rec)
	if m["access_token"] == "" || m["refresh_token"] == "" || m["token_type"] != "Bearer" {
		t.Fatalf("incomplete token response: %v", m)
	}

	expectStatus(t, call(t, h, http.MethodPost, "/api/v1/auth/register", "", creds), http.StatusConflict)

	weak := map[string]string{"username": newName(), "email": newName() + "@example.test", "password": "short"}
	expectStatus(t, call(t, h, http.MethodPost, "/api/v1/auth/register", "", weak), http.StatusBadRequest)

	badUser := map[string]string{"username": "a b", "email": "x@example.test", "password": testPassword}
	expectStatus(t, call(t, h, http.MethodPost, "/api/v1/auth/register", "", badUser), http.StatusBadRequest)

	wrong := call(t, h, http.MethodPost, "/api/v1/auth/login", "", map[string]string{
		"username": name, "password": "wrong-password-here"})
	expectStatus(t, wrong, http.StatusUnauthorized)
	unknown := call(t, h, http.MethodPost, "/api/v1/auth/login", "", map[string]string{
		"username": "nobody-" + name, "password": "wrong-password-here"})
	expectStatus(t, unknown, http.StatusUnauthorized)
	if wrong.Body.String() != unknown.Body.String() {
		t.Fatal("unknown username and wrong password must be indistinguishable")
	}

	ok := call(t, h, http.MethodPost, "/api/v1/auth/login", "", map[string]string{
		"username": strings.ToUpper(name), "password": testPassword})
	expectStatus(t, ok, http.StatusOK)

	me := call(t, h, http.MethodGet, "/api/v1/me", asMap(t, ok)["access_token"].(string), nil)
	expectStatus(t, me, http.StatusOK)
	if asMap(t, me)["username"] != name {
		t.Fatalf("/me returned the wrong user: %v", asMap(t, me))
	}
}

func TestAccessTokenValidation(t *testing.T) {
	a := newTestAPI(t)
	h := a.Routes()
	u := registerUser(t, h)

	expectStatus(t, call(t, h, http.MethodGet, "/api/v1/me", "", nil), http.StatusUnauthorized)
	expectStatus(t, call(t, h, http.MethodGet, "/api/v1/me", "not-a-jwt", nil), http.StatusUnauthorized)

	forged := signTestToken(t, []byte(strings.Repeat("x", 40)), u.ID, 1, time.Now().Add(time.Hour))
	expectStatus(t, call(t, h, http.MethodGet, "/api/v1/me", forged, nil), http.StatusUnauthorized)

	expired := signTestToken(t, a.jwtSecret, u.ID, 1, time.Now().Add(-time.Minute))
	expectStatus(t, call(t, h, http.MethodGet, "/api/v1/me", expired, nil), http.StatusUnauthorized)

	// A correctly signed token for a session that does not exist is rejected.
	ghost := signTestToken(t, a.jwtSecret, u.ID, 999_999_999, time.Now().Add(time.Hour))
	expectStatus(t, call(t, h, http.MethodGet, "/api/v1/me", ghost, nil), http.StatusUnauthorized)

	// The correct user id paired with someone else's session is rejected too.
	other := registerUser(t, h)
	sid := firstSessionID(t, h, other.Access)
	mixed := signTestToken(t, a.jwtSecret, u.ID, sid, time.Now().Add(time.Hour))
	expectStatus(t, call(t, h, http.MethodGet, "/api/v1/me", mixed, nil), http.StatusUnauthorized)

	expectStatus(t, call(t, h, http.MethodGet, "/api/v1/me", u.Access, nil), http.StatusOK)
}

func TestRefreshRotationAndReuseDetection(t *testing.T) {
	h := newTestAPI(t).Routes()
	u := registerUser(t, h)

	first := call(t, h, http.MethodPost, "/api/v1/auth/refresh", "", map[string]string{"refresh_token": u.Refresh})
	expectStatus(t, first, http.StatusOK)
	next := asMap(t, first)
	newRefresh := next["refresh_token"].(string)
	newAccess := next["access_token"].(string)
	if newRefresh == u.Refresh {
		t.Fatal("refresh token must rotate on every use")
	}
	expectStatus(t, call(t, h, http.MethodGet, "/api/v1/me", newAccess, nil), http.StatusOK)

	// Replaying the already-used token signals theft and kills the session.
	replay := call(t, h, http.MethodPost, "/api/v1/auth/refresh", "", map[string]string{"refresh_token": u.Refresh})
	expectStatus(t, replay, http.StatusUnauthorized)
	if asMap(t, replay)["error"] != "refresh_token_reused" {
		t.Fatalf("expected refresh_token_reused, got %v", asMap(t, replay))
	}
	expectStatus(t, call(t, h, http.MethodPost, "/api/v1/auth/refresh", "",
		map[string]string{"refresh_token": newRefresh}), http.StatusUnauthorized)
	expectStatus(t, call(t, h, http.MethodGet, "/api/v1/me", newAccess, nil), http.StatusUnauthorized)

	for _, bad := range []string{"", "kgr_unknownunknownunknownunknownunknownunknown", "garbage", strings.Repeat("kgr_a", 100)} {
		expectStatus(t, call(t, h, http.MethodPost, "/api/v1/auth/refresh", "",
			map[string]string{"refresh_token": bad}), http.StatusUnauthorized)
	}
}

func TestExpiredRefreshTokenIsRejected(t *testing.T) {
	a := newTestAPI(t)
	h := a.Routes()
	u := registerUser(t, h)
	if _, err := a.db.Pool.Exec(context.Background(),
		`UPDATE sessions SET expires_at=NOW()-INTERVAL '1 minute' WHERE user_id=$1`, u.ID); err != nil {
		t.Fatal(err)
	}
	expectStatus(t, call(t, h, http.MethodPost, "/api/v1/auth/refresh", "",
		map[string]string{"refresh_token": u.Refresh}), http.StatusUnauthorized)
	expectStatus(t, call(t, h, http.MethodGet, "/api/v1/me", u.Access, nil), http.StatusUnauthorized)
}

func TestLogoutRevokesAccessImmediately(t *testing.T) {
	h := newTestAPI(t).Routes()
	u := registerUser(t, h)
	expectStatus(t, call(t, h, http.MethodGet, "/api/v1/me", u.Access, nil), http.StatusOK)
	expectStatus(t, call(t, h, http.MethodPost, "/api/v1/auth/logout", u.Access, nil), http.StatusOK)
	expectStatus(t, call(t, h, http.MethodGet, "/api/v1/me", u.Access, nil), http.StatusUnauthorized)
	expectStatus(t, call(t, h, http.MethodPost, "/api/v1/auth/refresh", "",
		map[string]string{"refresh_token": u.Refresh}), http.StatusUnauthorized)
}

func TestSessionManagementAndIsolation(t *testing.T) {
	h := newTestAPI(t).Routes()
	alice := registerUser(t, h)
	bob := registerUser(t, h)

	aliceSession := firstSessionID(t, h, alice.Access)

	// Bob cannot revoke Alice's session; it looks like it does not exist.
	rec := call(t, h, http.MethodDelete, "/api/v1/sessions/"+strconv.FormatInt(aliceSession, 10), bob.Access, nil)
	expectStatus(t, rec, http.StatusNotFound)
	expectStatus(t, call(t, h, http.MethodGet, "/api/v1/me", alice.Access, nil), http.StatusOK)

	// Bob's session list never contains Alice's session.
	list := asMap(t, call(t, h, http.MethodGet, "/api/v1/sessions", bob.Access, nil))["sessions"].([]any)
	for _, s := range list {
		if int64(s.(map[string]any)["id"].(float64)) == aliceSession {
			t.Fatal("a session of another user leaked into the list")
		}
	}

	// Alice signs in on a second device, then signs every other device out.
	second := call(t, h, http.MethodPost, "/api/v1/auth/login", "", map[string]string{
		"username": alice.Username, "password": testPassword, "device_name": "tablet"})
	expectStatus(t, second, http.StatusOK)
	secondAccess := asMap(t, second)["access_token"].(string)

	sessions := asMap(t, call(t, h, http.MethodGet, "/api/v1/sessions", alice.Access, nil))["sessions"].([]any)
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}
	currentCount := 0
	for _, s := range sessions {
		if s.(map[string]any)["current"] == true {
			currentCount++
		}
	}
	if currentCount != 1 {
		t.Fatalf("exactly one session must be marked current, got %d", currentCount)
	}

	revoked := call(t, h, http.MethodPost, "/api/v1/sessions/revoke-others", alice.Access, nil)
	expectStatus(t, revoked, http.StatusOK)
	if asMap(t, revoked)["revoked"] != float64(1) {
		t.Fatalf("expected one revoked session: %v", asMap(t, revoked))
	}
	expectStatus(t, call(t, h, http.MethodGet, "/api/v1/me", secondAccess, nil), http.StatusUnauthorized)
	expectStatus(t, call(t, h, http.MethodGet, "/api/v1/me", alice.Access, nil), http.StatusOK)

	expectStatus(t, call(t, h, http.MethodDelete, "/api/v1/sessions/abc", alice.Access, nil), http.StatusBadRequest)
}

func TestLoginLockoutPerAccount(t *testing.T) {
	a := newTestAPI(t)
	a.loginLimiter = newIPRateLimiter(3, time.Minute)
	h := a.Routes()
	u := registerUser(t, h)

	for i := 0; i < 3; i++ {
		rec := call(t, h, http.MethodPost, "/api/v1/auth/login", "", map[string]string{
			"username": u.Username, "password": "definitely-wrong-password"})
		expectStatus(t, rec, http.StatusUnauthorized)
	}
	locked := call(t, h, http.MethodPost, "/api/v1/auth/login", "", map[string]string{
		"username": u.Username, "password": testPassword})
	expectStatus(t, locked, http.StatusTooManyRequests)
	if locked.Header().Get("Retry-After") == "" {
		t.Fatal("lockout response must include Retry-After")
	}
}

func TestAuthEndpointRateLimit(t *testing.T) {
	a := newTestAPI(t)
	a.authLimiter = newIPRateLimiter(2, time.Minute)
	h := a.Routes()
	body := map[string]string{"username": "nobody", "password": "whatever-password"}

	expectStatus(t, call(t, h, http.MethodPost, "/api/v1/auth/login", "", body), http.StatusUnauthorized)
	expectStatus(t, call(t, h, http.MethodPost, "/api/v1/auth/login", "", body), http.StatusUnauthorized)
	expectStatus(t, call(t, h, http.MethodPost, "/api/v1/auth/login", "", body), http.StatusTooManyRequests)
}

func TestRejectsMalformedJSON(t *testing.T) {
	h := newTestAPI(t).Routes()
	for _, raw := range []string{`{`, `{"username":1}`, `{"email":"a@b.c","password":"x","unknown":true}`, `{"email":"a@b.c"} {}`} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(raw))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		expectStatus(t, rec, http.StatusBadRequest)
	}
}

func TestMigrationsAreIdempotent(t *testing.T) {
	a := newTestAPI(t)
	ctx := context.Background()
	if err := a.db.Migrate(ctx); err != nil {
		t.Fatalf("second migrate run must be a no-op: %v", err)
	}
	var count int
	if err := a.db.Pool.QueryRow(ctx, `SELECT count(*) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count < 2 {
		t.Fatalf("expected at least 2 recorded migrations, got %d", count)
	}
}

func signTestToken(t *testing.T, key []byte, uid, sid int64, exp time.Time) string {
	t.Helper()
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": strconv.FormatInt(uid, 10),
		"sid": strconv.FormatInt(sid, 10),
		"iat": time.Now().Add(-time.Hour).Unix(),
		"exp": exp.Unix(),
		"iss": jwtIssuer,
	}).SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func firstSessionID(t *testing.T, h http.Handler, access string) int64 {
	t.Helper()
	rec := call(t, h, http.MethodGet, "/api/v1/sessions", access, nil)
	expectStatus(t, rec, http.StatusOK)
	list := asMap(t, rec)["sessions"].([]any)
	if len(list) == 0 {
		t.Fatal("expected at least one session")
	}
	return int64(list[0].(map[string]any)["id"].(float64))
}
