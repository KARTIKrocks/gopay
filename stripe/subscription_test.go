package stripe

import (
	"encoding/json"
	"testing"

	"github.com/KARTIKrocks/gopay"
	"github.com/stripe/stripe-go/v81"
)

func TestMapBillingInterval(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input stripe.PriceRecurringInterval
		want  gopay.BillingInterval
	}{
		{stripe.PriceRecurringIntervalDay, gopay.BillingIntervalDay},
		{stripe.PriceRecurringIntervalWeek, gopay.BillingIntervalWeek},
		{stripe.PriceRecurringIntervalMonth, gopay.BillingIntervalMonth},
		{stripe.PriceRecurringIntervalYear, gopay.BillingIntervalYear},
	}
	for _, tt := range tests {
		t.Run(string(tt.input), func(t *testing.T) {
			t.Parallel()
			if got := mapBillingInterval(tt.input); got != tt.want {
				t.Errorf("mapBillingInterval(%s) = %s, want %s", tt.input, got, tt.want)
			}
		})
	}
}

func TestMapSubscriptionStatus(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input stripe.SubscriptionStatus
		want  gopay.SubscriptionStatus
	}{
		{stripe.SubscriptionStatusActive, gopay.SubscriptionStatusActive},
		{stripe.SubscriptionStatusTrialing, gopay.SubscriptionStatusTrialing},
		{stripe.SubscriptionStatusPastDue, gopay.SubscriptionStatusPastDue},
		{stripe.SubscriptionStatusCanceled, gopay.SubscriptionStatusCanceled},
		{stripe.SubscriptionStatusIncomplete, gopay.SubscriptionStatusIncomplete},
		{stripe.SubscriptionStatusIncompleteExpired, gopay.SubscriptionStatusIncompleteExpired},
		{stripe.SubscriptionStatusUnpaid, gopay.SubscriptionStatusUnpaid},
	}
	for _, tt := range tests {
		t.Run(string(tt.input), func(t *testing.T) {
			t.Parallel()
			if got := mapSubscriptionStatus(tt.input); got != tt.want {
				t.Errorf("mapSubscriptionStatus(%s) = %s, want %s", tt.input, got, tt.want)
			}
		})
	}
}

func TestMapPlan(t *testing.T) {
	t.Parallel()
	p := &Provider{}
	plan := p.mapPlan(&stripe.Price{
		ID:         "price_123",
		Nickname:   "Pro",
		UnitAmount: 1999,
		Currency:   stripe.CurrencyUSD,
		Created:    1700000000,
		Metadata:   map[string]string{"tier": "pro"},
		Recurring: &stripe.PriceRecurring{
			Interval:      stripe.PriceRecurringIntervalMonth,
			IntervalCount: 3,
		},
	})

	if plan.ID != "price_123" {
		t.Errorf("ID = %s, want price_123", plan.ID)
	}
	if plan.Name != "Pro" {
		t.Errorf("Name = %s, want Pro", plan.Name)
	}
	if plan.Amount.Value != 1999 || plan.Amount.Currency != "USD" {
		t.Errorf("Amount = %+v, want {1999 USD}", plan.Amount)
	}
	if plan.Interval != gopay.BillingIntervalMonth || plan.IntervalCount != 3 {
		t.Errorf("interval = %s x%d, want month x3", plan.Interval, plan.IntervalCount)
	}
	if plan.Provider != "stripe" {
		t.Errorf("Provider = %s, want stripe", plan.Provider)
	}
}

func TestMapPlanFallbackName(t *testing.T) {
	t.Parallel()
	p := &Provider{}
	plan := p.mapPlan(&stripe.Price{
		ID:       "price_x",
		Currency: stripe.CurrencyEUR,
		Product:  &stripe.Product{Name: "From Product"},
	})
	if plan.Name != "From Product" {
		t.Errorf("Name = %s, want From Product (product fallback)", plan.Name)
	}
}

func TestMapSubscription(t *testing.T) {
	t.Parallel()
	p := &Provider{}
	sub := p.mapSubscription(&stripe.Subscription{
		ID:                   "sub_123",
		Status:               stripe.SubscriptionStatusActive,
		CancelAtPeriodEnd:    true,
		CurrentPeriodStart:   1700000000,
		CurrentPeriodEnd:     1702592000,
		CanceledAt:           0,
		Created:              1699990000,
		Metadata:             map[string]string{"k": "v"},
		Customer:             &stripe.Customer{ID: "cus_1"},
		DefaultPaymentMethod: &stripe.PaymentMethod{ID: "pm_1"},
		Items: &stripe.SubscriptionItemList{
			Data: []*stripe.SubscriptionItem{
				{Price: &stripe.Price{ID: "price_1"}},
			},
		},
	})

	if sub.ID != "sub_123" {
		t.Errorf("ID = %s, want sub_123", sub.ID)
	}
	if sub.Status != gopay.SubscriptionStatusActive {
		t.Errorf("Status = %s, want active", sub.Status)
	}
	if !sub.CancelAtPeriodEnd {
		t.Error("CancelAtPeriodEnd = false, want true")
	}
	if sub.CustomerID != "cus_1" {
		t.Errorf("CustomerID = %s, want cus_1", sub.CustomerID)
	}
	if sub.PaymentMethodID != "pm_1" {
		t.Errorf("PaymentMethodID = %s, want pm_1", sub.PaymentMethodID)
	}
	if sub.PlanID != "price_1" {
		t.Errorf("PlanID = %s, want price_1", sub.PlanID)
	}
	if sub.CurrentPeriodStart.Unix() != 1700000000 || sub.CurrentPeriodEnd.Unix() != 1702592000 {
		t.Errorf("period = %v..%v", sub.CurrentPeriodStart, sub.CurrentPeriodEnd)
	}
	if !sub.CanceledAt.IsZero() {
		t.Errorf("CanceledAt = %v, want zero", sub.CanceledAt)
	}
}

func TestMapSubscriptionMinimal(t *testing.T) {
	t.Parallel()
	p := &Provider{}
	sub := p.mapSubscription(&stripe.Subscription{
		ID:     "sub_min",
		Status: stripe.SubscriptionStatusIncomplete,
	})
	if sub.CustomerID != "" || sub.PaymentMethodID != "" || sub.PlanID != "" {
		t.Errorf("expected empty optional fields, got %+v", sub)
	}
	if sub.IsActive() {
		t.Error("IsActive() = true, want false for incomplete")
	}
}

func TestBuildWebhookEventSubscription(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		eventType string
		raw       string
		wantKind  gopay.WebhookEventKind
		wantSub   string
		wantInv   string
		wantAmt   *gopay.Amount
	}{
		{
			name:      "subscription created",
			eventType: "customer.subscription.created",
			raw:       `{"object":"subscription","id":"sub_1"}`,
			wantKind:  gopay.WebhookSubscriptionCreated,
			wantSub:   "sub_1",
		},
		{
			name:      "subscription updated",
			eventType: "customer.subscription.updated",
			raw:       `{"object":"subscription","id":"sub_2"}`,
			wantKind:  gopay.WebhookSubscriptionUpdated,
			wantSub:   "sub_2",
		},
		{
			name:      "subscription deleted",
			eventType: "customer.subscription.deleted",
			raw:       `{"object":"subscription","id":"sub_3"}`,
			wantKind:  gopay.WebhookSubscriptionCanceled,
			wantSub:   "sub_3",
		},
		{
			name:      "invoice payment succeeded",
			eventType: "invoice.payment_succeeded",
			raw:       `{"object":"invoice","id":"in_1","subscription":"sub_9","amount_paid":2500,"currency":"usd"}`,
			wantKind:  gopay.WebhookInvoicePaymentSucceeded,
			wantSub:   "sub_9",
			wantInv:   "in_1",
			wantAmt:   gopay.NewAmount(2500, "USD"),
		},
		{
			name:      "invoice payment failed",
			eventType: "invoice.payment_failed",
			raw:       `{"object":"invoice","id":"in_2","subscription":"sub_10","amount_paid":0,"currency":"usd"}`,
			wantKind:  gopay.WebhookInvoicePaymentFailed,
			wantSub:   "sub_10",
			wantInv:   "in_2",
			wantAmt:   gopay.NewAmount(0, "USD"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			event := stripe.Event{
				ID:   "evt_1",
				Type: stripe.EventType(tt.eventType),
				Data: &stripe.EventData{Raw: json.RawMessage(tt.raw)},
			}
			ev := buildWebhookEvent(event)

			if ev.Kind != tt.wantKind {
				t.Errorf("Kind = %s, want %s", ev.Kind, tt.wantKind)
			}
			if ev.SubscriptionID != tt.wantSub {
				t.Errorf("SubscriptionID = %s, want %s", ev.SubscriptionID, tt.wantSub)
			}
			if ev.InvoiceID != tt.wantInv {
				t.Errorf("InvoiceID = %s, want %s", ev.InvoiceID, tt.wantInv)
			}
			assertAmount(t, ev.Amount, tt.wantAmt)
		})
	}
}

// Compile-time check that the Stripe provider satisfies SubscriptionProvider.
var _ gopay.SubscriptionProvider = (*Provider)(nil)
