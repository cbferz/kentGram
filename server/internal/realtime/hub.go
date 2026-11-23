// Package realtime is kentGram's WebSocket event channel. It depends only on
// the standard library. REST stays the source of truth; the hub only pushes
// "something changed" events to the users who are allowed to see them.
package realtime

import (
	"bufio"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Event is the envelope delivered to clients.
type Event struct {
	Type   string `json:"type"`
	ChatID int64  `json:"chat_id,omitempty"`
	Data   any    `json:"data,omitempty"`
}

// Identity is what a valid access token resolves to.
type Identity struct {
	UserID    int64
	SessionID int64
}

// AuthFunc validates an access token (including that its session is live).
type AuthFunc func(token string) (Identity, bool)

// Config tunes the hub; zero values select safe defaults.
type Config struct {
	Auth AuthFunc
	// AllowOrigin decides whether a browser Origin may connect. Requests with
	// no Origin header (native apps) are always allowed. Nil allows only an
	// Origin whose host equals the request host.
	AllowOrigin func(origin string, r *http.Request) bool
	// OnPresence is called (in its own goroutine) when a user's first
	// connection opens or last connection closes.
	OnPresence func(userID int64, online bool)

	AuthTimeout   time.Duration // time allowed to send the auth frame
	PingInterval  time.Duration
	PongTimeout   time.Duration // read deadline, refreshed by any inbound frame
	WriteTimeout  time.Duration
	SendQueue     int // per-connection buffered events before it is dropped
	MaxPerUser    int // simultaneous connections per user
	MaxInboundMsg int64
}

type Hub struct {
	cfg Config

	mu       sync.RWMutex
	byUser   map[int64]map[*client]struct{}
	bySess   map[int64]map[*client]struct{}
	shutdown bool
}

func NewHub(cfg Config) *Hub {
	if cfg.AuthTimeout <= 0 {
		cfg.AuthTimeout = 5 * time.Second
	}
	if cfg.PingInterval <= 0 {
		cfg.PingInterval = 25 * time.Second
	}
	if cfg.PongTimeout <= 0 {
		cfg.PongTimeout = 2*cfg.PingInterval + 5*time.Second
	}
	if cfg.WriteTimeout <= 0 {
		cfg.WriteTimeout = 10 * time.Second
	}
	if cfg.SendQueue <= 0 {
		cfg.SendQueue = 64
	}
	if cfg.MaxPerUser <= 0 {
		cfg.MaxPerUser = 8
	}
	if cfg.MaxInboundMsg <= 0 {
		cfg.MaxInboundMsg = 4096
	}
	return &Hub{
		cfg:    cfg,
		byUser: map[int64]map[*client]struct{}{},
		bySess: map[int64]map[*client]struct{}{},
	}
}

// Publish delivers ev to every live connection of the listed users. It never
// blocks: a connection whose queue is full is closed instead of stalling the
// caller, and the client recovers by reconnecting and re-syncing over REST.
func (h *Hub) Publish(userIDs []int64, ev Event) {
	payload, err := json.Marshal(ev)
	if err != nil {
		return
	}
	var slow []*client
	h.mu.RLock()
	for _, uid := range userIDs {
		for c := range h.byUser[uid] {
			select {
			case c.send <- payload:
			default:
				slow = append(slow, c)
			}
		}
	}
	h.mu.RUnlock()
	for _, c := range slow {
		c.close(closePolicy, "slow consumer")
	}
}

// Online reports whether the user has at least one live connection.
func (h *Hub) Online(userID int64) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.byUser[userID]) > 0
}

// CloseSession drops every connection that authenticated with the session.
// Call it when a session is revoked so revocation is immediate for sockets too.
func (h *Hub) CloseSession(sessionID int64) {
	h.closeSet(func() map[*client]struct{} { return h.bySess[sessionID] })
}

// CloseUser drops every connection of the user.
func (h *Hub) CloseUser(userID int64) {
	h.closeSet(func() map[*client]struct{} { return h.byUser[userID] })
}

func (h *Hub) closeSet(pick func() map[*client]struct{}) {
	h.mu.RLock()
	var cs []*client
	for c := range pick() {
		cs = append(cs, c)
	}
	h.mu.RUnlock()
	for _, c := range cs {
		c.close(closePolicy, "session ended")
	}
}

// Close shuts the hub down and disconnects everyone.
func (h *Hub) Close() {
	h.mu.Lock()
	h.shutdown = true
	var cs []*client
	for _, set := range h.byUser {
		for c := range set {
			cs = append(cs, c)
		}
	}
	h.mu.Unlock()
	for _, c := range cs {
		c.close(closeGoingAway, "server shutting down")
	}
}

func (h *Hub) originAllowed(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	if h.cfg.AllowOrigin != nil {
		return h.cfg.AllowOrigin(origin, r)
	}
	// Default: same host only.
	i := strings.Index(origin, "://")
	return i >= 0 && strings.EqualFold(origin[i+3:], r.Host)
}

// ServeWS upgrades the request. The access token is NOT taken from the URL
// (URLs end up in logs); the client sends {"type":"auth","token":"..."} as its
// first frame and has AuthTimeout to do so.
func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet ||
		!headerHasToken(r.Header, "Connection", "upgrade") ||
		!headerHasToken(r.Header, "Upgrade", "websocket") ||
		r.Header.Get("Sec-WebSocket-Version") != "13" {
		w.Header().Set("Sec-WebSocket-Version", "13")
		http.Error(w, `{"error":"websocket_upgrade_required"}`, http.StatusUpgradeRequired)
		return
	}
	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		http.Error(w, `{"error":"bad_websocket_key"}`, http.StatusBadRequest)
		return
	}
	if !h.originAllowed(r) {
		http.Error(w, `{"error":"origin_not_allowed"}`, http.StatusForbidden)
		return
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, `{"error":"websocket_unsupported"}`, http.StatusInternalServerError)
		return
	}
	conn, rw, err := hj.Hijack()
	if err != nil {
		return
	}
	// The http.Server's Read/WriteTimeout deadlines are still set on a hijacked
	// connection and would kill a healthy socket; clear them.
	_ = conn.SetDeadline(time.Time{})

	_ = conn.SetWriteDeadline(time.Now().Add(h.cfg.WriteTimeout))
	resp := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + acceptKey(key) + "\r\n\r\n"
	if _, err := rw.WriteString(resp); err != nil || rw.Flush() != nil {
		_ = conn.Close()
		return
	}

	c := &client{hub: h, conn: conn, br: rw.Reader, send: make(chan []byte, h.cfg.SendQueue), done: make(chan struct{})}
	// Own goroutine: the HTTP handler (and chi's Timeout middleware) is done.
	go c.run()
}

type client struct {
	hub  *Hub
	conn net.Conn
	br   *bufio.Reader
	send chan []byte
	done chan struct{}

	wmu  sync.Mutex // serialises frame writes
	once sync.Once
	id   Identity
	reg  bool
}

func (c *client) run() {
	if !c.authenticate() {
		return
	}
	go c.writeLoop()
	c.readLoop()
}

func (c *client) authenticate() bool {
	h := c.hub
	_ = c.conn.SetReadDeadline(time.Now().Add(h.cfg.AuthTimeout))
	fin, op, payload, err := readFrame(c.br, 2048)
	if err != nil || !fin || op != opText {
		c.close(closePolicy, "auth required")
		return false
	}
	var msg struct {
		Type  string `json:"type"`
		Token string `json:"token"`
	}
	if json.Unmarshal(payload, &msg) != nil || msg.Type != "auth" || msg.Token == "" || h.cfg.Auth == nil {
		c.close(closePolicy, "auth required")
		return false
	}
	id, ok := h.cfg.Auth(msg.Token)
	if !ok {
		c.close(closePolicy, "invalid token")
		return false
	}
	c.id = id
	// reg must be set before the connection becomes visible in the hub maps so
	// a concurrent CloseUser/CloseSession always unregisters it. unregister
	// tolerates entries that were never added.
	c.reg = true
	if !h.register(c) {
		c.close(closeGoingAway, "server shutting down")
		return false
	}
	c.enqueueDirect(Event{Type: "ready", Data: map[string]int64{"user_id": id.UserID}})
	return true
}

func (c *client) enqueueDirect(ev Event) {
	if b, err := json.Marshal(ev); err == nil {
		select {
		case c.send <- b:
		default:
		}
	}
}

func (h *Hub) register(c *client) (ok bool) {
	var evict *client
	first := false
	h.mu.Lock()
	if h.shutdown {
		h.mu.Unlock()
		return false
	}
	set := h.byUser[c.id.UserID]
	if set == nil {
		set = map[*client]struct{}{}
		h.byUser[c.id.UserID] = set
		first = true
	}
	if len(set) >= h.cfg.MaxPerUser {
		// Evict an arbitrary existing connection to stay within the cap.
		for old := range set {
			evict = old
			break
		}
	}
	set[c] = struct{}{}
	ss := h.bySess[c.id.SessionID]
	if ss == nil {
		ss = map[*client]struct{}{}
		h.bySess[c.id.SessionID] = ss
	}
	ss[c] = struct{}{}
	h.mu.Unlock()

	if evict != nil {
		evict.close(closePolicy, "too many connections")
	}
	if first && h.cfg.OnPresence != nil {
		go h.cfg.OnPresence(c.id.UserID, true)
	}
	return true
}

func (h *Hub) unregister(c *client) {
	last := false
	h.mu.Lock()
	if set := h.byUser[c.id.UserID]; set != nil {
		delete(set, c)
		if len(set) == 0 {
			delete(h.byUser, c.id.UserID)
			last = true
		}
	}
	if ss := h.bySess[c.id.SessionID]; ss != nil {
		delete(ss, c)
		if len(ss) == 0 {
			delete(h.bySess, c.id.SessionID)
		}
	}
	h.mu.Unlock()
	if last && h.cfg.OnPresence != nil {
		go h.cfg.OnPresence(c.id.UserID, false)
	}
}

func (c *client) readLoop() {
	h := c.hub
	var msg []byte
	for {
		_ = c.conn.SetReadDeadline(time.Now().Add(h.cfg.PongTimeout))
		fin, op, payload, err := readFrame(c.br, h.cfg.MaxInboundMsg)
		if err != nil {
			switch err {
			case errTooBig:
				c.close(closeTooBig, "message too big")
			case errProtocol:
				c.close(closeProtocol, "protocol error")
			default:
				c.close(closeGoingAway, "")
			}
			return
		}
		switch op {
		case opPing:
			if c.write(opPong, payload) != nil {
				c.close(closeGoingAway, "")
				return
			}
		case opPong:
			// Deadline already refreshed above.
		case opClose:
			c.close(closeNormal, "")
			return
		case opText, opContinuation:
			msg = append(msg, payload...)
			if int64(len(msg)) > h.cfg.MaxInboundMsg {
				c.close(closeTooBig, "message too big")
				return
			}
			if fin {
				// Clients only talk to the server over REST after auth; inbound
				// application frames are accepted and ignored.
				msg = msg[:0]
			}
		default:
			c.close(closeUnsupported, "unsupported frame")
			return
		}
	}
}

func (c *client) writeLoop() {
	t := time.NewTicker(c.hub.cfg.PingInterval)
	defer t.Stop()
	for {
		select {
		case b := <-c.send:
			if c.write(opText, b) != nil {
				c.close(closeGoingAway, "")
				return
			}
		case <-t.C:
			if c.write(opPing, nil) != nil {
				c.close(closeGoingAway, "")
				return
			}
		case <-c.done:
			return
		}
	}
}

func (c *client) write(op byte, payload []byte) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	_ = c.conn.SetWriteDeadline(time.Now().Add(c.hub.cfg.WriteTimeout))
	_, err := c.conn.Write(encodeFrame(op, payload))
	return err
}

// close is idempotent: it sends a best-effort Close frame, drops the socket
// and removes the connection from the hub.
func (c *client) close(code int, reason string) {
	c.once.Do(func() {
		close(c.done)
		c.wmu.Lock()
		_ = c.conn.SetWriteDeadline(time.Now().Add(time.Second))
		_, _ = c.conn.Write(encodeFrame(opClose, closePayload(code, reason)))
		c.wmu.Unlock()
		_ = c.conn.Close()
		if c.reg {
			c.hub.unregister(c)
		}
	})
}

func headerHasToken(h http.Header, name, token string) bool {
	for _, v := range h.Values(name) {
		for _, part := range strings.Split(v, ",") {
			if strings.EqualFold(strings.TrimSpace(part), token) {
				return true
			}
		}
	}
	return false
}
