package lightning

import (
	"context"
)

type Invoice struct {
	PaymentHash    string
	PaymentRequest string
	Memo           string
	AmountSats     int64
	Paid           bool
}

type Backend interface {
	CreateInvoice(ctx context.Context, amountSats int64, memo string) (*Invoice, error)
	CheckInvoicePaid(ctx context.Context, paymentHash string) (bool, error)
}
