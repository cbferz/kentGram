package httpapi

import (
	"encoding/json"
	"net/http"
	"context"
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
}

type realtimeClient struct {
	userID int64
	conn   *websocket.Conn
	send   chan []byte
	done   chan struct{}
	hub    *realtimeHub
	once   sync.Once
}

type realtimeHub struct {
	mu      sync.RWMutex
	clients map[int64]map[*realtimeClient]struct{}
}

func newRealtimeHub() *realtimeHub {
	return &realtimeHub{clients: make(map[int64]map[*realtimeClient]struct{})}
}

func (h *realtimeHub) register(userID int64, conn *websocket.Conn) *realtimeClient {
	client := &realtimeClient{
		userID: userID,
		conn:   conn,
		send:   make(chan []byte, 64),
		done:   make(chan struct{}),
		hub:    h,
	}
	h.mu.Lock()
	if h.clients[userID] == nil {
		h.clients[userID] = make(map[*realtimeClient]struct{})
	}
	h.clients[userID][client] = struct{}{}
	h.mu.Unlock()
	return client
}

func (h *realtimeHub) unregister(client *realtimeClient) {
	client.once.Do(func() {
		h.mu.Lock()
		if userClients := h.clients[client.userID]; userClients != nil {
			delete(userClients, client)
			if len(userClients) == 0 {
				delete(h.clients, client.userID)
			}
		}
		h.mu.Unlock()
		close(client.done)
		_ = client.conn.Close()
	})
}

// publish sends an event only to users who belong to the chat. A full client
// queue drops that event; the client can recover the canonical state over HTTP.
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

func (c *realtimeClient) readPump(api *API, ctx context.Context) {
	defer c.hub.unregister(c)
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
		if incoming.Type != "typing" {
			continue
		}
		member, err := api.db.IsChatMember(ctx, incoming.ChatID, c.userID)
		if err != nil || !member {
			continue
		}
		members, err := api.db.ChatMemberIDs(apiRequestContext(c), incoming.ChatID)
		if err != nil {
			continue
		}
		c.hub.publish(members, realtimeEvent{
			Type: "typing",
			ChatID: incoming.ChatID,
			UserID: c.userID,
		}, c.userID)
	}
}

func (c *realtimeClient) writePump() {
	ticker := time.NewTicker(25 * time.Second)
	defer func() {
		ticker.Stop()
		c.hub.unregister(c)
	}()

	for {
		select {
		case payload := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.TextMessage, payload); err != nil {
				return
			}
		case <-ticker.C:
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
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			origin := strings.TrimSpace(r.Header.Get("Origin"))
			if origin == "" {
				return true // Native Android clients do not send an Origin header.
			}
			parsed, err := url.Parse(origin)
			return err == nil && strings.EqualFold(parsed.Host, r.Host)
		},
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	client := a.hub.register(userID(r), conn)
	go client.writePump()
	client.readPump(a, r.Context())
}
