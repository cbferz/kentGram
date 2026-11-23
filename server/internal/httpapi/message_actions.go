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

func (a *API) editChatMessage(w http.ResponseWriter, r *http.Request) {
	chatID, err := strconv.ParseInt(chi.URLParam(r, "chatID"), 10, 64)
	if err != nil || chatID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid_chat_id")
		return
	}
	messageID, err := strconv.ParseInt(chi.URLParam(r, "messageID"), 10, 64)
	if err != nil || messageID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid_message_id")
		return
	}
	var q struct {
		Text string `json:"text"`
	}
	if decodeJSON(w, r, &q) != nil {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	q.Text = strings.TrimSpace(q.Text)
	if q.Text == "" || utf8.RuneCountInString(q.Text) > 4000 {
		writeError(w, http.StatusBadRequest, "message_must_be_1_to_4000_characters")
		return
	}
	message, err := a.db.EditChatMessage(r.Context(), chatID, messageID, userID(r), q.Text)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "message_not_found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "message_edit_failed")
		return
	}
	a.publishChatEvent(r.Context(), chatID, realtimeEvent{Type: "message.edited", ChatID: chatID, MessageID: messageID}, 0)
	writeJSON(w, http.StatusOK, message)
}

func (a *API) toggleMessageReaction(w http.ResponseWriter, r *http.Request) {
	chatID, err := strconv.ParseInt(chi.URLParam(r, "chatID"), 10, 64)
	if err != nil || chatID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid_chat_id")
		return
	}
	messageID, err := strconv.ParseInt(chi.URLParam(r, "messageID"), 10, 64)
	if err != nil || messageID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid_message_id")
		return
	}
	var q struct {
		Emoji string `json:"emoji"`
	}
	if decodeJSON(w, r, &q) != nil {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	q.Emoji = strings.TrimSpace(q.Emoji)
	if q.Emoji == "" || utf8.RuneCountInString(q.Emoji) > 16 {
		writeError(w, http.StatusBadRequest, "invalid_reaction")
		return
	}
	reactions, err := a.db.ToggleMessageReaction(r.Context(), chatID, messageID, userID(r), q.Emoji)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "message_not_found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "reaction_update_failed")
		return
	}
	a.publishChatEvent(r.Context(), chatID, realtimeEvent{Type: "message.reactions_updated", ChatID: chatID, MessageID: messageID}, 0)
	writeJSON(w, http.StatusOK, map[string]any{"reactions": reactions})
}
