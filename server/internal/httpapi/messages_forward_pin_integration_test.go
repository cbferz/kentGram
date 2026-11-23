package httpapi

import (
	"fmt"
	"net/http"
	"testing"
)

func TestForwardMessageAndPinLifecycle(t *testing.T) {
	h := newTestAPI(t).Routes()
	alice := registerUser(t, h)
	bob := registerUser(t, h)
	carol := registerUser(t, h)

	source := call(t, h, http.MethodPost, "/api/v1/chats/direct", alice.Access, map[string]int64{"user_id": bob.ID})
	destination := call(t, h, http.MethodPost, "/api/v1/chats/direct", alice.Access, map[string]int64{"user_id": carol.ID})
	expectStatus(t, source, http.StatusOK)
	expectStatus(t, destination, http.StatusOK)
	sourceID := int64(asMap(t, source)["chat_id"].(float64))
	destinationID := int64(asMap(t, destination)["chat_id"].(float64))

	original := call(t, h, http.MethodPost, fmt.Sprintf("/api/v1/chats/%d/messages", sourceID),
		bob.Access, map[string]string{"text": "forward this message"})
	expectStatus(t, original, http.StatusCreated)
	messageID := int64(asMap(t, original)["id"].(float64))

	forwarded := call(t, h, http.MethodPost,
		fmt.Sprintf("/api/v1/chats/%d/messages/%d/forward", sourceID, messageID),
		alice.Access, map[string]int64{"destination_chat_id": destinationID})
	expectStatus(t, forwarded, http.StatusCreated)
	forwardMap := asMap(t, forwarded)
	forwardedID := int64(forwardMap["id"].(float64))
	if forwardMap["body"] != "forward this message" || forwardMap["forwarded_from_username"] != bob.Username {
		t.Fatalf("forward metadata/content mismatch: %#v", forwardMap)
	}
	if int64(forwardMap["forwarded_from_chat_id"].(float64)) != sourceID ||
		int64(forwardMap["forwarded_from_message_id"].(float64)) != messageID {
		t.Fatalf("forward source mismatch: %#v", forwardMap)
	}

	pin := call(t, h, http.MethodPost, fmt.Sprintf("/api/v1/chats/%d/pin", destinationID),
		alice.Access, map[string]int64{"message_id": forwardedID})
	expectStatus(t, pin, http.StatusOK)
	pinned := call(t, h, http.MethodGet, fmt.Sprintf("/api/v1/chats/%d/pin", destinationID), carol.Access, nil)
	expectStatus(t, pinned, http.StatusOK)
	pinnedMessage := asMap(t, pinned)["pinned_message"].(map[string]any)
	if int64(pinnedMessage["id"].(float64)) != forwardedID {
		t.Fatalf("wrong pinned message: %#v", pinnedMessage)
	}

	unpin := call(t, h, http.MethodDelete, fmt.Sprintf("/api/v1/chats/%d/pin", destinationID), carol.Access, nil)
	expectStatus(t, unpin, http.StatusOK)
	emptyPin := call(t, h, http.MethodGet, fmt.Sprintf("/api/v1/chats/%d/pin", destinationID), alice.Access, nil)
	expectStatus(t, emptyPin, http.StatusOK)
	if asMap(t, emptyPin)["pinned_message"] != nil {
		t.Fatalf("expected pin to be cleared: %#v", asMap(t, emptyPin))
	}
}

func TestForwardAndPinRequireMembershipAndCommunityRole(t *testing.T) {
	h := newTestAPI(t).Routes()
	owner := registerUser(t, h)
	member := registerUser(t, h)
	outside := registerUser(t, h)

	created := call(t, h, http.MethodPost, "/api/v1/chats/community", owner.Access,
		map[string]any{"kind": "group", "title": "Moderated group"})
	expectStatus(t, created, http.StatusCreated)
	chatID := int64(asMap(t, created)["id"].(float64))
	added := call(t, h, http.MethodPost, fmt.Sprintf("/api/v1/chats/%d/members", chatID),
		owner.Access, map[string]int64{"user_id": member.ID})
	expectStatus(t, added, http.StatusCreated)

	message := call(t, h, http.MethodPost, fmt.Sprintf("/api/v1/chats/%d/messages", chatID),
		member.Access, map[string]string{"text": "member message"})
	expectStatus(t, message, http.StatusCreated)
	messageID := int64(asMap(t, message)["id"].(float64))

	outsidePin := call(t, h, http.MethodPost, fmt.Sprintf("/api/v1/chats/%d/pin", chatID),
		outside.Access, map[string]int64{"message_id": messageID})
	expectStatus(t, outsidePin, http.StatusNotFound)

	memberPin := call(t, h, http.MethodPost, fmt.Sprintf("/api/v1/chats/%d/pin", chatID),
		member.Access, map[string]int64{"message_id": messageID})
	expectStatus(t, memberPin, http.StatusNotFound)

	ownerPin := call(t, h, http.MethodPost, fmt.Sprintf("/api/v1/chats/%d/pin", chatID),
		owner.Access, map[string]int64{"message_id": messageID})
	expectStatus(t, ownerPin, http.StatusOK)

	outsideForward := call(t, h, http.MethodPost,
		fmt.Sprintf("/api/v1/chats/%d/messages/%d/forward", chatID, messageID),
		outside.Access, map[string]int64{"destination_chat_id": chatID})
	expectStatus(t, outsideForward, http.StatusNotFound)
}
