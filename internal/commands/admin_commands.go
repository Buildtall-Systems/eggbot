package commands

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/buildtall-systems/eggbot/internal/db"
	"github.com/nbd-wtf/go-nostr/nip19"
)

// AddEggsCmd adds eggs to inventory.
// Args: [quantity]
func AddEggsCmd(ctx context.Context, database *db.DB, args []string) Result {
	if len(args) < 1 {
		return Result{Error: errors.New("usage: add <quantity>")}
	}

	quantity, err := strconv.Atoi(args[0])
	if err != nil || quantity < 1 {
		return Result{Error: errors.New("quantity must be a positive number")}
	}

	if err := database.AddEggs(ctx, quantity); err != nil {
		return Result{Error: fmt.Errorf("adding eggs: %w", err)}
	}

	// Get new total
	total, err := database.GetInventory(ctx)
	if err != nil {
		return Result{Message: fmt.Sprintf("Added %d eggs.", quantity)}
	}

	return Result{Message: fmt.Sprintf("Added %d eggs. Total: %d", quantity, total)}
}

// DeliverCmd fulfills pending orders for a customer.
// Args: [npub]
func DeliverCmd(ctx context.Context, database *db.DB, args []string) Result {
	if len(args) < 1 {
		return Result{Error: errors.New("usage: deliver <npub>")}
	}

	npub := args[0]
	if !strings.HasPrefix(npub, "npub1") {
		return Result{Error: errors.New("invalid npub format")}
	}

	// Validate npub
	prefix, _, err := nip19.Decode(npub)
	if err != nil || prefix != "npub" {
		return Result{Error: errors.New("invalid npub")}
	}

	customer, err := database.GetCustomerByNpub(ctx, npub)
	if errors.Is(err, db.ErrCustomerNotFound) {
		return Result{Error: errors.New("customer not found")}
	}
	if err != nil {
		return Result{Error: fmt.Errorf("looking up customer: %w", err)}
	}

	// Get pending orders
	orders, err := database.GetPendingOrdersByCustomer(ctx, customer.ID)
	if err != nil {
		return Result{Error: fmt.Errorf("getting pending orders: %w", err)}
	}

	if len(orders) == 0 {
		return Result{Message: "No pending orders for this customer."}
	}

	// Fulfill each order
	var fulfilled int
	var totalEggs int
	var errs []string
	for _, order := range orders {
		if err := database.FulfillOrder(ctx, order.ID); err != nil {
			if errors.Is(err, db.ErrInsufficientInventory) {
				errs = append(errs, fmt.Sprintf("order #%d: insufficient inventory", order.ID))
			} else {
				errs = append(errs, fmt.Sprintf("order #%d: %v", order.ID, err))
			}
			continue
		}
		fulfilled++
		totalEggs += order.Quantity
	}

	msg := fmt.Sprintf("Delivered %d orders (%d eggs).", fulfilled, totalEggs)
	if len(errs) > 0 {
		msg += "\nErrors: " + strings.Join(errs, "; ")
	}
	return Result{Message: msg}
}

// PaymentCmd records a manual payment for a customer.
// Args: [npub] [amount_sats]
func PaymentCmd(ctx context.Context, database *db.DB, args []string) Result {
	if len(args) < 2 {
		return Result{Error: errors.New("usage: payment <npub> <sats>")}
	}

	npub := args[0]
	if !strings.HasPrefix(npub, "npub1") {
		return Result{Error: errors.New("invalid npub format")}
	}

	amount, err := strconv.ParseInt(args[1], 10, 64)
	if err != nil || amount < 1 {
		return Result{Error: errors.New("amount must be a positive number")}
	}

	// Validate npub
	prefix, _, err := nip19.Decode(npub)
	if err != nil || prefix != "npub" {
		return Result{Error: errors.New("invalid npub")}
	}

	// Verify customer exists
	_, err = database.GetCustomerByNpub(ctx, npub)
	if errors.Is(err, db.ErrCustomerNotFound) {
		return Result{Error: errors.New("customer not found")}
	}
	if err != nil {
		return Result{Error: fmt.Errorf("looking up customer: %w", err)}
	}

	// Record transaction with a synthetic event ID
	eventID := fmt.Sprintf("manual-%d", amount)
	_, err = database.RecordTransaction(ctx, nil, eventID, amount, npub)
	if err != nil {
		return Result{Error: fmt.Errorf("recording payment: %w", err)}
	}

	return Result{Message: fmt.Sprintf("Recorded payment of %d sats for %s", amount, shortenNpub(npub))}
}

// AdjustCmd adjusts a customer's balance (can be negative).
// Args: [npub] [amount_sats]
func AdjustCmd(ctx context.Context, database *db.DB, args []string) Result {
	if len(args) < 2 {
		return Result{Error: errors.New("usage: adjust <npub> <sats>")}
	}

	npub := args[0]
	if !strings.HasPrefix(npub, "npub1") {
		return Result{Error: errors.New("invalid npub format")}
	}

	amount, err := strconv.ParseInt(args[1], 10, 64)
	if err != nil {
		return Result{Error: errors.New("amount must be a number (can be negative)")}
	}

	// Validate npub
	prefix, _, err := nip19.Decode(npub)
	if err != nil || prefix != "npub" {
		return Result{Error: errors.New("invalid npub")}
	}

	// Verify customer exists
	_, err = database.GetCustomerByNpub(ctx, npub)
	if errors.Is(err, db.ErrCustomerNotFound) {
		return Result{Error: errors.New("customer not found")}
	}
	if err != nil {
		return Result{Error: fmt.Errorf("looking up customer: %w", err)}
	}

	// Record adjustment transaction
	eventID := fmt.Sprintf("adjust-%d", amount)
	_, err = database.RecordTransaction(ctx, nil, eventID, amount, npub)
	if err != nil {
		return Result{Error: fmt.Errorf("recording adjustment: %w", err)}
	}

	if amount >= 0 {
		return Result{Message: fmt.Sprintf("Added %d sats to %s", amount, shortenNpub(npub))}
	}
	return Result{Message: fmt.Sprintf("Deducted %d sats from %s", -amount, shortenNpub(npub))}
}

// CustomersCmd lists all registered customers.
func CustomersCmd(ctx context.Context, database *db.DB) Result {
	customers, err := database.ListCustomers(ctx)
	if err != nil {
		return Result{Error: fmt.Errorf("listing customers: %w", err)}
	}

	if len(customers) == 0 {
		return Result{Message: "No registered customers."}
	}

	msg := fmt.Sprintf("%d registered customers:\n", len(customers))
	for _, c := range customers {
		name := ""
		if c.Name.Valid && c.Name.String != "" {
			name = fmt.Sprintf(" (%s)", c.Name.String)
		}
		msg += fmt.Sprintf("• %s%s\n", shortenNpub(c.Npub), name)
	}
	return Result{Message: msg}
}

// AddCustomerCmd registers a new customer.
// Args: [npub]
func AddCustomerCmd(ctx context.Context, database *db.DB, args []string) Result {
	if len(args) < 1 {
		return Result{Error: errors.New("usage: addcustomer <npub>")}
	}

	npub := args[0]
	if !strings.HasPrefix(npub, "npub1") {
		return Result{Error: errors.New("invalid npub format")}
	}

	// Validate npub
	prefix, _, err := nip19.Decode(npub)
	if err != nil || prefix != "npub" {
		return Result{Error: errors.New("invalid npub")}
	}

	_, err = database.CreateCustomer(ctx, npub)
	if errors.Is(err, db.ErrCustomerExists) {
		return Result{Message: "Customer already registered."}
	}
	if err != nil {
		return Result{Error: fmt.Errorf("adding customer: %w", err)}
	}

	return Result{Message: fmt.Sprintf("Registered customer %s", shortenNpub(npub))}
}

// RemoveCustomerCmd removes a customer.
// Args: [npub]
func RemoveCustomerCmd(ctx context.Context, database *db.DB, args []string) Result {
	if len(args) < 1 {
		return Result{Error: errors.New("usage: removecustomer <npub>")}
	}

	npub := args[0]
	if !strings.HasPrefix(npub, "npub1") {
		return Result{Error: errors.New("invalid npub format")}
	}

	// Validate npub
	prefix, _, err := nip19.Decode(npub)
	if err != nil || prefix != "npub" {
		return Result{Error: errors.New("invalid npub")}
	}

	err = database.RemoveCustomer(ctx, npub)
	if errors.Is(err, db.ErrCustomerNotFound) {
		return Result{Error: errors.New("customer not found")}
	}
	if err != nil {
		return Result{Error: fmt.Errorf("removing customer: %w", err)}
	}

	return Result{Message: fmt.Sprintf("Removed customer %s", shortenNpub(npub))}
}

// shortenNpub returns a shortened version of an npub for display.
func shortenNpub(npub string) string {
	if len(npub) < 20 {
		return npub
	}
	return npub[:12] + "..." + npub[len(npub)-4:]
}
