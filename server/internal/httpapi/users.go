package httpapi

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/cbferz/kentgram/server/internal/store"
	"github.com/go-chi/chi/v5"
)


func (a *API) contacts(w http.ResponseWriter, r *http.Request) {
	users, err := a.db.FollowingUsers(r.Context(), userID(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "contacts_load_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"contacts": users})
}

func (a *API) me(w http.ResponseWriter, r *http.Request) {
	u, err := a.db.UserByID(r.Context(), userID(r))
	if err != nil {
		writeError(w, http.StatusNotFound, "user_not_found")
		return
	}
	writeJSON(w, http.StatusOK, u)
}

type profileRequest struct {
	DisplayName string `json:"display_name"`
	Bio         string `json:"bio"`
	AvatarURL   string `json:"avatar_url"`
}

func (a *API) updateProfile(w http.ResponseWriter, r *http.Request) {
	var q profileRequest
	if decodeJSON(w, r, &q) != nil {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	q.DisplayName = strings.TrimSpace(q.DisplayName)
	q.Bio = strings.TrimSpace(q.Bio)
	q.AvatarURL = strings.TrimSpace(q.AvatarURL)
	if len(q.DisplayName) > 64 || len(q.Bio) > 160 || len(q.AvatarURL) > 2048 {
		writeError(w, http.StatusBadRequest, "profile_field_too_long")
		return
	}
	if q.AvatarURL != "" {
		u, err := url.ParseRequestURI(q.AvatarURL)
		if err != nil || u.Scheme != "https" || u.Host == "" {
			writeError(w, http.StatusBadRequest, "avatar_url_must_be_https")
			return
		}
	}
	u, err := a.db.UpdateProfile(r.Context(), userID(r), q.DisplayName, q.Bio, q.AvatarURL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "profile_update_failed")
		return
	}
	writeJSON(w, http.StatusOK, u)
}

func (a *API) publicProfile(w http.ResponseWriter, r *http.Request) {
	p, err := a.db.PublicProfile(r.Context(), chi.URLParam(r, "username"), userID(r))
	if err != nil {
		writeError(w, http.StatusNotFound, "user_not_found")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (a *API) searchUsers(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(q) < 2 || len(q) > 64 {
		writeError(w, http.StatusBadRequest, "query_must_be_2_to_64_characters")
		return
	}
	users, err := a.db.SearchUsers(r.Context(), q, userID(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "user_search_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": users})
}

func (a *API) follow(w http.ResponseWriter, r *http.Request)   { a.changeFollow(w, r, true) }
func (a *API) unfollow(w http.ResponseWriter, r *http.Request) { a.changeFollow(w, r, false) }

func (a *API) changeFollow(w http.ResponseWriter, r *http.Request, follow bool) {
	viewer := userID(r)
	target, err := strconv.ParseInt(chi.URLParam(r, "userID"), 10, 64)
	if err != nil || target <= 0 {
		writeError(w, http.StatusBadRequest, "invalid_user_id")
		return
	}
	if viewer == target {
		writeError(w, http.StatusBadRequest, "cannot_follow_self")
		return
	}
	if follow {
		err = a.db.Follow(r.Context(), viewer, target)
	} else {
		err = a.db.Unfollow(r.Context(), viewer, target)
	}
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "user_not_found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "follow_update_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "following": follow})
}
