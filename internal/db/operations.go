package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/buildtall-systems/eggbot/internal/fsm"
)

var orderSM = fsm.NewOrderStateMachine()

// ErrInsufficientInventory indicates not enough eggs available.
var ErrInsufficientInventory = errors.New("insufficient inventory")

// ErrCustomerNotFound indicates customer does not exist.
var ErrCustomerNotFound = errors.New("customer not found")

// ErrOrderNotFound indicates order does not exist.
var ErrOrderNotFound = errors.New("order not found")

// ErrCustomerExists indicates customer already registered.
var ErrCustomerExists = errors.New("customer already exists")

// ErrOrderNotPending indicates the order cannot be modified because it's not pending.
var ErrOrderNotPending = errors.New("order is not pending")

// ErrInvalidStateTransition indicates an invalid order state transition was attempted.
var ErrInvalidStateTransition = errors.New("invalid order state transition")

// Customer represents a registered customer.
type Customer struct {
	ID        int64
	Npub      string
	Name      sql.NullString
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Order represents an egg order.
type Order struct {
	ID         int64
	CustomerID int64
	Quantity   int
	TotalSats  int64
	Status     string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// OrderWithCustomer represents an order with customer info (for admin listing).
type OrderWithCustomer struct {
	ID           int64
	CustomerNpub string
	Quantity     int
	TotalSats    int64
	Status       string
	CreatedAt    time.Time
}

// Transaction represents a zap payment record.
type Transaction struct {
	ID         int64
	OrderID    sql.NullInt64
	ZapEventID string
	AmountSats int64
	SenderNpub string
	CreatedAt  time.Time
}

// InventoryNotification represents a customer's notification subscription.
type InventoryNotification struct {
	ID            int64
	CustomerID    int64
	ThresholdEggs int
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// InventoryNotificationWithCustomer includes customer npub for sending DMs.
type InventoryNotificationWithCustomer struct {
	InventoryNotification
	CustomerNpub string
}

// GetInventory returns the current egg count.
func (db *DB) GetInventory(ctx context.Context) (int, error) {
	var count int
	err := db.QueryRowContext(ctx, `SELECT eggs_available FROM inventory WHERE id = 1`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("querying inventory: %w", err)
	}
	return count, nil
}

// AddEggs increments the inventory by count.
func (db *DB) AddEggs(ctx context.Context, count int) error {
	_, err := db.ExecContext(ctx, `
		UPDATE inventory
		SET eggs_available = eggs_available + ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = 1
	`, count)
	if err != nil {
		return fmt.Errorf("adding eggs: %w", err)
	}
	return nil
}

// SetInventory sets the inventory to an exact count.
func (db *DB) SetInventory(ctx context.Context, count int) error {
	_, err := db.ExecContext(ctx, `
		UPDATE inventory
		SET eggs_available = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = 1
	`, count)
	if err != nil {
		return fmt.Errorf("setting inventory: %w", err)
	}
	return nil
}

// DeductEggs decrements the inventory by count. Returns ErrInsufficientInventory if not enough.
func (db *DB) DeductEggs(ctx context.Context, count int) error {
	result, err := db.ExecContext(ctx, `
		UPDATE inventory
		SET eggs_available = eggs_available - ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = 1 AND eggs_available >= ?
	`, count, count)
	if err != nil {
		return fmt.Errorf("deducting eggs: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if rows == 0 {
		return ErrInsufficientInventory
	}
	return nil
}

// GetReservedEggs returns the total eggs in pending (unpaid) orders.
func (db *DB) GetReservedEggs(ctx context.Context) (int, error) {
	var count int
	err := db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(quantity), 0) FROM orders WHERE status = 'pending'
	`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("querying reserved eggs: %w", err)
	}
	return count, nil
}

// GetSoldEggs returns the total eggs in paid orders awaiting delivery.
func (db *DB) GetSoldEggs(ctx context.Context) (int, error) {
	var count int
	err := db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(quantity), 0) FROM orders WHERE status = 'paid'
	`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("querying sold eggs: %w", err)
	}
	return count, nil
}

// GetCustomerByNpub returns a customer by their npub.
func (db *DB) GetCustomerByNpub(ctx context.Context, npub string) (*Customer, error) {
	var c Customer
	err := db.QueryRowContext(ctx, `
		SELECT id, npub, name, created_at, updated_at
		FROM customers WHERE npub = ?
	`, npub).Scan(&c.ID, &c.Npub, &c.Name, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrCustomerNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying customer: %w", err)
	}
	return &c, nil
}

// GetCustomerByID returns a customer by their ID.
func (db *DB) GetCustomerByID(ctx context.Context, id int64) (*Customer, error) {
	var c Customer
	err := db.QueryRowContext(ctx, `
		SELECT id, npub, name, created_at, updated_at
		FROM customers WHERE id = ?
	`, id).Scan(&c.ID, &c.Npub, &c.Name, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrCustomerNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying customer: %w", err)
	}
	return &c, nil
}

// CreateCustomer registers a new customer.
func (db *DB) CreateCustomer(ctx context.Context, npub string) (*Customer, error) {
	result, err := db.ExecContext(ctx, `
		INSERT INTO customers (npub) VALUES (?)
	`, npub)
	if err != nil {
		// Check for unique constraint violation
		if isUniqueViolation(err) {
			return nil, ErrCustomerExists
		}
		return nil, fmt.Errorf("creating customer: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("getting customer id: %w", err)
	}

	return &Customer{ID: id, Npub: npub}, nil
}

// RemoveCustomer deletes a customer by npub.
func (db *DB) RemoveCustomer(ctx context.Context, npub string) error {
	result, err := db.ExecContext(ctx, `DELETE FROM customers WHERE npub = ?`, npub)
	if err != nil {
		return fmt.Errorf("removing customer: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if rows == 0 {
		return ErrCustomerNotFound
	}
	return nil
}

// ListCustomers returns all registered customers.
func (db *DB) ListCustomers(ctx context.Context) ([]Customer, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, npub, name, created_at, updated_at
		FROM customers ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("querying customers: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var customers []Customer
	for rows.Next() {
		var c Customer
		if err := rows.Scan(&c.ID, &c.Npub, &c.Name, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning customer: %w", err)
		}
		customers = append(customers, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating customers: %w", err)
	}
	return customers, nil
}

// CreateOrder creates a new order for a customer and reserves inventory atomically.
// Inventory is deducted at order time (reservation model). Returns ErrInsufficientInventory
// if not enough eggs are available.
func (db *DB) CreateOrder(ctx context.Context, customerID int64, quantity int, totalSats int64) (*Order, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Reserve inventory atomically
	result, err := tx.ExecContext(ctx, `
		UPDATE inventory
		SET eggs_available = eggs_available - ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = 1 AND eggs_available >= ?
	`, quantity, quantity)
	if err != nil {
		return nil, fmt.Errorf("reserving inventory: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("checking rows affected: %w", err)
	}
	if rows == 0 {
		return nil, ErrInsufficientInventory
	}

	// Create the order
	result, err = tx.ExecContext(ctx, `
		INSERT INTO orders (customer_id, quantity, total_sats, status)
		VALUES (?, ?, ?, 'pending')
	`, customerID, quantity, totalSats)
	if err != nil {
		return nil, fmt.Errorf("creating order: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("getting order id: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing transaction: %w", err)
	}

	return &Order{
		ID:         id,
		CustomerID: customerID,
		Quantity:   quantity,
		TotalSats:  totalSats,
		Status:     "pending",
	}, nil
}

// GetOrderByID returns an order by ID.
func (db *DB) GetOrderByID(ctx context.Context, orderID int64) (*Order, error) {
	var o Order
	err := db.QueryRowContext(ctx, `
		SELECT id, customer_id, quantity, total_sats, status, created_at, updated_at
		FROM orders WHERE id = ?
	`, orderID).Scan(&o.ID, &o.CustomerID, &o.Quantity, &o.TotalSats, &o.Status, &o.CreatedAt, &o.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrOrderNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying order: %w", err)
	}
	return &o, nil
}

// GetCustomerOrders returns orders for a customer, most recent first.
func (db *DB) GetCustomerOrders(ctx context.Context, customerID int64, limit int) ([]Order, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, customer_id, quantity, total_sats, status, created_at, updated_at
		FROM orders WHERE customer_id = ? ORDER BY created_at DESC LIMIT ?
	`, customerID, limit)
	if err != nil {
		return nil, fmt.Errorf("querying orders: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var orders []Order
	for rows.Next() {
		var o Order
		if err := rows.Scan(&o.ID, &o.CustomerID, &o.Quantity, &o.TotalSats, &o.Status, &o.CreatedAt, &o.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning order: %w", err)
		}
		orders = append(orders, o)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating orders: %w", err)
	}
	return orders, nil
}

// GetPendingOrdersByCustomer returns pending orders for a customer.
func (db *DB) GetPendingOrdersByCustomer(ctx context.Context, customerID int64) ([]Order, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, customer_id, quantity, total_sats, status, created_at, updated_at
		FROM orders WHERE customer_id = ? AND status = 'pending' ORDER BY created_at DESC
	`, customerID)
	if err != nil {
		return nil, fmt.Errorf("querying pending orders: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var orders []Order
	for rows.Next() {
		var o Order
		if err := rows.Scan(&o.ID, &o.CustomerID, &o.Quantity, &o.TotalSats, &o.Status, &o.CreatedAt, &o.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning order: %w", err)
		}
		orders = append(orders, o)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating orders: %w", err)
	}
	return orders, nil
}

// GetAllOrders returns all orders with customer info for admin visibility.
// Returns most recent first, limited by the provided count.
func (db *DB) GetAllOrders(ctx context.Context, limit int) ([]OrderWithCustomer, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT o.id, c.npub, o.quantity, o.total_sats, o.status, o.created_at
		FROM orders o
		JOIN customers c ON o.customer_id = c.id
		ORDER BY o.created_at DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("querying all orders: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var orders []OrderWithCustomer
	for rows.Next() {
		var o OrderWithCustomer
		if err := rows.Scan(&o.ID, &o.CustomerNpub, &o.Quantity, &o.TotalSats, &o.Status, &o.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning order: %w", err)
		}
		orders = append(orders, o)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating orders: %w", err)
	}
	return orders, nil
}

// GetPaidOrdersByCustomer returns paid orders for a customer (ready for delivery).
func (db *DB) GetPaidOrdersByCustomer(ctx context.Context, customerID int64) ([]Order, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, customer_id, quantity, total_sats, status, created_at, updated_at
		FROM orders WHERE customer_id = ? AND status = 'paid' ORDER BY created_at ASC
	`, customerID)
	if err != nil {
		return nil, fmt.Errorf("querying paid orders: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var orders []Order
	for rows.Next() {
		var o Order
		if err := rows.Scan(&o.ID, &o.CustomerID, &o.Quantity, &o.TotalSats, &o.Status, &o.CreatedAt, &o.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning order: %w", err)
		}
		orders = append(orders, o)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating orders: %w", err)
	}
	return orders, nil
}

// CancelOrder cancels a pending order and restores the reserved inventory.
// Returns ErrOrderNotPending if the order is not in 'pending' status.
// Only pending orders can be cancelled. Uses FSM validation.
func (db *DB) CancelOrder(ctx context.Context, orderID int64) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var quantity int
	var status string
	err = tx.QueryRowContext(ctx, `SELECT quantity, status FROM orders WHERE id = ?`, orderID).Scan(&quantity, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrOrderNotFound
	}
	if err != nil {
		return fmt.Errorf("querying order: %w", err)
	}

	if !orderSM.CanTransition(status, fsm.OrderEventCancel) {
		return ErrOrderNotPending
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE inventory
		SET eggs_available = eggs_available + ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = 1
	`, quantity)
	if err != nil {
		return fmt.Errorf("restoring inventory: %w", err)
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE orders SET status = 'cancelled', updated_at = CURRENT_TIMESTAMP WHERE id = ?
	`, orderID)
	if err != nil {
		return fmt.Errorf("cancelling order: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}
	return nil
}

// UpdateOrderStatus updates the status of an order with FSM validation.
// Only valid state transitions are permitted.
func (db *DB) UpdateOrderStatus(ctx context.Context, orderID int64, newStatus string) error {
	order, err := db.GetOrderByID(ctx, orderID)
	if err != nil {
		return err
	}

	event := inferOrderEvent(order.Status, newStatus)
	if event == "" {
		return fmt.Errorf("%w: %s -> %s", ErrInvalidStateTransition, order.Status, newStatus)
	}

	if _, err := orderSM.Transition(ctx, order.Status, event); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidStateTransition, err)
	}

	result, err := db.ExecContext(ctx, `
		UPDATE orders SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?
	`, newStatus, orderID)
	if err != nil {
		return fmt.Errorf("updating order status: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if rows == 0 {
		return ErrOrderNotFound
	}
	return nil
}

func inferOrderEvent(from, to string) string {
	transitions := map[string]map[string]string{
		fsm.OrderStatePending: {
			fsm.OrderStatePaid:      fsm.OrderEventPay,
			fsm.OrderStateCancelled: fsm.OrderEventCancel,
		},
		fsm.OrderStatePaid: {
			fsm.OrderStateFulfilled: fsm.OrderEventFulfill,
		},
	}
	if events, ok := transitions[from]; ok {
		return events[to]
	}
	return ""
}

// FulfillOrder marks an order as fulfilled. Inventory was already reserved at order time,
// so no inventory deduction occurs here. Uses FSM validation and atomic WHERE clause
// to prevent race conditions.
func (db *DB) FulfillOrder(ctx context.Context, orderID int64) error {
	order, err := db.GetOrderByID(ctx, orderID)
	if err != nil {
		return err
	}

	if !orderSM.CanTransition(order.Status, fsm.OrderEventFulfill) {
		return fmt.Errorf("%w: cannot fulfill order in %s state", ErrInvalidStateTransition, order.Status)
	}

	result, err := db.ExecContext(ctx, `
		UPDATE orders SET status = 'fulfilled', updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND status = 'paid'
	`, orderID)
	if err != nil {
		return fmt.Errorf("updating order: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("%w: order state changed concurrently", ErrInvalidStateTransition)
	}
	return nil
}

// RecordTransaction records a zap payment.
func (db *DB) RecordTransaction(ctx context.Context, orderID *int64, zapEventID string, amountSats int64, senderNpub string) (*Transaction, error) {
	var orderIDVal sql.NullInt64
	if orderID != nil {
		orderIDVal = sql.NullInt64{Int64: *orderID, Valid: true}
	}

	result, err := db.ExecContext(ctx, `
		INSERT INTO transactions (order_id, zap_event_id, amount_sats, sender_npub)
		VALUES (?, ?, ?, ?)
	`, orderIDVal, zapEventID, amountSats, senderNpub)
	if err != nil {
		return nil, fmt.Errorf("recording transaction: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("getting transaction id: %w", err)
	}

	return &Transaction{
		ID:         id,
		OrderID:    orderIDVal,
		ZapEventID: zapEventID,
		AmountSats: amountSats,
		SenderNpub: senderNpub,
	}, nil
}

// GetCustomerBalance returns total sats received from a customer.
func (db *DB) GetCustomerBalance(ctx context.Context, npub string) (int64, error) {
	var balance sql.NullInt64
	err := db.QueryRowContext(ctx, `
		SELECT SUM(amount_sats) FROM transactions WHERE sender_npub = ?
	`, npub).Scan(&balance)
	if err != nil {
		return 0, fmt.Errorf("querying balance: %w", err)
	}
	if !balance.Valid {
		return 0, nil
	}
	return balance.Int64, nil
}

// GetCustomerSpent returns total sats spent by a customer on fulfilled orders.
func (db *DB) GetCustomerSpent(ctx context.Context, customerID int64) (int64, error) {
	var spent sql.NullInt64
	err := db.QueryRowContext(ctx, `
		SELECT SUM(total_sats) FROM orders WHERE customer_id = ? AND status = 'fulfilled'
	`, customerID).Scan(&spent)
	if err != nil {
		return 0, fmt.Errorf("querying spent: %w", err)
	}
	if !spent.Valid {
		return 0, nil
	}
	return spent.Int64, nil
}

// GetTotalSales returns total sats from all fulfilled orders.
func (db *DB) GetTotalSales(ctx context.Context) (int64, error) {
	var total sql.NullInt64
	err := db.QueryRowContext(ctx, `
		SELECT SUM(total_sats) FROM orders WHERE status = 'fulfilled'
	`).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("querying total sales: %w", err)
	}
	if !total.Valid {
		return 0, nil
	}
	return total.Int64, nil
}

// UpsertInventoryNotification creates or updates a notification subscription.
// Uses INSERT OR REPLACE for upsert semantics (one subscription per customer).
func (db *DB) UpsertInventoryNotification(ctx context.Context, customerID int64, threshold int) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO inventory_notifications (customer_id, threshold_eggs)
		VALUES (?, ?)
		ON CONFLICT(customer_id) DO UPDATE SET
			threshold_eggs = excluded.threshold_eggs,
			updated_at = CURRENT_TIMESTAMP
	`, customerID, threshold)
	if err != nil {
		return fmt.Errorf("upserting inventory notification: %w", err)
	}
	return nil
}

// DeleteInventoryNotification removes a subscription by customer ID.
func (db *DB) DeleteInventoryNotification(ctx context.Context, customerID int64) error {
	_, err := db.ExecContext(ctx, `
		DELETE FROM inventory_notifications WHERE customer_id = ?
	`, customerID)
	if err != nil {
		return fmt.Errorf("deleting inventory notification: %w", err)
	}
	return nil
}

// GetInventoryNotification returns the subscription for a customer, or nil if none.
func (db *DB) GetInventoryNotification(ctx context.Context, customerID int64) (*InventoryNotification, error) {
	var n InventoryNotification
	err := db.QueryRowContext(ctx, `
		SELECT id, customer_id, threshold_eggs, created_at, updated_at
		FROM inventory_notifications WHERE customer_id = ?
	`, customerID).Scan(&n.ID, &n.CustomerID, &n.ThresholdEggs, &n.CreatedAt, &n.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying inventory notification: %w", err)
	}
	return &n, nil
}

// GetTriggeredNotifications returns subscriptions where threshold <= available.
// Joins with customers table to get npub for DM sending.
func (db *DB) GetTriggeredNotifications(ctx context.Context, available int) ([]InventoryNotificationWithCustomer, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT n.id, n.customer_id, n.threshold_eggs, n.created_at, n.updated_at, c.npub
		FROM inventory_notifications n
		JOIN customers c ON n.customer_id = c.id
		WHERE n.threshold_eggs <= ?
	`, available)
	if err != nil {
		return nil, fmt.Errorf("querying triggered notifications: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var notifications []InventoryNotificationWithCustomer
	for rows.Next() {
		var n InventoryNotificationWithCustomer
		if err := rows.Scan(&n.ID, &n.CustomerID, &n.ThresholdEggs, &n.CreatedAt, &n.UpdatedAt, &n.CustomerNpub); err != nil {
			return nil, fmt.Errorf("scanning notification: %w", err)
		}
		notifications = append(notifications, n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating notifications: %w", err)
	}
	return notifications, nil
}

// DeleteInventoryNotificationByID removes a subscription by ID (after sending notification).
func (db *DB) DeleteInventoryNotificationByID(ctx context.Context, id int64) error {
	_, err := db.ExecContext(ctx, `
		DELETE FROM inventory_notifications WHERE id = ?
	`, id)
	if err != nil {
		return fmt.Errorf("deleting inventory notification by id: %w", err)
	}
	return nil
}

// isUniqueViolation checks if the error is a unique constraint violation.
func isUniqueViolation(err error) bool {
	// SQLite unique constraint error contains "UNIQUE constraint failed"
	return err != nil && (contains(err.Error(), "UNIQUE constraint failed") || contains(err.Error(), "constraint failed"))
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
