package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/cbferz/kentgram/server/internal/store"
	"github.com/go-chi/chi/v5"
)

func parseCommunityChatID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "chatID"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid_chat_id")
		return 0, false
	}
	return id, true
}

func (a *API) createCommunityChat(w http.ResponseWriter, r *http.Request) {
	var q struct {
		Kind        string `json:"kind"`
		Title       string `json:"title"`
		Description string `json:"description"`
		IsPublic    bool   `json:"is_public"`
		Username    string `json:"username"`
	}
	if decodeJSON(w, r, &q) != nil {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	q.Kind = strings.ToLower(strings.TrimSpace(q.Kind))
	q.Title = strings.TrimSpace(q.Title)
	q.Description = strings.TrimSpace(q.Description)
	q.Username = strings.TrimPrefix(strings.TrimSpace(q.Username), "@")
	if (q.Kind != "group" && q.Kind != "channel") ||
		utf8.RuneCountInString(q.Title) < 1 || utf8.RuneCountInString(q.Title) > 80 ||
		utf8.RuneCountInString(q.Description) > 500 ||
		(q.IsPublic && !usernamePattern.MatchString(q.Username)) ||
		(!q.IsPublic && q.Username != "") {
		writeError(w, http.StatusBadRequest, "invalid_community_chat")
		return
	}
	chat, err := a.db.CreateCommunityChat(r.Context(), userID(r), q.Kind, q.Title, q.Description, q.Username, q.IsPublic)
	if errors.Is(err, store.ErrConflict) {
		writeError(w, http.StatusConflict, "chat_username_taken")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "chat_create_failed")
		return
	}
	writeJSON(w, http.StatusCreated, chat)
}

func (a *API) getCommunityChat(w http.ResponseWriter, r *http.Request) {
	id, ok := parseCommunityChatID(w, r)
	if !ok { return }
	chat, err := a.db.CommunityChat(r.Context(), id, userID(r))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "chat_not_found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "chat_load_failed")
		return
	}
	writeJSON(w, http.StatusOK, chat)
}

func (a *API) listCommunityMembers(w http.ResponseWriter, r *http.Request) {
	id, ok := parseCommunityChatID(w, r)
	if !ok { return }
	members, err := a.db.ListCommunityMembers(r.Context(), id, userID(r))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "chat_not_found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "members_load_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"members": members})
}

func (a *API) addCommunityMember(w http.ResponseWriter, r *http.Request) {
	id, ok := parseCommunityChatID(w, r)
	if !ok { return }
	var q struct { UserID int64 `json:"user_id"` }
	if decodeJSON(w, r, &q) != nil || q.UserID <= 0 || q.UserID == userID(r) {
		writeError(w, http.StatusBadRequest, "invalid_user_id")
		return
	}
	err := a.db.AddCommunityMember(r.Context(), id, userID(r), q.UserID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "chat_or_permission_not_found")
		return
	}
	if errors.Is(err, store.ErrConflict) {
		writeError(w, http.StatusConflict, "user_already_member")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "member_add_failed")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]bool{"added": true})
}

func (a *API) leaveCommunityChat(w http.ResponseWriter, r *http.Request) {
	id, ok := parseCommunityChatID(w, r)
	if !ok { return }
	err := a.db.LeaveCommunityChat(r.Context(), id, userID(r))
	if errors.Is(err, store.ErrConflict) {
		writeError(w, http.StatusConflict, "owner_must_transfer_ownership")
		return
	}
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "chat_not_found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "chat_leave_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"left": true})
}

func (a *API) setCommunityMemberRole(w http.ResponseWriter, r *http.Request) {
	id, ok := parseCommunityChatID(w, r)
	if !ok { return }
	targetID, err := strconv.ParseInt(chi.URLParam(r, "userID"), 10, 64)
	if err != nil || targetID <= 0 || targetID == userID(r) {
		writeError(w, http.StatusBadRequest, "invalid_user_id")
		return
	}
	var q struct { Role string `json:"role"` }
	if decodeJSON(w, r, &q) != nil || (q.Role != "admin" && q.Role != "member") {
		writeError(w, http.StatusBadRequest, "invalid_member_role")
		return
	}
	err = a.db.SetCommunityMemberRole(r.Context(), id, userID(r), targetID, q.Role)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "member_or_permission_not_found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "member_role_update_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"updated": true})
}

func (a *API) transferCommunityOwnership(w http.ResponseWriter, r *http.Request) {
	id, ok := parseCommunityChatID(w, r)
	if !ok { return }
	var q struct { UserID int64 `json:"user_id"` }
	if decodeJSON(w, r, &q) != nil || q.UserID <= 0 || q.UserID == userID(r) {
		writeError(w, http.StatusBadRequest, "invalid_user_id")
		return
	}
	err := a.db.TransferCommunityOwnership(r.Context(), id, userID(r), q.UserID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "member_or_permission_not_found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ownership_transfer_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"transferred": true})
}

func (a *API) joinPublicCommunityChat(w http.ResponseWriter, r *http.Request) {
	var q struct { Username string `json:"username"` }
	if decodeJSON(w, r, &q) != nil {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	q.Username = strings.TrimPrefix(strings.TrimSpace(q.Username), "@")
	if !usernamePattern.MatchString(q.Username) {
		writeError(w, http.StatusBadRequest, "invalid_chat_username")
		return
	}
	chatID, err := a.db.JoinPublicCommunityChat(r.Context(), userID(r), q.Username)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "public_chat_not_found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "chat_join_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"joined": true, "chat_id": chatID})
}
