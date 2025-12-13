package db

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func setupTestDB(t *testing.T) *DB {
	t.Helper()

	sqlDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("opening test db: %v", err)
	}

	db := &DB{DB: sqlDB}

	// Run migrations
	if err := db.Migrate(); err != nil {
		_ = sqlDB.Close()
		t.Fatalf("migrating test db: %v", err)
	}

	t.Cleanup(func() { _ = db.Close() })

	return db
}

func TestInventory(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)

	// Initial inventory should be 0
	count, err := db.GetInventory(ctx)
	if err != nil {
		t.Fatalf("GetInventory: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0, got %d", count)
	}

	// Add eggs
	if err := db.AddEggs(ctx, 12); err != nil {
		t.Fatalf("AddEggs: %v", err)
	}

	count, err = db.GetInventory(ctx)
	if err != nil {
		t.Fatalf("GetInventory: %v", err)
	}
	if count != 12 {
		t.Errorf("expected 12, got %d", count)
	}

	// Deduct eggs
	if err := db.DeductEggs(ctx, 5); err != nil {
		t.Fatalf("DeductEggs: %v", err)
	}

	count, err = db.GetInventory(ctx)
	if err != nil {
		t.Fatalf("GetInventory: %v", err)
	}
	if count != 7 {
		t.Errorf("expected 7, got %d", count)
	}

	// Deduct more than available should fail
	err = db.DeductEggs(ctx, 10)
	if err != ErrInsufficientInventory {
		t.Errorf("expected ErrInsufficientInventory, got %v", err)
	}
}

func TestCustomerCRUD(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)

	npub := "npub1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqsutj2c5"

	// Customer should not exist
	_, err := db.GetCustomerByNpub(ctx, npub)
	if err != ErrCustomerNotFound {
		t.Errorf("expected ErrCustomerNotFound, got %v", err)
	}

	// Create customer
	c, err := db.CreateCustomer(ctx, npub)
	if err != nil {
		t.Fatalf("CreateCustomer: %v", err)
	}
	if c.Npub != npub {
		t.Errorf("expected npub %s, got %s", npub, c.Npub)
	}

	// Create duplicate should fail
	_, err = db.CreateCustomer(ctx, npub)
	if err != ErrCustomerExists {
		t.Errorf("expected ErrCustomerExists, got %v", err)
	}

	// Get customer
	c, err = db.GetCustomerByNpub(ctx, npub)
	if err != nil {
		t.Fatalf("GetCustomerByNpub: %v", err)
	}
	if c.Npub != npub {
		t.Errorf("expected npub %s, got %s", npub, c.Npub)
	}

	// List customers
	customers, err := db.ListCustomers(ctx)
	if err != nil {
		t.Fatalf("ListCustomers: %v", err)
	}
	if len(customers) != 1 {
		t.Errorf("expected 1 customer, got %d", len(customers))
	}

	// Remove customer
	if err := db.RemoveCustomer(ctx, npub); err != nil {
		t.Fatalf("RemoveCustomer: %v", err)
	}

	// Remove non-existent should fail
	err = db.RemoveCustomer(ctx, npub)
	if err != ErrCustomerNotFound {
		t.Errorf("expected ErrCustomerNotFound, got %v", err)
	}
}

func TestOrderOperations(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)

	// Create customer
	npub := "npub1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqsutj2c5"
	c, err := db.CreateCustomer(ctx, npub)
	if err != nil {
		t.Fatalf("CreateCustomer: %v", err)
	}

	// Create order
	order, err := db.CreateOrder(ctx, c.ID, 6, 3200)
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	if order.Quantity != 6 || order.TotalSats != 3200 {
		t.Errorf("unexpected order values: %+v", order)
	}
	if order.Status != "pending" {
		t.Errorf("expected status pending, got %s", order.Status)
	}

	// Get order by ID
	order, err = db.GetOrderByID(ctx, order.ID)
	if err != nil {
		t.Fatalf("GetOrderByID: %v", err)
	}

	// Get customer orders
	orders, err := db.GetCustomerOrders(ctx, c.ID, 10)
	if err != nil {
		t.Fatalf("GetCustomerOrders: %v", err)
	}
	if len(orders) != 1 {
		t.Errorf("expected 1 order, got %d", len(orders))
	}

	// Get pending orders
	pending, err := db.GetPendingOrdersByCustomer(ctx, c.ID)
	if err != nil {
		t.Fatalf("GetPendingOrdersByCustomer: %v", err)
	}
	if len(pending) != 1 {
		t.Errorf("expected 1 pending, got %d", len(pending))
	}

	// Update status
	if err := db.UpdateOrderStatus(ctx, order.ID, "paid"); err != nil {
		t.Fatalf("UpdateOrderStatus: %v", err)
	}

	order, _ = db.GetOrderByID(ctx, order.ID)
	if order.Status != "paid" {
		t.Errorf("expected status paid, got %s", order.Status)
	}
}

func TestFulfillOrder(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)

	// Create customer and order
	npub := "npub1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqsutj2c5"
	c, _ := db.CreateCustomer(ctx, npub)
	order, _ := db.CreateOrder(ctx, c.ID, 6, 3200)

	// Fulfill without inventory should fail
	err := db.FulfillOrder(ctx, order.ID)
	if err != ErrInsufficientInventory {
		t.Errorf("expected ErrInsufficientInventory, got %v", err)
	}

	// Add inventory
	_ = db.AddEggs(ctx, 10)

	// Fulfill should succeed
	if err := db.FulfillOrder(ctx, order.ID); err != nil {
		t.Fatalf("FulfillOrder: %v", err)
	}

	// Check inventory deducted
	count, _ := db.GetInventory(ctx)
	if count != 4 {
		t.Errorf("expected 4 eggs remaining, got %d", count)
	}

	// Check order status
	order, _ = db.GetOrderByID(ctx, order.ID)
	if order.Status != "fulfilled" {
		t.Errorf("expected status fulfilled, got %s", order.Status)
	}

	// Fulfill again should fail
	err = db.FulfillOrder(ctx, order.ID)
	if err == nil || err.Error() != "order already fulfilled" {
		t.Errorf("expected already fulfilled error, got %v", err)
	}
}

func TestTransactionsAndBalance(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)

	npub := "npub1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqsutj2c5"
	c, _ := db.CreateCustomer(ctx, npub)

	// Initial balance should be 0
	balance, err := db.GetCustomerBalance(ctx, npub)
	if err != nil {
		t.Fatalf("GetCustomerBalance: %v", err)
	}
	if balance != 0 {
		t.Errorf("expected 0 balance, got %d", balance)
	}

	// Record transaction
	tx, err := db.RecordTransaction(ctx, nil, "zap1", 5000, npub)
	if err != nil {
		t.Fatalf("RecordTransaction: %v", err)
	}
	if tx.AmountSats != 5000 {
		t.Errorf("expected 5000 sats, got %d", tx.AmountSats)
	}

	// Check balance
	balance, _ = db.GetCustomerBalance(ctx, npub)
	if balance != 5000 {
		t.Errorf("expected 5000 balance, got %d", balance)
	}

	// Add inventory and fulfill order to test spent calculation
	_ = db.AddEggs(ctx, 10)
	order, _ := db.CreateOrder(ctx, c.ID, 6, 3200)
	_ = db.FulfillOrder(ctx, order.ID)

	spent, err := db.GetCustomerSpent(ctx, c.ID)
	if err != nil {
		t.Fatalf("GetCustomerSpent: %v", err)
	}
	if spent != 3200 {
		t.Errorf("expected 3200 spent, got %d", spent)
	}
}

func TestOrderNotFound(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)

	_, err := db.GetOrderByID(ctx, 99999)
	if err != ErrOrderNotFound {
		t.Errorf("expected ErrOrderNotFound, got %v", err)
	}

	err = db.UpdateOrderStatus(ctx, 99999, "paid")
	if err != ErrOrderNotFound {
		t.Errorf("expected ErrOrderNotFound, got %v", err)
	}

	err = db.FulfillOrder(ctx, 99999)
	if err != ErrOrderNotFound {
		t.Errorf("expected ErrOrderNotFound, got %v", err)
	}
}
