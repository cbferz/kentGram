package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
)

func TestRealtimeTicketIsShortLivedSessionBoundCredential(t *testing.T) {
	a := newTestAPI(t)
	h := a.Routes()
	user := registerUser(t, h)

	rec := call(t, h, http.MethodPost, "/api/v1/realtime/ticket", user.Access, nil)
	expectStatus(t, rec, http.StatusCreated)
	body := asMap(t, rec)
	ticket, ok := body["ticket"].(string)
	if !ok || ticket == "" || body["expires_in"] != float64(30) {
		t.Fatalf("invalid realtime ticket response: %v", body)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/realtime?ticket="+url.QueryEscape(ticket), nil)
	uid, sid, valid := a.realtimeIdentity(req)
	if !valid || uid != user.ID {
		t.Fatalf("ticket identity mismatch: uid=%d valid=%v", uid, valid)
	}
	active, err := a.db.SessionActive(context.Background(), sid, uid)
	if err != nil || !active {
		t.Fatalf("ticket must be bound to an active session: active=%v err=%v", active, err)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/realtime?ticket="+url.QueryEscape(ticket+"x"), nil)
	if _, _, valid := a.realtimeIdentity(req); valid {
		t.Fatal("tampered realtime ticket must be rejected")
	}
}

func TestRealtimeEventsAreDurableResumableAndChatScoped(t *testing.T) {
	a := newTestAPI(t)
	h := a.Routes()
	alice := registerUser(t, h)
	bob := registerUser(t, h)
	outsider := registerUser(t, h)

	created := call(t, h, http.MethodPost, "/api/v1/chats/direct", alice.Access,
		map[string]int64{"user_id": bob.ID})
	expectStatus(t, created, http.StatusOK)
	chatID := int64(asMap(t, created)["chat_id"].(float64))

	sent := call(t, h, http.MethodPost, "/api/v1/chats/"+itoa(chatID)+"/messages", alice.Access,
		map[string]any{"text": "realtime test"})
	expectStatus(t, sent, http.StatusCreated)
	messageID := int64(asMap(t, sent)["id"].(float64))

	aliceEvents, cursor, high, err := a.db.RealtimeBatch(context.Background(), alice.ID, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(aliceEvents) != 2 || aliceEvents[0].Type != "chat.created" || aliceEvents[1].Type != "message.created" {
		t.Fatalf("unexpected initial events: %+v", aliceEvents)
	}
	if cursor != high || cursor <= 0 {
		t.Fatalf("initial cursor did not reach high-water mark: cursor=%d high=%d", cursor, high)
	}

	bobEvents, _, _, err := a.db.RealtimeBatch(context.Background(), bob.ID, 0, 100)
	if err != nil || len(bobEvents) != 2 {
		t.Fatalf("chat member should receive both events: events=%+v err=%v", bobEvents, err)
	}
	outsiderEvents, outsiderCursor, outsiderHigh, err := a.db.RealtimeBatch(context.Background(), outsider.ID, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(outsiderEvents) != 0 || outsiderCursor != outsiderHigh {
		t.Fatalf("non-member received chat events or failed to advance cursor: %+v %d %d", outsiderEvents, outsiderCursor, outsiderHigh)
	}

	deleted := call(t, h, http.MethodDelete, "/api/v1/chats/"+itoa(chatID)+"/messages/"+itoa(messageID), alice.Access, nil)
	expectStatus(t, deleted, http.StatusOK)
	resumed, next, resumedHigh, err := a.db.RealtimeBatch(context.Background(), bob.ID, cursor, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(resumed) != 1 || resumed[0].Type != "message.deleted" || next != resumedHigh {
		t.Fatalf("resume did not return the committed delete event: events=%+v next=%d high=%d", resumed, next, resumedHigh)
	}
}

func itoa(v int64) string {
	return strconv.FormatInt(v, 10)
}
