package commands

import (
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantNil  bool
		wantName string
		wantArgs []string
	}{
		{
			name:    "empty string",
			input:   "",
			wantNil: true,
		},
		{
			name:    "whitespace only",
			input:   "   \t\n  ",
			wantNil: true,
		},
		{
			name:     "single command",
			input:    "inventory",
			wantName: "inventory",
			wantArgs: []string{},
		},
		{
			name:     "command with args",
			input:    "order 6",
			wantName: "order",
			wantArgs: []string{"6"},
		},
		{
			name:     "command with multiple args",
			input:    "addcustomer npub1abc123 John",
			wantName: "addcustomer",
			wantArgs: []string{"npub1abc123", "John"},
		},
		{
			name:     "uppercase normalized to lowercase",
			input:    "INVENTORY",
			wantName: "inventory",
			wantArgs: []string{},
		},
		{
			name:     "mixed case normalized",
			input:    "AddCustomer npub1xyz",
			wantName: "addcustomer",
			wantArgs: []string{"npub1xyz"},
		},
		{
			name:     "leading/trailing whitespace trimmed",
			input:    "  help  ",
			wantName: "help",
			wantArgs: []string{},
		},
		{
			name:     "unknown command parses (validity checked separately)",
			input:    "foobar arg1",
			wantName: "foobar",
			wantArgs: []string{"arg1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.input)

			if tt.wantNil {
				if got != nil {
					t.Errorf("Parse(%q) = %+v, want nil", tt.input, got)
				}
				return
			}

			if got == nil {
				t.Fatalf("Parse(%q) = nil, want command", tt.input)
			}

			if got.Name != tt.wantName {
				t.Errorf("Parse(%q).Name = %q, want %q", tt.input, got.Name, tt.wantName)
			}

			if len(got.Args) != len(tt.wantArgs) {
				t.Errorf("Parse(%q).Args = %v, want %v", tt.input, got.Args, tt.wantArgs)
			} else {
				for i, arg := range got.Args {
					if arg != tt.wantArgs[i] {
						t.Errorf("Parse(%q).Args[%d] = %q, want %q", tt.input, i, arg, tt.wantArgs[i])
					}
				}
			}
		})
	}
}

func TestCommand_IsCustomerCommand(t *testing.T) {
	customerCmds := []string{CmdInventory, CmdOrder, CmdBalance, CmdHistory, CmdHelp}
	adminCmds := []string{CmdAdd, CmdDeliver, CmdPayment, CmdAdjust, CmdCustomers, CmdAddCustomer, CmdRemoveCustomer}

	for _, name := range customerCmds {
		cmd := &Command{Name: name}
		if !cmd.IsCustomerCommand() {
			t.Errorf("Command{Name: %q}.IsCustomerCommand() = false, want true", name)
		}
	}

	for _, name := range adminCmds {
		cmd := &Command{Name: name}
		if cmd.IsCustomerCommand() {
			t.Errorf("Command{Name: %q}.IsCustomerCommand() = true, want false", name)
		}
	}
}

func TestCommand_IsAdminCommand(t *testing.T) {
	customerCmds := []string{CmdInventory, CmdOrder, CmdBalance, CmdHistory, CmdHelp}
	adminCmds := []string{CmdAdd, CmdDeliver, CmdPayment, CmdAdjust, CmdCustomers, CmdAddCustomer, CmdRemoveCustomer}

	for _, name := range adminCmds {
		cmd := &Command{Name: name}
		if !cmd.IsAdminCommand() {
			t.Errorf("Command{Name: %q}.IsAdminCommand() = false, want true", name)
		}
	}

	for _, name := range customerCmds {
		cmd := &Command{Name: name}
		if cmd.IsAdminCommand() {
			t.Errorf("Command{Name: %q}.IsAdminCommand() = true, want false", name)
		}
	}
}

func TestCommand_IsValid(t *testing.T) {
	validCmds := []string{
		CmdInventory, CmdOrder, CmdBalance, CmdHistory, CmdHelp,
		CmdAdd, CmdDeliver, CmdPayment, CmdAdjust, CmdCustomers, CmdAddCustomer, CmdRemoveCustomer,
	}

	for _, name := range validCmds {
		cmd := &Command{Name: name}
		if !cmd.IsValid() {
			t.Errorf("Command{Name: %q}.IsValid() = false, want true", name)
		}
	}

	invalidCmds := []string{"foobar", "unknown", "exec", ""}
	for _, name := range invalidCmds {
		cmd := &Command{Name: name}
		if cmd.IsValid() {
			t.Errorf("Command{Name: %q}.IsValid() = true, want false", name)
		}
	}
}
