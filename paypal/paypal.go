// Package paypal provides a PayPal payment provider for gopay.
package paypal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/KARTIKrocks/gopay"
)

// zeroDecimalCurrencies contains ISO 4217 currencies that do not use fractional units.
// For these currencies, the smallest unit equals 1 (e.g., 500 JPY is "500", not "5.00").
var zeroDecimalCurrencies = map[string]bool{
	"BIF": true, "CLP": true, "DJF": true, "GNF": true,
	"JPY": true, "KMF": true, "KRW": true, "MGA": true,
	"PYG": true, "RWF": true, "UGX": true, "VND": true,
	"VUV": true, "XAF": true, "XOF": true, "XPF": true,
}

// isZeroDecimal reports whether the given currency uses no fractional units.
func isZeroDecimal(currency string) bool {
	return zeroDecimalCurrencies[strings.ToUpper(currency)]
}

// toDecimal converts an amount in smallest currency unit to a decimal string
// suitable for the PayPal API.
// For standard currencies: 1999 USD -> "19.99", 500 EUR -> "5.00"
// For zero-decimal currencies: 500 JPY -> "500"
func toDecimal(amount int64, currency string) string {
	if isZeroDecimal(currency) {
		return fmt.Sprintf("%d", amount)
	}
	whole := amount / 100
	frac := amount % 100
	if frac < 0 {
		frac = -frac
	}
	return fmt.Sprintf("%d.%02d", whole, frac)
}

// fromDecimal converts a decimal string from the PayPal API to an amount in
// smallest currency unit.
// For standard currencies: "19.99" USD -> 1999, "5.00" EUR -> 500
// For zero-decimal currencies: "500" JPY -> 500
func fromDecimal(s string, currency string) int64 {
	if isZeroDecimal(currency) {
		// Strip any trailing decimals (e.g., "500.00" -> "500")
		if idx := strings.IndexByte(s, '.'); idx >= 0 {
			s = s[:idx]
		}
		v, _ := strconv.ParseInt(s, 10, 64)
		return v
	}
	parts := strings.SplitN(s, ".", 2)
	whole, _ := strconv.ParseInt(parts[0], 10, 64)
	var frac int64
	if len(parts) == 2 {
		fracStr := parts[1]
		// Pad or truncate to 2 decimal places
		switch {
		case len(fracStr) == 0:
			frac = 0
		case len(fracStr) == 1:
			frac, _ = strconv.ParseInt(fracStr, 10, 64)
			frac *= 10
		default:
			frac, _ = strconv.ParseInt(fracStr[:2], 10, 64)
		}
	}
	result := whole*100 + frac
	if whole < 0 {
		result = whole*100 - frac
	}
	return result
}

// maxResponseSize is the maximum response body size (10 MB).
const maxResponseSize = 10 << 20

const (
	sandboxURL    = "https://api-m.sandbox.paypal.com"
	productionURL = "https://api-m.paypal.com"
)

// Config holds PayPal-specific configuration.
type Config struct {
	// ClientID is the PayPal client ID.
	ClientID string

	// ClientSecret is the PayPal client secret.
	ClientSecret string

	// Sandbox enables sandbox mode.
	Sandbox bool

	// WebhookID is the webhook ID for verification.
	WebhookID string

	// HTTPClient is a custom HTTP client (optional).
	HTTPClient *http.Client

	// BaseURL overrides the API base URL (for testing).
	BaseURL string
}

// DefaultConfig returns a default PayPal configuration.
func DefaultConfig() Config {
	return Config{
		Sandbox:    true,
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// WithCredentials sets the credentials.
func (c Config) WithCredentials(clientID, clientSecret string) Config {
	c.ClientID = clientID
	c.ClientSecret = clientSecret
	return c
}

// WithSandbox enables/disables sandbox mode.
func (c Config) WithSandbox(sandbox bool) Config {
	c.Sandbox = sandbox
	return c
}

// WithWebhookID sets the webhook ID.
func (c Config) WithWebhookID(webhookID string) Config {
	c.WebhookID = webhookID
	return c
}

// WithHTTPClient sets a custom HTTP client.
func (c Config) WithHTTPClient(client *http.Client) Config {
	c.HTTPClient = client
	return c
}

// Provider implements gopay.Provider for PayPal.
type Provider struct {
	config      Config
	accessToken string
	tokenExpiry time.Time
	mu          sync.RWMutex
}

// NewProvider creates a new PayPal provider.
func NewProvider(config Config) (*Provider, error) {
	if config.ClientID == "" || config.ClientSecret == "" {
		return nil, fmt.Errorf("%w: client ID and secret required", gopay.ErrInvalidConfig)
	}

	if config.HTTPClient == nil {
		config.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}

	if config.BaseURL == "" {
		if config.Sandbox {
			config.BaseURL = sandboxURL
		} else {
			config.BaseURL = productionURL
		}
	}

	return &Provider{config: config}, nil
}

// Name returns the provider name.
func (p *Provider) Name() string {
	return "paypal"
}

// CreatePayment creates an order in PayPal.
func (p *Provider) CreatePayment(ctx context.Context, req *gopay.PaymentRequest) (*gopay.Payment, error) {
	token, err := p.getAccessToken(ctx)
	if err != nil {
		return nil, err
	}

	amountStr := toDecimal(req.Amount.Value, req.Amount.Currency)

	orderReq := orderRequest{
		Intent: "CAPTURE",
		PurchaseUnits: []purchaseUnit{
			{
				Amount: amount{
					CurrencyCode: req.Amount.Currency,
					Value:        amountStr,
				},
				Description: req.Description,
			},
		},
	}

	if req.CaptureMethod == gopay.CaptureManual {
		orderReq.Intent = "AUTHORIZE"
	}

	body, err := json.Marshal(orderReq)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.config.BaseURL+"/v2/checkout/orders", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+token)

	if req.IdempotencyKey != "" {
		httpReq.Header.Set("PayPal-Request-Id", req.IdempotencyKey)
	}

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

// GetPayment retrieves an order.
func (p *Provider) GetPayment(ctx context.Context, paymentID string) (*gopay.Payment, error) {
	token, err := p.getAccessToken(ctx)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "GET", p.config.BaseURL+"/v2/checkout/orders/"+paymentID, nil)
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Authorization", "Bearer "+token)

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

	var o order
	if err := json.Unmarshal(respBody, &o); err != nil {
		return nil, err
	}

	return p.mapOrder(&o), nil
}

// CapturePayment captures an authorized order.
// For AUTHORIZE intent, this authorizes the order (if needed) and then captures the authorization.
// For CAPTURE intent, this captures the order directly.
func (p *Provider) CapturePayment(ctx context.Context, paymentID string, amt *gopay.Amount) (*gopay.Payment, error) {
	// Get the order to determine the correct capture flow.
	pay, err := p.GetPayment(ctx, paymentID)
	if err != nil {
		return nil, err
	}

	intent, _ := pay.Raw["intent"].(string)

	// AUTHORIZE intent: authorize the order (if needed), then capture the authorization.
	if intent == "AUTHORIZE" {
		token, err := p.getAccessToken(ctx)
		if err != nil {
			return nil, err
		}

		authID, _ := pay.Raw["authorization_id"].(string)
		if authID == "" {
			authID, err = p.authorizeOrder(ctx, token, paymentID)
			if err != nil {
				return nil, err
			}
		}
		return p.captureAuthorization(ctx, token, paymentID, authID, amt)
	}

	// CAPTURE intent: capture the order directly.
	token, err := p.getAccessToken(ctx)
	if err != nil {
		return nil, err
	}

	var bodyReader io.Reader
	if amt != nil {
		captureReq := map[string]any{
			"amount": map[string]string{
				"currency_code": amt.Currency,
				"value":         toDecimal(amt.Value, amt.Currency),
			},
		}
		body, _ := json.Marshal(captureReq)
		bodyReader = bytes.NewReader(body)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.config.BaseURL+"/v2/checkout/orders/"+paymentID+"/capture", bodyReader)
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+token)

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

	var o order
	if err := json.Unmarshal(respBody, &o); err != nil {
		return nil, err
	}

	return p.mapOrder(&o), nil
}

// authorizeOrder authorizes an AUTHORIZE-intent order after buyer approval.
func (p *Provider) authorizeOrder(ctx context.Context, token, orderID string) (string, error) {
	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.config.BaseURL+"/v2/checkout/orders/"+orderID+"/authorize", nil)
	if err != nil {
		return "", err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+token)

	resp, err := p.config.HTTPClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("%w: %v", gopay.ErrPaymentFailed, err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return "", err
	}

	if resp.StatusCode >= 400 {
		return "", p.parseError(respBody)
	}

	var o order
	if err := json.Unmarshal(respBody, &o); err != nil {
		return "", err
	}

	if len(o.PurchaseUnits) > 0 && o.PurchaseUnits[0].Payments != nil {
		if len(o.PurchaseUnits[0].Payments.Authorizations) > 0 {
			return o.PurchaseUnits[0].Payments.Authorizations[0].ID, nil
		}
	}

	return "", fmt.Errorf("%w: no authorization ID in response", gopay.ErrPaymentFailed)
}

// captureAuthorization captures a PayPal authorization.
func (p *Provider) captureAuthorization(ctx context.Context, token, orderID, authID string, amt *gopay.Amount) (*gopay.Payment, error) {
	var bodyReader io.Reader
	if amt != nil {
		captureReq := map[string]any{
			"amount": map[string]string{
				"currency_code": amt.Currency,
				"value":         toDecimal(amt.Value, amt.Currency),
			},
		}
		body, _ := json.Marshal(captureReq)
		bodyReader = bytes.NewReader(body)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.config.BaseURL+"/v2/payments/authorizations/"+authID+"/capture", bodyReader)
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+token)

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

	// Fetch the full order to return a complete Payment.
	return p.GetPayment(ctx, orderID)
}

// CancelPayment voids an authorized order.
func (p *Provider) CancelPayment(ctx context.Context, paymentID string) (*gopay.Payment, error) {
	// Get the order to find the authorization ID.
	pay, err := p.GetPayment(ctx, paymentID)
	if err != nil {
		return nil, err
	}

	authID, ok := pay.Raw["authorization_id"].(string)
	if !ok || authID == "" {
		return nil, fmt.Errorf("%w: no authorization found to void", gopay.ErrUnsupported)
	}

	token, err := p.getAccessToken(ctx)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.config.BaseURL+"/v2/payments/authorizations/"+authID+"/void", nil)
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+token)

	resp, err := p.config.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", gopay.ErrPaymentFailed, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
		return nil, p.parseError(respBody)
	}

	pay.Status = gopay.PaymentStatusCanceled
	return pay, nil
}

// Refund creates a refund for a captured payment.
func (p *Provider) Refund(ctx context.Context, req *gopay.RefundRequest) (*gopay.Refund, error) {
	token, err := p.getAccessToken(ctx)
	if err != nil {
		return nil, err
	}

	// First get the capture ID from the order
	pay, err := p.GetPayment(ctx, req.PaymentID)
	if err != nil {
		return nil, err
	}

	captureID, ok := pay.Raw["capture_id"].(string)
	if !ok || captureID == "" {
		return nil, fmt.Errorf("%w: no capture found", gopay.ErrRefundFailed)
	}

	refundReq := make(map[string]any)
	if req.Amount != nil {
		refundReq["amount"] = map[string]string{
			"currency_code": req.Amount.Currency,
			"value":         toDecimal(req.Amount.Value, req.Amount.Currency),
		}
	}

	if req.Reason != "" {
		refundReq["note_to_payer"] = string(req.Reason)
	}

	body, _ := json.Marshal(refundReq)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.config.BaseURL+"/v2/payments/captures/"+captureID+"/refund", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+token)

	if req.IdempotencyKey != "" {
		httpReq.Header.Set("PayPal-Request-Id", req.IdempotencyKey)
	}

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

	var refundResp refund
	if err := json.Unmarshal(respBody, &refundResp); err != nil {
		return nil, err
	}

	return p.mapRefund(&refundResp, req.PaymentID), nil
}

// GetRefund retrieves a refund.
func (p *Provider) GetRefund(ctx context.Context, refundID string) (*gopay.Refund, error) {
	token, err := p.getAccessToken(ctx)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "GET", p.config.BaseURL+"/v2/payments/refunds/"+refundID, nil)
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Authorization", "Bearer "+token)

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

	var refundResp refund
	if err := json.Unmarshal(respBody, &refundResp); err != nil {
		return nil, err
	}

	return p.mapRefund(&refundResp, ""), nil
}

// VerifyWebhook verifies and parses a PayPal webhook event.
// Headers should contain PayPal webhook headers for verification.
func (p *Provider) VerifyWebhook(ctx context.Context, payload []byte, headers map[string]string) (*gopay.WebhookEvent, error) {
	if p.config.WebhookID == "" {
		return nil, fmt.Errorf("%w: webhook ID not configured", gopay.ErrInvalidConfig)
	}

	token, err := p.getAccessToken(ctx)
	if err != nil {
		return nil, err
	}

	verifyReq := map[string]any{
		"auth_algo":         headers["PAYPAL-AUTH-ALGO"],
		"cert_url":          headers["PAYPAL-CERT-URL"],
		"transmission_id":   headers["PAYPAL-TRANSMISSION-ID"],
		"transmission_sig":  headers["PAYPAL-TRANSMISSION-SIG"],
		"transmission_time": headers["PAYPAL-TRANSMISSION-TIME"],
		"webhook_id":        p.config.WebhookID,
		"webhook_event":     json.RawMessage(payload),
	}

	body, err := json.Marshal(verifyReq)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.config.BaseURL+"/v1/notifications/verify-webhook-signature", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+token)

	resp, err := p.config.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%w: webhook verification failed: %s", gopay.ErrProviderError, err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return nil, err
	}

	var verifyResp struct {
		VerificationStatus string `json:"verification_status"`
	}
	if err := json.Unmarshal(respBody, &verifyResp); err != nil {
		return nil, err
	}

	if verifyResp.VerificationStatus != "SUCCESS" {
		return nil, fmt.Errorf("%w: webhook signature verification failed", gopay.ErrProviderError)
	}

	return ParseWebhook(payload)
}

// ParseWebhook parses a PayPal webhook event without verification.
func ParseWebhook(payload []byte) (*gopay.WebhookEvent, error) {
	var event struct {
		ID         string          `json:"id"`
		EventType  string          `json:"event_type"`
		Resource   json.RawMessage `json:"resource"`
		CreateTime time.Time       `json:"create_time"`
	}

	if err := json.Unmarshal(payload, &event); err != nil {
		return nil, err
	}

	return &gopay.WebhookEvent{
		ID:       event.ID,
		Type:     event.EventType,
		Provider: "paypal",
		Raw:      event.Resource,
	}, nil
}

func (p *Provider) getAccessToken(ctx context.Context) (string, error) {
	p.mu.RLock()
	if p.accessToken != "" && time.Now().Before(p.tokenExpiry) {
		token := p.accessToken
		p.mu.RUnlock()
		return token, nil
	}
	p.mu.RUnlock()

	p.mu.Lock()
	defer p.mu.Unlock()

	// Double-check after acquiring write lock
	if p.accessToken != "" && time.Now().Before(p.tokenExpiry) {
		return p.accessToken, nil
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.config.BaseURL+"/v1/oauth2/token", bytes.NewBufferString("grant_type=client_credentials"))
	if err != nil {
		return "", err
	}

	httpReq.SetBasicAuth(p.config.ClientID, p.config.ClientSecret)
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.config.HTTPClient.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return "", err
	}

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("%w: failed to get access token", gopay.ErrProviderError)
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}

	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", err
	}

	p.accessToken = tokenResp.AccessToken
	bufferSecs := tokenResp.ExpiresIn - 60
	if bufferSecs < 0 {
		bufferSecs = 0
	}
	p.tokenExpiry = time.Now().Add(time.Duration(bufferSecs) * time.Second)

	return p.accessToken, nil
}

func (p *Provider) mapOrder(o *order) *gopay.Payment {
	pay := &gopay.Payment{
		ID:        o.ID,
		Status:    mapOrderStatus(o.Status),
		CreatedAt: o.CreateTime,
		Provider:  p.Name(),
		Raw: map[string]any{
			"id":     o.ID,
			"intent": o.Intent,
			"status": o.Status,
		},
	}

	if len(o.PurchaseUnits) > 0 {
		pu := o.PurchaseUnits[0]
		pay.Amount = gopay.NewAmount(fromDecimal(pu.Amount.Value, pu.Amount.CurrencyCode), pu.Amount.CurrencyCode)
		pay.Description = pu.Description

		if pu.Payments != nil {
			if len(pu.Payments.Authorizations) > 0 {
				pay.Raw["authorization_id"] = pu.Payments.Authorizations[0].ID
			}
			if len(pu.Payments.Captures) > 0 {
				pay.Raw["capture_id"] = pu.Payments.Captures[0].ID
				pay.AmountCaptured = fromDecimal(pu.Payments.Captures[0].Amount.Value, pu.Payments.Captures[0].Amount.CurrencyCode)
			}
		}
	}

	for _, link := range o.Links {
		if link.Rel == "approve" {
			pay.RedirectURL = link.Href
			break
		}
	}

	return pay
}

func mapOrderStatus(status string) gopay.PaymentStatus {
	switch status {
	case "CREATED":
		return gopay.PaymentStatusPending
	case "SAVED":
		return gopay.PaymentStatusPending
	case "APPROVED":
		return gopay.PaymentStatusRequiresCapture
	case "VOIDED":
		return gopay.PaymentStatusCanceled
	case "COMPLETED":
		return gopay.PaymentStatusSucceeded
	case "PAYER_ACTION_REQUIRED":
		return gopay.PaymentStatusRequiresAction
	default:
		return gopay.PaymentStatusPending
	}
}

func (p *Provider) mapRefund(r *refund, paymentID string) *gopay.Refund {
	return &gopay.Refund{
		ID:        r.ID,
		PaymentID: paymentID,
		Amount:    gopay.NewAmount(fromDecimal(r.Amount.Value, r.Amount.CurrencyCode), r.Amount.CurrencyCode),
		Status:    mapRefundStatus(r.Status),
		CreatedAt: r.CreateTime,
		Provider:  p.Name(),
		Raw: map[string]any{
			"id":     r.ID,
			"status": r.Status,
		},
	}
}

func mapRefundStatus(status string) gopay.RefundStatus {
	switch status {
	case "CANCELLED":
		return gopay.RefundStatusCanceled
	case "FAILED":
		return gopay.RefundStatusFailed
	case "PENDING":
		return gopay.RefundStatusPending
	case "COMPLETED":
		return gopay.RefundStatusSucceeded
	default:
		return gopay.RefundStatusPending
	}
}

func (p *Provider) parseError(body []byte) error {
	var errResp struct {
		Name    string `json:"name"`
		Message string `json:"message"`
		Details []struct {
			Issue       string `json:"issue"`
			Description string `json:"description"`
		} `json:"details"`
	}

	if err := json.Unmarshal(body, &errResp); err != nil {
		return fmt.Errorf("%w: %s", gopay.ErrProviderError, string(body))
	}

	switch errResp.Name {
	case "RESOURCE_NOT_FOUND":
		return fmt.Errorf("%w: %s", gopay.ErrNotFound, errResp.Message)
	case "INVALID_RESOURCE_ID":
		return fmt.Errorf("%w: %s", gopay.ErrNotFound, errResp.Message)
	case "UNPROCESSABLE_ENTITY":
		if len(errResp.Details) > 0 {
			return fmt.Errorf("%w: %s", gopay.ErrPaymentFailed, errResp.Details[0].Description)
		}
		return fmt.Errorf("%w: %s", gopay.ErrPaymentFailed, errResp.Message)
	default:
		return fmt.Errorf("%w: %s", gopay.ErrProviderError, errResp.Message)
	}
}

// PayPal request/response types
type orderRequest struct {
	Intent        string         `json:"intent"`
	PurchaseUnits []purchaseUnit `json:"purchase_units"`
}

type purchaseUnit struct {
	Amount      amount    `json:"amount"`
	Description string    `json:"description,omitempty"`
	Payments    *payments `json:"payments,omitempty"`
}

type amount struct {
	CurrencyCode string `json:"currency_code"`
	Value        string `json:"value"`
}

type payments struct {
	Authorizations []authorization `json:"authorizations,omitempty"`
	Captures       []capture       `json:"captures,omitempty"`
}

type authorization struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

type capture struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Amount amount `json:"amount"`
}

type order struct {
	ID            string         `json:"id"`
	Intent        string         `json:"intent"`
	Status        string         `json:"status"`
	PurchaseUnits []purchaseUnit `json:"purchase_units"`
	Links         []link         `json:"links"`
	CreateTime    time.Time      `json:"create_time"`
}

type link struct {
	Href   string `json:"href"`
	Rel    string `json:"rel"`
	Method string `json:"method"`
}

type refund struct {
	ID         string    `json:"id"`
	Status     string    `json:"status"`
	Amount     amount    `json:"amount"`
	CreateTime time.Time `json:"create_time"`
}
