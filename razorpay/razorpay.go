// Package razorpay provides a Razorpay payment provider for gopay.
package razorpay

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/KARTIKrocks/gopay"
)

const (
	baseURL = "https://api.razorpay.com/v1"

	// maxResponseSize is the maximum response body size (10 MB).
	maxResponseSize = 10 << 20
)

// notes is Razorpay's key/value metadata bag as it comes back on responses.
//
// It needs its own unmarshaller because Razorpay does not serialise an empty
// bag as an empty object: it sends `"notes": []` — a JSON *array* — which a
// plain map[string]string rejects with "cannot unmarshal array into Go value of
// type map[string]string", failing the whole response. Every entity is affected
// (order, payment, customer, plan, subscription, invoice), so any object created
// without notes was unreadable.
//
// An empty array and null both decode to a nil map; an object decodes normally.
type notes map[string]string

func (n *notes) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		*n = nil
		return nil
	}
	// Razorpay's empty-notes form. Only an *empty* array is tolerated: a
	// populated array would mean the shape changed in a way we should not guess
	// at, and silently dropping it would hide real data.
	if trimmed[0] == '[' {
		var arr []json.RawMessage
		if err := json.Unmarshal(trimmed, &arr); err != nil {
			return err
		}
		if len(arr) > 0 {
			return fmt.Errorf("razorpay: unexpected non-empty array for notes: %s", trimmed)
		}
		*n = nil
		return nil
	}
	var m map[string]string
	if err := json.Unmarshal(trimmed, &m); err != nil {
		return err
	}
	*n = m
	return nil
}

// Config holds Razorpay-specific configuration.
type Config struct {
	// KeyID is the Razorpay key ID.
	KeyID string

	// KeySecret is the Razorpay key secret.
	KeySecret string

	// WebhookSecret is the webhook secret for signature verification.
	WebhookSecret string

	// HTTPClient is a custom HTTP client (optional).
	HTTPClient *http.Client

	// BaseURL overrides the API base URL (for testing).
	BaseURL string
}

// DefaultConfig returns a default Razorpay configuration.
func DefaultConfig() Config {
	return Config{
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
		BaseURL:    baseURL,
	}
}

// WithCredentials sets the credentials.
func (c Config) WithCredentials(keyID, keySecret string) Config {
	c.KeyID = keyID
	c.KeySecret = keySecret
	return c
}

// WithWebhookSecret sets the webhook secret.
func (c Config) WithWebhookSecret(secret string) Config {
	c.WebhookSecret = secret
	return c
}

// WithHTTPClient sets a custom HTTP client. The client is copied before use so
// the caller retains sole ownership of theirs, and a default 30s timeout is
// applied when none is set.
func (c Config) WithHTTPClient(client *http.Client) Config {
	if client != nil {
		clientCopy := *client
		if clientCopy.Timeout == 0 {
			clientCopy.Timeout = 30 * time.Second
		}
		c.HTTPClient = &clientCopy
	} else {
		c.HTTPClient = client
	}
	return c
}

// Provider implements gopay.Provider, gopay.CustomerProvider, gopay.ListProvider,
// gopay.WebhookProvider, and gopay.SubscriptionProvider for Razorpay.
type Provider struct {
	config Config
}

var (
	_ gopay.Provider             = (*Provider)(nil)
	_ gopay.CustomerProvider     = (*Provider)(nil)
	_ gopay.ListProvider         = (*Provider)(nil)
	_ gopay.WebhookProvider      = (*Provider)(nil)
	_ gopay.SubscriptionProvider = (*Provider)(nil)
)

// NewProvider creates a new Razorpay provider.
func NewProvider(config Config) (*Provider, error) {
	if config.KeyID == "" || config.KeySecret == "" {
		return nil, fmt.Errorf("%w: key ID and secret required", gopay.ErrInvalidConfig)
	}

	if config.HTTPClient == nil {
		config.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	} else if config.HTTPClient.Timeout == 0 {
		// Copy the caller's client before setting a default timeout so we never
		// mutate a *http.Client the caller still owns.
		clientCopy := *config.HTTPClient
		clientCopy.Timeout = 30 * time.Second
		config.HTTPClient = &clientCopy
	}

	if config.BaseURL == "" {
		config.BaseURL = baseURL
	}

	return &Provider{config: config}, nil
}

// Name returns the provider name.
func (p *Provider) Name() string {
	return "razorpay"
}

// CreatePayment creates an order in Razorpay.
func (p *Provider) CreatePayment(ctx context.Context, req *gopay.PaymentRequest) (*gopay.Payment, error) {
	if req == nil {
		return nil, fmt.Errorf("gopay: nil payment request")
	}
	if req.Amount == nil {
		return nil, fmt.Errorf("gopay: nil payment amount")
	}

	orderReq := orderRequest{
		Amount:   req.Amount.Value,
		Currency: req.Amount.Currency,
		Notes:    req.Metadata,
	}

	if req.IdempotencyKey != "" {
		orderReq.Receipt = req.IdempotencyKey
	}

	body, err := json.Marshal(orderReq)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.config.BaseURL+"/orders", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	httpReq.SetBasicAuth(p.config.KeyID, p.config.KeySecret)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.config.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", gopay.ErrPaymentFailed, err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		return nil, p.parseError(respBody)
	}

	var o order
	if err := json.Unmarshal(respBody, &o); err != nil {
		return nil, err
	}

	return p.mapOrder(&o), nil
}

// GetPayment retrieves an order or payment.
func (p *Provider) GetPayment(ctx context.Context, paymentID string) (*gopay.Payment, error) {
	httpReq, err := http.NewRequestWithContext(ctx, "GET", p.config.BaseURL+"/orders/"+paymentID, nil)
	if err != nil {
		return nil, err
	}

	httpReq.SetBasicAuth(p.config.KeyID, p.config.KeySecret)

	resp, err := p.config.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusNotFound {
		return p.getPaymentByID(ctx, paymentID)
	}
	if resp.StatusCode >= 400 {
		return nil, p.parseError(respBody)
	}

	var o order
	if err := json.Unmarshal(respBody, &o); err != nil {
		return nil, err
	}

	return p.mapOrder(&o), nil
}

func (p *Provider) getPaymentByID(ctx context.Context, paymentID string) (*gopay.Payment, error) {
	httpReq, err := http.NewRequestWithContext(ctx, "GET", p.config.BaseURL+"/payments/"+paymentID, nil)
	if err != nil {
		return nil, err
	}

	httpReq.SetBasicAuth(p.config.KeyID, p.config.KeySecret)

	resp, err := p.config.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		return nil, p.parseError(respBody)
	}

	var pay razorpayPayment
	if err := json.Unmarshal(respBody, &pay); err != nil {
		return nil, err
	}

	return p.mapPayment(&pay), nil
}

// CapturePayment captures an authorized payment.
func (p *Provider) CapturePayment(ctx context.Context, paymentID string, amt *gopay.Amount) (*gopay.Payment, error) {
	captureReq := map[string]any{}

	if amt != nil {
		captureReq["amount"] = amt.Value
		captureReq["currency"] = amt.Currency
	} else {
		// Razorpay requires amount and currency; fetch the payment to get them.
		existing, err := p.getPaymentByID(ctx, paymentID)
		if err != nil {
			return nil, err
		}
		captureReq["amount"] = existing.Amount.Value
		captureReq["currency"] = existing.Amount.Currency
	}

	body, err := json.Marshal(captureReq)
	if err != nil {
		return nil, fmt.Errorf("marshal capture request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.config.BaseURL+"/payments/"+paymentID+"/capture", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	httpReq.SetBasicAuth(p.config.KeyID, p.config.KeySecret)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.config.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		return nil, p.parseError(respBody)
	}

	var pay razorpayPayment
	if err := json.Unmarshal(respBody, &pay); err != nil {
		return nil, err
	}

	return p.mapPayment(&pay), nil
}

// CancelPayment is not directly supported by Razorpay.
func (p *Provider) CancelPayment(ctx context.Context, paymentID string) (*gopay.Payment, error) {
	return nil, fmt.Errorf("%w: use refund instead", gopay.ErrUnsupported)
}

// Refund creates a refund.
func (p *Provider) Refund(ctx context.Context, req *gopay.RefundRequest) (*gopay.Refund, error) {
	if req == nil {
		return nil, fmt.Errorf("gopay: nil refund request")
	}

	refundReq := refundRequest{
		Notes: req.Metadata,
	}

	if req.Amount != nil {
		refundReq.Amount = req.Amount.Value
	}

	body, err := json.Marshal(refundReq)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.config.BaseURL+"/payments/"+req.PaymentID+"/refund", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	httpReq.SetBasicAuth(p.config.KeyID, p.config.KeySecret)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.config.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", gopay.ErrRefundFailed, err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		return nil, p.parseError(respBody)
	}

	var refundResp razorpayRefund
	if err := json.Unmarshal(respBody, &refundResp); err != nil {
		return nil, err
	}

	return p.mapRefund(&refundResp), nil
}

// GetRefund retrieves a refund.
func (p *Provider) GetRefund(ctx context.Context, refundID string) (*gopay.Refund, error) {
	httpReq, err := http.NewRequestWithContext(ctx, "GET", p.config.BaseURL+"/refunds/"+refundID, nil)
	if err != nil {
		return nil, err
	}

	httpReq.SetBasicAuth(p.config.KeyID, p.config.KeySecret)

	resp, err := p.config.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		return nil, p.parseError(respBody)
	}

	var refundResp razorpayRefund
	if err := json.Unmarshal(respBody, &refundResp); err != nil {
		return nil, err
	}

	return p.mapRefund(&refundResp), nil
}

// CreateCustomer creates a customer.
func (p *Provider) CreateCustomer(ctx context.Context, req *gopay.CustomerRequest) (*gopay.Customer, error) {
	if req == nil {
		return nil, fmt.Errorf("gopay: nil customer request")
	}

	custReq := customerRequest{
		Name:    req.Name,
		Email:   req.Email,
		Contact: req.Phone,
		Notes:   req.Metadata,
	}

	body, err := json.Marshal(custReq)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.config.BaseURL+"/customers", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	httpReq.SetBasicAuth(p.config.KeyID, p.config.KeySecret)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.config.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		return nil, p.parseError(respBody)
	}

	var cust customer
	if err := json.Unmarshal(respBody, &cust); err != nil {
		return nil, err
	}

	return p.mapCustomer(&cust), nil
}

// GetCustomer retrieves a customer.
func (p *Provider) GetCustomer(ctx context.Context, customerID string) (*gopay.Customer, error) {
	httpReq, err := http.NewRequestWithContext(ctx, "GET", p.config.BaseURL+"/customers/"+customerID, nil)
	if err != nil {
		return nil, err
	}

	httpReq.SetBasicAuth(p.config.KeyID, p.config.KeySecret)

	resp, err := p.config.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		return nil, p.parseError(respBody)
	}

	var cust customer
	if err := json.Unmarshal(respBody, &cust); err != nil {
		return nil, err
	}

	return p.mapCustomer(&cust), nil
}

// UpdateCustomer updates a customer.
func (p *Provider) UpdateCustomer(ctx context.Context, customerID string, req *gopay.CustomerRequest) (*gopay.Customer, error) {
	custReq := customerRequest{
		Name:    req.Name,
		Email:   req.Email,
		Contact: req.Phone,
		Notes:   req.Metadata,
	}

	body, err := json.Marshal(custReq)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "PUT", p.config.BaseURL+"/customers/"+customerID, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	httpReq.SetBasicAuth(p.config.KeyID, p.config.KeySecret)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.config.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		return nil, p.parseError(respBody)
	}

	var cust customer
	if err := json.Unmarshal(respBody, &cust); err != nil {
		return nil, err
	}

	return p.mapCustomer(&cust), nil
}

// DeleteCustomer is not supported by Razorpay.
func (p *Provider) DeleteCustomer(ctx context.Context, customerID string) error {
	return gopay.ErrUnsupported
}

// CreatePlan creates a recurring billing plan. Razorpay models the recurring
// price as the plan's item; the returned plan ID is the Razorpay plan ID.
func (p *Provider) CreatePlan(ctx context.Context, req *gopay.PlanRequest) (*gopay.Plan, error) {
	if req == nil {
		return nil, fmt.Errorf("gopay: nil plan request")
	}
	if req.Amount == nil {
		return nil, fmt.Errorf("gopay: nil amount")
	}
	period, ok := intervalToPeriod(req.Interval)
	if !ok {
		return nil, fmt.Errorf("gopay: unsupported billing interval %q", req.Interval)
	}

	count := req.IntervalCount
	if count <= 0 {
		count = 1
	}
	name := req.Name
	if name == "" {
		name = "gopay plan"
	}

	reqBody := planRequest{
		Period:   period,
		Interval: count,
		Item: planItem{
			Name:     name,
			Amount:   req.Amount.Value,
			Currency: req.Amount.Currency,
		},
		Notes: req.Metadata,
	}

	respBody, err := p.doJSON(ctx, "POST", "/plans", reqBody)
	if err != nil {
		return nil, err
	}

	var pl plan
	if err := json.Unmarshal(respBody, &pl); err != nil {
		return nil, err
	}
	return p.mapPlan(&pl), nil
}

// GetPlan retrieves a plan by ID.
func (p *Provider) GetPlan(ctx context.Context, planID string) (*gopay.Plan, error) {
	respBody, err := p.doJSON(ctx, "GET", "/plans/"+planID, nil)
	if err != nil {
		return nil, err
	}

	var pl plan
	if err := json.Unmarshal(respBody, &pl); err != nil {
		return nil, err
	}
	return p.mapPlan(&pl), nil
}

// CreateSubscription subscribes a customer to a plan. Razorpay requires a finite
// number of billing cycles (TotalCount) and activates the subscription only
// after the customer authorizes the mandate at the returned Subscription.AuthURL;
// until then the subscription's status is SubscriptionStatusIncomplete.
//
// The request's CustomerID and PaymentMethodID are not forwarded: Razorpay's
// Subscriptions API does not accept a customer ID at creation and rejects unknown
// fields — the customer and payment method are bound when the customer authorizes
// the mandate at AuthURL. The created/returned subscription's CustomerID is
// therefore populated by Razorpay only once authorization completes. TrialDays
// delays the first charge via the subscription's start time.
func (p *Provider) CreateSubscription(ctx context.Context, req *gopay.SubscriptionRequest) (*gopay.Subscription, error) {
	if req == nil {
		return nil, fmt.Errorf("gopay: nil subscription request")
	}
	if req.TotalCount <= 0 {
		return nil, fmt.Errorf("gopay: Razorpay subscriptions require a positive TotalCount (number of billing cycles)")
	}

	reqBody := subscriptionRequest{
		PlanID:     req.PlanID,
		TotalCount: req.TotalCount,
		Notes:      req.Metadata,
	}
	if req.TrialDays > 0 {
		reqBody.StartAt = time.Now().Add(time.Duration(req.TrialDays) * 24 * time.Hour).Unix()
	}

	respBody, err := p.doJSON(ctx, "POST", "/subscriptions", reqBody)
	if err != nil {
		return nil, err
	}

	var sub subscription
	if err := json.Unmarshal(respBody, &sub); err != nil {
		return nil, err
	}
	return p.mapSubscription(&sub), nil
}

// GetSubscription retrieves a subscription by ID.
func (p *Provider) GetSubscription(ctx context.Context, subscriptionID string) (*gopay.Subscription, error) {
	respBody, err := p.doJSON(ctx, "GET", "/subscriptions/"+subscriptionID, nil)
	if err != nil {
		return nil, err
	}

	var sub subscription
	if err := json.Unmarshal(respBody, &sub); err != nil {
		return nil, err
	}
	return p.mapSubscription(&sub), nil
}

// GetInvoice retrieves an invoice by ID.
func (p *Provider) GetInvoice(ctx context.Context, invoiceID string) (*gopay.Invoice, error) {
	respBody, err := p.doJSON(ctx, "GET", "/invoices/"+invoiceID, nil)
	if err != nil {
		return nil, err
	}

	var inv invoice
	if err := json.Unmarshal(respBody, &inv); err != nil {
		return nil, err
	}
	return p.mapInvoice(&inv), nil
}

// mapInvoice converts a Razorpay invoice entity to a gopay Invoice.
func (p *Provider) mapInvoice(inv *invoice) *gopay.Invoice {
	result := &gopay.Invoice{
		ID:               inv.ID,
		Status:           mapInvoiceStatus(inv.Status),
		SubscriptionID:   inv.SubscriptionID,
		CustomerID:       inv.CustomerID,
		Number:           inv.InvoiceNumber,
		HostedInvoiceURL: inv.ShortURL,
		Metadata:         inv.Notes,
		Provider:         p.Name(),
		Raw: map[string]any{
			"id":     inv.ID,
			"status": inv.Status,
		},
	}
	if inv.Currency != "" {
		result.Amount = gopay.NewAmount(inv.Amount, inv.Currency)
		result.AmountPaid = gopay.NewAmount(inv.AmountPaid, inv.Currency)
	}
	// Razorpay reports issuance time as `date`; fall back to it when the entity
	// omits a separate created_at.
	created := inv.CreatedAt
	if created == 0 {
		created = inv.Date
	}
	if created > 0 {
		result.CreatedAt = time.Unix(created, 0)
	}
	if inv.ExpireBy > 0 {
		result.DueDate = time.Unix(inv.ExpireBy, 0)
	}
	if inv.PaidAt > 0 {
		result.PaidAt = time.Unix(inv.PaidAt, 0)
	}
	return result
}

// mapInvoiceStatus maps a Razorpay invoice status to a normalized invoice status.
func mapInvoiceStatus(status string) gopay.InvoiceStatus {
	switch status {
	case "draft":
		return gopay.InvoiceStatusDraft
	case "issued", "partially_paid":
		return gopay.InvoiceStatusOpen
	case "paid":
		return gopay.InvoiceStatusPaid
	case "cancelled", "expired", "deleted":
		return gopay.InvoiceStatusVoid
	default:
		return gopay.InvoiceStatus(status)
	}
}

// CancelSubscription cancels a subscription. When opts.AtPeriodEnd is set the
// cancellation is scheduled for the end of the current billing cycle; otherwise
// it cancels immediately. A nil opts cancels immediately.
func (p *Provider) CancelSubscription(ctx context.Context, subscriptionID string, opts *gopay.CancelOptions) (*gopay.Subscription, error) {
	reqBody := map[string]int{"cancel_at_cycle_end": 0}
	if opts != nil && opts.AtPeriodEnd {
		reqBody["cancel_at_cycle_end"] = 1
	}

	respBody, err := p.doJSON(ctx, "POST", "/subscriptions/"+subscriptionID+"/cancel", reqBody)
	if err != nil {
		return nil, err
	}

	var sub subscription
	if err := json.Unmarshal(respBody, &sub); err != nil {
		return nil, err
	}
	return p.mapSubscription(&sub), nil
}

// doJSON performs an authenticated JSON request against the Razorpay API and
// returns the raw response body. A nil reqBody sends no payload (e.g. for GET).
func (p *Provider) doJSON(ctx context.Context, method, path string, reqBody any) ([]byte, error) {
	var bodyReader io.Reader
	if reqBody != nil {
		raw, err := json.Marshal(reqBody)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(raw)
	}

	httpReq, err := http.NewRequestWithContext(ctx, method, p.config.BaseURL+path, bodyReader)
	if err != nil {
		return nil, err
	}
	httpReq.SetBasicAuth(p.config.KeyID, p.config.KeySecret)
	if reqBody != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}

	resp, err := p.config.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, p.parseError(respBody)
	}
	return respBody, nil
}

// intervalToPeriod maps a gopay billing interval to Razorpay's period value.
func intervalToPeriod(interval gopay.BillingInterval) (string, bool) {
	switch interval {
	case gopay.BillingIntervalDay:
		return "daily", true
	case gopay.BillingIntervalWeek:
		return "weekly", true
	case gopay.BillingIntervalMonth:
		return "monthly", true
	case gopay.BillingIntervalYear:
		return "yearly", true
	default:
		return "", false
	}
}

// periodToInterval maps a Razorpay period value back to a gopay billing interval.
func periodToInterval(period string) gopay.BillingInterval {
	switch period {
	case "daily":
		return gopay.BillingIntervalDay
	case "weekly":
		return gopay.BillingIntervalWeek
	case "monthly":
		return gopay.BillingIntervalMonth
	case "yearly":
		return gopay.BillingIntervalYear
	default:
		return gopay.BillingInterval(period)
	}
}

func (p *Provider) mapPlan(pl *plan) *gopay.Plan {
	return &gopay.Plan{
		ID:            pl.ID,
		Name:          pl.Item.Name,
		Amount:        gopay.NewAmount(pl.Item.Amount, pl.Item.Currency),
		Interval:      periodToInterval(pl.Period),
		IntervalCount: pl.Interval,
		Metadata:      pl.Notes,
		CreatedAt:     time.Unix(pl.CreatedAt, 0),
		Provider:      p.Name(),
		Raw: map[string]any{
			"id":     pl.ID,
			"period": pl.Period,
		},
	}
}

func (p *Provider) mapSubscription(sub *subscription) *gopay.Subscription {
	result := &gopay.Subscription{
		ID:         sub.ID,
		CustomerID: sub.CustomerID,
		PlanID:     sub.PlanID,
		Status:     mapSubscriptionStatus(sub.Status),
		AuthURL:    sub.ShortURL,
		Metadata:   sub.Notes,
		CreatedAt:  time.Unix(sub.CreatedAt, 0),
		Provider:   p.Name(),
		Raw: map[string]any{
			"id":     sub.ID,
			"status": sub.Status,
		},
	}
	if sub.CurrentStart > 0 {
		result.CurrentPeriodStart = time.Unix(sub.CurrentStart, 0)
	}
	if sub.CurrentEnd > 0 {
		result.CurrentPeriodEnd = time.Unix(sub.CurrentEnd, 0)
	}
	if sub.Status == "cancelled" && sub.EndedAt > 0 {
		result.CanceledAt = time.Unix(sub.EndedAt, 0)
	}
	return result
}

// mapSubscriptionStatus maps a Razorpay subscription status to a normalized
// gopay subscription status.
func mapSubscriptionStatus(status string) gopay.SubscriptionStatus {
	switch status {
	case "created", "authenticated":
		// Mandate not yet authorized / first charge pending: not active yet.
		return gopay.SubscriptionStatusIncomplete
	case "active":
		return gopay.SubscriptionStatusActive
	case "pending":
		return gopay.SubscriptionStatusPastDue
	case "halted":
		return gopay.SubscriptionStatusUnpaid
	case "cancelled":
		return gopay.SubscriptionStatusCanceled
	case "completed":
		return gopay.SubscriptionStatusCompleted
	case "expired":
		return gopay.SubscriptionStatusIncompleteExpired
	default:
		return gopay.SubscriptionStatusIncomplete
	}
}

// ListPayments lists payments with pagination. Razorpay uses skip/count
// offsets; the opaque cursor encodes the next skip offset.
func (p *Provider) ListPayments(ctx context.Context, params *gopay.ListParams) (*gopay.List[*gopay.Payment], error) {
	skip := cursorToSkip(params)
	count := params.EffectiveLimit()

	body, err := p.doListRequest(ctx, "/payments", skip, count)
	if err != nil {
		return nil, err
	}

	var coll struct {
		Items []razorpayPayment `json:"items"`
	}
	if err := json.Unmarshal(body, &coll); err != nil {
		return nil, err
	}

	items := make([]*gopay.Payment, 0, len(coll.Items))
	for i := range coll.Items {
		items = append(items, p.mapPayment(&coll.Items[i]))
	}
	return buildRazorpayList(items, skip, count), nil
}

// ListRefunds lists refunds with pagination. The opaque cursor encodes the next
// skip offset.
func (p *Provider) ListRefunds(ctx context.Context, params *gopay.ListParams) (*gopay.List[*gopay.Refund], error) {
	skip := cursorToSkip(params)
	count := params.EffectiveLimit()

	body, err := p.doListRequest(ctx, "/refunds", skip, count)
	if err != nil {
		return nil, err
	}

	var coll struct {
		Items []razorpayRefund `json:"items"`
	}
	if err := json.Unmarshal(body, &coll); err != nil {
		return nil, err
	}

	items := make([]*gopay.Refund, 0, len(coll.Items))
	for i := range coll.Items {
		items = append(items, p.mapRefund(&coll.Items[i]))
	}
	return buildRazorpayList(items, skip, count), nil
}

// ListCustomers lists customers with pagination. The opaque cursor encodes the
// next skip offset.
func (p *Provider) ListCustomers(ctx context.Context, params *gopay.ListParams) (*gopay.List[*gopay.Customer], error) {
	skip := cursorToSkip(params)
	count := params.EffectiveLimit()

	body, err := p.doListRequest(ctx, "/customers", skip, count)
	if err != nil {
		return nil, err
	}

	var coll struct {
		Items []customer `json:"items"`
	}
	if err := json.Unmarshal(body, &coll); err != nil {
		return nil, err
	}

	items := make([]*gopay.Customer, 0, len(coll.Items))
	for i := range coll.Items {
		items = append(items, p.mapCustomer(&coll.Items[i]))
	}
	return buildRazorpayList(items, skip, count), nil
}

// doListRequest performs a paginated GET against a Razorpay collection endpoint
// and returns the raw response body.
func (p *Provider) doListRequest(ctx context.Context, path string, skip, count int) ([]byte, error) {
	url := fmt.Sprintf("%s%s?count=%d&skip=%d", p.config.BaseURL, path, count, skip)

	httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	httpReq.SetBasicAuth(p.config.KeyID, p.config.KeySecret)

	resp, err := p.config.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		return nil, p.parseError(respBody)
	}
	return respBody, nil
}

// cursorToSkip decodes the opaque pagination cursor into a Razorpay skip offset.
// An empty or malformed cursor starts from the beginning.
func cursorToSkip(params *gopay.ListParams) int {
	if params == nil || params.Cursor == "" {
		return 0
	}
	skip, err := strconv.Atoi(params.Cursor)
	if err != nil || skip < 0 {
		return 0
	}
	return skip
}

// buildRazorpayList assembles a gopay.List from a page of items. Razorpay does
// not return a has_more flag, so a full page (len == count) is treated as
// "there may be more", and the next cursor is the advanced skip offset.
func buildRazorpayList[T any](items []T, skip, count int) *gopay.List[T] {
	list := &gopay.List[T]{Items: items}
	if len(items) == count {
		list.HasMore = true
		list.NextCursor = strconv.Itoa(skip + len(items))
	}
	return list
}

// VerifyWebhook verifies and parses a Razorpay webhook event using HMAC-SHA256.
// Headers should contain "X-Razorpay-Signature".
func (p *Provider) VerifyWebhook(_ context.Context, payload []byte, headers map[string]string) (*gopay.WebhookEvent, error) {
	if p.config.WebhookSecret == "" {
		return nil, fmt.Errorf("%w: webhook secret not configured", gopay.ErrInvalidConfig)
	}

	signature := headers["X-Razorpay-Signature"]
	if signature == "" {
		return nil, fmt.Errorf("%w: missing X-Razorpay-Signature header", gopay.ErrProviderError)
	}

	mac := hmac.New(sha256.New, []byte(p.config.WebhookSecret))
	mac.Write(payload)
	expectedSig := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(signature), []byte(expectedSig)) {
		return nil, fmt.Errorf("%w: webhook signature verification failed", gopay.ErrProviderError)
	}

	return ParseWebhook(payload)
}

// ParseWebhook parses a Razorpay webhook event without verification.
func ParseWebhook(payload []byte) (*gopay.WebhookEvent, error) {
	var event struct {
		AccountID string          `json:"account_id"`
		Event     string          `json:"event"`
		Payload   json.RawMessage `json:"payload"`
	}

	if err := json.Unmarshal(payload, &event); err != nil {
		return nil, err
	}

	// Razorpay webhooks don't have a dedicated event ID at the top level.
	// Use account_id + event as a composite identifier.
	eventID := event.AccountID + ":" + event.Event

	ev := &gopay.WebhookEvent{
		ID:       eventID,
		Type:     event.Event,
		Provider: "razorpay",
		Raw:      event.Payload,
		Kind:     mapWebhookKind(event.Event),
	}

	// Razorpay nests the relevant entities under payload.{payment,order,refund}.entity.
	var pl struct {
		Payment struct {
			Entity struct {
				ID        string `json:"id"`
				OrderID   string `json:"order_id"`
				InvoiceID string `json:"invoice_id"`
				Amount    int64  `json:"amount"`
				Currency  string `json:"currency"`
			} `json:"entity"`
		} `json:"payment"`
		Order struct {
			Entity struct {
				ID       string `json:"id"`
				Amount   int64  `json:"amount"`
				Currency string `json:"currency"`
			} `json:"entity"`
		} `json:"order"`
		Refund struct {
			Entity struct {
				ID        string `json:"id"`
				PaymentID string `json:"payment_id"`
				Status    string `json:"status"`
				Amount    int64  `json:"amount"`
				Currency  string `json:"currency"`
			} `json:"entity"`
		} `json:"refund"`
		Subscription struct {
			Entity struct {
				ID string `json:"id"`
			} `json:"entity"`
		} `json:"subscription"`
	}
	if err := json.Unmarshal(event.Payload, &pl); err != nil {
		return ev, nil // best-effort: Raw is still available to the caller
	}

	// Subscription events carry a subscription entity, and subscription.charged
	// additionally carries the payment entity handled by the switch below. Set
	// the subscription ID independently so it is populated even when a payment
	// entity is also present.
	if pl.Subscription.Entity.ID != "" {
		ev.SubscriptionID = pl.Subscription.Entity.ID
	}

	switch {
	case pl.Refund.Entity.ID != "":
		ev.RefundID = pl.Refund.Entity.ID
		ev.PaymentID = pl.Refund.Entity.PaymentID
		if pl.Refund.Entity.Currency != "" {
			ev.Amount = gopay.NewAmount(pl.Refund.Entity.Amount, pl.Refund.Entity.Currency)
		}
		// refund.created can be pending; only a processed/failed entity status
		// yields a normalized success/failure so callers don't act early.
		switch pl.Refund.Entity.Status {
		case "processed":
			ev.Kind = gopay.WebhookRefundSucceeded
		case "failed":
			ev.Kind = gopay.WebhookRefundFailed
		}
	case pl.Payment.Entity.ID != "":
		ev.PaymentID = pl.Payment.Entity.ID
		ev.OrderID = pl.Payment.Entity.OrderID
		// subscription.charged carries the billed invoice on the payment entity;
		// surface it so InvoiceID is populated like it is for Stripe.
		ev.InvoiceID = pl.Payment.Entity.InvoiceID
		if pl.Payment.Entity.Currency != "" {
			ev.Amount = gopay.NewAmount(pl.Payment.Entity.Amount, pl.Payment.Entity.Currency)
		}
	case pl.Order.Entity.ID != "":
		ev.OrderID = pl.Order.Entity.ID
		if pl.Order.Entity.Currency != "" {
			ev.Amount = gopay.NewAmount(pl.Order.Entity.Amount, pl.Order.Entity.Currency)
		}
	}
	return ev, nil
}

// mapWebhookKind maps a Razorpay event name to a normalized webhook event kind.
func mapWebhookKind(event string) gopay.WebhookEventKind {
	switch event {
	case "payment.authorized":
		return gopay.WebhookPaymentCreated
	case "payment.captured", "order.paid":
		return gopay.WebhookPaymentSucceeded
	case "payment.failed":
		return gopay.WebhookPaymentFailed
	case "refund.processed":
		return gopay.WebhookRefundSucceeded
	case "refund.failed":
		return gopay.WebhookRefundFailed
	// refund.created is classified from the refund entity's status in
	// ParseWebhook rather than assumed successful here.
	case "subscription.activated":
		return gopay.WebhookSubscriptionCreated
	case "subscription.charged":
		// A recurring cycle was billed successfully; the payment entity carries
		// the charged amount.
		return gopay.WebhookInvoicePaymentSucceeded
	case "subscription.pending", "subscription.halted":
		// A recurring charge failed (pending = retrying, halted = retries
		// exhausted).
		return gopay.WebhookInvoicePaymentFailed
	case "subscription.cancelled":
		return gopay.WebhookSubscriptionCanceled
	case "subscription.authenticated", "subscription.updated",
		"subscription.paused", "subscription.resumed", "subscription.completed":
		// Non-terminal (or naturally terminal, for completed) state changes that
		// don't map to a more specific kind.
		return gopay.WebhookSubscriptionUpdated
	default:
		return gopay.WebhookUnknown
	}
}

func (p *Provider) mapOrder(o *order) *gopay.Payment {
	return &gopay.Payment{
		ID:          o.ID,
		Amount:      gopay.NewAmount(o.Amount, o.Currency),
		Status:      mapOrderStatus(o.Status),
		Description: o.Receipt,
		Metadata:    o.Notes,
		CreatedAt:   time.Unix(o.CreatedAt, 0),
		Provider:    p.Name(),
		Raw: map[string]any{
			"id":     o.ID,
			"status": o.Status,
		},
	}
}

func (p *Provider) mapPayment(pay *razorpayPayment) *gopay.Payment {
	result := &gopay.Payment{
		ID:             pay.ID,
		Amount:         gopay.NewAmount(pay.Amount, pay.Currency),
		Status:         mapPaymentStatus(pay.Status),
		Description:    pay.Description,
		Metadata:       pay.Notes,
		AmountRefunded: pay.AmountRefunded,
		CreatedAt:      time.Unix(pay.CreatedAt, 0),
		Provider:       p.Name(),
		Raw: map[string]any{
			"id":     pay.ID,
			"status": pay.Status,
			"method": pay.Method,
		},
	}

	if pay.Status == "captured" {
		result.AmountCaptured = pay.Amount
	}

	if pay.ErrorCode != "" {
		result.FailureCode = pay.ErrorCode
		result.FailureMessage = pay.ErrorDescription
	}

	return result
}

func mapOrderStatus(status string) gopay.PaymentStatus {
	switch status {
	case "created":
		return gopay.PaymentStatusPending
	case "attempted":
		return gopay.PaymentStatusProcessing
	case "paid":
		return gopay.PaymentStatusSucceeded
	default:
		return gopay.PaymentStatusPending
	}
}

func mapPaymentStatus(status string) gopay.PaymentStatus {
	switch status {
	case "created":
		return gopay.PaymentStatusPending
	case "authorized":
		return gopay.PaymentStatusRequiresCapture
	case "captured":
		return gopay.PaymentStatusSucceeded
	case "refunded":
		return gopay.PaymentStatusSucceeded
	case "failed":
		return gopay.PaymentStatusFailed
	default:
		return gopay.PaymentStatusPending
	}
}

func (p *Provider) mapRefund(r *razorpayRefund) *gopay.Refund {
	return &gopay.Refund{
		ID:        r.ID,
		PaymentID: r.PaymentID,
		Amount:    gopay.NewAmount(r.Amount, r.Currency),
		Status:    mapRefundStatus(r.Status),
		CreatedAt: time.Unix(r.CreatedAt, 0),
		Provider:  p.Name(),
		Raw: map[string]any{
			"id":     r.ID,
			"status": r.Status,
		},
	}
}

func mapRefundStatus(status string) gopay.RefundStatus {
	switch status {
	case "pending":
		return gopay.RefundStatusPending
	case "processed":
		return gopay.RefundStatusSucceeded
	case "failed":
		return gopay.RefundStatusFailed
	default:
		return gopay.RefundStatusPending
	}
}

func (p *Provider) mapCustomer(c *customer) *gopay.Customer {
	return &gopay.Customer{
		ID:        c.ID,
		Name:      c.Name,
		Email:     c.Email,
		Phone:     c.Contact,
		Metadata:  c.Notes,
		CreatedAt: time.Unix(c.CreatedAt, 0),
		Provider:  p.Name(),
		Raw: map[string]any{
			"id": c.ID,
		},
	}
}

func (p *Provider) parseError(body []byte) error {
	var errResp struct {
		Error struct {
			Code        string `json:"code"`
			Description string `json:"description"`
			Field       string `json:"field"`
		} `json:"error"`
	}

	if err := json.Unmarshal(body, &errResp); err != nil {
		return fmt.Errorf("%w: unparseable provider error response", gopay.ErrProviderError)
	}

	switch errResp.Error.Code {
	case "BAD_REQUEST_ERROR":
		if errResp.Error.Field == "amount" {
			return fmt.Errorf("%w: %s", gopay.ErrInvalidAmount, errResp.Error.Description)
		}
		return fmt.Errorf("%w: %s", gopay.ErrPaymentFailed, errResp.Error.Description)
	case "GATEWAY_ERROR":
		return fmt.Errorf("%w: %s", gopay.ErrCardDeclined, errResp.Error.Description)
	case "SERVER_ERROR":
		return fmt.Errorf("%w: %s", gopay.ErrProviderError, errResp.Error.Description)
	default:
		return fmt.Errorf("%w: %s", gopay.ErrProviderError, errResp.Error.Description)
	}
}

// Razorpay request/response types
type orderRequest struct {
	Amount   int64             `json:"amount"`
	Currency string            `json:"currency"`
	Receipt  string            `json:"receipt,omitempty"`
	Notes    map[string]string `json:"notes,omitempty"`
}

type order struct {
	ID        string `json:"id"`
	Amount    int64  `json:"amount"`
	Currency  string `json:"currency"`
	Status    string `json:"status"`
	Receipt   string `json:"receipt"`
	Notes     notes  `json:"notes"`
	CreatedAt int64  `json:"created_at"`
}

type razorpayPayment struct {
	ID               string `json:"id"`
	Amount           int64  `json:"amount"`
	Currency         string `json:"currency"`
	Status           string `json:"status"`
	Method           string `json:"method"`
	Description      string `json:"description"`
	AmountRefunded   int64  `json:"amount_refunded"`
	ErrorCode        string `json:"error_code"`
	ErrorDescription string `json:"error_description"`
	Notes            notes  `json:"notes"`
	CreatedAt        int64  `json:"created_at"`
}

type refundRequest struct {
	Amount int64             `json:"amount,omitempty"`
	Notes  map[string]string `json:"notes,omitempty"`
}

type razorpayRefund struct {
	ID        string `json:"id"`
	PaymentID string `json:"payment_id"`
	Amount    int64  `json:"amount"`
	Currency  string `json:"currency"`
	Status    string `json:"status"`
	CreatedAt int64  `json:"created_at"`
}

type customerRequest struct {
	Name    string            `json:"name,omitempty"`
	Email   string            `json:"email,omitempty"`
	Contact string            `json:"contact,omitempty"`
	Notes   map[string]string `json:"notes,omitempty"`
}

type customer struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	Contact   string `json:"contact"`
	Notes     notes  `json:"notes"`
	CreatedAt int64  `json:"created_at"`
}

type planItem struct {
	Name     string `json:"name"`
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
}

type planRequest struct {
	Period   string            `json:"period"`
	Interval int               `json:"interval"`
	Item     planItem          `json:"item"`
	Notes    map[string]string `json:"notes,omitempty"`
}

type plan struct {
	ID       string `json:"id"`
	Period   string `json:"period"`
	Interval int    `json:"interval"`
	Item     struct {
		Name     string `json:"name"`
		Amount   int64  `json:"amount"`
		Currency string `json:"currency"`
	} `json:"item"`
	Notes     notes `json:"notes"`
	CreatedAt int64 `json:"created_at"`
}

type subscriptionRequest struct {
	PlanID     string            `json:"plan_id"`
	TotalCount int               `json:"total_count"`
	StartAt    int64             `json:"start_at,omitempty"`
	Notes      map[string]string `json:"notes,omitempty"`
}

type subscription struct {
	ID           string `json:"id"`
	PlanID       string `json:"plan_id"`
	CustomerID   string `json:"customer_id"`
	Status       string `json:"status"`
	CurrentStart int64  `json:"current_start"`
	CurrentEnd   int64  `json:"current_end"`
	EndedAt      int64  `json:"ended_at"`
	ShortURL     string `json:"short_url"`
	Notes        notes  `json:"notes"`
	CreatedAt    int64  `json:"created_at"`
}

type invoice struct {
	ID             string `json:"id"`
	InvoiceNumber  string `json:"invoice_number"`
	CustomerID     string `json:"customer_id"`
	SubscriptionID string `json:"subscription_id"`
	Status         string `json:"status"`
	Amount         int64  `json:"amount"`
	AmountPaid     int64  `json:"amount_paid"`
	Currency       string `json:"currency"`
	ShortURL       string `json:"short_url"`
	Date           int64  `json:"date"`
	PaidAt         int64  `json:"paid_at"`
	ExpireBy       int64  `json:"expire_by"`
	CreatedAt      int64  `json:"created_at"`
	Notes          notes  `json:"notes"`
}
