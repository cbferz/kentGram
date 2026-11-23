package store

import (
    "context"
    "encoding/json"
    "errors"
    "time"
    "strconv"

    "github.com/jackc/pgx/v5"
)

type ChatSummary struct {
    ID int64 `json:"id"`
    Kind string `json:"kind"`
    Title string `json:"title"`
    Description string `json:"description"`
    IsPublic bool `json:"is_public"`
    Username string `json:"username,omitempty"`
    Role string `json:"role"`
    MemberCount int64 `json:"member_count"`
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
    EditedAt *time.Time `json:"edited_at,omitempty"`
    Reactions []MessageReaction `json:"reactions"`
    IsMine bool `json:"is_mine"`
	ReplyToMessageID *int64 `json:"reply_to_message_id,omitempty"`
	ReplyToBody string `json:"reply_to_body,omitempty"`
	ReplyToSenderUsername string `json:"reply_to_sender_username,omitempty"`
	MediaID string `json:"media_id,omitempty"`
	MediaURL string `json:"media_url,omitempty"`
	MediaMime string `json:"media_mime,omitempty"`
	MediaName string `json:"media_name,omitempty"`
	MediaSize int64 `json:"media_size,omitempty"`
	ForwardedFromChatID int64 `json:"forwarded_from_chat_id,omitempty"`
	ForwardedFromMessageID int64 `json:"forwarded_from_message_id,omitempty"`
	ForwardedFromUsername string `json:"forwarded_from_username,omitempty"`
}

func (s *Store) CreateDirectChat(ctx context.Context, me, peer int64) (int64, error) {
    tx, err := s.Pool.Begin(ctx)
    if err != nil { return 0, err }
    defer tx.Rollback(ctx)
    var id int64
    err = tx.QueryRow(ctx, `SELECT c.id FROM chats c
        JOIN chat_members a ON a.chat_id=c.id AND a.user_id=$1
        JOIN chat_members b ON b.chat_id=c.id AND b.user_id=$2
        WHERE c.kind='direct'
        LIMIT 1`, me, peer).Scan(&id)
    if err == nil { return id, tx.Commit(ctx) }
    if !errors.Is(err, pgx.ErrNoRows) { return 0, err }
    if err := tx.QueryRow(ctx, `INSERT INTO chats(kind) VALUES('direct') RETURNING id`).Scan(&id); err != nil { return 0, err }
    if _, err := tx.Exec(ctx, `INSERT INTO chat_members(chat_id,user_id) VALUES($1,$2),($1,$3)`, id, me, peer); err != nil { return 0, err }
    return id, tx.Commit(ctx)
}

func (s *Store) ListChats(ctx context.Context, me int64) ([]ChatSummary, error) {
    rows, err := s.Pool.Query(ctx, `SELECT c.id,c.kind,c.title,c.description,c.is_public,COALESCE(c.username,''),mine.role,
        (SELECT count(*) FROM chat_members cm_count WHERE cm_count.chat_id=c.id),
        COALESCE(peer.id,0),COALESCE(peer.username,''),COALESCE(peer.display_name,''),COALESCE(peer.avatar_url,''),
        COALESCE(m.body,''),m.created_at,
        (SELECT count(*) FROM messages unread WHERE unread.chat_id=c.id AND unread.sender_id<>$1
          AND unread.id>(SELECT last_read_message_id FROM chat_members WHERE chat_id=c.id AND user_id=$1)
          AND unread.deleted_at IS NULL),
        c.created_at
        FROM chats c
        JOIN chat_members mine ON mine.chat_id=c.id AND mine.user_id=$1
        LEFT JOIN LATERAL (
          SELECT u.id,u.username,u.display_name,u.avatar_url
          FROM chat_members peer_member JOIN users u ON u.id=peer_member.user_id
          WHERE peer_member.chat_id=c.id AND peer_member.user_id<>$1
          ORDER BY peer_member.joined_at LIMIT 1
        ) peer ON TRUE
        LEFT JOIN LATERAL (SELECT body,created_at FROM messages WHERE chat_id=c.id AND deleted_at IS NULL ORDER BY id DESC LIMIT 1) m ON TRUE
        ORDER BY COALESCE(m.created_at,c.updated_at,c.created_at) DESC
        LIMIT 100`, me)
    if err != nil { return nil, err }
    defer rows.Close()
    out := []ChatSummary{}
    for rows.Next() {
        var c ChatSummary
        if err := rows.Scan(&c.ID,&c.Kind,&c.Title,&c.Description,&c.IsPublic,&c.Username,&c.Role,&c.MemberCount,
            &c.PeerID,&c.PeerUsername,&c.PeerDisplayName,&c.PeerAvatarURL,&c.LastMessage,&c.LastMessageAt,&c.UnreadCount,&c.CreatedAt); err != nil { return nil, err }
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
    rows,err := s.Pool.Query(ctx,`SELECT m.id,m.chat_id,m.sender_id,u.username,m.body,m.created_at,m.edited_at,(m.sender_id=$2),m.reply_to_message_id,COALESCE(reply.body,''),COALESCE(reply_user.username,''),
        COALESCE(m.media_id,''),COALESCE(media.content_type,''),COALESCE(media.original_name,''),COALESCE(media.size_bytes,0),COALESCE(m.forwarded_from_chat_id,0),COALESCE(m.forwarded_from_message_id,0),m.forwarded_from_username,
        COALESCE((
          SELECT jsonb_agg(jsonb_build_object('emoji',r.emoji,'count',r.reaction_count,'reacted',r.reacted) ORDER BY r.emoji)
          FROM (
            SELECT emoji,COUNT(*)::bigint AS reaction_count,BOOL_OR(user_id=$2) AS reacted
            FROM message_reactions WHERE chat_id=m.chat_id AND message_id=m.id GROUP BY emoji
          ) r
        ),'[]'::jsonb)
        FROM messages m JOIN users u ON u.id=m.sender_id
        LEFT JOIN messages reply ON reply.id=m.reply_to_message_id AND reply.chat_id=m.chat_id AND reply.deleted_at IS NULL
        LEFT JOIN users reply_user ON reply_user.id=reply.sender_id
        LEFT JOIN chat_media media ON media.media_id=m.media_id AND media.chat_id=m.chat_id
        WHERE m.chat_id=$1 AND m.deleted_at IS NULL AND ($3=0 OR m.id<$3)
        ORDER BY m.id DESC LIMIT $4`,chatID,viewerID,beforeID,limit)
    if err != nil { return nil,err }
    defer rows.Close()
    out:=[]ChatMessage{}
    for rows.Next() {
        var m ChatMessage
        var reactionsJSON []byte
        if err:=rows.Scan(&m.ID,&m.ChatID,&m.SenderID,&m.SenderUsername,&m.Body,&m.CreatedAt,&m.EditedAt,&m.IsMine,&m.ReplyToMessageID,&m.ReplyToBody,&m.ReplyToSenderUsername,&m.MediaID,&m.MediaMime,&m.MediaName,&m.MediaSize,&m.ForwardedFromChatID,&m.ForwardedFromMessageID,&m.ForwardedFromUsername,&reactionsJSON);err!=nil{return nil,err}
        if err:=json.Unmarshal(reactionsJSON,&m.Reactions);err!=nil{return nil,err}
        if m.MediaID!="" { m.MediaURL="/chats/"+strconv.FormatInt(m.ChatID,10)+"/media/"+m.MediaID }
        out=append(out,m)
    }
    for i,j:=0,len(out)-1;i<j;i,j=i+1,j-1 { out[i],out[j]=out[j],out[i] }
    if rows.Err()!=nil{return nil,rows.Err()}
    // Loading history is not itself a read acknowledgement. The client
    // explicitly advances its cursor after the latest messages are displayed.
    return out,nil
}

func (s *Store) SendChatMessage(ctx context.Context, chatID,senderID int64,body string, replyToID int64)(ChatMessage,error) {
    var m ChatMessage
    err:=s.Pool.QueryRow(ctx,`INSERT INTO messages(chat_id,sender_id,body,reply_to_message_id)
        SELECT $1,$2,$3,CASE WHEN $4=0 THEN NULL ELSE $4 END
        WHERE EXISTS(SELECT 1 FROM chat_members WHERE chat_id=$1 AND user_id=$2)
          AND EXISTS(SELECT 1 FROM chats c WHERE c.id=$1 AND
              (c.kind <> 'channel' OR EXISTS(
                  SELECT 1 FROM chat_members permission
                  WHERE permission.chat_id=c.id AND permission.user_id=$2
                    AND permission.role IN ('owner','admin')
              )))
          AND ($4=0 OR EXISTS(SELECT 1 FROM messages WHERE id=$4 AND chat_id=$1 AND deleted_at IS NULL))
        RETURNING id,chat_id,sender_id,body,created_at`,chatID,senderID,body,replyToID).
        Scan(&m.ID,&m.ChatID,&m.SenderID,&m.Body,&m.CreatedAt)
    if err!=nil{return m,norm(err)}
    m.IsMine=true
    if _,err=s.Pool.Exec(ctx,`UPDATE chats SET updated_at=NOW() WHERE id=$1`,chatID);err!=nil{return m,err}
    if err=s.Pool.QueryRow(ctx,`SELECT username FROM users WHERE id=$1`,senderID).Scan(&m.SenderUsername);err!=nil{return m,err}
    return m,nil
}


// DeleteChatMessage soft-deletes only messages sent by a current chat member.
// Both IDs are scoped in the UPDATE to prevent cross-chat message mutations.
func (s *Store) DeleteChatMessage(ctx context.Context, chatID, messageID, senderID int64) (string, error) {
	var storageName string
	err := s.Pool.QueryRow(ctx, `UPDATE messages AS m
		SET deleted_at = NOW()
		WHERE m.id = $1 AND m.chat_id = $2 AND m.sender_id = $3
		  AND m.deleted_at IS NULL
		  AND EXISTS (SELECT 1 FROM chat_members WHERE chat_id = $2 AND user_id = $3)
		RETURNING COALESCE((
			SELECT cm.storage_name FROM chat_media cm
			WHERE cm.media_id=m.media_id AND cm.chat_id=m.chat_id
		),'')`, messageID, chatID, senderID).Scan(&storageName)
	if errors.Is(err, pgx.ErrNoRows) {
		var alreadyDeleted bool
		checkErr := s.Pool.QueryRow(ctx, `SELECT EXISTS(
			SELECT 1 FROM messages
			WHERE id=$1 AND chat_id=$2 AND sender_id=$3 AND deleted_at IS NOT NULL
			  AND EXISTS(SELECT 1 FROM chat_members WHERE chat_id=$2 AND user_id=$3)
		)`, messageID, chatID, senderID).Scan(&alreadyDeleted)
		if checkErr != nil {
			return "", checkErr
		}
		if alreadyDeleted {
			return "", nil
		}
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	_, _ = s.Pool.Exec(ctx, `UPDATE chats SET pinned_message_id=NULL,pinned_by=NULL
		WHERE id=$1 AND pinned_message_id=$2`, chatID, messageID)
	return storageName, nil
}

 // ChatMemberIDs returns the exact member set for event fan-out. Callers must
 // never broadcast a chat event to users outside this membership list.
func (s *Store) ChatMemberIDs(ctx context.Context, chatID int64) ([]int64, error) {
	rows, err := s.Pool.Query(ctx, `SELECT user_id FROM chat_members WHERE chat_id=$1 ORDER BY user_id`, chatID)
	if err != nil { return nil, err }
	defer rows.Close()
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil { return nil, err }
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

 // MarkChatRead advances the caller's read cursor only within a chat they
 // currently belong to. A client cannot acknowledge a message from another chat.
func (s *Store) MarkChatRead(ctx context.Context, chatID, userID, lastReadMessageID int64) (int64, error) {
	var cursor int64
	err := s.Pool.QueryRow(ctx, `UPDATE chat_members cm
		SET last_read_message_id=GREATEST(cm.last_read_message_id,
			LEAST($3, COALESCE((SELECT max(id) FROM messages WHERE chat_id=$1 AND deleted_at IS NULL),0)))
		WHERE cm.chat_id=$1 AND cm.user_id=$2
		RETURNING last_read_message_id`, chatID, userID, lastReadMessageID).Scan(&cursor)
	return cursor, norm(err)
}

// ChatReadCursors returns the current read cursor for each member of a chat.
func (s *Store) ChatReadCursors(ctx context.Context, chatID, viewerID int64) (map[int64]int64, error) {
	var member bool
	if err := s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM chat_members WHERE chat_id=$1 AND user_id=$2)`, chatID, viewerID).Scan(&member); err != nil {
		return nil, err
	}
	if !member {
		return nil, ErrNotFound
	}
	rows, err := s.Pool.Query(ctx, `SELECT cm.user_id,cm.last_read_message_id
		FROM chat_members cm
		WHERE cm.chat_id=$1`, chatID)
	if err != nil { return nil, err }
	defer rows.Close()
	out := make(map[int64]int64)
	for rows.Next() {
		var userID, cursor int64
		if err := rows.Scan(&userID, &cursor); err != nil { return nil, err }
		out[userID] = cursor
	}
	return out, rows.Err()
}

func (s *Store) ChatIDsForUser(ctx context.Context, userID int64) ([]int64, error) {
	rows, err := s.Pool.Query(ctx, `SELECT chat_id FROM chat_members WHERE user_id=$1 ORDER BY chat_id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []int64{}
	for rows.Next() {
		var chatID int64
		if err := rows.Scan(&chatID); err != nil {
			return nil, err
		}
		ids = append(ids, chatID)
	}
	return ids, rows.Err()
}

 // ForwardChatMessage copies a visible source message into another chat while
 // retaining immutable source identifiers and a username snapshot. Both chat
 // memberships and destination channel posting permissions are checked in SQL.
func (s *Store) ForwardChatMessage(ctx context.Context, sourceChatID, sourceMessageID, destinationChatID, senderID int64) (ChatMessage, error) {
	var m ChatMessage
	err := s.Pool.QueryRow(ctx, `INSERT INTO messages(
		chat_id,sender_id,body,forwarded_from_chat_id,forwarded_from_message_id,forwarded_from_username
	)
	SELECT destination.id,$4,source.body,source.chat_id,source.id,source_user.username
	FROM chats destination
	JOIN chat_members destination_member ON destination_member.chat_id=destination.id AND destination_member.user_id=$4
	JOIN messages source ON source.chat_id=$1 AND source.id=$2 AND source.deleted_at IS NULL AND source.media_id IS NULL
	JOIN users source_user ON source_user.id=source.sender_id
	WHERE destination.id=$3
	  AND EXISTS(SELECT 1 FROM chat_members source_member WHERE source_member.chat_id=$1 AND source_member.user_id=$4)
	  AND (destination.kind <> 'channel' OR destination_member.role IN ('owner','admin'))
	RETURNING id,chat_id,sender_id,body,created_at,forwarded_from_chat_id,forwarded_from_message_id,forwarded_from_username`,
		sourceChatID, sourceMessageID, destinationChatID, senderID,
	).Scan(&m.ID,&m.ChatID,&m.SenderID,&m.Body,&m.CreatedAt,&m.ForwardedFromChatID,&m.ForwardedFromMessageID,&m.ForwardedFromUsername)
	if err != nil { return m, norm(err) }
	m.IsMine = true
	if err = s.Pool.QueryRow(ctx, `SELECT username FROM users WHERE id=$1`, senderID).Scan(&m.SenderUsername); err != nil { return m, err }
	if _, err = s.Pool.Exec(ctx, `UPDATE chats SET updated_at=NOW() WHERE id=$1`, destinationChatID); err != nil { return m, err }
	return m, nil
}

// PinChatMessage pins one message per chat. In community chats only owners and
// admins may pin; either participant may pin in a direct conversation.
func (s *Store) PinChatMessage(ctx context.Context, chatID, messageID, actorID int64) error {
	tag, err := s.Pool.Exec(ctx, `UPDATE chats c SET pinned_message_id=$2,pinned_by=$3
		WHERE c.id=$1
		  AND EXISTS(SELECT 1 FROM messages m WHERE m.id=$2 AND m.chat_id=c.id AND m.deleted_at IS NULL)
		  AND EXISTS(SELECT 1 FROM chat_members cm WHERE cm.chat_id=c.id AND cm.user_id=$3
		    AND (c.kind='direct' OR cm.role IN ('owner','admin')))`, chatID, messageID, actorID)
	if err != nil { return err }
	if tag.RowsAffected()==0 { return ErrNotFound }
	return nil
}

func (s *Store) UnpinChatMessage(ctx context.Context, chatID, actorID int64) error {
	tag, err := s.Pool.Exec(ctx, `UPDATE chats c SET pinned_message_id=NULL,pinned_by=NULL
		WHERE c.id=$1 AND c.pinned_message_id IS NOT NULL
		  AND EXISTS(SELECT 1 FROM chat_members cm WHERE cm.chat_id=c.id AND cm.user_id=$2
		    AND (c.kind='direct' OR cm.role IN ('owner','admin')))`, chatID, actorID)
	if err != nil { return err }
	if tag.RowsAffected()==0 { return ErrNotFound }
	return nil
}

func (s *Store) PinnedChatMessage(ctx context.Context, chatID, viewerID int64) (ChatMessage, error) {
	var m ChatMessage
	err := s.Pool.QueryRow(ctx, `SELECT m.id,m.chat_id,m.sender_id,u.username,m.body,m.created_at,(m.sender_id=$2),
		COALESCE(m.forwarded_from_chat_id,0),COALESCE(m.forwarded_from_message_id,0),m.forwarded_from_username
		FROM chats c
		JOIN chat_members cm ON cm.chat_id=c.id AND cm.user_id=$2
		JOIN messages m ON m.id=c.pinned_message_id AND m.chat_id=c.id AND m.deleted_at IS NULL
		JOIN users u ON u.id=m.sender_id WHERE c.id=$1`, chatID, viewerID).
		Scan(&m.ID,&m.ChatID,&m.SenderID,&m.SenderUsername,&m.Body,&m.CreatedAt,&m.IsMine,
			&m.ForwardedFromChatID,&m.ForwardedFromMessageID,&m.ForwardedFromUsername)
	if err != nil { return m, norm(err) }
	return m, nil
}
