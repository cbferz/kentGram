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

func (a *API) createPoll(w http.ResponseWriter, r *http.Request) {
	chatID, err := strconv.ParseInt(chi.URLParam(r,"chatID"),10,64)
	if err != nil || chatID<=0 { writeError(w,http.StatusBadRequest,"invalid_chat_id"); return }
	var q struct {
		Question string `json:"question"`
		Options []string `json:"options"`
		MultipleChoice bool `json:"multiple_choice"`
		Anonymous bool `json:"anonymous"`
	}
	if decodeJSON(w,r,&q)!=nil { writeError(w,http.StatusBadRequest,"invalid_json"); return }
	q.Question=strings.TrimSpace(q.Question)
	if utf8.RuneCountInString(q.Question)<1 || utf8.RuneCountInString(q.Question)>300 || len(q.Options)<2 || len(q.Options)>10 {
		writeError(w,http.StatusBadRequest,"poll_requires_question_and_2_to_10_options"); return
	}
	for i := range q.Options {
		q.Options[i]=strings.TrimSpace(q.Options[i])
		if n:=utf8.RuneCountInString(q.Options[i]); n<1 || n>100 {
			writeError(w,http.StatusBadRequest,"poll_option_must_be_1_to_100_characters"); return
		}
	}
	p, err := a.db.CreatePoll(r.Context(),chatID,userID(r),q.Question,q.Options,q.MultipleChoice,q.Anonymous)
	if errors.Is(err,store.ErrNotFound) { writeError(w,http.StatusNotFound,"chat_or_permission_not_found"); return }
	if err!=nil { writeError(w,http.StatusInternalServerError,"poll_create_failed"); return }
	a.publishChatEvent(r.Context(),chatID,realtimeEvent{Type:"poll.created",ChatID:chatID,MessageID:p.ID},0)
	writeJSON(w,http.StatusCreated,p)
}


func (a *API) listPolls(w http.ResponseWriter, r *http.Request) {
	chatID, err := strconv.ParseInt(chi.URLParam(r,"chatID"),10,64)
	if err != nil || chatID<=0 { writeError(w,http.StatusBadRequest,"invalid_chat_id"); return }
	polls,err:=a.db.ListPolls(r.Context(),chatID,userID(r),50)
	if errors.Is(err,store.ErrNotFound) { writeError(w,http.StatusNotFound,"chat_not_found"); return }
	if err!=nil { writeError(w,http.StatusInternalServerError,"polls_load_failed"); return }
	writeJSON(w,http.StatusOK,map[string]any{"polls":polls})
}

func (a *API) getPoll(w http.ResponseWriter, r *http.Request) {
	chatID, e1 := strconv.ParseInt(chi.URLParam(r,"chatID"),10,64)
	pollID, e2 := strconv.ParseInt(chi.URLParam(r,"pollID"),10,64)
	if e1!=nil || e2!=nil || chatID<=0 || pollID<=0 { writeError(w,http.StatusBadRequest,"invalid_poll_id"); return }
	p,err:=a.db.GetPoll(r.Context(),chatID,pollID,userID(r))
	if errors.Is(err,store.ErrNotFound) { writeError(w,http.StatusNotFound,"poll_not_found"); return }
	if err!=nil { writeError(w,http.StatusInternalServerError,"poll_load_failed"); return }
	writeJSON(w,http.StatusOK,p)
}

func (a *API) votePoll(w http.ResponseWriter, r *http.Request) {
	chatID,e1:=strconv.ParseInt(chi.URLParam(r,"chatID"),10,64)
	pollID,e2:=strconv.ParseInt(chi.URLParam(r,"pollID"),10,64)
	if e1!=nil || e2!=nil || chatID<=0 || pollID<=0 { writeError(w,http.StatusBadRequest,"invalid_poll_id"); return }
	var q struct { OptionIDs []int64 `json:"option_ids"` }
	if decodeJSON(w,r,&q)!=nil || len(q.OptionIDs)==0 || len(q.OptionIDs)>10 { writeError(w,http.StatusBadRequest,"invalid_poll_vote"); return }
	p,err:=a.db.VotePoll(r.Context(),chatID,pollID,userID(r),q.OptionIDs)
	if errors.Is(err,store.ErrNotFound) { writeError(w,http.StatusNotFound,"poll_or_option_not_found"); return }
	if errors.Is(err,store.ErrConflict) { writeError(w,http.StatusConflict,"poll_closed_or_vote_invalid"); return }
	if err!=nil { writeError(w,http.StatusInternalServerError,"poll_vote_failed"); return }
	a.publishChatEvent(r.Context(),chatID,realtimeEvent{Type:"poll.updated",ChatID:chatID,MessageID:pollID},userID(r))
	writeJSON(w,http.StatusOK,p)
}

func (a *API) closePoll(w http.ResponseWriter, r *http.Request) {
	chatID,e1:=strconv.ParseInt(chi.URLParam(r,"chatID"),10,64)
	pollID,e2:=strconv.ParseInt(chi.URLParam(r,"pollID"),10,64)
	if e1!=nil || e2!=nil || chatID<=0 || pollID<=0 { writeError(w,http.StatusBadRequest,"invalid_poll_id"); return }
	err:=a.db.ClosePoll(r.Context(),chatID,pollID,userID(r))
	if errors.Is(err,store.ErrNotFound) { writeError(w,http.StatusNotFound,"poll_or_permission_not_found"); return }
	if err!=nil { writeError(w,http.StatusInternalServerError,"poll_close_failed"); return }
	a.publishChatEvent(r.Context(),chatID,realtimeEvent{Type:"poll.closed",ChatID:chatID,MessageID:pollID},0)
	writeJSON(w,http.StatusOK,map[string]any{"closed":true,"poll_id":pollID})
}
