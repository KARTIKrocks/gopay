package gopay

import (
	"context"
	"errors"
	"strings"
	"time"
)

// validCurrencies contains common ISO 4217 currency codes.
var validCurrencies = map[string]bool{
	"USD": true, "EUR": true, "GBP": true, "INR": true, "JPY": true,
	"CAD": true, "AUD": true, "CHF": true, "CNY": true, "HKD": true,
	"SGD": true, "SEK": true, "NOK": true, "DKK": true, "NZD": true,
	"ZAR": true, "MXN": true, "BRL": true, "PLN": true, "THB": true,
	"MYR": true, "IDR": true, "PHP": true, "CZK": true, "HUF": true,
	"ILS": true, "KRW": true, "TRY": true, "RUB": true, "AED": true,
	"SAR": true, "TWD": true, "ARS": true, "CLP": true, "COP": true,
	"PEN": true, "NGN": true, "EGP": true, "KES": true, "GHS": true,
	"BDT": true, "PKR": true, "LKR": true, "MMK": true, "VND": true,
}

// Sentinel errors for payment operations.
var (
	ErrInvalidConfig          = errors.New("gopay: invalid configuration")
	ErrInvalidAmount          = errors.New("gopay: invalid amount")
	ErrInvalidCurrency        = errors.New("gopay: invalid currency")
	ErrInvalidCard            = errors.New("gopay: invalid card")
	ErrCardDeclined           = errors.New("gopay: card declined")
	ErrInsufficientFunds      = errors.New("gopay: insufficient funds")
	ErrExpiredCard            = errors.New("gopay: expired card")
	ErrPaymentFailed          = errors.New("gopay: payment failed")
	ErrRefundFailed           = errors.New("gopay: refund failed")
	ErrNotFound               = errors.New("gopay: not found")
	ErrAlreadyRefunded        = errors.New("gopay: already refunded")
	ErrAlreadyCaptured        = errors.New("gopay: already captured")
	ErrAuthenticationRequired = errors.New("gopay: authentication required")
	ErrProviderError          = errors.New("gopay: provider error")
	ErrUnsupported            = errors.New("gopay: operation not supported")
)

// Provider represents a payment provider.
type Provider interface {
	// Name returns the provider name.
	Name() string

	// CreatePayment creates a new payment.
	CreatePayment(ctx context.Context, req *PaymentRequest) (*Payment, error)

	// GetPayment retrieves a payment by ID.
	GetPayment(ctx context.Context, paymentID string) (*Payment, error)

	// CapturePayment captures an authorized payment.
	CapturePayment(ctx context.Context, paymentID string, amount *Amount) (*Payment, error)

	// CancelPayment cancels an authorized payment.
	CancelPayment(ctx context.Context, paymentID string) (*Payment, error)

	// Refund creates a refund for a payment.
	Refund(ctx context.Context, req *RefundRequest) (*Refund, error)

	// GetRefund retrieves a refund by ID.
	GetRefund(ctx context.Context, refundID string) (*Refund, error)
}

// CustomerProvider extends Provider with customer management.
type CustomerProvider interface {
	Provider

	// CreateCustomer creates a new customer.
	CreateCustomer(ctx context.Context, req *CustomerRequest) (*Customer, error)

	// GetCustomer retrieves a customer by ID.
	GetCustomer(ctx context.Context, customerID string) (*Customer, error)

	// UpdateCustomer updates a customer.
	UpdateCustomer(ctx context.Context, customerID string, req *CustomerRequest) (*Customer, error)

	// DeleteCustomer deletes a customer.
	DeleteCustomer(ctx context.Context, customerID string) error
}

// PaymentMethodProvider extends Provider with payment method management.
type PaymentMethodProvider interface {
	Provider

	// AttachPaymentMethod attaches a payment method to a customer.
	AttachPaymentMethod(ctx context.Context, customerID, paymentMethodID string) error

	// DetachPaymentMethod detaches a payment method from a customer.
	DetachPaymentMethod(ctx context.Context, paymentMethodID string) error

	// ListPaymentMethods lists payment methods for a customer.
	ListPaymentMethods(ctx context.Context, customerID string) ([]*PaymentMethod, error)
}

// WebhookProvider extends Provider with webhook verification.
type WebhookProvider interface {
	// VerifyWebhook verifies and parses a webhook event.
	// The headers map should contain the relevant signature headers from the HTTP request.
	VerifyWebhook(ctx context.Context, payload []byte, headers map[string]string) (*WebhookEvent, error)
}

// Amount represents a monetary amount.
type Amount struct {
	// Value is the amount in the smallest currency unit (e.g., cents).
	Value int64

	// Currency is the three-letter ISO currency code.
	Currency string
}

// NewAmount creates a new amount.
func NewAmount(value int64, currency string) *Amount {
	return &Amount{
		Value:    value,
		Currency: strings.ToUpper(currency),
	}
}

// USD creates a USD amount (in cents).
func USD(cents int64) *Amount {
	return NewAmount(cents, "USD")
}

// EUR creates a EUR amount (in cents).
func EUR(cents int64) *Amount {
	return NewAmount(cents, "EUR")
}

// GBP creates a GBP amount (in pence).
func GBP(pence int64) *Amount {
	return NewAmount(pence, "GBP")
}

// INR creates an INR amount (in paise).
func INR(paise int64) *Amount {
	return NewAmount(paise, "INR")
}

// Validate validates the amount.
// Currency should already be uppercase (NewAmount normalizes it).
func (a *Amount) Validate() error {
	if a == nil {
		return ErrInvalidAmount
	}
	if a.Value < 0 {
		return ErrInvalidAmount
	}
	if a.Currency == "" {
		return ErrInvalidCurrency
	}
	if !validCurrencies[strings.ToUpper(a.Currency)] {
		return ErrInvalidCurrency
	}
	return nil
}

// PaymentRequest represents a payment creation request.
type PaymentRequest struct {
	// Amount is the payment amount.
	Amount *Amount

	// Description is an optional description.
	Description string

	// CustomerID is the customer ID (optional).
	CustomerID string

	// PaymentMethodID is the payment method ID (optional).
	PaymentMethodID string

	// ReturnURL is the URL to redirect after payment (for 3DS).
	ReturnURL string

	// CaptureMethod determines when to capture funds.
	CaptureMethod CaptureMethod

	// Metadata holds additional data.
	Metadata map[string]string

	// IdempotencyKey for idempotent requests.
	IdempotencyKey string
}

// NewPaymentRequest creates a new payment request.
func NewPaymentRequest(amount *Amount) *PaymentRequest {
	return &PaymentRequest{
		Amount:        amount,
		CaptureMethod: CaptureAutomatic,
		Metadata:      make(map[string]string),
	}
}

// WithDescription sets the description.
func (r *PaymentRequest) WithDescription(desc string) *PaymentRequest {
	r.Description = desc
	return r
}

// WithCustomer sets the customer ID.
func (r *PaymentRequest) WithCustomer(customerID string) *PaymentRequest {
	r.CustomerID = customerID
	return r
}

// WithPaymentMethod sets the payment method ID.
func (r *PaymentRequest) WithPaymentMethod(paymentMethodID string) *PaymentRequest {
	r.PaymentMethodID = paymentMethodID
	return r
}

// WithReturnURL sets the return URL.
func (r *PaymentRequest) WithReturnURL(url string) *PaymentRequest {
	r.ReturnURL = url
	return r
}

// WithCaptureMethod sets the capture method.
func (r *PaymentRequest) WithCaptureMethod(method CaptureMethod) *PaymentRequest {
	r.CaptureMethod = method
	return r
}

// WithMetadata adds metadata.
func (r *PaymentRequest) WithMetadata(key, value string) *PaymentRequest {
	if r.Metadata == nil {
		r.Metadata = make(map[string]string)
	}
	r.Metadata[key] = value
	return r
}

// WithIdempotencyKey sets the idempotency key.
func (r *PaymentRequest) WithIdempotencyKey(key string) *PaymentRequest {
	r.IdempotencyKey = key
	return r
}

// Validate validates the payment request.
func (r *PaymentRequest) Validate() error {
	if r == nil {
		return errors.New("gopay: nil payment request")
	}
	if err := r.Amount.Validate(); err != nil {
		return err
	}
	if r.CaptureMethod != "" && r.CaptureMethod != CaptureAutomatic && r.CaptureMethod != CaptureManual {
		return errors.New("gopay: invalid capture method")
	}
	return nil
}

// CaptureMethod determines when funds are captured.
type CaptureMethod string

const (
	// CaptureAutomatic captures funds immediately.
	CaptureAutomatic CaptureMethod = "automatic"
	// CaptureManual requires a separate capture call.
	CaptureManual CaptureMethod = "manual"
)

// String returns the string representation.
func (c CaptureMethod) String() string { return string(c) }

// Payment represents a payment.
type Payment struct {
	// ID is the payment ID.
	ID string

	// Amount is the payment amount.
	Amount *Amount

	// Status is the payment status.
	Status PaymentStatus

	// Description is the payment description.
	Description string

	// CustomerID is the associated customer ID.
	CustomerID string

	// PaymentMethodID is the payment method used.
	PaymentMethodID string

	// CaptureMethod is the capture method.
	CaptureMethod CaptureMethod

	// AmountCaptured is the amount captured.
	AmountCaptured int64

	// AmountRefunded is the amount refunded.
	AmountRefunded int64

	// FailureCode is the failure code if failed.
	FailureCode string

	// FailureMessage is the failure message if failed.
	FailureMessage string

	// ClientSecret is the client secret (for frontend).
	ClientSecret string

	// RedirectURL is the URL for 3DS redirect.
	RedirectURL string

	// Metadata holds additional data.
	Metadata map[string]string

	// CreatedAt is the creation timestamp.
	CreatedAt time.Time

	// Provider is the provider name.
	Provider string

	// Raw contains the raw provider response.
	Raw map[string]any
}

// IsSuccessful returns true if the payment was successful.
func (p *Payment) IsSuccessful() bool {
	return p.Status == PaymentStatusSucceeded
}

// RequiresAction returns true if the payment requires additional action.
func (p *Payment) RequiresAction() bool {
	return p.Status == PaymentStatusRequiresAction
}

// IsCaptured returns true if the payment was captured.
func (p *Payment) IsCaptured() bool {
	return p.AmountCaptured > 0
}

// PaymentStatus represents the status of a payment.
type PaymentStatus string

const (
	PaymentStatusPending         PaymentStatus = "pending"
	PaymentStatusRequiresAction  PaymentStatus = "requires_action"
	PaymentStatusProcessing      PaymentStatus = "processing"
	PaymentStatusSucceeded       PaymentStatus = "succeeded"
	PaymentStatusFailed          PaymentStatus = "failed"
	PaymentStatusCanceled        PaymentStatus = "canceled"
	PaymentStatusRequiresCapture PaymentStatus = "requires_capture"
)

// String returns the string representation.
func (s PaymentStatus) String() string { return string(s) }

// RefundRequest represents a refund request.
type RefundRequest struct {
	// PaymentID is the payment to refund.
	PaymentID string

	// Amount is the refund amount (nil for full refund).
	Amount *Amount

	// Reason is the refund reason.
	Reason RefundReason

	// Metadata holds additional data.
	Metadata map[string]string

	// IdempotencyKey for idempotent requests.
	IdempotencyKey string
}

// NewRefundRequest creates a new refund request.
func NewRefundRequest(paymentID string) *RefundRequest {
	return &RefundRequest{
		PaymentID: paymentID,
		Metadata:  make(map[string]string),
	}
}

// WithAmount sets the refund amount.
func (r *RefundRequest) WithAmount(amount *Amount) *RefundRequest {
	r.Amount = amount
	return r
}

// WithReason sets the refund reason.
func (r *RefundRequest) WithReason(reason RefundReason) *RefundRequest {
	r.Reason = reason
	return r
}

// WithMetadata adds metadata.
func (r *RefundRequest) WithMetadata(key, value string) *RefundRequest {
	if r.Metadata == nil {
		r.Metadata = make(map[string]string)
	}
	r.Metadata[key] = value
	return r
}

// WithIdempotencyKey sets the idempotency key.
func (r *RefundRequest) WithIdempotencyKey(key string) *RefundRequest {
	r.IdempotencyKey = key
	return r
}

// RefundReason represents the reason for a refund.
type RefundReason string

const (
	RefundReasonDuplicate           RefundReason = "duplicate"
	RefundReasonFraudulent          RefundReason = "fraudulent"
	RefundReasonRequestedByCustomer RefundReason = "requested_by_customer"
	RefundReasonOther               RefundReason = "other"
)

// String returns the string representation.
func (r RefundReason) String() string { return string(r) }

// Validate validates the refund request.
func (r *RefundRequest) Validate() error {
	if r == nil {
		return errors.New("gopay: nil refund request")
	}
	if r.PaymentID == "" {
		return errors.New("gopay: payment ID required for refund")
	}
	if r.Amount != nil {
		if err := r.Amount.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// Refund represents a refund.
type Refund struct {
	// ID is the refund ID.
	ID string

	// PaymentID is the refunded payment ID.
	PaymentID string

	// Amount is the refund amount.
	Amount *Amount

	// Status is the refund status.
	Status RefundStatus

	// Reason is the refund reason.
	Reason RefundReason

	// FailureReason is the failure reason if failed.
	FailureReason string

	// Metadata holds additional data.
	Metadata map[string]string

	// CreatedAt is the creation timestamp.
	CreatedAt time.Time

	// Provider is the provider name.
	Provider string

	// Raw contains the raw provider response.
	Raw map[string]any
}

// IsSuccessful returns true if the refund was successful.
func (r *Refund) IsSuccessful() bool {
	return r.Status == RefundStatusSucceeded
}

// RefundStatus represents the status of a refund.
type RefundStatus string

const (
	RefundStatusPending   RefundStatus = "pending"
	RefundStatusSucceeded RefundStatus = "succeeded"
	RefundStatusFailed    RefundStatus = "failed"
	RefundStatusCanceled  RefundStatus = "canceled"
)

// String returns the string representation.
func (s RefundStatus) String() string { return string(s) }

// CustomerRequest represents a customer creation/update request.
type CustomerRequest struct {
	// Email is the customer's email.
	Email string

	// Name is the customer's name.
	Name string

	// Phone is the customer's phone.
	Phone string

	// Description is an optional description.
	Description string

	// Metadata holds additional data.
	Metadata map[string]string
}

// NewCustomerRequest creates a new customer request.
func NewCustomerRequest(email string) *CustomerRequest {
	return &CustomerRequest{
		Email:    email,
		Metadata: make(map[string]string),
	}
}

// WithName sets the name.
func (r *CustomerRequest) WithName(name string) *CustomerRequest {
	r.Name = name
	return r
}

// WithPhone sets the phone.
func (r *CustomerRequest) WithPhone(phone string) *CustomerRequest {
	r.Phone = phone
	return r
}

// WithDescription sets the description.
func (r *CustomerRequest) WithDescription(desc string) *CustomerRequest {
	r.Description = desc
	return r
}

// WithMetadata adds metadata.
func (r *CustomerRequest) WithMetadata(key, value string) *CustomerRequest {
	if r.Metadata == nil {
		r.Metadata = make(map[string]string)
	}
	r.Metadata[key] = value
	return r
}

// Validate validates the customer request.
func (r *CustomerRequest) Validate() error {
	if r == nil {
		return errors.New("gopay: nil customer request")
	}
	if r.Email == "" {
		return errors.New("gopay: email required")
	}
	return nil
}

// Customer represents a customer.
type Customer struct {
	// ID is the customer ID.
	ID string

	// Email is the customer's email.
	Email string

	// Name is the customer's name.
	Name string

	// Phone is the customer's phone.
	Phone string

	// Description is the description.
	Description string

	// DefaultPaymentMethodID is the default payment method.
	DefaultPaymentMethodID string

	// Metadata holds additional data.
	Metadata map[string]string

	// CreatedAt is the creation timestamp.
	CreatedAt time.Time

	// Provider is the provider name.
	Provider string

	// Raw contains the raw provider response.
	Raw map[string]any
}

// PaymentMethod represents a payment method.
type PaymentMethod struct {
	// ID is the payment method ID.
	ID string

	// Type is the payment method type.
	Type PaymentMethodType

	// CustomerID is the associated customer ID.
	CustomerID string

	// Card contains card details (if type is card).
	Card *CardDetails

	// BillingDetails contains billing information.
	BillingDetails *BillingDetails

	// CreatedAt is the creation timestamp.
	CreatedAt time.Time

	// Provider is the provider name.
	Provider string

	// Raw contains the raw provider response.
	Raw map[string]any
}

// PaymentMethodType represents the type of payment method.
type PaymentMethodType string

const (
	PaymentMethodCard        PaymentMethodType = "card"
	PaymentMethodBankAccount PaymentMethodType = "bank_account"
	PaymentMethodWallet      PaymentMethodType = "wallet"
	PaymentMethodUPI         PaymentMethodType = "upi"
	PaymentMethodNetBanking  PaymentMethodType = "netbanking"
)

// String returns the string representation.
func (t PaymentMethodType) String() string { return string(t) }

// CardDetails contains card information.
type CardDetails struct {
	// Brand is the card brand (visa, mastercard, etc.).
	Brand string

	// Last4 is the last 4 digits.
	Last4 string

	// ExpMonth is the expiration month.
	ExpMonth int

	// ExpYear is the expiration year.
	ExpYear int

	// Funding is the funding type (credit, debit, prepaid).
	Funding string

	// Country is the card's country.
	Country string
}

// BillingDetails contains billing information.
type BillingDetails struct {
	// Name is the billing name.
	Name string

	// Email is the billing email.
	Email string

	// Phone is the billing phone.
	Phone string

	// Address is the billing address.
	Address *Address
}

// Address represents a physical address.
type Address struct {
	Line1      string
	Line2      string
	City       string
	State      string
	PostalCode string
	Country    string
}

// Client is the main payment client.
type Client struct {
	provider Provider
}

// NewClient creates a new payment client.
func NewClient(provider Provider) (*Client, error) {
	if provider == nil {
		return nil, errors.New("gopay: provider must not be nil")
	}
	return &Client{provider: provider}, nil
}

// CreatePayment creates a new payment.
func (c *Client) CreatePayment(ctx context.Context, req *PaymentRequest) (*Payment, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	return c.provider.CreatePayment(ctx, req)
}

// GetPayment retrieves a payment.
func (c *Client) GetPayment(ctx context.Context, paymentID string) (*Payment, error) {
	if paymentID == "" {
		return nil, ErrNotFound
	}
	return c.provider.GetPayment(ctx, paymentID)
}

// CapturePayment captures an authorized payment.
func (c *Client) CapturePayment(ctx context.Context, paymentID string, amount *Amount) (*Payment, error) {
	if paymentID == "" {
		return nil, ErrNotFound
	}
	if amount != nil {
		if err := amount.Validate(); err != nil {
			return nil, err
		}
	}
	return c.provider.CapturePayment(ctx, paymentID, amount)
}

// CancelPayment cancels an authorized payment.
func (c *Client) CancelPayment(ctx context.Context, paymentID string) (*Payment, error) {
	if paymentID == "" {
		return nil, ErrNotFound
	}
	return c.provider.CancelPayment(ctx, paymentID)
}

// Refund creates a refund.
func (c *Client) Refund(ctx context.Context, req *RefundRequest) (*Refund, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	return c.provider.Refund(ctx, req)
}

// FullRefund creates a full refund for a payment.
func (c *Client) FullRefund(ctx context.Context, paymentID string) (*Refund, error) {
	req := NewRefundRequest(paymentID)
	if err := req.Validate(); err != nil {
		return nil, err
	}
	return c.provider.Refund(ctx, req)
}

// GetRefund retrieves a refund.
func (c *Client) GetRefund(ctx context.Context, refundID string) (*Refund, error) {
	if refundID == "" {
		return nil, ErrNotFound
	}
	return c.provider.GetRefund(ctx, refundID)
}

// VerifyWebhook verifies and parses a webhook event (if supported).
func (c *Client) VerifyWebhook(ctx context.Context, payload []byte, headers map[string]string) (*WebhookEvent, error) {
	wp, ok := c.provider.(WebhookProvider)
	if !ok {
		return nil, ErrUnsupported
	}
	return wp.VerifyWebhook(ctx, payload, headers)
}

// Provider returns the underlying provider.
func (c *Client) Provider() Provider {
	return c.provider
}

// ProviderName returns the provider name.
func (c *Client) ProviderName() string {
	return c.provider.Name()
}

// CreateCustomer creates a customer (if supported).
func (c *Client) CreateCustomer(ctx context.Context, req *CustomerRequest) (*Customer, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	cp, ok := c.provider.(CustomerProvider)
	if !ok {
		return nil, ErrUnsupported
	}
	return cp.CreateCustomer(ctx, req)
}

// GetCustomer retrieves a customer (if supported).
func (c *Client) GetCustomer(ctx context.Context, customerID string) (*Customer, error) {
	cp, ok := c.provider.(CustomerProvider)
	if !ok {
		return nil, ErrUnsupported
	}
	return cp.GetCustomer(ctx, customerID)
}

// UpdateCustomer updates a customer (if supported).
func (c *Client) UpdateCustomer(ctx context.Context, customerID string, req *CustomerRequest) (*Customer, error) {
	cp, ok := c.provider.(CustomerProvider)
	if !ok {
		return nil, ErrUnsupported
	}
	return cp.UpdateCustomer(ctx, customerID, req)
}

// DeleteCustomer deletes a customer (if supported).
func (c *Client) DeleteCustomer(ctx context.Context, customerID string) error {
	cp, ok := c.provider.(CustomerProvider)
	if !ok {
		return ErrUnsupported
	}
	return cp.DeleteCustomer(ctx, customerID)
}

// AttachPaymentMethod attaches a payment method to a customer (if supported).
func (c *Client) AttachPaymentMethod(ctx context.Context, customerID, paymentMethodID string) error {
	pmp, ok := c.provider.(PaymentMethodProvider)
	if !ok {
		return ErrUnsupported
	}
	return pmp.AttachPaymentMethod(ctx, customerID, paymentMethodID)
}

// DetachPaymentMethod detaches a payment method from a customer (if supported).
func (c *Client) DetachPaymentMethod(ctx context.Context, paymentMethodID string) error {
	pmp, ok := c.provider.(PaymentMethodProvider)
	if !ok {
		return ErrUnsupported
	}
	return pmp.DetachPaymentMethod(ctx, paymentMethodID)
}

// ListPaymentMethods lists payment methods for a customer (if supported).
func (c *Client) ListPaymentMethods(ctx context.Context, customerID string) ([]*PaymentMethod, error) {
	pmp, ok := c.provider.(PaymentMethodProvider)
	if !ok {
		return nil, ErrUnsupported
	}
	return pmp.ListPaymentMethods(ctx, customerID)
}

// WebhookEvent represents a parsed webhook event.
type WebhookEvent struct {
	// ID is the event ID.
	ID string

	// Type is the event type.
	Type string

	// Provider is the provider name.
	Provider string

	// Raw contains the raw event payload.
	Raw []byte
}
