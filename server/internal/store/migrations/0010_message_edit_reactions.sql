-- Message editing and per-user emoji reactions. Existing messages are preserved.
ALTER TABLE messages
    ADD COLUMN IF NOT EXISTS edited_at TIMESTAMPTZ;

CREATE TABLE IF NOT EXISTS message_reactions (
    message_id BIGINT NOT NULL,
    chat_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    emoji VARCHAR(16) NOT NULL CHECK (char_length(emoji) BETWEEN 1 AND 16),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (message_id, user_id, emoji),
    FOREIGN KEY (message_id, chat_id)
        REFERENCES messages(id, chat_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS message_reactions_chat_message_idx
    ON message_reactions(chat_id, message_id);
