package httpapi

import (
    "errors"
    "net/http"
    "strconv"
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
    var q struct { Text string `json:"text"` }
    if decodeJSON(w,r,&q)!=nil { writeError(w,http.StatusBadRequest,"invalid_json");return }
    q.Text=strings.TrimSpace(q.Text)
    if q.Text=="" || len([]rune(q.Text))>4000 { writeError(w,http.StatusBadRequest,"message_must_be_1_to_4000_characters");return }
    m,err:=a.db.SendChatMessage(r.Context(),chatID,userID(r),q.Text)
    if errors.Is(err,store.ErrNotFound) { writeError(w,http.StatusNotFound,"chat_not_found");return }
    if err!=nil { writeError(w,http.StatusInternalServerError,"message_send_failed");return }
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
    err = a.db.DeleteChatMessage(r.Context(), chatID, messageID, userID(r))
    if errors.Is(err, store.ErrNotFound) {
        writeError(w, http.StatusNotFound, "message_not_found")
        return
    }
    if err != nil {
        writeError(w, http.StatusInternalServerError, "message_delete_failed")
        return
    }
    writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}
