package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type realtimeEvent struct {
	Type      string `json:"type"`
	ChatID    int64  `json:"chat_id,omitempty"`
	MessageID int64  `json:"message_id,omitempty"`
	UserID    int64  `json:"user_id,omitempty"`
	Online    bool   `json:"online"`
}

type realtimeClient struct {
	userID    int64
	sessionID int64
	conn      *websocket.Conn
	send      chan []byte
	done      chan struct{}
	hub       *realtimeHub
	api       *API
	once      sync.Once
}

type realtimeHub struct {
	mu      sync.RWMutex
	clients         map[int64]map[*realtimeClient]struct{}
	presenceVersion map[int64]uint64
}

func newRealtimeHub() *realtimeHub {
	return &realtimeHub{
		clients: make(map[int64]map[*realtimeClient]struct{}),
		presenceVersion: make(map[int64]uint64),
	}
}

// register returns true only when this is the user's first live connection.
func (h *realtimeHub) register(userID, sessionID int64, conn *websocket.Conn, api *API) (*realtimeClient, bool, uint64) {
	client := &realtimeClient{
		userID: userID, sessionID: sessionID, conn: conn,
		send: make(chan []byte, 64), done: make(chan struct{}), hub: h, api: api,
	}
	h.mu.Lock()
	firstOnline := len(h.clients[userID]) == 0
	if firstOnline {
		h.presenceVersion[userID]++
	}
	if h.clients[userID] == nil {
		h.clients[userID] = make(map[*realtimeClient]struct{})
	}
	h.clients[userID][client] = struct{}{}
	version := h.presenceVersion[userID]
	h.mu.Unlock()
	return client, firstOnline, version
}

// unregister returns true only when the user's final live connection closed.
func (h *realtimeHub) unregister(client *realtimeClient) (bool, uint64) {
	becameOffline := false
	var version uint64
	client.once.Do(func() {
		h.mu.Lock()
		if userClients := h.clients[client.userID]; userClients != nil {
			delete(userClients, client)
			if len(userClients) == 0 {
				delete(h.clients, client.userID)
				becameOffline = true
				h.presenceVersion[client.userID]++
				version = h.presenceVersion[client.userID]
			}
		}
		h.mu.Unlock()
		close(client.done)
		_ = client.conn.Close()
	})
	return becameOffline, version
}

// publishPresence discards stale online/offline transitions if another
// connection changed the user's state while membership was being loaded.
func (h *realtimeHub) publishPresence(userIDs []int64, event realtimeEvent, excludeUserID, subjectUserID int64, online bool, version uint64) {
	payload, err := json.Marshal(event)
	if err != nil {
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.presenceVersion[subjectUserID] != version || (len(h.clients[subjectUserID]) > 0) != online {
		return
	}
	for _, userID := range userIDs {
		if userID == excludeUserID {
			continue
		}
		for client := range h.clients[userID] {
			select {
			case client.send <- payload:
			default:
			}
		}
	}
}

func (h *realtimeHub) isOnline(userID int64) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients[userID]) > 0
}

// publish sends events only to users in the supplied chat membership set.
func (h *realtimeHub) publish(userIDs []int64, event realtimeEvent, excludeUserID int64) {
	payload, err := json.Marshal(event)
	if err != nil {
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, userID := range userIDs {
		if userID == excludeUserID {
			continue
		}
		for client := range h.clients[userID] {
			select {
			case client.send <- payload:
			default:
			}
		}
	}
}

func (c *realtimeClient) enqueue(event realtimeEvent) {
	payload, err := json.Marshal(event)
	if err != nil {
		return
	}
	select {
	case c.send <- payload:
	default:
	}
}

func (c *realtimeClient) unregister() {
	becameOffline, version := c.hub.unregister(c)
	if !becameOffline {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c.api.publishUserPresence(ctx, c.userID, false, version)
}

func (c *realtimeClient) readPump(ctx context.Context) {
	defer c.unregister()
	c.conn.SetReadLimit(1024)
	_ = c.conn.SetReadDeadline(time.Now().Add(70 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(70 * time.Second))
	})

	for {
		_, payload, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		var incoming struct {
			Type   string `json:"type"`
			ChatID int64  `json:"chat_id"`
		}
		if json.Unmarshal(payload, &incoming) != nil || incoming.ChatID <= 0 {
			continue
		}
		member, err := c.api.db.IsChatMember(ctx, incoming.ChatID, c.userID)
		if err != nil || !member {
			continue
		}
		members, err := c.api.db.ChatMemberIDs(ctx, incoming.ChatID)
		if err != nil {
			continue
		}

		switch incoming.Type {
		case "typing":
			c.hub.publish(members, realtimeEvent{
				Type: "typing", ChatID: incoming.ChatID, UserID: c.userID,
			}, c.userID)
		case "presence.subscribe":
			for _, memberID := range members {
				if memberID == c.userID {
					continue
				}
				c.enqueue(realtimeEvent{
					Type: "user.presence", ChatID: incoming.ChatID,
					UserID: memberID, Online: c.hub.isOnline(memberID),
				})
			}
		}
	}
}

func (c *realtimeClient) writePump() {
	ticker := time.NewTicker(25 * time.Second)
	defer func() {
		ticker.Stop()
		c.unregister()
	}()

	for {
		select {
		case payload := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.TextMessage, payload); err != nil {
				return
			}
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			active, err := c.api.db.SessionActive(ctx, c.sessionID, c.userID)
			cancel()
			if err != nil || !active {
				return
			}
			_ = c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		case <-c.done:
			return
		}
	}
}

func (a *API) realtime(w http.ResponseWriter, r *http.Request) {
	upgrader := websocket.Upgrader{
		Subprotocols:    []string{"kentgram"},
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			origin := strings.TrimSpace(r.Header.Get("Origin"))
			if origin == "" {
				return true // Native Android clients do not send an Origin header.
			}
			parsed, err := url.Parse(origin)
			return err == nil && (strings.EqualFold(parsed.Host, r.Host) || strings.EqualFold(origin, webClientOrigin))
		},
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	client, firstOnline, version := a.hub.register(userID(r), sessionID(r), conn, a)
	if firstOnline {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		a.publishUserPresence(ctx, client.userID, true, version)
		cancel()
	}
	go client.writePump()
	client.readPump(r.Context())
}

func (a *API) publishUserPresence(ctx context.Context, userID int64, online bool, version uint64) {
	chatIDs, err := a.db.ChatIDsForUser(ctx, userID)
	if err != nil {
		return
	}
	for _, chatID := range chatIDs {
		members, err := a.db.ChatMemberIDs(ctx, chatID)
		if err != nil {
			continue
		}
		a.hub.publishPresence(members, realtimeEvent{
			Type: "user.presence", ChatID: chatID, UserID: userID, Online: online,
		}, userID, userID, online, version)
	}
}

func (a *API) publishChatEvent(ctx context.Context, chatID int64, event realtimeEvent, excludeUserID int64) {
	members, err := a.db.ChatMemberIDs(ctx, chatID)
	if err != nil {
		return
	}
	a.hub.publish(members, event, excludeUserID)
}
