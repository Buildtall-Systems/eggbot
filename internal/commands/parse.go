package commands

import (
	"strings"
)

// Command represents a parsed user command.
type Command struct {
	Name string   // Command name (lowercase)
	Args []string // Arguments after the command name
}

// Known command names
const (
	// Customer commands
	CmdInventory = "inventory"
	CmdOrder     = "order"
	CmdBalance   = "balance"
	CmdHistory   = "history"
	CmdHelp      = "help"

	// Admin commands
	CmdAdd            = "add"
	CmdDeliver        = "deliver"
	CmdPayment        = "payment"
	CmdAdjust         = "adjust"
	CmdCustomers      = "customers"
	CmdAddCustomer    = "addcustomer"
	CmdRemoveCustomer = "removecustomer"
)

// Parse extracts a command from message content.
// Returns nil if the message is empty or contains only whitespace.
func Parse(content string) *Command {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil
	}

	parts := strings.Fields(content)
	if len(parts) == 0 {
		return nil
	}

	return &Command{
		Name: strings.ToLower(parts[0]),
		Args: parts[1:],
	}
}

// IsCustomerCommand returns true if the command is available to customers.
func (c *Command) IsCustomerCommand() bool {
	switch c.Name {
	case CmdInventory, CmdOrder, CmdBalance, CmdHistory, CmdHelp:
		return true
	default:
		return false
	}
}

// IsAdminCommand returns true if the command requires admin privileges.
func (c *Command) IsAdminCommand() bool {
	switch c.Name {
	case CmdAdd, CmdDeliver, CmdPayment, CmdAdjust, CmdCustomers, CmdAddCustomer, CmdRemoveCustomer:
		return true
	default:
		return false
	}
}

// IsValid returns true if the command name is recognized.
func (c *Command) IsValid() bool {
	return c.IsCustomerCommand() || c.IsAdminCommand()
}
