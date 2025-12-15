package zaps

import (
	"context"
	"strings"
	"testing"

	"github.com/buildtall-systems/eggbot/internal/db"
)

func setupProcessorTestDB(t *testing.T) *db.DB {
	t.Helper()
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("opening database: %v", err)
	}
	if err := database.Migrate(); err != nil {
		t.Fatalf("migrating database: %v", err)
	}
	return database
}

// Test keypair (generated with nak):
// hex: dcfafaaebf643e0c8517e49e13ad25c60ee4a57a0b5f5fc401adbcb9d151f5f5
// npub: npub1mna04t4lvslqepghuj0p8tf9cc8wfft6pd04l3qp4k7tn5237h6sj6ru9w
const testSenderNpub = "npub1mna04t4lvslqepghuj0p8tf9cc8wfft6pd04l3qp4k7tn5237h6sj6ru9w"

func TestProcessZap_UnknownSender(t *testing.T) {
	database := setupProcessorTestDB(t)
	defer func() { _ = database.Close() }()

	ctx := context.Background()

	zap := &ValidatedZap{
		SenderNpub: testSenderNpub,
		AmountSats: 1000,
		ZapEventID: "test-zap-event-1",
	}

	result, err := ProcessZap(ctx, database, zap)
	if err != nil {
		t.Fatalf("ProcessZap() error = %v", err)
	}

	if result.CustomerFound {
		t.Error("expected CustomerFound = false for unknown sender")
	}

	if result.AmountSats != 1000 {
		t.Errorf("AmountSats = %d, want 1000", result.AmountSats)
	}

	// Message must contain full npub, not truncated
	if !strings.Contains(result.Message, testSenderNpub) {
		t.Errorf("Message should contain full npub %s, got: %s", testSenderNpub, result.Message)
	}
}

func TestProcessZap_KnownCustomer(t *testing.T) {
	database := setupProcessorTestDB(t)
	defer func() { _ = database.Close() }()

	ctx := context.Background()

	// Register customer first
	_, err := database.CreateCustomer(ctx, testSenderNpub)
	if err != nil {
		t.Fatalf("creating customer: %v", err)
	}

	zap := &ValidatedZap{
		SenderNpub: testSenderNpub,
		AmountSats: 2000,
		ZapEventID: "test-zap-event-2",
	}

	result, err := ProcessZap(ctx, database, zap)
	if err != nil {
		t.Fatalf("ProcessZap() error = %v", err)
	}

	if !result.CustomerFound {
		t.Error("expected CustomerFound = true for registered customer")
	}

	if result.AmountSats != 2000 {
		t.Errorf("AmountSats = %d, want 2000", result.AmountSats)
	}

	// Verify transaction was recorded
	balance, err := database.GetCustomerBalance(ctx, testSenderNpub)
	if err != nil {
		t.Fatalf("GetCustomerBalance() error = %v", err)
	}

	if balance != 2000 {
		t.Errorf("balance = %d, want 2000", balance)
	}
}

func TestProcessZap_DuplicateZap(t *testing.T) {
	database := setupProcessorTestDB(t)
	defer func() { _ = database.Close() }()

	ctx := context.Background()

	// Register customer
	_, err := database.CreateCustomer(ctx, testSenderNpub)
	if err != nil {
		t.Fatalf("creating customer: %v", err)
	}

	zap := &ValidatedZap{
		SenderNpub: testSenderNpub,
		AmountSats: 1000,
		ZapEventID: "duplicate-zap-id",
	}

	// First zap should succeed
	_, err = ProcessZap(ctx, database, zap)
	if err != nil {
		t.Fatalf("first ProcessZap() error = %v", err)
	}

	// Second zap with same ID should fail
	_, err = ProcessZap(ctx, database, zap)
	if err != ErrDuplicateZap {
		t.Errorf("expected ErrDuplicateZap, got %v", err)
	}

	// Verify balance is only credited once
	balance, err := database.GetCustomerBalance(ctx, testSenderNpub)
	if err != nil {
		t.Fatalf("GetCustomerBalance() error = %v", err)
	}

	if balance != 1000 {
		t.Errorf("balance = %d, want 1000 (only one zap credited)", balance)
	}
}

func TestProcessZap_AutoMarkPaid(t *testing.T) {
	database := setupProcessorTestDB(t)
	defer func() { _ = database.Close() }()

	ctx := context.Background()

	// Register customer
	customer, err := database.CreateCustomer(ctx, testSenderNpub)
	if err != nil {
		t.Fatalf("creating customer: %v", err)
	}

	// Add inventory (required for reservation model)
	_ = database.AddEggs(ctx, 10)

	// Create a pending order for 3200 sats (reserves inventory)
	order, err := database.CreateOrder(ctx, customer.ID, 6, 3200)
	if err != nil {
		t.Fatalf("creating order: %v", err)
	}

	// Send a zap that covers the order
	zap := &ValidatedZap{
		SenderNpub: testSenderNpub,
		AmountSats: 3500, // More than needed
		ZapEventID: "auto-pay-zap",
	}

	result, err := ProcessZap(ctx, database, zap)
	if err != nil {
		t.Fatalf("ProcessZap() error = %v", err)
	}

	if !result.CustomerFound {
		t.Error("expected CustomerFound = true")
	}

	// Check if order was marked as paid
	updatedOrder, err := database.GetOrderByID(ctx, order.ID)
	if err != nil {
		t.Fatalf("GetOrderByID() error = %v", err)
	}

	if updatedOrder.Status != "paid" {
		t.Errorf("order status = %s, want 'paid'", updatedOrder.Status)
	}
}

func TestProcessZap_InsufficientForOrder(t *testing.T) {
	database := setupProcessorTestDB(t)
	defer func() { _ = database.Close() }()

	ctx := context.Background()

	// Register customer
	customer, err := database.CreateCustomer(ctx, testSenderNpub)
	if err != nil {
		t.Fatalf("creating customer: %v", err)
	}

	// Add inventory (required for reservation model)
	_ = database.AddEggs(ctx, 10)

	// Create a pending order for 3200 sats (reserves inventory)
	order, err := database.CreateOrder(ctx, customer.ID, 6, 3200)
	if err != nil {
		t.Fatalf("creating order: %v", err)
	}

	// Send a zap that doesn't cover the order
	zap := &ValidatedZap{
		SenderNpub: testSenderNpub,
		AmountSats: 1000, // Not enough
		ZapEventID: "partial-zap",
	}

	result, err := ProcessZap(ctx, database, zap)
	if err != nil {
		t.Fatalf("ProcessZap() error = %v", err)
	}

	if !result.CustomerFound {
		t.Error("expected CustomerFound = true")
	}

	// Check order is still pending
	updatedOrder, err := database.GetOrderByID(ctx, order.ID)
	if err != nil {
		t.Fatalf("GetOrderByID() error = %v", err)
	}

	if updatedOrder.Status != "pending" {
		t.Errorf("order status = %s, want 'pending' (insufficient funds)", updatedOrder.Status)
	}

	// Verify balance was credited though
	balance, err := database.GetCustomerBalance(ctx, testSenderNpub)
	if err != nil {
		t.Fatalf("GetCustomerBalance() error = %v", err)
	}

	if balance != 1000 {
		t.Errorf("balance = %d, want 1000", balance)
	}
}
