package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
)

type RealtimeEvent struct {
	ID        int64           `json:"id"`
	ChatID    int64           `json:"chat_id"`
	ActorID   int64           `json:"actor_id"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt string          `json:"created_at"`
}

// insertRealtimeEvent must be called inside the same transaction as the
// mutation. NOTIFY is transactional in PostgreSQL, so listeners are woken only
// after the event and its originating mutation have committed.
func insertRealtimeEvent(ctx context.Context, tx pgx.Tx, chatID, actorID int64, eventType string, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal realtime event: %w", err)
	}
	var id int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO realtime_events(chat_id, actor_id, event_type, payload)
		VALUES($1, $2, $3, $4::jsonb)
		RETURNING id
	`, chatID, actorID, eventType, string(raw)).Scan(&id); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `SELECT pg_notify('kentgram_events', $1)`, fmt.Sprint(id))
	return err
}

// RealtimeBatch returns only events belonging to chats the user currently
// belongs to. The global high-water mark lets clients advance over events from
// other users' chats without exposing those events or getting stuck on gaps.
func (s *Store) RealtimeBatch(ctx context.Context, uid, afterID int64, limit int) ([]RealtimeEvent, int64, int64, error) {
	if limit < 1 || limit > 200 {
		limit = 100
	}
	var highWater int64
	if err := s.Pool.QueryRow(ctx, `SELECT COALESCE(MAX(id), $1) FROM realtime_events`, afterID).Scan(&highWater); err != nil {
		return nil, afterID, afterID, err
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT e.id, e.chat_id, e.actor_id, e.event_type, e.payload::text,
		       to_char(e.created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')
		FROM realtime_events e
		WHERE e.id > $1 AND e.id <= $2
		  AND EXISTS (
		      SELECT 1 FROM chat_members cm
		      WHERE cm.chat_id = e.chat_id AND cm.user_id = $3
		  )
		ORDER BY e.id ASC
		LIMIT $4
	`, afterID, highWater, uid, limit)
	if err != nil {
		return nil, afterID, highWater, err
	}
	defer rows.Close()

	out := make([]RealtimeEvent, 0, limit)
	for rows.Next() {
		var ev RealtimeEvent
		var payload string
		if err := rows.Scan(&ev.ID, &ev.ChatID, &ev.ActorID, &ev.Type, &payload, &ev.CreatedAt); err != nil {
			return nil, afterID, highWater, err
		}
		ev.Payload = json.RawMessage(payload)
		out = append(out, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, afterID, highWater, err
	}

	next := highWater
	if len(out) == limit {
		next = out[len(out)-1].ID
	}
	return out, next, highWater, nil
}
