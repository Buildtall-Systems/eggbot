package commands

import (
	"context"
	"database/sql"
	"fmt"
)

// IsAdmin checks if the given npub is in the admin list.
func IsAdmin(npub string, admins []string) bool {
	for _, admin := range admins {
		if admin == npub {
			return true
		}
	}
	return false
}

// IsCustomer checks if the given npub exists in the customers table
// or is an admin (admins are implicitly customers).
func IsCustomer(ctx context.Context, db *sql.DB, npub string, admins []string) (bool, error) {
	// Admins are implicitly customers
	if IsAdmin(npub, admins) {
		return true, nil
	}

	var exists bool
	err := db.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM customers WHERE npub = ?)",
		npub,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("checking customer: %w", err)
	}

	return exists, nil
}

// CanExecute returns an error if the sender lacks permission to run the command.
// Admins can execute any command. Customers can only execute customer commands.
// Unknown users get an "not a customer" error.
func CanExecute(ctx context.Context, db *sql.DB, cmd *Command, senderNpub string, admins []string) error {
	// Admins can do anything
	if IsAdmin(senderNpub, admins) {
		return nil
	}

	// Check if sender is a customer (admins are implicitly customers)
	isCustomer, err := IsCustomer(ctx, db, senderNpub, admins)
	if err != nil {
		return fmt.Errorf("checking permissions: %w", err)
	}

	if !isCustomer {
		return fmt.Errorf("you are not a registered customer")
	}

	// Customers can only run customer commands
	if cmd.IsAdminCommand() {
		return fmt.Errorf("admin command requires admin privileges")
	}

	return nil
}
