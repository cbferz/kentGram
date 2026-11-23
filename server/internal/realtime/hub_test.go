package realtime

import (
	"bufio"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// testClient is a tiny hand-rolled WebSocket client so the tests exercise the
// real wire format rather than the hub's own helpers.
type testClient struct {
	t    *testing.T
	conn net.Conn
	br   *bufio.Reader
}

func dial(t *testing.T, srv *httptest.Server, origin string) (*testClient, *http.Response) {
	t.Helper()
	conn, err := net.Dial("tcp", strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	keyRaw := make([]byte, 16)
	_, _ = rand.Read(keyRaw)
	key := base64.StdEncoding.EncodeToString(keyRaw)
	req := "GET /ws HTTP/1.1\r\nHost: " + strings.TrimPrefix(srv.URL, "http://") + "\r\n" +
		"Upgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Version: 13\r\n" +
		"Sec-WebSocket-Key: " + key + "\r\n"
	if origin != "" {
		req += "Origin: " + origin + "\r\n"
	}
	req += "\r\n"
	if _, err := conn.Write([]byte(req)); err != nil {
		t.Fatal(err)
	}
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode == http.StatusSwitchingProtocols {
		if got := resp.Header.Get("Sec-WebSocket-Accept"); got != acceptKey(key) {
			t.Fatalf("bad accept key %q", got)
		}
	}
	t.Cleanup(func() { conn.Close() })
	return &testClient{t: t, conn: conn, br: br}, resp
}

func (c *testClient) send(op byte, payload []byte, masked bool) {
	c.t.Helper()
	hdr := []byte{0x80 | op}
	n := len(payload)
	m := byte(0)
	if masked {
		m = 0x80
	}
	switch {
	case n < 126:
		hdr = append(hdr, m|byte(n))
	case n <= 0xFFFF:
		hdr = append(hdr, m|126, byte(n>>8), byte(n))
	default:
		var l [8]byte
		binary.BigEndian.PutUint64(l[:], uint64(n))
		hdr = append(hdr, m|127)
		hdr = append(hdr, l[:]...)
	}
	body := payload
	if masked {
		key := []byte{0x11, 0x22, 0x33, 0x44}
		hdr = append(hdr, key...)
		body = make([]byte, n)
		for i := range payload {
			body[i] = payload[i] ^ key[i&3]
		}
	}
	if _, err := c.conn.Write(append(hdr, body...)); err != nil {
		c.t.Fatal(err)
	}
}

// read returns the next server frame (opcode, payload).
func (c *testClient) read(timeout time.Duration) (byte, []byte, error) {
	_ = c.conn.SetReadDeadline(time.Now().Add(timeout))
	var h [2]byte
	if _, err := io.ReadFull(c.br, h[:]); err != nil {
		return 0, nil, err
	}
	n := int(h[1] & 0x7F)
	switch n {
	case 126:
		var b [2]byte
		io.ReadFull(c.br, b[:])
		n = int(binary.BigEndian.Uint16(b[:]))
	case 127:
		var b [8]byte
		io.ReadFull(c.br, b[:])
		n = int(binary.BigEndian.Uint64(b[:]))
	}
	p := make([]byte, n)
	if _, err := io.ReadFull(c.br, p); err != nil {
		return 0, nil, err
	}
	return h[0] & 0x0F, p, nil
}

func (c *testClient) auth(token string) {
	c.t.Helper()
	b, _ := json.Marshal(map[string]string{"type": "auth", "token": token})
	c.send(opText, b, true)
}

// nextEvent skips pings and returns the next JSON event.
func (c *testClient) nextEvent(timeout time.Duration) (Event, error) {
	deadline := time.Now().Add(timeout)
	for {
		op, p, err := c.read(time.Until(deadline))
		if err != nil {
			return Event{}, err
		}
		if op == opPing || op == opPong {
			continue
		}
		if op == opClose {
			return Event{}, io.EOF
		}
		var raw struct {
			Type   string          `json:"type"`
			ChatID int64           `json:"chat_id"`
			Data   json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(p, &raw); err != nil {
			return Event{}, err
		}
		return Event{Type: raw.Type, ChatID: raw.ChatID, Data: string(raw.Data)}, nil
	}
}

func (c *testClient) expectClosed(timeout time.Duration) (code int) {
	c.t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		op, p, err := c.read(time.Until(deadline))
		if err != nil {
			return -1 // connection dropped without a close frame
		}
		if op == opClose {
			if len(p) >= 2 {
				return int(binary.BigEndian.Uint16(p))
			}
			return 0
		}
	}
}

func newTestHub(t *testing.T, cfg Config) (*Hub, *httptest.Server) {
	t.Helper()
	if cfg.Auth == nil {
		cfg.Auth = func(tok string) (Identity, bool) {
			switch tok {
			case "u1":
				return Identity{UserID: 1, SessionID: 101}, true
			case "u1b":
				return Identity{UserID: 1, SessionID: 102}, true
			case "u2":
				return Identity{UserID: 2, SessionID: 201}, true
			}
			return Identity{}, false
		}
	}
	h := NewHub(cfg)
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", h.ServeWS)
	srv := httptest.NewServer(mux)
	t.Cleanup(func() { h.Close(); srv.Close() })
	return h, srv
}

func connect(t *testing.T, srv *httptest.Server, token string) *testClient {
	t.Helper()
	c, resp := dial(t, srv, "")
	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("status %d", resp.StatusCode)
	}
	c.auth(token)
	ev, err := c.nextEvent(2 * time.Second)
	if err != nil || ev.Type != "ready" {
		t.Fatalf("expected ready, got %+v err=%v", ev, err)
	}
	return c
}

func TestAcceptKeyMatchesRFC6455Example(t *testing.T) {
	if got := acceptKey("dGhlIHNhbXBsZSBub25jZQ=="); got != "s3pPLMBiTxaQ9kYGzzhZRbK+xOo=" {
		t.Fatalf("got %s", got)
	}
}

func TestPublishReachesOnlyTargetUsers(t *testing.T) {
	h, srv := newTestHub(t, Config{})
	c1 := connect(t, srv, "u1")
	c2 := connect(t, srv, "u2")

	h.Publish([]int64{1}, Event{Type: "message.new", ChatID: 7, Data: map[string]string{"body": "hi"}})

	ev, err := c1.nextEvent(2 * time.Second)
	if err != nil || ev.Type != "message.new" || ev.ChatID != 7 {
		t.Fatalf("user 1 got %+v err=%v", ev, err)
	}
	if ev, err := c2.nextEvent(300 * time.Millisecond); err == nil {
		t.Fatalf("user 2 must not receive user 1's event, got %+v", ev)
	}
}

func TestPublishReachesAllDevicesOfUser(t *testing.T) {
	h, srv := newTestHub(t, Config{})
	a := connect(t, srv, "u1")
	b := connect(t, srv, "u1b")
	h.Publish([]int64{1}, Event{Type: "x"})
	for _, c := range []*testClient{a, b} {
		if ev, err := c.nextEvent(2 * time.Second); err != nil || ev.Type != "x" {
			t.Fatalf("device missed event: %+v %v", ev, err)
		}
	}
}

func TestBadTokenIsRejectedAndNeverRegistered(t *testing.T) {
	h, srv := newTestHub(t, Config{})
	c, _ := dial(t, srv, "")
	c.auth("nope")
	if code := c.expectClosed(2 * time.Second); code != closePolicy && code != -1 {
		t.Fatalf("close code %d", code)
	}
	if h.Online(1) || h.Online(2) {
		t.Fatal("rejected client became online")
	}
}

func TestAuthTimeout(t *testing.T) {
	_, srv := newTestHub(t, Config{AuthTimeout: 150 * time.Millisecond})
	c, _ := dial(t, srv, "")
	start := time.Now()
	code := c.expectClosed(2 * time.Second)
	if code != closePolicy && code != -1 {
		t.Fatalf("close code %d", code)
	}
	if time.Since(start) > time.Second {
		t.Fatal("silent client was kept open past the auth timeout")
	}
}

func TestEventsBeforeAuthAreNotDelivered(t *testing.T) {
	h, srv := newTestHub(t, Config{AuthTimeout: 300 * time.Millisecond})
	c, _ := dial(t, srv, "")
	h.Publish([]int64{1, 2}, Event{Type: "secret"})
	if ev, err := c.nextEvent(200 * time.Millisecond); err == nil && ev.Type == "secret" {
		t.Fatal("unauthenticated socket received an event")
	}
}

func TestUnmaskedClientFrameIsProtocolError(t *testing.T) {
	_, srv := newTestHub(t, Config{})
	c := connect(t, srv, "u1")
	c.send(opText, []byte(`{}`), false)
	if code := c.expectClosed(2 * time.Second); code != closeProtocol && code != -1 {
		t.Fatalf("close code %d", code)
	}
}

func TestOversizedFrameIsRejected(t *testing.T) {
	_, srv := newTestHub(t, Config{MaxInboundMsg: 64})
	c := connect(t, srv, "u1")
	c.send(opText, []byte(strings.Repeat("a", 500)), true)
	if code := c.expectClosed(2 * time.Second); code != closeTooBig && code != -1 {
		t.Fatalf("close code %d", code)
	}
}

func TestPingIsAnsweredWithPong(t *testing.T) {
	_, srv := newTestHub(t, Config{})
	c := connect(t, srv, "u1")
	c.send(opPing, []byte("hello"), true)
	for i := 0; i < 5; i++ {
		op, p, err := c.read(2 * time.Second)
		if err != nil {
			t.Fatal(err)
		}
		if op == opPong {
			if string(p) != "hello" {
				t.Fatalf("pong payload %q", p)
			}
			return
		}
	}
	t.Fatal("no pong")
}

func TestServerSendsHeartbeatPings(t *testing.T) {
	_, srv := newTestHub(t, Config{PingInterval: 80 * time.Millisecond})
	c := connect(t, srv, "u1")
	op, _, err := c.read(2 * time.Second)
	if err != nil || op != opPing {
		t.Fatalf("expected ping, got op=%d err=%v", op, err)
	}
}

func TestSilentClientIsDroppedAfterPongTimeout(t *testing.T) {
	h, srv := newTestHub(t, Config{PingInterval: time.Hour, PongTimeout: 200 * time.Millisecond})
	c := connect(t, srv, "u1")
	_ = c
	deadline := time.Now().Add(2 * time.Second)
	for h.Online(1) && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if h.Online(1) {
		t.Fatal("dead connection was never reaped")
	}
}

func TestCloseSessionDisconnectsOnlyThatSession(t *testing.T) {
	h, srv := newTestHub(t, Config{})
	a := connect(t, srv, "u1")  // session 101
	b := connect(t, srv, "u1b") // session 102
	h.CloseSession(101)
	if code := a.expectClosed(2 * time.Second); code != closePolicy && code != -1 {
		t.Fatalf("revoked session close code %d", code)
	}
	h.Publish([]int64{1}, Event{Type: "still-here"})
	if ev, err := b.nextEvent(2 * time.Second); err != nil || ev.Type != "still-here" {
		t.Fatalf("other session was affected: %+v %v", ev, err)
	}
}

func TestPerUserConnectionCapEvictsOldest(t *testing.T) {
	h, srv := newTestHub(t, Config{MaxPerUser: 2})
	connect(t, srv, "u1")
	connect(t, srv, "u1")
	connect(t, srv, "u1")
	time.Sleep(100 * time.Millisecond)
	h.mu.RLock()
	n := len(h.byUser[1])
	h.mu.RUnlock()
	if n != 2 {
		t.Fatalf("expected 2 connections after cap, got %d", n)
	}
}

func TestSlowConsumerIsDroppedWithoutBlockingPublisher(t *testing.T) {
	h, srv := newTestHub(t, Config{SendQueue: 2, WriteTimeout: 200 * time.Millisecond})
	c := connect(t, srv, "u1")
	_ = c // never reads: its TCP buffers and queue fill up
	big := strings.Repeat("x", 64<<10)
	done := make(chan struct{})
	go func() {
		for i := 0; i < 2000; i++ {
			h.Publish([]int64{1}, Event{Type: "flood", Data: big})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Publish blocked on a slow consumer")
	}
	deadline := time.Now().Add(3 * time.Second)
	for h.Online(1) && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if h.Online(1) {
		t.Fatal("slow consumer was never dropped")
	}
}

func TestOriginPolicy(t *testing.T) {
	_, srv := newTestHub(t, Config{})
	host := strings.TrimPrefix(srv.URL, "http://")

	if _, r := dial(t, srv, "http://evil.example"); r.StatusCode != http.StatusForbidden {
		t.Fatalf("foreign origin got %d", r.StatusCode)
	}
	if _, r := dial(t, srv, "http://"+host); r.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("same-host origin got %d", r.StatusCode)
	}
	if _, r := dial(t, srv, ""); r.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("native client (no Origin) got %d", r.StatusCode)
	}

	_, srv2 := newTestHub(t, Config{AllowOrigin: func(o string, _ *http.Request) bool { return o == "https://web.kentgram.test" }})
	if _, r := dial(t, srv2, "https://web.kentgram.test"); r.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("allow-listed origin got %d", r.StatusCode)
	}
	if _, r := dial(t, srv2, "https://other.test"); r.StatusCode != http.StatusForbidden {
		t.Fatalf("non-listed origin got %d", r.StatusCode)
	}
}

func TestNonUpgradeRequestGets426(t *testing.T) {
	_, srv := newTestHub(t, Config{})
	resp, err := http.Get(srv.URL + "/ws")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUpgradeRequired {
		t.Fatalf("got %d", resp.StatusCode)
	}
}

func TestPresenceCallbacks(t *testing.T) {
	var online, offline atomic.Int32
	_, srv := newTestHub(t, Config{OnPresence: func(_ int64, up bool) {
		if up {
			online.Add(1)
		} else {
			offline.Add(1)
		}
	}})
	a := connect(t, srv, "u1")
	b := connect(t, srv, "u1b")
	time.Sleep(50 * time.Millisecond)
	if online.Load() != 1 {
		t.Fatalf("online fired %d times for two devices, want 1", online.Load())
	}
	a.conn.Close()
	time.Sleep(150 * time.Millisecond)
	if offline.Load() != 0 {
		t.Fatal("offline fired while another device is still connected")
	}
	b.conn.Close()
	deadline := time.Now().Add(2 * time.Second)
	for offline.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if offline.Load() != 1 {
		t.Fatalf("offline fired %d times, want 1", offline.Load())
	}
}

func TestFragmentedInboundAuthIsNotAccepted(t *testing.T) {
	// The auth frame must be a single complete text frame.
	_, srv := newTestHub(t, Config{AuthTimeout: 500 * time.Millisecond})
	c, _ := dial(t, srv, "")
	b, _ := json.Marshal(map[string]string{"type": "auth", "token": "u1"})
	// FIN=0 text frame (masked).
	key := []byte{1, 2, 3, 4}
	body := make([]byte, len(b))
	for i := range b {
		body[i] = b[i] ^ key[i&3]
	}
	frame := append([]byte{byte(opText), 0x80 | byte(len(b))}, key...)
	c.conn.Write(append(frame, body...))
	if code := c.expectClosed(2 * time.Second); code != closePolicy && code != -1 {
		t.Fatalf("close code %d", code)
	}
}

func TestCloseUserExceptKeepsCurrentSession(t *testing.T) {
	h, srv := newTestHub(t, Config{})
	keep := connect(t, srv, "u1")   // session 101
	other := connect(t, srv, "u1b") // session 102
	h.CloseUserExcept(1, 101)
	if code := other.expectClosed(2 * time.Second); code != closePolicy && code != -1 {
		t.Fatalf("other device close code %d", code)
	}
	h.Publish([]int64{1}, Event{Type: "kept"})
	if ev, err := keep.nextEvent(2 * time.Second); err != nil || ev.Type != "kept" {
		t.Fatalf("current session was dropped: %+v %v", ev, err)
	}
}
