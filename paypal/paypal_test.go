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
		got := fromDecimal(tt.decimal, tt.currency)
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
		back := fromDecimal(decimal, "USD")
		if back != v {
			t.Errorf("USD round trip failed: %d -> %q -> %d", v, decimal, back)
		}
	}

	// Zero-decimal currencies
	for _, v := range values {
		decimal := toDecimal(v, "JPY")
		back := fromDecimal(decimal, "JPY")
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
		name    string
		body    string
		wantErr error
	}{
		{
			"resource not found",
			`{"name":"RESOURCE_NOT_FOUND","message":"resource not found"}`,
			gopay.ErrNotFound,
		},
		{
			"invalid resource ID",
			`{"name":"INVALID_RESOURCE_ID","message":"invalid ID"}`,
			gopay.ErrNotFound,
		},
		{
			"unprocessable entity with details",
			`{"name":"UNPROCESSABLE_ENTITY","message":"error","details":[{"issue":"ISSUE","description":"detail desc"}]}`,
			gopay.ErrPaymentFailed,
		},
		{
			"unprocessable entity without details",
			`{"name":"UNPROCESSABLE_ENTITY","message":"generic error"}`,
			gopay.ErrPaymentFailed,
		},
		{
			"unknown error",
			`{"name":"INTERNAL_SERVER_ERROR","message":"server error"}`,
			gopay.ErrProviderError,
		},
		{
			"invalid JSON",
			`not json`,
			gopay.ErrProviderError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := p.parseError([]byte(tt.body))
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("parseError() = %v, want %v", err, tt.wantErr)
			}
		})
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

	pay := p.mapOrder(o)

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

	pay := p.mapOrder(o)

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

	pay := p.mapOrder(o)

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

	ref := p.mapRefund(r, "ORDER-123")

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
			w.Write([]byte(tokenResponse))
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
		if r.Method != "POST" || r.URL.Path != "/v2/checkout/orders" {
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

		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"intent":"CAPTURE"`) {
			t.Errorf("body missing intent CAPTURE: %s", body)
		}
		if !strings.Contains(string(body), `"value":"19.99"`) {
			t.Errorf("body missing value 19.99: %s", body)
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"id": "ORDER-001",
			"intent": "CAPTURE",
			"status": "CREATED",
			"purchase_units": [{"amount": {"currency_code": "USD", "value": "19.99"}, "description": "Test"}],
			"links": [{"href": "https://paypal.com/approve", "rel": "approve", "method": "GET"}],
			"create_time": "2023-11-14T22:13:20Z"
		}`))
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
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"intent":"AUTHORIZE"`) {
			t.Errorf("expected AUTHORIZE intent, got: %s", body)
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"id": "ORDER-002",
			"intent": "AUTHORIZE",
			"status": "CREATED",
			"purchase_units": [{"amount": {"currency_code": "USD", "value": "50.00"}}],
			"create_time": "2023-11-14T22:13:20Z"
		}`))
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
		if r.Method != "GET" || r.URL.Path != "/v2/checkout/orders/ORDER-001" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
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
		w.WriteHeader(404)
		w.Write([]byte(`{"name":"RESOURCE_NOT_FOUND","message":"not found"}`))
	})

	_, err := p.GetPayment(context.Background(), "ORDER-INVALID")
	if !errors.Is(err, gopay.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestGetRefundHTTP(t *testing.T) {
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/v2/payments/refunds/REFUND-001" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
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
		if r.Method == "GET" && r.URL.Path == "/v2/checkout/orders/ORDER-001" {
			w.Write([]byte(`{
				"id": "ORDER-001",
				"intent": "CAPTURE",
				"status": "COMPLETED",
				"purchase_units": [{"amount": {"currency_code": "USD", "value": "19.99"}, "payments": {"captures": [{"id": "CAP-001", "status": "COMPLETED", "amount": {"currency_code": "USD", "value": "19.99"}}]}}],
				"create_time": "2023-11-14T22:13:20Z"
			}`))
			return
		}

		// Second call: actual refund
		if r.Method == "POST" && r.URL.Path == "/v2/payments/captures/CAP-001/refund" {
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), `"value":"5.00"`) {
				t.Errorf("refund body missing amount: %s", body)
			}

			w.Write([]byte(`{
				"id": "REFUND-001",
				"status": "COMPLETED",
				"amount": {"currency_code": "USD", "value": "5.00"},
				"create_time": "2023-11-14T22:13:20Z"
			}`))
			return
		}

		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(500)
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
		w.WriteHeader(500)
		w.Write([]byte(`{"name":"INTERNAL_SERVER_ERROR","message":"server error"}`))
	})

	req := gopay.NewPaymentRequest(gopay.USD(1999))
	_, err := p.CreatePayment(context.Background(), req)
	if !errors.Is(err, gopay.ErrProviderError) {
		t.Errorf("expected ErrProviderError, got %v", err)
	}
}

func TestVerifyWebhookMissingConfig(t *testing.T) {
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {})

	_, err := p.VerifyWebhook(context.Background(), []byte(`{}`), map[string]string{})
	if !errors.Is(err, gopay.ErrInvalidConfig) {
		t.Errorf("expected ErrInvalidConfig, got %v", err)
	}
}

func TestCancelPaymentHTTP(t *testing.T) {
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.Method == "GET" && r.URL.Path == "/v2/checkout/orders/ORDER-AUTH" {
			w.Write([]byte(`{
				"id": "ORDER-AUTH",
				"intent": "AUTHORIZE",
				"status": "APPROVED",
				"purchase_units": [{"amount": {"currency_code": "USD", "value": "25.00"}, "payments": {"authorizations": [{"id": "AUTH-001", "status": "CREATED"}]}}],
				"create_time": "2023-11-14T22:13:20Z"
			}`))
			return
		}

		if r.Method == "POST" && r.URL.Path == "/v2/payments/authorizations/AUTH-001/void" {
			w.WriteHeader(204)
			return
		}

		t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(500)
	})

	pay, err := p.CancelPayment(context.Background(), "ORDER-AUTH")
	if err != nil {
		t.Fatalf("CancelPayment: %v", err)
	}
	if pay.Status != gopay.PaymentStatusCanceled {
		t.Errorf("Status = %s, want canceled", pay.Status)
	}
}
