package paypal

import (
	"testing"

	"github.com/KARTIKrocks/gopay"
)

func TestParseWebhookNormalized(t *testing.T) {
	tests := []struct {
		name          string
		payload       string
		wantKind      gopay.WebhookEventKind
		wantPaymentID string
		wantOrderID   string
		wantRefundID  string
		wantAmount    *gopay.Amount
	}{
		{
			name:          "capture completed converts major units",
			payload:       `{"id":"WH-1","event_type":"PAYMENT.CAPTURE.COMPLETED","resource":{"id":"CAP-1","amount":{"value":"10.00","currency_code":"USD"},"supplementary_data":{"related_ids":{"order_id":"ORDER-1"}}}}`,
			wantKind:      gopay.WebhookPaymentSucceeded,
			wantPaymentID: "CAP-1",
			wantOrderID:   "ORDER-1",
			wantAmount:    gopay.NewAmount(1000, "USD"),
		},
		{
			name:          "capture completed zero-decimal currency",
			payload:       `{"id":"WH-2","event_type":"PAYMENT.CAPTURE.COMPLETED","resource":{"id":"CAP-2","amount":{"value":"500","currency_code":"JPY"}}}`,
			wantKind:      gopay.WebhookPaymentSucceeded,
			wantPaymentID: "CAP-2",
			wantAmount:    gopay.NewAmount(500, "JPY"),
		},
		{
			name:          "capture denied",
			payload:       `{"id":"WH-3","event_type":"PAYMENT.CAPTURE.DENIED","resource":{"id":"CAP-3","amount":{"value":"4.20","currency_code":"USD"}}}`,
			wantKind:      gopay.WebhookPaymentFailed,
			wantPaymentID: "CAP-3",
			wantAmount:    gopay.NewAmount(420, "USD"),
		},
		{
			name:         "capture refunded sets refund id",
			payload:      `{"id":"WH-4","event_type":"PAYMENT.CAPTURE.REFUNDED","resource":{"id":"REF-1","amount":{"value":"3.50","currency_code":"USD"}}}`,
			wantKind:     gopay.WebhookRefundSucceeded,
			wantRefundID: "REF-1",
			wantAmount:   gopay.NewAmount(350, "USD"),
		},
		{
			name:         "refund failed",
			payload:      `{"id":"WH-4b","event_type":"PAYMENT.REFUND.FAILED","resource":{"id":"REF-2","amount":{"value":"3.50","currency_code":"USD"}}}`,
			wantKind:     gopay.WebhookRefundFailed,
			wantRefundID: "REF-2",
			wantAmount:   gopay.NewAmount(350, "USD"),
		},
		{
			name:          "unknown currency leaves amount nil",
			payload:       `{"id":"WH-5","event_type":"PAYMENT.CAPTURE.COMPLETED","resource":{"id":"CAP-5","amount":{"value":"5.00","currency_code":"XYZ"}}}`,
			wantKind:      gopay.WebhookPaymentSucceeded,
			wantPaymentID: "CAP-5",
			wantAmount:    nil,
		},
		{
			name:          "unmapped event",
			payload:       `{"id":"WH-6","event_type":"BILLING.SUBSCRIPTION.CREATED","resource":{"id":"SUB-1"}}`,
			wantKind:      gopay.WebhookUnknown,
			wantPaymentID: "SUB-1",
			wantAmount:    nil,
		},
		{
			name:          "malformed resource is best-effort",
			payload:       `{"id":"WH-7","event_type":"PAYMENT.CAPTURE.COMPLETED","resource":"not-an-object"}`,
			wantKind:      gopay.WebhookPaymentSucceeded,
			wantPaymentID: "",
			wantAmount:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ev, err := ParseWebhook([]byte(tt.payload))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ev.Kind != tt.wantKind {
				t.Errorf("Kind = %s, want %s", ev.Kind, tt.wantKind)
			}
			if ev.PaymentID != tt.wantPaymentID {
				t.Errorf("PaymentID = %s, want %s", ev.PaymentID, tt.wantPaymentID)
			}
			if ev.OrderID != tt.wantOrderID {
				t.Errorf("OrderID = %s, want %s", ev.OrderID, tt.wantOrderID)
			}
			if ev.RefundID != tt.wantRefundID {
				t.Errorf("RefundID = %s, want %s", ev.RefundID, tt.wantRefundID)
			}
			assertWebhookAmount(t, ev.Amount, tt.wantAmount)
		})
	}
}

func assertWebhookAmount(t *testing.T, got, want *gopay.Amount) {
	t.Helper()
	switch {
	case want == nil && got != nil:
		t.Errorf("Amount = %+v, want nil", got)
	case want != nil && got == nil:
		t.Errorf("Amount = nil, want %+v", want)
	case want != nil && got != nil:
		if got.Value != want.Value || got.Currency != want.Currency {
			t.Errorf("Amount = {%d %s}, want {%d %s}", got.Value, got.Currency, want.Value, want.Currency)
		}
	}
}
