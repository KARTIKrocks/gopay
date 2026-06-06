package gopay

import (
	"context"
	"errors"
	"testing"
)

func TestAmountValidate(t *testing.T) {
	tests := []struct {
		name    string
		amount  *Amount
		wantErr error
	}{
		{"nil amount", nil, ErrInvalidAmount},
		{"negative value", NewAmount(-1, "USD"), ErrInvalidAmount},
		{"empty currency", NewAmount(100, ""), ErrInvalidCurrency},
		{"invalid currency", NewAmount(100, "XYZ"), ErrInvalidCurrency},
		{"valid USD", NewAmount(100, "USD"), nil},
		{"valid INR", NewAmount(100, "INR"), nil},
		{"zero amount", NewAmount(0, "USD"), nil},
		{"lowercase normalized", NewAmount(100, "usd"), nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.amount.Validate()
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("Validate() = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestAmountValidateNil(t *testing.T) {
	var a *Amount
	if err := a.Validate(); !errors.Is(err, ErrInvalidAmount) {
		t.Errorf("nil Amount.Validate() = %v, want ErrInvalidAmount", err)
	}
}

func TestCurrencyHelpers(t *testing.T) {
	tests := []struct {
		name     string
		amount   *Amount
		wantVal  int64
		wantCurr string
	}{
		{"USD", USD(1999), 1999, "USD"},
		{"EUR", EUR(500), 500, "EUR"},
		{"GBP", GBP(250), 250, "GBP"},
		{"INR", INR(10000), 10000, "INR"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.amount.Value != tt.wantVal {
				t.Errorf("Value = %d, want %d", tt.amount.Value, tt.wantVal)
			}
			if tt.amount.Currency != tt.wantCurr {
				t.Errorf("Currency = %s, want %s", tt.amount.Currency, tt.wantCurr)
			}
		})
	}
}

func TestPaymentRequestValidate(t *testing.T) {
	tests := []struct {
		name    string
		req     *PaymentRequest
		wantErr bool
	}{
		{"nil request", nil, true},
		{"nil amount", &PaymentRequest{}, true},
		{"valid", NewPaymentRequest(USD(100)), false},
		{"invalid currency", NewPaymentRequest(NewAmount(100, "BAD")), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRefundRequestValidate(t *testing.T) {
	tests := []struct {
		name    string
		req     *RefundRequest
		wantErr bool
	}{
		{"nil request", nil, true},
		{"empty payment ID", &RefundRequest{Metadata: map[string]string{}}, true},
		{"valid full refund", NewRefundRequest("pi_123"), false},
		{"valid partial refund", NewRefundRequest("pi_123").WithAmount(USD(50)), false},
		{"invalid partial amount", NewRefundRequest("pi_123").WithAmount(NewAmount(50, "BAD")), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestPaymentRequestBuilder(t *testing.T) {
	req := NewPaymentRequest(USD(1000)).
		WithDescription("test").
		WithCustomer("cus_123").
		WithPaymentMethod("pm_456").
		WithReturnURL("https://example.com").
		WithCaptureMethod(CaptureManual).
		WithMetadata("key", "value").
		WithIdempotencyKey("idem_123")

	if req.Amount.Value != 1000 {
		t.Error("unexpected amount")
	}
	if req.Description != "test" {
		t.Error("unexpected description")
	}
	if req.CustomerID != "cus_123" {
		t.Error("unexpected customer")
	}
	if req.PaymentMethodID != "pm_456" {
		t.Error("unexpected payment method")
	}
	if req.ReturnURL != "https://example.com" {
		t.Error("unexpected return URL")
	}
	if req.CaptureMethod != CaptureManual {
		t.Error("unexpected capture method")
	}
	if req.Metadata["key"] != "value" {
		t.Error("unexpected metadata")
	}
	if req.IdempotencyKey != "idem_123" {
		t.Error("unexpected idempotency key")
	}
}

func TestRefundRequestBuilder(t *testing.T) {
	req := NewRefundRequest("pi_123").
		WithAmount(USD(500)).
		WithReason(RefundReasonDuplicate).
		WithMetadata("key", "val").
		WithIdempotencyKey("idem_456")

	if req.PaymentID != "pi_123" {
		t.Error("unexpected payment ID")
	}
	if req.Amount.Value != 500 {
		t.Error("unexpected amount")
	}
	if req.Reason != RefundReasonDuplicate {
		t.Error("unexpected reason")
	}
	if req.Metadata["key"] != "val" {
		t.Error("unexpected metadata")
	}
	if req.IdempotencyKey != "idem_456" {
		t.Error("unexpected idempotency key")
	}
}

func TestCustomerRequestBuilder(t *testing.T) {
	req := NewCustomerRequest("test@example.com").
		WithName("John").
		WithPhone("+1234567890").
		WithDescription("VIP customer").
		WithMetadata("tier", "gold")

	if req.Email != "test@example.com" {
		t.Error("unexpected email")
	}
	if req.Name != "John" {
		t.Error("unexpected name")
	}
	if req.Phone != "+1234567890" {
		t.Error("unexpected phone")
	}
	if req.Description != "VIP customer" {
		t.Error("unexpected description")
	}
	if req.Metadata["tier"] != "gold" {
		t.Error("unexpected metadata")
	}
}

func TestStringMethods(t *testing.T) {
	if PaymentStatusSucceeded.String() != "succeeded" {
		t.Error("PaymentStatus.String() failed")
	}
	if RefundStatusPending.String() != "pending" {
		t.Error("RefundStatus.String() failed")
	}
	if CaptureManual.String() != "manual" {
		t.Error("CaptureMethod.String() failed")
	}
	if RefundReasonDuplicate.String() != "duplicate" {
		t.Error("RefundReason.String() failed")
	}
	if PaymentMethodCard.String() != "card" {
		t.Error("PaymentMethodType.String() failed")
	}
}

func TestPaymentHelpers(t *testing.T) {
	p := &Payment{Status: PaymentStatusSucceeded, AmountCaptured: 100}
	if !p.IsSuccessful() {
		t.Error("IsSuccessful should be true")
	}
	if !p.IsCaptured() {
		t.Error("IsCaptured should be true")
	}
	if p.RequiresAction() {
		t.Error("RequiresAction should be false")
	}

	p2 := &Payment{Status: PaymentStatusRequiresAction}
	if !p2.RequiresAction() {
		t.Error("RequiresAction should be true")
	}
	if p2.IsSuccessful() {
		t.Error("IsSuccessful should be false")
	}
}

func TestRefundHelpers(t *testing.T) {
	r := &Refund{Status: RefundStatusSucceeded}
	if !r.IsSuccessful() {
		t.Error("IsSuccessful should be true")
	}
	r2 := &Refund{Status: RefundStatusFailed}
	if r2.IsSuccessful() {
		t.Error("IsSuccessful should be false")
	}
}

func mustNewClient(t *testing.T, p Provider) *Client {
	t.Helper()
	c, err := NewClient(p)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	return c
}

// Client tests using MockProvider

func TestClientCreatePayment(t *testing.T) {
	mock := NewMockProvider()
	client := mustNewClient(t, mock)
	ctx := context.Background()

	// Nil request
	_, err := client.CreatePayment(ctx, nil)
	if err == nil {
		t.Error("expected error for nil request")
	}

	// Valid request
	req := NewPaymentRequest(USD(1000)).WithPaymentMethod("pm_test")
	payment, err := client.CreatePayment(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if payment.Amount.Value != 1000 {
		t.Errorf("amount = %d, want 1000", payment.Amount.Value)
	}
	if payment.Status != PaymentStatusSucceeded {
		t.Errorf("status = %s, want succeeded", payment.Status)
	}
	if payment.Provider != "mock" {
		t.Errorf("provider = %s, want mock", payment.Provider)
	}
}

func TestClientGetPayment(t *testing.T) {
	mock := NewMockProvider()
	client := mustNewClient(t, mock)
	ctx := context.Background()

	// Empty ID
	_, err := client.GetPayment(ctx, "")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}

	// Create then get
	req := NewPaymentRequest(USD(500)).WithPaymentMethod("pm_test")
	payment, _ := client.CreatePayment(ctx, req)

	got, err := client.GetPayment(ctx, payment.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != payment.ID {
		t.Errorf("ID = %s, want %s", got.ID, payment.ID)
	}
}

func TestClientCapturePayment(t *testing.T) {
	mock := NewMockProvider()
	client := mustNewClient(t, mock)
	ctx := context.Background()

	// Create manual capture payment
	req := NewPaymentRequest(USD(1000)).
		WithPaymentMethod("pm_test").
		WithCaptureMethod(CaptureManual)
	payment, _ := client.CreatePayment(ctx, req)

	if payment.Status != PaymentStatusRequiresCapture {
		t.Fatalf("status = %s, want requires_capture", payment.Status)
	}

	// Capture
	captured, err := client.CapturePayment(ctx, payment.ID, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured.Status != PaymentStatusSucceeded {
		t.Errorf("status = %s, want succeeded", captured.Status)
	}
	if captured.AmountCaptured != 1000 {
		t.Errorf("captured = %d, want 1000", captured.AmountCaptured)
	}
}

func TestClientCancelPayment(t *testing.T) {
	mock := NewMockProvider()
	client := mustNewClient(t, mock)
	ctx := context.Background()

	// Create a manual-capture payment (status = requires_capture, which is cancelable)
	req := NewPaymentRequest(USD(1000)).
		WithPaymentMethod("pm_test").
		WithCaptureMethod(CaptureManual)
	payment, _ := client.CreatePayment(ctx, req)

	canceled, err := client.CancelPayment(ctx, payment.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if canceled.Status != PaymentStatusCanceled {
		t.Errorf("status = %s, want canceled", canceled.Status)
	}

	// Canceling a succeeded payment should fail
	mock.Reset()
	req2 := NewPaymentRequest(USD(500)).WithPaymentMethod("pm_test")
	p2, _ := client.CreatePayment(ctx, req2)
	_, err = client.CancelPayment(ctx, p2.ID)
	if err == nil {
		t.Error("expected error canceling succeeded payment")
	}
}

func TestClientRefund(t *testing.T) {
	mock := NewMockProvider()
	client := mustNewClient(t, mock)
	ctx := context.Background()

	// Create payment
	req := NewPaymentRequest(USD(1000)).WithPaymentMethod("pm_test")
	payment, _ := client.CreatePayment(ctx, req)

	// Full refund
	refund, err := client.FullRefund(ctx, payment.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if refund.Amount.Value != 1000 {
		t.Errorf("refund amount = %d, want 1000", refund.Amount.Value)
	}
	if !refund.IsSuccessful() {
		t.Error("refund should be successful")
	}

	// Get refund
	got, err := client.GetRefund(ctx, refund.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != refund.ID {
		t.Errorf("refund ID = %s, want %s", got.ID, refund.ID)
	}
}

func TestClientPartialRefund(t *testing.T) {
	mock := NewMockProvider()
	client := mustNewClient(t, mock)
	ctx := context.Background()

	req := NewPaymentRequest(USD(1000)).WithPaymentMethod("pm_test")
	payment, _ := client.CreatePayment(ctx, req)

	refundReq := NewRefundRequest(payment.ID).
		WithAmount(USD(300)).
		WithReason(RefundReasonRequestedByCustomer)

	refund, err := client.Refund(ctx, refundReq)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if refund.Amount.Value != 300 {
		t.Errorf("refund amount = %d, want 300", refund.Amount.Value)
	}
}

func TestClientCustomer(t *testing.T) {
	mock := NewMockProvider()
	client := mustNewClient(t, mock)
	ctx := context.Background()

	// Create customer
	custReq := NewCustomerRequest("test@example.com").WithName("Test User")
	cust, err := client.CreateCustomer(ctx, custReq)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cust.Email != "test@example.com" {
		t.Errorf("email = %s, want test@example.com", cust.Email)
	}

	// Get customer
	got, err := client.GetCustomer(ctx, cust.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Name != "Test User" {
		t.Errorf("name = %s, want Test User", got.Name)
	}

	// Update customer
	updateReq := NewCustomerRequest("updated@example.com").WithName("Updated Name")
	updated, err := client.UpdateCustomer(ctx, cust.ID, updateReq)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.Name != "Updated Name" {
		t.Errorf("name = %s, want Updated Name", updated.Name)
	}

	// Delete customer
	err = client.DeleteCustomer(ctx, cust.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Get deleted customer
	_, err = client.GetCustomer(ctx, cust.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestClientPaymentMethods(t *testing.T) {
	mock := NewMockProvider()
	client := mustNewClient(t, mock)
	ctx := context.Background()

	// Create customer first
	cust, _ := client.CreateCustomer(ctx, NewCustomerRequest("test@example.com"))

	// Attach payment method
	err := mock.AttachPaymentMethod(ctx, cust.ID, "pm_test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// List payment methods
	methods, err := client.ListPaymentMethods(ctx, cust.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(methods) != 1 {
		t.Fatalf("methods count = %d, want 1", len(methods))
	}
	if methods[0].ID != "pm_test" {
		t.Errorf("method ID = %s, want pm_test", methods[0].ID)
	}
}

func TestClientProviderInfo(t *testing.T) {
	mock := NewMockProvider()
	client := mustNewClient(t, mock)

	if client.ProviderName() != "mock" {
		t.Errorf("provider name = %s, want mock", client.ProviderName())
	}
	if client.Provider() != mock {
		t.Error("provider mismatch")
	}
}

func TestClientVerifyWebhook(t *testing.T) {
	mock := NewMockProvider()
	client := mustNewClient(t, mock)
	ctx := context.Background()

	// Successful webhook verification
	payload := []byte(`{"id":"evt_123","type":"payment.completed"}`)
	event, err := client.VerifyWebhook(ctx, payload, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if event.ID != "evt_123" {
		t.Errorf("event ID = %s, want evt_123", event.ID)
	}
	if event.Type != "payment.completed" {
		t.Errorf("event Type = %s, want payment.completed", event.Type)
	}
	if event.Provider != "mock" {
		t.Errorf("event Provider = %s, want mock", event.Provider)
	}

	// Simulate webhook error
	mock.WithWebhookError(ErrProviderError)
	_, err = client.VerifyWebhook(ctx, payload, nil)
	if !errors.Is(err, ErrProviderError) {
		t.Errorf("expected ErrProviderError, got %v", err)
	}

	// Reset clears webhook error
	mock.Reset()
	event, err = client.VerifyWebhook(ctx, payload, nil)
	if err != nil {
		t.Fatalf("unexpected error after reset: %v", err)
	}
	if event.ID != "evt_123" {
		t.Errorf("event ID after reset = %s, want evt_123", event.ID)
	}

	// Invalid JSON
	_, err = client.VerifyWebhook(ctx, []byte("not json"), nil)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestMockProviderErrors(t *testing.T) {
	mock := NewMockProvider()
	ctx := context.Background()

	// Set create error
	testErr := errors.New("test error")
	mock.WithCreateError(testErr)

	_, err := mock.CreatePayment(ctx, NewPaymentRequest(USD(100)))
	if !errors.Is(err, testErr) {
		t.Errorf("expected test error, got %v", err)
	}

	// Reset and test capture error
	mock.Reset()
	mock.WithCaptureError(testErr)

	req := NewPaymentRequest(USD(100)).
		WithPaymentMethod("pm_test").
		WithCaptureMethod(CaptureManual)
	payment, _ := mock.CreatePayment(ctx, req)

	_, err = mock.CapturePayment(ctx, payment.ID, nil)
	if !errors.Is(err, testErr) {
		t.Errorf("expected test error, got %v", err)
	}

	// Test refund error
	mock.Reset()
	mock.WithRefundError(testErr)
	req2 := NewPaymentRequest(USD(100)).WithPaymentMethod("pm_test")
	p, _ := mock.CreatePayment(ctx, req2)

	_, err = mock.Refund(ctx, NewRefundRequest(p.ID))
	if !errors.Is(err, testErr) {
		t.Errorf("expected test error, got %v", err)
	}
}

func TestMockProviderAutoSettings(t *testing.T) {
	mock := NewMockProvider()
	ctx := context.Background()

	// Disable auto-succeed
	mock.WithAutoSucceed(false)
	req := NewPaymentRequest(USD(100)).WithPaymentMethod("pm_test")
	payment, _ := mock.CreatePayment(ctx, req)
	if payment.Status != PaymentStatusPending {
		t.Errorf("status = %s, want pending", payment.Status)
	}

	// Re-enable and test auto-capture
	mock.Reset()
	mock.WithAutoCapture(false)
	req2 := NewPaymentRequest(USD(100)).WithPaymentMethod("pm_test")
	payment2, _ := mock.CreatePayment(ctx, req2)
	if payment2.AmountCaptured != 0 {
		t.Errorf("captured = %d, want 0", payment2.AmountCaptured)
	}
}

func TestMockProviderSetters(t *testing.T) {
	mock := NewMockProvider()

	// Set payment manually
	payment := &Payment{ID: "pi_manual", Amount: USD(999)}
	mock.SetPayment(payment)

	payments := mock.Payments()
	if _, ok := payments["pi_manual"]; !ok {
		t.Error("manual payment not found")
	}

	// Set refund manually
	refund := &Refund{ID: "re_manual"}
	mock.SetRefund(refund)

	refunds := mock.Refunds()
	if _, ok := refunds["re_manual"]; !ok {
		t.Error("manual refund not found")
	}

	// Set customer manually
	cust := &Customer{ID: "cus_manual"}
	mock.SetCustomer(cust)

	customers := mock.Customers()
	if _, ok := customers["cus_manual"]; !ok {
		t.Error("manual customer not found")
	}
}

func TestNewClientErrorOnNil(t *testing.T) {
	_, err := NewClient(nil)
	if err == nil {
		t.Error("NewClient(nil) should return error")
	}
}

func TestFullRefundEmptyPaymentID(t *testing.T) {
	mock := NewMockProvider()
	client := mustNewClient(t, mock)
	ctx := context.Background()

	_, err := client.FullRefund(ctx, "")
	if err == nil {
		t.Error("FullRefund with empty paymentID should return error")
	}
}

func TestCustomerRequestValidate(t *testing.T) {
	tests := []struct {
		name    string
		req     *CustomerRequest
		wantErr bool
	}{
		{"nil request", nil, true},
		{"empty email", &CustomerRequest{}, true},
		{"valid", NewCustomerRequest("test@example.com"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestPaymentRequestInvalidCaptureMethod(t *testing.T) {
	req := NewPaymentRequest(USD(100))
	req.CaptureMethod = "invalid"
	if err := req.Validate(); err == nil {
		t.Error("expected error for invalid capture method")
	}
}

func TestWithMetadataNilMap(t *testing.T) {
	// PaymentRequest without constructor
	pr := &PaymentRequest{Amount: USD(100)}
	pr.WithMetadata("key", "val")
	if pr.Metadata["key"] != "val" {
		t.Error("PaymentRequest.WithMetadata should init nil map")
	}

	// RefundRequest without constructor
	rr := &RefundRequest{PaymentID: "pi_123"}
	rr.WithMetadata("key", "val")
	if rr.Metadata["key"] != "val" {
		t.Error("RefundRequest.WithMetadata should init nil map")
	}

	// CustomerRequest without constructor
	cr := &CustomerRequest{Email: "a@b.com"}
	cr.WithMetadata("key", "val")
	if cr.Metadata["key"] != "val" {
		t.Error("CustomerRequest.WithMetadata should init nil map")
	}
}

func TestAmountValidateAcceptsLowercase(t *testing.T) {
	a := &Amount{Value: 100, Currency: "usd"}
	if err := a.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Validate no longer mutates; NewAmount normalizes.
	if a.Currency != "usd" {
		t.Errorf("Currency = %s, want usd (unmutated)", a.Currency)
	}
	// NewAmount should normalize.
	b := NewAmount(100, "usd")
	if b.Currency != "USD" {
		t.Errorf("NewAmount Currency = %s, want USD", b.Currency)
	}
}

func TestClientValidationGuards(t *testing.T) {
	mock := NewMockProvider()
	client := mustNewClient(t, mock)
	ctx := context.Background()

	// Empty paymentID
	_, err := client.GetPayment(ctx, "")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("GetPayment empty ID: expected ErrNotFound, got %v", err)
	}

	_, err = client.CapturePayment(ctx, "", nil)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("CapturePayment empty ID: expected ErrNotFound, got %v", err)
	}

	_, err = client.CancelPayment(ctx, "")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("CancelPayment empty ID: expected ErrNotFound, got %v", err)
	}

	_, err = client.GetRefund(ctx, "")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("GetRefund empty ID: expected ErrNotFound, got %v", err)
	}

	// Nil refund request
	_, err = client.Refund(ctx, nil)
	if err == nil {
		t.Error("Refund nil req: expected error")
	}
}
