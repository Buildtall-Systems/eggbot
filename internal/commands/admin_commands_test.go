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
			name:        "no pending orders",
			args:        []string{testCustomerNpub},
			setup:       func() {},
			wantErr:     false,
			msgContains: "No pending orders",
		},
		{
			name: "deliver pending order",
			args: []string{testCustomerNpub},
			setup: func() {
				_ = database.AddEggs(ctx, 10)
				_, _ = database.CreateOrder(ctx, c.ID, 6, 3200)
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

func TestShortenNpub(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"short", "short"},
		// Using properly generated npub - last 4 chars are "8ejt"
		{testCustomerNpub, "npub1rm9q804...8ejt"},
	}

	for _, tt := range tests {
		result := shortenNpub(tt.input)
		if result != tt.expected {
			t.Errorf("shortenNpub(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}
