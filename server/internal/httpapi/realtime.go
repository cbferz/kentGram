package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
)

const realtimeTicketTTL = 30 * time.Second

var realtimeUpgrader = websocket.Upgrader{
	HandshakeTimeout: 5 * time.Second,
	CheckOrigin: func(r *http.Request) bool {
		origin := strings.TrimSpace(r.Header.Get("Origin"))
		if origin == "" {
			return true // Native clients do not send an Origin header.
		}
		u, err := url.Parse(origin)
		if err != nil || u.Host == "" {
			return false
		}
		switch strings.ToLower(u.Hostname()) {
		case "web.kentgram.tendercheck.pro", "kentgram.tendercheck.pro", "localhost", "127.0.0.1":
			return true
		default:
			return false
		}
	},
}

// createRealtimeTicket issues a short-lived JWT for browser WebSocket clients.
// The long-lived access token is never placed in a WebSocket URL.
func (a *API) createRealtimeTicket(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	claims := jwt.MapClaims{
		"sub": strconv.FormatInt(userID(r), 10),
		"sid": strconv.FormatInt(sessionID(r), 10),
		"iat": now.Unix(),
		"exp": now.Add(realtimeTicketTTL).Unix(),
		"iss": jwtIssuer,
		"typ": "realtime",
	}
	ticket, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(a.jwtSecret)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "realtime_ticket_failed")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"ticket": ticket,
		"expires_in": int(realtimeTicketTTL.Seconds()),
	})
}

func (a *API) realtime(w http.ResponseWriter, r *http.Request) {
	uid, sid, ok := a.realtimeIdentity(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid_realtime_credentials")
		return
	}
	active, err := a.db.SessionActive(r.Context(), sid, uid)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "auth_unavailable")
		return
	}
	if !active {
		writeError(w, http.StatusUnauthorized, "session_revoked")
		return
	}

	select {
	case a.realtimeSlots <- struct{}{}:
		defer func() { <-a.realtimeSlots }()
	default:
		writeError(w, http.StatusServiceUnavailable, "realtime_capacity_reached")
		return
	}

	// A dedicated connection avoids starving the small query pool when many
	// clients keep their WebSockets open.
	listener, err := a.db.OpenListener(r.Context())
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "realtime_unavailable")
		return
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = listener.Close(closeCtx)
	}()
	if _, err := listener.Exec(r.Context(), "LISTEN kentgram_events"); err != nil {
		writeError(w, http.StatusServiceUnavailable, "realtime_unavailable")
		return
	}

	cursor := int64(0)
	if raw := r.URL.Query().Get("since"); raw != "" {
		cursor, err = strconv.ParseInt(raw, 10, 64)
		if err != nil || cursor < 0 {
			writeError(w, http.StatusBadRequest, "invalid_realtime_cursor")
			return
		}
	}

	ws, err := realtimeUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer ws.Close()
	ws.SetReadLimit(4096)
	_ = ws.SetReadDeadline(time.Now().Add(70 * time.Second))
	ws.SetPongHandler(func(string) error {
		return ws.SetReadDeadline(time.Now().Add(70 * time.Second))
	})

	disconnected := make(chan struct{})
	go func() {
		defer close(disconnected)
		for {
			if _, _, readErr := ws.ReadMessage(); readErr != nil {
				return
			}
		}
	}()

	writeBatch := func() error {
		for {
			events, next, highWater, batchErr := a.db.RealtimeBatch(r.Context(), uid, cursor, 100)
			if batchErr != nil {
				return batchErr
			}
			if err := ws.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
				return err
			}
			if err := ws.WriteJSON(map[string]any{
				"type": "sync.batch",
				// Cursors are decimal strings so JavaScript clients never lose
				// precision when PostgreSQL BIGINT values exceed 2^53-1.
				"cursor": strconv.FormatInt(next, 10),
				"high_water": strconv.FormatInt(highWater, 10),
				"events": events,
			}); err != nil {
				return err
			}
			cursor = next
			if len(events) < 100 {
				return nil
			}
		}
	}

	if err := writeBatch(); err != nil {
		return
	}

	for {
		waitCtx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
		_, waitErr := listener.WaitForNotification(waitCtx)
		cancel()

		select {
		case <-disconnected:
			return
		default:
		}

		if waitErr != nil && !errors.Is(waitErr, context.DeadlineExceeded) {
			return
		}
		active, authErr := a.db.SessionActive(r.Context(), sid, uid)
		if authErr != nil || !active {
			_ = ws.WriteControl(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "session revoked"),
				time.Now().Add(2*time.Second))
			return
		}
		if errors.Is(waitErr, context.DeadlineExceeded) {
			_ = ws.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second))
			continue
		}
		if err := writeBatch(); err != nil {
			return
		}
	}
}

func (a *API) realtimeIdentity(r *http.Request) (int64, int64, bool) {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(strings.ToLower(header), "bearer ") {
		raw := strings.TrimSpace(header[len("Bearer "):])
		uid, sid, ok := a.parseAccess(raw)
		return uid, sid, ok
	}

	raw := strings.TrimSpace(r.URL.Query().Get("ticket"))
	if raw == "" {
		return 0, 0, false
	}
	token, err := jwt.Parse(raw, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, errors.New("unexpected signing method")
		}
		return a.jwtSecret, nil
	}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithIssuer(jwtIssuer), jwt.WithExpirationRequired())
	if err != nil || !token.Valid {
		return 0, 0, false
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || claims["typ"] != "realtime" {
		return 0, 0, false
	}
	sub, _ := claims["sub"].(string)
	sidRaw, _ := claims["sid"].(string)
	uid, uidErr := strconv.ParseInt(sub, 10, 64)
	sid, sidErr := strconv.ParseInt(sidRaw, 10, 64)
	if uidErr != nil || sidErr != nil || uid <= 0 || sid <= 0 {
		return 0, 0, false
	}
	return uid, sid, true
}
