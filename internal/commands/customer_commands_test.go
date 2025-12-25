package commands

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"github.com/buildtall-systems/eggbot/internal/db"
	_ "modernc.org/sqlite"
)

func setupCmdTestDB(t *testing.T) *db.DB {
	t.Helper()

	sqlDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("opening test db: %v", err)
	}

	database := &db.DB{DB: sqlDB}

	if err := database.Migrate(); err != nil {
		_ = sqlDB.Close()
		t.Fatalf("migrating test db: %v", err)
	}

	t.Cleanup(func() { _ = database.Close() })

	return database
}

// Test keypairs - generated with nak key generate && nak key public && nak encode npub
// NEVER fabricate these - always derive properly
const (
	// Customer keypair
	testCustomerSecretHex = "234702910939c3394838131938e8da0dcfec369df3e51990263eae626aa73f87"
	testCustomerPubkeyHex = "1eca03bebec0590b918861b4431d57ff574702fa8cb015ccd566b509e9480c42"
	testCustomerNpub      = "npub1rm9q8047cpvshyvgvx6yx82hlat5wqh63jcptnx4v66sn62gp3pqsm8ejt"

	// Admin keypair
	testAdminSecretHex = "044d5d4b5961612682ce0749a9ad7f8527b42d95ab9b8cf7a2d7dd6175d8639d"
	testAdminPubkeyHex = "f28af81d4e2150fdf2366d373a125b22014397460aed537b370a58d116d5a158"
	testAdminNpub = "npub17290s82wy9g0mu3kd5mn5yjmygq5896xptk4x7ehpfvdz9k459vqywh6q7"
)

func TestInventoryCmd_Show(t *testing.T) {
	ctx := context.Background()
	database := setupCmdTestDB(t)

	tests := []struct {
		name     string
		setup    func()
		contains string
	}{
		{
			name:     "empty inventory",
			setup:    func() {},
			contains: "No eggs available",
		},
		{
			name: "one egg",
			setup: func() {
				_ = database.AddEggs(ctx, 1)
			},
			contains: "1 egg available",
		},
		{
			name: "multiple eggs",
			setup: func() {
				_ = database.AddEggs(ctx, 11) // now 12 total
			},
			contains: "12 eggs available",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()
			// Test without args (show inventory) - works for both admin and non-admin
			result := InventoryCmd(ctx, database, []string{}, false)
			if result.Error != nil {
				t.Fatalf("unexpected error: %v", result.Error)
			}
			if !strings.Contains(result.Message, tt.contains) {
				t.Errorf("expected message containing %q, got %q", tt.contains, result.Message)
			}
		})
	}
}

func TestInventoryCmd_Add(t *testing.T) {
	ctx := context.Background()
	database := setupCmdTestDB(t)

	tests := []struct {
		name        string
		args        []string
		isAdmin     bool
		wantErr     bool
		errContains string
		msgContains string
	}{
		{
			name:        "non-admin denied",
			args:        []string{"add", "10"},
			isAdmin:     false,
			wantErr:     true,
			errContains: "admin access required",
		},
		{
			name:        "admin no quantity",
			args:        []string{"add"},
			isAdmin:     true,
			wantErr:     true,
			errContains: "usage",
		},
		{
			name:        "admin invalid number",
			args:        []string{"add", "abc"},
			isAdmin:     true,
			wantErr:     true,
			errContains: "positive number",
		},
		{
			name:        "admin zero",
			args:        []string{"add", "0"},
			isAdmin:     true,
			wantErr:     true,
			errContains: "positive number",
		},
		{
			name:        "admin valid add",
			args:        []string{"add", "12"},
			isAdmin:     true,
			wantErr:     false,
			msgContains: "Added 12 eggs",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := InventoryCmd(ctx, database, tt.args, tt.isAdmin)
			if tt.wantErr {
				if result.Error == nil {
					t.Fatal("expected error")
				}
				if !strings.Contains(result.Error.Error(), tt.errContains) {
					t.Errorf("expected error containing %q, got %q", tt.errContains, result.Error.Error())
				}
			} else {
				if result.Error != nil {
					t.Fatalf("unexpected error: %v", result.Error)
				}
				if !strings.Contains(result.Message, tt.msgContains) {
					t.Errorf("expected message containing %q, got %q", tt.msgContains, result.Message)
				}
			}
		})
	}
}

func TestInventoryCmd_Set(t *testing.T) {
	ctx := context.Background()
	database := setupCmdTestDB(t)

	// Start with some eggs
	_ = database.AddEggs(ctx, 50)

	tests := []struct {
		name        string
		args        []string
		isAdmin     bool
		wantErr     bool
		errContains string
		msgContains string
	}{
		{
			name:        "non-admin denied",
			args:        []string{"set", "10"},
			isAdmin:     false,
			wantErr:     true,
			errContains: "admin access required",
		},
		{
			name:        "admin no quantity",
			args:        []string{"set"},
			isAdmin:     true,
			wantErr:     true,
			errContains: "usage",
		},
		{
			name:        "admin invalid number",
			args:        []string{"set", "abc"},
			isAdmin:     true,
			wantErr:     true,
			errContains: "non-negative number",
		},
		{
			name:        "admin negative",
			args:        []string{"set", "-5"},
			isAdmin:     true,
			wantErr:     true,
			errContains: "non-negative number",
		},
		{
			name:        "admin set to zero",
			args:        []string{"set", "0"},
			isAdmin:     true,
			wantErr:     false,
			msgContains: "Inventory set to 0 eggs",
		},
		{
			name:        "admin set to 25",
			args:        []string{"set", "25"},
			isAdmin:     true,
			wantErr:     false,
			msgContains: "Inventory set to 25 eggs",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := InventoryCmd(ctx, database, tt.args, tt.isAdmin)
			if tt.wantErr {
				if result.Error == nil {
					t.Fatal("expected error")
				}
				if !strings.Contains(result.Error.Error(), tt.errContains) {
					t.Errorf("expected error containing %q, got %q", tt.errContains, result.Error.Error())
				}
			} else {
				if result.Error != nil {
					t.Fatalf("unexpected error: %v", result.Error)
				}
				if !strings.Contains(result.Message, tt.msgContains) {
					t.Errorf("expected message containing %q, got %q", tt.msgContains, result.Message)
				}
			}
		})
	}

	// Verify final state
	count, _ := database.GetInventory(ctx)
	if count != 25 {
		t.Errorf("expected final inventory 25, got %d", count)
	}
}

func TestInventoryCmd_UnknownSubcommand(t *testing.T) {
	ctx := context.Background()
	database := setupCmdTestDB(t)
	_ = database.AddEggs(ctx, 10)

	// Non-admin with unknown subcommand gets inventory shown
	result := InventoryCmd(ctx, database, []string{"foobar"}, false)
	if result.Error != nil {
		t.Fatalf("expected no error for non-admin, got %v", result.Error)
	}
	if !strings.Contains(result.Message, "10 eggs available") {
		t.Errorf("expected inventory message, got %q", result.Message)
	}

	// Admin with unknown subcommand gets error
	result = InventoryCmd(ctx, database, []string{"foobar"}, true)
	if result.Error == nil {
		t.Fatal("expected error for admin with unknown subcommand")
	}
	if !strings.Contains(result.Error.Error(), "unknown subcommand") {
		t.Errorf("expected unknown subcommand error, got %q", result.Error.Error())
	}
}

func TestInventoryCmd_AdminView(t *testing.T) {
	ctx := context.Background()
	database := setupCmdTestDB(t)

	// Setup: create customer and inventory
	c, _ := database.CreateCustomer(ctx, testCustomerNpub)
	_ = database.AddEggs(ctx, 30)

	// Create orders in different states to test breakdown
	// Pending order: 6 eggs (reserved)
	_, _ = database.CreateOrder(ctx, c.ID, 6, 3200)

	// Paid order: 12 eggs (sold)
	paidOrder, _ := database.CreateOrder(ctx, c.ID, 12, 6400)
	_ = database.UpdateOrderStatus(ctx, paidOrder.ID, "paid")

	// After orders: available = 30 - 6 - 12 = 12 eggs

	// Test customer view - should only show available
	result := InventoryCmd(ctx, database, []string{}, false)
	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}
	if !strings.Contains(result.Message, "12 eggs available") {
		t.Errorf("customer view should show simple count, got %q", result.Message)
	}
	// Should NOT show breakdown
	if strings.Contains(result.Message, "Reserved") {
		t.Error("customer view should not show Reserved breakdown")
	}

	// Test admin view - should show full breakdown
	result = InventoryCmd(ctx, database, []string{}, true)
	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}

	// Check for all categories
	if !strings.Contains(result.Message, "Available:") {
		t.Errorf("admin view should show Available, got %q", result.Message)
	}
	if !strings.Contains(result.Message, "Reserved:") {
		t.Errorf("admin view should show Reserved, got %q", result.Message)
	}
	if !strings.Contains(result.Message, "Sold:") {
		t.Errorf("admin view should show Sold, got %q", result.Message)
	}
	if !strings.Contains(result.Message, "On-hand:") {
		t.Errorf("admin view should show On-hand, got %q", result.Message)
	}

	// Check values (available=12, reserved=6, sold=12, on-hand=18)
	if !strings.Contains(result.Message, "12 eggs (can be sold)") {
		t.Errorf("expected 12 available eggs, got %q", result.Message)
	}
	if !strings.Contains(result.Message, "6 eggs (pending payment)") {
		t.Errorf("expected 6 reserved eggs, got %q", result.Message)
	}
	if !strings.Contains(result.Message, "12 eggs (awaiting delivery)") {
		t.Errorf("expected 12 sold eggs, got %q", result.Message)
	}
	if !strings.Contains(result.Message, "30 eggs (total in storage)") {
		t.Errorf("expected 30 on-hand eggs, got %q", result.Message)
	}
}

func TestOrderCmd(t *testing.T) {
	ctx := context.Background()
	database := setupCmdTestDB(t)

	// Setup: add inventory and customer using properly generated keypair
	_ = database.AddEggs(ctx, 50)
	_, _ = database.CreateCustomer(ctx, testCustomerNpub)

	tests := []struct {
		name        string
		args        []string
		setup       func() // cancel any pending orders
		wantErr     bool
		errContains string
		msgContains string
	}{
		{
			name:        "no args",
			args:        []string{},
			wantErr:     true,
			errContains: "usage",
		},
		{
			name:        "invalid quantity string",
			args:        []string{"abc"},
			wantErr:     true,
			errContains: "6 or 12",
		},
		{
			name:        "zero quantity",
			args:        []string{"0"},
			wantErr:     true,
			errContains: "6 or 12",
		},
		{
			name:        "quantity 1 rejected",
			args:        []string{"1"},
			wantErr:     true,
			errContains: "6 or 12",
		},
		{
			name:        "quantity 7 rejected",
			args:        []string{"7"},
			wantErr:     true,
			errContains: "6 or 12",
		},
		{
			name:        "quantity 18 rejected (over max)",
			args:        []string{"18"},
			wantErr:     true,
			errContains: "6 or 12",
		},
		{
			name:        "valid order 6 eggs",
			args:        []string{"6"},
			wantErr:     false,
			msgContains: "6 eggs reserved",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Cancel any pending orders from previous test
			c, _ := database.GetCustomerByNpub(ctx, testCustomerNpub)
			pending, _ := database.GetPendingOrdersByCustomer(ctx, c.ID)
			for _, o := range pending {
				_ = database.CancelOrder(ctx, o.ID)
			}

			result := OrderCmd(ctx, database, testCustomerNpub, tt.args, 3200, "", "", nil)
			if tt.wantErr {
				if result.Error == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(result.Error.Error(), tt.errContains) {
					t.Errorf("expected error containing %q, got %q", tt.errContains, result.Error.Error())
				}
			} else {
				if result.Error != nil {
					t.Fatalf("unexpected error: %v", result.Error)
				}
				if !strings.Contains(result.Message, tt.msgContains) {
					t.Errorf("expected message containing %q, got %q", tt.msgContains, result.Message)
				}
			}
		})
	}
}

func TestOrderCmd_ValidDozen(t *testing.T) {
	ctx := context.Background()
	database := setupCmdTestDB(t)

	_ = database.AddEggs(ctx, 20)
	_, _ = database.CreateCustomer(ctx, testCustomerNpub)

	result := OrderCmd(ctx, database, testCustomerNpub, []string{"12"}, 3200, "", "", nil)
	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}
	if !strings.Contains(result.Message, "12 eggs reserved") {
		t.Errorf("expected 12 eggs reserved, got %q", result.Message)
	}
	if !strings.Contains(result.Message, "6400 sats") {
		t.Errorf("expected 6400 sats (2 half-dozens), got %q", result.Message)
	}
}

func TestOrderCmd_PendingOrderBlocks(t *testing.T) {
	ctx := context.Background()
	database := setupCmdTestDB(t)

	_ = database.AddEggs(ctx, 50)
	c, _ := database.CreateCustomer(ctx, testCustomerNpub)

	// First order succeeds
	result := OrderCmd(ctx, database, testCustomerNpub, []string{"6"}, 3200, "", "", nil)
	if result.Error != nil {
		t.Fatalf("first order failed: %v", result.Error)
	}

	// Second order blocked due to pending
	result = OrderCmd(ctx, database, testCustomerNpub, []string{"6"}, 3200, "", "", nil)
	if result.Error == nil {
		t.Fatal("expected error for second order with pending")
	}
	if !strings.Contains(result.Error.Error(), "unpaid order") {
		t.Errorf("expected unpaid order error, got %q", result.Error.Error())
	}

	// Cancel the pending order
	pending, _ := database.GetPendingOrdersByCustomer(ctx, c.ID)
	_ = database.CancelOrder(ctx, pending[0].ID)

	// Now ordering works again
	result = OrderCmd(ctx, database, testCustomerNpub, []string{"6"}, 3200, "", "", nil)
	if result.Error != nil {
		t.Fatalf("order after cancel failed: %v", result.Error)
	}
}

func TestOrderCmd_InsufficientInventory(t *testing.T) {
	ctx := context.Background()
	database := setupCmdTestDB(t)

	// Setup: only 5 eggs, customer orders 6
	_ = database.AddEggs(ctx, 5)
	_, _ = database.CreateCustomer(ctx, testCustomerNpub)

	result := OrderCmd(ctx, database, testCustomerNpub, []string{"6"}, 3200, "", "", nil)
	if result.Error == nil {
		t.Fatal("expected error for insufficient inventory")
	}
	if !strings.Contains(result.Error.Error(), "only 5 eggs available") {
		t.Errorf("expected inventory error, got %v", result.Error)
	}
}

func TestBalanceCmd(t *testing.T) {
	ctx := context.Background()
	database := setupCmdTestDB(t)

	c, _ := database.CreateCustomer(ctx, testCustomerNpub)

	// No payments
	result := BalanceCmd(ctx, database, testCustomerNpub)
	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}
	if !strings.Contains(result.Message, "No payments received") {
		t.Errorf("expected no payments message, got %q", result.Message)
	}

	// Add payment
	_, _ = database.RecordTransaction(ctx, nil, "zap1", 5000, testCustomerNpub)

	result = BalanceCmd(ctx, database, testCustomerNpub)
	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}
	if !strings.Contains(result.Message, "Received: 5000 sats") {
		t.Errorf("expected 5000 sats received, got %q", result.Message)
	}

	// Create, pay, and fulfill an order to test spent
	_ = database.AddEggs(ctx, 10)
	order, _ := database.CreateOrder(ctx, c.ID, 6, 3200)
	_ = database.UpdateOrderStatus(ctx, order.ID, "paid")
	_ = database.FulfillOrder(ctx, order.ID)

	result = BalanceCmd(ctx, database, testCustomerNpub)
	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}
	if !strings.Contains(result.Message, "Spent: 3200 sats") {
		t.Errorf("expected 3200 sats spent, got %q", result.Message)
	}
	if !strings.Contains(result.Message, "Balance: 1800 sats") {
		t.Errorf("expected 1800 sats balance, got %q", result.Message)
	}
}

func TestHistoryCmd(t *testing.T) {
	ctx := context.Background()
	database := setupCmdTestDB(t)

	c, _ := database.CreateCustomer(ctx, testCustomerNpub)

	// No orders
	result := HistoryCmd(ctx, database, testCustomerNpub)
	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}
	if !strings.Contains(result.Message, "No orders yet") {
		t.Errorf("expected no orders message, got %q", result.Message)
	}

	// Add inventory (required for reservation model)
	_ = database.AddEggs(ctx, 30)

	// Create orders (reserves inventory)
	_, _ = database.CreateOrder(ctx, c.ID, 6, 3200)
	_, _ = database.CreateOrder(ctx, c.ID, 12, 6400)

	result = HistoryCmd(ctx, database, testCustomerNpub)
	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}
	if !strings.Contains(result.Message, "Recent orders") {
		t.Errorf("expected recent orders, got %q", result.Message)
	}
	if !strings.Contains(result.Message, "6 eggs") {
		t.Errorf("expected 6 eggs order, got %q", result.Message)
	}
	if !strings.Contains(result.Message, "12 eggs") {
		t.Errorf("expected 12 eggs order, got %q", result.Message)
	}
}

func TestHelpCmd(t *testing.T) {
	// Non-admin help
	result := HelpCmd(false)
	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}
	if !strings.Contains(result.Message, "inventory") {
		t.Error("expected inventory in help")
	}
	if !strings.Contains(result.Message, "cancel") {
		t.Error("expected cancel in help")
	}
	if strings.Contains(result.Message, "Admin commands") {
		t.Error("non-admin should not see admin commands")
	}

	// Admin help
	result = HelpCmd(true)
	if !strings.Contains(result.Message, "Admin commands") {
		t.Error("admin should see admin commands")
	}
	if !strings.Contains(result.Message, "addcustomer") {
		t.Error("admin should see addcustomer")
	}
}

func TestCancelOrderCmd(t *testing.T) {
	ctx := context.Background()
	database := setupCmdTestDB(t)

	// Setup: customer, inventory, and order
	c, _ := database.CreateCustomer(ctx, testCustomerNpub)
	_ = database.AddEggs(ctx, 20) // Required for reservation model
	order, _ := database.CreateOrder(ctx, c.ID, 6, 3200)

	tests := []struct {
		name        string
		args        []string
		wantErr     bool
		errContains string
		msgContains string
	}{
		{
			name:        "no args",
			args:        []string{},
			wantErr:     true,
			errContains: "usage",
		},
		{
			name:        "invalid order id",
			args:        []string{"abc"},
			wantErr:     true,
			errContains: "must be a number",
		},
		{
			name:        "non-existent order",
			args:        []string{"99999"},
			wantErr:     true,
			errContains: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CancelOrderCmd(ctx, database, testCustomerNpub, tt.args)
			if tt.wantErr {
				if result.Error == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(result.Error.Error(), tt.errContains) {
					t.Errorf("expected error containing %q, got %q", tt.errContains, result.Error.Error())
				}
			} else {
				if result.Error != nil {
					t.Fatalf("unexpected error: %v", result.Error)
				}
				if !strings.Contains(result.Message, tt.msgContains) {
					t.Errorf("expected message containing %q, got %q", tt.msgContains, result.Message)
				}
			}
		})
	}

	// Test successful cancellation
	t.Run("cancel pending order", func(t *testing.T) {
		result := CancelOrderCmd(ctx, database, testCustomerNpub, []string{fmt.Sprintf("%d", order.ID)})
		if result.Error != nil {
			t.Fatalf("unexpected error: %v", result.Error)
		}
		if !strings.Contains(result.Message, "cancelled") {
			t.Errorf("expected cancelled message, got %q", result.Message)
		}
	})

	// Test cancelling already cancelled order
	t.Run("cancel already cancelled", func(t *testing.T) {
		result := CancelOrderCmd(ctx, database, testCustomerNpub, []string{fmt.Sprintf("%d", order.ID)})
		if result.Error == nil {
			t.Fatal("expected error for already cancelled order")
		}
		if !strings.Contains(result.Error.Error(), "cannot be cancelled") {
			t.Errorf("expected cannot be cancelled error, got %v", result.Error)
		}
	})
}

func TestCancelOrderCmd_OwnershipCheck(t *testing.T) {
	ctx := context.Background()
	database := setupCmdTestDB(t)

	// Setup: two customers
	c1, _ := database.CreateCustomer(ctx, testCustomerNpub)
	_, _ = database.CreateCustomer(ctx, testAdminNpub)

	// Add inventory (required for reservation model)
	_ = database.AddEggs(ctx, 20)

	// Create order for customer 1
	order, _ := database.CreateOrder(ctx, c1.ID, 6, 3200)

	// Customer 2 (admin npub) tries to cancel customer 1's order
	result := CancelOrderCmd(ctx, database, testAdminNpub, []string{fmt.Sprintf("%d", order.ID)})
	if result.Error == nil {
		t.Fatal("expected error when cancelling another's order")
	}
	if !strings.Contains(result.Error.Error(), "your own orders") {
		t.Errorf("expected ownership error, got %v", result.Error)
	}
}
