package commands

import (
	"context"
	"strings"
	"testing"
)

// Test npubs are defined in customer_commands_test.go:
// - testCustomerNpub
// - testAdminNpub
// - testUnknownNpub

func TestExecute(t *testing.T) {
	ctx := context.Background()
	database := setupCmdTestDB(t)

	// Setup: create customer and add inventory using properly generated keypairs
	_, _ = database.CreateCustomer(ctx, testCustomerNpub)
	_ = database.AddEggs(ctx, 20)

	cfg := ExecuteConfig{
		SatsPerHalfDozen: 3200,
		Admins:           []string{testAdminNpub},
	}

	tests := []struct {
		name        string
		cmd         *Command
		npub        string
		wantErr     bool
		msgContains string
	}{
		{
			name:        "inventory command",
			cmd:         &Command{Name: CmdInventory, Args: []string{}},
			npub:        testCustomerNpub,
			wantErr:     false,
			msgContains: "eggs available",
		},
		{
			name:        "help command customer",
			cmd:         &Command{Name: CmdHelp, Args: []string{}},
			npub:        testCustomerNpub,
			wantErr:     false,
			msgContains: "Available commands",
		},
		{
			name:        "help command admin",
			cmd:         &Command{Name: CmdHelp, Args: []string{}},
			npub:        testAdminNpub,
			wantErr:     false,
			msgContains: "Admin commands",
		},
		{
			name:        "order command",
			cmd:         &Command{Name: CmdOrder, Args: []string{"6"}},
			npub:        testCustomerNpub,
			wantErr:     false,
			msgContains: "Order ",
		},
		{
			name:        "order command missing args",
			cmd:         &Command{Name: CmdOrder, Args: []string{}},
			npub:        testCustomerNpub,
			wantErr:     true,
			msgContains: "",
		},
		{
			name:        "balance command",
			cmd:         &Command{Name: CmdBalance, Args: []string{}},
			npub:        testCustomerNpub,
			wantErr:     false,
			msgContains: "No payments received",
		},
		{
			name:        "history command",
			cmd:         &Command{Name: CmdHistory, Args: []string{}},
			npub:        testCustomerNpub,
			wantErr:     false,
			msgContains: "", // Could be "No orders" or orders list
		},
		{
			name:        "inventory add command (admin)",
			cmd:         &Command{Name: CmdInventory, Args: []string{"add", "5"}},
			npub:        testAdminNpub,
			wantErr:     false,
			msgContains: "Added 5 eggs",
		},
		{
			name:        "customers command (admin)",
			cmd:         &Command{Name: CmdCustomers, Args: []string{}},
			npub:        testAdminNpub,
			wantErr:     false,
			msgContains: "registered customers",
		},
		{
			name:        "unknown command returns help",
			cmd:         &Command{Name: "unknown", Args: []string{}},
			npub:        testCustomerNpub,
			wantErr:     false,
			msgContains: "Available commands",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Execute(ctx, database, tt.cmd, tt.npub, cfg)
			if tt.wantErr {
				if result.Error == nil {
					t.Fatal("expected error")
				}
			} else {
				if result.Error != nil {
					t.Fatalf("unexpected error: %v", result.Error)
				}
				if tt.msgContains != "" && !strings.Contains(result.Message, tt.msgContains) {
					t.Errorf("expected message containing %q, got %q", tt.msgContains, result.Message)
				}
			}
		})
	}
}

func TestExecute_AllCommands(t *testing.T) {
	// Test that all command constants are handled
	ctx := context.Background()
	database := setupCmdTestDB(t)

	_, _ = database.CreateCustomer(ctx, testCustomerNpub)
	_ = database.AddEggs(ctx, 100)

	cfg := ExecuteConfig{
		SatsPerHalfDozen: 3200,
		Admins:           []string{testCustomerNpub}, // Make customer also admin for testing
	}

	commands := []string{
		CmdInventory, CmdOrder, CmdBalance, CmdHistory, CmdHelp,
		CmdDeliver, CmdPayment, CmdAdjust,
		CmdCustomers, CmdAddCustomer, CmdRemoveCustomer,
	}

	for _, cmdName := range commands {
		t.Run(cmdName, func(t *testing.T) {
			// Create command with minimal valid args where needed
			args := []string{}
			switch cmdName {
			case CmdOrder:
				args = []string{"1"}
			case CmdDeliver, CmdAddCustomer, CmdRemoveCustomer:
				args = []string{testCustomerNpub}
			case CmdPayment, CmdAdjust:
				args = []string{testCustomerNpub, "100"}
			}

			cmd := &Command{Name: cmdName, Args: args}
			result := Execute(ctx, database, cmd, testCustomerNpub, cfg)

			// Just verify no panic and we get a response
			if result.Message == "" && result.Error == nil {
				t.Errorf("command %s returned empty result", cmdName)
			}
		})
	}
}
