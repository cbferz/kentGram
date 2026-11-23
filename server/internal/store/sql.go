package store

// SQL for messages and message interactions. Every statement scopes chat,
// message and acting user together so an identifier from one chat can never
// touch another chat's data.
//
// The constants are plain strings (no formatting) so each one can be extracted
// and executed against a real PostgreSQL in CI or locally.

const sqlMessageSelect = `SELECT m.id, m.chat_id, m.sender_id, u.username, m.body, m.created_at, (m.sender_id=$2),
        m.reply_to_message_id, COALESCE(reply.body,''), COALESCE(reply_user.username,''),
        m.edited_at, m.forwarded_from_user_id, COALESCE(fwd.username,''),
        EXISTS(SELECT 1 FROM pinned_messages p WHERE p.chat_id=m.chat_id AND p.message_id=m.id),
        COALESCE((SELECT jsonb_agg(jsonb_build_object('emoji',r.emoji,'count',r.cnt,'mine',r.mine) ORDER BY r.first_at, r.emoji)
            FROM (SELECT emoji, count(*) AS cnt, bool_or(user_id=$2) AS mine, min(created_at) AS first_at
                  FROM message_reactions WHERE message_id=m.id GROUP BY emoji) r), '[]'::jsonb)
    FROM messages m
    JOIN users u ON u.id=m.sender_id
    LEFT JOIN messages reply ON reply.id=m.reply_to_message_id AND reply.chat_id=m.chat_id AND reply.deleted_at IS NULL
    LEFT JOIN users reply_user ON reply_user.id=reply.sender_id
    LEFT JOIN users fwd ON fwd.id=m.forwarded_from_user_id`

// $1 chat, $2 viewer, $3 message
const sqlMessageByID = sqlMessageSelect + `
    WHERE m.chat_id=$1 AND m.id=$3 AND m.deleted_at IS NULL`

// $1 chat, $2 viewer, $3 before_id (0 = newest), $4 limit
const sqlMessageHistory = sqlMessageSelect + `
    WHERE m.chat_id=$1 AND m.deleted_at IS NULL AND ($3::bigint=0 OR m.id<$3)
    ORDER BY m.id DESC LIMIT $4`

// $1 chat, $2 viewer
const sqlPinnedMessages = sqlMessageSelect + `
    JOIN pinned_messages pin ON pin.chat_id=m.chat_id AND pin.message_id=m.id
    WHERE m.chat_id=$1 AND m.deleted_at IS NULL
    ORDER BY pin.pinned_at DESC, m.id DESC`

// $1 chat, $2 sender, $3 body, $4 reply_to (0 = none), $5 client_id (” = none)
const sqlSendMessage = `INSERT INTO messages(chat_id,sender_id,body,reply_to_message_id,client_id)
    SELECT $1,$2,$3,NULLIF($4::bigint,0),NULLIF($5::text,'')
    WHERE EXISTS(SELECT 1 FROM chat_members WHERE chat_id=$1 AND user_id=$2)
      AND ($4::bigint=0 OR EXISTS(SELECT 1 FROM messages WHERE id=$4 AND chat_id=$1 AND deleted_at IS NULL))
    ON CONFLICT (sender_id, client_id) WHERE client_id IS NOT NULL DO NOTHING
    RETURNING id`

// $1 sender, $2 client_id
const sqlMessageByClientID = `SELECT id, chat_id FROM messages WHERE sender_id=$1 AND client_id=$2`

// $1 chat
const sqlTouchChat = `UPDATE chats SET updated_at=NOW() WHERE id=$1`

// $1 message, $2 chat, $3 user, $4 body
const sqlEditMessage = `UPDATE messages SET body=$4, edited_at=NOW()
    WHERE id=$1 AND chat_id=$2 AND sender_id=$3 AND deleted_at IS NULL
      AND EXISTS(SELECT 1 FROM chat_members WHERE chat_id=$2 AND user_id=$3)
    RETURNING id`

// $1 message, $2 chat, $3 user. Also drops any pin on the deleted message.
const sqlDeleteMessage = `WITH d AS (
        UPDATE messages SET deleted_at=NOW()
        WHERE id=$1 AND chat_id=$2 AND sender_id=$3 AND deleted_at IS NULL
          AND EXISTS(SELECT 1 FROM chat_members WHERE chat_id=$2 AND user_id=$3)
        RETURNING id),
    p AS (DELETE FROM pinned_messages WHERE chat_id=$2 AND message_id IN (SELECT id FROM d) RETURNING 1)
    SELECT id FROM d`

// $1 source chat, $2 message, $3 user, $4 destination chat, $5 client_id (” = none)
const sqlForwardMessage = `INSERT INTO messages(chat_id,sender_id,body,forwarded_from_user_id,forwarded_from_message_id,client_id)
    SELECT $4,$3,src.body,COALESCE(src.forwarded_from_user_id,src.sender_id),COALESCE(src.forwarded_from_message_id,src.id),NULLIF($5::text,'')
    FROM messages src
    WHERE src.id=$2 AND src.chat_id=$1 AND src.deleted_at IS NULL
      AND EXISTS(SELECT 1 FROM chat_members WHERE chat_id=$1 AND user_id=$3)
      AND EXISTS(SELECT 1 FROM chat_members WHERE chat_id=$4 AND user_id=$3)
    ON CONFLICT (sender_id, client_id) WHERE client_id IS NOT NULL DO NOTHING
    RETURNING id`

// $1 chat, $2 message, $3 user. Returns whether the message was visible to the
// user (false = not a member / wrong chat / deleted).
const sqlPinMessage = `WITH m AS (
        SELECT id FROM messages
        WHERE id=$2 AND chat_id=$1 AND deleted_at IS NULL
          AND EXISTS(SELECT 1 FROM chat_members WHERE chat_id=$1 AND user_id=$3)),
    ins AS (
        INSERT INTO pinned_messages(chat_id,message_id,pinned_by)
        SELECT $1,id,$3 FROM m
        ON CONFLICT DO NOTHING RETURNING 1)
    SELECT EXISTS(SELECT 1 FROM m)`

// $1 chat, $2 message, $3 user. Returns whether the user is a chat member.
const sqlUnpinMessage = `WITH member AS (
        SELECT 1 FROM chat_members WHERE chat_id=$1 AND user_id=$3),
    del AS (
        DELETE FROM pinned_messages
        WHERE chat_id=$1 AND message_id=$2 AND EXISTS(SELECT 1 FROM member) RETURNING 1)
    SELECT EXISTS(SELECT 1 FROM member)`

// $1 message, $2 chat, $3 user. Locks the message row; no row means the user
// may not react (not a member, wrong chat, or deleted).
const sqlLockMessageForReaction = `SELECT 1 FROM messages
    WHERE id=$1 AND chat_id=$2 AND deleted_at IS NULL
      AND EXISTS(SELECT 1 FROM chat_members WHERE chat_id=$2 AND user_id=$3)
    FOR UPDATE`

// $1 message, $2 chat, $3 user, $4 emoji
const sqlAddReaction = `INSERT INTO message_reactions(message_id,chat_id,user_id,emoji)
    VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`

// $1 message
const sqlCountReactionTypes = `SELECT count(DISTINCT emoji) FROM message_reactions WHERE message_id=$1`

// $1 message, $2 user, $3 how many of the user's newest reactions to keep
const sqlEvictOldReactions = `DELETE FROM message_reactions
    WHERE message_id=$1 AND user_id=$2 AND emoji IN (
        SELECT emoji FROM message_reactions
        WHERE message_id=$1 AND user_id=$2
        ORDER BY created_at DESC, emoji
        OFFSET $3)`

// $1 message, $2 user, $3 emoji
const sqlRemoveReaction = `DELETE FROM message_reactions WHERE message_id=$1 AND user_id=$2 AND emoji=$3`

// $1 message, $2 acting user
const sqlReactionTotals = `SELECT emoji, count(*), bool_or(user_id=$2)
    FROM message_reactions WHERE message_id=$1
    GROUP BY emoji ORDER BY min(created_at), emoji`

// $1 chat, $2 user, $3 requested marker
const sqlMarkRead = `UPDATE chat_members
    SET last_read_message_id=GREATEST(last_read_message_id, LEAST($3::bigint, COALESCE((SELECT max(id) FROM messages WHERE chat_id=$1),0)))
    WHERE chat_id=$1 AND user_id=$2
    RETURNING last_read_message_id`

// $1 chat, $2 viewer
const sqlPeerLastRead = `SELECT COALESCE(max(last_read_message_id),0) FROM chat_members WHERE chat_id=$1 AND user_id<>$2`

// $1 chat
const sqlChatMemberIDs = `SELECT user_id FROM chat_members WHERE chat_id=$1`

// $1 user
const sqlPeerIDs = `SELECT DISTINCT other.user_id FROM chat_members mine
    JOIN chat_members other ON other.chat_id=mine.chat_id AND other.user_id<>mine.user_id
    WHERE mine.user_id=$1`
