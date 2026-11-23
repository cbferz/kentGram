package httpapi

import (
	"fmt"
	"net/http"
	"testing"
)

func TestRealtimeEventRecoveryIsDurableAndChatScoped(t *testing.T) {
	h := newTestAPI(t).Routes()
	alice := registerUser(t, h)
	bob := registerUser(t, h)
	outsider := registerUser(t, h)

	created := call(t, h, http.MethodPost, "/api/v1/chats/direct", alice.Access, map[string]int64{"user_id": bob.ID})
	expectStatus(t, created, http.StatusOK)
	chatID := int64(asMap(t, created)["chat_id"].(float64))

	sent := call(t, h, http.MethodPost, fmt.Sprintf("/api/v1/chats/%d/messages", chatID), alice.Access, map[string]string{"text": "persist this event"})
	expectStatus(t, sent, http.StatusCreated)
	messageID := int64(asMap(t, sent)["id"].(float64))

	aliceSync := call(t, h, http.MethodGet, "/api/v1/realtime/events?since=0", alice.Access, nil)
	expectStatus(t, aliceSync, http.StatusOK)
	aliceBody := asMap(t, aliceSync)
	aliceEvents := aliceBody["events"].([]any)
	if len(aliceEvents) != 2 {
		t.Fatalf("expected chat.created and message.created, got %d: %s", len(aliceEvents), aliceSync.Body.String())
	}
	if aliceEvents[0].(map[string]any)["type"] != "chat.created" ||
		aliceEvents[1].(map[string]any)["type"] != "message.created" {
		t.Fatalf("unexpected event order: %s", aliceSync.Body.String())
	}
	cursor := int64(aliceBody["cursor"].(float64))
	highWater := int64(aliceBody["high_water"].(float64))
	if cursor <= 0 || cursor != highWater {
		t.Fatalf("cursor did not reach high-water mark: cursor=%d high=%d", cursor, highWater)
	}

	edited := call(t, h, http.MethodPatch, fmt.Sprintf("/api/v1/chats/%d/messages/%d", chatID, messageID), alice.Access, map[string]string{"text": "edited durable message"})
	expectStatus(t, edited, http.StatusOK)
	reaction := call(t, h, http.MethodPost, fmt.Sprintf("/api/v1/chats/%d/messages/%d/reactions", chatID, messageID), bob.Access, map[string]string{"emoji": "🔥"})
	expectStatus(t, reaction, http.StatusOK)

	createdPoll := call(t, h, http.MethodPost, fmt.Sprintf("/api/v1/chats/%d/polls", chatID), alice.Access, map[string]any{
		"question": "Realtime poll?", "options": []string{"Yes", "No"},
	})
	expectStatus(t, createdPoll, http.StatusCreated)
	poll := asMap(t, createdPoll)
	pollID := int64(poll["id"].(float64))
	options := poll["options"].([]any)
	optionID := int64(options[0].(map[string]any)["id"].(float64))
	vote := call(t, h, http.MethodPost, fmt.Sprintf("/api/v1/chats/%d/polls/%d/vote", chatID, pollID), bob.Access, map[string]any{"option_ids": []int64{optionID}})
	expectStatus(t, vote, http.StatusOK)
	closedPoll := call(t, h, http.MethodPost, fmt.Sprintf("/api/v1/chats/%d/polls/%d/close", chatID, pollID), alice.Access, nil)
	expectStatus(t, closedPoll, http.StatusOK)

	bobSync := call(t, h, http.MethodGet, "/api/v1/realtime/events?since=0", bob.Access, nil)
	expectStatus(t, bobSync, http.StatusOK)
	bobInitialEvents := asMap(t, bobSync)["events"].([]any)
	if len(bobInitialEvents) != 7 ||
		bobInitialEvents[2].(map[string]any)["type"] != "message.edited" ||
		bobInitialEvents[3].(map[string]any)["type"] != "message.reactions_updated" ||
		bobInitialEvents[4].(map[string]any)["type"] != "poll.created" ||
		bobInitialEvents[5].(map[string]any)["type"] != "poll.updated" ||
		bobInitialEvents[6].(map[string]any)["type"] != "poll.closed" {
		t.Fatalf("chat member did not receive durable message and poll events: %s", bobSync.Body.String())
	}

	outsideSync := call(t, h, http.MethodGet, "/api/v1/realtime/events?since=0", outsider.Access, nil)
	expectStatus(t, outsideSync, http.StatusOK)
	outsideBody := asMap(t, outsideSync)
	if len(outsideBody["events"].([]any)) != 0 ||
		outsideBody["cursor"].(float64) != outsideBody["high_water"].(float64) {
		t.Fatalf("non-member saw events or cursor stalled: %s", outsideSync.Body.String())
	}

	deleted := call(t, h, http.MethodDelete, fmt.Sprintf("/api/v1/chats/%d/messages/%d", chatID, messageID), alice.Access, nil)
	expectStatus(t, deleted, http.StatusOK)
	afterEditCursor := int64(asMap(t, bobSync)["cursor"].(float64))
	resumed := call(t, h, http.MethodGet, fmt.Sprintf("/api/v1/realtime/events?since=%d", afterEditCursor), bob.Access, nil)
	expectStatus(t, resumed, http.StatusOK)
	resumedEvents := asMap(t, resumed)["events"].([]any)
	if len(resumedEvents) != 1 || resumedEvents[0].(map[string]any)["type"] != "message.deleted" {
		t.Fatalf("resume did not return the committed delete event: %s", resumed.Body.String())
	}

	invalid := call(t, h, http.MethodGet, "/api/v1/realtime/events?since=-1", alice.Access, nil)
	expectStatus(t, invalid, http.StatusBadRequest)
}
