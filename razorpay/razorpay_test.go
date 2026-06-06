package razorpay

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
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
	_, err = NewProvider(Config{KeyID: "key_id"})
	if !errors.Is(err, gopay.ErrInvalidConfig) {
		t.Errorf("expected ErrInvalidConfig for missing secret, got %v", err)
	}

	// Missing key ID
	_, err = NewProvider(Config{KeySecret: "secret"})
	if !errors.Is(err, gopay.ErrInvalidConfig) {
		t.Errorf("expected ErrInvalidConfig for missing key ID, got %v", err)
	}

	// Valid config
	p, err := NewProvider(Config{KeyID: "key_id", KeySecret: "secret"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name() != "razorpay" {
		t.Errorf("name = %s, want razorpay", p.Name())
	}
	if p.config.HTTPClient == nil {
		t.Error("HTTPClient should not be nil")
	}
	if p.config.BaseURL != baseURL {
		t.Errorf("BaseURL = %s, want %s", p.config.BaseURL, baseURL)
	}
}

func TestConfigBuilders(t *testing.T) {
	cfg := DefaultConfig().
		WithCredentials("key_id", "key_secret").
		WithWebhookSecret("whsec_123")

	if cfg.KeyID != "key_id" {
		t.Errorf("KeyID = %s, want key_id", cfg.KeyID)
	}
	if cfg.KeySecret != "key_secret" {
		t.Errorf("KeySecret = %s, want key_secret", cfg.KeySecret)
	}
	if cfg.WebhookSecret != "whsec_123" {
		t.Errorf("WebhookSecret = %s, want whsec_123", cfg.WebhookSecret)
	}
	if cfg.HTTPClient == nil {
		t.Error("HTTPClient should not be nil from DefaultConfig")
	}
	if cfg.BaseURL != baseURL {
		t.Errorf("BaseURL = %s, want %s", cfg.BaseURL, baseURL)
	}
}

func TestCancelPaymentUnsupported(t *testing.T) {
	p := &Provider{}
	_, err := p.CancelPayment(context.Background(), "pay_123")
	if !errors.Is(err, gopay.ErrUnsupported) {
		t.Errorf("expected ErrUnsupported, got %v", err)
	}
}

func TestDeleteCustomerUnsupported(t *testing.T) {
	p := &Provider{}
	err := p.DeleteCustomer(context.Background(), "cust_123")
	if !errors.Is(err, gopay.ErrUnsupported) {
		t.Errorf("expected ErrUnsupported, got %v", err)
	}
}

func TestMapOrderStatus(t *testing.T) {
	tests := []struct {
		input string
		want  gopay.PaymentStatus
	}{
		{"created", gopay.PaymentStatusPending},
		{"attempted", gopay.PaymentStatusProcessing},
		{"paid", gopay.PaymentStatusSucceeded},
		{"unknown", gopay.PaymentStatusPending},
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

func TestMapPaymentStatus(t *testing.T) {
	tests := []struct {
		input string
		want  gopay.PaymentStatus
	}{
		{"created", gopay.PaymentStatusPending},
		{"authorized", gopay.PaymentStatusRequiresCapture},
		{"captured", gopay.PaymentStatusSucceeded},
		{"refunded", gopay.PaymentStatusSucceeded},
		{"failed", gopay.PaymentStatusFailed},
		{"unknown", gopay.PaymentStatusPending},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := mapPaymentStatus(tt.input)
			if got != tt.want {
				t.Errorf("mapPaymentStatus(%s) = %s, want %s", tt.input, got, tt.want)
			}
		})
	}
}

func TestMapRefundStatus(t *testing.T) {
	tests := []struct {
		input string
		want  gopay.RefundStatus
	}{
		{"pending", gopay.RefundStatusPending},
		{"processed", gopay.RefundStatusSucceeded},
		{"failed", gopay.RefundStatusFailed},
		{"unknown", gopay.RefundStatusPending},
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
			"bad request - amount field",
			`{"error":{"code":"BAD_REQUEST_ERROR","description":"invalid amount","field":"amount"}}`,
			gopay.ErrInvalidAmount,
		},
		{
			"bad request - other field",
			`{"error":{"code":"BAD_REQUEST_ERROR","description":"invalid param","field":"currency"}}`,
			gopay.ErrPaymentFailed,
		},
		{
			"gateway error",
			`{"error":{"code":"GATEWAY_ERROR","description":"gateway declined"}}`,
			gopay.ErrCardDeclined,
		},
		{
			"server error",
			`{"error":{"code":"SERVER_ERROR","description":"internal error"}}`,
			gopay.ErrProviderError,
		},
		{
			"unknown error code",
			`{"error":{"code":"UNKNOWN_CODE","description":"something"}}`,
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

func TestMapOrder(t *testing.T) {
	p := &Provider{}

	o := &order{
		ID:        "order_123",
		Amount:    1999,
		Currency:  "INR",
		Status:    "paid",
		Receipt:   "receipt_1",
		Notes:     map[string]string{"key": "val"},
		CreatedAt: 1700000000,
	}

	pay := p.mapOrder(o)

	if pay.ID != "order_123" {
		t.Errorf("ID = %s, want order_123", pay.ID)
	}
	if pay.Amount.Value != 1999 {
		t.Errorf("Amount = %d, want 1999", pay.Amount.Value)
	}
	if pay.Amount.Currency != "INR" {
		t.Errorf("Currency = %s, want INR", pay.Amount.Currency)
	}
	if pay.Status != gopay.PaymentStatusSucceeded {
		t.Errorf("Status = %s, want succeeded", pay.Status)
	}
	if pay.Description != "receipt_1" {
		t.Errorf("Description = %s, want receipt_1", pay.Description)
	}
	if pay.Provider != "razorpay" {
		t.Errorf("Provider = %s, want razorpay", pay.Provider)
	}
	if pay.CreatedAt != time.Unix(1700000000, 0) {
		t.Errorf("CreatedAt = %v, want %v", pay.CreatedAt, time.Unix(1700000000, 0))
	}
	if pay.Metadata["key"] != "val" {
		t.Errorf("Metadata[key] = %s, want val", pay.Metadata["key"])
	}
}

func TestMapPayment(t *testing.T) {
	p := &Provider{}

	pay := &razorpayPayment{
		ID:               "pay_123",
		Amount:           5000,
		Currency:         "INR",
		Status:           "captured",
		Method:           "card",
		Description:      "Test payment",
		AmountRefunded:   1000,
		ErrorCode:        "",
		ErrorDescription: "",
		Notes:            map[string]string{"order": "42"},
		CreatedAt:        1700000000,
	}

	result := p.mapPayment(pay)

	if result.ID != "pay_123" {
		t.Errorf("ID = %s, want pay_123", result.ID)
	}
	if result.Amount.Value != 5000 {
		t.Errorf("Amount = %d, want 5000", result.Amount.Value)
	}
	if result.Status != gopay.PaymentStatusSucceeded {
		t.Errorf("Status = %s, want succeeded", result.Status)
	}
	if result.AmountCaptured != 5000 {
		t.Errorf("AmountCaptured = %d, want 5000", result.AmountCaptured)
	}
	if result.AmountRefunded != 1000 {
		t.Errorf("AmountRefunded = %d, want 1000", result.AmountRefunded)
	}
	if result.FailureCode != "" {
		t.Errorf("FailureCode = %s, want empty", result.FailureCode)
	}
	if result.Raw["method"] != "card" {
		t.Errorf("Raw[method] = %v, want card", result.Raw["method"])
	}
}

func TestMapPaymentWithError(t *testing.T) {
	p := &Provider{}

	pay := &razorpayPayment{
		ID:               "pay_fail",
		Amount:           1000,
		Currency:         "INR",
		Status:           "failed",
		ErrorCode:        "BAD_REQUEST_ERROR",
		ErrorDescription: "card declined",
		CreatedAt:        1700000000,
	}

	result := p.mapPayment(pay)

	if result.Status != gopay.PaymentStatusFailed {
		t.Errorf("Status = %s, want failed", result.Status)
	}
	if result.FailureCode != "BAD_REQUEST_ERROR" {
		t.Errorf("FailureCode = %s, want BAD_REQUEST_ERROR", result.FailureCode)
	}
	if result.FailureMessage != "card declined" {
		t.Errorf("FailureMessage = %s, want card declined", result.FailureMessage)
	}
	// Not captured, so AmountCaptured should be 0
	if result.AmountCaptured != 0 {
		t.Errorf("AmountCaptured = %d, want 0", result.AmountCaptured)
	}
}

func TestMapRefund(t *testing.T) {
	p := &Provider{}

	r := &razorpayRefund{
		ID:        "rfnd_123",
		PaymentID: "pay_123",
		Amount:    500,
		Currency:  "INR",
		Status:    "processed",
		CreatedAt: 1700000000,
	}

	ref := p.mapRefund(r)

	if ref.ID != "rfnd_123" {
		t.Errorf("ID = %s, want rfnd_123", ref.ID)
	}
	if ref.PaymentID != "pay_123" {
		t.Errorf("PaymentID = %s, want pay_123", ref.PaymentID)
	}
	if ref.Amount.Value != 500 {
		t.Errorf("Amount = %d, want 500", ref.Amount.Value)
	}
	if ref.Status != gopay.RefundStatusSucceeded {
		t.Errorf("Status = %s, want succeeded", ref.Status)
	}
	if ref.Provider != "razorpay" {
		t.Errorf("Provider = %s, want razorpay", ref.Provider)
	}
}

func TestMapCustomer(t *testing.T) {
	p := &Provider{}

	c := &customer{
		ID:        "cust_123",
		Name:      "Test User",
		Email:     "test@example.com",
		Contact:   "+911234567890",
		Notes:     map[string]string{"tier": "premium"},
		CreatedAt: 1700000000,
	}

	cust := p.mapCustomer(c)

	if cust.ID != "cust_123" {
		t.Errorf("ID = %s, want cust_123", cust.ID)
	}
	if cust.Name != "Test User" {
		t.Errorf("Name = %s, want Test User", cust.Name)
	}
	if cust.Email != "test@example.com" {
		t.Errorf("Email = %s, want test@example.com", cust.Email)
	}
	if cust.Phone != "+911234567890" {
		t.Errorf("Phone = %s, want +911234567890", cust.Phone)
	}
	if cust.Provider != "razorpay" {
		t.Errorf("Provider = %s, want razorpay", cust.Provider)
	}
	if cust.Metadata["tier"] != "premium" {
		t.Errorf("Metadata[tier] = %s, want premium", cust.Metadata["tier"])
	}
	if cust.CreatedAt != time.Unix(1700000000, 0) {
		t.Errorf("CreatedAt = %v, want %v", cust.CreatedAt, time.Unix(1700000000, 0))
	}
}

func TestVerifyWebhook(t *testing.T) {
	provider := &Provider{
		config: Config{
			WebhookSecret: "test_secret_123",
		},
	}

	payload := []byte(`{"event":"payment.captured","account_id":"acc_123","payload":{"payment":{"id":"pay_123"}}}`)

	// Generate valid signature
	mac := hmac.New(sha256.New, []byte("test_secret_123"))
	mac.Write(payload)
	validSig := hex.EncodeToString(mac.Sum(nil))

	// Valid signature
	event, err := provider.VerifyWebhook(context.Background(), payload, map[string]string{
		"X-Razorpay-Signature": validSig,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if event.Type != "payment.captured" {
		t.Errorf("type = %s, want payment.captured", event.Type)
	}
	if event.Provider != "razorpay" {
		t.Errorf("provider = %s, want razorpay", event.Provider)
	}
	if event.ID != "acc_123:payment.captured" {
		t.Errorf("ID = %s, want acc_123:payment.captured", event.ID)
	}

	// Invalid signature
	_, err = provider.VerifyWebhook(context.Background(), payload, map[string]string{
		"X-Razorpay-Signature": "invalid_signature",
	})
	if err == nil {
		t.Error("expected error for invalid signature")
	}
	if !errors.Is(err, gopay.ErrProviderError) {
		t.Errorf("expected ErrProviderError for invalid sig, got %v", err)
	}

	// Missing signature header
	_, err = provider.VerifyWebhook(context.Background(), payload, map[string]string{})
	if err == nil {
		t.Error("expected error for missing signature")
	}
	if !errors.Is(err, gopay.ErrProviderError) {
		t.Errorf("expected ErrProviderError for missing sig, got %v", err)
	}

	// Missing webhook secret
	noSecret := &Provider{config: Config{}}
	_, err = noSecret.VerifyWebhook(context.Background(), payload, map[string]string{
		"X-Razorpay-Signature": validSig,
	})
	if err == nil {
		t.Error("expected error for missing webhook secret")
	}
	if !errors.Is(err, gopay.ErrInvalidConfig) {
		t.Errorf("expected ErrInvalidConfig for missing secret, got %v", err)
	}
}

func TestParseWebhook(t *testing.T) {
	payload := []byte(`{"event":"payment.authorized","account_id":"acc_456","payload":{"payment":{"id":"pay_456"}}}`)

	event, err := ParseWebhook(payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if event.Type != "payment.authorized" {
		t.Errorf("type = %s, want payment.authorized", event.Type)
	}
	if event.ID != "acc_456:payment.authorized" {
		t.Errorf("ID = %s, want acc_456:payment.authorized", event.ID)
	}

	// Verify raw payload is valid JSON
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

// --- HTTP-level tests using httptest ---

func newTestProvider(t *testing.T, handler http.HandlerFunc) *Provider {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	p, err := NewProvider(Config{
		KeyID:     "key_test",
		KeySecret: "secret_test",
		BaseURL:   srv.URL,
	})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	return p
}

func TestCreatePaymentHTTP(t *testing.T) {
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/orders" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}

		user, pass, ok := r.BasicAuth()
		if !ok || user != "key_test" || pass != "secret_test" {
			t.Errorf("BasicAuth = (%s, %s, %v)", user, pass, ok)
		}

		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"amount":1999`) {
			t.Errorf("body missing amount: %s", body)
		}
		if !strings.Contains(string(body), `"currency":"INR"`) {
			t.Errorf("body missing currency: %s", body)
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"order_001","amount":1999,"currency":"INR","status":"created","receipt":"","notes":{},"created_at":1700000000}`))
	})

	ctx := context.Background()
	req := gopay.NewPaymentRequest(gopay.INR(1999))

	pay, err := p.CreatePayment(ctx, req)
	if err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}
	if pay.ID != "order_001" {
		t.Errorf("ID = %s, want order_001", pay.ID)
	}
	if pay.Status != gopay.PaymentStatusPending {
		t.Errorf("Status = %s, want pending", pay.Status)
	}
	if pay.Amount.Value != 1999 {
		t.Errorf("Amount = %d, want 1999", pay.Amount.Value)
	}
}

func TestGetPaymentHTTP(t *testing.T) {
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/orders/order_001" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"order_001","amount":5000,"currency":"INR","status":"paid","receipt":"rcpt_1","notes":{"key":"val"},"created_at":1700000000}`))
	})

	pay, err := p.GetPayment(context.Background(), "order_001")
	if err != nil {
		t.Fatalf("GetPayment: %v", err)
	}
	if pay.ID != "order_001" {
		t.Errorf("ID = %s, want order_001", pay.ID)
	}
	if pay.Status != gopay.PaymentStatusSucceeded {
		t.Errorf("Status = %s, want succeeded", pay.Status)
	}
	if pay.Amount.Value != 5000 {
		t.Errorf("Amount = %d, want 5000", pay.Amount.Value)
	}
}

func TestGetPaymentFallbackHTTP(t *testing.T) {
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		// Order endpoint returns 404
		if r.URL.Path == "/orders/pay_001" {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"error":{"code":"BAD_REQUEST_ERROR","description":"not found"}}`))
			return
		}
		// Falls back to payment endpoint
		if r.URL.Path == "/payments/pay_001" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"id":"pay_001","amount":3000,"currency":"INR","status":"captured","method":"upi","description":"Test","amount_refunded":0,"error_code":"","error_description":"","notes":{},"created_at":1700000000}`))
			return
		}

		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(500)
	})

	pay, err := p.GetPayment(context.Background(), "pay_001")
	if err != nil {
		t.Fatalf("GetPayment fallback: %v", err)
	}
	if pay.ID != "pay_001" {
		t.Errorf("ID = %s, want pay_001", pay.ID)
	}
	if pay.Status != gopay.PaymentStatusSucceeded {
		t.Errorf("Status = %s, want succeeded", pay.Status)
	}
}

func TestGetPaymentNotFoundHTTP(t *testing.T) {
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":{"code":"BAD_REQUEST_ERROR","description":"not found"}}`))
	})

	// The first call returns 404 (triggers fallback), fallback also 404
	_, err := p.GetPayment(context.Background(), "invalid_id")
	if err == nil {
		t.Error("expected error for not found")
	}
}

func TestCapturePaymentHTTP(t *testing.T) {
	callCount := 0
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")

		// First: fetches payment to get amount (when amt provided, skips this)
		if r.Method == "POST" && r.URL.Path == "/payments/pay_001/capture" {
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), `"amount":5000`) {
				t.Errorf("capture body missing amount: %s", body)
			}
			w.Write([]byte(`{"id":"pay_001","amount":5000,"currency":"INR","status":"captured","method":"card","description":"","amount_refunded":0,"error_code":"","error_description":"","notes":{},"created_at":1700000000}`))
			return
		}

		t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(500)
	})

	pay, err := p.CapturePayment(context.Background(), "pay_001", gopay.INR(5000))
	if err != nil {
		t.Fatalf("CapturePayment: %v", err)
	}
	if pay.Status != gopay.PaymentStatusSucceeded {
		t.Errorf("Status = %s, want succeeded", pay.Status)
	}
	if pay.AmountCaptured != 5000 {
		t.Errorf("AmountCaptured = %d, want 5000", pay.AmountCaptured)
	}
}

func TestRefundHTTP(t *testing.T) {
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/payments/pay_001/refund" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}

		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"amount":500`) {
			t.Errorf("refund body missing amount: %s", body)
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"rfnd_001","payment_id":"pay_001","amount":500,"currency":"INR","status":"processed","created_at":1700000000}`))
	})

	req := gopay.NewRefundRequest("pay_001").
		WithAmount(gopay.INR(500))

	ref, err := p.Refund(context.Background(), req)
	if err != nil {
		t.Fatalf("Refund: %v", err)
	}
	if ref.ID != "rfnd_001" {
		t.Errorf("ID = %s, want rfnd_001", ref.ID)
	}
	if ref.Amount.Value != 500 {
		t.Errorf("Amount = %d, want 500", ref.Amount.Value)
	}
	if ref.Status != gopay.RefundStatusSucceeded {
		t.Errorf("Status = %s, want succeeded", ref.Status)
	}
}

func TestGetRefundHTTP(t *testing.T) {
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/refunds/rfnd_001" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"rfnd_001","payment_id":"pay_001","amount":500,"currency":"INR","status":"processed","created_at":1700000000}`))
	})

	ref, err := p.GetRefund(context.Background(), "rfnd_001")
	if err != nil {
		t.Fatalf("GetRefund: %v", err)
	}
	if ref.ID != "rfnd_001" {
		t.Errorf("ID = %s, want rfnd_001", ref.ID)
	}
}

func TestCreateCustomerHTTP(t *testing.T) {
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/customers" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}

		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"email":"test@example.com"`) {
			t.Errorf("body missing email: %s", body)
		}
		if !strings.Contains(string(body), `"name":"Test User"`) {
			t.Errorf("body missing name: %s", body)
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"cust_001","name":"Test User","email":"test@example.com","contact":"+91123","notes":{},"created_at":1700000000}`))
	})

	req := gopay.NewCustomerRequest("test@example.com").
		WithName("Test User").
		WithPhone("+91123")

	cust, err := p.CreateCustomer(context.Background(), req)
	if err != nil {
		t.Fatalf("CreateCustomer: %v", err)
	}
	if cust.ID != "cust_001" {
		t.Errorf("ID = %s, want cust_001", cust.ID)
	}
	if cust.Email != "test@example.com" {
		t.Errorf("Email = %s, want test@example.com", cust.Email)
	}
}

func TestGetCustomerHTTP(t *testing.T) {
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/customers/cust_001" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"cust_001","name":"Test User","email":"test@example.com","contact":"+91123","notes":{},"created_at":1700000000}`))
	})

	cust, err := p.GetCustomer(context.Background(), "cust_001")
	if err != nil {
		t.Fatalf("GetCustomer: %v", err)
	}
	if cust.ID != "cust_001" {
		t.Errorf("ID = %s, want cust_001", cust.ID)
	}
}

func TestUpdateCustomerHTTP(t *testing.T) {
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PUT" || r.URL.Path != "/customers/cust_001" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"cust_001","name":"Updated","email":"new@example.com","contact":"+91456","notes":{},"created_at":1700000000}`))
	})

	req := gopay.NewCustomerRequest("new@example.com").WithName("Updated")
	cust, err := p.UpdateCustomer(context.Background(), "cust_001", req)
	if err != nil {
		t.Fatalf("UpdateCustomer: %v", err)
	}
	if cust.Name != "Updated" {
		t.Errorf("Name = %s, want Updated", cust.Name)
	}
}

func TestCreatePaymentServerErrorHTTP(t *testing.T) {
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte(`{"error":{"code":"SERVER_ERROR","description":"internal error"}}`))
	})

	req := gopay.NewPaymentRequest(gopay.INR(1999))
	_, err := p.CreatePayment(context.Background(), req)
	if !errors.Is(err, gopay.ErrProviderError) {
		t.Errorf("expected ErrProviderError, got %v", err)
	}
}
