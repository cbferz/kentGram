package store

import (
	"context"
	"time"
	"strconv"
)

type ChatMedia struct {
	MediaID      string    `json:"media_id"`
	ChatID       int64     `json:"chat_id"`
	UploaderID   int64     `json:"uploader_id"`
	StorageName  string    `json:"-"`
	OriginalName string    `json:"original_name"`
	ContentType  string    `json:"content_type"`
	SizeBytes    int64     `json:"size_bytes"`
	CreatedAt    time.Time `json:"created_at"`
}

func (s *Store) CreateChatMediaMessage(ctx context.Context, chatID, uploaderID int64, body string, media ChatMedia) (ChatMessage, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return ChatMessage{}, err
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `INSERT INTO chat_media(
		media_id,chat_id,uploader_id,storage_name,original_name,content_type,size_bytes
	)
	SELECT $1,$2,$3,$4,$5,$6,$7
	WHERE EXISTS(SELECT 1 FROM chat_members WHERE chat_id=$2 AND user_id=$3)
	  AND EXISTS(SELECT 1 FROM chats c WHERE c.id=$2 AND
		(c.kind <> 'channel' OR EXISTS(
			SELECT 1 FROM chat_members permission
			WHERE permission.chat_id=c.id AND permission.user_id=$3
			  AND permission.role IN ('owner','admin')
		)))`,
		media.MediaID, chatID, uploaderID, media.StorageName, media.OriginalName,
		media.ContentType, media.SizeBytes)
	if err != nil {
		return ChatMessage{}, norm(err)
	}

	var message ChatMessage
	err = tx.QueryRow(ctx, `INSERT INTO messages(chat_id,sender_id,body,media_id)
		VALUES($1,$2,$3,$4)
		RETURNING id,chat_id,sender_id,body,created_at,media_id`,
		chatID, uploaderID, body, media.MediaID,
	).Scan(&message.ID, &message.ChatID, &message.SenderID, &message.Body, &message.CreatedAt, &message.MediaID)
	if err != nil {
		return ChatMessage{}, norm(err)
	}
	if err = tx.QueryRow(ctx, `SELECT username FROM users WHERE id=$1`, uploaderID).Scan(&message.SenderUsername); err != nil {
		return ChatMessage{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE chats SET updated_at=NOW() WHERE id=$1`, chatID); err != nil {
		return ChatMessage{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return ChatMessage{}, err
	}
	message.IsMine = true
	message.MediaURL = "/api/v1/chats/" + strconv.FormatInt(chatID, 10) + "/media/" + media.MediaID
	message.MediaMime = media.ContentType
	message.MediaName = media.OriginalName
	message.MediaSize = media.SizeBytes
	return message, nil
}

func (s *Store) GetChatMedia(ctx context.Context, chatID, viewerID int64, mediaID string) (ChatMedia, error) {
	var media ChatMedia
	err := s.Pool.QueryRow(ctx, `SELECT cm.media_id,cm.chat_id,cm.uploader_id,cm.storage_name,
		cm.original_name,cm.content_type,cm.size_bytes,cm.created_at
		FROM chat_media cm
		JOIN chat_members member ON member.chat_id=cm.chat_id AND member.user_id=$3
		WHERE cm.chat_id=$1 AND cm.media_id=$2`,
		chatID, mediaID, viewerID,
	).Scan(&media.MediaID, &media.ChatID, &media.UploaderID, &media.StorageName,
		&media.OriginalName, &media.ContentType, &media.SizeBytes, &media.CreatedAt)
	return media, norm(err)
}
