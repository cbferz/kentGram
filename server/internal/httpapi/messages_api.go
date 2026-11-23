package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/cbferz/kentgram/server/internal/realtime"
	"github.com/cbferz/kentgram/server/internal/store"
	"github.com/go-chi/chi/v5"
)

const maxClientIDLen = 64

func pathID(r *http.Request, name string) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, name), 10, 64)
	return id, err == nil && id > 0
}

func validMessageText(s string) bool {
	n := utf8.RuneCountInString(s)
	return n >= 1 && n <= 4000
}

func validClientID(s string) bool {
	if len(s) > maxClientIDLen {
		return false
	}
	for _, r := range s {
		if r > unicode.MaxASCII || !(r == '-' || r == '_' || r == '.' || r == ':' ||
			(r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')) {
			return false
		}
	}
	return true
}

// validEmoji accepts a short symbol sequence (including ZWJ/skin-tone
// sequences) and rejects plain ASCII text, whitespace and control characters,
// so reactions cannot be used to attach arbitrary words to a message.
func validEmoji(s string) bool {
	if !utf8.ValidString(s) || len(s) > 64 {
		return false
	}
	n := utf8.RuneCountInString(s)
	if n < 1 || n > 16 {
		return false
	}
	nonASCII := false
	for _, r := range s {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return false
		}
		if r > unicode.MaxASCII {
			nonASCII = true
		}
	}
	return nonASCII
}

// memberOr404 reports false (after writing the response) unless the caller is
// a member of the chat. Non-members always see "chat_not_found".
func (a *API) memberOr404(w http.ResponseWriter, r *http.Request, chatID int64) bool {
	member, err := a.db.IsChatMember(r.Context(), chatID, userID(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "chat_access_failed")
		return false
	}
	if !member {
		writeError(w, http.StatusNotFound, "chat_not_found")
		return false
	}
	return true
}

func (a *API) editChatMessage(w http.ResponseWriter, r *http.Request) {
	chatID, ok1 := pathID(r, "chatID")
	messageID, ok2 := pathID(r, "messageID")
	if !ok1 {
		writeError(w, http.StatusBadRequest, "invalid_chat_id")
		return
	}
	if !ok2 {
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
	if !validMessageText(q.Text) {
		writeError(w, http.StatusBadRequest, "message_must_be_1_to_4000_characters")
		return
	}
	m, err := a.db.EditChatMessage(r.Context(), chatID, messageID, userID(r), q.Text)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "message_not_found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "message_edit_failed")
		return
	}
	a.publishChat(r.Context(), chatID, 0, realtime.Event{Type: "message.edited", Data: neutral(m)})
	writeJSON(w, http.StatusOK, m)
}

func (a *API) addReaction(w http.ResponseWriter, r *http.Request) {
	var q struct {
		Emoji string `json:"emoji"`
	}
	if decodeJSON(w, r, &q) != nil {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	a.changeReaction(w, r, strings.TrimSpace(q.Emoji), true)
}

func (a *API) removeReaction(w http.ResponseWriter, r *http.Request) {
	a.changeReaction(w, r, strings.TrimSpace(r.URL.Query().Get("emoji")), false)
}

func (a *API) changeReaction(w http.ResponseWriter, r *http.Request, emoji string, on bool) {
	chatID, ok1 := pathID(r, "chatID")
	messageID, ok2 := pathID(r, "messageID")
	if !ok1 {
		writeError(w, http.StatusBadRequest, "invalid_chat_id")
		return
	}
	if !ok2 {
		writeError(w, http.StatusBadRequest, "invalid_message_id")
		return
	}
	if !validEmoji(emoji) {
		writeError(w, http.StatusBadRequest, "invalid_emoji")
		return
	}
	res, err := a.db.SetReaction(r.Context(), chatID, messageID, userID(r), emoji, on)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "message_not_found")
		return
	}
	if errors.Is(err, store.ErrReactionLimit) {
		writeError(w, http.StatusConflict, "reaction_limit_reached")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "reaction_failed")
		return
	}
	a.publishChat(r.Context(), chatID, 0, realtime.Event{Type: "reaction.updated", Data: map[string]any{
		"message_id":   res.MessageID,
		"actor_id":     userID(r),
		"emoji":        emoji,
		"added":        on,
		"counts":       res.Counts,
		"actor_emojis": res.ActorEmoji,
	}})
	writeJSON(w, http.StatusOK, res)
}

func (a *API) forwardChatMessage(w http.ResponseWriter, r *http.Request) {
	srcChat, ok1 := pathID(r, "chatID")
	messageID, ok2 := pathID(r, "messageID")
	if !ok1 {
		writeError(w, http.StatusBadRequest, "invalid_chat_id")
		return
	}
	if !ok2 {
		writeError(w, http.StatusBadRequest, "invalid_message_id")
		return
	}
	var q struct {
		ToChatID int64  `json:"to_chat_id"`
		ClientID string `json:"client_id"`
	}
	if decodeJSON(w, r, &q) != nil {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	if q.ToChatID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid_target_chat")
		return
	}
	if !validClientID(q.ClientID) {
		writeError(w, http.StatusBadRequest, "invalid_client_id")
		return
	}
	m, created, err := a.db.ForwardMessage(r.Context(), srcChat, messageID, q.ToChatID, userID(r), q.ClientID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "message_or_chat_not_found")
		return
	}
	if errors.Is(err, store.ErrConflict) {
		writeError(w, http.StatusConflict, "client_id_conflict")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "forward_failed")
		return
	}
	if !created {
		writeJSON(w, http.StatusOK, m)
		return
	}
	a.publishChat(r.Context(), q.ToChatID, 0, realtime.Event{Type: "message.new", Data: neutral(m)})
	writeJSON(w, http.StatusCreated, m)
}

func (a *API) pinChatMessage(w http.ResponseWriter, r *http.Request) {
	chatID, ok1 := pathID(r, "chatID")
	messageID, ok2 := pathID(r, "messageID")
	if !ok1 || !ok2 {
		writeError(w, http.StatusBadRequest, "invalid_id")
		return
	}
	err := a.db.PinMessage(r.Context(), chatID, messageID, userID(r))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "message_not_found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "pin_failed")
		return
	}
	a.publishChat(r.Context(), chatID, 0, realtime.Event{Type: "message.pinned",
		Data: map[string]int64{"message_id": messageID, "actor_id": userID(r)}})
	writeJSON(w, http.StatusOK, map[string]bool{"pinned": true})
}

func (a *API) unpinChatMessage(w http.ResponseWriter, r *http.Request) {
	chatID, ok1 := pathID(r, "chatID")
	messageID, ok2 := pathID(r, "messageID")
	if !ok1 || !ok2 {
		writeError(w, http.StatusBadRequest, "invalid_id")
		return
	}
	err := a.db.UnpinMessage(r.Context(), chatID, messageID, userID(r))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "chat_not_found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "unpin_failed")
		return
	}
	a.publishChat(r.Context(), chatID, 0, realtime.Event{Type: "message.unpinned",
		Data: map[string]int64{"message_id": messageID, "actor_id": userID(r)}})
	writeJSON(w, http.StatusOK, map[string]bool{"pinned": false})
}

func (a *API) listPinnedMessages(w http.ResponseWriter, r *http.Request) {
	chatID, ok := pathID(r, "chatID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_chat_id")
		return
	}
	if !a.memberOr404(w, r, chatID) {
		return
	}
	msgs, err := a.db.PinnedMessages(r.Context(), chatID, userID(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "pins_load_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"messages": msgs})
}

func (a *API) markChatRead(w http.ResponseWriter, r *http.Request) {
	chatID, ok := pathID(r, "chatID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_chat_id")
		return
	}
	var q struct {
		MessageID int64 `json:"message_id"`
	}
	if decodeJSON(w, r, &q) != nil || q.MessageID < 0 {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	last, err := a.db.MarkRead(r.Context(), chatID, userID(r), q.MessageID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "chat_not_found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "mark_read_failed")
		return
	}
	a.publishChat(r.Context(), chatID, 0, realtime.Event{Type: "chat.read",
		Data: map[string]int64{"user_id": userID(r), "last_read_message_id": last}})
	writeJSON(w, http.StatusOK, map[string]int64{"last_read_message_id": last})
}

// typing is ephemeral: nothing is stored, peers are just told.
func (a *API) typing(w http.ResponseWriter, r *http.Request) {
	chatID, ok := pathID(r, "chatID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_chat_id")
		return
	}
	if !a.memberOr404(w, r, chatID) {
		return
	}
	a.publishChat(r.Context(), chatID, userID(r), realtime.Event{Type: "typing",
		Data: map[string]int64{"user_id": userID(r)}})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
