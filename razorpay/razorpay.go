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
	"time"

	"github.com/KARTIKrocks/gopay"
)

const (
	baseURL = "https://api.razorpay.com/v1"

	// maxResponseSize is the maximum response body size (10 MB).
	maxResponseSize = 10 << 20
)

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

// WithHTTPClient sets a custom HTTP client.
func (c Config) WithHTTPClient(client *http.Client) Config {
	c.HTTPClient = client
	return c
}

// Provider implements gopay.Provider and gopay.CustomerProvider for Razorpay.
type Provider struct {
	config Config
}

// NewProvider creates a new Razorpay provider.
func NewProvider(config Config) (*Provider, error) {
	if config.KeyID == "" || config.KeySecret == "" {
		return nil, fmt.Errorf("%w: key ID and secret required", gopay.ErrInvalidConfig)
	}

	if config.HTTPClient == nil {
		config.HTTPClient = &http.Client{Timeout: 30 * time.Second}
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

	body, _ := json.Marshal(captureReq)

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
		Contains  []struct {
			// Razorpay webhooks nest entity IDs inside "contains".
		} `json:"-"`
	}

	if err := json.Unmarshal(payload, &event); err != nil {
		return nil, err
	}

	// Razorpay webhooks don't have a dedicated event ID at the top level.
	// Use account_id + event as a composite identifier.
	eventID := event.AccountID + ":" + event.Event

	return &gopay.WebhookEvent{
		ID:       eventID,
		Type:     event.Event,
		Provider: "razorpay",
		Raw:      event.Payload,
	}, nil
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
		return fmt.Errorf("%w: %s", gopay.ErrProviderError, string(body))
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
	ID        string            `json:"id"`
	Amount    int64             `json:"amount"`
	Currency  string            `json:"currency"`
	Status    string            `json:"status"`
	Receipt   string            `json:"receipt"`
	Notes     map[string]string `json:"notes"`
	CreatedAt int64             `json:"created_at"`
}

type razorpayPayment struct {
	ID               string            `json:"id"`
	Amount           int64             `json:"amount"`
	Currency         string            `json:"currency"`
	Status           string            `json:"status"`
	Method           string            `json:"method"`
	Description      string            `json:"description"`
	AmountRefunded   int64             `json:"amount_refunded"`
	ErrorCode        string            `json:"error_code"`
	ErrorDescription string            `json:"error_description"`
	Notes            map[string]string `json:"notes"`
	CreatedAt        int64             `json:"created_at"`
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
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Email     string            `json:"email"`
	Contact   string            `json:"contact"`
	Notes     map[string]string `json:"notes"`
	CreatedAt int64             `json:"created_at"`
}
