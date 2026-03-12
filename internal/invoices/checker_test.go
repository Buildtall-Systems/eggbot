package invoices

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Buildtall-Systems/btk/lightning"

	"github.com/buildtall-systems/eggbot/internal/db"
)

func setupTestDB(t *testing.T) *db.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	if err := database.Migrate(); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}

func createPendingOrderWithHash(t *testing.T, database *db.DB, npub string, paymentHash string) (*db.Customer, *db.Order) {
	t.Helper()
	ctx := context.Background()

	customer, err := database.CreateCustomer(ctx, npub)
	if err != nil {
		t.Fatalf("failed to create customer: %v", err)
	}

	if err = database.SetInventory(ctx, 100); err != nil {
		t.Fatalf("failed to set inventory: %v", err)
	}

	order, err := database.CreateOrder(ctx, customer.ID, 6, 3000)
	if err != nil {
		t.Fatalf("failed to create order: %v", err)
	}

	expiresAt := time.Now().UTC().Add(1 * time.Hour)
	if err = database.SetOrderPaymentHash(ctx, order.ID, paymentHash, &expiresAt); err != nil {
		t.Fatalf("failed to set payment hash: %v", err)
	}

	// Re-fetch to get the payment_hash field populated
	order, err = database.GetOrderByID(ctx, order.ID)
	if err != nil {
		t.Fatalf("failed to re-fetch order: %v", err)
	}

	return customer, order
}

func TestCheckPendingInvoices_Settled(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()
	backend := lightning.NewMockBackend()

	inv, err := backend.CreateInvoice(ctx, 3000, "order")
	if err != nil {
		t.Fatalf("failed to create invoice: %v", err)
	}

	customer, _ := createPendingOrderWithHash(t, database, "npub1settled", inv.PaymentHash)

	if err = backend.MarkPaid(inv.PaymentHash); err != nil {
		t.Fatalf("failed to mark paid: %v", err)
	}

	result := CheckPendingInvoices(ctx, database, backend)

	if len(result.Settled) != 1 {
		t.Fatalf("expected 1 settled order, got %d", len(result.Settled))
	}
	if result.Settled[0].Customer.ID != customer.ID {
		t.Errorf("settled customer ID = %d, want %d", result.Settled[0].Customer.ID, customer.ID)
	}
	if result.Settled[0].Customer.Npub != "npub1settled" {
		t.Errorf("settled customer npub = %q, want %q", result.Settled[0].Customer.Npub, "npub1settled")
	}

	// Verify order status was updated in DB
	order, err := database.GetOrderByID(ctx, result.Settled[0].Order.ID)
	if err != nil {
		t.Fatalf("failed to get order: %v", err)
	}
	if order.Status != "paid" {
		t.Errorf("order status = %q, want %q", order.Status, "paid")
	}
}

func TestCheckPendingInvoices_NotPaid(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()
	backend := lightning.NewMockBackend()

	inv, err := backend.CreateInvoice(ctx, 3000, "order")
	if err != nil {
		t.Fatalf("failed to create invoice: %v", err)
	}

	createPendingOrderWithHash(t, database, "npub1unpaid", inv.PaymentHash)
	// Do NOT mark paid

	result := CheckPendingInvoices(ctx, database, backend)

	if len(result.Settled) != 0 {
		t.Errorf("expected 0 settled orders, got %d", len(result.Settled))
	}

	// Verify order remains pending
	orders, err := database.GetPendingOrdersWithPaymentHash(ctx)
	if err != nil {
		t.Fatalf("failed to get pending orders: %v", err)
	}
	if len(orders) != 1 {
		t.Errorf("expected 1 pending order, got %d", len(orders))
	}
}

func TestCheckPendingInvoices_BackendError(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()
	backend := lightning.NewMockBackend()

	// Create order with a payment hash that the mock backend doesn't know about
	// This will cause CheckInvoicePaid to return an error
	createPendingOrderWithHash(t, database, "npub1error", "nonexistent_hash")

	result := CheckPendingInvoices(ctx, database, backend)

	// Should not crash, just log the error
	if len(result.Settled) != 0 {
		t.Errorf("expected 0 settled orders, got %d", len(result.Settled))
	}

	// Order should still be pending
	orders, err := database.GetPendingOrdersWithPaymentHash(ctx)
	if err != nil {
		t.Fatalf("failed to get pending orders: %v", err)
	}
	if len(orders) != 1 {
		t.Errorf("expected 1 pending order still pending, got %d", len(orders))
	}
}

func TestCheckPendingInvoices_NilBackend(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()

	result := CheckPendingInvoices(ctx, database, nil)

	if len(result.Settled) != 0 {
		t.Errorf("expected 0 settled orders with nil backend, got %d", len(result.Settled))
	}
	if len(result.Cancelled) != 0 {
		t.Errorf("expected 0 cancelled orders with nil backend, got %d", len(result.Cancelled))
	}
}

func TestCheckPendingInvoices_NoPendingOrders(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()
	backend := lightning.NewMockBackend()

	result := CheckPendingInvoices(ctx, database, backend)

	if len(result.Settled) != 0 {
		t.Errorf("expected 0 settled orders, got %d", len(result.Settled))
	}
}
