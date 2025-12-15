package commands

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// Test keypairs are defined in customer_commands_test.go:
// - testCustomerNpub, testCustomerPubkeyHex
// - testAdminNpub, testAdminPubkeyHex
// - testUnknownNpub, testUnknownPubkeyHex

func TestDeliverCmd(t *testing.T) {
	ctx := context.Background()
	database := setupCmdTestDB(t)

	// Setup customer and inventory
	c, _ := database.CreateCustomer(ctx, testCustomerNpub)
	_ = database.AddEggs(ctx, 30)

	// Create orders in different states for testing
	pendingOrder, _ := database.CreateOrder(ctx, c.ID, 6, 3200)
	paidOrder, _ := database.CreateOrder(ctx, c.ID, 12, 6400)
	_ = database.UpdateOrderStatus(ctx, paidOrder.ID, "paid")

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
			name:        "invalid order_id format",
			args:        []string{"notanumber"},
			wantErr:     true,
			errContains: "order_id must be a number",
		},
		{
			name:        "order not found",
			args:        []string{"9999"},
			wantErr:     true,
			errContains: "order 9999 not found",
		},
		{
			name:        "order not paid",
			args:        []string{fmt.Sprintf("%d", pendingOrder.ID)},
			wantErr:     true,
			errContains: "is pending, not paid",
		},
		{
			name:        "deliver paid order",
			args:        []string{fmt.Sprintf("%d", paidOrder.ID)},
			wantErr:     false,
			msgContains: "Delivered order",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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

func TestDeliverCmd_VerifiesOrderStateChange(t *testing.T) {
	ctx := context.Background()
	database := setupCmdTestDB(t)

	// Setup customer and inventory
	c, _ := database.CreateCustomer(ctx, testCustomerNpub)
	_ = database.AddEggs(ctx, 30)

	// Create a paid order
	order, _ := database.CreateOrder(ctx, c.ID, 12, 6400)
	_ = database.UpdateOrderStatus(ctx, order.ID, "paid")

	// Deliver the order
	result := DeliverCmd(ctx, database, []string{fmt.Sprintf("%d", order.ID)})
	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}

	// Verify message contains order ID and quantity
	if !strings.Contains(result.Message, fmt.Sprintf("Delivered order %d", order.ID)) {
		t.Errorf("expected order ID in message, got %q", result.Message)
	}
	if !strings.Contains(result.Message, "12 eggs") {
		t.Errorf("expected '12 eggs' in message, got %q", result.Message)
	}

	// Verify the order is now fulfilled
	updatedOrder, _ := database.GetOrderByID(ctx, order.ID)
	if updatedOrder.Status != "fulfilled" {
		t.Errorf("expected order to be fulfilled, got %s", updatedOrder.Status)
	}

	// Try delivering again - should fail (already fulfilled)
	result = DeliverCmd(ctx, database, []string{fmt.Sprintf("%d", order.ID)})
	if result.Error == nil {
		t.Fatal("expected error when delivering already fulfilled order")
	}
	if !strings.Contains(result.Error.Error(), "is fulfilled, not paid") {
		t.Errorf("expected 'is fulfilled, not paid' error, got %q", result.Error.Error())
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

func TestSalesCmd(t *testing.T) {
	ctx := context.Background()
	database := setupCmdTestDB(t)

	// No sales yet
	result := SalesCmd(ctx, database)
	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}
	if !strings.Contains(result.Message, "No sales yet") {
		t.Errorf("expected no sales message, got %q", result.Message)
	}

	// Create customer and inventory
	c, _ := database.CreateCustomer(ctx, testCustomerNpub)
	_ = database.AddEggs(ctx, 50)

	// Pending order should not count
	_, _ = database.CreateOrder(ctx, c.ID, 6, 3200)
	result = SalesCmd(ctx, database)
	if !strings.Contains(result.Message, "No sales yet") {
		t.Errorf("pending order should not count as sale, got %q", result.Message)
	}

	// Fulfilled order should count
	order2, _ := database.CreateOrder(ctx, c.ID, 6, 3200)
	_ = database.UpdateOrderStatus(ctx, order2.ID, "paid")
	_ = database.FulfillOrder(ctx, order2.ID)

	result = SalesCmd(ctx, database)
	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}
	if !strings.Contains(result.Message, "Total sales:") {
		t.Errorf("expected total sales message, got %q", result.Message)
	}
	if !strings.Contains(result.Message, "3200 sats") {
		t.Errorf("expected 3200 sats, got %q", result.Message)
	}

	// Multiple fulfilled orders
	order3, _ := database.CreateOrder(ctx, c.ID, 12, 6400)
	_ = database.UpdateOrderStatus(ctx, order3.ID, "paid")
	_ = database.FulfillOrder(ctx, order3.ID)

	result = SalesCmd(ctx, database)
	if !strings.Contains(result.Message, "9600 sats") {
		t.Errorf("expected 9600 sats (3200+6400), got %q", result.Message)
	}
}

