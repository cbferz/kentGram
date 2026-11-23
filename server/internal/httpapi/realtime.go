package httpapi

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/cbferz/kentgram/server/internal/realtime"
	"github.com/cbferz/kentgram/server/internal/store"
)

// newHub wires the realtime hub to this API's authentication, origin policy
// and presence fan-out.
func (a *API) newHub() *realtime.Hub {
	return realtime.NewHub(realtime.Config{
		Auth:        a.wsAuth,
		AllowOrigin: a.wsOriginAllowed,
		OnPresence:  a.onPresence,
	})
}

// Close releases background resources (open WebSockets).
func (a *API) Close() {
	if a.hub != nil {
		a.hub.Close()
	}
}

// wsAuth validates the access token sent as the first WebSocket frame and
// confirms its session is still live, exactly like the HTTP middleware.
func (a *API) wsAuth(token string) (realtime.Identity, bool) {
	uid, sid, ok := a.parseAccess(token)
	if !ok {
		return realtime.Identity{}, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	active, err := a.db.SessionActive(ctx, sid, uid)
	if err != nil || !active {
		return realtime.Identity{}, false
	}
	return realtime.Identity{UserID: uid, SessionID: sid}, true
}

// wsOriginAllowed permits the configured web origins and same-host pages.
func (a *API) wsOriginAllowed(origin string, r *http.Request) bool {
	origin = strings.TrimRight(origin, "/")
	for _, allowed := range a.cfg.WSAllowedOrigins {
		if strings.EqualFold(allowed, origin) {
			return true
		}
	}
	i := strings.Index(origin, "://")
	return i >= 0 && strings.EqualFold(origin[i+3:], r.Host)
}

func (a *API) onPresence(uid int64, online bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	peers, err := a.db.PeerIDs(ctx, uid)
	if err != nil || len(peers) == 0 {
		return
	}
	a.hub.Publish(peers, realtime.Event{
		Type: "presence",
		Data: map[string]any{"user_id": uid, "online": online},
	})
}

// publishChat sends an event to every member of the chat except `except`
// (pass 0 to include everyone). Delivery is best effort: REST is the source of
// truth and clients re-sync after reconnecting.
func (a *API) publishChat(ctx context.Context, chatID, except int64, ev realtime.Event) {
	if a.hub == nil {
		return
	}
	ids, err := a.db.ChatMemberIDs(ctx, chatID)
	if err != nil {
		return
	}
	if except != 0 {
		filtered := ids[:0]
		for _, id := range ids {
			if id != except {
				filtered = append(filtered, id)
			}
		}
		ids = filtered
	}
	ev.ChatID = chatID
	a.hub.Publish(ids, ev)
}

// neutral strips viewer-specific fields from a message before it is broadcast
// to several users. Receivers derive "is mine" from sender_id and keep their
// own "mine" reaction flags.
func neutral(m store.ChatMessage) store.ChatMessage {
	m.IsMine = false
	rs := make([]store.Reaction, len(m.Reactions))
	for i, r := range m.Reactions {
		r.Mine = false
		rs[i] = r
	}
	m.Reactions = rs
	return m
}

// presence returns which of the caller's chat peers are online right now.
func (a *API) presence(w http.ResponseWriter, r *http.Request) {
	peers, err := a.db.PeerIDs(r.Context(), userID(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "presence_failed")
		return
	}
	online := []int64{}
	for _, id := range peers {
		if a.hub.Online(id) {
			online = append(online, id)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"online": online})
}
