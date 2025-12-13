-- +goose Up
-- +goose StatementBegin

-- Inventory: tracks available egg count
CREATE TABLE IF NOT EXISTS inventory (
    id INTEGER PRIMARY KEY,
    eggs_available INTEGER NOT NULL DEFAULT 0,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Seed initial inventory row
INSERT INTO inventory (id, eggs_available) VALUES (1, 0);

-- Customers: registered customers by npub
CREATE TABLE IF NOT EXISTS customers (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    npub TEXT NOT NULL UNIQUE,
    name TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_customers_npub ON customers(npub);

-- Orders: customer orders with status tracking
CREATE TABLE IF NOT EXISTS orders (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    customer_id INTEGER NOT NULL REFERENCES customers(id),
    quantity INTEGER NOT NULL,  -- number of eggs
    total_sats INTEGER NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',  -- pending, paid, fulfilled, cancelled
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_orders_customer_id ON orders(customer_id);
CREATE INDEX idx_orders_status ON orders(status);

-- Transactions: zap payment records
CREATE TABLE IF NOT EXISTS transactions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    order_id INTEGER REFERENCES orders(id),
    zap_event_id TEXT NOT NULL UNIQUE,  -- NIP-57 zap receipt event ID
    amount_sats INTEGER NOT NULL,
    sender_npub TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_transactions_order_id ON transactions(order_id);
CREATE INDEX idx_transactions_zap_event_id ON transactions(zap_event_id);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS transactions;
DROP TABLE IF EXISTS orders;
DROP TABLE IF EXISTS customers;
DROP TABLE IF EXISTS inventory;
-- +goose StatementEnd
