package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/cbferz/kentgram/server/internal/realtime"
	"github.com/cbferz/kentgram/server/internal/store"
)

func (a *API) listChats(w http.ResponseWriter, r *http.Request) {
	chats, err := a.db.ListChats(r.Context(), userID(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "chats_load_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"chats": chats})
}

func (a *API) createDirectChat(w http.ResponseWriter, r *http.Request) {
	var q struct {
		UserID int64 `json:"user_id"`
	}
	if decodeJSON(w, r, &q) != nil {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	if q.UserID <= 0 || q.UserID == userID(r) {
		writeError(w, http.StatusBadRequest, "invalid_peer")
		return
	}
	if _, err := a.db.UserByID(r.Context(), q.UserID); errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "user_not_found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "chat_create_failed")
		return
	}
	id, err := a.db.CreateDirectChat(r.Context(), userID(r), q.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "chat_create_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{"chat_id": id})
}

func (a *API) listChatMessages(w http.ResponseWriter, r *http.Request) {
	chatID, ok := pathID(r, "chatID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_chat_id")
		return
	}
	if !a.memberOr404(w, r, chatID) {
		return
	}
	before, _ := strconv.ParseInt(r.URL.Query().Get("before_id"), 10, 64)
	messages, err := a.db.ChatMessages(r.Context(), chatID, userID(r), before, 50)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "messages_load_failed")
		return
	}
	peerRead, err := a.db.PeerLastRead(r.Context(), chatID, userID(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "messages_load_failed")
		return
	}
	// Loading history marks the chat read for this viewer; tell the peers.
	if last, err := a.db.MarkRead(r.Context(), chatID, userID(r), 1<<62); err == nil {
		a.publishChat(r.Context(), chatID, userID(r), realtime.Event{Type: "chat.read",
			Data: map[string]int64{"user_id": userID(r), "last_read_message_id": last}})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"messages":                  messages,
		"peer_last_read_message_id": peerRead,
	})
}

func (a *API) sendChatMessage(w http.ResponseWriter, r *http.Request) {
	chatID, ok := pathID(r, "chatID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_chat_id")
		return
	}
	var q struct {
		Text             string `json:"text"`
		ReplyToMessageID int64  `json:"reply_to_message_id"`
		ClientID         string `json:"client_id"`
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
	if q.ReplyToMessageID < 0 || !validClientID(q.ClientID) {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	m, created, err := a.db.SendMessage(r.Context(), store.SendInput{
		ChatID: chatID, SenderID: userID(r), Body: q.Text,
		ReplyToID: q.ReplyToMessageID, ClientID: q.ClientID,
	})
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "chat_or_reply_not_found")
		return
	}
	if errors.Is(err, store.ErrConflict) {
		writeError(w, http.StatusConflict, "client_id_conflict")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "message_send_failed")
		return
	}
	if !created {
		// Idempotent replay of an earlier send: return it, do not broadcast again.
		writeJSON(w, http.StatusOK, m)
		return
	}
	a.publishChat(r.Context(), chatID, 0, realtime.Event{Type: "message.new", Data: neutral(m)})
	writeJSON(w, http.StatusCreated, m)
}

func (a *API) deleteChatMessage(w http.ResponseWriter, r *http.Request) {
	chatID, ok := pathID(r, "chatID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_chat_id")
		return
	}
	messageID, ok := pathID(r, "messageID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_message_id")
		return
	}
	err := a.db.DeleteChatMessage(r.Context(), chatID, messageID, userID(r))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "message_not_found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "message_delete_failed")
		return
	}
	a.publishChat(r.Context(), chatID, 0, realtime.Event{Type: "message.deleted",
		Data: map[string]int64{"message_id": messageID}})
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}
