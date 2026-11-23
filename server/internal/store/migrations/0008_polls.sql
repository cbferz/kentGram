CREATE TABLE polls (
    id BIGSERIAL PRIMARY KEY,
    chat_id BIGINT NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    creator_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    question VARCHAR(300) NOT NULL CHECK (length(btrim(question)) BETWEEN 1 AND 300),
    multiple_choice BOOLEAN NOT NULL DEFAULT FALSE,
    anonymous BOOLEAN NOT NULL DEFAULT TRUE,
    is_closed BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (id, chat_id)
);

CREATE TABLE poll_options (
    id BIGSERIAL PRIMARY KEY,
    poll_id BIGINT NOT NULL REFERENCES polls(id) ON DELETE CASCADE,
    option_text VARCHAR(100) NOT NULL CHECK (length(btrim(option_text)) BETWEEN 1 AND 100),
    position SMALLINT NOT NULL CHECK (position BETWEEN 0 AND 9),
    UNIQUE (id, poll_id),
    UNIQUE (poll_id, position)
);

CREATE TABLE poll_votes (
    poll_id BIGINT NOT NULL REFERENCES polls(id) ON DELETE CASCADE,
    option_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (poll_id, user_id, option_id),
    FOREIGN KEY (option_id, poll_id) REFERENCES poll_options(id, poll_id) ON DELETE CASCADE
);

CREATE INDEX poll_votes_option_idx ON poll_votes(poll_id, option_id);
CREATE INDEX polls_chat_idx ON polls(chat_id, created_at DESC);
