ALTER TABLE messages
    ADD COLUMN reply_to_message_id BIGINT;

ALTER TABLE messages
    ADD CONSTRAINT messages_id_chat_unique UNIQUE (id, chat_id);

ALTER TABLE messages
    ADD CONSTRAINT messages_reply_same_chat_fk
    FOREIGN KEY (reply_to_message_id, chat_id)
    REFERENCES messages (id, chat_id)
    ON DELETE RESTRICT;

CREATE INDEX messages_reply_to_idx
    ON messages (reply_to_message_id)
    WHERE reply_to_message_id IS NOT NULL;
