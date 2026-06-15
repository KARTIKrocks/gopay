package gopay

import (
	"context"
	"errors"
	"testing"
)

func TestPlanRequestBuilderAndValidate(t *testing.T) {
	t.Parallel()
	req := NewPlanRequest(USD(1999), BillingIntervalMonth).
		WithName("Pro").
		WithIntervalCount(3).
		WithMetadata("tier", "pro").
		WithIdempotencyKey("idem_1")

	if req.Name != "Pro" || req.IntervalCount != 3 || req.Interval != BillingIntervalMonth {
		t.Errorf("unexpected plan request: %+v", req)
	}
	if req.Metadata["tier"] != "pro" || req.IdempotencyKey != "idem_1" {
		t.Errorf("unexpected metadata/idempotency: %+v", req)
	}
	if err := req.Validate(); err != nil {
		t.Errorf("Validate() = %v, want nil", err)
	}
}

func TestPlanRequestValidate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		req     *PlanRequest
		wantErr bool
	}{
		{"nil", nil, true},
		{"valid monthly", NewPlanRequest(USD(1000), BillingIntervalMonth), false},
		{"invalid interval", NewPlanRequest(USD(1000), "fortnight"), true},
		{"invalid amount currency", NewPlanRequest(NewAmount(1000, "ZZZ"), BillingIntervalYear), true},
		{"negative interval count", NewPlanRequest(USD(1000), BillingIntervalDay).WithIntervalCount(-1), true},
		{"default interval count", NewPlanRequest(USD(1000), BillingIntervalWeek), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := tt.req.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("Validate() err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSubscriptionRequestValidate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		req     *SubscriptionRequest
		wantErr bool
	}{
		{"nil", nil, true},
		{"valid", NewSubscriptionRequest("cus_1", "plan_1"), false},
		{"missing customer", NewSubscriptionRequest("", "plan_1"), true},
		{"missing plan", NewSubscriptionRequest("cus_1", ""), true},
		{"negative trial", NewSubscriptionRequest("cus_1", "plan_1").WithTrialDays(-5), true},
		{"with trial", NewSubscriptionRequest("cus_1", "plan_1").WithTrialDays(14), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := tt.req.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("Validate() err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSubscriptionIsActive(t *testing.T) {
	t.Parallel()
	for _, s := range []SubscriptionStatus{SubscriptionStatusActive, SubscriptionStatusTrialing} {
		if !(&Subscription{Status: s}).IsActive() {
			t.Errorf("IsActive() = false for %s, want true", s)
		}
	}
	for _, s := range []SubscriptionStatus{SubscriptionStatusPastDue, SubscriptionStatusCanceled, SubscriptionStatusUnpaid} {
		if (&Subscription{Status: s}).IsActive() {
			t.Errorf("IsActive() = true for %s, want false", s)
		}
	}
}

func newSubClient(t *testing.T) *Client {
	t.Helper()
	client, err := NewClient(NewMockProvider())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return client
}

func TestClientPlanAndSubscriptionFlow(t *testing.T) {
	t.Parallel()
	client := newSubClient(t)
	ctx := context.Background()

	plan, err := client.CreatePlan(ctx, NewPlanRequest(USD(1500), BillingIntervalMonth).WithName("Pro"))
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}
	if plan.ID == "" || plan.Interval != BillingIntervalMonth {
		t.Errorf("unexpected plan: %+v", plan)
	}

	gotPlan, err := client.GetPlan(ctx, plan.ID)
	if err != nil {
		t.Fatalf("GetPlan: %v", err)
	}
	if gotPlan.ID != plan.ID {
		t.Errorf("GetPlan ID = %s, want %s", gotPlan.ID, plan.ID)
	}

	sub, err := client.CreateSubscription(ctx, NewSubscriptionRequest("cus_1", plan.ID).
		WithPaymentMethod("pm_1"))
	if err != nil {
		t.Fatalf("CreateSubscription: %v", err)
	}
	if !sub.IsActive() {
		t.Errorf("Status = %s, want active", sub.Status)
	}
	if sub.CurrentPeriodEnd.Before(sub.CurrentPeriodStart) {
		t.Error("CurrentPeriodEnd before CurrentPeriodStart")
	}

	gotSub, err := client.GetSubscription(ctx, sub.ID)
	if err != nil {
		t.Fatalf("GetSubscription: %v", err)
	}
	if gotSub.ID != sub.ID {
		t.Errorf("GetSubscription ID = %s, want %s", gotSub.ID, sub.ID)
	}
}

func TestClientCreateSubscriptionTrialing(t *testing.T) {
	t.Parallel()
	client := newSubClient(t)
	ctx := context.Background()
	plan, err := client.CreatePlan(ctx, NewPlanRequest(USD(1000), BillingIntervalYear))
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}
	sub, err := client.CreateSubscription(ctx, NewSubscriptionRequest("cus_1", plan.ID).WithTrialDays(7))
	if err != nil {
		t.Fatalf("CreateSubscription: %v", err)
	}
	if sub.Status != SubscriptionStatusTrialing {
		t.Errorf("Status = %s, want trialing", sub.Status)
	}
}

func TestClientCancelSubscription(t *testing.T) {
	t.Parallel()
	client := newSubClient(t)
	ctx := context.Background()
	plan, err := client.CreatePlan(ctx, NewPlanRequest(USD(1000), BillingIntervalMonth))
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}

	// Immediate cancel.
	s1, err := client.CreateSubscription(ctx, NewSubscriptionRequest("cus_1", plan.ID))
	if err != nil {
		t.Fatalf("CreateSubscription: %v", err)
	}
	canceled, err := client.CancelSubscription(ctx, s1.ID, nil)
	if err != nil {
		t.Fatalf("CancelSubscription: %v", err)
	}
	if canceled.Status != SubscriptionStatusCanceled {
		t.Errorf("Status = %s, want canceled", canceled.Status)
	}
	if canceled.CanceledAt.IsZero() {
		t.Error("CanceledAt is zero after immediate cancel")
	}

	// Cancel at period end.
	s2, err := client.CreateSubscription(ctx, NewSubscriptionRequest("cus_1", plan.ID))
	if err != nil {
		t.Fatalf("CreateSubscription: %v", err)
	}
	atEnd, err := client.CancelSubscription(ctx, s2.ID, &CancelOptions{AtPeriodEnd: true})
	if err != nil {
		t.Fatalf("CancelSubscription(atPeriodEnd): %v", err)
	}
	if !atEnd.CancelAtPeriodEnd {
		t.Error("CancelAtPeriodEnd = false, want true")
	}
	if atEnd.Status == SubscriptionStatusCanceled {
		t.Error("status should not be canceled yet for at-period-end")
	}
}

func TestClientCreateSubscriptionUnknownPlan(t *testing.T) {
	t.Parallel()
	client := newSubClient(t)
	_, err := client.CreateSubscription(context.Background(), NewSubscriptionRequest("cus_1", "plan_missing"))
	if !errors.Is(err, ErrSubscriptionFailed) {
		t.Errorf("err = %v, want ErrSubscriptionFailed", err)
	}
}

func TestClientSubscriptionEmptyIDs(t *testing.T) {
	t.Parallel()
	client := newSubClient(t)
	ctx := context.Background()
	if _, err := client.GetPlan(ctx, ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetPlan err = %v, want ErrNotFound", err)
	}
	if _, err := client.GetSubscription(ctx, ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetSubscription err = %v, want ErrNotFound", err)
	}
	if _, err := client.CancelSubscription(ctx, "", nil); !errors.Is(err, ErrNotFound) {
		t.Errorf("CancelSubscription err = %v, want ErrNotFound", err)
	}
}

func TestClientSubscriptionUnsupported(t *testing.T) {
	t.Parallel()
	client, err := NewClient(baseOnlyProvider{})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	ctx := context.Background()

	if _, err := client.CreatePlan(ctx, NewPlanRequest(USD(1000), BillingIntervalMonth)); !errors.Is(err, ErrUnsupported) {
		t.Errorf("CreatePlan err = %v, want ErrUnsupported", err)
	}
	if _, err := client.GetPlan(ctx, "plan_1"); !errors.Is(err, ErrUnsupported) {
		t.Errorf("GetPlan err = %v, want ErrUnsupported", err)
	}
	if _, err := client.CreateSubscription(ctx, NewSubscriptionRequest("cus_1", "plan_1")); !errors.Is(err, ErrUnsupported) {
		t.Errorf("CreateSubscription err = %v, want ErrUnsupported", err)
	}
	if _, err := client.GetSubscription(ctx, "sub_1"); !errors.Is(err, ErrUnsupported) {
		t.Errorf("GetSubscription err = %v, want ErrUnsupported", err)
	}
	if _, err := client.CancelSubscription(ctx, "sub_1", nil); !errors.Is(err, ErrUnsupported) {
		t.Errorf("CancelSubscription err = %v, want ErrUnsupported", err)
	}
}

func TestMockSubscriptionError(t *testing.T) {
	t.Parallel()
	sentinel := errors.New("boom")
	mock := NewMockProvider().WithSubscriptionError(sentinel)
	if _, err := mock.CreatePlan(context.Background(), NewPlanRequest(USD(1000), BillingIntervalMonth)); !errors.Is(err, sentinel) {
		t.Errorf("CreatePlan err = %v, want %v", err, sentinel)
	}
}

func TestMockSubscriptionNoAutoSucceed(t *testing.T) {
	t.Parallel()
	mock := NewMockProvider().WithAutoSucceed(false)
	ctx := context.Background()
	plan, err := mock.CreatePlan(ctx, NewPlanRequest(USD(1000), BillingIntervalMonth))
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}
	sub, err := mock.CreateSubscription(ctx, NewSubscriptionRequest("cus_1", plan.ID))
	if err != nil {
		t.Fatalf("CreateSubscription: %v", err)
	}
	if sub.Status != SubscriptionStatusIncomplete {
		t.Errorf("Status = %s, want incomplete", sub.Status)
	}
}
