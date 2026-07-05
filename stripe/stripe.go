// Package stripe provides a Stripe payment provider for gopay.
package stripe

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"strings"
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
		// Copy the caller's client before applying a default timeout so we never
		// mutate a *http.Client the caller still owns, and so a custom client
		// with no timeout can't block requests indefinitely.
		if config.HTTPClient.Timeout == 0 {
			clientCopy := *config.HTTPClient
			clientCopy.Timeout = 30 * time.Second
			config.HTTPClient = &clientCopy
		}
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
	if req == nil {
		return nil, fmt.Errorf("gopay: nil payment request")
	}
	if req.Amount == nil {
		return nil, fmt.Errorf("gopay: nil amount")
	}

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
	if req == nil {
		return nil, fmt.Errorf("gopay: nil refund request")
	}

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
		maps.Copy(meta, req.Metadata)
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
		maps.Copy(meta, req.Metadata)
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
		maps.Copy(meta, req.Metadata)
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

// CreateSetupIntent creates a setup intent to store a payment method for future
// off-session charges without moving money. When the request carries a
// PaymentMethodID, the intent is confirmed immediately.
func (p *Provider) CreateSetupIntent(ctx context.Context, req *gopay.SetupIntentRequest) (*gopay.SetupIntent, error) {
	if req == nil {
		return nil, fmt.Errorf("gopay: nil setup intent request")
	}

	params := &stripe.SetupIntentParams{}

	usage := req.Usage
	if usage == "" {
		usage = gopay.SetupIntentUsageOffSession
	}
	params.Usage = stripe.String(string(usage))

	if req.CustomerID != "" {
		params.Customer = stripe.String(req.CustomerID)
	}
	if req.PaymentMethodID != "" {
		params.PaymentMethod = stripe.String(req.PaymentMethodID)
		params.Confirm = stripe.Bool(true)
		// Stripe only accepts return_url when the intent is confirmed
		// (confirm=true); otherwise the API rejects the request. In the
		// frontend-confirmation flow the return URL is supplied at confirm time.
		if req.ReturnURL != "" {
			params.ReturnURL = stripe.String(req.ReturnURL)
		}
	}
	if req.Description != "" {
		params.Description = stripe.String(req.Description)
	}
	if len(req.Metadata) > 0 {
		meta := make(map[string]string, len(req.Metadata))
		maps.Copy(meta, req.Metadata)
		params.Metadata = meta
	}
	if req.IdempotencyKey != "" {
		params.SetIdempotencyKey(req.IdempotencyKey)
	}

	params.Context = ctx
	si, err := p.api.SetupIntents.New(params)
	if err != nil {
		return nil, p.mapError(err)
	}

	return p.mapSetupIntent(si), nil
}

// GetSetupIntent retrieves a setup intent.
func (p *Provider) GetSetupIntent(ctx context.Context, setupIntentID string) (*gopay.SetupIntent, error) {
	params := &stripe.SetupIntentParams{}
	params.Context = ctx
	si, err := p.api.SetupIntents.Get(setupIntentID, params)
	if err != nil {
		return nil, p.mapError(err)
	}

	return p.mapSetupIntent(si), nil
}

// CancelSetupIntent cancels a setup intent.
func (p *Provider) CancelSetupIntent(ctx context.Context, setupIntentID string) (*gopay.SetupIntent, error) {
	params := &stripe.SetupIntentCancelParams{}
	params.Context = ctx
	si, err := p.api.SetupIntents.Cancel(setupIntentID, params)
	if err != nil {
		return nil, p.mapError(err)
	}

	return p.mapSetupIntent(si), nil
}

// CreatePlan creates a recurring plan. In Stripe this is modeled as a recurring
// Price with an inline Product; the returned plan ID is the Price ID.
func (p *Provider) CreatePlan(ctx context.Context, req *gopay.PlanRequest) (*gopay.Plan, error) {
	if req == nil {
		return nil, fmt.Errorf("gopay: nil plan request")
	}
	if req.Amount == nil {
		return nil, fmt.Errorf("gopay: nil amount")
	}

	count := req.IntervalCount
	if count <= 0 {
		count = 1
	}
	name := req.Name
	if name == "" {
		name = "gopay plan"
	}

	params := &stripe.PriceParams{
		Currency:   stripe.String(req.Amount.Currency),
		UnitAmount: stripe.Int64(req.Amount.Value),
		Nickname:   stripe.String(name),
		Recurring: &stripe.PriceRecurringParams{
			Interval:      stripe.String(string(req.Interval)),
			IntervalCount: stripe.Int64(int64(count)),
		},
		ProductData: &stripe.PriceProductDataParams{
			Name: stripe.String(name),
		},
	}
	if len(req.Metadata) > 0 {
		meta := make(map[string]string, len(req.Metadata))
		maps.Copy(meta, req.Metadata)
		params.Metadata = meta
	}
	if req.IdempotencyKey != "" {
		params.SetIdempotencyKey(req.IdempotencyKey)
	}

	params.Context = ctx
	price, err := p.api.Prices.New(params)
	if err != nil {
		return nil, p.mapError(err)
	}

	return p.mapPlan(price), nil
}

// GetPlan retrieves a plan (Stripe Price) by ID.
func (p *Provider) GetPlan(ctx context.Context, planID string) (*gopay.Plan, error) {
	params := &stripe.PriceParams{}
	// Expand the product so mapPlan can fall back to the product name when the
	// price has no nickname.
	params.AddExpand("product")
	params.Context = ctx
	price, err := p.api.Prices.Get(planID, params)
	if err != nil {
		return nil, p.mapError(err)
	}

	return p.mapPlan(price), nil
}

// CreateSubscription subscribes a customer to a plan.
func (p *Provider) CreateSubscription(ctx context.Context, req *gopay.SubscriptionRequest) (*gopay.Subscription, error) {
	if req == nil {
		return nil, fmt.Errorf("gopay: nil subscription request")
	}
	// Stripe subscriptions charge a stored payment method on an existing
	// customer, so CustomerID is required here (core validation leaves this to
	// the provider, since mandate-redirect providers like Razorpay don't need it).
	if req.CustomerID == "" {
		return nil, errors.New("gopay: customer ID required for Stripe subscription")
	}

	params := &stripe.SubscriptionParams{
		Customer: stripe.String(req.CustomerID),
		Items: []*stripe.SubscriptionItemsParams{
			{Price: stripe.String(req.PlanID)},
		},
	}
	if req.PaymentMethodID != "" {
		params.DefaultPaymentMethod = stripe.String(req.PaymentMethodID)
	}
	if req.TrialDays > 0 {
		params.TrialPeriodDays = stripe.Int64(int64(req.TrialDays))
	}
	if len(req.Metadata) > 0 {
		meta := make(map[string]string, len(req.Metadata))
		maps.Copy(meta, req.Metadata)
		params.Metadata = meta
	}
	if req.IdempotencyKey != "" {
		params.SetIdempotencyKey(req.IdempotencyKey)
	}

	params.Context = ctx
	sub, err := p.api.Subscriptions.New(params)
	if err != nil {
		return nil, p.mapError(err)
	}

	return p.mapSubscription(sub), nil
}

// GetSubscription retrieves a subscription by ID.
func (p *Provider) GetSubscription(ctx context.Context, subscriptionID string) (*gopay.Subscription, error) {
	params := &stripe.SubscriptionParams{}
	params.Context = ctx
	sub, err := p.api.Subscriptions.Get(subscriptionID, params)
	if err != nil {
		return nil, p.mapError(err)
	}

	return p.mapSubscription(sub), nil
}

// GetInvoice retrieves an invoice by ID.
func (p *Provider) GetInvoice(ctx context.Context, invoiceID string) (*gopay.Invoice, error) {
	params := &stripe.InvoiceParams{}
	params.Context = ctx
	inv, err := p.api.Invoices.Get(invoiceID, params)
	if err != nil {
		return nil, p.mapError(err)
	}

	return p.mapInvoice(inv), nil
}

// mapInvoice converts a Stripe invoice to a gopay Invoice.
func (p *Provider) mapInvoice(inv *stripe.Invoice) *gopay.Invoice {
	out := &gopay.Invoice{
		ID:               inv.ID,
		Status:           mapInvoiceStatus(inv.Status),
		Number:           inv.Number,
		HostedInvoiceURL: inv.HostedInvoiceURL,
		PDFURL:           inv.InvoicePDF,
		Metadata:         inv.Metadata,
		CreatedAt:        time.Unix(inv.Created, 0),
		Provider:         p.Name(),
		Raw: map[string]any{
			"id":     inv.ID,
			"status": string(inv.Status),
		},
	}

	if inv.Currency != "" {
		out.Amount = gopay.NewAmount(inv.AmountDue, string(inv.Currency))
		out.AmountPaid = gopay.NewAmount(inv.AmountPaid, string(inv.Currency))
	}
	if inv.Customer != nil {
		out.CustomerID = inv.Customer.ID
	}
	if inv.Subscription != nil {
		out.SubscriptionID = inv.Subscription.ID
	}
	if inv.DueDate > 0 {
		out.DueDate = time.Unix(inv.DueDate, 0)
	}
	if inv.StatusTransitions != nil && inv.StatusTransitions.PaidAt > 0 {
		out.PaidAt = time.Unix(inv.StatusTransitions.PaidAt, 0)
	}

	return out
}

// mapInvoiceStatus maps a Stripe invoice status to a normalized invoice status.
func mapInvoiceStatus(s stripe.InvoiceStatus) gopay.InvoiceStatus {
	switch s {
	case stripe.InvoiceStatusDraft:
		return gopay.InvoiceStatusDraft
	case stripe.InvoiceStatusOpen:
		return gopay.InvoiceStatusOpen
	case stripe.InvoiceStatusPaid:
		return gopay.InvoiceStatusPaid
	case stripe.InvoiceStatusVoid:
		return gopay.InvoiceStatusVoid
	case stripe.InvoiceStatusUncollectible:
		return gopay.InvoiceStatusUncollectible
	default:
		return gopay.InvoiceStatus(s)
	}
}

// CancelSubscription cancels a subscription. When opts.AtPeriodEnd is set the
// subscription is updated to cancel at period end; otherwise it is canceled
// immediately.
func (p *Provider) CancelSubscription(ctx context.Context, subscriptionID string, opts *gopay.CancelOptions) (*gopay.Subscription, error) {
	if opts != nil && opts.AtPeriodEnd {
		params := &stripe.SubscriptionParams{
			CancelAtPeriodEnd: stripe.Bool(true),
		}
		params.Context = ctx
		sub, err := p.api.Subscriptions.Update(subscriptionID, params)
		if err != nil {
			return nil, p.mapError(err)
		}
		return p.mapSubscription(sub), nil
	}

	params := &stripe.SubscriptionCancelParams{}
	params.Context = ctx
	sub, err := p.api.Subscriptions.Cancel(subscriptionID, params)
	if err != nil {
		return nil, p.mapError(err)
	}

	return p.mapSubscription(sub), nil
}

// ListPayments lists payment intents with cursor-based pagination.
// The cursor is a Stripe object ID used as starting_after.
func (p *Provider) ListPayments(ctx context.Context, params *gopay.ListParams) (*gopay.List[*gopay.Payment], error) {
	listParams := &stripe.PaymentIntentListParams{}
	applyStripeListParams(&listParams.ListParams, ctx, params)

	iter := p.api.PaymentIntents.List(listParams)
	var items []*gopay.Payment
	for iter.Next() {
		items = append(items, p.mapPaymentIntent(iter.PaymentIntent()))
	}
	if err := iter.Err(); err != nil {
		return nil, p.mapError(err)
	}
	return buildStripeList(items, iter.Meta(), func(x *gopay.Payment) string { return x.ID }), nil
}

// ListRefunds lists refunds with cursor-based pagination.
// The cursor is a Stripe object ID used as starting_after.
func (p *Provider) ListRefunds(ctx context.Context, params *gopay.ListParams) (*gopay.List[*gopay.Refund], error) {
	listParams := &stripe.RefundListParams{}
	applyStripeListParams(&listParams.ListParams, ctx, params)

	iter := p.api.Refunds.List(listParams)
	var items []*gopay.Refund
	for iter.Next() {
		items = append(items, p.mapRefund(iter.Refund()))
	}
	if err := iter.Err(); err != nil {
		return nil, p.mapError(err)
	}
	return buildStripeList(items, iter.Meta(), func(x *gopay.Refund) string { return x.ID }), nil
}

// ListCustomers lists customers with cursor-based pagination.
// The cursor is a Stripe object ID used as starting_after.
func (p *Provider) ListCustomers(ctx context.Context, params *gopay.ListParams) (*gopay.List[*gopay.Customer], error) {
	listParams := &stripe.CustomerListParams{}
	applyStripeListParams(&listParams.ListParams, ctx, params)

	iter := p.api.Customers.List(listParams)
	var items []*gopay.Customer
	for iter.Next() {
		items = append(items, p.mapCustomer(iter.Customer()))
	}
	if err := iter.Err(); err != nil {
		return nil, p.mapError(err)
	}
	return buildStripeList(items, iter.Meta(), func(x *gopay.Customer) string { return x.ID }), nil
}

// applyStripeListParams configures a Stripe list request for a single page from
// the gopay pagination params. Single disables the SDK's auto-pagination so we
// return one page at a time.
func applyStripeListParams(lp *stripe.ListParams, ctx context.Context, params *gopay.ListParams) {
	lp.Context = ctx
	lp.Single = true
	lp.Limit = stripe.Int64(int64(params.EffectiveLimit()))
	if params != nil && params.Cursor != "" {
		lp.StartingAfter = stripe.String(params.Cursor)
	}
}

// buildStripeList assembles a gopay.List from the fetched items and Stripe's
// list metadata, deriving the next cursor from the last item's ID.
func buildStripeList[T any](items []T, meta *stripe.ListMeta, id func(T) string) *gopay.List[T] {
	list := &gopay.List[T]{Items: items}
	if meta != nil {
		list.HasMore = meta.HasMore
	}
	if list.HasMore && len(items) > 0 {
		list.NextCursor = id(items[len(items)-1])
	}
	return list
}

// VerifyWebhook verifies and parses a Stripe webhook.
// Headers should contain "Stripe-Signature".
func (p *Provider) VerifyWebhook(_ context.Context, payload []byte, headers map[string]string) (*gopay.WebhookEvent, error) {
	var signature string
	for k, v := range headers {
		if strings.EqualFold(k, "Stripe-Signature") {
			signature = v
			break
		}
	}
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
	return buildWebhookEvent(event), nil
}

// ParseWebhook parses a Stripe webhook event.
func ParseWebhook(payload []byte, signature, webhookSecret string) (*gopay.WebhookEvent, error) {
	event, err := webhook.ConstructEvent(payload, signature, webhookSecret)
	if err != nil {
		return nil, fmt.Errorf("ParseWebhook: %w", err)
	}
	return buildWebhookEvent(event), nil
}

// buildWebhookEvent maps a verified Stripe event into a normalized
// gopay.WebhookEvent, extracting the payment/refund identifiers and amount from
// the event's data object so callers don't have to parse Raw themselves.
func buildWebhookEvent(event stripe.Event) *gopay.WebhookEvent {
	ev := &gopay.WebhookEvent{
		ID:       event.ID,
		Type:     string(event.Type),
		Provider: "stripe",
		Raw:      event.Data.Raw,
		Kind:     mapWebhookKind(string(event.Type)),
	}

	var obj stripeWebhookObject
	if err := json.Unmarshal(event.Data.Raw, &obj); err != nil {
		return ev
	}
	obj.normalizeInto(ev)
	return ev
}

// stripeWebhookObject is the subset of a Stripe event's data object that we pull
// normalized identifiers and amounts from across the event types we map.
type stripeWebhookObject struct {
	Object         string `json:"object"`
	ID             string `json:"id"`
	Status         string `json:"status"`
	Amount         int64  `json:"amount"`
	AmountRefunded int64  `json:"amount_refunded"`
	Currency       string `json:"currency"`
	PaymentIntent  string `json:"payment_intent"`
	Subscription   string `json:"subscription"`
	AmountPaid     int64  `json:"amount_paid"`
	Refunds        struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	} `json:"refunds"`
}

// normalizeInto populates the normalized identifier/amount fields of ev based on
// the object's type, keeping the per-type extraction out of buildWebhookEvent.
func (obj stripeWebhookObject) normalizeInto(ev *gopay.WebhookEvent) {
	switch obj.Object {
	case "setup_intent":
		ev.SetupIntentID = obj.ID
		// setup_intent events carry no monetary amount.
	case "subscription":
		ev.SubscriptionID = obj.ID
		// subscription events carry no single monetary amount.
	case "invoice":
		ev.InvoiceID = obj.ID
		ev.SubscriptionID = obj.Subscription
		// Report the amount actually paid for the recurring charge.
		if obj.Currency != "" {
			ev.Amount = gopay.NewAmount(obj.AmountPaid, obj.Currency)
		}
	case "refund":
		ev.RefundID = obj.ID
		ev.PaymentID = obj.PaymentIntent
		if obj.Currency != "" {
			ev.Amount = gopay.NewAmount(obj.Amount, obj.Currency)
		}
		// refund.created/refund.updated only become a normalized success or
		// failure once the refund object reports a terminal status; until then
		// the event stays WebhookUnknown so callers don't act on it prematurely.
		switch obj.Status {
		case "succeeded":
			ev.Kind = gopay.WebhookRefundSucceeded
		case "failed", "canceled":
			ev.Kind = gopay.WebhookRefundFailed
		}
	case "charge":
		ev.PaymentID = obj.PaymentIntent
		if len(obj.Refunds.Data) > 0 {
			ev.RefundID = obj.Refunds.Data[0].ID
		}
		// A charge.refunded event reports the refunded amount; other charge
		// events report the charge amount.
		amount := obj.Amount
		if ev.Kind == gopay.WebhookRefundSucceeded || ev.Kind == gopay.WebhookRefundFailed {
			amount = obj.AmountRefunded
		}
		if obj.Currency != "" {
			ev.Amount = gopay.NewAmount(amount, obj.Currency)
		}
	default: // payment_intent and anything else with an id/amount
		ev.PaymentID = obj.ID
		if obj.Currency != "" {
			ev.Amount = gopay.NewAmount(obj.Amount, obj.Currency)
		}
	}
}

// mapWebhookKind maps a Stripe event type to a normalized webhook event kind.
func mapWebhookKind(eventType string) gopay.WebhookEventKind {
	switch eventType {
	case "payment_intent.created":
		return gopay.WebhookPaymentCreated
	case "payment_intent.succeeded":
		return gopay.WebhookPaymentSucceeded
	case "payment_intent.payment_failed":
		return gopay.WebhookPaymentFailed
	case "payment_intent.canceled":
		return gopay.WebhookPaymentCanceled
	case "charge.refunded":
		return gopay.WebhookRefundSucceeded
	case "refund.failed":
		return gopay.WebhookRefundFailed
	case "setup_intent.succeeded":
		return gopay.WebhookSetupSucceeded
	case "setup_intent.setup_failed", "setup_intent.canceled":
		return gopay.WebhookSetupFailed
	case "customer.subscription.created":
		return gopay.WebhookSubscriptionCreated
	case "customer.subscription.updated":
		return gopay.WebhookSubscriptionUpdated
	case "customer.subscription.deleted":
		return gopay.WebhookSubscriptionCanceled
	case "invoice.payment_succeeded":
		return gopay.WebhookInvoicePaymentSucceeded
	case "invoice.payment_failed":
		return gopay.WebhookInvoicePaymentFailed
	// refund.created / refund.updated are classified from the refund object's
	// status in buildWebhookEvent rather than assumed successful here.
	default:
		return gopay.WebhookUnknown
	}
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

func (p *Provider) mapSetupIntent(si *stripe.SetupIntent) *gopay.SetupIntent {
	out := &gopay.SetupIntent{
		ID:           si.ID,
		Status:       mapSetupIntentStatus(si.Status),
		Usage:        gopay.SetupIntentUsage(si.Usage),
		Description:  si.Description,
		ClientSecret: si.ClientSecret,
		Metadata:     si.Metadata,
		CreatedAt:    time.Unix(si.Created, 0),
		Provider:     p.Name(),
		Raw: map[string]any{
			"id":     si.ID,
			"status": string(si.Status),
		},
	}

	if si.Customer != nil {
		out.CustomerID = si.Customer.ID
	}
	if si.PaymentMethod != nil {
		out.PaymentMethodID = si.PaymentMethod.ID
	}
	if si.LastSetupError != nil {
		out.FailureCode = string(si.LastSetupError.Code)
		out.FailureMessage = si.LastSetupError.Msg
	}
	if si.NextAction != nil && si.NextAction.RedirectToURL != nil {
		out.RedirectURL = si.NextAction.RedirectToURL.URL
	}

	return out
}

func mapSetupIntentStatus(status stripe.SetupIntentStatus) gopay.SetupIntentStatus {
	switch status {
	case stripe.SetupIntentStatusRequiresPaymentMethod:
		return gopay.SetupIntentStatusRequiresPaymentMethod
	case stripe.SetupIntentStatusRequiresConfirmation:
		return gopay.SetupIntentStatusRequiresConfirmation
	case stripe.SetupIntentStatusRequiresAction:
		return gopay.SetupIntentStatusRequiresAction
	case stripe.SetupIntentStatusProcessing:
		return gopay.SetupIntentStatusProcessing
	case stripe.SetupIntentStatusSucceeded:
		return gopay.SetupIntentStatusSucceeded
	case stripe.SetupIntentStatusCanceled:
		return gopay.SetupIntentStatusCanceled
	default:
		return gopay.SetupIntentStatusRequiresPaymentMethod
	}
}

func (p *Provider) mapPlan(price *stripe.Price) *gopay.Plan {
	plan := &gopay.Plan{
		ID:        price.ID,
		Name:      price.Nickname,
		Amount:    gopay.NewAmount(price.UnitAmount, string(price.Currency)),
		Metadata:  price.Metadata,
		CreatedAt: time.Unix(price.Created, 0),
		Provider:  p.Name(),
		Raw: map[string]any{
			"id": price.ID,
		},
	}

	if price.Recurring != nil {
		plan.Interval = mapBillingInterval(price.Recurring.Interval)
		plan.IntervalCount = int(price.Recurring.IntervalCount)
	}
	// When the price has no nickname, fall back to the (expanded) product name.
	if plan.Name == "" && price.Product != nil {
		plan.Name = price.Product.Name
	}

	return plan
}

func mapBillingInterval(interval stripe.PriceRecurringInterval) gopay.BillingInterval {
	switch interval {
	case stripe.PriceRecurringIntervalDay:
		return gopay.BillingIntervalDay
	case stripe.PriceRecurringIntervalWeek:
		return gopay.BillingIntervalWeek
	case stripe.PriceRecurringIntervalMonth:
		return gopay.BillingIntervalMonth
	case stripe.PriceRecurringIntervalYear:
		return gopay.BillingIntervalYear
	default:
		return gopay.BillingInterval(interval)
	}
}

func (p *Provider) mapSubscription(sub *stripe.Subscription) *gopay.Subscription {
	out := &gopay.Subscription{
		ID:                 sub.ID,
		Status:             mapSubscriptionStatus(sub.Status),
		CancelAtPeriodEnd:  sub.CancelAtPeriodEnd,
		CurrentPeriodStart: time.Unix(sub.CurrentPeriodStart, 0),
		CurrentPeriodEnd:   time.Unix(sub.CurrentPeriodEnd, 0),
		Metadata:           sub.Metadata,
		CreatedAt:          time.Unix(sub.Created, 0),
		Provider:           p.Name(),
		Raw: map[string]any{
			"id":     sub.ID,
			"status": string(sub.Status),
		},
	}

	if sub.Customer != nil {
		out.CustomerID = sub.Customer.ID
	}
	if sub.DefaultPaymentMethod != nil {
		out.PaymentMethodID = sub.DefaultPaymentMethod.ID
	}
	if sub.CanceledAt > 0 {
		out.CanceledAt = time.Unix(sub.CanceledAt, 0)
	}
	if sub.Items != nil && len(sub.Items.Data) > 0 && sub.Items.Data[0].Price != nil {
		out.PlanID = sub.Items.Data[0].Price.ID
	}

	return out
}

func mapSubscriptionStatus(status stripe.SubscriptionStatus) gopay.SubscriptionStatus {
	switch status {
	case stripe.SubscriptionStatusActive:
		return gopay.SubscriptionStatusActive
	case stripe.SubscriptionStatusTrialing:
		return gopay.SubscriptionStatusTrialing
	case stripe.SubscriptionStatusPastDue:
		return gopay.SubscriptionStatusPastDue
	case stripe.SubscriptionStatusCanceled:
		return gopay.SubscriptionStatusCanceled
	case stripe.SubscriptionStatusIncomplete:
		return gopay.SubscriptionStatusIncomplete
	case stripe.SubscriptionStatusIncompleteExpired:
		return gopay.SubscriptionStatusIncompleteExpired
	case stripe.SubscriptionStatusUnpaid:
		return gopay.SubscriptionStatusUnpaid
	default:
		return gopay.SubscriptionStatus(status)
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
