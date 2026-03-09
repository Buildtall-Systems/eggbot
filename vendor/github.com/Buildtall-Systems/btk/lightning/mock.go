package lightning

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

type MockBackend struct {
	invoices map[string]*Invoice
	mu       sync.Mutex
}

func NewMockBackend() *MockBackend {
	return &MockBackend{
		invoices: make(map[string]*Invoice),
	}
}

func (m *MockBackend) CreateInvoice(_ context.Context, amountSats int64, memo string) (*Invoice, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	hash := generateHash()
	inv := &Invoice{
		PaymentHash:    hash,
		PaymentRequest: "lnbc" + hash[:20],
		AmountSats:     amountSats,
		Memo:           memo,
		Paid:           false,
		ExpiresAt:      time.Now().Add(1 * time.Hour).Unix(),
	}
	m.invoices[hash] = inv
	return inv, nil
}

func (m *MockBackend) CheckInvoicePaid(_ context.Context, paymentHash string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	inv, ok := m.invoices[paymentHash]
	if !ok {
		return false, fmt.Errorf("invoice not found: %s", paymentHash)
	}
	return inv.Paid, nil
}

func (m *MockBackend) MarkPaid(paymentHash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	inv, ok := m.invoices[paymentHash]
	if !ok {
		return fmt.Errorf("invoice not found: %s", paymentHash)
	}
	inv.Paid = true
	inv.SettledAt = time.Now().Unix()
	return nil
}

func generateHash() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("failed to generate random bytes: %v", err))
	}
	return hex.EncodeToString(b)
}
