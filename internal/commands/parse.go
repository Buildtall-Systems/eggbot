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
	CmdCancel    = "cancel"
	CmdBalance   = "balance"
	CmdHistory   = "history"
	CmdHelp      = "help"

	// Admin commands
	CmdDeliver        = "deliver"
	CmdPayment        = "payment"
	CmdAdjust         = "adjust"
	CmdOrders         = "orders"
	CmdCustomers      = "customers"
	CmdAddCustomer    = "addcustomer"
	CmdRemoveCustomer = "removecustomer"
	CmdSales          = "sales"
)

// Parse extracts a command from message content.
// Returns nil if the message is empty or contains only whitespace.
// Strips markdown comment prefixes that some clients (e.g. Amethyst) add.
func Parse(content string) *Command {
	content = stripMarkdownComments(content)
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

// stripMarkdownComments removes markdown reference-style link definitions
// that some Nostr clients prepend to messages, e.g. "[//]: # (nip18)"
func stripMarkdownComments(content string) string {
	lines := strings.Split(content, "\n")
	var result []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Skip markdown comment lines: [//]: # (something) or [//]: ...
		if strings.HasPrefix(trimmed, "[//]:") {
			continue
		}
		result = append(result, line)
	}
	return strings.Join(result, "\n")
}

// IsCustomerCommand returns true if the command is available to customers.
func (c *Command) IsCustomerCommand() bool {
	switch c.Name {
	case CmdInventory, CmdOrder, CmdCancel, CmdBalance, CmdHistory, CmdHelp:
		return true
	default:
		return false
	}
}

// IsAdminCommand returns true if the command requires admin privileges.
func (c *Command) IsAdminCommand() bool {
	switch c.Name {
	case CmdDeliver, CmdPayment, CmdAdjust, CmdOrders, CmdCustomers, CmdAddCustomer, CmdRemoveCustomer, CmdSales:
		return true
	default:
		return false
	}
}

// IsValid returns true if the command name is recognized.
func (c *Command) IsValid() bool {
	return c.IsCustomerCommand() || c.IsAdminCommand()
}
