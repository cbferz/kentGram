package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type ChatSummary struct {
	ID int64 `json:"id"`
	Kind string `json:"kind"`
	PeerID int64 `json:"peer_id"`
	PeerUsername string `json:"peer_username"`
	PeerDisplayName string `json:"peer_display_name"`
	PeerAvatarURL string `json:"peer_avatar_url"`
	LastMessage string `json:"last_message"`
	LastMessageAt *time.Time `json:"last_message_at,omitempty"`
	UnreadCount int64 `json:"unread_count"`
	CreatedAt time.Time `json:"created_at"`
}

type ChatMessage struct {
	ID int64 `json:"id"`
	ChatID int64 `json:"chat_id"`
	SenderID int64 `json:"sender_id"`
	SenderUsername string `json:"sender_username"`
	Body string `json:"body"`
	CreatedAt time.Time `json:"created_at"`
	IsMine bool `json:"is_mine"`
	ReplyToMessageID *int64 `json:"reply_to_message_id,omitempty"`
	ReplyToBody string `json:"reply_to_body,omitempty"`
	ReplyToSenderUsername string `json:"reply_to_sender_username,omitempty"`
}

func (s *Store) CreateDirectChat(ctx context.Context, me, peer int64) (int64, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil { return 0, err }
	defer tx.Rollback(ctx)
	var id int64
	err = tx.QueryRow(ctx, `SELECT c.id FROM chats c
		JOIN chat_members a ON a.chat_id=c.id AND a.user_id=$1
		JOIN chat_members b ON b.chat_id=c.id AND b.user_id=$2
		WHERE c.kind='direct' LIMIT 1`, me, peer).Scan(&id)
	if err == nil { return id, tx.Commit(ctx) }
	if !errors.Is(err, pgx.ErrNoRows) { return 0, err }
	if err := tx.QueryRow(ctx, `INSERT INTO chats(kind) VALUES('direct') RETURNING id`).Scan(&id); err != nil { return 0, err }
	if _, err := tx.Exec(ctx, `INSERT INTO chat_members(chat_id,user_id) VALUES($1,$2),($1,$3)`, id, me, peer); err != nil { return 0, err }
	if err := insertRealtimeEvent(ctx, tx, id, me, "chat.created", map[string]any{"chat_id": id}); err != nil { return 0, err }
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
		ORDER BY COALESCE(m.created_at,c.created_at) DESC LIMIT 100`, me)
	if err != nil { return nil, err }
	defer rows.Close()
	out := []ChatSummary{}
	for rows.Next() {
		var c ChatSummary
		if err := rows.Scan(&c.ID,&c.Kind,&c.PeerID,&c.PeerUsername,&c.PeerDisplayName,&c.PeerAvatarURL,&c.LastMessage,&c.LastMessageAt,&c.UnreadCount,&c.CreatedAt); err != nil { return nil, err }
		out = append(out,c)
	}
	return out, rows.Err()
}

func (s *Store) IsChatMember(ctx context.Context, chatID, userID int64) (bool,error) {
	var ok bool
	err := s.Pool.QueryRow(ctx,`SELECT EXISTS(SELECT 1 FROM chat_members WHERE chat_id=$1 AND user_id=$2)`,chatID,userID).Scan(&ok)
	return ok,err
}

func (s *Store) ChatMessages(ctx context.Context, chatID, viewerID, beforeID int64, limit int) ([]ChatMessage,error) {
	if limit < 1 || limit > 100 { limit = 50 }
	rows,err := s.Pool.Query(ctx,`SELECT m.id,m.chat_id,m.sender_id,u.username,m.body,m.created_at,(m.sender_id=$2),m.reply_to_message_id,COALESCE(reply.body,''),COALESCE(reply_user.username,'')
		FROM messages m JOIN users u ON u.id=m.sender_id
		LEFT JOIN messages reply ON reply.id=m.reply_to_message_id AND reply.chat_id=m.chat_id AND reply.deleted_at IS NULL
		LEFT JOIN users reply_user ON reply_user.id=reply.sender_id
		WHERE m.chat_id=$1 AND m.deleted_at IS NULL AND ($3=0 OR m.id<$3)
		ORDER BY m.id DESC LIMIT $4`,chatID,viewerID,beforeID,limit)
	if err != nil { return nil,err }
	defer rows.Close()
	out:=[]ChatMessage{}
	for rows.Next() {
		var m ChatMessage
		if err:=rows.Scan(&m.ID,&m.ChatID,&m.SenderID,&m.SenderUsername,&m.Body,&m.CreatedAt,&m.IsMine,&m.ReplyToMessageID,&m.ReplyToBody,&m.ReplyToSenderUsername);err!=nil{return nil,err}
		out=append(out,m)
	}
	for i,j:=0,len(out)-1;i<j;i,j=i+1,j-1 { out[i],out[j]=out[j],out[i] }
	if rows.Err()!=nil{return nil,rows.Err()}
	if _,err:=s.Pool.Exec(ctx,`UPDATE chat_members SET last_read_message_id=GREATEST(last_read_message_id,COALESCE((SELECT max(id) FROM messages WHERE chat_id=$1),0)) WHERE chat_id=$1 AND user_id=$2`,chatID,viewerID);err!=nil{return nil,err}
	return out,nil
}

func (s *Store) SendChatMessage(ctx context.Context, chatID,senderID int64,body string, replyToID int64)(ChatMessage,error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil { return ChatMessage{}, err }
	defer tx.Rollback(ctx)

	var m ChatMessage
	err = tx.QueryRow(ctx,`INSERT INTO messages(chat_id,sender_id,body,reply_to_message_id)
		SELECT $1,$2,$3,CASE WHEN $4=0 THEN NULL ELSE $4 END
		WHERE EXISTS(SELECT 1 FROM chat_members WHERE chat_id=$1 AND user_id=$2)
		  AND ($4=0 OR EXISTS(SELECT 1 FROM messages WHERE id=$4 AND chat_id=$1 AND deleted_at IS NULL))
		RETURNING id,chat_id,sender_id,body,created_at`,chatID,senderID,body,replyToID).
		Scan(&m.ID,&m.ChatID,&m.SenderID,&m.Body,&m.CreatedAt)
	if err!=nil{return m,norm(err)}
	m.IsMine=true
	if replyToID > 0 { m.ReplyToMessageID = &replyToID }
	if err=tx.QueryRow(ctx,`SELECT username FROM users WHERE id=$1`,senderID).Scan(&m.SenderUsername);err!=nil{return m,err}
	if _,err=tx.Exec(ctx,`UPDATE chats SET updated_at=NOW() WHERE id=$1`,chatID);err!=nil{return m,err}
	if err=insertRealtimeEvent(ctx,tx,chatID,senderID,"message.created",m);err!=nil{return m,err}
	if err=tx.Commit(ctx);err!=nil{return m,err}
	return m,nil
}

// DeleteChatMessage and its event are committed atomically. Both identifiers
// are scoped in the UPDATE, so a message ID from another chat cannot be used.
func (s *Store) DeleteChatMessage(ctx context.Context, chatID, messageID, senderID int64) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var deletedAt time.Time
	err = tx.QueryRow(ctx, `UPDATE messages
		SET deleted_at = NOW()
		WHERE id = $1 AND chat_id = $2 AND sender_id = $3
		  AND deleted_at IS NULL
		  AND EXISTS (SELECT 1 FROM chat_members WHERE chat_id = $2 AND user_id = $3)
		RETURNING deleted_at`, messageID, chatID, senderID).Scan(&deletedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		// Preserve idempotency without exposing whether another user's or
		// another chat's message exists. A repeat succeeds only for the
		// original sender, in the original chat, while still a member.
		var alreadyDeleted bool
		checkErr := tx.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM messages m
				WHERE m.id = $1 AND m.chat_id = $2 AND m.sender_id = $3
				  AND m.deleted_at IS NOT NULL
				  AND EXISTS (
					SELECT 1 FROM chat_members cm
					WHERE cm.chat_id = $2 AND cm.user_id = $3
				  )
			)`, messageID, chatID, senderID).Scan(&alreadyDeleted)
		if checkErr != nil {
			return checkErr
		}
		if alreadyDeleted {
			return nil
		}
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if err := insertRealtimeEvent(ctx, tx, chatID, senderID, "message.deleted", map[string]any{
		"chat_id": chatID, "message_id": messageID, "deleted_at": deletedAt,
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
