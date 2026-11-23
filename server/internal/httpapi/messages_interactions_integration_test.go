package httpapi

import (
	"bufio"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newChat(t *testing.T, h http.Handler, a, b testUser) int64 {
	t.Helper()
	rec := call(t, h, http.MethodPost, "/api/v1/chats/direct", a.Access, map[string]int64{"user_id": b.ID})
	expectStatus(t, rec, http.StatusOK)
	return int64(asMap(t, rec)["chat_id"].(float64))
}

func chatBase(chatID int64) string { return fmt.Sprintf("/api/v1/chats/%d", chatID) }

func msgBase(chatID, msgID int64) string {
	return fmt.Sprintf("/api/v1/chats/%d/messages/%d", chatID, msgID)
}

func sendText(t *testing.T, h http.Handler, u testUser, chatID int64, text string) int64 {
	t.Helper()
	rec := call(t, h, http.MethodPost, chatBase(chatID)+"/messages", u.Access, map[string]string{"text": text})
	expectStatus(t, rec, http.StatusCreated)
	return int64(asMap(t, rec)["id"].(float64))
}

func history(t *testing.T, h http.Handler, u testUser, chatID int64) []map[string]any {
	t.Helper()
	rec := call(t, h, http.MethodGet, chatBase(chatID)+"/messages", u.Access, nil)
	expectStatus(t, rec, http.StatusOK)
	var out []map[string]any
	for _, m := range asMap(t, rec)["messages"].([]any) {
		out = append(out, m.(map[string]any))
	}
	return out
}

func findMsg(msgs []map[string]any, id int64) map[string]any {
	for _, m := range msgs {
		if int64(m["id"].(float64)) == id {
			return m
		}
	}
	return nil
}

func TestEditMessageOnlyBySenderAndVisibleToPeer(t *testing.T) {
	h := newTestAPI(t).Routes()
	alice, bob, carol := registerUser(t, h), registerUser(t, h), registerUser(t, h)
	chat := newChat(t, h, alice, bob)
	id := sendText(t, h, alice, chat, "original")

	expectStatus(t, call(t, h, http.MethodPatch, msgBase(chat, id), bob.Access, map[string]string{"text": "hijack"}), http.StatusNotFound)
	expectStatus(t, call(t, h, http.MethodPatch, msgBase(chat, id), carol.Access, map[string]string{"text": "outsider"}), http.StatusNotFound)
	expectStatus(t, call(t, h, http.MethodPatch, msgBase(chat, id), alice.Access, map[string]string{"text": "   "}), http.StatusBadRequest)

	rec := call(t, h, http.MethodPatch, msgBase(chat, id), alice.Access, map[string]string{"text": "changed"})
	expectStatus(t, rec, http.StatusOK)
	if asMap(t, rec)["edited_at"] == nil {
		t.Fatal("edited_at not set")
	}
	got := findMsg(history(t, h, bob, chat), id)
	if got == nil || got["body"] != "changed" || got["edited_at"] == nil {
		t.Fatalf("peer does not see the edit: %#v", got)
	}

	// An edit cannot be aimed at another chat's message via a different chat id.
	other := newChat(t, h, alice, carol)
	expectStatus(t, call(t, h, http.MethodPatch, msgBase(other, id), alice.Access, map[string]string{"text": "cross"}), http.StatusNotFound)

	// Deleted messages cannot be edited.
	expectStatus(t, call(t, h, http.MethodDelete, msgBase(chat, id), alice.Access, nil), http.StatusOK)
	expectStatus(t, call(t, h, http.MethodPatch, msgBase(chat, id), alice.Access, map[string]string{"text": "zombie"}), http.StatusNotFound)
}

func TestReactionsLimitsAndIsolation(t *testing.T) {
	h := newTestAPI(t).Routes()
	alice, bob, carol := registerUser(t, h), registerUser(t, h), registerUser(t, h)
	chat := newChat(t, h, alice, bob)
	id := sendText(t, h, alice, chat, "react to me")
	path := msgBase(chat, id) + "/reactions"

	expectStatus(t, call(t, h, http.MethodPost, path, carol.Access, map[string]string{"emoji": "👍"}), http.StatusNotFound)
	expectStatus(t, call(t, h, http.MethodPost, path, bob.Access, map[string]string{"emoji": "abc"}), http.StatusBadRequest)
	expectStatus(t, call(t, h, http.MethodPost, path, bob.Access, map[string]string{"emoji": ""}), http.StatusBadRequest)
	expectStatus(t, call(t, h, http.MethodPost, path, bob.Access, map[string]string{"emoji": "👍 👍"}), http.StatusBadRequest)

	// Adding twice is idempotent.
	for i := 0; i < 2; i++ {
		expectStatus(t, call(t, h, http.MethodPost, path, bob.Access, map[string]string{"emoji": "👍"}), http.StatusOK)
	}
	msg := findMsg(history(t, h, alice, chat), id)
	rs := msg["reactions"].([]any)
	if len(rs) != 1 || int(rs[0].(map[string]any)["count"].(float64)) != 1 || rs[0].(map[string]any)["mine"] != false {
		t.Fatalf("alice should see one reaction she did not make: %#v", rs)
	}
	bobView := findMsg(history(t, h, bob, chat), id)["reactions"].([]any)
	if bobView[0].(map[string]any)["mine"] != true {
		t.Fatalf("bob's own reaction must be flagged mine: %#v", bobView)
	}

	// Per-user cap is 3: the 4th evicts the oldest (FIFO).
	for _, e := range []string{"❤️", "😂", "🔥"} {
		expectStatus(t, call(t, h, http.MethodPost, path, bob.Access, map[string]string{"emoji": e}), http.StatusOK)
		time.Sleep(5 * time.Millisecond)
	}
	rec := call(t, h, http.MethodPost, path, bob.Access, map[string]string{"emoji": "🎉"})
	expectStatus(t, rec, http.StatusOK)
	mine := asMap(t, rec)["actor_emojis"].([]any)
	if len(mine) != 3 {
		t.Fatalf("user must hold exactly 3 reactions, has %v", mine)
	}
	for _, e := range mine {
		if e == "👍" {
			t.Fatalf("oldest reaction was not evicted: %v", mine)
		}
	}

	// Removing works and is scoped to the caller.
	expectStatus(t, call(t, h, http.MethodDelete, path+"?emoji=%F0%9F%8E%89", bob.Access, nil), http.StatusOK)
	expectStatus(t, call(t, h, http.MethodDelete, path+"?emoji=%F0%9F%8E%89", carol.Access, nil), http.StatusNotFound)

	// A reaction cannot be attached through a different chat's path.
	other := newChat(t, h, alice, carol)
	expectStatus(t, call(t, h, http.MethodPost, msgBase(other, id)+"/reactions", alice.Access, map[string]string{"emoji": "👍"}), http.StatusNotFound)
}

// A direct chat has two members, so at most 3+3 = 6 distinct reactions can be
// placed through the API. The 10-types-per-message ceiling only becomes
// reachable once group chats exist; it is covered at the SQL level for now.
func TestTwoMembersCanHoldSixDistinctReactions(t *testing.T) {
	h := newTestAPI(t).Routes()
	alice, bob := registerUser(t, h), registerUser(t, h)
	chat := newChat(t, h, alice, bob)
	id := sendText(t, h, alice, chat, "popular")
	path := msgBase(chat, id) + "/reactions"
	for _, e := range []string{"👍", "👎", "❤️"} {
		expectStatus(t, call(t, h, http.MethodPost, path, alice.Access, map[string]string{"emoji": e}), http.StatusOK)
	}
	for _, e := range []string{"😂", "😮", "😢"} {
		expectStatus(t, call(t, h, http.MethodPost, path, bob.Access, map[string]string{"emoji": e}), http.StatusOK)
	}
	got := findMsg(history(t, h, alice, chat), id)["reactions"].([]any)
	if len(got) != 6 {
		t.Fatalf("expected 6 distinct reactions, got %d", len(got))
	}
}

func TestIdempotentSendWithClientID(t *testing.T) {
	h := newTestAPI(t).Routes()
	alice, bob := registerUser(t, h), registerUser(t, h)
	chat := newChat(t, h, alice, bob)
	body := map[string]string{"text": "once", "client_id": "retry-1"}

	first := call(t, h, http.MethodPost, chatBase(chat)+"/messages", alice.Access, body)
	expectStatus(t, first, http.StatusCreated)
	second := call(t, h, http.MethodPost, chatBase(chat)+"/messages", alice.Access, body)
	expectStatus(t, second, http.StatusOK)
	if asMap(t, first)["id"] != asMap(t, second)["id"] {
		t.Fatal("replayed send created a different message")
	}
	if n := len(history(t, h, alice, chat)); n != 1 {
		t.Fatalf("expected exactly 1 message after replay, got %d", n)
	}

	// The same key cannot be reused to write into a different chat.
	carol := registerUser(t, h)
	other := newChat(t, h, alice, carol)
	expectStatus(t, call(t, h, http.MethodPost, chatBase(other)+"/messages", alice.Access, body), http.StatusConflict)

	// Another user may use the same key independently.
	expectStatus(t, call(t, h, http.MethodPost, chatBase(chat)+"/messages", bob.Access, body), http.StatusCreated)

	expectStatus(t, call(t, h, http.MethodPost, chatBase(chat)+"/messages", alice.Access,
		map[string]string{"text": "x", "client_id": "bad id!"}), http.StatusBadRequest)
}

func TestForwardRequiresMembershipOfBothChats(t *testing.T) {
	h := newTestAPI(t).Routes()
	alice, bob, carol := registerUser(t, h), registerUser(t, h), registerUser(t, h)
	ab := newChat(t, h, alice, bob)
	ac := newChat(t, h, alice, carol)
	bc := newChat(t, h, bob, carol)
	id := sendText(t, h, bob, ab, "bob's words")
	fwd := msgBase(ab, id) + "/forward"

	// Alice is not in bob/carol's chat; carol is not in the source chat.
	expectStatus(t, call(t, h, http.MethodPost, fwd, alice.Access, map[string]any{"to_chat_id": bc}), http.StatusNotFound)
	expectStatus(t, call(t, h, http.MethodPost, fwd, carol.Access, map[string]any{"to_chat_id": ac}), http.StatusNotFound)

	rec := call(t, h, http.MethodPost, fwd, alice.Access, map[string]any{"to_chat_id": ac, "client_id": "fw-1"})
	expectStatus(t, rec, http.StatusCreated)
	m := asMap(t, rec)
	if m["forwarded_from_username"] != bob.Username || m["sender_username"] != alice.Username {
		t.Fatalf("forward must credit the original author: %#v", m)
	}
	replay := call(t, h, http.MethodPost, fwd, alice.Access, map[string]any{"to_chat_id": ac, "client_id": "fw-1"})
	expectStatus(t, replay, http.StatusOK)
	if n := len(history(t, h, carol, ac)); n != 1 {
		t.Fatalf("expected 1 forwarded copy after replay, got %d", n)
	}

	// Forwarding a deleted message is refused.
	expectStatus(t, call(t, h, http.MethodDelete, msgBase(ab, id), bob.Access, nil), http.StatusOK)
	expectStatus(t, call(t, h, http.MethodPost, fwd, alice.Access, map[string]any{"to_chat_id": ac}), http.StatusNotFound)
}

func TestPinsAreMemberScopedAndClearedOnDelete(t *testing.T) {
	h := newTestAPI(t).Routes()
	alice, bob, carol := registerUser(t, h), registerUser(t, h), registerUser(t, h)
	chat := newChat(t, h, alice, bob)
	id := sendText(t, h, alice, chat, "pin me")
	pin := chatBase(chat) + fmt.Sprintf("/pins/%d", id)

	expectStatus(t, call(t, h, http.MethodPut, pin, carol.Access, nil), http.StatusNotFound)
	expectStatus(t, call(t, h, http.MethodGet, chatBase(chat)+"/pins", carol.Access, nil), http.StatusNotFound)
	expectStatus(t, call(t, h, http.MethodPut, pin, bob.Access, nil), http.StatusOK)
	expectStatus(t, call(t, h, http.MethodPut, pin, bob.Access, nil), http.StatusOK) // idempotent

	pins := asMap(t, call(t, h, http.MethodGet, chatBase(chat)+"/pins", alice.Access, nil))["messages"].([]any)
	if len(pins) != 1 {
		t.Fatalf("expected 1 pin, got %d", len(pins))
	}
	if findMsg(history(t, h, alice, chat), id)["pinned"] != true {
		t.Fatal("history must report the message as pinned")
	}

	expectStatus(t, call(t, h, http.MethodDelete, pin, carol.Access, nil), http.StatusNotFound)
	if len(asMap(t, call(t, h, http.MethodGet, chatBase(chat)+"/pins", alice.Access, nil))["messages"].([]any)) != 1 {
		t.Fatal("an outsider's unpin must not remove the pin")
	}

	expectStatus(t, call(t, h, http.MethodDelete, msgBase(chat, id), alice.Access, nil), http.StatusOK)
	if len(asMap(t, call(t, h, http.MethodGet, chatBase(chat)+"/pins", alice.Access, nil))["messages"].([]any)) != 0 {
		t.Fatal("deleting a message must unpin it")
	}
}

func TestReadMarkersClampAndPeerSeesThem(t *testing.T) {
	h := newTestAPI(t).Routes()
	alice, bob, carol := registerUser(t, h), registerUser(t, h), registerUser(t, h)
	chat := newChat(t, h, alice, bob)
	first := sendText(t, h, alice, chat, "one")
	sendText(t, h, alice, chat, "two")

	expectStatus(t, call(t, h, http.MethodPost, chatBase(chat)+"/read", carol.Access, map[string]int64{"message_id": 1}), http.StatusNotFound)

	rec := call(t, h, http.MethodPost, chatBase(chat)+"/read", bob.Access, map[string]int64{"message_id": first})
	expectStatus(t, rec, http.StatusOK)
	if int64(asMap(t, rec)["last_read_message_id"].(float64)) != first {
		t.Fatalf("marker should be %d: %s", first, rec.Body.String())
	}
	// Never backwards, never beyond the chat's newest message.
	rec = call(t, h, http.MethodPost, chatBase(chat)+"/read", bob.Access, map[string]int64{"message_id": 1})
	if int64(asMap(t, rec)["last_read_message_id"].(float64)) != first {
		t.Fatal("marker moved backwards")
	}
	rec = call(t, h, http.MethodPost, chatBase(chat)+"/read", bob.Access, map[string]int64{"message_id": 1 << 50})
	expectStatus(t, rec, http.StatusOK)
	if got := int64(asMap(t, rec)["last_read_message_id"].(float64)); got >= 1<<50 {
		t.Fatalf("marker was not clamped to the chat's newest message: %d", got)
	}

	list := call(t, h, http.MethodGet, chatBase(chat)+"/messages", alice.Access, nil)
	expectStatus(t, list, http.StatusOK)
	if asMap(t, list)["peer_last_read_message_id"].(float64) < float64(first) {
		t.Fatal("alice should see that bob has read her messages")
	}
}

func TestTypingAndPresenceAuthorization(t *testing.T) {
	h := newTestAPI(t).Routes()
	alice, bob, carol := registerUser(t, h), registerUser(t, h), registerUser(t, h)
	chat := newChat(t, h, alice, bob)
	expectStatus(t, call(t, h, http.MethodPost, chatBase(chat)+"/typing", alice.Access, nil), http.StatusOK)
	expectStatus(t, call(t, h, http.MethodPost, chatBase(chat)+"/typing", carol.Access, nil), http.StatusNotFound)
	expectStatus(t, call(t, h, http.MethodGet, "/api/v1/presence", alice.Access, nil), http.StatusOK)
	expectStatus(t, call(t, h, http.MethodGet, "/api/v1/presence", "", nil), http.StatusUnauthorized)
}

// ---- WebSocket end to end ---------------------------------------------------

type wsConn struct {
	t  *testing.T
	c  net.Conn
	br *bufio.Reader
}

func wsDial(t *testing.T, srv *httptest.Server, token string) *wsConn {
	t.Helper()
	host := strings.TrimPrefix(srv.URL, "http://")
	conn, err := net.Dial("tcp", host)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	k := make([]byte, 16)
	_, _ = rand.Read(k)
	req := "GET /api/v1/ws HTTP/1.1\r\nHost: " + host + "\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n" +
		"Sec-WebSocket-Version: 13\r\nSec-WebSocket-Key: " + base64.StdEncoding.EncodeToString(k) + "\r\n\r\n"
	if _, err := conn.Write([]byte(req)); err != nil {
		t.Fatal(err)
	}
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, nil)
	if err != nil || resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("websocket upgrade failed: %v %v", resp, err)
	}
	w := &wsConn{t: t, c: conn, br: br}
	auth, _ := json.Marshal(map[string]string{"type": "auth", "token": token})
	w.write(auth)
	if ev := w.next(3 * time.Second); ev["type"] != "ready" {
		t.Fatalf("expected ready, got %#v", ev)
	}
	return w
}

// wsExpectRejected upgrades, sends the given token as the auth frame and
// requires the server to close the socket without ever sending "ready".
func wsExpectRejected(t *testing.T, srv *httptest.Server, token string) {
	t.Helper()
	host := strings.TrimPrefix(srv.URL, "http://")
	conn, err := net.Dial("tcp", host)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	k := make([]byte, 16)
	_, _ = rand.Read(k)
	req := "GET /api/v1/ws HTTP/1.1\r\nHost: " + host + "\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n" +
		"Sec-WebSocket-Version: 13\r\nSec-WebSocket-Key: " + base64.StdEncoding.EncodeToString(k) + "\r\n\r\n"
	_, _ = conn.Write([]byte(req))
	br := bufio.NewReader(conn)
	if resp, err := http.ReadResponse(br, nil); err != nil || resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("upgrade failed: %v %v", resp, err)
	}
	w := &wsConn{t: t, c: conn, br: br}
	auth, _ := json.Marshal(map[string]string{"type": "auth", "token": token})
	w.write(auth)
	if ev := w.next(3 * time.Second); ev != nil {
		t.Fatalf("a bad token must not receive any event, got %#v", ev)
	}
}

func (w *wsConn) write(payload []byte) {
	key := []byte{9, 8, 7, 6}
	hdr := []byte{0x81}
	n := len(payload)
	if n < 126 {
		hdr = append(hdr, 0x80|byte(n))
	} else {
		hdr = append(hdr, 0x80|126, byte(n>>8), byte(n))
	}
	hdr = append(hdr, key...)
	body := make([]byte, n)
	for i := range payload {
		body[i] = payload[i] ^ key[i&3]
	}
	if _, err := w.c.Write(append(hdr, body...)); err != nil {
		w.t.Fatal(err)
	}
}

// next returns the next JSON event, skipping control frames; nil on timeout/close.
func (w *wsConn) next(timeout time.Duration) map[string]any {
	deadline := time.Now().Add(timeout)
	for {
		_ = w.c.SetReadDeadline(deadline)
		var h [2]byte
		if _, err := io.ReadFull(w.br, h[:]); err != nil {
			return nil
		}
		n := int(h[1] & 0x7F)
		if n == 126 {
			var b [2]byte
			_, _ = io.ReadFull(w.br, b[:])
			n = int(binary.BigEndian.Uint16(b[:]))
		} else if n == 127 {
			var b [8]byte
			_, _ = io.ReadFull(w.br, b[:])
			n = int(binary.BigEndian.Uint64(b[:]))
		}
		p := make([]byte, n)
		if _, err := io.ReadFull(w.br, p); err != nil {
			return nil
		}
		switch h[0] & 0x0F {
		case 0x1:
			var m map[string]any
			if json.Unmarshal(p, &m) != nil {
				return nil
			}
			return m
		case 0x8:
			return nil
		}
	}
}

func TestRealtimeEventsReachOnlyChatMembersAndSessionsEndSockets(t *testing.T) {
	api := newTestAPI(t)
	t.Cleanup(api.Close)
	srv := httptest.NewServer(api.Routes())
	t.Cleanup(srv.Close)
	h := api.Routes()

	alice, bob, carol := registerUser(t, h), registerUser(t, h), registerUser(t, h)
	chat := newChat(t, h, alice, bob)

	bobWS := wsDial(t, srv, bob.Access)
	carolWS := wsDial(t, srv, carol.Access)

	// A bad token is rejected and never receives events.
	wsExpectRejected(t, srv, "not-a-real-token")

	id := sendText(t, h, alice, chat, "live!")
	ev := bobWS.next(3 * time.Second)
	if ev == nil || ev["type"] != "message.new" || int64(ev["chat_id"].(float64)) != chat {
		t.Fatalf("bob should receive message.new, got %#v", ev)
	}
	data := ev["data"].(map[string]any)
	if int64(data["id"].(float64)) != id || data["is_mine"] != false {
		t.Fatalf("broadcast payload must be viewer-neutral: %#v", data)
	}
	if got := carolWS.next(400 * time.Millisecond); got != nil && got["type"] == "message.new" {
		t.Fatalf("carol is not in the chat and must not receive its messages: %#v", got)
	}

	expectStatus(t, call(t, h, http.MethodPost, msgBase(chat, id)+"/reactions", bob.Access, map[string]string{"emoji": "👍"}), http.StatusOK)
	if ev := bobWS.next(3 * time.Second); ev == nil || ev["type"] != "reaction.updated" {
		t.Fatalf("expected reaction.updated, got %#v", ev)
	}

	expectStatus(t, call(t, h, http.MethodPost, chatBase(chat)+"/typing", alice.Access, nil), http.StatusOK)
	if ev := bobWS.next(3 * time.Second); ev == nil || ev["type"] != "typing" {
		t.Fatalf("expected typing, got %#v", ev)
	}

	// Logging out ends that session's socket immediately.
	expectStatus(t, call(t, h, http.MethodPost, "/api/v1/auth/logout", bob.Access, nil), http.StatusOK)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if bobWS.next(300*time.Millisecond) == nil {
			return
		}
	}
	t.Fatal("socket stayed open after the session was revoked")
}
