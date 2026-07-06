package gopay

import (
	"context"
	"errors"
	"testing"
)

func TestClientGetInvoice(t *testing.T) {
	t.Parallel()
	mock := NewMockProvider().AddInvoice(&Invoice{
		ID:               "inv_1",
		Status:           InvoiceStatusPaid,
		SubscriptionID:   "sub_1",
		HostedInvoiceURL: "https://example.com/i/inv_1",
		Amount:           NewAmount(1999, "USD"),
	})
	client, err := NewClient(mock)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	inv, err := client.GetInvoice(context.Background(), "inv_1")
	if err != nil {
		t.Fatalf("GetInvoice: %v", err)
	}
	if inv.ID != "inv_1" {
		t.Errorf("ID = %s, want inv_1", inv.ID)
	}
	if inv.SubscriptionID != "sub_1" {
		t.Errorf("SubscriptionID = %s, want sub_1", inv.SubscriptionID)
	}
	if !inv.IsPaid() {
		t.Error("IsPaid() = false, want true")
	}
}

func TestClientGetInvoiceEmptyID(t *testing.T) {
	t.Parallel()
	client, err := NewClient(NewMockProvider())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if _, err := client.GetInvoice(context.Background(), ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetInvoice(\"\") err = %v, want ErrNotFound", err)
	}
}

func TestClientGetInvoiceNotFound(t *testing.T) {
	t.Parallel()
	client, err := NewClient(NewMockProvider())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if _, err := client.GetInvoice(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetInvoice(missing) err = %v, want ErrNotFound", err)
	}
}

func TestClientGetInvoiceUnsupported(t *testing.T) {
	t.Parallel()
	client, err := NewClient(baseOnlyProvider{})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if _, err := client.GetInvoice(context.Background(), "inv_1"); !errors.Is(err, ErrUnsupported) {
		t.Errorf("GetInvoice err = %v, want ErrUnsupported", err)
	}
}

func TestInvoiceStatusString(t *testing.T) {
	t.Parallel()
	if InvoiceStatusPaid.String() != "paid" {
		t.Errorf("String() = %s, want paid", InvoiceStatusPaid.String())
	}
}
