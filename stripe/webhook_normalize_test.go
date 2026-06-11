package stripe

import (
	"encoding/json"
	"testing"

	"github.com/KARTIKrocks/gopay"
	"github.com/stripe/stripe-go/v81"
)

func TestBuildWebhookEventNormalized(t *testing.T) {
	tests := []struct {
		name          string
		eventType     string
		raw           string
		wantKind      gopay.WebhookEventKind
		wantPaymentID string
		wantRefundID  string
		wantAmount    *gopay.Amount
	}{
		{
			name:          "payment succeeded",
			eventType:     "payment_intent.succeeded",
			raw:           `{"object":"payment_intent","id":"pi_123","amount":2000,"currency":"usd"}`,
			wantKind:      gopay.WebhookPaymentSucceeded,
			wantPaymentID: "pi_123",
			wantAmount:    gopay.NewAmount(2000, "USD"),
		},
		{
			name:          "payment failed",
			eventType:     "payment_intent.payment_failed",
			raw:           `{"object":"payment_intent","id":"pi_456","amount":500,"currency":"eur"}`,
			wantKind:      gopay.WebhookPaymentFailed,
			wantPaymentID: "pi_456",
			wantAmount:    gopay.NewAmount(500, "EUR"),
		},
		{
			name:          "charge refunded",
			eventType:     "charge.refunded",
			raw:           `{"object":"charge","id":"ch_1","payment_intent":"pi_9","amount":5000,"amount_refunded":5000,"currency":"usd","refunds":{"data":[{"id":"re_1"}]}}`,
			wantKind:      gopay.WebhookRefundSucceeded,
			wantPaymentID: "pi_9",
			wantRefundID:  "re_1",
			wantAmount:    gopay.NewAmount(5000, "USD"),
		},
		{
			name:          "refund object failed",
			eventType:     "refund.failed",
			raw:           `{"object":"refund","id":"re_2","payment_intent":"pi_5","amount":300,"currency":"usd"}`,
			wantKind:      gopay.WebhookRefundFailed,
			wantPaymentID: "pi_5",
			wantRefundID:  "re_2",
			wantAmount:    gopay.NewAmount(300, "USD"),
		},
		{
			name:          "refund created with succeeded status",
			eventType:     "refund.created",
			raw:           `{"object":"refund","id":"re_3","payment_intent":"pi_7","status":"succeeded","amount":250,"currency":"usd"}`,
			wantKind:      gopay.WebhookRefundSucceeded,
			wantPaymentID: "pi_7",
			wantRefundID:  "re_3",
			wantAmount:    gopay.NewAmount(250, "USD"),
		},
		{
			name:          "refund updated with failed status",
			eventType:     "refund.updated",
			raw:           `{"object":"refund","id":"re_4","payment_intent":"pi_8","status":"failed","amount":250,"currency":"usd"}`,
			wantKind:      gopay.WebhookRefundFailed,
			wantPaymentID: "pi_8",
			wantRefundID:  "re_4",
			wantAmount:    gopay.NewAmount(250, "USD"),
		},
		{
			name:          "refund created still pending stays unknown",
			eventType:     "refund.created",
			raw:           `{"object":"refund","id":"re_5","payment_intent":"pi_9","status":"pending","amount":250,"currency":"usd"}`,
			wantKind:      gopay.WebhookUnknown,
			wantPaymentID: "pi_9",
			wantRefundID:  "re_5",
			wantAmount:    gopay.NewAmount(250, "USD"),
		},
		{
			name:          "unmapped event",
			eventType:     "customer.created",
			raw:           `{"object":"customer","id":"cus_1"}`,
			wantKind:      gopay.WebhookUnknown,
			wantPaymentID: "cus_1",
			wantAmount:    nil,
		},
		{
			name:          "malformed data json is best-effort",
			eventType:     "payment_intent.succeeded",
			raw:           `{"object":"payment_intent",`,
			wantKind:      gopay.WebhookPaymentSucceeded,
			wantPaymentID: "",
			wantAmount:    nil,
		},
		{
			name:          "missing currency leaves amount nil",
			eventType:     "payment_intent.succeeded",
			raw:           `{"object":"payment_intent","id":"pi_x","amount":100}`,
			wantKind:      gopay.WebhookPaymentSucceeded,
			wantPaymentID: "pi_x",
			wantAmount:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := stripe.Event{
				ID:   "evt_1",
				Type: stripe.EventType(tt.eventType),
				Data: &stripe.EventData{Raw: json.RawMessage(tt.raw)},
			}
			ev := buildWebhookEvent(event)

			if ev.Provider != "stripe" {
				t.Errorf("Provider = %s, want stripe", ev.Provider)
			}
			if ev.Type != tt.eventType {
				t.Errorf("Type = %s, want %s", ev.Type, tt.eventType)
			}
			if ev.Kind != tt.wantKind {
				t.Errorf("Kind = %s, want %s", ev.Kind, tt.wantKind)
			}
			if ev.PaymentID != tt.wantPaymentID {
				t.Errorf("PaymentID = %s, want %s", ev.PaymentID, tt.wantPaymentID)
			}
			if ev.RefundID != tt.wantRefundID {
				t.Errorf("RefundID = %s, want %s", ev.RefundID, tt.wantRefundID)
			}
			assertAmount(t, ev.Amount, tt.wantAmount)
		})
	}
}

func assertAmount(t *testing.T, got, want *gopay.Amount) {
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
