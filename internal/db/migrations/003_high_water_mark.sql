-- +goose Up
-- +goose StatementBegin

-- High water mark: tracks the most recent event timestamp processed
-- Used to filter subscription on startup to avoid reprocessing old messages
CREATE TABLE IF NOT EXISTS high_water_mark (
    id INTEGER PRIMARY KEY,
    last_event_at INTEGER NOT NULL DEFAULT 0,  -- Unix timestamp of most recent processed event
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Seed initial row with zero (process all historical events on first run)
INSERT INTO high_water_mark (id, last_event_at) VALUES (1, 0);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS high_water_mark;
-- +goose StatementEnd
