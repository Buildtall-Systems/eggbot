-- +goose Up
-- +goose StatementBegin

-- Inventory notifications: one-shot subscription for inventory threshold alerts
-- Customer requests DM when inventory reaches threshold, subscription deleted after notification sent
CREATE TABLE IF NOT EXISTS inventory_notifications (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    customer_id INTEGER NOT NULL UNIQUE REFERENCES customers(id) ON DELETE CASCADE,
    threshold_eggs INTEGER NOT NULL CHECK (threshold_eggs IN (6, 12)),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Index for efficient threshold queries when inventory changes
CREATE INDEX IF NOT EXISTS idx_inventory_notifications_threshold ON inventory_notifications(threshold_eggs);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_inventory_notifications_threshold;
DROP TABLE IF EXISTS inventory_notifications;
-- +goose StatementEnd
