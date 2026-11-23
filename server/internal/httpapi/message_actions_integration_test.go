package httpapi

import (
	"fmt"
	"net/http"
	"testing"
)

func TestEditMessageAndToggleReaction(t *testing.T) {
	h := newTestAPI(t).Routes()
	alice := registerUser(t, h)
	bob := registerUser(t, h)
	outsider := registerUser(t, h)
	created := call(t, h, http.MethodPost, "/api/v1/chats/direct", alice.Access, map[string]int64{"user_id": bob.ID})
	expectStatus(t, created, http.StatusOK)
	chatID := int64(asMap(t, created)["chat_id"].(float64))

	sent := call(t, h, http.MethodPost, fmt.Sprintf("/api/v1/chats/%d/messages", chatID), alice.Access, map[string]string{"text": "original"})
	expectStatus(t, sent, http.StatusCreated)
	messageID := int64(asMap(t, sent)["id"].(float64))
	base := fmt.Sprintf("/api/v1/chats/%d/messages/%d", chatID, messageID)

	edited := call(t, h, http.MethodPatch, base, alice.Access, map[string]string{"text": "updated"})
	expectStatus(t, edited, http.StatusOK)
	if asMap(t, edited)["body"] != "updated" || asMap(t, edited)["edited_at"] == nil {
		t.Fatalf("edited message response is incomplete: %s", edited.Body.String())
	}
	putEdited := call(t, h, http.MethodPut, base, alice.Access, map[string]string{"text": "updated via put"})
	expectStatus(t, putEdited, http.StatusOK)
	if asMap(t, putEdited)["body"] != "updated via put" {
		t.Fatalf("PUT edit alias did not update body: %s", putEdited.Body.String())
	}
	forbiddenEdit := call(t, h, http.MethodPatch, base, bob.Access, map[string]string{"text": "hijack"})
	expectStatus(t, forbiddenEdit, http.StatusNotFound)

	reactionURL := base + "/reactions"
	first := call(t, h, http.MethodPost, reactionURL, bob.Access, map[string]string{"emoji": "👍"})
	expectStatus(t, first, http.StatusOK)
	reactions := asMap(t, first)["reactions"].([]any)
	if len(reactions) != 1 || reactions[0].(map[string]any)["count"] != float64(1) {
		t.Fatalf("reaction not added: %s", first.Body.String())
	}
	second := call(t, h, http.MethodPost, reactionURL, bob.Access, map[string]string{"emoji": "👍"})
	expectStatus(t, second, http.StatusOK)
	if len(asMap(t, second)["reactions"].([]any)) != 0 {
		t.Fatalf("reaction toggle did not remove reaction: %s", second.Body.String())
	}
	for _, emoji := range []string{"👍", "❤️", "😂"} {
		added := call(t, h, http.MethodPost, reactionURL, bob.Access, map[string]string{"emoji": emoji})
		expectStatus(t, added, http.StatusOK)
	}
	limited := call(t, h, http.MethodPost, reactionURL, bob.Access, map[string]string{"emoji": "🔥"})
	expectStatus(t, limited, http.StatusConflict)
	if asMap(t, limited)["error"] != "reaction_limit_reached" {
		t.Fatalf("unexpected reaction limit response: %s", limited.Body.String())
	}
	outside := call(t, h, http.MethodPost, reactionURL, outsider.Access, map[string]string{"emoji": "🔥"})
	expectStatus(t, outside, http.StatusNotFound)
}
