package commands

import (
	"context"
	"strings"
	"testing"
)

// Test keypairs are defined in customer_commands_test.go:
// - testCustomerNpub, testCustomerPubkeyHex
// - testAdminNpub, testAdminPubkeyHex
// - testUnknownNpub, testUnknownPubkeyHex

func TestAddEggsCmd(t *testing.T) {
	ctx := context.Background()
	database := setupCmdTestDB(t)

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
			name:        "invalid number",
			args:        []string{"abc"},
			wantErr:     true,
			errContains: "positive number",
		},
		{
			name:        "zero",
			args:        []string{"0"},
			wantErr:     true,
			errContains: "positive number",
		},
		{
			name:        "valid add",
			args:        []string{"12"},
			wantErr:     false,
			msgContains: "Added 12 eggs",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := AddEggsCmd(ctx, database, tt.args)
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

func TestDeliverCmd(t *testing.T) {
	ctx := context.Background()
	database := setupCmdTestDB(t)

	// Setup customer using properly generated keypair
	c, _ := database.CreateCustomer(ctx, testCustomerNpub)

	tests := []struct {
		name        string
		args        []string
		setup       func()
		wantErr     bool
		errContains string
		msgContains string
	}{
		{
			name:        "no args",
			args:        []string{},
			setup:       func() {},
			wantErr:     true,
			errContains: "usage",
		},
		{
			name:        "invalid npub format",
			args:        []string{"notanpub"},
			setup:       func() {},
			wantErr:     true,
			errContains: "invalid npub",
		},
		{
			name:        "customer not found",
			args:        []string{testUnknownNpub}, // properly generated but not registered
			setup:       func() {},
			wantErr:     true,
			errContains: "customer not found",
		},
		{
			name:        "no paid orders",
			args:        []string{testCustomerNpub},
			setup:       func() {},
			wantErr:     false,
			msgContains: "No paid orders to deliver",
		},
		{
			name: "deliver paid order",
			args: []string{testCustomerNpub},
			setup: func() {
				_ = database.AddEggs(ctx, 10)
				order, _ := database.CreateOrder(ctx, c.ID, 6, 3200)
				_ = database.UpdateOrderStatus(ctx, order.ID, "paid") // Must be paid to deliver
			},
			wantErr:     false,
			msgContains: "Delivered 1 orders",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()
			result := DeliverCmd(ctx, database, tt.args)
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

func TestDeliverCmd_OnlyDeliversPaidOrders(t *testing.T) {
	ctx := context.Background()
	database := setupCmdTestDB(t)

	// Setup customer and inventory
	c, _ := database.CreateCustomer(ctx, testCustomerNpub)
	_ = database.AddEggs(ctx, 30)

	// Create orders in different states
	pendingOrder, _ := database.CreateOrder(ctx, c.ID, 6, 3200)   // status: pending (unpaid)
	paidOrder, _ := database.CreateOrder(ctx, c.ID, 12, 6400)     // status: paid (will be set below)
	cancelledOrder, _ := database.CreateOrder(ctx, c.ID, 6, 3200) // status: cancelled (will be set below)

	// Set statuses
	_ = database.UpdateOrderStatus(ctx, paidOrder.ID, "paid")
	_ = database.CancelOrder(ctx, cancelledOrder.ID)

	// Deliver - should only deliver the paid order
	result := DeliverCmd(ctx, database, []string{testCustomerNpub})
	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}

	// Should have delivered 1 order (12 eggs)
	if !strings.Contains(result.Message, "Delivered 1 orders") {
		t.Errorf("expected 1 order delivered, got %q", result.Message)
	}
	if !strings.Contains(result.Message, "12 eggs") {
		t.Errorf("expected 12 eggs, got %q", result.Message)
	}

	// Verify the paid order is now fulfilled
	order, _ := database.GetOrderByID(ctx, paidOrder.ID)
	if order.Status != "fulfilled" {
		t.Errorf("expected paid order to be fulfilled, got %s", order.Status)
	}

	// Verify the pending order is still pending (not delivered)
	order, _ = database.GetOrderByID(ctx, pendingOrder.ID)
	if order.Status != "pending" {
		t.Errorf("expected pending order to remain pending, got %s", order.Status)
	}

	// Verify cancelled order is still cancelled
	order, _ = database.GetOrderByID(ctx, cancelledOrder.ID)
	if order.Status != "cancelled" {
		t.Errorf("expected cancelled order to remain cancelled, got %s", order.Status)
	}

	// Try delivering again - should have no paid orders
	result = DeliverCmd(ctx, database, []string{testCustomerNpub})
	if !strings.Contains(result.Message, "No paid orders to deliver") {
		t.Errorf("expected no paid orders message, got %q", result.Message)
	}
}

func TestPaymentCmd(t *testing.T) {
	ctx := context.Background()
	database := setupCmdTestDB(t)

	_, _ = database.CreateCustomer(ctx, testCustomerNpub)

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
			name:        "missing amount",
			args:        []string{testCustomerNpub},
			wantErr:     true,
			errContains: "usage",
		},
		{
			name:        "invalid amount",
			args:        []string{testCustomerNpub, "abc"},
			wantErr:     true,
			errContains: "positive number",
		},
		{
			name:        "valid payment",
			args:        []string{testCustomerNpub, "5000"},
			wantErr:     false,
			msgContains: "Recorded payment of 5000 sats",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := PaymentCmd(ctx, database, tt.args)
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

func TestAdjustCmd(t *testing.T) {
	ctx := context.Background()
	database := setupCmdTestDB(t)

	_, _ = database.CreateCustomer(ctx, testCustomerNpub)

	tests := []struct {
		name        string
		args        []string
		wantErr     bool
		errContains string
		msgContains string
	}{
		{
			name:        "positive adjustment",
			args:        []string{testCustomerNpub, "100"},
			wantErr:     false,
			msgContains: "Added 100 sats",
		},
		{
			name:        "negative adjustment",
			args:        []string{testCustomerNpub, "-50"},
			wantErr:     false,
			msgContains: "Deducted 50 sats",
		},
		{
			name:        "invalid number",
			args:        []string{testCustomerNpub, "notanumber"},
			wantErr:     true,
			errContains: "must be a number",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := AdjustCmd(ctx, database, tt.args)
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

func TestCustomersCmd(t *testing.T) {
	ctx := context.Background()
	database := setupCmdTestDB(t)

	// Empty list
	result := CustomersCmd(ctx, database)
	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}
	if !strings.Contains(result.Message, "No registered customers") {
		t.Errorf("expected no customers message, got %q", result.Message)
	}

	// Add customer
	_, _ = database.CreateCustomer(ctx, testCustomerNpub)

	result = CustomersCmd(ctx, database)
	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}
	if !strings.Contains(result.Message, "1 registered customers") {
		t.Errorf("expected 1 customer, got %q", result.Message)
	}
}

func TestAddCustomerCmd(t *testing.T) {
	ctx := context.Background()
	database := setupCmdTestDB(t)

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
			name:        "invalid npub",
			args:        []string{"notanpub"},
			wantErr:     true,
			errContains: "invalid npub",
		},
		{
			name:        "valid add",
			args:        []string{testCustomerNpub},
			wantErr:     false,
			msgContains: "Registered customer",
		},
		{
			name:        "duplicate add",
			args:        []string{testCustomerNpub},
			wantErr:     false,
			msgContains: "already registered",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := AddCustomerCmd(ctx, database, tt.args)
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

func TestOrdersCmd(t *testing.T) {
	ctx := context.Background()
	database := setupCmdTestDB(t)

	// Empty orders list
	result := OrdersCmd(ctx, database)
	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}
	if !strings.Contains(result.Message, "No orders found") {
		t.Errorf("expected no orders message, got %q", result.Message)
	}

	// Setup: create customers and inventory
	c1, _ := database.CreateCustomer(ctx, testCustomerNpub)
	c2, _ := database.CreateCustomer(ctx, testAdminNpub)
	_ = database.AddEggs(ctx, 50)

	// Create orders for different customers in different states
	order1, _ := database.CreateOrder(ctx, c1.ID, 6, 3200)  // pending
	order2, _ := database.CreateOrder(ctx, c2.ID, 12, 6400) // will be paid
	_ = database.UpdateOrderStatus(ctx, order2.ID, "paid")

	// List orders
	result = OrdersCmd(ctx, database)
	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}

	// Should show 2 orders
	if !strings.Contains(result.Message, "2 orders") {
		t.Errorf("expected 2 orders, got %q", result.Message)
	}

	// Should show order IDs (format: #1, #2, etc.)
	if !strings.Contains(result.Message, "#") {
		t.Errorf("expected order IDs with # prefix, got %q", result.Message)
	}

	// Should show different statuses
	if !strings.Contains(result.Message, "pending") {
		t.Errorf("expected pending status, got %q", result.Message)
	}
	if !strings.Contains(result.Message, "paid") {
		t.Errorf("expected paid status, got %q", result.Message)
	}

	// Should truncate npubs
	if strings.Contains(result.Message, testCustomerNpub) {
		t.Error("npub should be truncated, not shown in full")
	}

	// Verify both orders are represented
	_ = order1 // Ensure we created both orders
}

func TestRemoveCustomerCmd(t *testing.T) {
	ctx := context.Background()
	database := setupCmdTestDB(t)

	_, _ = database.CreateCustomer(ctx, testCustomerNpub)

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
			name:        "valid remove",
			args:        []string{testCustomerNpub},
			wantErr:     false,
			msgContains: "Removed customer",
		},
		{
			name:        "remove non-existent",
			args:        []string{testCustomerNpub},
			wantErr:     true,
			errContains: "customer not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RemoveCustomerCmd(ctx, database, tt.args)
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

