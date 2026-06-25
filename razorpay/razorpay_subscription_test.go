package razorpay

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/KARTIKrocks/gopay"
)

func TestCreatePlanHTTP(t *testing.T) {
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/plans" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed to read request body: %v", err)
		}
		if !strings.Contains(string(body), `"period":"monthly"`) {
			t.Errorf("body missing period: %s", body)
		}
		if !strings.Contains(string(body), `"interval":3`) {
			t.Errorf("body missing interval: %s", body)
		}
		if !strings.Contains(string(body), `"amount":69900`) {
			t.Errorf("body missing amount: %s", body)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"plan_001","period":"monthly","interval":3,"item":{"name":"Pro","amount":69900,"currency":"INR"},"notes":{"k":"v"},"created_at":1700000000}`))
	})

	req := gopay.NewPlanRequest(gopay.INR(69900), gopay.BillingIntervalMonth).
		WithIntervalCount(3).
		WithName("Pro")

	plan, err := p.CreatePlan(context.Background(), req)
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}
	if plan.ID != "plan_001" {
		t.Errorf("ID = %s, want plan_001", plan.ID)
	}
	if plan.Interval != gopay.BillingIntervalMonth {
		t.Errorf("Interval = %s, want month", plan.Interval)
	}
	if plan.IntervalCount != 3 {
		t.Errorf("IntervalCount = %d, want 3", plan.IntervalCount)
	}
	if plan.Amount.Value != 69900 {
		t.Errorf("Amount = %d, want 69900", plan.Amount.Value)
	}
	if plan.Name != "Pro" {
		t.Errorf("Name = %s, want Pro", plan.Name)
	}
}

func TestCreatePlanUnsupportedInterval(t *testing.T) {
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("server should not be called for an invalid interval")
	})

	req := gopay.NewPlanRequest(gopay.INR(100), gopay.BillingInterval("fortnightly"))
	if _, err := p.CreatePlan(context.Background(), req); err == nil {
		t.Fatal("expected error for unsupported interval, got nil")
	}
}

func TestGetPlanHTTP(t *testing.T) {
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/plans/plan_001" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"plan_001","period":"yearly","interval":1,"item":{"name":"Annual","amount":120000,"currency":"INR"},"created_at":1700000000}`))
	})

	plan, err := p.GetPlan(context.Background(), "plan_001")
	if err != nil {
		t.Fatalf("GetPlan: %v", err)
	}
	if plan.Interval != gopay.BillingIntervalYear {
		t.Errorf("Interval = %s, want year", plan.Interval)
	}
}

func TestCreateSubscriptionHTTP(t *testing.T) {
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/subscriptions" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed to read request body: %v", err)
		}
		if !strings.Contains(string(body), `"plan_id":"plan_001"`) {
			t.Errorf("body missing plan_id: %s", body)
		}
		if !strings.Contains(string(body), `"total_count":12`) {
			t.Errorf("body missing total_count: %s", body)
		}
		// Razorpay binds the customer at mandate authorization, so the customer ID
		// must not be forwarded on create (it would be rejected as an unknown field).
		if strings.Contains(string(body), "customer_id") {
			t.Errorf("body should not forward customer_id: %s", body)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"sub_001","plan_id":"plan_001","customer_id":"cust_001","status":"created","short_url":"https://rzp.io/i/abc","created_at":1700000000}`))
	})

	req := gopay.NewSubscriptionRequest("cust_001", "plan_001").WithTotalCount(12)

	sub, err := p.CreateSubscription(context.Background(), req)
	if err != nil {
		t.Fatalf("CreateSubscription: %v", err)
	}
	if sub.ID != "sub_001" {
		t.Errorf("ID = %s, want sub_001", sub.ID)
	}
	// A freshly created Razorpay subscription is awaiting mandate authorization.
	if sub.Status != gopay.SubscriptionStatusIncomplete {
		t.Errorf("Status = %s, want incomplete", sub.Status)
	}
	if sub.AuthURL != "https://rzp.io/i/abc" {
		t.Errorf("AuthURL = %s, want the short_url", sub.AuthURL)
	}
	if sub.IsActive() {
		t.Error("IsActive() = true, want false for a created subscription")
	}
}

func TestCreateSubscriptionRequiresTotalCount(t *testing.T) {
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("server should not be called when TotalCount is missing")
	})

	req := gopay.NewSubscriptionRequest("cust_001", "plan_001") // no TotalCount
	if _, err := p.CreateSubscription(context.Background(), req); err == nil {
		t.Fatal("expected error for missing TotalCount, got nil")
	}
}

func TestGetSubscriptionHTTP(t *testing.T) {
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/subscriptions/sub_001" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"sub_001","plan_id":"plan_001","customer_id":"cust_001","status":"active","current_start":1700000000,"current_end":1702592000,"created_at":1699000000}`))
	})

	sub, err := p.GetSubscription(context.Background(), "sub_001")
	if err != nil {
		t.Fatalf("GetSubscription: %v", err)
	}
	if sub.Status != gopay.SubscriptionStatusActive {
		t.Errorf("Status = %s, want active", sub.Status)
	}
	if !sub.IsActive() {
		t.Error("IsActive() = false, want true for an active subscription")
	}
	if sub.CurrentPeriodStart.IsZero() || sub.CurrentPeriodEnd.IsZero() {
		t.Error("expected current period start/end to be set")
	}
}

func TestCancelSubscriptionImmediateHTTP(t *testing.T) {
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/subscriptions/sub_001/cancel" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"cancel_at_cycle_end":0`) {
			t.Errorf("body should request immediate cancel: %s", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"sub_001","plan_id":"plan_001","status":"cancelled","ended_at":1700500000,"created_at":1699000000}`))
	})

	sub, err := p.CancelSubscription(context.Background(), "sub_001", nil)
	if err != nil {
		t.Fatalf("CancelSubscription: %v", err)
	}
	if sub.Status != gopay.SubscriptionStatusCanceled {
		t.Errorf("Status = %s, want canceled", sub.Status)
	}
	if sub.CanceledAt.IsZero() {
		t.Error("expected CanceledAt to be set")
	}
}

func TestCancelSubscriptionAtPeriodEndHTTP(t *testing.T) {
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"cancel_at_cycle_end":1`) {
			t.Errorf("body should request cancel at cycle end: %s", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"sub_001","plan_id":"plan_001","status":"active","created_at":1699000000}`))
	})

	_, err := p.CancelSubscription(context.Background(), "sub_001", &gopay.CancelOptions{AtPeriodEnd: true})
	if err != nil {
		t.Fatalf("CancelSubscription: %v", err)
	}
}

func TestCreateSubscriptionErrorMapped(t *testing.T) {
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"code":"BAD_REQUEST_ERROR","description":"plan does not exist","field":"plan_id"}}`))
	})

	req := gopay.NewSubscriptionRequest("cust_001", "plan_bad").WithTotalCount(6)
	_, err := p.CreateSubscription(context.Background(), req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, gopay.ErrPaymentFailed) {
		t.Errorf("error = %v, want wrapped ErrPaymentFailed", err)
	}
}

func TestMapSubscriptionStatus(t *testing.T) {
	cases := map[string]gopay.SubscriptionStatus{
		"created":       gopay.SubscriptionStatusIncomplete,
		"authenticated": gopay.SubscriptionStatusIncomplete,
		"active":        gopay.SubscriptionStatusActive,
		"pending":       gopay.SubscriptionStatusPastDue,
		"halted":        gopay.SubscriptionStatusUnpaid,
		"cancelled":     gopay.SubscriptionStatusCanceled,
		"completed":     gopay.SubscriptionStatusCompleted,
		"expired":       gopay.SubscriptionStatusIncompleteExpired,
		"unknown":       gopay.SubscriptionStatusIncomplete,
	}
	for in, want := range cases {
		if got := mapSubscriptionStatus(in); got != want {
			t.Errorf("mapSubscriptionStatus(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestIntervalPeriodRoundTrip(t *testing.T) {
	intervals := []gopay.BillingInterval{
		gopay.BillingIntervalDay,
		gopay.BillingIntervalWeek,
		gopay.BillingIntervalMonth,
		gopay.BillingIntervalYear,
	}
	for _, iv := range intervals {
		period, ok := intervalToPeriod(iv)
		if !ok {
			t.Errorf("intervalToPeriod(%s) not ok", iv)
			continue
		}
		if got := periodToInterval(period); got != iv {
			t.Errorf("round trip %s -> %s -> %s", iv, period, got)
		}
	}
	if _, ok := intervalToPeriod(gopay.BillingInterval("decade")); ok {
		t.Error("intervalToPeriod(decade) = ok, want not ok")
	}
}
