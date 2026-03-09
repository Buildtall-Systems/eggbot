package invoices

import (
	"context"
	"log"

	"github.com/Buildtall-Systems/btk/lightning"
	"github.com/buildtall-systems/eggbot/internal/db"
)

type SettledOrder struct {
	Order    db.Order
	Customer *db.Customer
}

type CheckResult struct {
	Settled   []SettledOrder
	Cancelled []db.Order
}

func CheckPendingInvoices(ctx context.Context, database *db.DB, backend lightning.Backend) CheckResult {
	var result CheckResult

	if backend == nil {
		return result
	}

	pending, err := database.GetPendingOrdersWithPaymentHash(ctx)
	if err != nil {
		log.Printf("failed to get pending orders with payment hash: %v", err)
		return result
	}

	for _, order := range pending {
		if !order.PaymentHash.Valid {
			continue
		}

		paid, checkErr := backend.CheckInvoicePaid(ctx, order.PaymentHash.String)
		if checkErr != nil {
			log.Printf("failed to check invoice for order %d: %v", order.ID, checkErr)
			continue
		}

		if !paid {
			continue
		}

		if updateErr := database.UpdateOrderStatus(ctx, order.ID, "paid"); updateErr != nil {
			log.Printf("failed to mark order %d as paid: %v", order.ID, updateErr)
			continue
		}

		customer, custErr := database.GetCustomerByID(ctx, order.CustomerID)
		if custErr != nil {
			log.Printf("failed to get customer for order %d: %v", order.ID, custErr)
			continue
		}

		log.Printf("order %d marked paid via NWC invoice settlement", order.ID)
		result.Settled = append(result.Settled, SettledOrder{Order: order, Customer: customer})
	}

	return result
}
