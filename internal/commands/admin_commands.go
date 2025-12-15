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

// DeliverCmd fulfills a specific paid order by ID.
// Args: [order_id]
// Only orders with status='paid' can be delivered.
func DeliverCmd(ctx context.Context, database *db.DB, args []string) Result {
	if len(args) < 1 {
		return Result{Error: errors.New("usage: deliver <order_id>")}
	}

	orderID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return Result{Error: errors.New("order_id must be a number")}
	}

	// Get the order
	order, err := database.GetOrderByID(ctx, orderID)
	if errors.Is(err, db.ErrOrderNotFound) {
		return Result{Error: fmt.Errorf("order %d not found", orderID)}
	}
	if err != nil {
		return Result{Error: fmt.Errorf("looking up order: %w", err)}
	}

	// Verify order is in paid status
	if order.Status != "paid" {
		return Result{Error: fmt.Errorf("order %d is %s, not paid", orderID, order.Status)}
	}

	// Get customer info for response
	customer, err := database.GetCustomerByID(ctx, order.CustomerID)
	if err != nil {
		return Result{Error: fmt.Errorf("looking up customer: %w", err)}
	}

	// Fulfill the order
	if err := database.FulfillOrder(ctx, orderID); err != nil {
		return Result{Error: fmt.Errorf("fulfilling order: %w", err)}
	}

	// Truncate npub for display: npub1abc...xyz
	npubShort := customer.Npub
	if len(npubShort) > 20 {
		npubShort = npubShort[:12] + "..." + npubShort[len(npubShort)-4:]
	}

	return Result{Message: fmt.Sprintf("Delivered order %d: %d eggs to %s", orderID, order.Quantity, npubShort)}
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

	return Result{Message: fmt.Sprintf("Recorded payment of %d sats for %s", amount, npub)}
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
		return Result{Message: fmt.Sprintf("Added %d sats to %s", amount, npub)}
	}
	return Result{Message: fmt.Sprintf("Deducted %d sats from %s", -amount, npub)}
}

// OrdersCmd lists all orders across all customers for admin visibility.
func OrdersCmd(ctx context.Context, database *db.DB) Result {
	orders, err := database.GetAllOrders(ctx, 50)
	if err != nil {
		return Result{Error: fmt.Errorf("listing orders: %w", err)}
	}

	if len(orders) == 0 {
		return Result{Message: "No orders found."}
	}

	msg := fmt.Sprintf("%d orders (most recent first):\n", len(orders))
	for _, o := range orders {
		// Truncate npub for display: npub1abc...xyz
		npubShort := o.CustomerNpub
		if len(npubShort) > 20 {
			npubShort = npubShort[:12] + "..." + npubShort[len(npubShort)-4:]
		}
		msg += fmt.Sprintf("• #%d: %s | %d eggs | %d sats | %s\n",
			o.ID, npubShort, o.Quantity, o.TotalSats, o.Status)
	}
	return Result{Message: msg}
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
		msg += fmt.Sprintf("• %s%s\n", c.Npub, name)
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

	return Result{Message: fmt.Sprintf("Registered customer %s", npub)}
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

	return Result{Message: fmt.Sprintf("Removed customer %s", npub)}
}

// SalesCmd returns total sales from fulfilled orders.
func SalesCmd(ctx context.Context, database *db.DB) Result {
	total, err := database.GetTotalSales(ctx)
	if err != nil {
		return Result{Error: fmt.Errorf("getting total sales: %w", err)}
	}

	if total == 0 {
		return Result{Message: "No sales yet."}
	}

	return Result{Message: fmt.Sprintf("Total sales: %d sats", total)}
}

