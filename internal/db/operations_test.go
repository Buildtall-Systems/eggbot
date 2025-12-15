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

func TestGetReservedEggs(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)

	// Initially no reserved eggs
	reserved, err := db.GetReservedEggs(ctx)
	if err != nil {
		t.Fatalf("GetReservedEggs: %v", err)
	}
	if reserved != 0 {
		t.Errorf("expected 0 reserved eggs, got %d", reserved)
	}

	// Create customer and inventory
	c, _ := db.CreateCustomer(ctx, "npub1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqsutj2c5")
	_ = db.AddEggs(ctx, 30)

	// Create pending order - should be counted as reserved
	_, err = db.CreateOrder(ctx, c.ID, 6, 3200)
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}

	reserved, err = db.GetReservedEggs(ctx)
	if err != nil {
		t.Fatalf("GetReservedEggs: %v", err)
	}
	if reserved != 6 {
		t.Errorf("expected 6 reserved eggs, got %d", reserved)
	}

	// Create another pending order
	_, err = db.CreateOrder(ctx, c.ID, 12, 6400)
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}

	reserved, err = db.GetReservedEggs(ctx)
	if err != nil {
		t.Fatalf("GetReservedEggs: %v", err)
	}
	if reserved != 18 {
		t.Errorf("expected 18 reserved eggs (6+12), got %d", reserved)
	}
}

func TestGetSoldEggs(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)

	// Initially no sold eggs
	sold, err := db.GetSoldEggs(ctx)
	if err != nil {
		t.Fatalf("GetSoldEggs: %v", err)
	}
	if sold != 0 {
		t.Errorf("expected 0 sold eggs, got %d", sold)
	}

	// Create customer and inventory
	c, _ := db.CreateCustomer(ctx, "npub1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqsutj2c5")
	_ = db.AddEggs(ctx, 30)

	// Create and pay order - should be counted as sold
	order, _ := db.CreateOrder(ctx, c.ID, 6, 3200)
	_ = db.UpdateOrderStatus(ctx, order.ID, "paid")

	sold, err = db.GetSoldEggs(ctx)
	if err != nil {
		t.Fatalf("GetSoldEggs: %v", err)
	}
	if sold != 6 {
		t.Errorf("expected 6 sold eggs, got %d", sold)
	}

	// Pending order should NOT count as sold
	_, _ = db.CreateOrder(ctx, c.ID, 12, 6400)

	sold, err = db.GetSoldEggs(ctx)
	if err != nil {
		t.Fatalf("GetSoldEggs: %v", err)
	}
	if sold != 6 {
		t.Errorf("pending order should not affect sold count, expected 6, got %d", sold)
	}

	// Fulfilled order should NOT count as sold (already delivered)
	order2, _ := db.CreateOrder(ctx, c.ID, 6, 3200)
	_ = db.UpdateOrderStatus(ctx, order2.ID, "paid")
	_ = db.FulfillOrder(ctx, order2.ID)

	sold, err = db.GetSoldEggs(ctx)
	if err != nil {
		t.Fatalf("GetSoldEggs: %v", err)
	}
	if sold != 6 {
		t.Errorf("fulfilled order should not count as sold, expected 6, got %d", sold)
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

	// Add inventory (required for order creation in reservation model)
	_ = db.AddEggs(ctx, 20)

	// Create order (now reserves inventory atomically)
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

	npub := "npub1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqsutj2c5"
	c, _ := db.CreateCustomer(ctx, npub)

	_ = db.AddEggs(ctx, 10)

	order, _ := db.CreateOrder(ctx, c.ID, 6, 3200)

	count, _ := db.GetInventory(ctx)
	if count != 4 {
		t.Errorf("expected 4 eggs after order reservation, got %d", count)
	}

	// FSM requires pending -> paid -> fulfilled (cannot skip paid)
	err := db.FulfillOrder(ctx, order.ID)
	if err == nil {
		t.Error("expected error when fulfilling pending order (must be paid first)")
	}

	// Mark as paid first
	if err := db.UpdateOrderStatus(ctx, order.ID, "paid"); err != nil {
		t.Fatalf("UpdateOrderStatus to paid: %v", err)
	}

	// Now fulfill should succeed
	if err := db.FulfillOrder(ctx, order.ID); err != nil {
		t.Fatalf("FulfillOrder: %v", err)
	}

	count, _ = db.GetInventory(ctx)
	if count != 4 {
		t.Errorf("expected 4 eggs after fulfill (no change), got %d", count)
	}

	order, _ = db.GetOrderByID(ctx, order.ID)
	if order.Status != "fulfilled" {
		t.Errorf("expected status fulfilled, got %s", order.Status)
	}

	// Fulfill again should fail
	err = db.FulfillOrder(ctx, order.ID)
	if err == nil {
		t.Error("expected error when fulfilling already fulfilled order")
	}
}

func TestCreateOrder_ReservesInventory(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)

	npub := "npub1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqsutj2c5"
	c, _ := db.CreateCustomer(ctx, npub)

	// No inventory - order should fail
	_, err := db.CreateOrder(ctx, c.ID, 6, 3200)
	if err != ErrInsufficientInventory {
		t.Errorf("expected ErrInsufficientInventory with no inventory, got %v", err)
	}

	// Add 5 eggs, try to order 6
	_ = db.AddEggs(ctx, 5)
	_, err = db.CreateOrder(ctx, c.ID, 6, 3200)
	if err != ErrInsufficientInventory {
		t.Errorf("expected ErrInsufficientInventory for 6 eggs with 5 available, got %v", err)
	}

	// Add 5 more (total 10), order 6 should succeed
	_ = db.AddEggs(ctx, 5)
	order, err := db.CreateOrder(ctx, c.ID, 6, 3200)
	if err != nil {
		t.Fatalf("CreateOrder should succeed with sufficient inventory: %v", err)
	}

	// Verify inventory was deducted
	count, _ := db.GetInventory(ctx)
	if count != 4 {
		t.Errorf("expected 4 eggs after reservation, got %d", count)
	}

	// Verify order created
	if order.Status != "pending" {
		t.Errorf("expected pending status, got %s", order.Status)
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

	// Add inventory first (required for reservation model)
	_ = db.AddEggs(ctx, 10)

	// Create, pay, and fulfill order to test spent calculation
	order, _ := db.CreateOrder(ctx, c.ID, 6, 3200)
	_ = db.UpdateOrderStatus(ctx, order.ID, "paid")
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

func TestCancelOrder(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)

	// Create customer
	npub := "npub1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqsutj2c5"
	c, _ := db.CreateCustomer(ctx, npub)

	// Add inventory (required for reservation model)
	_ = db.AddEggs(ctx, 30)

	// Create order (reserves 6 eggs, leaving 24)
	order, _ := db.CreateOrder(ctx, c.ID, 6, 3200)

	// Verify inventory was reserved
	count, _ := db.GetInventory(ctx)
	if count != 24 {
		t.Errorf("expected 24 eggs after order, got %d", count)
	}

	// Cancel pending order should succeed and restore inventory
	err := db.CancelOrder(ctx, order.ID)
	if err != nil {
		t.Fatalf("CancelOrder: %v", err)
	}

	// Verify status changed
	order, _ = db.GetOrderByID(ctx, order.ID)
	if order.Status != "cancelled" {
		t.Errorf("expected status cancelled, got %s", order.Status)
	}

	// Verify inventory was restored
	count, _ = db.GetInventory(ctx)
	if count != 30 {
		t.Errorf("expected 30 eggs after cancel (restored), got %d", count)
	}

	// Cancel already cancelled order should fail
	err = db.CancelOrder(ctx, order.ID)
	if err != ErrOrderNotPending {
		t.Errorf("expected ErrOrderNotPending, got %v", err)
	}

	// Cancel non-existent order should fail
	err = db.CancelOrder(ctx, 99999)
	if err != ErrOrderNotFound {
		t.Errorf("expected ErrOrderNotFound, got %v", err)
	}

	// Cancel paid order should fail
	order2, _ := db.CreateOrder(ctx, c.ID, 6, 3200)
	_ = db.UpdateOrderStatus(ctx, order2.ID, "paid")
	err = db.CancelOrder(ctx, order2.ID)
	if err != ErrOrderNotPending {
		t.Errorf("expected ErrOrderNotPending for paid order, got %v", err)
	}

	// Cancel fulfilled order should fail
	order3, _ := db.CreateOrder(ctx, c.ID, 6, 3200)
	_ = db.UpdateOrderStatus(ctx, order3.ID, "paid")
	_ = db.FulfillOrder(ctx, order3.ID)
	err = db.CancelOrder(ctx, order3.ID)
	if err != ErrOrderNotPending {
		t.Errorf("expected ErrOrderNotPending for fulfilled order, got %v", err)
	}
}

func TestGetTotalSales(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)

	// No orders - should return 0
	total, err := db.GetTotalSales(ctx)
	if err != nil {
		t.Fatalf("GetTotalSales: %v", err)
	}
	if total != 0 {
		t.Errorf("expected 0 with no orders, got %d", total)
	}

	// Create customer and inventory
	c, _ := db.CreateCustomer(ctx, "npub1test")
	_ = db.AddEggs(ctx, 100)

	// Create pending order - should not count
	_, _ = db.CreateOrder(ctx, c.ID, 6, 3200)
	total, _ = db.GetTotalSales(ctx)
	if total != 0 {
		t.Errorf("expected 0 with pending order only, got %d", total)
	}

	// Create paid order - should not count
	order2, _ := db.CreateOrder(ctx, c.ID, 6, 3200)
	_ = db.UpdateOrderStatus(ctx, order2.ID, "paid")
	total, _ = db.GetTotalSales(ctx)
	if total != 0 {
		t.Errorf("expected 0 with paid order only, got %d", total)
	}

	// Fulfill the paid order - now it should count
	_ = db.FulfillOrder(ctx, order2.ID)
	total, _ = db.GetTotalSales(ctx)
	if total != 3200 {
		t.Errorf("expected 3200 after fulfillment, got %d", total)
	}

	// Add another fulfilled order
	order3, _ := db.CreateOrder(ctx, c.ID, 12, 6400)
	_ = db.UpdateOrderStatus(ctx, order3.ID, "paid")
	_ = db.FulfillOrder(ctx, order3.ID)
	total, _ = db.GetTotalSales(ctx)
	if total != 9600 {
		t.Errorf("expected 9600 (3200+6400), got %d", total)
	}

	// Cancelled orders should not count
	order4, _ := db.CreateOrder(ctx, c.ID, 6, 3200)
	_ = db.CancelOrder(ctx, order4.ID)
	total, _ = db.GetTotalSales(ctx)
	if total != 9600 {
		t.Errorf("expected 9600 (cancelled order not counted), got %d", total)
	}
}
