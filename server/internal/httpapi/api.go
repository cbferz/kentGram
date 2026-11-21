package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/golang-jwt/jwt/v5"
	"github.com/cbferz/kentgram/server/internal/store"
	"golang.org/x/crypto/bcrypt"
)

type API struct {
	db        *store.Store
	jwtSecret []byte
}

func New(db *store.Store, secret string) *API {
	return &API{db: db, jwtSecret: []byte(secret)}
}

func (a *API) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, middleware.Recoverer, middleware.Timeout(20*time.Second))
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		if err := a.db.Pool.Ping(r.Context()); err != nil {
			writeError(w, http.StatusServiceUnavailable, "database_unavailable")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "kentgram-api"})
	})
	r.Route("/api/v1", func(r chi.Router) {
		r.Post("/auth/register", a.register)
		r.Post("/auth/login", a.login)
		r.Group(func(r chi.Router) {
			r.Use(a.authenticate)
			r.Get("/me", a.me)
		})
	})
	return r
}

type registerRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

var usernamePattern = regexp.MustCompile(`^[a-zA-Z0-9_]{3,32}$`)

func (a *API) register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if !usernamePattern.MatchString(req.Username) || !strings.Contains(req.Email, "@") ||
		len(req.Email) > 254 || len(req.Password) < 10 || len(req.Password) > 72 {
		writeError(w, http.StatusBadRequest, "invalid_registration_data")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "password_hash_failed")
		return
	}
	user, err := a.db.CreateUser(r.Context(), req.Username, req.Email, string(hash))
	if errors.Is(err, store.ErrConflict) {
		writeError(w, http.StatusConflict, "username_or_email_taken")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "registration_failed")
		return
	}
	token, err := a.issueToken(user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "token_issue_failed")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"user": user, "access_token": token, "token_type": "Bearer"})
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (a *API) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	user, err := a.db.UserByEmail(r.Context(), strings.ToLower(strings.TrimSpace(req.Email)))
	if err != nil || bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)) != nil {
		writeError(w, http.StatusUnauthorized, "invalid_credentials")
		return
	}
	token, err := a.issueToken(user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "token_issue_failed")
		return
	}
	user.PasswordHash = ""
	writeJSON(w, http.StatusOK, map[string]any{"user": user, "access_token": token, "token_type": "Bearer"})
}

func (a *API) me(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.Header.Get("X-User-ID"), 10, 64)
	user, err := a.db.UserByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "user_not_found")
		return
	}
	writeJSON(w, http.StatusOK, user)
}

func (a *API) issueToken(userID int64) (string, error) {
	claims := jwt.MapClaims{
		"sub": strconv.FormatInt(userID, 10),
		"iat": time.Now().Unix(),
		"exp": time.Now().Add(24 * time.Hour).Unix(),
		"iss": "kentgram",
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(a.jwtSecret)
}

func (a *API) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := strings.TrimSpace(r.Header.Get("Authorization"))
		parts := strings.SplitN(header, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			writeError(w, http.StatusUnauthorized, "missing_bearer_token")
			return
		}
		token, err := jwt.Parse(parts[1], func(token *jwt.Token) (any, error) {
			if token.Method != jwt.SigningMethodHS256 {
				return nil, errors.New("unexpected signing method")
			}
			return a.jwtSecret, nil
		}, jwt.WithIssuer("kentgram"))
		if err != nil || !token.Valid {
			writeError(w, http.StatusUnauthorized, "invalid_token")
			return
		}
		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			writeError(w, http.StatusUnauthorized, "invalid_token")
			return
		}
		sub, ok := claims["sub"].(string)
		if !ok {
			writeError(w, http.StatusUnauthorized, "invalid_token")
			return
		}
		if _, err := strconv.ParseInt(sub, 10, 64); err != nil {
			writeError(w, http.StatusUnauthorized, "invalid_token")
			return
		}
		r.Header.Set("X-User-ID", sub)
		next.ServeHTTP(w, r)
	})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]string{"error": code})
}

