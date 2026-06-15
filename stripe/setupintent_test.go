package stripe

import (
	"encoding/json"
	"testing"

	"github.com/KARTIKrocks/gopay"
	"github.com/stripe/stripe-go/v81"
)

func TestMapSetupIntentStatus(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input stripe.SetupIntentStatus
		want  gopay.SetupIntentStatus
	}{
		{stripe.SetupIntentStatusRequiresPaymentMethod, gopay.SetupIntentStatusRequiresPaymentMethod},
		{stripe.SetupIntentStatusRequiresConfirmation, gopay.SetupIntentStatusRequiresConfirmation},
		{stripe.SetupIntentStatusRequiresAction, gopay.SetupIntentStatusRequiresAction},
		{stripe.SetupIntentStatusProcessing, gopay.SetupIntentStatusProcessing},
		{stripe.SetupIntentStatusSucceeded, gopay.SetupIntentStatusSucceeded},
		{stripe.SetupIntentStatusCanceled, gopay.SetupIntentStatusCanceled},
		{"unknown_status", gopay.SetupIntentStatusRequiresPaymentMethod},
	}
	for _, tt := range tests {
		t.Run(string(tt.input), func(t *testing.T) {
			t.Parallel()
			if got := mapSetupIntentStatus(tt.input); got != tt.want {
				t.Errorf("mapSetupIntentStatus(%s) = %s, want %s", tt.input, got, tt.want)
			}
		})
	}
}

func TestMapSetupIntentFull(t *testing.T) {
	t.Parallel()
	p := &Provider{}
	si := &stripe.SetupIntent{
		ID:            "seti_123",
		Status:        stripe.SetupIntentStatusRequiresAction,
		Usage:         stripe.SetupIntentUsageOffSession,
		Description:   "save card",
		ClientSecret:  "seti_123_secret_abc",
		Created:       1700000000,
		Metadata:      map[string]string{"k": "v"},
		Customer:      &stripe.Customer{ID: "cus_1"},
		PaymentMethod: &stripe.PaymentMethod{ID: "pm_1"},
		LastSetupError: &stripe.Error{
			Code: stripe.ErrorCodeCardDeclined,
			Msg:  "declined",
		},
		NextAction: &stripe.SetupIntentNextAction{
			RedirectToURL: &stripe.SetupIntentNextActionRedirectToURL{URL: "https://3ds.example.com"},
		},
	}

	out := p.mapSetupIntent(si)

	if out.ID != "seti_123" {
		t.Errorf("ID = %s, want seti_123", out.ID)
	}
	if out.Status != gopay.SetupIntentStatusRequiresAction {
		t.Errorf("Status = %s, want requires_action", out.Status)
	}
	if out.Usage != gopay.SetupIntentUsageOffSession {
		t.Errorf("Usage = %s, want off_session", out.Usage)
	}
	if out.Description != "save card" {
		t.Errorf("Description = %s, want save card", out.Description)
	}
	if out.ClientSecret != "seti_123_secret_abc" {
		t.Errorf("ClientSecret = %s", out.ClientSecret)
	}
	if out.CustomerID != "cus_1" {
		t.Errorf("CustomerID = %s, want cus_1", out.CustomerID)
	}
	if out.PaymentMethodID != "pm_1" {
		t.Errorf("PaymentMethodID = %s, want pm_1", out.PaymentMethodID)
	}
	if out.FailureCode != string(stripe.ErrorCodeCardDeclined) {
		t.Errorf("FailureCode = %s", out.FailureCode)
	}
	if out.FailureMessage != "declined" {
		t.Errorf("FailureMessage = %s, want declined", out.FailureMessage)
	}
	if out.RedirectURL != "https://3ds.example.com" {
		t.Errorf("RedirectURL = %s", out.RedirectURL)
	}
	if out.Provider != "stripe" {
		t.Errorf("Provider = %s, want stripe", out.Provider)
	}
	if out.Metadata["k"] != "v" {
		t.Errorf("Metadata[k] = %s, want v", out.Metadata["k"])
	}
}

func TestMapSetupIntentMinimal(t *testing.T) {
	t.Parallel()
	p := &Provider{}
	out := p.mapSetupIntent(&stripe.SetupIntent{
		ID:     "seti_min",
		Status: stripe.SetupIntentStatusSucceeded,
	})
	if out.CustomerID != "" || out.PaymentMethodID != "" || out.RedirectURL != "" || out.FailureCode != "" {
		t.Errorf("expected empty optional fields, got %+v", out)
	}
	if !out.IsSucceeded() {
		t.Error("IsSucceeded() = false, want true")
	}
}

func TestBuildWebhookEventSetupIntent(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		eventType string
		raw       string
		wantKind  gopay.WebhookEventKind
		wantSetup string
	}{
		{
			name:      "setup succeeded",
			eventType: "setup_intent.succeeded",
			raw:       `{"object":"setup_intent","id":"seti_1"}`,
			wantKind:  gopay.WebhookSetupSucceeded,
			wantSetup: "seti_1",
		},
		{
			name:      "setup failed",
			eventType: "setup_intent.setup_failed",
			raw:       `{"object":"setup_intent","id":"seti_2"}`,
			wantKind:  gopay.WebhookSetupFailed,
			wantSetup: "seti_2",
		},
		{
			name:      "setup canceled",
			eventType: "setup_intent.canceled",
			raw:       `{"object":"setup_intent","id":"seti_3"}`,
			wantKind:  gopay.WebhookSetupFailed,
			wantSetup: "seti_3",
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
			if ev.SetupIntentID != tt.wantSetup {
				t.Errorf("SetupIntentID = %s, want %s", ev.SetupIntentID, tt.wantSetup)
			}
			if ev.PaymentID != "" {
				t.Errorf("PaymentID = %s, want empty", ev.PaymentID)
			}
			if ev.Amount != nil {
				t.Errorf("Amount = %+v, want nil", ev.Amount)
			}
		})
	}
}

// Ensure the Stripe provider satisfies the optional SetupIntentProvider
// interface at compile time.
var _ gopay.SetupIntentProvider = (*Provider)(nil)
