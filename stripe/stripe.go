// Package stripe provides a Stripe payment provider for gopay.
package stripe

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/KARTIKrocks/gopay"
	"github.com/stripe/stripe-go/v81"
	"github.com/stripe/stripe-go/v81/client"
	"github.com/stripe/stripe-go/v81/webhook"
)

// Config holds Stripe-specific configuration.
type Config struct {
	// SecretKey is the Stripe secret key.
	SecretKey string

	// WebhookSecret is the webhook signing secret.
	WebhookSecret string

	// HTTPClient is a custom HTTP client (optional).
	HTTPClient *http.Client
}

// DefaultConfig returns a default Stripe configuration.
func DefaultConfig() Config {
	return Config{}
}

// WithSecretKey sets the secret key.
func (c Config) WithSecretKey(key string) Config {
	c.SecretKey = key
	return c
}

// WithWebhookSecret sets the webhook secret.
func (c Config) WithWebhookSecret(secret string) Config {
	c.WebhookSecret = secret
	return c
}

// WithHTTPClient sets a custom HTTP client.
func (c Config) WithHTTPClient(httpClient *http.Client) Config {
	c.HTTPClient = httpClient
	return c
}

// Provider implements gopay.Provider for Stripe.
type Provider struct {
	config Config
	api    *client.API
}

// NewProvider creates a new Stripe provider.
func NewProvider(config Config) (*Provider, error) {
	if config.SecretKey == "" {
		return nil, fmt.Errorf("%w: secret key required", gopay.ErrInvalidConfig)
	}

	var backends *stripe.Backends
	if config.HTTPClient != nil {
		httpBackend := stripe.GetBackendWithConfig(stripe.APIBackend, &stripe.BackendConfig{
			HTTPClient: config.HTTPClient,
		})
		uploadBackend := stripe.GetBackendWithConfig(stripe.UploadsBackend, &stripe.BackendConfig{
			HTTPClient: config.HTTPClient,
		})
		backends = &stripe.Backends{
			API:     httpBackend,
			Uploads: uploadBackend,
		}
	}

	api := client.New(config.SecretKey, backends)

	return &Provider{config: config, api: api}, nil
}

// Name returns the provider name.
func (p *Provider) Name() string {
	return "stripe"
}

// CreatePayment creates a payment intent.
func (p *Provider) CreatePayment(ctx context.Context, req *gopay.PaymentRequest) (*gopay.Payment, error) {
	params := &stripe.PaymentIntentParams{
		Amount:   stripe.Int64(req.Amount.Value),
		Currency: stripe.String(req.Amount.Currency),
	}

	if req.Description != "" {
		params.Description = stripe.String(req.Description)
	}

	if req.CustomerID != "" {
		params.Customer = stripe.String(req.CustomerID)
	}

	if req.PaymentMethodID != "" {
		params.PaymentMethod = stripe.String(req.PaymentMethodID)
		params.Confirm = stripe.Bool(true)
	}

	if req.ReturnURL != "" {
		params.ReturnURL = stripe.String(req.ReturnURL)
	}

	if req.CaptureMethod == gopay.CaptureManual {
		params.CaptureMethod = stripe.String("manual")
	} else {
		params.CaptureMethod = stripe.String("automatic")
	}

	if len(req.Metadata) > 0 {
		meta := make(map[string]string, len(req.Metadata))
		for k, v := range req.Metadata {
			meta[k] = v
		}
		params.Metadata = meta
	}

	if req.IdempotencyKey != "" {
		params.SetIdempotencyKey(req.IdempotencyKey)
	}

	params.Context = ctx
	pi, err := p.api.PaymentIntents.New(params)
	if err != nil {
		return nil, p.mapError(err)
	}

	return p.mapPaymentIntent(pi), nil
}

// GetPayment retrieves a payment intent.
func (p *Provider) GetPayment(ctx context.Context, paymentID string) (*gopay.Payment, error) {
	params := &stripe.PaymentIntentParams{}
	params.Context = ctx
	pi, err := p.api.PaymentIntents.Get(paymentID, params)
	if err != nil {
		return nil, p.mapError(err)
	}

	return p.mapPaymentIntent(pi), nil
}

// CapturePayment captures a payment intent.
func (p *Provider) CapturePayment(ctx context.Context, paymentID string, amount *gopay.Amount) (*gopay.Payment, error) {
	params := &stripe.PaymentIntentCaptureParams{}

	if amount != nil {
		params.AmountToCapture = stripe.Int64(amount.Value)
	}

	params.Context = ctx
	pi, err := p.api.PaymentIntents.Capture(paymentID, params)
	if err != nil {
		return nil, p.mapError(err)
	}

	return p.mapPaymentIntent(pi), nil
}

// CancelPayment cancels a payment intent.
func (p *Provider) CancelPayment(ctx context.Context, paymentID string) (*gopay.Payment, error) {
	params := &stripe.PaymentIntentCancelParams{}
	params.Context = ctx
	pi, err := p.api.PaymentIntents.Cancel(paymentID, params)
	if err != nil {
		return nil, p.mapError(err)
	}

	return p.mapPaymentIntent(pi), nil
}

// Refund creates a refund.
func (p *Provider) Refund(ctx context.Context, req *gopay.RefundRequest) (*gopay.Refund, error) {
	params := &stripe.RefundParams{
		PaymentIntent: stripe.String(req.PaymentID),
	}

	if req.Amount != nil {
		params.Amount = stripe.Int64(req.Amount.Value)
	}

	if req.Reason != "" {
		params.Reason = stripe.String(p.mapRefundReason(req.Reason))
	}

	if len(req.Metadata) > 0 {
		meta := make(map[string]string, len(req.Metadata))
		for k, v := range req.Metadata {
			meta[k] = v
		}
		params.Metadata = meta
	}

	if req.IdempotencyKey != "" {
		params.SetIdempotencyKey(req.IdempotencyKey)
	}

	params.Context = ctx
	r, err := p.api.Refunds.New(params)
	if err != nil {
		return nil, p.mapError(err)
	}

	return p.mapRefund(r), nil
}

// GetRefund retrieves a refund.
func (p *Provider) GetRefund(ctx context.Context, refundID string) (*gopay.Refund, error) {
	params := &stripe.RefundParams{}
	params.Context = ctx
	r, err := p.api.Refunds.Get(refundID, params)
	if err != nil {
		return nil, p.mapError(err)
	}

	return p.mapRefund(r), nil
}

// CreateCustomer creates a customer.
func (p *Provider) CreateCustomer(ctx context.Context, req *gopay.CustomerRequest) (*gopay.Customer, error) {
	params := &stripe.CustomerParams{}

	if req.Email != "" {
		params.Email = stripe.String(req.Email)
	}
	if req.Name != "" {
		params.Name = stripe.String(req.Name)
	}
	if req.Phone != "" {
		params.Phone = stripe.String(req.Phone)
	}
	if req.Description != "" {
		params.Description = stripe.String(req.Description)
	}
	if len(req.Metadata) > 0 {
		meta := make(map[string]string, len(req.Metadata))
		for k, v := range req.Metadata {
			meta[k] = v
		}
		params.Metadata = meta
	}

	params.Context = ctx
	c, err := p.api.Customers.New(params)
	if err != nil {
		return nil, p.mapError(err)
	}

	return p.mapCustomer(c), nil
}

// GetCustomer retrieves a customer.
func (p *Provider) GetCustomer(ctx context.Context, customerID string) (*gopay.Customer, error) {
	params := &stripe.CustomerParams{}
	params.Context = ctx
	c, err := p.api.Customers.Get(customerID, params)
	if err != nil {
		return nil, p.mapError(err)
	}

	return p.mapCustomer(c), nil
}

// UpdateCustomer updates a customer.
func (p *Provider) UpdateCustomer(ctx context.Context, customerID string, req *gopay.CustomerRequest) (*gopay.Customer, error) {
	params := &stripe.CustomerParams{}

	if req.Email != "" {
		params.Email = stripe.String(req.Email)
	}
	if req.Name != "" {
		params.Name = stripe.String(req.Name)
	}
	if req.Phone != "" {
		params.Phone = stripe.String(req.Phone)
	}
	if req.Description != "" {
		params.Description = stripe.String(req.Description)
	}
	if len(req.Metadata) > 0 {
		meta := make(map[string]string, len(req.Metadata))
		for k, v := range req.Metadata {
			meta[k] = v
		}
		params.Metadata = meta
	}

	params.Context = ctx
	c, err := p.api.Customers.Update(customerID, params)
	if err != nil {
		return nil, p.mapError(err)
	}

	return p.mapCustomer(c), nil
}

// DeleteCustomer deletes a customer.
func (p *Provider) DeleteCustomer(ctx context.Context, customerID string) error {
	params := &stripe.CustomerParams{}
	params.Context = ctx
	_, err := p.api.Customers.Del(customerID, params)
	if err != nil {
		return p.mapError(err)
	}
	return nil
}

// AttachPaymentMethod attaches a payment method to a customer.
func (p *Provider) AttachPaymentMethod(ctx context.Context, customerID, paymentMethodID string) error {
	params := &stripe.PaymentMethodAttachParams{
		Customer: stripe.String(customerID),
	}
	params.Context = ctx
	_, err := p.api.PaymentMethods.Attach(paymentMethodID, params)
	if err != nil {
		return p.mapError(err)
	}
	return nil
}

// DetachPaymentMethod detaches a payment method.
func (p *Provider) DetachPaymentMethod(ctx context.Context, paymentMethodID string) error {
	params := &stripe.PaymentMethodDetachParams{}
	params.Context = ctx
	_, err := p.api.PaymentMethods.Detach(paymentMethodID, params)
	if err != nil {
		return p.mapError(err)
	}
	return nil
}

// ListPaymentMethods lists payment methods for a customer.
func (p *Provider) ListPaymentMethods(ctx context.Context, customerID string) ([]*gopay.PaymentMethod, error) {
	params := &stripe.PaymentMethodListParams{
		Customer: stripe.String(customerID),
	}
	params.Context = ctx

	iter := p.api.PaymentMethods.List(params)

	var methods []*gopay.PaymentMethod
	for iter.Next() {
		pm := iter.PaymentMethod()
		methods = append(methods, p.mapPaymentMethod(pm))
	}

	if err := iter.Err(); err != nil {
		return nil, p.mapError(err)
	}

	return methods, nil
}

// VerifyWebhook verifies and parses a Stripe webhook.
// Headers should contain "Stripe-Signature".
func (p *Provider) VerifyWebhook(_ context.Context, payload []byte, headers map[string]string) (*gopay.WebhookEvent, error) {
	signature := headers["Stripe-Signature"]
	if signature == "" {
		return nil, fmt.Errorf("%w: missing Stripe-Signature header", gopay.ErrProviderError)
	}
	if p.config.WebhookSecret == "" {
		return nil, fmt.Errorf("%w: webhook secret not configured", gopay.ErrInvalidConfig)
	}
	event, err := webhook.ConstructEvent(payload, signature, p.config.WebhookSecret)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", gopay.ErrProviderError, err)
	}
	return &gopay.WebhookEvent{
		ID:       event.ID,
		Type:     string(event.Type),
		Provider: "stripe",
		Raw:      event.Data.Raw,
	}, nil
}

// ParseWebhook parses a Stripe webhook event.
func ParseWebhook(payload []byte, signature, webhookSecret string) (*gopay.WebhookEvent, error) {
	event, err := webhook.ConstructEvent(payload, signature, webhookSecret)
	if err != nil {
		return nil, err
	}

	return &gopay.WebhookEvent{
		ID:       event.ID,
		Type:     string(event.Type),
		Provider: "stripe",
		Raw:      event.Data.Raw,
	}, nil
}

func (p *Provider) mapPaymentIntent(pi *stripe.PaymentIntent) *gopay.Payment {
	pay := &gopay.Payment{
		ID:              pi.ID,
		Amount:          gopay.NewAmount(pi.Amount, string(pi.Currency)),
		Status:          p.mapPaymentStatus(pi.Status),
		Description:     pi.Description,
		PaymentMethodID: "",
		CaptureMethod:   gopay.CaptureAutomatic,
		AmountCaptured:  pi.AmountReceived,
		ClientSecret:    pi.ClientSecret,
		Metadata:        pi.Metadata,
		CreatedAt:       time.Unix(pi.Created, 0),
		Provider:        p.Name(),
		Raw: map[string]any{
			"id":     pi.ID,
			"status": string(pi.Status),
		},
	}

	if pi.Customer != nil {
		pay.CustomerID = pi.Customer.ID
	}
	if pi.PaymentMethod != nil {
		pay.PaymentMethodID = pi.PaymentMethod.ID
	}
	if pi.LatestCharge != nil {
		pay.AmountRefunded = pi.LatestCharge.AmountRefunded
	}
	if pi.CaptureMethod == stripe.PaymentIntentCaptureMethodManual {
		pay.CaptureMethod = gopay.CaptureManual
	}
	if pi.LastPaymentError != nil {
		pay.FailureCode = string(pi.LastPaymentError.Code)
		pay.FailureMessage = pi.LastPaymentError.Msg
	}
	if pi.NextAction != nil && pi.NextAction.RedirectToURL != nil {
		pay.RedirectURL = pi.NextAction.RedirectToURL.URL
	}

	return pay
}

func (p *Provider) mapPaymentStatus(status stripe.PaymentIntentStatus) gopay.PaymentStatus {
	switch status {
	case stripe.PaymentIntentStatusRequiresPaymentMethod:
		return gopay.PaymentStatusPending
	case stripe.PaymentIntentStatusRequiresConfirmation:
		return gopay.PaymentStatusPending
	case stripe.PaymentIntentStatusRequiresAction:
		return gopay.PaymentStatusRequiresAction
	case stripe.PaymentIntentStatusProcessing:
		return gopay.PaymentStatusProcessing
	case stripe.PaymentIntentStatusSucceeded:
		return gopay.PaymentStatusSucceeded
	case stripe.PaymentIntentStatusCanceled:
		return gopay.PaymentStatusCanceled
	case stripe.PaymentIntentStatusRequiresCapture:
		return gopay.PaymentStatusRequiresCapture
	default:
		return gopay.PaymentStatusPending
	}
}

func (p *Provider) mapRefund(r *stripe.Refund) *gopay.Refund {
	var paymentID string
	if r.PaymentIntent != nil {
		paymentID = r.PaymentIntent.ID
	}

	ref := &gopay.Refund{
		ID:        r.ID,
		PaymentID: paymentID,
		Amount:    gopay.NewAmount(r.Amount, string(r.Currency)),
		Status:    p.mapRefundStatus(r.Status),
		Metadata:  r.Metadata,
		CreatedAt: time.Unix(r.Created, 0),
		Provider:  p.Name(),
		Raw: map[string]any{
			"id":     r.ID,
			"status": string(r.Status),
		},
	}

	if r.Reason != "" {
		ref.Reason = p.reverseMapRefundReason(string(r.Reason))
	}
	if r.FailureReason != "" {
		ref.FailureReason = string(r.FailureReason)
	}

	return ref
}

func (p *Provider) mapRefundStatus(status stripe.RefundStatus) gopay.RefundStatus {
	switch status {
	case stripe.RefundStatusPending:
		return gopay.RefundStatusPending
	case stripe.RefundStatusSucceeded:
		return gopay.RefundStatusSucceeded
	case stripe.RefundStatusFailed:
		return gopay.RefundStatusFailed
	case stripe.RefundStatusCanceled:
		return gopay.RefundStatusCanceled
	default:
		return gopay.RefundStatusPending
	}
}

func (p *Provider) mapRefundReason(reason gopay.RefundReason) string {
	switch reason {
	case gopay.RefundReasonDuplicate:
		return "duplicate"
	case gopay.RefundReasonFraudulent:
		return "fraudulent"
	case gopay.RefundReasonRequestedByCustomer:
		return "requested_by_customer"
	default:
		return ""
	}
}

func (p *Provider) reverseMapRefundReason(reason string) gopay.RefundReason {
	switch reason {
	case "duplicate":
		return gopay.RefundReasonDuplicate
	case "fraudulent":
		return gopay.RefundReasonFraudulent
	case "requested_by_customer":
		return gopay.RefundReasonRequestedByCustomer
	default:
		return gopay.RefundReasonOther
	}
}

func (p *Provider) mapCustomer(c *stripe.Customer) *gopay.Customer {
	cust := &gopay.Customer{
		ID:          c.ID,
		Email:       c.Email,
		Name:        c.Name,
		Phone:       c.Phone,
		Description: c.Description,
		Metadata:    c.Metadata,
		CreatedAt:   time.Unix(c.Created, 0),
		Provider:    p.Name(),
		Raw:         map[string]any{"id": c.ID},
	}

	if c.InvoiceSettings != nil && c.InvoiceSettings.DefaultPaymentMethod != nil {
		cust.DefaultPaymentMethodID = c.InvoiceSettings.DefaultPaymentMethod.ID
	}

	return cust
}

func (p *Provider) mapPaymentMethod(pm *stripe.PaymentMethod) *gopay.PaymentMethod {
	method := &gopay.PaymentMethod{
		ID:        pm.ID,
		Type:      mapPaymentMethodType(pm.Type),
		CreatedAt: time.Unix(pm.Created, 0),
		Provider:  p.Name(),
		Raw: map[string]any{
			"id":   pm.ID,
			"type": string(pm.Type),
		},
	}

	if pm.Customer != nil {
		method.CustomerID = pm.Customer.ID
	}
	if pm.Card != nil {
		method.Card = &gopay.CardDetails{
			Brand:    string(pm.Card.Brand),
			Last4:    pm.Card.Last4,
			ExpMonth: int(pm.Card.ExpMonth),
			ExpYear:  int(pm.Card.ExpYear),
			Funding:  string(pm.Card.Funding),
			Country:  pm.Card.Country,
		}
	}
	if pm.BillingDetails != nil {
		method.BillingDetails = &gopay.BillingDetails{
			Name:  pm.BillingDetails.Name,
			Email: pm.BillingDetails.Email,
			Phone: pm.BillingDetails.Phone,
		}
		if pm.BillingDetails.Address != nil {
			method.BillingDetails.Address = &gopay.Address{
				Line1:      pm.BillingDetails.Address.Line1,
				Line2:      pm.BillingDetails.Address.Line2,
				City:       pm.BillingDetails.Address.City,
				State:      pm.BillingDetails.Address.State,
				PostalCode: pm.BillingDetails.Address.PostalCode,
				Country:    pm.BillingDetails.Address.Country,
			}
		}
	}

	return method
}

func mapPaymentMethodType(t stripe.PaymentMethodType) gopay.PaymentMethodType {
	switch t {
	case stripe.PaymentMethodTypeCard:
		return gopay.PaymentMethodCard
	case stripe.PaymentMethodTypeUSBankAccount:
		return gopay.PaymentMethodBankAccount
	default:
		return gopay.PaymentMethodType(t)
	}
}

func (p *Provider) mapError(err error) error {
	var stripeErr *stripe.Error
	if errors.As(err, &stripeErr) {
		switch stripeErr.Code {
		case stripe.ErrorCodeCardDeclined:
			return fmt.Errorf("%w: %s", gopay.ErrCardDeclined, stripeErr.Msg)
		case stripe.ErrorCodeExpiredCard:
			return fmt.Errorf("%w: %s", gopay.ErrExpiredCard, stripeErr.Msg)
		case stripe.ErrorCodeInsufficientFunds:
			return fmt.Errorf("%w: %s", gopay.ErrInsufficientFunds, stripeErr.Msg)
		case stripe.ErrorCodeIncorrectNumber:
			return fmt.Errorf("%w: %s", gopay.ErrInvalidCard, stripeErr.Msg)
		case stripe.ErrorCodeResourceMissing:
			return fmt.Errorf("%w: %s", gopay.ErrNotFound, stripeErr.Msg)
		case stripe.ErrorCodeChargeAlreadyRefunded:
			return fmt.Errorf("%w: %s", gopay.ErrAlreadyRefunded, stripeErr.Msg)
		case stripe.ErrorCodeChargeAlreadyCaptured:
			return fmt.Errorf("%w: %s", gopay.ErrAlreadyCaptured, stripeErr.Msg)
		default:
			return fmt.Errorf("%w: %s", gopay.ErrProviderError, stripeErr.Msg)
		}
	}
	return err
}
