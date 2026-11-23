package httpapi

import (
    "errors"
    "net/http"
    "strconv"
    "os"
    "path/filepath"
    "strings"

    "github.com/cbferz/kentgram/server/internal/store"
    "github.com/go-chi/chi/v5"
)

func (a *API) listChats(w http.ResponseWriter,r *http.Request) {
    chats,err:=a.db.ListChats(r.Context(),userID(r))
    if err!=nil { writeError(w,http.StatusInternalServerError,"chats_load_failed");return }
    writeJSON(w,http.StatusOK,map[string]any{"chats":chats})
}

func (a *API) createDirectChat(w http.ResponseWriter,r *http.Request) {
    var q struct { UserID int64 `json:"user_id"` }
    if decodeJSON(w,r,&q)!=nil { writeError(w,http.StatusBadRequest,"invalid_json");return }
    if q.UserID<=0 || q.UserID==userID(r) { writeError(w,http.StatusBadRequest,"invalid_peer");return }
    if _,err:=a.db.UserByID(r.Context(),q.UserID);errors.Is(err,store.ErrNotFound) {
        writeError(w,http.StatusNotFound,"user_not_found");return
    } else if err!=nil { writeError(w,http.StatusInternalServerError,"chat_create_failed");return }
    id,err:=a.db.CreateDirectChat(r.Context(),userID(r),q.UserID)
    if err!=nil { writeError(w,http.StatusInternalServerError,"chat_create_failed");return }
    writeJSON(w,http.StatusOK,map[string]int64{"chat_id":id})
}

func (a *API) listChatMessages(w http.ResponseWriter,r *http.Request) {
    chatID,err:=strconv.ParseInt(chi.URLParam(r,"chatID"),10,64)
    if err!=nil || chatID<=0 { writeError(w,http.StatusBadRequest,"invalid_chat_id");return }
    member,err:=a.db.IsChatMember(r.Context(),chatID,userID(r))
    if err!=nil { writeError(w,http.StatusInternalServerError,"chat_access_failed");return }
    if !member { writeError(w,http.StatusNotFound,"chat_not_found");return }
    before,_:=strconv.ParseInt(r.URL.Query().Get("before_id"),10,64)
    messages,err:=a.db.ChatMessages(r.Context(),chatID,userID(r),before,50)
    if err!=nil { writeError(w,http.StatusInternalServerError,"messages_load_failed");return }
    writeJSON(w,http.StatusOK,map[string]any{"messages":messages})
}

func (a *API) sendChatMessage(w http.ResponseWriter,r *http.Request) {
    chatID,err:=strconv.ParseInt(chi.URLParam(r,"chatID"),10,64)
    if err!=nil || chatID<=0 { writeError(w,http.StatusBadRequest,"invalid_chat_id");return }
    var q struct { Text string `json:"text"`; ReplyToMessageID int64 `json:"reply_to_message_id"` }
    if decodeJSON(w,r,&q)!=nil { writeError(w,http.StatusBadRequest,"invalid_json");return }
    q.Text=strings.TrimSpace(q.Text)
    if q.Text=="" || len([]rune(q.Text))>4000 { writeError(w,http.StatusBadRequest,"message_must_be_1_to_4000_characters");return }
    m,err:=a.db.SendChatMessage(r.Context(),chatID,userID(r),q.Text,q.ReplyToMessageID)
    if errors.Is(err,store.ErrNotFound) { writeError(w,http.StatusNotFound,"chat_or_reply_not_found");return }
    if err!=nil { writeError(w,http.StatusInternalServerError,"message_send_failed");return }
    a.publishChatEvent(r.Context(), chatID, realtimeEvent{Type:"message.created", ChatID:chatID, MessageID:m.ID}, 0)
    writeJSON(w,http.StatusCreated,m)
}


func (a *API) deleteChatMessage(w http.ResponseWriter, r *http.Request) {
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
    storageName, err := a.db.DeleteChatMessage(r.Context(), chatID, messageID, userID(r))
    if errors.Is(err, store.ErrNotFound) {
        writeError(w, http.StatusNotFound, "message_not_found")
        return
    }
    if err != nil {
        writeError(w, http.StatusInternalServerError, "message_delete_failed")
        return
    }
    if storageName != "" {
        _ = os.Remove(filepath.Join(a.cfg.MediaDir, storageName))
    }
    a.publishChatEvent(r.Context(), chatID, realtimeEvent{Type:"message.deleted", ChatID:chatID, MessageID:messageID}, 0)
    writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

func (a *API) markChatRead(w http.ResponseWriter, r *http.Request) {
	chatID, err := strconv.ParseInt(chi.URLParam(r, "chatID"), 10, 64)
	if err != nil || chatID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid_chat_id")
		return
	}
	var q struct {
		LastReadMessageID int64 `json:"last_read_message_id"`
	}
	if decodeJSON(w, r, &q) != nil || q.LastReadMessageID < 0 {
		writeError(w, http.StatusBadRequest, "invalid_read_cursor")
		return
	}
	cursor, err := a.db.MarkChatRead(r.Context(), chatID, userID(r), q.LastReadMessageID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "chat_not_found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "chat_read_update_failed")
		return
	}
	a.publishChatEvent(r.Context(), chatID, realtimeEvent{
		Type: "chat.read", ChatID: chatID, MessageID: cursor, UserID: userID(r),
	}, 0)
	writeJSON(w, http.StatusOK, map[string]int64{"last_read_message_id": cursor})
}

func (a *API) getChatReadCursors(w http.ResponseWriter, r *http.Request) {
	chatID, err := strconv.ParseInt(chi.URLParam(r, "chatID"), 10, 64)
	if err != nil || chatID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid_chat_id")
		return
	}
	cursors, err := a.db.ChatReadCursors(r.Context(), chatID, userID(r))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "chat_not_found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "read_cursors_load_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"read_cursors": cursors})
}

func (a *API) forwardChatMessage(w http.ResponseWriter, r *http.Request) {
	sourceChatID, err := strconv.ParseInt(chi.URLParam(r, "chatID"), 10, 64)
	if err != nil || sourceChatID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid_chat_id")
		return
	}
	messageID, err := strconv.ParseInt(chi.URLParam(r, "messageID"), 10, 64)
	if err != nil || messageID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid_message_id")
		return
	}
	var q struct { DestinationChatID int64 `json:"destination_chat_id"` }
	if decodeJSON(w, r, &q) != nil || q.DestinationChatID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid_destination_chat")
		return
	}
	message, err := a.db.ForwardChatMessage(r.Context(), sourceChatID, messageID, q.DestinationChatID, userID(r))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "source_or_destination_not_found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "message_forward_failed")
		return
	}
	a.publishChatEvent(r.Context(), q.DestinationChatID, realtimeEvent{
		Type: "message.created", ChatID: q.DestinationChatID, MessageID: message.ID,
	}, 0)
	writeJSON(w, http.StatusCreated, message)
}

func (a *API) pinChatMessage(w http.ResponseWriter, r *http.Request) {
	chatID, err := strconv.ParseInt(chi.URLParam(r, "chatID"), 10, 64)
	if err != nil || chatID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid_chat_id")
		return
	}
	var q struct { MessageID int64 `json:"message_id"` }
	if decodeJSON(w, r, &q) != nil || q.MessageID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid_message_id")
		return
	}
	if err := a.db.PinChatMessage(r.Context(), chatID, q.MessageID, userID(r)); errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "message_or_permission_not_found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "message_pin_failed")
		return
	}
	a.publishChatEvent(r.Context(), chatID, realtimeEvent{
		Type: "chat.pinned", ChatID: chatID, MessageID: q.MessageID, UserID: userID(r),
	}, 0)
	writeJSON(w, http.StatusOK, map[string]any{"pinned": true, "message_id": q.MessageID})
}

func (a *API) unpinChatMessage(w http.ResponseWriter, r *http.Request) {
	chatID, err := strconv.ParseInt(chi.URLParam(r, "chatID"), 10, 64)
	if err != nil || chatID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid_chat_id")
		return
	}
	if err := a.db.UnpinChatMessage(r.Context(), chatID, userID(r)); errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "pin_or_permission_not_found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "message_unpin_failed")
		return
	}
	a.publishChatEvent(r.Context(), chatID, realtimeEvent{Type: "chat.unpinned", ChatID: chatID, UserID: userID(r)}, 0)
	writeJSON(w, http.StatusOK, map[string]bool{"unpinned": true})
}

func (a *API) getPinnedChatMessage(w http.ResponseWriter, r *http.Request) {
	chatID, err := strconv.ParseInt(chi.URLParam(r, "chatID"), 10, 64)
	if err != nil || chatID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid_chat_id")
		return
	}
	message, err := a.db.PinnedChatMessage(r.Context(), chatID, userID(r))
	if errors.Is(err, store.ErrNotFound) {
		member, memberErr := a.db.IsChatMember(r.Context(), chatID, userID(r))
		if memberErr != nil {
			writeError(w, http.StatusInternalServerError, "chat_access_failed")
			return
		}
		if !member {
			writeError(w, http.StatusNotFound, "chat_not_found")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"pinned_message": nil})
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "pinned_message_load_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"pinned_message": message})
}
