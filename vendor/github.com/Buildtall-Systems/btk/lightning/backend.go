package lightning

import (
	"context"

	"github.com/nbd-wtf/go-nostr"
)

type Invoice struct {
	PaymentHash    string
	PaymentRequest string
	Memo           string
	AmountSats     int64
	Paid           bool
	ExpiresAt      int64
	SettledAt      int64
}

type Backend interface {
	CreateInvoice(ctx context.Context, amountSats int64, memo string) (*Invoice, error)
	CheckInvoicePaid(ctx context.Context, paymentHash string) (bool, error)
}

type Encryption int

const (
	EncryptionNIP04 Encryption = iota
	EncryptionNIP44
)

type NWCOption func(*NWCBackend)

func WithEncryption(enc Encryption) NWCOption {
	return func(b *NWCBackend) {
		b.encryption = enc
	}
}

func WithPool(pool *nostr.SimplePool) NWCOption {
	return func(b *NWCBackend) {
		b.pool = pool
	}
}
