package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/cbferz/kentgram/server/internal/secret"
	"github.com/cbferz/kentgram/server/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

const (
	jwtIssuer          = "kentgram"
	refreshTokenPrefix = "kgr"
)

var usernamePattern = regexp.MustCompile(`^[a-zA-Z0-9_]{3,32}$`)

// dummyHash is compared against when an e-mail is unknown, so that a login
// attempt takes the same time whether or not the account exists.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("kentgram-timing-equaliser"), bcrypt.DefaultCost)

type authResponse struct {
	User         *store.User `json:"user,omitempty"`
	AccessToken  string      `json:"access_token"`
	RefreshToken string      `json:"refresh_token"`
	TokenType    string      `json:"token_type"`
	ExpiresIn    int         `json:"expires_in"`
}

type registerRequest struct {
	Username   string `json:"username"`
	Email      string `json:"email"`
	Password   string `json:"password"`
	DeviceName string `json:"device_name"`
}

type loginRequest struct {
	Email      string `json:"email"`
	Password   string `json:"password"`
	DeviceName string `json:"device_name"`
}

// securityEvent writes a structured record for events worth alerting on.
func securityEvent(r *http.Request, event string, attrs ...any) {
	args := []any{
		"event", event,
		"ip", clientIP(r),
		"request_id", middleware.GetReqID(r.Context()),
	}
	slog.Warn("security", append(args, attrs...)...)
}

func (a *API) register(w http.ResponseWriter, r *http.Request) {
	var q registerRequest
	if decodeJSON(w, r, &q) != nil {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	q.Username = strings.TrimSpace(q.Username)
	q.Email = normalizeEmail(q.Email)
	if !usernamePattern.MatchString(q.Username) ||
		!strings.Contains(q.Email, "@") || len(q.Email) > 254 ||
		len(q.Password) < 10 || len(q.Password) > 72 {
		writeError(w, http.StatusBadRequest, "invalid_registration_data")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(q.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "password_hash_failed")
		return
	}
	user, err := a.db.CreateUser(r.Context(), q.Username, q.Email, string(hash))
	if errors.Is(err, store.ErrConflict) {
		writeError(w, http.StatusConflict, "username_or_email_taken")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "registration_failed")
		return
	}

	resp, err := a.startSession(r, user.ID, q.DeviceName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "token_issue_failed")
		return
	}
	resp.User = &user
	writeJSON(w, http.StatusCreated, resp)
}

func (a *API) login(w http.ResponseWriter, r *http.Request) {
	var q loginRequest
	if decodeJSON(w, r, &q) != nil {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	email := normalizeEmail(q.Email)

	if ok, retry := a.loginLimiter.allow("login:"+email, time.Now()); !ok {
		securityEvent(r, "login_locked_out")
		w.Header().Set("Retry-After", strconv.Itoa(max(1, int(retry.Seconds()))))
		writeError(w, http.StatusTooManyRequests, "too_many_login_attempts")
		return
	}

	user, err := a.db.UserByEmail(r.Context(), email)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusInternalServerError, "login_failed")
		return
	}
	hash := dummyHash
	if err == nil {
		hash = []byte(user.PasswordHash)
	}
	passwordOK := bcrypt.CompareHashAndPassword(hash, []byte(q.Password)) == nil
	if err != nil || !passwordOK {
		securityEvent(r, "login_failed")
		writeError(w, http.StatusUnauthorized, "invalid_credentials")
		return
	}

	resp, err := a.startSession(r, user.ID, q.DeviceName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "token_issue_failed")
		return
	}
	user.PasswordHash = ""
	resp.User = &user
	writeJSON(w, http.StatusOK, resp)
}

func (a *API) refresh(w http.ResponseWriter, r *http.Request) {
	var q struct {
		RefreshToken string `json:"refresh_token"`
	}
	if decodeJSON(w, r, &q) != nil {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	if !secret.HasPrefix(q.RefreshToken, refreshTokenPrefix, 128) {
		writeError(w, http.StatusUnauthorized, "invalid_refresh_token")
		return
	}

	next, err := secret.Token(refreshTokenPrefix, 32)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "token_issue_failed")
		return
	}
	rot, err := a.db.RotateSession(r.Context(), secret.Hash(q.RefreshToken),
		secret.Hash(next), time.Now().Add(a.cfg.RefreshTTL))
	switch {
	case errors.Is(err, store.ErrTokenReuse):
		securityEvent(r, "refresh_token_reuse",
			"user_id", rot.UserID, "session_id", rot.SessionID)
		writeError(w, http.StatusUnauthorized, "refresh_token_reused")
		return
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusUnauthorized, "invalid_refresh_token")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "refresh_failed")
		return
	}

	access, err := a.signAccess(rot.UserID, rot.SessionID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "token_issue_failed")
		return
	}
	writeJSON(w, http.StatusOK, authResponse{
		AccessToken:  access,
		RefreshToken: next,
		TokenType:    "Bearer",
		ExpiresIn:    int(a.cfg.AccessTTL.Seconds()),
	})
}

func (a *API) logout(w http.ResponseWriter, r *http.Request) {
	if err := a.db.RevokeSession(r.Context(), sessionID(r), userID(r)); err != nil &&
		!errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusInternalServerError, "logout_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func (a *API) listSessions(w http.ResponseWriter, r *http.Request) {
	sessions, err := a.db.ListSessions(r.Context(), userID(r), sessionID(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "sessions_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions": sessions})
}

func (a *API) revokeSession(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "sessionID"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid_session_id")
		return
	}
	err = a.db.RevokeSession(r.Context(), id, userID(r))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "session_not_found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "revoke_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func (a *API) revokeOtherSessions(w http.ResponseWriter, r *http.Request) {
	n, err := a.db.RevokeOtherSessions(r.Context(), userID(r), sessionID(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "revoke_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{"revoked": n})
}

// startSession creates a server-side session and the first token pair.
func (a *API) startSession(r *http.Request, uid int64, device string) (authResponse, error) {
	refresh, err := secret.Token(refreshTokenPrefix, 32)
	if err != nil {
		return authResponse{}, err
	}
	sid, err := a.db.CreateSession(r.Context(), uid, secret.Hash(refresh),
		truncateRunes(strings.TrimSpace(device), 64),
		truncateRunes(r.UserAgent(), 256), clientIP(r),
		time.Now().Add(a.cfg.RefreshTTL))
	if err != nil {
		return authResponse{}, err
	}
	access, err := a.signAccess(uid, sid)
	if err != nil {
		return authResponse{}, err
	}
	return authResponse{
		AccessToken:  access,
		RefreshToken: refresh,
		TokenType:    "Bearer",
		ExpiresIn:    int(a.cfg.AccessTTL.Seconds()),
	}, nil
}

func (a *API) signAccess(uid, sid int64) (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"sub": strconv.FormatInt(uid, 10),
		"sid": strconv.FormatInt(sid, 10),
		"iat": now.Unix(),
		"exp": now.Add(a.cfg.AccessTTL).Unix(),
		"iss": jwtIssuer,
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(a.jwtSecret)
}

// authenticate accepts a Bearer access token, then confirms in the database
// that its session is still live, which is what makes revocation immediate.
func (a *API) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.SplitN(strings.TrimSpace(r.Header.Get("Authorization")), " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			writeError(w, http.StatusUnauthorized, "missing_bearer_token")
			return
		}

		uid, sid, ok := a.parseAccess(parts[1])
		if !ok {
			writeError(w, http.StatusUnauthorized, "invalid_token")
			return
		}
		active, err := a.db.SessionActive(r.Context(), sid, uid)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "auth_unavailable")
			return
		}
		if !active {
			writeError(w, http.StatusUnauthorized, "session_revoked")
			return
		}

		ctx := context.WithValue(r.Context(), ctxUserID, uid)
		ctx = context.WithValue(ctx, ctxSessionID, sid)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (a *API) parseAccess(raw string) (uid, sid int64, ok bool) {
	token, err := jwt.Parse(raw, func(t *jwt.Token) (any, error) {
		return a.jwtSecret, nil
	}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithIssuer(jwtIssuer), jwt.WithExpirationRequired())
	if err != nil || !token.Valid {
		return 0, 0, false
	}
	claims, isMap := token.Claims.(jwt.MapClaims)
	if !isMap {
		return 0, 0, false
	}
	sub, _ := claims["sub"].(string)
	sidRaw, _ := claims["sid"].(string)
	uid, err1 := strconv.ParseInt(sub, 10, 64)
	sid, err2 := strconv.ParseInt(sidRaw, 10, 64)
	if err1 != nil || err2 != nil || uid <= 0 || sid <= 0 {
		return 0, 0, false
	}
	return uid, sid, true
}

func normalizeEmail(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}
