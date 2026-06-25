package razorpay

import (
	"testing"

	"github.com/KARTIKrocks/gopay"
)

func TestParseWebhookNormalized(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name               string
		payload            string
		wantKind           gopay.WebhookEventKind
		wantPaymentID      string
		wantOrderID        string
		wantRefundID       string
		wantSubscriptionID string
		wantAmount         *gopay.Amount
	}{
		{
			name:          "payment captured",
			payload:       `{"event":"payment.captured","account_id":"acc_1","payload":{"payment":{"entity":{"id":"pay_1","order_id":"order_1","amount":15000,"currency":"INR"}}}}`,
			wantKind:      gopay.WebhookPaymentSucceeded,
			wantPaymentID: "pay_1",
			wantOrderID:   "order_1",
			wantAmount:    gopay.NewAmount(15000, "INR"),
		},
		{
			name:          "refund processed",
			payload:       `{"event":"refund.processed","account_id":"acc_2","payload":{"refund":{"entity":{"id":"rfnd_1","payment_id":"pay_2","amount":5000,"currency":"INR"}}}}`,
			wantKind:      gopay.WebhookRefundSucceeded,
			wantPaymentID: "pay_2",
			wantRefundID:  "rfnd_1",
			wantAmount:    gopay.NewAmount(5000, "INR"),
		},
		{
			name:          "order paid prefers payment entity",
			payload:       `{"event":"order.paid","account_id":"acc_3","payload":{"order":{"entity":{"id":"order_9","amount":20000,"currency":"INR"}},"payment":{"entity":{"id":"pay_3","order_id":"order_9","amount":20000,"currency":"INR"}}}}`,
			wantKind:      gopay.WebhookPaymentSucceeded,
			wantPaymentID: "pay_3",
			wantOrderID:   "order_9",
			wantAmount:    gopay.NewAmount(20000, "INR"),
		},
		{
			name:          "refund created with processed status",
			payload:       `{"event":"refund.created","account_id":"acc_5","payload":{"refund":{"entity":{"id":"rfnd_2","payment_id":"pay_5","status":"processed","amount":1000,"currency":"INR"}}}}`,
			wantKind:      gopay.WebhookRefundSucceeded,
			wantPaymentID: "pay_5",
			wantRefundID:  "rfnd_2",
			wantAmount:    gopay.NewAmount(1000, "INR"),
		},
		{
			name:          "refund created still pending stays unknown",
			payload:       `{"event":"refund.created","account_id":"acc_6","payload":{"refund":{"entity":{"id":"rfnd_3","payment_id":"pay_6","status":"pending","amount":1000,"currency":"INR"}}}}`,
			wantKind:      gopay.WebhookUnknown,
			wantPaymentID: "pay_6",
			wantRefundID:  "rfnd_3",
			wantAmount:    gopay.NewAmount(1000, "INR"),
		},
		{
			name:               "subscription charged carries subscription and payment",
			payload:            `{"event":"subscription.charged","account_id":"acc_8","payload":{"subscription":{"entity":{"id":"sub_1"}},"payment":{"entity":{"id":"pay_8","amount":69900,"currency":"INR"}}}}`,
			wantKind:           gopay.WebhookInvoicePaymentSucceeded,
			wantPaymentID:      "pay_8",
			wantSubscriptionID: "sub_1",
			wantAmount:         gopay.NewAmount(69900, "INR"),
		},
		{
			name:               "subscription cancelled",
			payload:            `{"event":"subscription.cancelled","account_id":"acc_9","payload":{"subscription":{"entity":{"id":"sub_2"}}}}`,
			wantKind:           gopay.WebhookSubscriptionCanceled,
			wantSubscriptionID: "sub_2",
		},
		{
			name:               "subscription activated",
			payload:            `{"event":"subscription.activated","account_id":"acc_10","payload":{"subscription":{"entity":{"id":"sub_3"}}}}`,
			wantKind:           gopay.WebhookSubscriptionCreated,
			wantSubscriptionID: "sub_3",
		},
		{
			name:               "subscription halted is payment failed",
			payload:            `{"event":"subscription.halted","account_id":"acc_11","payload":{"subscription":{"entity":{"id":"sub_4"}}}}`,
			wantKind:           gopay.WebhookInvoicePaymentFailed,
			wantSubscriptionID: "sub_4",
		},
		{
			name:       "unmapped event",
			payload:    `{"event":"payment.dispute.created","account_id":"acc_4","payload":{}}`,
			wantKind:   gopay.WebhookUnknown,
			wantAmount: nil,
		},
		{
			name:       "malformed payload is best-effort",
			payload:    `{"event":"payment.captured","account_id":"acc_7","payload":"{invalid}"}`,
			wantKind:   gopay.WebhookPaymentSucceeded,
			wantAmount: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
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
			if ev.SubscriptionID != tt.wantSubscriptionID {
				t.Errorf("SubscriptionID = %s, want %s", ev.SubscriptionID, tt.wantSubscriptionID)
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
