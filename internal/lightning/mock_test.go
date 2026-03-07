package lightning

import (
	"context"
	"testing"
)

func TestMockBackendRoundtrip(t *testing.T) {
	ctx := context.Background()
	m := NewMockBackend()

	inv, err := m.CreateInvoice(ctx, 100, "test invoice")
	if err != nil {
		t.Fatal(err)
	}
	if inv.AmountSats != 100 {
		t.Errorf("amount = %d, want 100", inv.AmountSats)
	}
	if inv.PaymentHash == "" {
		t.Fatal("payment hash should not be empty")
	}

	paid, err := m.CheckInvoicePaid(ctx, inv.PaymentHash)
	if err != nil {
		t.Fatal(err)
	}
	if paid {
		t.Error("invoice should not be paid yet")
	}

	if markErr := m.MarkPaid(inv.PaymentHash); markErr != nil {
		t.Fatal(markErr)
	}

	paid, err = m.CheckInvoicePaid(ctx, inv.PaymentHash)
	if err != nil {
		t.Fatal(err)
	}
	if !paid {
		t.Error("invoice should be paid after MarkPaid")
	}
}

func TestMockBackendNotFound(t *testing.T) {
	ctx := context.Background()
	m := NewMockBackend()

	_, err := m.CheckInvoicePaid(ctx, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent invoice")
	}

	err = m.MarkPaid("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent invoice")
	}
}
