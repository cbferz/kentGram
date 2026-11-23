ALTER TABLE messages
    ADD COLUMN forwarded_from_chat_id BIGINT,
    ADD COLUMN forwarded_from_message_id BIGINT,
    ADD COLUMN forwarded_from_username VARCHAR(32) NOT NULL DEFAULT '';

ALTER TABLE messages
    ADD CONSTRAINT messages_forward_source_same_chat_fk
    FOREIGN KEY (forwarded_from_message_id, forwarded_from_chat_id)
    REFERENCES messages (id, chat_id)
    ON DELETE RESTRICT;

CREATE INDEX messages_forward_source_idx
    ON messages (forwarded_from_message_id)
    WHERE forwarded_from_message_id IS NOT NULL;

ALTER TABLE chats
    ADD COLUMN pinned_message_id BIGINT,
    ADD COLUMN pinned_by BIGINT REFERENCES users(id) ON DELETE SET NULL;

ALTER TABLE chats
    ADD CONSTRAINT chats_pinned_message_same_chat_fk
    FOREIGN KEY (pinned_message_id, id)
    REFERENCES messages (id, chat_id)
    ON DELETE RESTRICT;
