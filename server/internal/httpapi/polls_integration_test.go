package httpapi

import (
	"fmt"
	"net/http"
	"testing"
)

func TestPollCreateVoteChangeAndClose(t *testing.T) {
	api := newTestAPI(t)
	h := api.Routes()
	owner := registerUser(t,h)
	member := registerUser(t,h)

	created := call(t,h,http.MethodPost,"/api/v1/chats/community",owner.Access,map[string]any{
		"kind":"group","title":"Poll room",
	})
	expectStatus(t,created,http.StatusCreated)
	chatID:=int64(asMap(t,created)["id"].(float64))
	added:=call(t,h,http.MethodPost,fmt.Sprintf("/api/v1/chats/%d/members",chatID),owner.Access,map[string]int64{"user_id":member.ID})
	expectStatus(t,added,http.StatusCreated)

	pollRec:=call(t,h,http.MethodPost,fmt.Sprintf("/api/v1/chats/%d/polls",chatID),owner.Access,map[string]any{
		"question":"Choose one","options":[]string{"Alpha","Beta"},"anonymous":true,
	})
	expectStatus(t,pollRec,http.StatusCreated)
	poll:=asMap(t,pollRec)
	pollID:=int64(poll["id"].(float64))
	options:=poll["options"].([]any)
	first:=int64(options[0].(map[string]any)["id"].(float64))
	second:=int64(options[1].(map[string]any)["id"].(float64))

	voted:=call(t,h,http.MethodPost,fmt.Sprintf("/api/v1/chats/%d/polls/%d/vote",chatID,pollID),member.Access,map[string][]int64{"option_ids":{first}})
	expectStatus(t,voted,http.StatusOK)
	if int64(asMap(t,voted)["total_votes"].(float64))!=1 { t.Fatal("expected one distinct voter") }

	changed:=call(t,h,http.MethodPost,fmt.Sprintf("/api/v1/chats/%d/polls/%d/vote",chatID,pollID),member.Access,map[string][]int64{"option_ids":{second}})
	expectStatus(t,changed,http.StatusOK)
	changedPoll:=asMap(t,changed)
	changedOptions:=changedPoll["options"].([]any)
	if int64(changedOptions[0].(map[string]any)["votes"].(float64))!=0 || int64(changedOptions[1].(map[string]any)["votes"].(float64))!=1 {
		t.Fatalf("single-choice vote was not replaced: %#v",changedOptions)
	}

	closed:=call(t,h,http.MethodPost,fmt.Sprintf("/api/v1/chats/%d/polls/%d/close",chatID,pollID),owner.Access,nil)
	expectStatus(t,closed,http.StatusOK)
	afterClose:=call(t,h,http.MethodPost,fmt.Sprintf("/api/v1/chats/%d/polls/%d/vote",chatID,pollID),member.Access,map[string][]int64{"option_ids":{first}})
	expectStatus(t,afterClose,http.StatusConflict)
}

func TestChannelPollRequiresPublisherRole(t *testing.T) {
	api:=newTestAPI(t)
	h:=api.Routes()
	owner:=registerUser(t,h)
	member:=registerUser(t,h)
	created:=call(t,h,http.MethodPost,"/api/v1/chats/community",owner.Access,map[string]any{"kind":"channel","title":"News"})
	expectStatus(t,created,http.StatusCreated)
	chatID:=int64(asMap(t,created)["id"].(float64))
	added:=call(t,h,http.MethodPost,fmt.Sprintf("/api/v1/chats/%d/members",chatID),owner.Access,map[string]int64{"user_id":member.ID})
	expectStatus(t,added,http.StatusCreated)
	blocked:=call(t,h,http.MethodPost,fmt.Sprintf("/api/v1/chats/%d/polls",chatID),member.Access,map[string]any{
		"question":"Unauthorized","options":[]string{"A","B"},
	})
	expectStatus(t,blocked,http.StatusNotFound)
}
