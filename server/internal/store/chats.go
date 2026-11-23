package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type ChatSummary struct {
	ID              int64      `json:"id"`
	Kind            string     `json:"kind"`
	PeerID          int64      `json:"peer_id"`
	PeerUsername    string     `json:"peer_username"`
	PeerDisplayName string     `json:"peer_display_name"`
	PeerAvatarURL   string     `json:"peer_avatar_url"`
	LastMessage     string     `json:"last_message"`
	LastMessageAt   *time.Time `json:"last_message_at,omitempty"`
	UnreadCount     int64      `json:"unread_count"`
	CreatedAt       time.Time  `json:"created_at"`
}

func (s *Store) CreateDirectChat(ctx context.Context, me, peer int64) (int64, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	var id int64
	err = tx.QueryRow(ctx, `SELECT c.id FROM chats c
        JOIN chat_members a ON a.chat_id=c.id AND a.user_id=$1
        JOIN chat_members b ON b.chat_id=c.id AND b.user_id=$2
        WHERE c.kind='direct'
        LIMIT 1`, me, peer).Scan(&id)
	if err == nil {
		return id, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, err
	}
	if err := tx.QueryRow(ctx, `INSERT INTO chats(kind) VALUES('direct') RETURNING id`).Scan(&id); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO chat_members(chat_id,user_id) VALUES($1,$2),($1,$3)`, id, me, peer); err != nil {
		return 0, err
	}
	return id, tx.Commit(ctx)
}

func (s *Store) ListChats(ctx context.Context, me int64) ([]ChatSummary, error) {
	rows, err := s.Pool.Query(ctx, `SELECT c.id,c.kind,u.id,u.username,u.display_name,u.avatar_url,
        COALESCE(m.body,''),m.created_at,
        (SELECT count(*) FROM messages unread WHERE unread.chat_id=c.id AND unread.sender_id<>$1
          AND unread.id>(SELECT last_read_message_id FROM chat_members WHERE chat_id=c.id AND user_id=$1)
          AND unread.deleted_at IS NULL),
        c.created_at
        FROM chats c
        JOIN chat_members mine ON mine.chat_id=c.id AND mine.user_id=$1
        JOIN chat_members peer ON peer.chat_id=c.id AND peer.user_id<>$1
        JOIN users u ON u.id=peer.user_id
        LEFT JOIN LATERAL (SELECT body,created_at FROM messages WHERE chat_id=c.id AND deleted_at IS NULL ORDER BY id DESC LIMIT 1) m ON TRUE
        ORDER BY COALESCE(m.created_at,c.created_at) DESC
        LIMIT 100`, me)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ChatSummary{}
	for rows.Next() {
		var c ChatSummary
		if err := rows.Scan(&c.ID, &c.Kind, &c.PeerID, &c.PeerUsername, &c.PeerDisplayName, &c.PeerAvatarURL, &c.LastMessage, &c.LastMessageAt, &c.UnreadCount, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) IsChatMember(ctx context.Context, chatID, userID int64) (bool, error) {
	var ok bool
	err := s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM chat_members WHERE chat_id=$1 AND user_id=$2)`, chatID, userID).Scan(&ok)
	return ok, err
}

// ChatMemberIDs lists every member of a chat; used to address realtime events.
func (s *Store) ChatMemberIDs(ctx context.Context, chatID int64) ([]int64, error) {
	rows, err := s.Pool.Query(ctx, sqlChatMemberIDs, chatID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// PeerIDs lists every distinct user who shares a chat with userID; used to
// fan out presence changes.
func (s *Store) PeerIDs(ctx context.Context, userID int64) ([]int64, error) {
	rows, err := s.Pool.Query(ctx, sqlPeerIDs, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
