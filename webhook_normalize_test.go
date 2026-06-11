package gopay

import (
	"context"
	"errors"
	"testing"
)

func TestMockVerifyWebhookNormalized(t *testing.T) {
	mock := NewMockProvider()
	payload := []byte(`{"id":"evt_1","type":"payment.captured","kind":"payment.succeeded","payment_id":"pay_1","order_id":"order_1","amount":2500,"currency":"USD"}`)

	ev, err := mock.VerifyWebhook(context.Background(), payload, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Kind != WebhookPaymentSucceeded {
		t.Errorf("Kind = %s, want %s", ev.Kind, WebhookPaymentSucceeded)
	}
	if ev.PaymentID != "pay_1" {
		t.Errorf("PaymentID = %s, want pay_1", ev.PaymentID)
	}
	if ev.OrderID != "order_1" {
		t.Errorf("OrderID = %s, want order_1", ev.OrderID)
	}
	if ev.Amount == nil || ev.Amount.Value != 2500 || ev.Amount.Currency != "USD" {
		t.Errorf("Amount = %+v, want {2500 USD}", ev.Amount)
	}
}

func TestMockVerifyWebhookError(t *testing.T) {
	sentinel := errors.New("boom")
	mock := NewMockProvider().WithWebhookError(sentinel)

	ev, err := mock.VerifyWebhook(context.Background(), []byte(`{"id":"evt_1","type":"payment.captured"}`), nil)
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want %v", err, sentinel)
	}
	if ev != nil {
		t.Errorf("event = %+v, want nil on error", ev)
	}
}

func TestMockVerifyWebhookNoAmount(t *testing.T) {
	mock := NewMockProvider()
	// Minimal payload without normalized fields stays backward-compatible.
	payload := []byte(`{"id":"evt_2","type":"payment.completed"}`)

	ev, err := mock.VerifyWebhook(context.Background(), payload, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Kind != WebhookUnknown {
		t.Errorf("Kind = %q, want empty", ev.Kind)
	}
	if ev.Amount != nil {
		t.Errorf("Amount = %+v, want nil", ev.Amount)
	}
}
