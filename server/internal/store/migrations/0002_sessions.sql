-- Server-side sessions. One row per signed-in device. The refresh token itself
-- is never stored, only its SHA-256. prev_refresh_hash remembers the token that
-- was just rotated out so that replaying it can be detected as token theft.

CREATE TABLE IF NOT EXISTS sessions(
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    refresh_hash BYTEA NOT NULL UNIQUE,
    prev_refresh_hash BYTEA UNIQUE,
    device_name VARCHAR(64) NOT NULL DEFAULT '',
    user_agent VARCHAR(256) NOT NULL DEFAULT '',
    ip VARCHAR(64) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_used_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS sessions_user_active_idx ON sessions(user_id) WHERE revoked_at IS NULL;
