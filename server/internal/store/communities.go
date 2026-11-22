package store

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type ChatMember struct {
	UserID      int64     `json:"user_id"`
	Username    string    `json:"username"`
	DisplayName string    `json:"display_name"`
	AvatarURL   string    `json:"avatar_url"`
	Role        string    `json:"role"`
	JoinedAt    time.Time `json:"joined_at"`
}

type CommunityChat struct {
	ID          int64     `json:"id"`
	Kind        string    `json:"kind"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	IsPublic    bool      `json:"is_public"`
	Username    string    `json:"username,omitempty"`
	Role        string    `json:"role"`
	MemberCount int64     `json:"member_count"`
	CreatedAt   time.Time `json:"created_at"`
}

// CreateCommunityChat atomically creates a group/channel and assigns its creator
// the owner role. Public usernames are unique case-insensitively.
func (s *Store) CreateCommunityChat(ctx context.Context, ownerID int64, kind, title, description, username string, isPublic bool) (CommunityChat, error) {
	var chat CommunityChat
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return chat, err
	}
	defer tx.Rollback(ctx)

	err = tx.QueryRow(ctx, `INSERT INTO chats(kind,title,description,is_public,username)
		VALUES($1,$2,$3,$4,NULLIF($5,'')) RETURNING id,kind,title,description,is_public,COALESCE(username,''),created_at`,
		kind, title, description, isPublic, username,
	).Scan(&chat.ID, &chat.Kind, &chat.Title, &chat.Description, &chat.IsPublic, &chat.Username, &chat.CreatedAt)
	if err != nil {
		return chat, norm(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO chat_members(chat_id,user_id,role) VALUES($1,$2,'owner')`, chat.ID, ownerID); err != nil {
		return chat, norm(err)
	}
	chat.Role = "owner"
	chat.MemberCount = 1
	if err = tx.Commit(ctx); err != nil {
		return CommunityChat{}, err
	}
	return chat, nil
}

func (s *Store) ListCommunityMembers(ctx context.Context, chatID, viewerID int64) ([]ChatMember, error) {
	var allowed bool
	if err := s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM chat_members WHERE chat_id=$1 AND user_id=$2)`, chatID, viewerID).Scan(&allowed); err != nil {
		return nil, err
	}
	if !allowed {
		return nil, ErrNotFound
	}
	rows, err := s.Pool.Query(ctx, `SELECT u.id,u.username,u.display_name,u.avatar_url,cm.role,cm.joined_at
		FROM chat_members cm JOIN users u ON u.id=cm.user_id
		WHERE cm.chat_id=$1 ORDER BY CASE cm.role WHEN 'owner' THEN 0 WHEN 'admin' THEN 1 ELSE 2 END,cm.joined_at`, chatID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ChatMember{}
	for rows.Next() {
		var m ChatMember
		if err := rows.Scan(&m.UserID,&m.Username,&m.DisplayName,&m.AvatarURL,&m.Role,&m.JoinedAt); err != nil {
			return nil, err
		}
		out = append(out,m)
	}
	return out, rows.Err()
}

// AddCommunityMember allows only owners and admins to invite users. A channel
// or group membership is always scoped to the exact chat ID.
func (s *Store) AddCommunityMember(ctx context.Context, chatID, actorID, targetID int64) error {
	tag, err := s.Pool.Exec(ctx, `INSERT INTO chat_members(chat_id,user_id)
		SELECT $1,$3
		WHERE EXISTS(SELECT 1 FROM chats WHERE id=$1 AND kind IN ('group','channel'))
		  AND EXISTS(SELECT 1 FROM chat_members WHERE chat_id=$1 AND user_id=$2 AND role IN ('owner','admin'))
		  AND EXISTS(SELECT 1 FROM users WHERE id=$3)
		ON CONFLICT(chat_id,user_id) DO NOTHING`, chatID, actorID, targetID)
	if err != nil {
		return norm(err)
	}
	if tag.RowsAffected() == 0 {
		var permitted bool
		if err := s.Pool.QueryRow(ctx, `SELECT EXISTS(
			SELECT 1 FROM chats c JOIN chat_members a ON a.chat_id=c.id
			WHERE c.id=$1 AND c.kind IN ('group','channel')
			  AND a.user_id=$2 AND a.role IN ('owner','admin')
		)`, chatID, actorID).Scan(&permitted); err != nil {
			return err
		}
		if !permitted {
			return ErrNotFound
		}
		return ErrConflict
	}
	return nil
}

func (s *Store) LeaveCommunityChat(ctx context.Context, chatID, userID int64) error {
	tag, err := s.Pool.Exec(ctx, `DELETE FROM chat_members cm
		WHERE cm.chat_id=$1 AND cm.user_id=$2
		  AND EXISTS(SELECT 1 FROM chats c WHERE c.id=cm.chat_id AND c.kind IN ('group','channel'))
		  AND cm.role <> 'owner'`, chatID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		var owner bool
		if err := s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM chat_members WHERE chat_id=$1 AND user_id=$2 AND role='owner')`, chatID, userID).Scan(&owner); err != nil {
			return err
		}
		if owner {
			return ErrConflict
		}
		return ErrNotFound
	}
	return nil
}

func (s *Store) SetCommunityMemberRole(ctx context.Context, chatID, ownerID, targetID int64, role string) error {
	tag, err := s.Pool.Exec(ctx, `UPDATE chat_members target SET role=$4
		WHERE target.chat_id=$1 AND target.user_id=$3 AND target.role <> 'owner'
		  AND EXISTS(SELECT 1 FROM chat_members actor WHERE actor.chat_id=$1 AND actor.user_id=$2 AND actor.role='owner')
		  AND EXISTS(SELECT 1 FROM chats WHERE id=$1 AND kind IN ('group','channel'))`,
		chatID, ownerID, targetID, role)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) CommunityChat(ctx context.Context, chatID, viewerID int64) (CommunityChat, error) {
	var c CommunityChat
	err := s.Pool.QueryRow(ctx, `SELECT c.id,c.kind,c.title,c.description,c.is_public,COALESCE(c.username,''),cm.role,
		(SELECT count(*) FROM chat_members WHERE chat_id=c.id),c.created_at
		FROM chats c JOIN chat_members cm ON cm.chat_id=c.id AND cm.user_id=$2
		WHERE c.id=$1 AND c.kind IN ('group','channel')`, chatID, viewerID).
		Scan(&c.ID,&c.Kind,&c.Title,&c.Description,&c.IsPublic,&c.Username,&c.Role,&c.MemberCount,&c.CreatedAt)
	return c, norm(err)
}

var _ = strings.TrimSpace
var _ = pgx.ErrNoRows
