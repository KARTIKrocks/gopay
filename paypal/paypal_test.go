package paypal

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/KARTIKrocks/gopay"
)

func TestNewProviderValidation(t *testing.T) {
	// Missing credentials
	_, err := NewProvider(Config{})
	if !errors.Is(err, gopay.ErrInvalidConfig) {
		t.Errorf("expected ErrInvalidConfig, got %v", err)
	}

	// Missing secret
	_, err = NewProvider(Config{ClientID: "id"})
	if !errors.Is(err, gopay.ErrInvalidConfig) {
		t.Errorf("expected ErrInvalidConfig for missing secret, got %v", err)
	}

	// Missing ID
	_, err = NewProvider(Config{ClientSecret: "secret"})
	if !errors.Is(err, gopay.ErrInvalidConfig) {
		t.Errorf("expected ErrInvalidConfig for missing ID, got %v", err)
	}

	// Valid config - sandbox
	p, err := NewProvider(Config{ClientID: "id", ClientSecret: "secret", Sandbox: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name() != "paypal" {
		t.Errorf("name = %s, want paypal", p.Name())
	}
	if p.config.BaseURL != sandboxURL {
		t.Errorf("BaseURL = %s, want %s", p.config.BaseURL, sandboxURL)
	}

	// Valid config - production
	p, err = NewProvider(Config{ClientID: "id", ClientSecret: "secret", Sandbox: false})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.config.BaseURL != productionURL {
		t.Errorf("BaseURL = %s, want %s", p.config.BaseURL, productionURL)
	}

	// Nil HTTPClient gets default
	if p.config.HTTPClient == nil {
		t.Error("HTTPClient should not be nil")
	}
}

func TestConfigBuilders(t *testing.T) {
	cfg := DefaultConfig().
		WithCredentials("client_id", "client_secret").
		WithSandbox(false).
		WithWebhookID("wh_123")

	if cfg.ClientID != "client_id" {
		t.Errorf("ClientID = %s, want client_id", cfg.ClientID)
	}
	if cfg.ClientSecret != "client_secret" {
		t.Errorf("ClientSecret = %s, want client_secret", cfg.ClientSecret)
	}
	if cfg.Sandbox {
		t.Error("Sandbox should be false")
	}
	if cfg.WebhookID != "wh_123" {
		t.Errorf("WebhookID = %s, want wh_123", cfg.WebhookID)
	}
	if cfg.HTTPClient == nil {
		t.Error("HTTPClient should not be nil from DefaultConfig")
	}
}

func TestToDecimal(t *testing.T) {
	tests := []struct {
		amount   int64
		currency string
		want     string
	}{
		{0, "USD", "0.00"},
		{1, "USD", "0.01"},
		{10, "USD", "0.10"},
		{99, "USD", "0.99"},
		{100, "USD", "1.00"},
		{199, "USD", "1.99"},
		{1999, "USD", "19.99"},
		{10000, "USD", "100.00"},
		{123456, "USD", "1234.56"},
		// Negative amounts
		{-100, "USD", "-1.00"},
		{-199, "USD", "-1.99"},
		// Zero-decimal currencies
		{500, "JPY", "500"},
		{1, "JPY", "1"},
		{0, "JPY", "0"},
		{1999, "KRW", "1999"},
		{100, "VND", "100"},
	}

	for _, tt := range tests {
		got := toDecimal(tt.amount, tt.currency)
		if got != tt.want {
			t.Errorf("toDecimal(%d, %s) = %q, want %q", tt.amount, tt.currency, got, tt.want)
		}
	}
}

func TestFromDecimal(t *testing.T) {
	tests := []struct {
		decimal  string
		currency string
		want     int64
	}{
		{"0.00", "USD", 0},
		{"0.01", "USD", 1},
		{"0.10", "USD", 10},
		{"0.99", "USD", 99},
		{"1.00", "USD", 100},
		{"1.99", "USD", 199},
		{"19.99", "USD", 1999},
		{"100.00", "USD", 10000},
		{"1234.56", "USD", 123456},
		{"5", "USD", 500},
		{"5.5", "USD", 550},
		{"10.123", "USD", 1012}, // truncated to 2 decimal places
		// Negative amounts
		{"-1.00", "USD", -100},
		{"-19.99", "USD", -1999},
		// Negative amounts with a zero whole part must keep their sign.
		{"-0.99", "USD", -99},
		{"-0.50", "USD", -50},
		{"-0.01", "USD", -1},
		// Edge cases
		{"0", "USD", 0},
		{"5.", "USD", 500},
		// Zero-decimal currencies
		{"500", "JPY", 500},
		{"1", "JPY", 1},
		{"0", "JPY", 0},
		{"1999", "KRW", 1999},
		{"100", "VND", 100},
		{"500.00", "JPY", 500}, // strip trailing decimals
	}

	for _, tt := range tests {
		got, err := fromDecimal(tt.decimal, tt.currency)
		if err != nil {
			t.Errorf("fromDecimal(%q, %s) error: %v", tt.decimal, tt.currency, err)
			continue
		}
		if got != tt.want {
			t.Errorf("fromDecimal(%q, %s) = %d, want %d", tt.decimal, tt.currency, got, tt.want)
		}
	}
}

func TestRoundTrip(t *testing.T) {
	// Standard currencies
	values := []int64{0, 1, 10, 99, 100, 199, 1999, 10000, 123456, 999999}
	for _, v := range values {
		decimal := toDecimal(v, "USD")
		back, err := fromDecimal(decimal, "USD")
		if err != nil {
			t.Errorf("USD round trip error: %d -> %q: %v", v, decimal, err)
			continue
		}
		if back != v {
			t.Errorf("USD round trip failed: %d -> %q -> %d", v, decimal, back)
		}
	}

	// Zero-decimal currencies
	for _, v := range values {
		decimal := toDecimal(v, "JPY")
		back, err := fromDecimal(decimal, "JPY")
		if err != nil {
			t.Errorf("JPY round trip error: %d -> %q: %v", v, decimal, err)
			continue
		}
		if back != v {
			t.Errorf("JPY round trip failed: %d -> %q -> %d", v, decimal, back)
		}
	}
}

func TestIsZeroDecimal(t *testing.T) {
	zeroDecimal := []string{"JPY", "KRW", "VND", "BIF", "CLP", "XPF"}
	for _, c := range zeroDecimal {
		if !isZeroDecimal(c) {
			t.Errorf("isZeroDecimal(%s) = false, want true", c)
		}
	}

	// Case insensitive
	if !isZeroDecimal("jpy") {
		t.Error("isZeroDecimal(jpy) = false, want true")
	}

	notZeroDecimal := []string{"USD", "EUR", "GBP", "INR"}
	for _, c := range notZeroDecimal {
		if isZeroDecimal(c) {
			t.Errorf("isZeroDecimal(%s) = true, want false", c)
		}
	}
}

func TestMapOrderStatus(t *testing.T) {
	tests := []struct {
		input string
		want  gopay.PaymentStatus
	}{
		{"CREATED", gopay.PaymentStatusPending},
		{"SAVED", gopay.PaymentStatusPending},
		{"APPROVED", gopay.PaymentStatusRequiresCapture},
		{"VOIDED", gopay.PaymentStatusCanceled},
		{"COMPLETED", gopay.PaymentStatusSucceeded},
		{"PAYER_ACTION_REQUIRED", gopay.PaymentStatusRequiresAction},
		{"UNKNOWN_STATUS", gopay.PaymentStatusPending},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := mapOrderStatus(tt.input)
			if got != tt.want {
				t.Errorf("mapOrderStatus(%s) = %s, want %s", tt.input, got, tt.want)
			}
		})
	}
}

func TestMapRefundStatus(t *testing.T) {
	tests := []struct {
		input string
		want  gopay.RefundStatus
	}{
		{"CANCELLED", gopay.RefundStatusCanceled},
		{"FAILED", gopay.RefundStatusFailed},
		{"PENDING", gopay.RefundStatusPending},
		{"COMPLETED", gopay.RefundStatusSucceeded},
		{"UNKNOWN", gopay.RefundStatusPending},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := mapRefundStatus(tt.input)
			if got != tt.want {
				t.Errorf("mapRefundStatus(%s) = %s, want %s", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseError(t *testing.T) {
	p := &Provider{}

	tests := []struct {
		name     string
		status   int
		body     string
		wantErr  error
		wantCode string
	}{
		{
			"resource not found",
			http.StatusNotFound,
			`{"name":"RESOURCE_NOT_FOUND","message":"resource not found"}`,
			gopay.ErrNotFound, "RESOURCE_NOT_FOUND",
		},
		{
			"invalid resource ID",
			http.StatusNotFound,
			`{"name":"INVALID_RESOURCE_ID","message":"invalid ID"}`,
			gopay.ErrNotFound, "INVALID_RESOURCE_ID",
		},
		{
			"unprocessable entity with unmapped issue",
			http.StatusUnprocessableEntity,
			`{"name":"UNPROCESSABLE_ENTITY","message":"error","details":[{"issue":"ISSUE","description":"detail desc"}]}`,
			gopay.ErrPaymentFailed, "ISSUE",
		},
		{
			"unprocessable entity without details",
			http.StatusUnprocessableEntity,
			`{"name":"UNPROCESSABLE_ENTITY","message":"generic error"}`,
			gopay.ErrPaymentFailed, "UNPROCESSABLE_ENTITY",
		},
		{
			"instrument declined",
			http.StatusUnprocessableEntity,
			`{"name":"UNPROCESSABLE_ENTITY","details":[{"issue":"INSTRUMENT_DECLINED","description":"declined"}]}`,
			gopay.ErrCardDeclined, "INSTRUMENT_DECLINED",
		},
		{
			"card expired",
			http.StatusUnprocessableEntity,
			`{"name":"UNPROCESSABLE_ENTITY","details":[{"issue":"CARD_EXPIRED","description":"expired"}]}`,
			gopay.ErrExpiredCard, "CARD_EXPIRED",
		},
		{
			"already captured",
			http.StatusUnprocessableEntity,
			`{"name":"UNPROCESSABLE_ENTITY","details":[{"issue":"ORDER_ALREADY_CAPTURED","description":"captured"}]}`,
			gopay.ErrAlreadyCaptured, "ORDER_ALREADY_CAPTURED",
		},
		{
			"already refunded",
			http.StatusUnprocessableEntity,
			`{"name":"UNPROCESSABLE_ENTITY","details":[{"issue":"CAPTURE_FULLY_REFUNDED","description":"refunded"}]}`,
			gopay.ErrAlreadyRefunded, "CAPTURE_FULLY_REFUNDED",
		},
		{
			"rate limit name",
			http.StatusTooManyRequests,
			`{"name":"RATE_LIMIT_REACHED","message":"slow down"}`,
			gopay.ErrRateLimited, "RATE_LIMIT_REACHED",
		},
		{
			"server error",
			http.StatusInternalServerError,
			`{"name":"INTERNAL_SERVER_ERROR","message":"server error"}`,
			gopay.ErrProviderUnavailable, "INTERNAL_SERVER_ERROR",
		},
		{
			"oauth invalid client",
			http.StatusUnauthorized,
			`{"error":"invalid_client","error_description":"Client Authentication failed"}`,
			gopay.ErrInvalidCredentials, "invalid_client",
		},
		{
			"bad request",
			http.StatusBadRequest,
			`{"name":"INVALID_REQUEST","message":"bad"}`,
			gopay.ErrProviderError, "INVALID_REQUEST",
		},
		{
			"non-JSON gateway error",
			http.StatusBadGateway,
			`<html>bad gateway</html>`,
			gopay.ErrProviderUnavailable, "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &http.Response{StatusCode: tt.status, Header: http.Header{}}
			err := p.parseError(resp, []byte(tt.body))
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("parseError() = %v, want %v", err, tt.wantErr)
			}
			perr, ok := errors.AsType[*gopay.Error](err)
			if !ok {
				t.Fatalf("parseError() = %T, want *gopay.Error", err)
			}
			if perr.Code != tt.wantCode || perr.StatusCode != tt.status || perr.Provider != "paypal" {
				t.Errorf("details = {Code:%q Status:%d Provider:%q}, want {Code:%q Status:%d Provider:paypal}",
					perr.Code, perr.StatusCode, perr.Provider, tt.wantCode, tt.status)
			}
		})
	}
}

func TestParseErrorHeaders(t *testing.T) {
	p := &Provider{}
	resp := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header: http.Header{
			"Paypal-Debug-Id": {"dbg123"},
			"Retry-After":     {"7"},
		},
	}

	err := p.parseError(resp, []byte(`{"name":"RATE_LIMIT_REACHED","message":"slow down"}`))

	if !gopay.IsRetryable(err) {
		t.Errorf("IsRetryable(%v) = false, want true", err)
	}
	if got := gopay.RetryAfter(err); got != 7*time.Second {
		t.Errorf("RetryAfter() = %v, want 7s", got)
	}
	if perr, _ := errors.AsType[*gopay.Error](err); perr.RequestID != "dbg123" {
		t.Errorf("RequestID = %q, want dbg123", perr.RequestID)
	}
}

func TestParseWebhook(t *testing.T) {
	payload := []byte(`{"id":"WH-123","event_type":"PAYMENT.CAPTURE.COMPLETED","resource":{"id":"CAP-456"}}`)

	event, err := ParseWebhook(payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if event.ID != "WH-123" {
		t.Errorf("ID = %s, want WH-123", event.ID)
	}
	if event.Type != "PAYMENT.CAPTURE.COMPLETED" {
		t.Errorf("Type = %s, want PAYMENT.CAPTURE.COMPLETED", event.Type)
	}
	if event.Provider != "paypal" {
		t.Errorf("Provider = %s, want paypal", event.Provider)
	}

	// Verify raw is valid JSON
	var raw json.RawMessage
	if err := json.Unmarshal(event.Raw, &raw); err != nil {
		t.Errorf("invalid raw payload: %v", err)
	}

	// Invalid JSON
	_, err = ParseWebhook([]byte("invalid"))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestMapOrder(t *testing.T) {
	p := &Provider{}

	o := &order{
		ID:     "ORDER-123",
		Intent: "CAPTURE",
		Status: "COMPLETED",
		PurchaseUnits: []purchaseUnit{
			{
				Amount: amount{
					CurrencyCode: "USD",
					Value:        "19.99",
				},
				Description: "Test order",
				Payments: &payments{
					Captures: []capture{
						{
							ID:     "CAP-123",
							Status: "COMPLETED",
							Amount: amount{CurrencyCode: "USD", Value: "19.99"},
						},
					},
				},
			},
		},
		Links: []link{
			{Href: "https://example.com/approve", Rel: "approve", Method: "GET"},
		},
	}

	pay, err := p.mapOrder(o)
	if err != nil {
		t.Fatalf("mapOrder: %v", err)
	}

	if pay.ID != "ORDER-123" {
		t.Errorf("ID = %s, want ORDER-123", pay.ID)
	}
	if pay.Status != gopay.PaymentStatusSucceeded {
		t.Errorf("Status = %s, want succeeded", pay.Status)
	}
	if pay.Amount.Value != 1999 {
		t.Errorf("Amount = %d, want 1999", pay.Amount.Value)
	}
	if pay.Amount.Currency != "USD" {
		t.Errorf("Currency = %s, want USD", pay.Amount.Currency)
	}
	if pay.Description != "Test order" {
		t.Errorf("Description = %s, want Test order", pay.Description)
	}
	if pay.Provider != "paypal" {
		t.Errorf("Provider = %s, want paypal", pay.Provider)
	}
	if pay.RedirectURL != "https://example.com/approve" {
		t.Errorf("RedirectURL = %s, want approve URL", pay.RedirectURL)
	}
	if pay.AmountCaptured != 1999 {
		t.Errorf("AmountCaptured = %d, want 1999", pay.AmountCaptured)
	}
	if pay.Raw["capture_id"] != "CAP-123" {
		t.Errorf("Raw[capture_id] = %v, want CAP-123", pay.Raw["capture_id"])
	}
	if pay.Raw["intent"] != "CAPTURE" {
		t.Errorf("Raw[intent] = %v, want CAPTURE", pay.Raw["intent"])
	}
}

func TestMapOrderWithAuthorization(t *testing.T) {
	p := &Provider{}

	o := &order{
		ID:     "ORDER-456",
		Intent: "AUTHORIZE",
		Status: "APPROVED",
		PurchaseUnits: []purchaseUnit{
			{
				Amount: amount{CurrencyCode: "EUR", Value: "50.00"},
				Payments: &payments{
					Authorizations: []authorization{
						{ID: "AUTH-789", Status: "CREATED"},
					},
				},
			},
		},
	}

	pay, err := p.mapOrder(o)
	if err != nil {
		t.Fatalf("mapOrder: %v", err)
	}

	if pay.Status != gopay.PaymentStatusRequiresCapture {
		t.Errorf("Status = %s, want requires_capture", pay.Status)
	}
	if pay.Raw["authorization_id"] != "AUTH-789" {
		t.Errorf("Raw[authorization_id] = %v, want AUTH-789", pay.Raw["authorization_id"])
	}
}

func TestMapOrderMinimal(t *testing.T) {
	p := &Provider{}

	o := &order{
		ID:     "ORDER-EMPTY",
		Status: "CREATED",
	}

	pay, err := p.mapOrder(o)
	if err != nil {
		t.Fatalf("mapOrder: %v", err)
	}

	if pay.ID != "ORDER-EMPTY" {
		t.Errorf("ID = %s, want ORDER-EMPTY", pay.ID)
	}
	if pay.Status != gopay.PaymentStatusPending {
		t.Errorf("Status = %s, want pending", pay.Status)
	}
	if pay.RedirectURL != "" {
		t.Errorf("RedirectURL = %s, want empty", pay.RedirectURL)
	}
}

func TestMapRefund(t *testing.T) {
	p := &Provider{}

	r := &refund{
		ID:     "REFUND-123",
		Status: "COMPLETED",
		Amount: amount{CurrencyCode: "USD", Value: "10.00"},
	}

	ref, err := p.mapRefund(r, "ORDER-123")
	if err != nil {
		t.Fatalf("mapRefund: %v", err)
	}

	if ref.ID != "REFUND-123" {
		t.Errorf("ID = %s, want REFUND-123", ref.ID)
	}
	if ref.PaymentID != "ORDER-123" {
		t.Errorf("PaymentID = %s, want ORDER-123", ref.PaymentID)
	}
	if ref.Amount.Value != 1000 {
		t.Errorf("Amount = %d, want 1000", ref.Amount.Value)
	}
	if ref.Status != gopay.RefundStatusSucceeded {
		t.Errorf("Status = %s, want succeeded", ref.Status)
	}
	if ref.Provider != "paypal" {
		t.Errorf("Provider = %s, want paypal", ref.Provider)
	}
}

// --- HTTP-level tests using httptest ---

const tokenResponse = `{"access_token":"test_token","expires_in":3600}`

func newTestProvider(t *testing.T, handler http.HandlerFunc) *Provider {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/oauth2/token" {
			w.Header().Set("Content-Type", "application/json")
			if _, err := w.Write([]byte(tokenResponse)); err != nil {
				t.Errorf("failed to write token response: %v", err)
			}
			return
		}
		handler(w, r)
	}))
	t.Cleanup(srv.Close)

	p, err := NewProvider(Config{
		ClientID:     "test_id",
		ClientSecret: "test_secret",
		BaseURL:      srv.URL,
	})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	return p
}

func TestCreatePaymentHTTP(t *testing.T) {
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v2/checkout/orders" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}

		if auth := r.Header.Get("Authorization"); auth != "Bearer test_token" {
			t.Errorf("Authorization = %s, want Bearer test_token", auth)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %s, want application/json", ct)
		}
		if idem := r.Header.Get("PayPal-Request-Id"); idem != "idem_123" {
			t.Errorf("PayPal-Request-Id = %s, want idem_123", idem)
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed to read request body: %v", err)
		}
		if !strings.Contains(string(body), `"intent":"CAPTURE"`) {
			t.Errorf("body missing intent CAPTURE: %s", body)
		}
		if !strings.Contains(string(body), `"value":"19.99"`) {
			t.Errorf("body missing value 19.99: %s", body)
		}

		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(`{
			"id": "ORDER-001",
			"intent": "CAPTURE",
			"status": "CREATED",
			"purchase_units": [{"amount": {"currency_code": "USD", "value": "19.99"}, "description": "Test"}],
			"links": [{"href": "https://paypal.com/approve", "rel": "approve", "method": "GET"}],
			"create_time": "2023-11-14T22:13:20Z"
		}`)); err != nil {
			t.Fatalf("failed to write response: %v", err)
		}
	})

	ctx := context.Background()
	req := gopay.NewPaymentRequest(gopay.USD(1999)).
		WithDescription("Test").
		WithIdempotencyKey("idem_123")

	pay, err := p.CreatePayment(ctx, req)
	if err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}
	if pay.ID != "ORDER-001" {
		t.Errorf("ID = %s, want ORDER-001", pay.ID)
	}
	if pay.Status != gopay.PaymentStatusPending {
		t.Errorf("Status = %s, want pending", pay.Status)
	}
	if pay.Amount.Value != 1999 {
		t.Errorf("Amount = %d, want 1999", pay.Amount.Value)
	}
	if pay.RedirectURL != "https://paypal.com/approve" {
		t.Errorf("RedirectURL = %s, want approve URL", pay.RedirectURL)
	}
}

func TestCreatePaymentManualCapture(t *testing.T) {
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed to read request body: %v", err)
		}
		if !strings.Contains(string(body), `"intent":"AUTHORIZE"`) {
			t.Errorf("expected AUTHORIZE intent, got: %s", body)
		}

		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(`{
			"id": "ORDER-002",
			"intent": "AUTHORIZE",
			"status": "CREATED",
			"purchase_units": [{"amount": {"currency_code": "USD", "value": "50.00"}}],
			"create_time": "2023-11-14T22:13:20Z"
		}`)); err != nil {
			t.Fatalf("failed to write response: %v", err)
		}
	})

	ctx := context.Background()
	req := gopay.NewPaymentRequest(gopay.USD(5000)).
		WithCaptureMethod(gopay.CaptureManual)

	pay, err := p.CreatePayment(ctx, req)
	if err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}
	if pay.Raw["intent"] != "AUTHORIZE" {
		t.Errorf("intent = %v, want AUTHORIZE", pay.Raw["intent"])
	}
}

func TestGetPaymentHTTP(t *testing.T) {
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v2/checkout/orders/ORDER-001" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id": "ORDER-001",
			"intent": "CAPTURE",
			"status": "COMPLETED",
			"purchase_units": [{"amount": {"currency_code": "USD", "value": "19.99"}, "payments": {"captures": [{"id": "CAP-001", "status": "COMPLETED", "amount": {"currency_code": "USD", "value": "19.99"}}]}}],
			"create_time": "2023-11-14T22:13:20Z"
		}`))
	})

	pay, err := p.GetPayment(context.Background(), "ORDER-001")
	if err != nil {
		t.Fatalf("GetPayment: %v", err)
	}
	if pay.ID != "ORDER-001" {
		t.Errorf("ID = %s, want ORDER-001", pay.ID)
	}
	if pay.Status != gopay.PaymentStatusSucceeded {
		t.Errorf("Status = %s, want succeeded", pay.Status)
	}
	if pay.AmountCaptured != 1999 {
		t.Errorf("AmountCaptured = %d, want 1999", pay.AmountCaptured)
	}
}

func TestGetPaymentNotFoundHTTP(t *testing.T) {
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"name":"RESOURCE_NOT_FOUND","message":"not found"}`))
	})

	_, err := p.GetPayment(context.Background(), "ORDER-INVALID")
	if !errors.Is(err, gopay.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestGetRefundHTTP(t *testing.T) {
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v2/payments/refunds/REFUND-001" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id": "REFUND-001",
			"status": "COMPLETED",
			"amount": {"currency_code": "USD", "value": "5.00"},
			"create_time": "2023-11-14T22:13:20Z"
		}`))
	})

	ref, err := p.GetRefund(context.Background(), "REFUND-001")
	if err != nil {
		t.Fatalf("GetRefund: %v", err)
	}
	if ref.ID != "REFUND-001" {
		t.Errorf("ID = %s, want REFUND-001", ref.ID)
	}
	if ref.Amount.Value != 500 {
		t.Errorf("Amount = %d, want 500", ref.Amount.Value)
	}
	if ref.Status != gopay.RefundStatusSucceeded {
		t.Errorf("Status = %s, want succeeded", ref.Status)
	}
}

func TestRefundHTTP(t *testing.T) {
	callCount := 0
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")

		// First call: GetPayment (to find capture_id)
		if r.Method == http.MethodGet && r.URL.Path == "/v2/checkout/orders/ORDER-001" {
			_, _ = w.Write([]byte(`{
				"id": "ORDER-001",
				"intent": "CAPTURE",
				"status": "COMPLETED",
				"purchase_units": [{"amount": {"currency_code": "USD", "value": "19.99"}, "payments": {"captures": [{"id": "CAP-001", "status": "COMPLETED", "amount": {"currency_code": "USD", "value": "19.99"}}]}}],
				"create_time": "2023-11-14T22:13:20Z"
			}`))
			return
		}

		// Second call: actual refund
		if r.Method == http.MethodPost && r.URL.Path == "/v2/payments/captures/CAP-001/refund" {
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), `"value":"5.00"`) {
				t.Errorf("refund body missing amount: %s", body)
			}

			_, _ = w.Write([]byte(`{
				"id": "REFUND-001",
				"status": "COMPLETED",
				"amount": {"currency_code": "USD", "value": "5.00"},
				"create_time": "2023-11-14T22:13:20Z"
			}`))
			return
		}

		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	})

	req := gopay.NewRefundRequest("ORDER-001").
		WithAmount(gopay.USD(500))

	ref, err := p.Refund(context.Background(), req)
	if err != nil {
		t.Fatalf("Refund: %v", err)
	}
	if ref.ID != "REFUND-001" {
		t.Errorf("ID = %s, want REFUND-001", ref.ID)
	}
	if ref.Amount.Value != 500 {
		t.Errorf("Amount = %d, want 500", ref.Amount.Value)
	}
}

func TestCreatePaymentServerError(t *testing.T) {
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"name":"INTERNAL_SERVER_ERROR","message":"server error"}`))
	})

	req := gopay.NewPaymentRequest(gopay.USD(1999))
	_, err := p.CreatePayment(context.Background(), req)
	if !errors.Is(err, gopay.ErrProviderUnavailable) || !gopay.IsRetryable(err) {
		t.Errorf("expected retryable ErrProviderUnavailable, got %v", err)
	}
	// Refined kinds keep matching the broader sentinel callers used before.
	if !errors.Is(err, gopay.ErrProviderError) {
		t.Errorf("expected ErrProviderError, got %v", err)
	}
}

func TestCreatePaymentTransportError(t *testing.T) {
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		// Drop the connection without a response.
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Fatal("response writer does not support hijacking")
		}
		conn, _, err := hj.Hijack()
		if err != nil {
			t.Fatalf("hijack: %v", err)
		}
		_ = conn.Close()
	})

	_, err := p.CreatePayment(context.Background(), gopay.NewPaymentRequest(gopay.USD(1999)))
	if !errors.Is(err, gopay.ErrProviderUnavailable) || !gopay.IsRetryable(err) {
		t.Errorf("expected retryable ErrProviderUnavailable, got %v", err)
	}
}

func TestCreatePaymentCanceledContext(t *testing.T) {
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("request should not reach the server")
	})
	// Seed the token cache so the cancellation hits the order request itself.
	p.accessToken, p.tokenExpiry = "test_token", time.Now().Add(time.Hour)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := p.CreatePayment(ctx, gopay.NewPaymentRequest(gopay.USD(1999)))
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
	if gopay.IsRetryable(err) {
		t.Errorf("a canceled call must not be retryable: %v", err)
	}
}

func TestAccessTokenInvalidCredentials(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid_client","error_description":"Client Authentication failed"}`))
	}))
	t.Cleanup(srv.Close)

	p, err := NewProvider(Config{ClientID: "bad", ClientSecret: "bad", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}

	_, err = p.CreatePayment(context.Background(), gopay.NewPaymentRequest(gopay.USD(1999)))
	if !errors.Is(err, gopay.ErrInvalidCredentials) {
		t.Errorf("expected ErrInvalidCredentials, got %v", err)
	}
	if gopay.IsRetryable(err) {
		t.Errorf("invalid credentials must not be retryable: %v", err)
	}
}

func TestVerifyWebhookMissingConfig(t *testing.T) {
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {})

	_, err := p.VerifyWebhook(context.Background(), []byte(`{}`), map[string]string{})
	if !errors.Is(err, gopay.ErrInvalidConfig) {
		t.Errorf("expected ErrInvalidConfig, got %v", err)
	}
}

// TestVerifyWebhookHeaderCasing ensures the verification headers are read
// case-insensitively. HTTP header names are case-insensitive and net/http
// canonicalizes them (e.g. "Paypal-Transmission-Id"), so a caller-supplied map
// keyed in canonical casing must still resolve to the verify request.
func TestVerifyWebhookHeaderCasing(t *testing.T) {
	const transmissionID = "txn-123"
	var forwarded map[string]any

	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/notifications/verify-webhook-signature" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&forwarded); err != nil {
			t.Errorf("decode verify request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"verification_status":"SUCCESS"}`))
	})
	p.config.WebhookID = "WH-TEST"

	// Canonical (net/http) casing, not the all-caps literals the code uses.
	headers := map[string]string{
		"Paypal-Auth-Algo":         "SHA256withRSA",
		"Paypal-Cert-Url":          "https://example.com/cert",
		"Paypal-Transmission-Id":   transmissionID,
		"Paypal-Transmission-Sig":  "sig",
		"Paypal-Transmission-Time": "2023-11-14T22:13:20Z",
	}
	payload := []byte(`{"id":"EVT-1","event_type":"PAYMENT.CAPTURE.COMPLETED","resource":{"id":"CAP-1"}}`)

	ev, err := p.VerifyWebhook(context.Background(), payload, headers)
	if err != nil {
		t.Fatalf("VerifyWebhook: %v", err)
	}
	if ev.Type != "PAYMENT.CAPTURE.COMPLETED" {
		t.Errorf("Type = %q, want PAYMENT.CAPTURE.COMPLETED", ev.Type)
	}

	// Every header the casing fix touches must be forwarded, so a regression in
	// any single lookup (not just transmission_id) is caught.
	for field, want := range map[string]string{
		"auth_algo":         "SHA256withRSA",
		"cert_url":          "https://example.com/cert",
		"transmission_id":   transmissionID,
		"transmission_sig":  "sig",
		"transmission_time": "2023-11-14T22:13:20Z",
	} {
		if got := forwarded[field]; got != want {
			t.Errorf("forwarded %s = %v, want %q", field, got, want)
		}
	}
}

// TestVerifyWebhookMissingHeaders ensures a missing required transmission header
// fails locally with ErrProviderError instead of calling out to PayPal.
func TestVerifyWebhookMissingHeaders(t *testing.T) {
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/notifications/verify-webhook-signature" {
			t.Errorf("verification endpoint should not be called with missing headers")
		}
	})
	p.config.WebhookID = "WH-TEST"

	// Complete set minus PAYPAL-TRANSMISSION-SIG.
	headers := map[string]string{
		"Paypal-Auth-Algo":         "SHA256withRSA",
		"Paypal-Cert-Url":          "https://example.com/cert",
		"Paypal-Transmission-Id":   "txn-123",
		"Paypal-Transmission-Time": "2023-11-14T22:13:20Z",
	}

	_, err := p.VerifyWebhook(context.Background(), []byte(`{}`), headers)
	if !errors.Is(err, gopay.ErrProviderError) {
		t.Errorf("expected ErrProviderError, got %v", err)
	}
}

// TestFromDecimalOverflow ensures an amount that would overflow int64 when
// scaled to minor units errors instead of silently wrapping.
func TestFromDecimalOverflow(t *testing.T) {
	cases := []string{
		// One above math.MaxInt64/100 (92233720368547758): overflows on *100.
		"92233720368547759.00",
		// At the *100 limit but the fraction pushes base+frac past MaxInt64.
		"92233720368547758.99",
		// Symmetric underflow on the negative path.
		"-92233720368547759.00",
		"-92233720368547758.99",
	}
	for _, c := range cases {
		if _, err := fromDecimal(c, "USD"); err == nil {
			t.Errorf("fromDecimal(%q): expected overflow error, got nil", c)
		}
	}
}

func TestCancelPaymentHTTP(t *testing.T) {
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.Method == http.MethodGet && r.URL.Path == "/v2/checkout/orders/ORDER-AUTH" {
			_, _ = w.Write([]byte(`{
				"id": "ORDER-AUTH",
				"intent": "AUTHORIZE",
				"status": "APPROVED",
				"purchase_units": [{"amount": {"currency_code": "USD", "value": "25.00"}, "payments": {"authorizations": [{"id": "AUTH-001", "status": "CREATED"}]}}],
				"create_time": "2023-11-14T22:13:20Z"
			}`))
			return
		}

		if r.Method == http.MethodPost && r.URL.Path == "/v2/payments/authorizations/AUTH-001/void" {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	})

	pay, err := p.CancelPayment(context.Background(), "ORDER-AUTH")
	if err != nil {
		t.Fatalf("CancelPayment: %v", err)
	}
	if pay.Status != gopay.PaymentStatusCanceled {
		t.Errorf("Status = %s, want canceled", pay.Status)
	}
}

func TestRefundErrorKind(t *testing.T) {
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet { // the capture lookup Refund does first
			_, _ = w.Write([]byte(`{"id":"ORDER-001","status":"COMPLETED","purchase_units":[{"amount":{"currency_code":"USD","value":"19.99"},"payments":{"captures":[{"id":"CAP-001","status":"COMPLETED","amount":{"currency_code":"USD","value":"19.99"}}]}}]}`))
			return
		}
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"name":"UNPROCESSABLE_ENTITY","details":[{"issue":"REFUND_AMOUNT_EXCEEDED","description":"too much"}]}`))
	})

	_, err := p.Refund(context.Background(), gopay.NewRefundRequest("ORDER-001"))
	if !errors.Is(err, gopay.ErrRefundFailed) || errors.Is(err, gopay.ErrPaymentFailed) {
		t.Errorf("Refund() = %v, want ErrRefundFailed only", err)
	}
}

func TestRefundErrorKeepsSpecificKind(t *testing.T) {
	err := refundError(&gopay.Error{Kind: gopay.ErrAlreadyRefunded})
	if !errors.Is(err, gopay.ErrAlreadyRefunded) {
		t.Errorf("refundError() = %v, want ErrAlreadyRefunded kept", err)
	}
}

func TestUnauthorizedDropsCachedToken(t *testing.T) {
	tokenFetches := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/oauth2/token" {
			tokenFetches++
			_, _ = w.Write([]byte(tokenResponse))
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"name":"AUTHENTICATION_FAILURE","message":"token revoked"}`))
	}))
	t.Cleanup(srv.Close)

	p, err := NewProvider(Config{ClientID: "id", ClientSecret: "secret", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}

	for range 2 {
		if _, err := p.GetPayment(context.Background(), "ORDER-1"); !errors.Is(err, gopay.ErrInvalidCredentials) {
			t.Fatalf("GetPayment() = %v, want ErrInvalidCredentials", err)
		}
	}
	if tokenFetches != 2 {
		t.Errorf("token fetches = %d, want 2: a 401 must drop the cached token", tokenFetches)
	}
}
