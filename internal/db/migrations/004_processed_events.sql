-- +goose Up
-- +goose StatementBegin

-- Processed events: idempotent event processing via INSERT OR IGNORE
-- Event ID is the deduplication key - same event from multiple relays processed once
CREATE TABLE IF NOT EXISTS processed_events (
    event_id TEXT PRIMARY KEY,
    kind INTEGER NOT NULL,
    created_at INTEGER NOT NULL,
    processed_at INTEGER NOT NULL DEFAULT (unixepoch())
);

-- Index for cleanup of old events (optional periodic maintenance)
CREATE INDEX IF NOT EXISTS idx_processed_events_processed_at ON processed_events(processed_at);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_processed_events_processed_at;
DROP TABLE IF EXISTS processed_events;
-- +goose StatementEnd
