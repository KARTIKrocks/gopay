package gopay

import (
	"context"
	"errors"
	"testing"
)

func TestSetupIntentRequestBuilder(t *testing.T) {
	t.Parallel()
	req := NewSetupIntentRequest().
		WithCustomer("cus_1").
		WithPaymentMethod("pm_1").
		WithUsage(SetupIntentUsageOnSession).
		WithDescription("save card").
		WithReturnURL("https://example.com/return").
		WithMetadata("k", "v").
		WithIdempotencyKey("idem_1")

	if req.CustomerID != "cus_1" {
		t.Errorf("CustomerID = %s, want cus_1", req.CustomerID)
	}
	if req.PaymentMethodID != "pm_1" {
		t.Errorf("PaymentMethodID = %s, want pm_1", req.PaymentMethodID)
	}
	if req.Usage != SetupIntentUsageOnSession {
		t.Errorf("Usage = %s, want on_session", req.Usage)
	}
	if req.Description != "save card" {
		t.Errorf("Description = %s, want save card", req.Description)
	}
	if req.ReturnURL != "https://example.com/return" {
		t.Errorf("ReturnURL = %s", req.ReturnURL)
	}
	if req.Metadata["k"] != "v" {
		t.Errorf("Metadata[k] = %s, want v", req.Metadata["k"])
	}
	if req.IdempotencyKey != "idem_1" {
		t.Errorf("IdempotencyKey = %s, want idem_1", req.IdempotencyKey)
	}
}

func TestSetupIntentRequestDefaultUsage(t *testing.T) {
	t.Parallel()
	req := NewSetupIntentRequest()
	if req.Usage != SetupIntentUsageOffSession {
		t.Errorf("default Usage = %s, want off_session", req.Usage)
	}
}

func TestSetupIntentRequestValidate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		req     *SetupIntentRequest
		wantErr bool
	}{
		{"nil request", nil, true},
		{"default valid", NewSetupIntentRequest(), false},
		{"empty usage valid", &SetupIntentRequest{}, false},
		{"on_session valid", NewSetupIntentRequest().WithUsage(SetupIntentUsageOnSession), false},
		{"invalid usage", NewSetupIntentRequest().WithUsage("whenever"), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.req.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSetupIntentStatusHelpers(t *testing.T) {
	t.Parallel()
	if !(&SetupIntent{Status: SetupIntentStatusSucceeded}).IsSucceeded() {
		t.Error("IsSucceeded() = false, want true")
	}
	if (&SetupIntent{Status: SetupIntentStatusProcessing}).IsSucceeded() {
		t.Error("IsSucceeded() = true, want false")
	}
	if !(&SetupIntent{Status: SetupIntentStatusRequiresAction}).RequiresAction() {
		t.Error("RequiresAction() = false, want true")
	}
	if (&SetupIntent{Status: SetupIntentStatusSucceeded}).RequiresAction() {
		t.Error("RequiresAction() = true, want false")
	}
}

func TestClientCreateSetupIntent(t *testing.T) {
	t.Parallel()
	client, err := NewClient(NewMockProvider())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	ctx := context.Background()

	// Without a payment method, the intent awaits one.
	si, err := client.CreateSetupIntent(ctx, NewSetupIntentRequest().WithCustomer("cus_1"))
	if err != nil {
		t.Fatalf("CreateSetupIntent: %v", err)
	}
	if si.Status != SetupIntentStatusRequiresPaymentMethod {
		t.Errorf("Status = %s, want requires_payment_method", si.Status)
	}
	if si.Usage != SetupIntentUsageOffSession {
		t.Errorf("Usage = %s, want off_session", si.Usage)
	}
	if si.ClientSecret == "" {
		t.Error("ClientSecret is empty")
	}

	// With a payment method and auto-succeed, the intent succeeds.
	si2, err := client.CreateSetupIntent(ctx, NewSetupIntentRequest().WithPaymentMethod("pm_1"))
	if err != nil {
		t.Fatalf("CreateSetupIntent: %v", err)
	}
	if !si2.IsSucceeded() {
		t.Errorf("Status = %s, want succeeded", si2.Status)
	}
}

func TestClientGetAndCancelSetupIntent(t *testing.T) {
	t.Parallel()
	client, err := NewClient(NewMockProvider())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	ctx := context.Background()

	created, err := client.CreateSetupIntent(ctx, NewSetupIntentRequest().WithCustomer("cus_1"))
	if err != nil {
		t.Fatalf("CreateSetupIntent: %v", err)
	}

	got, err := client.GetSetupIntent(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetSetupIntent: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("ID = %s, want %s", got.ID, created.ID)
	}

	canceled, err := client.CancelSetupIntent(ctx, created.ID)
	if err != nil {
		t.Fatalf("CancelSetupIntent: %v", err)
	}
	if canceled.Status != SetupIntentStatusCanceled {
		t.Errorf("Status = %s, want canceled", canceled.Status)
	}
}

func TestClientGetSetupIntentEmptyID(t *testing.T) {
	t.Parallel()
	client, _ := NewClient(NewMockProvider())
	if _, err := client.GetSetupIntent(context.Background(), ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
	if _, err := client.CancelSetupIntent(context.Background(), ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestClientCreateSetupIntentInvalid(t *testing.T) {
	t.Parallel()
	client, _ := NewClient(NewMockProvider())
	_, err := client.CreateSetupIntent(context.Background(), NewSetupIntentRequest().WithUsage("nonsense"))
	if err == nil {
		t.Error("expected validation error, got nil")
	}
}

func TestClientSetupIntentUnsupported(t *testing.T) {
	t.Parallel()
	client, err := NewClient(baseOnlyProvider{})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	ctx := context.Background()

	if _, err := client.CreateSetupIntent(ctx, NewSetupIntentRequest()); !errors.Is(err, ErrUnsupported) {
		t.Errorf("CreateSetupIntent err = %v, want ErrUnsupported", err)
	}
	if _, err := client.GetSetupIntent(ctx, "seti_1"); !errors.Is(err, ErrUnsupported) {
		t.Errorf("GetSetupIntent err = %v, want ErrUnsupported", err)
	}
	if _, err := client.CancelSetupIntent(ctx, "seti_1"); !errors.Is(err, ErrUnsupported) {
		t.Errorf("CancelSetupIntent err = %v, want ErrUnsupported", err)
	}
}

func TestMockSetupIntentError(t *testing.T) {
	t.Parallel()
	sentinel := errors.New("boom")
	mock := NewMockProvider().WithSetupError(sentinel)
	if _, err := mock.CreateSetupIntent(context.Background(), NewSetupIntentRequest()); !errors.Is(err, sentinel) {
		t.Errorf("err = %v, want %v", err, sentinel)
	}
}

func TestMockCancelSucceededSetupIntentFails(t *testing.T) {
	t.Parallel()
	mock := NewMockProvider()
	ctx := context.Background()
	si, err := mock.CreateSetupIntent(ctx, NewSetupIntentRequest().WithPaymentMethod("pm_1"))
	if err != nil {
		t.Fatalf("CreateSetupIntent: %v", err)
	}
	if _, err := mock.CancelSetupIntent(ctx, si.ID); !errors.Is(err, ErrSetupFailed) {
		t.Errorf("err = %v, want ErrSetupFailed", err)
	}
}

func TestMockGetSetupIntentNotFound(t *testing.T) {
	t.Parallel()
	mock := NewMockProvider()
	if _, err := mock.GetSetupIntent(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestMockSetupIntentNoAutoSucceed(t *testing.T) {
	t.Parallel()
	mock := NewMockProvider().WithAutoSucceed(false)
	si, err := mock.CreateSetupIntent(context.Background(), NewSetupIntentRequest().WithPaymentMethod("pm_1"))
	if err != nil {
		t.Fatalf("CreateSetupIntent: %v", err)
	}
	if si.Status != SetupIntentStatusRequiresConfirmation {
		t.Errorf("Status = %s, want requires_confirmation", si.Status)
	}
}
