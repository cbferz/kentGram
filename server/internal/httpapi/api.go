package httpapi

import (
	"net/http"
	"time"

	"github.com/cbferz/kentgram/server/internal/config"
	"github.com/cbferz/kentgram/server/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type API struct {
	db           *store.Store
	cfg          config.Config
	jwtSecret    []byte
	apiLimiter   *ipRateLimiter
	authLimiter  *ipRateLimiter
	loginLimiter *ipRateLimiter
}

func New(db *store.Store, cfg config.Config) *API {
	return &API{
		db:        db,
		cfg:       cfg,
		jwtSecret: []byte(cfg.JWTSecret),
		apiLimiter: newIPRateLimiter(120, time.Minute),
		authLimiter: newIPRateLimiter(10, time.Minute),
		loginLimiter: newIPRateLimiter(10, 15*time.Minute),
	}
}

func (a *API) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, middleware.Recoverer, securityHeaders)

	// WebSocket connections are intentionally outside chi's request timeout:
	// the socket is long-lived and has its own bounded heartbeat/read deadlines.
	r.With(a.apiLimiter.middleware).Get("/api/v1/realtime", a.realtime)
	r.Get("/health", a.health)
	r.Get("/media/avatars/{filename}", a.avatarFile)

	r.Route("/api/v1", func(r chi.Router) {
		r.Use(middleware.Timeout(20*time.Second), a.apiLimiter.middleware)

		r.Group(func(r chi.Router) {
			r.Use(a.authLimiter.middleware)
			r.Post("/auth/register", a.register)
			r.Post("/auth/login", a.login)
			r.Post("/auth/refresh", a.refresh)
		})

		r.Group(func(r chi.Router) {
			r.Use(a.authenticate)

			r.Post("/auth/logout", a.logout)
			r.Get("/sessions", a.listSessions)
			r.Delete("/sessions/{sessionID}", a.revokeSession)
			r.Post("/sessions/revoke-others", a.revokeOtherSessions)
			r.Post("/realtime/ticket", a.createRealtimeTicket)

			r.Get("/chats", a.listChats)
			r.Post("/chats/direct", a.createDirectChat)
			r.Get("/chats/{chatID}/messages", a.listChatMessages)
			r.Post("/chats/{chatID}/messages", a.sendChatMessage)
			r.Delete("/chats/{chatID}/messages/{messageID}", a.deleteChatMessage)

			r.Get("/contacts", a.contacts)
			r.Get("/me", a.me)
			r.Patch("/me/profile", a.updateProfile)
			r.Post("/me/avatar", a.uploadAvatar)
			r.Get("/users/search", a.searchUsers)
			r.Get("/users/{username}", a.publicProfile)
			r.Post("/users/{userID}/follow", a.follow)
			r.Delete("/users/{userID}/follow", a.unfollow)
		})
	})
	return r
}

func (a *API) health(w http.ResponseWriter, r *http.Request) {
	if err := a.db.Pool.Ping(r.Context()); err != nil {
		writeError(w, http.StatusServiceUnavailable, "database_unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "kentgram-api"})
}
