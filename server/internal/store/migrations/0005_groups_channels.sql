-- Extend direct chats into groups and broadcast channels without rewriting
-- already-applied migrations. Existing direct conversations remain unchanged.
ALTER TABLE chats DROP CONSTRAINT IF EXISTS chats_kind_check;
ALTER TABLE chats
    ADD CONSTRAINT chats_kind_check
    CHECK (kind IN ('direct', 'group', 'channel'));

ALTER TABLE chats
    ADD COLUMN title VARCHAR(80) NOT NULL DEFAULT '',
    ADD COLUMN description VARCHAR(500) NOT NULL DEFAULT '',
    ADD COLUMN is_public BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN username VARCHAR(32);

CREATE UNIQUE INDEX chats_username_lower_unique
    ON chats (lower(username))
    WHERE username IS NOT NULL;

ALTER TABLE chat_members
    ADD COLUMN role VARCHAR(16) NOT NULL DEFAULT 'member'
    CHECK (role IN ('owner', 'admin', 'member'));

CREATE INDEX chat_members_role_idx ON chat_members(chat_id, role);
