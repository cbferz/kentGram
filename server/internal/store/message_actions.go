package store

import (
	"context"
)

type MessageReaction struct {
	Emoji string `json:"emoji"`
	Count int64 `json:"count"`
	Reacted bool `json:"reacted"`
}

func (s *Store) EditChatMessage(ctx context.Context, chatID, messageID, senderID int64, body string) (ChatMessage, error) {
	var message ChatMessage
	err := s.Pool.QueryRow(ctx, `
		UPDATE messages AS m
		SET body=$4, edited_at=NOW()
		WHERE m.id=$1 AND m.chat_id=$2 AND m.sender_id=$3
		  AND m.deleted_at IS NULL
		  AND EXISTS (
		    SELECT 1 FROM chat_members cm
		    WHERE cm.chat_id=$2 AND cm.user_id=$3
		  )
		RETURNING m.id,m.chat_id,m.sender_id,m.body,m.created_at,m.edited_at
	`, messageID, chatID, senderID, body).Scan(
		&message.ID, &message.ChatID, &message.SenderID, &message.Body,
		&message.CreatedAt, &message.EditedAt,
	)
	if err != nil {
		return message, norm(err)
	}
	message.IsMine = true
	if err := s.Pool.QueryRow(ctx, `SELECT username FROM users WHERE id=$1`, senderID).Scan(&message.SenderUsername); err != nil {
		return message, err
	}
	return message, nil
}

func (s *Store) ToggleMessageReaction(ctx context.Context, chatID, messageID, userID int64, emoji string) ([]MessageReaction, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var visible bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM messages m
			JOIN chat_members cm ON cm.chat_id=m.chat_id AND cm.user_id=$3
			WHERE m.id=$1 AND m.chat_id=$2 AND m.deleted_at IS NULL
		)
	`, messageID, chatID, userID).Scan(&visible); err != nil {
		return nil, err
	}
	if !visible {
		return nil, ErrNotFound
	}

	tag, err := tx.Exec(ctx, `
		DELETE FROM message_reactions
		WHERE message_id=$1 AND chat_id=$2 AND user_id=$3 AND emoji=$4
	`, messageID, chatID, userID, emoji)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		if _, err := tx.Exec(ctx, `
			INSERT INTO message_reactions(message_id,chat_id,user_id,emoji)
			VALUES($1,$2,$3,$4)
			ON CONFLICT(message_id,user_id,emoji) DO NOTHING
		`, messageID, chatID, userID, emoji); err != nil {
			return nil, err
		}
	}

	rows, err := tx.Query(ctx, `
		SELECT emoji,COUNT(*),BOOL_OR(user_id=$3)
		FROM message_reactions
		WHERE chat_id=$1 AND message_id=$2
		GROUP BY emoji
		ORDER BY emoji
	`, chatID, messageID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	reactions := make([]MessageReaction, 0)
	for rows.Next() {
		var reaction MessageReaction
		if err := rows.Scan(&reaction.Emoji, &reaction.Count, &reaction.Reacted); err != nil {
			return nil, err
		}
		reactions = append(reactions, reaction)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return reactions, nil
}

