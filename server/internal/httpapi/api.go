package httpapi

import (
	"net/http"
	"time"

	"github.com/cbferz/kentgram/server/internal/config"
	"github.com/cbferz/kentgram/server/internal/realtime"
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
	hub          *realtime.Hub
}

func New(db *store.Store, cfg config.Config) *API {
	a := &API{
		db:        db,
		cfg:       cfg,
		jwtSecret: []byte(cfg.JWTSecret),
		// General API traffic, per client IP.
		apiLimiter: newIPRateLimiter(120, time.Minute),
		// Credential endpoints are much stricter per client IP.
		authLimiter: newIPRateLimiter(10, time.Minute),
		// Brute-force guard per target account, independent of source IP.
		loginLimiter: newIPRateLimiter(10, 15*time.Minute),
	}
	a.hub = a.newHub()
	return a
}

func (a *API) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, middleware.Recoverer,
		middleware.Timeout(20*time.Second), securityHeaders)

	r.Get("/health", a.health)

	r.Route("/api/v1", func(r chi.Router) {
		r.Use(a.apiLimiter.middleware)

		r.Group(func(r chi.Router) {
			r.Use(a.authLimiter.middleware)
			r.Post("/auth/register", a.register)
			r.Post("/auth/login", a.login)
			r.Post("/auth/refresh", a.refresh)
		})

		// The WebSocket authenticates with its first frame, not a header, so it
		// sits outside the Bearer-token group.
		r.Get("/ws", a.hub.ServeWS)

		r.Group(func(r chi.Router) {
			r.Use(a.authenticate)

			r.Post("/auth/logout", a.logout)
			r.Get("/sessions", a.listSessions)
			r.Delete("/sessions/{sessionID}", a.revokeSession)
			r.Post("/sessions/revoke-others", a.revokeOtherSessions)

			r.Get("/chats", a.listChats)
			r.Post("/chats/direct", a.createDirectChat)
			r.Get("/chats/{chatID}/messages", a.listChatMessages)
			r.Post("/chats/{chatID}/messages", a.sendChatMessage)
			r.Patch("/chats/{chatID}/messages/{messageID}", a.editChatMessage)
			r.Delete("/chats/{chatID}/messages/{messageID}", a.deleteChatMessage)
			r.Post("/chats/{chatID}/messages/{messageID}/reactions", a.addReaction)
			r.Delete("/chats/{chatID}/messages/{messageID}/reactions", a.removeReaction)
			r.Post("/chats/{chatID}/messages/{messageID}/forward", a.forwardChatMessage)
			r.Get("/chats/{chatID}/pins", a.listPinnedMessages)
			r.Put("/chats/{chatID}/pins/{messageID}", a.pinChatMessage)
			r.Delete("/chats/{chatID}/pins/{messageID}", a.unpinChatMessage)
			r.Post("/chats/{chatID}/read", a.markChatRead)
			r.Post("/chats/{chatID}/typing", a.typing)
			r.Get("/presence", a.presence)

			r.Get("/contacts", a.contacts)
			r.Get("/me", a.me)
			r.Patch("/me/profile", a.updateProfile)
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
