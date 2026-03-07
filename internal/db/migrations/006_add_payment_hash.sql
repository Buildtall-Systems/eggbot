-- +goose Up
-- +goose StatementBegin
ALTER TABLE orders ADD COLUMN payment_hash TEXT;
ALTER TABLE orders ADD COLUMN invoice_expires_at TIMESTAMP;
CREATE INDEX idx_orders_payment_hash ON orders(payment_hash);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_orders_payment_hash;
-- SQLite doesn't support DROP COLUMN before 3.35.0; this is best-effort
-- +goose StatementEnd
