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
	hub          *realtimeHub
}

func New(db *store.Store, cfg config.Config) *API {
	return &API{
		db:        db,
		cfg:       cfg,
		jwtSecret: []byte(cfg.JWTSecret),
		// General API traffic, per client IP.
		apiLimiter: newIPRateLimiter(120, time.Minute),
		// Credential endpoints are much stricter per client IP.
		authLimiter: newIPRateLimiter(10, time.Minute),
		// Brute-force guard per target account, independent of source IP.
		loginLimiter: newIPRateLimiter(10, 15*time.Minute),
		hub:          newRealtimeHub(),
	}
}

func (a *API) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, middleware.Recoverer, webCORS, securityHeaders)

	r.Get("/health", a.health)
	r.Get("/media/avatars/{filename}", a.avatarFile)

	r.Route("/api/v1", func(r chi.Router) {
		r.Use(a.apiLimiter.middleware)

		r.Group(func(r chi.Router) {
			r.Use(a.authenticate)
			r.Get("/realtime", a.realtime)
		})

		r.Group(func(r chi.Router) {
			r.Use(middleware.Timeout(20*time.Second), a.authLimiter.middleware)
			r.Post("/auth/register", a.register)
			r.Post("/auth/login", a.login)
			r.Post("/auth/refresh", a.refresh)
		})

		r.Group(func(r chi.Router) {
			r.Use(middleware.Timeout(90*time.Second), a.authenticate)
			r.Post("/chats/{chatID}/media", a.uploadChatMedia)
			r.Get("/chats/{chatID}/media/{mediaID}", a.downloadChatMedia)
		})

		r.Group(func(r chi.Router) {
			r.Use(middleware.Timeout(20*time.Second), a.authenticate)

			r.Post("/auth/logout", a.logout)
			r.Get("/sessions", a.listSessions)
			r.Delete("/sessions/{sessionID}", a.revokeSession)
			r.Post("/sessions/revoke-others", a.revokeOtherSessions)

			r.Get("/chats", a.listChats)
			r.Post("/chats/direct", a.createDirectChat)
			r.Post("/chats/community", a.createCommunityChat)
			r.Post("/chats/join", a.joinPublicCommunityChat)
			r.Get("/chats/{chatID}", a.getCommunityChat)
			r.Get("/chats/{chatID}/members", a.listCommunityMembers)
			r.Post("/chats/{chatID}/members", a.addCommunityMember)
			r.Delete("/chats/{chatID}/members/{userID}", a.removeCommunityMember)
			r.Patch("/chats/{chatID}/members/{userID}/role", a.setCommunityMemberRole)
			r.Post("/chats/{chatID}/members/{userID}/role", a.setCommunityMemberRole)
			r.Post("/chats/{chatID}/leave", a.leaveCommunityChat)
			r.Post("/chats/{chatID}/transfer-ownership", a.transferCommunityOwnership)
			r.Get("/chats/{chatID}/messages", a.listChatMessages)
		r.Patch("/chats/{chatID}/messages/{messageID}", a.editChatMessage)
		r.Put("/chats/{chatID}/messages/{messageID}", a.editChatMessage)
		r.Post("/chats/{chatID}/messages/{messageID}/reactions", a.toggleMessageReaction)
			r.Post("/chats/{chatID}/messages/{messageID}/forward", a.forwardChatMessage)
			r.Get("/chats/{chatID}/pin", a.getPinnedChatMessage)
			r.Post("/chats/{chatID}/pin", a.pinChatMessage)
			r.Delete("/chats/{chatID}/pin", a.unpinChatMessage)
			r.Get("/chats/{chatID}/read", a.getChatReadCursors)
			r.Post("/chats/{chatID}/read", a.markChatRead)
			r.Get("/chats/{chatID}/polls", a.listPolls)
			r.Post("/chats/{chatID}/polls", a.createPoll)
			r.Get("/chats/{chatID}/polls/{pollID}", a.getPoll)
			r.Post("/chats/{chatID}/polls/{pollID}/vote", a.votePoll)
			r.Post("/chats/{chatID}/polls/{pollID}/close", a.closePoll)
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
