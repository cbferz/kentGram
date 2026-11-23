-- Durable per-chat event log. Events are committed atomically with the
-- message/chat mutation that produced them. PostgreSQL NOTIFY is only a wake-up
-- signal; clients always recover from this table using their last cursor.
CREATE TABLE IF NOT EXISTS realtime_events (
    id BIGSERIAL PRIMARY KEY,
    chat_id BIGINT NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    actor_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    event_type VARCHAR(64) NOT NULL,
    payload JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS realtime_events_chat_id_idx
    ON realtime_events(chat_id, id);
CREATE INDEX IF NOT EXISTS realtime_events_created_at_idx
    ON realtime_events(created_at);
