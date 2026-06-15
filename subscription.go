package gopay

import (
	"context"
	"errors"
	"time"
)

// SubscriptionProvider extends Provider with recurring-billing capabilities:
// plans (recurring prices) and subscriptions that charge a customer's stored
// payment method each billing cycle.
//
// It is an optional interface: providers implement it only when their API
// supports subscriptions. The Client gates each method with a runtime type
// assertion and returns ErrUnsupported for providers that don't implement it.
// Subscriptions charge a saved off-session payment method, so they build on the
// setup-intent flow (see SetupIntentProvider).
type SubscriptionProvider interface {
	Provider

	// CreatePlan creates a recurring plan (amount + interval).
	CreatePlan(ctx context.Context, req *PlanRequest) (*Plan, error)

	// GetPlan retrieves a plan by ID.
	GetPlan(ctx context.Context, planID string) (*Plan, error)

	// CreateSubscription subscribes a customer to a plan, charging their payment
	// method each billing cycle.
	CreateSubscription(ctx context.Context, req *SubscriptionRequest) (*Subscription, error)

	// GetSubscription retrieves a subscription by ID.
	GetSubscription(ctx context.Context, subscriptionID string) (*Subscription, error)

	// CancelSubscription cancels a subscription, either immediately or at the end
	// of the current billing period (see CancelOptions). A nil opts cancels
	// immediately.
	CancelSubscription(ctx context.Context, subscriptionID string, opts *CancelOptions) (*Subscription, error)
}

// BillingInterval is the unit of a plan's recurring billing period.
type BillingInterval string

// Billing interval values.
const (
	BillingIntervalDay   BillingInterval = "day"
	BillingIntervalWeek  BillingInterval = "week"
	BillingIntervalMonth BillingInterval = "month"
	BillingIntervalYear  BillingInterval = "year"
)

// String returns the string representation.
func (b BillingInterval) String() string { return string(b) }

// validBillingInterval reports whether b is a recognized billing interval.
func validBillingInterval(b BillingInterval) bool {
	switch b {
	case BillingIntervalDay, BillingIntervalWeek, BillingIntervalMonth, BillingIntervalYear:
		return true
	default:
		return false
	}
}

// PlanRequest represents a plan-creation request.
type PlanRequest struct {
	// Amount is the recurring charge per interval.
	Amount *Amount

	// Interval is the billing interval unit (day/week/month/year).
	Interval BillingInterval

	// IntervalCount is the number of intervals between charges (e.g. Interval
	// month + IntervalCount 3 = quarterly). Defaults to 1.
	IntervalCount int

	// Name is a human-readable plan name.
	Name string

	// Metadata holds additional data.
	Metadata map[string]string

	// IdempotencyKey for idempotent requests.
	IdempotencyKey string
}

// NewPlanRequest creates a new plan request for the given amount and interval,
// defaulting IntervalCount to 1.
func NewPlanRequest(amount *Amount, interval BillingInterval) *PlanRequest {
	return &PlanRequest{
		Amount:        amount,
		Interval:      interval,
		IntervalCount: 1,
		Metadata:      make(map[string]string),
	}
}

// WithName sets the plan name.
func (r *PlanRequest) WithName(name string) *PlanRequest {
	r.Name = name
	return r
}

// WithIntervalCount sets the number of intervals between charges.
func (r *PlanRequest) WithIntervalCount(count int) *PlanRequest {
	r.IntervalCount = count
	return r
}

// WithMetadata adds metadata.
func (r *PlanRequest) WithMetadata(key, value string) *PlanRequest {
	if r.Metadata == nil {
		r.Metadata = make(map[string]string)
	}
	r.Metadata[key] = value
	return r
}

// WithIdempotencyKey sets the idempotency key.
func (r *PlanRequest) WithIdempotencyKey(key string) *PlanRequest {
	r.IdempotencyKey = key
	return r
}

// Validate validates the plan request.
func (r *PlanRequest) Validate() error {
	if r == nil {
		return errors.New("gopay: nil plan request")
	}
	if err := r.Amount.Validate(); err != nil {
		return err
	}
	if !validBillingInterval(r.Interval) {
		return errors.New("gopay: invalid billing interval")
	}
	if r.IntervalCount < 0 {
		return errors.New("gopay: invalid interval count")
	}
	return nil
}

// Plan represents a recurring price a customer can subscribe to.
type Plan struct {
	// ID is the plan ID.
	ID string

	// Name is the human-readable plan name.
	Name string

	// Amount is the recurring charge per interval.
	Amount *Amount

	// Interval is the billing interval unit.
	Interval BillingInterval

	// IntervalCount is the number of intervals between charges.
	IntervalCount int

	// Metadata holds additional data.
	Metadata map[string]string

	// CreatedAt is the creation timestamp.
	CreatedAt time.Time

	// Provider is the provider name.
	Provider string

	// Raw contains the raw provider response.
	Raw map[string]any
}

// SubscriptionRequest represents a subscription-creation request.
type SubscriptionRequest struct {
	// CustomerID is the customer to subscribe.
	CustomerID string

	// PlanID is the plan to subscribe the customer to.
	PlanID string

	// PaymentMethodID is the payment method to charge each cycle. Optional when
	// the customer already has a default payment method.
	PaymentMethodID string

	// TrialDays is the number of trial days before the first charge. Zero means
	// no trial.
	TrialDays int

	// Metadata holds additional data.
	Metadata map[string]string

	// IdempotencyKey for idempotent requests.
	IdempotencyKey string
}

// NewSubscriptionRequest creates a new subscription request for the given
// customer and plan.
func NewSubscriptionRequest(customerID, planID string) *SubscriptionRequest {
	return &SubscriptionRequest{
		CustomerID: customerID,
		PlanID:     planID,
		Metadata:   make(map[string]string),
	}
}

// WithPaymentMethod sets the payment method to charge each cycle.
func (r *SubscriptionRequest) WithPaymentMethod(paymentMethodID string) *SubscriptionRequest {
	r.PaymentMethodID = paymentMethodID
	return r
}

// WithTrialDays sets the number of trial days before the first charge.
func (r *SubscriptionRequest) WithTrialDays(days int) *SubscriptionRequest {
	r.TrialDays = days
	return r
}

// WithMetadata adds metadata.
func (r *SubscriptionRequest) WithMetadata(key, value string) *SubscriptionRequest {
	if r.Metadata == nil {
		r.Metadata = make(map[string]string)
	}
	r.Metadata[key] = value
	return r
}

// WithIdempotencyKey sets the idempotency key.
func (r *SubscriptionRequest) WithIdempotencyKey(key string) *SubscriptionRequest {
	r.IdempotencyKey = key
	return r
}

// Validate validates the subscription request.
func (r *SubscriptionRequest) Validate() error {
	if r == nil {
		return errors.New("gopay: nil subscription request")
	}
	if r.CustomerID == "" {
		return errors.New("gopay: customer ID required for subscription")
	}
	if r.PlanID == "" {
		return errors.New("gopay: plan ID required for subscription")
	}
	if r.TrialDays < 0 {
		return errors.New("gopay: invalid trial days")
	}
	return nil
}

// CancelOptions controls how a subscription is canceled.
type CancelOptions struct {
	// AtPeriodEnd cancels at the end of the current billing period instead of
	// immediately. The subscription stays active (and continues to bill nothing
	// further) until the period ends.
	AtPeriodEnd bool
}

// SubscriptionStatus represents the status of a subscription.
type SubscriptionStatus string

// Subscription status values.
const (
	SubscriptionStatusActive            SubscriptionStatus = "active"
	SubscriptionStatusTrialing          SubscriptionStatus = "trialing"
	SubscriptionStatusPastDue           SubscriptionStatus = "past_due"
	SubscriptionStatusCanceled          SubscriptionStatus = "canceled"
	SubscriptionStatusIncomplete        SubscriptionStatus = "incomplete"
	SubscriptionStatusIncompleteExpired SubscriptionStatus = "incomplete_expired"
	SubscriptionStatusUnpaid            SubscriptionStatus = "unpaid"
)

// String returns the string representation.
func (s SubscriptionStatus) String() string { return string(s) }

// Subscription represents a recurring billing arrangement between a customer and
// a plan.
type Subscription struct {
	// ID is the subscription ID.
	ID string

	// CustomerID is the subscribed customer.
	CustomerID string

	// PlanID is the plan being billed.
	PlanID string

	// PaymentMethodID is the payment method charged each cycle.
	PaymentMethodID string

	// Status is the subscription status.
	Status SubscriptionStatus

	// CurrentPeriodStart is the start of the current billing period.
	CurrentPeriodStart time.Time

	// CurrentPeriodEnd is the end of the current billing period (next charge).
	CurrentPeriodEnd time.Time

	// CancelAtPeriodEnd reports whether the subscription is set to cancel at the
	// end of the current period.
	CancelAtPeriodEnd bool

	// CanceledAt is when the subscription was canceled; zero if not canceled.
	CanceledAt time.Time

	// Metadata holds additional data.
	Metadata map[string]string

	// CreatedAt is the creation timestamp.
	CreatedAt time.Time

	// Provider is the provider name.
	Provider string

	// Raw contains the raw provider response.
	Raw map[string]any
}

// IsActive returns true if the subscription is active or in a trial period.
func (s *Subscription) IsActive() bool {
	return s.Status == SubscriptionStatusActive || s.Status == SubscriptionStatusTrialing
}
