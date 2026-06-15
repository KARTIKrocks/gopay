package gopay

import (
	"context"
	"errors"
	"time"
)

// SetupIntentProvider extends Provider with the ability to set up (tokenize and
// store) a payment method for future off-session charges without moving money.
//
// It is an optional interface: providers implement it only when their API
// supports a save-card-without-charging flow. The Client gates each method with a
// runtime type assertion and returns ErrUnsupported for providers that don't
// implement it. SetupIntents are the standard primitive underpinning
// subscriptions and one-click checkout, where a stored mandate is charged later.
type SetupIntentProvider interface {
	Provider

	// CreateSetupIntent creates a setup intent. When the request carries a
	// PaymentMethodID, the provider confirms it immediately (mirroring how
	// CreatePayment confirms an inline payment method); otherwise the intent is
	// returned awaiting client-side confirmation via its ClientSecret.
	CreateSetupIntent(ctx context.Context, req *SetupIntentRequest) (*SetupIntent, error)

	// GetSetupIntent retrieves a setup intent by ID.
	GetSetupIntent(ctx context.Context, setupIntentID string) (*SetupIntent, error)

	// CancelSetupIntent cancels a setup intent.
	CancelSetupIntent(ctx context.Context, setupIntentID string) (*SetupIntent, error)
}

// SetupIntentUsage indicates how the stored payment method is intended to be
// charged later.
type SetupIntentUsage string

const (
	// SetupIntentUsageOffSession sets up a payment method to be charged when the
	// customer is not present (e.g. recurring subscription billing). This is the
	// default.
	SetupIntentUsageOffSession SetupIntentUsage = "off_session"
	// SetupIntentUsageOnSession sets up a payment method to be charged while the
	// customer is present (e.g. one-click checkout).
	SetupIntentUsageOnSession SetupIntentUsage = "on_session"
)

// String returns the string representation.
func (u SetupIntentUsage) String() string { return string(u) }

// SetupIntentRequest represents a setup-intent creation request.
type SetupIntentRequest struct {
	// CustomerID is the customer the payment method will be saved to. Optional,
	// but required by most providers for the stored method to be reusable
	// off-session.
	CustomerID string

	// PaymentMethodID is an existing payment method to attach and confirm
	// immediately. Optional; when empty the intent awaits client-side
	// confirmation via its ClientSecret.
	PaymentMethodID string

	// Usage indicates how the stored method will be charged later. Defaults to
	// SetupIntentUsageOffSession.
	Usage SetupIntentUsage

	// Description is an optional description.
	Description string

	// ReturnURL is the URL to redirect to after authentication (for 3DS).
	ReturnURL string

	// Metadata holds additional data.
	Metadata map[string]string

	// IdempotencyKey for idempotent requests.
	IdempotencyKey string
}

// NewSetupIntentRequest creates a new setup-intent request defaulting to
// off-session usage.
func NewSetupIntentRequest() *SetupIntentRequest {
	return &SetupIntentRequest{
		Usage:    SetupIntentUsageOffSession,
		Metadata: make(map[string]string),
	}
}

// WithCustomer sets the customer ID.
func (r *SetupIntentRequest) WithCustomer(customerID string) *SetupIntentRequest {
	r.CustomerID = customerID
	return r
}

// WithPaymentMethod sets the payment method ID to attach and confirm.
func (r *SetupIntentRequest) WithPaymentMethod(paymentMethodID string) *SetupIntentRequest {
	r.PaymentMethodID = paymentMethodID
	return r
}

// WithUsage sets the intended usage.
func (r *SetupIntentRequest) WithUsage(usage SetupIntentUsage) *SetupIntentRequest {
	r.Usage = usage
	return r
}

// WithDescription sets the description.
func (r *SetupIntentRequest) WithDescription(desc string) *SetupIntentRequest {
	r.Description = desc
	return r
}

// WithReturnURL sets the return URL.
func (r *SetupIntentRequest) WithReturnURL(url string) *SetupIntentRequest {
	r.ReturnURL = url
	return r
}

// WithMetadata adds metadata.
func (r *SetupIntentRequest) WithMetadata(key, value string) *SetupIntentRequest {
	if r.Metadata == nil {
		r.Metadata = make(map[string]string)
	}
	r.Metadata[key] = value
	return r
}

// WithIdempotencyKey sets the idempotency key.
func (r *SetupIntentRequest) WithIdempotencyKey(key string) *SetupIntentRequest {
	r.IdempotencyKey = key
	return r
}

// Validate validates the setup-intent request.
func (r *SetupIntentRequest) Validate() error {
	if r == nil {
		return errors.New("gopay: nil setup intent request")
	}
	if r.Usage != "" && r.Usage != SetupIntentUsageOffSession && r.Usage != SetupIntentUsageOnSession {
		return errors.New("gopay: invalid setup intent usage")
	}
	return nil
}

// SetupIntentStatus represents the status of a setup intent.
type SetupIntentStatus string

// Setup intent status values.
const (
	SetupIntentStatusRequiresPaymentMethod SetupIntentStatus = "requires_payment_method"
	SetupIntentStatusRequiresConfirmation  SetupIntentStatus = "requires_confirmation"
	SetupIntentStatusRequiresAction        SetupIntentStatus = "requires_action"
	SetupIntentStatusProcessing            SetupIntentStatus = "processing"
	SetupIntentStatusSucceeded             SetupIntentStatus = "succeeded"
	SetupIntentStatusCanceled              SetupIntentStatus = "canceled"
)

// String returns the string representation.
func (s SetupIntentStatus) String() string { return string(s) }

// SetupIntent represents the setup of a payment method for future off-session
// charges.
type SetupIntent struct {
	// ID is the setup intent ID.
	ID string

	// Status is the setup intent status.
	Status SetupIntentStatus

	// CustomerID is the associated customer ID.
	CustomerID string

	// PaymentMethodID is the payment method being set up (set once collected).
	PaymentMethodID string

	// Usage indicates how the stored method will be charged later.
	Usage SetupIntentUsage

	// Description is the setup intent description.
	Description string

	// ClientSecret is the client secret used to confirm the intent from a
	// frontend.
	ClientSecret string

	// RedirectURL is the URL for a 3DS / next-action redirect.
	RedirectURL string

	// FailureCode is the failure code if setup failed.
	FailureCode string

	// FailureMessage is the failure message if setup failed.
	FailureMessage string

	// Metadata holds additional data.
	Metadata map[string]string

	// CreatedAt is the creation timestamp.
	CreatedAt time.Time

	// Provider is the provider name.
	Provider string

	// Raw contains the raw provider response.
	Raw map[string]any
}

// IsSucceeded returns true if the payment method was successfully set up.
func (s *SetupIntent) IsSucceeded() bool {
	return s.Status == SetupIntentStatusSucceeded
}

// RequiresAction returns true if the setup intent requires additional action
// (e.g. 3DS authentication).
func (s *SetupIntent) RequiresAction() bool {
	return s.Status == SetupIntentStatusRequiresAction
}
