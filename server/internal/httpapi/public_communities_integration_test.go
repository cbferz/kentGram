package httpapi

import (
	"fmt"
	"net/http"
	"testing"
)

func TestPublicCommunityJoin(t *testing.T) {
	api := newTestAPI(t)
	h := api.Routes()
	owner := registerUser(t, h)
	visitor := registerUser(t, h)
	handle := newName()

	created := call(t, h, http.MethodPost, "/api/v1/chats/community", owner.Access, map[string]any{
		"kind": "channel", "title": "Public updates", "is_public": true, "username": handle,
	})
	expectStatus(t, created, http.StatusCreated)
	chatID := int64(asMap(t, created)["id"].(float64))

	joined := call(t, h, http.MethodPost, "/api/v1/chats/join", visitor.Access, map[string]string{"username": "@" + handle})
	expectStatus(t, joined, http.StatusOK)
	if int64(asMap(t, joined)["chat_id"].(float64)) != chatID {
		t.Fatal("join returned a different chat ID")
	}

	joinedAgain := call(t, h, http.MethodPost, "/api/v1/chats/join", visitor.Access, map[string]string{"username": handle})
	expectStatus(t, joinedAgain, http.StatusOK)

	details := call(t, h, http.MethodGet, fmt.Sprintf("/api/v1/chats/%d", chatID), visitor.Access, nil)
	expectStatus(t, details, http.StatusOK)
	if asMap(t, details)["role"] != "member" {
		t.Fatalf("joined member role = %v, want member", asMap(t, details)["role"])
	}
}
