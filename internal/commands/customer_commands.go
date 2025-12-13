package commands

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/buildtall-systems/eggbot/internal/db"
)

// Result holds the response from a command execution.
type Result struct {
	Message string
	Error   error
}

// InventoryCmd returns the current egg inventory.
func InventoryCmd(ctx context.Context, database *db.DB) Result {
	count, err := database.GetInventory(ctx)
	if err != nil {
		return Result{Error: fmt.Errorf("checking inventory: %w", err)}
	}

	if count == 0 {
		return Result{Message: "No eggs available. Check back later!"}
	}
	if count == 1 {
		return Result{Message: "1 egg available."}
	}
	return Result{Message: fmt.Sprintf("%d eggs available.", count)}
}

// OrderCmd creates a new order for eggs.
// Args: [quantity]
func OrderCmd(ctx context.Context, database *db.DB, senderNpub string, args []string, satsPerHalfDozen int) Result {
	if len(args) < 1 {
		return Result{Error: errors.New("usage: order <quantity>")}
	}

	quantity, err := strconv.Atoi(args[0])
	if err != nil || quantity < 1 {
		return Result{Error: errors.New("quantity must be a positive number")}
	}

	// Check inventory
	available, err := database.GetInventory(ctx)
	if err != nil {
		return Result{Error: fmt.Errorf("checking inventory: %w", err)}
	}
	if available < quantity {
		return Result{Error: fmt.Errorf("only %d eggs available, cannot order %d", available, quantity)}
	}

	// Get customer by npub (hex pubkey needs to be converted to npub first by caller)
	customer, err := database.GetCustomerByNpub(ctx, senderNpub)
	if err != nil {
		return Result{Error: fmt.Errorf("looking up customer: %w", err)}
	}

	// Calculate price: satsPerHalfDozen per 6 eggs, rounded up
	halfDozens := (quantity + 5) / 6 // Round up
	totalSats := int64(halfDozens * satsPerHalfDozen)

	// Create order
	order, err := database.CreateOrder(ctx, customer.ID, quantity, totalSats)
	if err != nil {
		return Result{Error: fmt.Errorf("creating order: %w", err)}
	}

	return Result{Message: fmt.Sprintf("Order #%d created: %d eggs for %d sats. Send a zap to confirm!", order.ID, quantity, totalSats)}
}

// BalanceCmd returns the customer's balance (received payments minus spent on fulfilled orders).
func BalanceCmd(ctx context.Context, database *db.DB, senderNpub string) Result {
	customer, err := database.GetCustomerByNpub(ctx, senderNpub)
	if err != nil {
		return Result{Error: fmt.Errorf("looking up customer: %w", err)}
	}

	received, err := database.GetCustomerBalance(ctx, senderNpub)
	if err != nil {
		return Result{Error: fmt.Errorf("getting received: %w", err)}
	}

	spent, err := database.GetCustomerSpent(ctx, customer.ID)
	if err != nil {
		return Result{Error: fmt.Errorf("getting spent: %w", err)}
	}

	balance := received - spent

	if balance == 0 && received == 0 {
		return Result{Message: "No payments received yet."}
	}

	return Result{Message: fmt.Sprintf("Received: %d sats | Spent: %d sats | Balance: %d sats", received, spent, balance)}
}

// HistoryCmd returns the customer's recent order history.
func HistoryCmd(ctx context.Context, database *db.DB, senderNpub string) Result {
	customer, err := database.GetCustomerByNpub(ctx, senderNpub)
	if err != nil {
		return Result{Error: fmt.Errorf("looking up customer: %w", err)}
	}

	orders, err := database.GetCustomerOrders(ctx, customer.ID, 5)
	if err != nil {
		return Result{Error: fmt.Errorf("getting orders: %w", err)}
	}

	if len(orders) == 0 {
		return Result{Message: "No orders yet."}
	}

	msg := "Recent orders:\n"
	for _, o := range orders {
		msg += fmt.Sprintf("• #%d: %d eggs, %d sats (%s)\n", o.ID, o.Quantity, o.TotalSats, o.Status)
	}
	return Result{Message: msg}
}

// HelpCmd returns available commands for the user.
func HelpCmd(isAdmin bool) Result {
	msg := `Available commands:
• inventory - Check egg availability
• order <qty> - Order eggs
• balance - Check your payment balance
• history - View recent orders
• help - Show this message`

	if isAdmin {
		msg += `

Admin commands:
• add <qty> - Add eggs to inventory
• deliver <npub> - Fulfill customer's pending orders
• payment <npub> <sats> - Record manual payment
• adjust <npub> <sats> - Adjust customer balance
• customers - List registered customers
• addcustomer <npub> - Register new customer
• removecustomer <npub> - Remove customer`
	}

	return Result{Message: msg}
}
