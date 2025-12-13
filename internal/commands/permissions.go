package commands

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/nbd-wtf/go-nostr/nip19"
)

// IsAdmin checks if the given pubkey (hex) is in the admin list (npubs).
func IsAdmin(pubkeyHex string, admins []string) bool {
	// Convert hex pubkey to npub for comparison
	npub, err := nip19.EncodePublicKey(pubkeyHex)
	if err != nil {
		return false
	}

	for _, admin := range admins {
		if admin == npub {
			return true
		}
	}
	return false
}

// IsCustomer checks if the given pubkey (hex) exists in the customers table.
func IsCustomer(ctx context.Context, db *sql.DB, pubkeyHex string) (bool, error) {
	// Convert hex pubkey to npub for database lookup
	npub, err := nip19.EncodePublicKey(pubkeyHex)
	if err != nil {
		return false, fmt.Errorf("encoding pubkey: %w", err)
	}

	var exists bool
	err = db.QueryRowContext(ctx,
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
func CanExecute(ctx context.Context, db *sql.DB, cmd *Command, senderPubkeyHex string, admins []string) error {
	// Admins can do anything
	if IsAdmin(senderPubkeyHex, admins) {
		return nil
	}

	// Check if sender is a customer
	isCustomer, err := IsCustomer(ctx, db, senderPubkeyHex)
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
