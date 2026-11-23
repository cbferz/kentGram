package httpapi

import (
	"fmt"
	"net/http"
	"testing"
)

func TestChatReadCursorIsScopedAndMonotonic(t *testing.T) {
	api := newTestAPI(t)
	h := api.Routes()
	owner := registerUser(t, h)
	peer := registerUser(t, h)
	outsider := registerUser(t, h)

	created := call(t, h, http.MethodPost, "/api/v1/chats/direct", owner.Access,
		map[string]int64{"user_id": peer.ID})
	expectStatus(t, created, http.StatusOK)
	chatID := int64(asMap(t, created)["chat_id"].(float64))

	first := call(t, h, http.MethodPost, fmt.Sprintf("/api/v1/chats/%d/messages", chatID), owner.Access,
		map[string]string{"text": "first"})
	expectStatus(t, first, http.StatusCreated)
	firstID := int64(asMap(t, first)["id"].(float64))
	second := call(t, h, http.MethodPost, fmt.Sprintf("/api/v1/chats/%d/messages", chatID), owner.Access,
		map[string]string{"text": "second"})
	expectStatus(t, second, http.StatusCreated)
	secondID := int64(asMap(t, second)["id"].(float64))

	// Fetching history must not mark it read; only an explicit acknowledgement does.
	loaded := call(t, h, http.MethodGet, fmt.Sprintf("/api/v1/chats/%d/messages", chatID), peer.Access, nil)
	expectStatus(t, loaded, http.StatusOK)
	beforeAck := call(t, h, http.MethodGet, fmt.Sprintf("/api/v1/chats/%d/read", chatID), owner.Access, nil)
	expectStatus(t, beforeAck, http.StatusOK)
	beforeCursors, ok := asMap(t, beforeAck)["read_cursors"].(map[string]any)
	if !ok {
		t.Fatal("read_cursors is not an object before acknowledgement")
	}
	beforeCursor, exists := beforeCursors[fmt.Sprint(peer.ID)]
	beforeCursorValue, numeric := beforeCursor.(float64)
	if !exists || !numeric || beforeCursorValue != 0 {
		t.Fatal("loading messages unexpectedly advanced the read cursor")
	}

	marked := call(t, h, http.MethodPost, fmt.Sprintf("/api/v1/chats/%d/read", chatID), peer.Access,
		map[string]int64{"last_read_message_id": secondID})
	expectStatus(t, marked, http.StatusOK)
	if got := int64(asMap(t, marked)["last_read_message_id"].(float64)); got != secondID {
		t.Fatalf("read cursor = %d, want %d", got, secondID)
	}

	backwards := call(t, h, http.MethodPost, fmt.Sprintf("/api/v1/chats/%d/read", chatID), peer.Access,
		map[string]int64{"last_read_message_id": firstID})
	expectStatus(t, backwards, http.StatusOK)
	if got := int64(asMap(t, backwards)["last_read_message_id"].(float64)); got != secondID {
		t.Fatalf("read cursor moved backwards: got %d, want %d", got, secondID)
	}

	cursors := call(t, h, http.MethodGet, fmt.Sprintf("/api/v1/chats/%d/read", chatID), owner.Access, nil)
	expectStatus(t, cursors, http.StatusOK)
	readCursors, ok := asMap(t, cursors)["read_cursors"].(map[string]any)
	if !ok {
		t.Fatal("read_cursors is not an object")
	}
	if _, ok := readCursors[fmt.Sprint(peer.ID)]; !ok {
		t.Fatal("peer read cursor missing")
	}

	outsiderRead := call(t, h, http.MethodGet, fmt.Sprintf("/api/v1/chats/%d/read", chatID), outsider.Access, nil)
	expectStatus(t, outsiderRead, http.StatusNotFound)
	outsiderMark := call(t, h, http.MethodPost, fmt.Sprintf("/api/v1/chats/%d/read", chatID), outsider.Access,
		map[string]int64{"last_read_message_id": secondID})
	expectStatus(t, outsiderMark, http.StatusNotFound)
}
