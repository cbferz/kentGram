CREATE TABLE chat_media (
    media_id VARCHAR(32) PRIMARY KEY,
    chat_id BIGINT NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    uploader_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    storage_name VARCHAR(64) NOT NULL UNIQUE,
    original_name VARCHAR(255) NOT NULL,
    content_type VARCHAR(100) NOT NULL,
    size_bytes BIGINT NOT NULL CHECK (size_bytes > 0 AND size_bytes <= 20971520),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (media_id, chat_id)
);

ALTER TABLE messages
    ADD COLUMN media_id VARCHAR(32);

ALTER TABLE messages
    ADD CONSTRAINT messages_media_same_chat_fk
    FOREIGN KEY (media_id, chat_id)
    REFERENCES chat_media(media_id, chat_id)
    ON DELETE RESTRICT;

CREATE INDEX chat_media_chat_idx ON chat_media(chat_id, created_at DESC);
