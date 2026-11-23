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

func TestCommunityMembershipEventsReachAffectedUsers(t *testing.T) {
	h := newTestAPI(t).Routes()
	owner := registerUser(t, h)
	member := registerUser(t, h)
	outsider := registerUser(t, h)

	created := call(t, h, http.MethodPost, "/api/v1/chats/community", owner.Access, map[string]any{
		"kind": "group", "title": "Realtime community",
	})
	expectStatus(t, created, http.StatusCreated)
	chatID := int64(asMap(t, created)["id"].(float64))

	added := call(t, h, http.MethodPost, fmt.Sprintf("/api/v1/chats/%d/members", chatID), owner.Access,
		map[string]int64{"user_id": member.ID})
	expectStatus(t, added, http.StatusCreated)

	promoted := call(t, h, http.MethodPatch, fmt.Sprintf("/api/v1/chats/%d/members/%d/role", chatID, member.ID),
		owner.Access, map[string]string{"role": "admin"})
	expectStatus(t, promoted, http.StatusOK)

	removed := call(t, h, http.MethodDelete, fmt.Sprintf("/api/v1/chats/%d/members/%d", chatID, member.ID),
		owner.Access, nil)
	expectStatus(t, removed, http.StatusOK)

	ownerSync := call(t, h, http.MethodGet, "/api/v1/realtime/events?since=0", owner.Access, nil)
	expectStatus(t, ownerSync, http.StatusOK)
	ownerEvents := asMap(t, ownerSync)["events"].([]any)
	wantOwnerTypes := []string{
		"community.created", "community.member_added",
		"community.member_role_changed", "community.member_removed",
	}
	if len(ownerEvents) != len(wantOwnerTypes) {
		t.Fatalf("owner event count = %d, want %d: %s", len(ownerEvents), len(wantOwnerTypes), ownerSync.Body.String())
	}
	for i, want := range wantOwnerTypes {
		if got := ownerEvents[i].(map[string]any)["type"]; got != want {
			t.Fatalf("owner event %d type = %v, want %s", i, got, want)
		}
	}

	// A removed member must receive the removal event even though they are no
	// longer a chat member; the event lets the client discard stale chat data.
	memberSync := call(t, h, http.MethodGet, "/api/v1/realtime/events?since=0", member.Access, nil)
	expectStatus(t, memberSync, http.StatusOK)
	memberEvents := asMap(t, memberSync)["events"].([]any)
	if len(memberEvents) != 1 || memberEvents[0].(map[string]any)["type"] != "community.member_removed" {
		t.Fatalf("removed member did not receive exactly the removal event: %s", memberSync.Body.String())
	}
	payload := memberEvents[0].(map[string]any)["payload"].(map[string]any)
	if int64(payload["user_id"].(float64)) != member.ID {
		t.Fatalf("removal event target = %v, want %d", payload["user_id"], member.ID)
	}

	outsideSync := call(t, h, http.MethodGet, "/api/v1/realtime/events?since=0", outsider.Access, nil)
	expectStatus(t, outsideSync, http.StatusOK)
	outside := asMap(t, outsideSync)
	if len(outside["events"].([]any)) != 0 ||
		outside["cursor"].(float64) != outside["high_water"].(float64) {
		t.Fatalf("outsider received community events or cursor stalled: %s", outsideSync.Body.String())
	}
}

func TestCommunityLeaveEventReachesLeavingMember(t *testing.T) {
	h := newTestAPI(t).Routes()
	owner := registerUser(t, h)
	member := registerUser(t, h)

	created := call(t, h, http.MethodPost, "/api/v1/chats/community", owner.Access, map[string]any{
		"kind": "group", "title": "Leave event",
	})
	expectStatus(t, created, http.StatusCreated)
	chatID := int64(asMap(t, created)["id"].(float64))
	added := call(t, h, http.MethodPost, fmt.Sprintf("/api/v1/chats/%d/members", chatID), owner.Access,
		map[string]int64{"user_id": member.ID})
	expectStatus(t, added, http.StatusCreated)

	left := call(t, h, http.MethodPost, fmt.Sprintf("/api/v1/chats/%d/leave", chatID), member.Access, nil)
	expectStatus(t, left, http.StatusOK)

	sync := call(t, h, http.MethodGet, "/api/v1/realtime/events?since=0", member.Access, nil)
	expectStatus(t, sync, http.StatusOK)
	events := asMap(t, sync)["events"].([]any)
	if len(events) != 1 || events[0].(map[string]any)["type"] != "community.member_left" {
		t.Fatalf("leaving member did not receive leave event: %s", sync.Body.String())
	}
}

func TestPublicCommunityJoinEmitsOnlyNewMembershipEvent(t *testing.T) {
	h := newTestAPI(t).Routes()
	owner := registerUser(t, h)
	visitor := registerUser(t, h)
	handle := newName()

	created := call(t, h, http.MethodPost, "/api/v1/chats/community", owner.Access, map[string]any{
		"kind": "channel", "title": "Public realtime", "is_public": true, "username": handle,
	})
	expectStatus(t, created, http.StatusCreated)

	joined := call(t, h, http.MethodPost, "/api/v1/chats/join", visitor.Access, map[string]string{"username": handle})
	expectStatus(t, joined, http.StatusOK)
	first := call(t, h, http.MethodGet, "/api/v1/realtime/events?since=0", visitor.Access, nil)
	expectStatus(t, first, http.StatusOK)
	firstEvents := asMap(t, first)["events"].([]any)
	if len(firstEvents) != 1 || firstEvents[0].(map[string]any)["type"] != "community.member_joined" {
		t.Fatalf("new public member did not receive join event: %s", first.Body.String())
	}
	cursor := int64(asMap(t, first)["cursor"].(float64))

	joinedAgain := call(t, h, http.MethodPost, "/api/v1/chats/join", visitor.Access, map[string]string{"username": handle})
	expectStatus(t, joinedAgain, http.StatusOK)
	second := call(t, h, http.MethodGet, fmt.Sprintf("/api/v1/realtime/events?since=%d", cursor), visitor.Access, nil)
	expectStatus(t, second, http.StatusOK)
	if len(asMap(t, second)["events"].([]any)) != 0 {
		t.Fatalf("idempotent repeat join emitted another event: %s", second.Body.String())
	}
}

func TestCommunityOwnershipTransferEmitsRealtimeEvent(t *testing.T) {
	h := newTestAPI(t).Routes()
	owner := registerUser(t, h)
	successor := registerUser(t, h)

	created := call(t, h, http.MethodPost, "/api/v1/chats/community", owner.Access, map[string]any{
		"kind": "channel", "title": "Ownership realtime",
	})
	expectStatus(t, created, http.StatusCreated)
	chatID := int64(asMap(t, created)["id"].(float64))
	added := call(t, h, http.MethodPost, fmt.Sprintf("/api/v1/chats/%d/members", chatID), owner.Access,
		map[string]int64{"user_id": successor.ID})
	expectStatus(t, added, http.StatusCreated)

	transferred := call(t, h, http.MethodPost, fmt.Sprintf("/api/v1/chats/%d/transfer-ownership", chatID),
		owner.Access, map[string]int64{"user_id": successor.ID})
	expectStatus(t, transferred, http.StatusOK)

	sync := call(t, h, http.MethodGet, "/api/v1/realtime/events?since=0", successor.Access, nil)
	expectStatus(t, sync, http.StatusOK)
	events := asMap(t, sync)["events"].([]any)
	if len(events) != 3 || events[2].(map[string]any)["type"] != "community.ownership_transferred" {
		t.Fatalf("ownership transfer event missing: %s", sync.Body.String())
	}
	payload := events[2].(map[string]any)["payload"].(map[string]any)
	if int64(payload["previous_owner_id"].(float64)) != owner.ID ||
		int64(payload["new_owner_id"].(float64)) != successor.ID {
		t.Fatalf("ownership event payload mismatch: %v", payload)
	}
}
