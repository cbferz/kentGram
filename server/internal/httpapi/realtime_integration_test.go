package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func dialRealtime(t *testing.T, serverURL, token string) *websocket.Conn {
	t.Helper()
	header := http.Header{}
	header.Set("Authorization", "Bearer "+token)
	url := "ws" + strings.TrimPrefix(serverURL, "http") + "/api/v1/realtime"
	conn, response, err := websocket.DefaultDialer.Dial(url, header)
	if err != nil {
		if response != nil {
			t.Fatalf("websocket dial failed: %v (HTTP %d)", err, response.StatusCode)
		}
		t.Fatalf("websocket dial failed: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func TestRealtimeMessageAndTypingEvents(t *testing.T) {
	api := newTestAPI(t)
	handler := api.Routes()
	owner := registerUser(t, handler)
	peer := registerUser(t, handler)

	created := call(t, handler, http.MethodPost, "/api/v1/chats/direct", owner.Access,
		map[string]int64{"user_id": peer.ID})
	expectStatus(t, created, http.StatusOK)
	chatID := int64(asMap(t, created)["chat_id"].(float64))

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	ownerSocket := dialRealtime(t, server.URL, owner.Access)
	peerSocket := dialRealtime(t, server.URL, peer.Access)

	sent := call(t, handler, http.MethodPost, fmt.Sprintf("/api/v1/chats/%d/messages", chatID), owner.Access,
		map[string]string{"text": "realtime delivery"})
	expectStatus(t, sent, http.StatusCreated)
	messageID := int64(asMap(t, sent)["id"].(float64))

	_ = peerSocket.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, payload, err := peerSocket.ReadMessage()
	if err != nil {
		t.Fatalf("read message event: %v", err)
	}
	var messageEvent realtimeEvent
	if err := json.Unmarshal(payload, &messageEvent); err != nil {
		t.Fatalf("decode message event: %v", err)
	}
	if messageEvent.Type != "message.created" || messageEvent.ChatID != chatID || messageEvent.MessageID != messageID {
		t.Fatalf("unexpected message event: %+v", messageEvent)
	}

	if err := peerSocket.WriteJSON(map[string]any{"type": "typing", "chat_id": chatID}); err != nil {
		t.Fatalf("send typing event: %v", err)
	}
	_ = ownerSocket.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, payload, err = ownerSocket.ReadMessage()
	if err != nil {
		t.Fatalf("read typing event: %v", err)
	}
	var typingEvent realtimeEvent
	if err := json.Unmarshal(payload, &typingEvent); err != nil {
		t.Fatalf("decode typing event: %v", err)
	}
	if typingEvent.Type != "typing" || typingEvent.ChatID != chatID || typingEvent.UserID != peer.ID {
		t.Fatalf("unexpected typing event: %+v", typingEvent)
	}
}
