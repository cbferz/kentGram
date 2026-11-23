-- Message interactions: idempotent sends, forwarding, pins and reactions.
-- Additive only: no existing column, row or constraint is changed or dropped.

-- Client-generated idempotency key. A retried send with the same key from the
-- same sender returns the original message instead of creating a duplicate.
ALTER TABLE messages
    ADD COLUMN IF NOT EXISTS client_id VARCHAR(64);

CREATE UNIQUE INDEX IF NOT EXISTS messages_sender_client_idx
    ON messages (sender_id, client_id)
    WHERE client_id IS NOT NULL;

-- Forwarded copies keep a snapshot of the original author. The source message
-- id is informational only (no FK) so deleting the original never breaks the copy.
ALTER TABLE messages
    ADD COLUMN IF NOT EXISTS forwarded_from_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE messages
    ADD COLUMN IF NOT EXISTS forwarded_from_message_id BIGINT;

-- Pinned messages. The composite FK guarantees a pin always points at a
-- message that belongs to the same chat.
CREATE TABLE IF NOT EXISTS pinned_messages (
    chat_id    BIGINT NOT NULL,
    message_id BIGINT NOT NULL,
    pinned_by  BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    pinned_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (chat_id, message_id),
    FOREIGN KEY (message_id, chat_id) REFERENCES messages (id, chat_id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS pinned_messages_chat_idx
    ON pinned_messages (chat_id, pinned_at DESC);

-- Reactions. One row per (message, user, emoji); the per-user and per-message
-- limits are enforced in the store inside a transaction that locks the message.
CREATE TABLE IF NOT EXISTS message_reactions (
    message_id BIGINT NOT NULL,
    chat_id    BIGINT NOT NULL,
    user_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    emoji      VARCHAR(32) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (message_id, user_id, emoji),
    FOREIGN KEY (message_id, chat_id) REFERENCES messages (id, chat_id) ON DELETE CASCADE,
    CHECK (char_length(emoji) BETWEEN 1 AND 32)
);
CREATE INDEX IF NOT EXISTS message_reactions_user_idx
    ON message_reactions (message_id, user_id, created_at);
