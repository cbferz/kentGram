CREATE TABLE IF NOT EXISTS message_reactions (
    message_id BIGINT NOT NULL,
    chat_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    emoji VARCHAR(16) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (message_id, user_id, emoji),
    CONSTRAINT message_reactions_message_chat_fk
        FOREIGN KEY (message_id, chat_id)
        REFERENCES messages (id, chat_id)
        ON DELETE CASCADE,
    CONSTRAINT message_reactions_emoji_check
        CHECK (emoji IN ('👍','❤️','😂','😮','😢','🔥','👏','🎉'))
);
CREATE INDEX IF NOT EXISTS message_reactions_chat_message_idx
    ON message_reactions (chat_id, message_id);