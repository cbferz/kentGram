package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

var (
	// ErrReactionLimit: the message already carries the maximum number of
	// distinct reaction types.
	ErrReactionLimit = errors.New("too many distinct reactions on message")
)

const (
	maxReactionTypesPerMessage = 10
	maxReactionsPerUser        = 3
)

type Reaction struct {
	Emoji string `json:"emoji"`
	Count int    `json:"count"`
	Mine  bool   `json:"mine"`
}

type ChatMessage struct {
	ID                    int64      `json:"id"`
	ChatID                int64      `json:"chat_id"`
	SenderID              int64      `json:"sender_id"`
	SenderUsername        string     `json:"sender_username"`
	Body                  string     `json:"body"`
	CreatedAt             time.Time  `json:"created_at"`
	IsMine                bool       `json:"is_mine"`
	ReplyToMessageID      *int64     `json:"reply_to_message_id,omitempty"`
	ReplyToBody           string     `json:"reply_to_body,omitempty"`
	ReplyToSenderUsername string     `json:"reply_to_sender_username,omitempty"`
	EditedAt              *time.Time `json:"edited_at,omitempty"`
	ForwardedFromUserID   *int64     `json:"forwarded_from_user_id,omitempty"`
	ForwardedFromUsername string     `json:"forwarded_from_username,omitempty"`
	Pinned                bool       `json:"pinned"`
	Reactions             []Reaction `json:"reactions"`
}

// SendInput describes one message to send. ClientID is an optional
// idempotency key: repeating a send with the same key returns the original.
type SendInput struct {
	ChatID    int64
	SenderID  int64
	Body      string
	ReplyToID int64
	ClientID  string
}

// ReactionResult is what a reaction change leaves behind: the per-emoji totals
// and the emojis the acting user now has on the message (so every device of
// that user can refresh its own "mine" flags, including after FIFO eviction).
type ReactionResult struct {
	MessageID  int64           `json:"message_id"`
	Counts     []ReactionCount `json:"counts"`
	ActorEmoji []string        `json:"actor_emojis"`
}

type ReactionCount struct {
	Emoji string `json:"emoji"`
	Count int    `json:"count"`
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanMessage(row rowScanner) (ChatMessage, error) {
	var m ChatMessage
	var reactions []byte
	err := row.Scan(&m.ID, &m.ChatID, &m.SenderID, &m.SenderUsername, &m.Body, &m.CreatedAt, &m.IsMine,
		&m.ReplyToMessageID, &m.ReplyToBody, &m.ReplyToSenderUsername,
		&m.EditedAt, &m.ForwardedFromUserID, &m.ForwardedFromUsername, &m.Pinned, &reactions)
	if err != nil {
		return m, err
	}
	m.Reactions = []Reaction{}
	if len(reactions) > 0 {
		if err := json.Unmarshal(reactions, &m.Reactions); err != nil {
			return m, err
		}
	}
	return m, nil
}

// MessageByID loads one visible message of the chat from viewerID's point of
// view. Membership is NOT checked here; callers authorise first.
func (s *Store) MessageByID(ctx context.Context, chatID, messageID, viewerID int64) (ChatMessage, error) {
	return scanMessage(s.Pool.QueryRow(ctx, sqlMessageByID, chatID, viewerID, messageID))
}

// ChatMessages returns a page of history, oldest first. It does not touch read
// markers; the HTTP handler calls MarkRead so peers can be notified.
func (s *Store) ChatMessages(ctx context.Context, chatID, viewerID, beforeID int64, limit int) ([]ChatMessage, error) {
	if limit < 1 || limit > 100 {
		limit = 50
	}
	rows, err := s.Pool.Query(ctx, sqlMessageHistory, chatID, viewerID, beforeID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ChatMessage{}
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

// SendChatMessage keeps the original signature for existing callers.
func (s *Store) SendChatMessage(ctx context.Context, chatID, senderID int64, body string, replyToID int64) (ChatMessage, error) {
	m, _, err := s.SendMessage(ctx, SendInput{ChatID: chatID, SenderID: senderID, Body: body, ReplyToID: replyToID})
	return m, err
}

// SendMessage inserts a message. created is false when ClientID matched an
// earlier send by the same user in the same chat and that message was returned
// instead; callers must not broadcast such a replay again.
func (s *Store) SendMessage(ctx context.Context, in SendInput) (m ChatMessage, created bool, err error) {
	var id int64
	err = s.Pool.QueryRow(ctx, sqlSendMessage, in.ChatID, in.SenderID, in.Body, in.ReplyToID, in.ClientID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		if in.ClientID == "" {
			return m, false, ErrNotFound
		}
		// Either the send was refused (not a member / bad reply target) or this
		// is a replay of an earlier send.
		var existingID, existingChat int64
		lookup := s.Pool.QueryRow(ctx, sqlMessageByClientID, in.SenderID, in.ClientID).Scan(&existingID, &existingChat)
		if errors.Is(lookup, pgx.ErrNoRows) {
			return m, false, ErrNotFound
		}
		if lookup != nil {
			return m, false, lookup
		}
		if existingChat != in.ChatID {
			return m, false, ErrConflict
		}
		m, err = s.MessageByID(ctx, in.ChatID, existingID, in.SenderID)
		return m, false, norm(err)
	}
	if err != nil {
		return m, false, norm(err)
	}
	if _, err = s.Pool.Exec(ctx, sqlTouchChat, in.ChatID); err != nil {
		return m, true, err
	}
	m, err = s.MessageByID(ctx, in.ChatID, id, in.SenderID)
	return m, true, err
}

// EditChatMessage changes the text of a message its sender still owns.
// Chat, message and sender are all scoped in the UPDATE.
func (s *Store) EditChatMessage(ctx context.Context, chatID, messageID, userID int64, body string) (ChatMessage, error) {
	var id int64
	if err := s.Pool.QueryRow(ctx, sqlEditMessage, messageID, chatID, userID, body).Scan(&id); err != nil {
		return ChatMessage{}, err
	}
	return s.MessageByID(ctx, chatID, id, userID)
}

// DeleteChatMessage soft-deletes only messages sent by a current chat member.
// Both IDs are scoped in the UPDATE to prevent cross-chat message mutations.
func (s *Store) DeleteChatMessage(ctx context.Context, chatID, messageID, senderID int64) error {
	var id int64
	err := s.Pool.QueryRow(ctx, sqlDeleteMessage, messageID, chatID, senderID).Scan(&id)
	if err != nil {
		return err // pgx.ErrNoRows == ErrNotFound
	}
	return nil
}

// ForwardMessage copies a message from a chat the user belongs to into
// another chat the user belongs to. The original author is preserved even
// through repeated forwards. created is false for an idempotent replay.
func (s *Store) ForwardMessage(ctx context.Context, srcChatID, messageID, dstChatID, userID int64, clientID string) (m ChatMessage, created bool, err error) {
	var id int64
	err = s.Pool.QueryRow(ctx, sqlForwardMessage, srcChatID, messageID, userID, dstChatID, clientID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		if clientID == "" {
			return m, false, ErrNotFound
		}
		var existingID, existingChat int64
		lookup := s.Pool.QueryRow(ctx, sqlMessageByClientID, userID, clientID).Scan(&existingID, &existingChat)
		if errors.Is(lookup, pgx.ErrNoRows) {
			return m, false, ErrNotFound
		}
		if lookup != nil {
			return m, false, lookup
		}
		if existingChat != dstChatID {
			return m, false, ErrConflict
		}
		m, err = s.MessageByID(ctx, dstChatID, existingID, userID)
		return m, false, norm(err)
	}
	if err != nil {
		return m, false, norm(err)
	}
	if _, err = s.Pool.Exec(ctx, sqlTouchChat, dstChatID); err != nil {
		return m, true, err
	}
	m, err = s.MessageByID(ctx, dstChatID, id, userID)
	return m, true, err
}

// PinMessage pins a message for everyone in the chat. Pinning an already
// pinned message succeeds and changes nothing.
func (s *Store) PinMessage(ctx context.Context, chatID, messageID, userID int64) error {
	var visible bool
	if err := s.Pool.QueryRow(ctx, sqlPinMessage, chatID, messageID, userID).Scan(&visible); err != nil {
		return err
	}
	if !visible {
		return ErrNotFound
	}
	return nil
}

// UnpinMessage removes a pin. It is idempotent for members and ErrNotFound for
// non-members, so membership is never leaked.
func (s *Store) UnpinMessage(ctx context.Context, chatID, messageID, userID int64) error {
	var member bool
	if err := s.Pool.QueryRow(ctx, sqlUnpinMessage, chatID, messageID, userID).Scan(&member); err != nil {
		return err
	}
	if !member {
		return ErrNotFound
	}
	return nil
}

// PinnedMessages lists the chat's pins, newest pin first.
func (s *Store) PinnedMessages(ctx context.Context, chatID, viewerID int64) ([]ChatMessage, error) {
	rows, err := s.Pool.Query(ctx, sqlPinnedMessages, chatID, viewerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ChatMessage{}
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// SetReaction adds or removes one reaction. The message row is locked for the
// duration, which serialises concurrent reaction changes on that message and
// makes the per-message and per-user limits race-free.
//
// Limits: at most 10 distinct emojis per message (ErrReactionLimit) and at
// most 3 reactions per user per message (the user's oldest is evicted).
func (s *Store) SetReaction(ctx context.Context, chatID, messageID, userID int64, emoji string, on bool) (ReactionResult, error) {
	res := ReactionResult{MessageID: messageID}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return res, err
	}
	defer tx.Rollback(ctx)

	var locked int
	if err := tx.QueryRow(ctx, sqlLockMessageForReaction, messageID, chatID, userID).Scan(&locked); err != nil {
		return res, err // no row: not a member, wrong chat or deleted
	}

	if on {
		tag, err := tx.Exec(ctx, sqlAddReaction, messageID, chatID, userID, emoji)
		if err != nil {
			return res, norm(err)
		}
		if tag.RowsAffected() == 1 {
			var types int
			if err := tx.QueryRow(ctx, sqlCountReactionTypes, messageID).Scan(&types); err != nil {
				return res, err
			}
			if types > maxReactionTypesPerMessage {
				return res, ErrReactionLimit // deferred Rollback undoes the insert
			}
			if _, err := tx.Exec(ctx, sqlEvictOldReactions, messageID, userID, maxReactionsPerUser); err != nil {
				return res, err
			}
		}
	} else if _, err := tx.Exec(ctx, sqlRemoveReaction, messageID, userID, emoji); err != nil {
		return res, err
	}

	rows, err := tx.Query(ctx, sqlReactionTotals, messageID, userID)
	if err != nil {
		return res, err
	}
	res.Counts = []ReactionCount{}
	res.ActorEmoji = []string{}
	for rows.Next() {
		var rc ReactionCount
		var mine bool
		if err := rows.Scan(&rc.Emoji, &rc.Count, &mine); err != nil {
			rows.Close()
			return res, err
		}
		res.Counts = append(res.Counts, rc)
		if mine {
			res.ActorEmoji = append(res.ActorEmoji, rc.Emoji)
		}
	}
	rows.Close()
	if rows.Err() != nil {
		return res, rows.Err()
	}
	return res, tx.Commit(ctx)
}

// MarkRead advances the user's read marker, never backwards and never past the
// newest message that exists in this chat. It returns the resulting marker.
func (s *Store) MarkRead(ctx context.Context, chatID, userID, upTo int64) (int64, error) {
	var last int64
	err := s.Pool.QueryRow(ctx, sqlMarkRead, chatID, userID, upTo).Scan(&last)
	return last, err
}

// PeerLastRead is the highest read marker among the chat's other members; a
// message with id <= this value has been read by the peer.
func (s *Store) PeerLastRead(ctx context.Context, chatID, viewerID int64) (int64, error) {
	var last int64
	err := s.Pool.QueryRow(ctx, sqlPeerLastRead, chatID, viewerID).Scan(&last)
	return last, err
}
