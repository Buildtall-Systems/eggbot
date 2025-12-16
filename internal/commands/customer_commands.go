package commands

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"

	"github.com/buildtall-systems/eggbot/internal/db"
	"github.com/buildtall-systems/eggbot/internal/lightning"
)

// Result holds the response from a command execution.
type Result struct {
	Message string
	Error   error
}

// InventoryCmd handles inventory commands.
// No args: show inventory (all users)
// add <n>: add eggs (admin only)
// set <n>: set inventory (admin only)
func InventoryCmd(ctx context.Context, database *db.DB, args []string, isAdmin bool) Result {
	// No subcommand: show inventory
	if len(args) == 0 {
		return showInventory(ctx, database, isAdmin)
	}

	subcommand := args[0]

	switch subcommand {
	case "add":
		if !isAdmin {
			return Result{Error: errors.New("admin access required")}
		}
		return inventoryAdd(ctx, database, args[1:])

	case "set":
		if !isAdmin {
			return Result{Error: errors.New("admin access required")}
		}
		return inventorySet(ctx, database, args[1:])

	default:
		// Unknown subcommand - show inventory for customers, error for attempted admin commands
		if isAdmin {
			return Result{Error: fmt.Errorf("unknown subcommand: %s (use add or set)", subcommand)}
		}
		return showInventory(ctx, database, false)
	}
}

// showInventory returns the current egg count.
// For admins, shows a breakdown of available, reserved (pending), and sold (paid) eggs.
func showInventory(ctx context.Context, database *db.DB, isAdmin bool) Result {
	available, err := database.GetInventory(ctx)
	if err != nil {
		return Result{Error: fmt.Errorf("checking inventory: %w", err)}
	}

	if !isAdmin {
		// Customer view: simple count
		if available == 0 {
			return Result{Message: "No eggs available. Check back later!"}
		}
		if available == 1 {
			return Result{Message: "1 egg available."}
		}
		return Result{Message: fmt.Sprintf("%d eggs available.", available)}
	}

	// Admin view: full breakdown
	reserved, err := database.GetReservedEggs(ctx)
	if err != nil {
		return Result{Error: fmt.Errorf("checking reserved eggs: %w", err)}
	}

	sold, err := database.GetSoldEggs(ctx)
	if err != nil {
		return Result{Error: fmt.Errorf("checking sold eggs: %w", err)}
	}

	onHand := available + reserved + sold
	msg := fmt.Sprintf("Available: %3d eggs (can be sold)\n", available)
	msg += fmt.Sprintf("Reserved:  %3d eggs (pending payment)\n", reserved)
	msg += fmt.Sprintf("Sold:      %3d eggs (awaiting delivery)\n", sold)
	msg += "---\n"
	msg += fmt.Sprintf("On-hand:   %3d eggs (total in storage)", onHand)

	return Result{Message: msg}
}

// inventoryAdd adds eggs to inventory.
func inventoryAdd(ctx context.Context, database *db.DB, args []string) Result {
	if len(args) < 1 {
		return Result{Error: errors.New("usage: inventory add <quantity>")}
	}

	quantity, err := strconv.Atoi(args[0])
	if err != nil || quantity < 1 {
		return Result{Error: errors.New("quantity must be a positive number")}
	}

	if err := database.AddEggs(ctx, quantity); err != nil {
		return Result{Error: fmt.Errorf("adding eggs: %w", err)}
	}

	total, err := database.GetInventory(ctx)
	if err != nil {
		return Result{Message: fmt.Sprintf("Added %d eggs.", quantity)}
	}

	return Result{Message: fmt.Sprintf("Added %d eggs. Total: %d", quantity, total)}
}

// inventorySet sets inventory to an exact count.
func inventorySet(ctx context.Context, database *db.DB, args []string) Result {
	if len(args) < 1 {
		return Result{Error: errors.New("usage: inventory set <quantity>")}
	}

	quantity, err := strconv.Atoi(args[0])
	if err != nil || quantity < 0 {
		return Result{Error: errors.New("quantity must be a non-negative number")}
	}

	if err := database.SetInventory(ctx, quantity); err != nil {
		return Result{Error: fmt.Errorf("setting inventory: %w", err)}
	}

	return Result{Message: fmt.Sprintf("Inventory set to %d eggs.", quantity)}
}

// OrderCmd creates a new order for eggs and reserves inventory atomically.
// Args: [quantity] - must be 6 or 12 (half-dozen or dozen)
func OrderCmd(ctx context.Context, database *db.DB, senderNpub string, args []string, satsPerHalfDozen int, lightningAddress, botNpub string, lnClient *lightning.Client) Result {
	if len(args) < 1 {
		return Result{Error: errors.New("usage: order <quantity> (6 or 12)")}
	}

	quantity, err := strconv.Atoi(args[0])
	if err != nil {
		return Result{Error: errors.New("quantity must be 6 or 12")}
	}

	// Only allow multiples of 6, max 12
	if quantity != 6 && quantity != 12 {
		return Result{Error: errors.New("quantity must be 6 or 12")}
	}

	// Get customer by npub
	customer, err := database.GetCustomerByNpub(ctx, senderNpub)
	if err != nil {
		return Result{Error: fmt.Errorf("looking up customer: %w", err)}
	}

	// Check for pending orders
	pending, err := database.GetPendingOrdersByCustomer(ctx, customer.ID)
	if err != nil {
		return Result{Error: fmt.Errorf("checking pending orders: %w", err)}
	}
	if len(pending) > 0 {
		return Result{Error: fmt.Errorf("you have %d unpaid order(s) - please pay or cancel before ordering more", len(pending))}
	}

	// Calculate price
	halfDozens := quantity / 6
	totalSats := int64(halfDozens * satsPerHalfDozen)

	// Create order (reserves inventory atomically)
	order, err := database.CreateOrder(ctx, customer.ID, quantity, totalSats)
	if err != nil {
		if errors.Is(err, db.ErrInsufficientInventory) {
			// Get current inventory for helpful error message
			available, _ := database.GetInventory(ctx)
			return Result{Error: fmt.Errorf("only %d eggs available, cannot order %d", available, quantity)}
		}
		return Result{Error: fmt.Errorf("creating order: %w", err)}
	}

	msg := fmt.Sprintf("Order %d: %d eggs reserved for %d sats.", order.ID, quantity, totalSats)

	// Generate bolt11 invoice for clickable payment in Amethyst
	var hasInvoice bool
	if lnClient != nil && lightningAddress != "" {
		invoice, err := lnClient.RequestInvoice(ctx, lightningAddress, totalSats)
		if err != nil {
			log.Printf("invoice generation failed: %v", err)
		} else {
			msg += fmt.Sprintf("\n\nPay invoice:\n%s", invoice)
			hasInvoice = true
		}
	}

	// Include zap instructions
	if botNpub != "" {
		if hasInvoice {
			msg += fmt.Sprintf("\n\nOr zap this profile:\nnostr:%s", botNpub)
		} else {
			msg += fmt.Sprintf("\n\nZap this profile to pay:\nnostr:%s", botNpub)
		}
	}

	return Result{Message: msg}
}

// CancelOrderCmd cancels a pending order.
// Args: [order_id]
func CancelOrderCmd(ctx context.Context, database *db.DB, senderNpub string, args []string) Result {
	if len(args) < 1 {
		return Result{Error: errors.New("usage: cancel <order_id>")}
	}

	orderID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return Result{Error: errors.New("order_id must be a number")}
	}

	// Get customer to verify ownership
	customer, err := database.GetCustomerByNpub(ctx, senderNpub)
	if err != nil {
		return Result{Error: fmt.Errorf("looking up customer: %w", err)}
	}

	// Get order to verify ownership
	order, err := database.GetOrderByID(ctx, orderID)
	if err != nil {
		if errors.Is(err, db.ErrOrderNotFound) {
			return Result{Error: fmt.Errorf("order %d not found", orderID)}
		}
		return Result{Error: fmt.Errorf("looking up order: %w", err)}
	}

	// Verify caller owns this order
	if order.CustomerID != customer.ID {
		return Result{Error: errors.New("you can only cancel your own orders")}
	}

	// Cancel the order
	err = database.CancelOrder(ctx, orderID)
	if err != nil {
		if errors.Is(err, db.ErrOrderNotPending) {
			return Result{Error: fmt.Errorf("order %d cannot be cancelled (status: %s)", orderID, order.Status)}
		}
		return Result{Error: fmt.Errorf("cancelling order: %w", err)}
	}

	return Result{Message: fmt.Sprintf("Order %d cancelled.", orderID)}
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

	orders, err := database.GetCustomerOrders(ctx, customer.ID, 25)
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
• order <6|12> - Order eggs (half-dozen or dozen)
• cancel <order_id> - Cancel a pending order
• balance - Check your payment balance
• history - View recent orders
• help - Show this message`

	if isAdmin {
		msg += `

Admin commands:
• inventory add <qty> - Add eggs to inventory
• inventory set <qty> - Set inventory to exact count
• sell <npub> <qty> - Create order for a customer
• markpaid <order_id> - Mark pending order as paid
• deliver <order_id> - Fulfill a paid order
• adjust <npub> <sats> - Adjust customer balance
• orders - List all orders
• customers - List registered customers
• addcustomer <npub> - Register new customer
• removecustomer <npub> - Remove customer
• sales - Show total sales`
	}

	return Result{Message: msg}
}
