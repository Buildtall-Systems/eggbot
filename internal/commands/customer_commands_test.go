package commands

import (
	"context"
	"database/sql"
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
	testAdminNpub      = "npub17290s82wy9g0mu3kd5mn5yjmygq5896xptk4x7ehpfvdz9k459vqywh6q7"

	// Unknown user keypair (not registered)
	testUnknownSecretHex = "d067b66a004de257ff3f467e754d22bb2b64a9a59c669e8224d8c624b7decb4f"
	testUnknownPubkeyHex = "dcfafaaebf643e0c8517e49e13ad25c60ee4a57a0b5f5fc401adbcb9d151f5f5"
	testUnknownNpub      = "npub1mna04t4lvslqepghuj0p8tf9cc8wfft6pd04l3qp4k7tn5237h6sj6ru9w"
)

func TestInventoryCmd(t *testing.T) {
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
			result := InventoryCmd(ctx, database)
			if result.Error != nil {
				t.Fatalf("unexpected error: %v", result.Error)
			}
			if !strings.Contains(result.Message, tt.contains) {
				t.Errorf("expected message containing %q, got %q", tt.contains, result.Message)
			}
		})
	}
}

func TestOrderCmd(t *testing.T) {
	ctx := context.Background()
	database := setupCmdTestDB(t)

	// Setup: add inventory and customer using properly generated keypair
	_ = database.AddEggs(ctx, 20)
	_, _ = database.CreateCustomer(ctx, testCustomerNpub)

	tests := []struct {
		name      string
		args      []string
		wantErr   bool
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
			name:        "invalid quantity",
			args:        []string{"abc"},
			wantErr:     true,
			errContains: "positive number",
		},
		{
			name:        "zero quantity",
			args:        []string{"0"},
			wantErr:     true,
			errContains: "positive number",
		},
		{
			name:        "valid order 6 eggs",
			args:        []string{"6"},
			wantErr:     false,
			msgContains: "Order #",
		},
		{
			name:        "valid order 7 eggs rounds up",
			args:        []string{"7"},
			wantErr:     false,
			msgContains: "6400 sats", // 2 half-dozens
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := OrderCmd(ctx, database, testCustomerNpub, tt.args, 3200, "")
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

func TestOrderCmd_InsufficientInventory(t *testing.T) {
	ctx := context.Background()
	database := setupCmdTestDB(t)

	// Setup: only 5 eggs, customer orders 10
	_ = database.AddEggs(ctx, 5)
	_, _ = database.CreateCustomer(ctx, testCustomerNpub)

	result := OrderCmd(ctx, database, testCustomerNpub, []string{"10"}, 3200, "")
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

	// Fulfill an order to test spent
	_ = database.AddEggs(ctx, 10)
	order, _ := database.CreateOrder(ctx, c.ID, 6, 3200)
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

	// Create orders
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
