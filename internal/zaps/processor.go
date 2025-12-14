package zaps

import (
	"context"
	"errors"
	"fmt"

	"github.com/buildtall-systems/eggbot/internal/db"
)

// ProcessResult contains the outcome of processing a zap.
type ProcessResult struct {
	CustomerFound bool   // Whether the sender is a registered customer
	AmountSats    int64  // Amount credited
	Message       string // Human-readable result message
}

// ErrDuplicateZap indicates the zap has already been processed.
var ErrDuplicateZap = errors.New("duplicate zap event")

// ProcessZap records a validated zap payment for a customer.
// Only credits known customers (whitelist check).
// Returns ProcessResult with CustomerFound=false if sender is not a customer.
func ProcessZap(ctx context.Context, database *db.DB, zap *ValidatedZap) (*ProcessResult, error) {
	// Check if customer exists (whitelist check)
	customer, err := database.GetCustomerByNpub(ctx, zap.SenderNpub)
	if errors.Is(err, db.ErrCustomerNotFound) {
		return &ProcessResult{
			CustomerFound: false,
			AmountSats:    zap.AmountSats,
			Message:       fmt.Sprintf("Zap received from unknown sender %s...%s (%d sats) - not credited", zap.SenderNpub[:12], zap.SenderNpub[len(zap.SenderNpub)-4:], zap.AmountSats),
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("checking customer: %w", err)
	}

	// Record the transaction
	_, err = database.RecordTransaction(ctx, nil, zap.ZapEventID, zap.AmountSats, zap.SenderNpub)
	if err != nil {
		// Check for duplicate (unique constraint on zap_event_id)
		if isDuplicateZap(err) {
			return nil, ErrDuplicateZap
		}
		return nil, fmt.Errorf("recording transaction: %w", err)
	}

	// Check for pending orders and attempt to mark as paid
	pendingOrders, err := database.GetPendingOrdersByCustomer(ctx, customer.ID)
	if err != nil {
		// Non-fatal: transaction is recorded, but we couldn't check orders
		return &ProcessResult{
			CustomerFound: true,
			AmountSats:    zap.AmountSats,
			Message:       fmt.Sprintf("Credited %d sats (warning: could not check pending orders)", zap.AmountSats),
		}, nil
	}

	// If customer has pending orders, check if balance covers any
	if len(pendingOrders) > 0 {
		balance, err := database.GetCustomerBalance(ctx, zap.SenderNpub)
		if err != nil {
			return &ProcessResult{
				CustomerFound: true,
				AmountSats:    zap.AmountSats,
				Message:       fmt.Sprintf("Credited %d sats (has %d pending order(s))", zap.AmountSats, len(pendingOrders)),
			}, nil
		}

		// Check if balance covers oldest pending order
		oldestOrder := pendingOrders[len(pendingOrders)-1] // Orders are DESC, so last is oldest
		if balance >= oldestOrder.TotalSats {
			// Mark order as paid
			if err := database.UpdateOrderStatus(ctx, oldestOrder.ID, "paid"); err == nil {
				return &ProcessResult{
					CustomerFound: true,
					AmountSats:    zap.AmountSats,
					Message:       fmt.Sprintf("Credited %d sats - order #%d marked as paid!", zap.AmountSats, oldestOrder.ID),
				}, nil
			}
		}

		return &ProcessResult{
			CustomerFound: true,
			AmountSats:    zap.AmountSats,
			Message:       fmt.Sprintf("Credited %d sats (balance: %d, order needs %d)", zap.AmountSats, balance, pendingOrders[len(pendingOrders)-1].TotalSats),
		}, nil
	}

	return &ProcessResult{
		CustomerFound: true,
		AmountSats:    zap.AmountSats,
		Message:       fmt.Sprintf("Credited %d sats", zap.AmountSats),
	}, nil
}

// isDuplicateZap checks if the error indicates a duplicate zap event ID.
func isDuplicateZap(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return contains(errStr, "UNIQUE constraint failed") || contains(errStr, "zap_event_id")
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
