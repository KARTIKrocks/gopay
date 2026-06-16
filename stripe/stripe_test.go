package stripe

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/KARTIKrocks/gopay"
	"github.com/stripe/stripe-go/v81"
)

func TestNewProviderValidation(t *testing.T) {
	_, err := NewProvider(Config{})
	if !errors.Is(err, gopay.ErrInvalidConfig) {
		t.Errorf("expected ErrInvalidConfig, got %v", err)
	}

	p, err := NewProvider(Config{SecretKey: "sk_test_123"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name() != "stripe" {
		t.Errorf("name = %s, want stripe", p.Name())
	}
}

func TestConfigBuilders(t *testing.T) {
	cfg := DefaultConfig().
		WithSecretKey("sk_test_123").
		WithWebhookSecret("whsec_123")

	if cfg.SecretKey != "sk_test_123" {
		t.Error("unexpected secret key")
	}
	if cfg.WebhookSecret != "whsec_123" {
		t.Error("unexpected webhook secret")
	}
}

func TestMapPaymentStatus(t *testing.T) {
	p := &Provider{}
	tests := []struct {
		input stripe.PaymentIntentStatus
		want  gopay.PaymentStatus
	}{
		{stripe.PaymentIntentStatusRequiresPaymentMethod, gopay.PaymentStatusPending},
		{stripe.PaymentIntentStatusRequiresConfirmation, gopay.PaymentStatusPending},
		{stripe.PaymentIntentStatusRequiresAction, gopay.PaymentStatusRequiresAction},
		{stripe.PaymentIntentStatusProcessing, gopay.PaymentStatusProcessing},
		{stripe.PaymentIntentStatusSucceeded, gopay.PaymentStatusSucceeded},
		{stripe.PaymentIntentStatusCanceled, gopay.PaymentStatusCanceled},
		{stripe.PaymentIntentStatusRequiresCapture, gopay.PaymentStatusRequiresCapture},
		{"unknown_status", gopay.PaymentStatusPending},
	}

	for _, tt := range tests {
		t.Run(string(tt.input), func(t *testing.T) {
			got := p.mapPaymentStatus(tt.input)
			if got != tt.want {
				t.Errorf("mapPaymentStatus(%s) = %s, want %s", tt.input, got, tt.want)
			}
		})
	}
}

func TestMapRefundStatus(t *testing.T) {
	p := &Provider{}
	tests := []struct {
		input stripe.RefundStatus
		want  gopay.RefundStatus
	}{
		{stripe.RefundStatusPending, gopay.RefundStatusPending},
		{stripe.RefundStatusSucceeded, gopay.RefundStatusSucceeded},
		{stripe.RefundStatusFailed, gopay.RefundStatusFailed},
		{stripe.RefundStatusCanceled, gopay.RefundStatusCanceled},
		{"unknown", gopay.RefundStatusPending},
	}

	for _, tt := range tests {
		t.Run(string(tt.input), func(t *testing.T) {
			got := p.mapRefundStatus(tt.input)
			if got != tt.want {
				t.Errorf("mapRefundStatus(%s) = %s, want %s", tt.input, got, tt.want)
			}
		})
	}
}

func TestMapRefundReason(t *testing.T) {
	p := &Provider{}

	tests := []struct {
		input gopay.RefundReason
		want  string
	}{
		{gopay.RefundReasonDuplicate, "duplicate"},
		{gopay.RefundReasonFraudulent, "fraudulent"},
		{gopay.RefundReasonRequestedByCustomer, "requested_by_customer"},
		{gopay.RefundReasonOther, ""},
	}

	for _, tt := range tests {
		t.Run(string(tt.input), func(t *testing.T) {
			got := p.mapRefundReason(tt.input)
			if got != tt.want {
				t.Errorf("mapRefundReason(%s) = %s, want %s", tt.input, got, tt.want)
			}
		})
	}
}

func TestReverseMapRefundReason(t *testing.T) {
	p := &Provider{}

	tests := []struct {
		input string
		want  gopay.RefundReason
	}{
		{"duplicate", gopay.RefundReasonDuplicate},
		{"fraudulent", gopay.RefundReasonFraudulent},
		{"requested_by_customer", gopay.RefundReasonRequestedByCustomer},
		{"something_else", gopay.RefundReasonOther},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := p.reverseMapRefundReason(tt.input)
			if got != tt.want {
				t.Errorf("reverseMapRefundReason(%s) = %s, want %s", tt.input, got, tt.want)
			}
		})
	}
}

func TestMapPaymentMethodType(t *testing.T) {
	tests := []struct {
		input stripe.PaymentMethodType
		want  gopay.PaymentMethodType
	}{
		{stripe.PaymentMethodTypeCard, gopay.PaymentMethodCard},
		{stripe.PaymentMethodTypeUSBankAccount, gopay.PaymentMethodBankAccount},
		{"sepa_debit", gopay.PaymentMethodType("sepa_debit")},
	}

	for _, tt := range tests {
		t.Run(string(tt.input), func(t *testing.T) {
			got := mapPaymentMethodType(tt.input)
			if got != tt.want {
				t.Errorf("mapPaymentMethodType(%s) = %s, want %s", tt.input, got, tt.want)
			}
		})
	}
}

func TestMapError(t *testing.T) {
	p := &Provider{}

	tests := []struct {
		name    string
		code    stripe.ErrorCode
		wantErr error
	}{
		{"card declined", stripe.ErrorCodeCardDeclined, gopay.ErrCardDeclined},
		{"expired card", stripe.ErrorCodeExpiredCard, gopay.ErrExpiredCard},
		{"insufficient funds", stripe.ErrorCodeInsufficientFunds, gopay.ErrInsufficientFunds},
		{"incorrect number", stripe.ErrorCodeIncorrectNumber, gopay.ErrInvalidCard},
		{"resource missing", stripe.ErrorCodeResourceMissing, gopay.ErrNotFound},
		{"already refunded", stripe.ErrorCodeChargeAlreadyRefunded, gopay.ErrAlreadyRefunded},
		{"already captured", stripe.ErrorCodeChargeAlreadyCaptured, gopay.ErrAlreadyCaptured},
		{"unknown code", "some_other_code", gopay.ErrProviderError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stripeErr := &stripe.Error{
				Code: tt.code,
				Msg:  "test message",
			}
			got := p.mapError(stripeErr)
			if !errors.Is(got, tt.wantErr) {
				t.Errorf("mapError(%s) = %v, want %v", tt.code, got, tt.wantErr)
			}
		})
	}

	// Non-stripe error should pass through
	plainErr := errors.New("network error")
	got := p.mapError(plainErr)
	if got != plainErr {
		t.Errorf("mapError(plain) = %v, want %v", got, plainErr)
	}
}

func newFullPaymentIntent() *stripe.PaymentIntent {
	return &stripe.PaymentIntent{
		ID:             "pi_test",
		Amount:         1999,
		Currency:       "usd",
		Status:         stripe.PaymentIntentStatusSucceeded,
		Description:    "Test payment",
		Customer:       &stripe.Customer{ID: "cus_123"},
		PaymentMethod:  &stripe.PaymentMethod{ID: "pm_456"},
		CaptureMethod:  stripe.PaymentIntentCaptureMethodManual,
		AmountReceived: 1999,
		ClientSecret:   "cs_secret",
		LatestCharge:   &stripe.Charge{AmountRefunded: 200},
		Metadata:       map[string]string{"key": "val"},
		Created:        1700000000,
		LastPaymentError: &stripe.Error{
			Code: "card_declined",
			Msg:  "declined",
		},
		NextAction: &stripe.PaymentIntentNextAction{
			RedirectToURL: &stripe.PaymentIntentNextActionRedirectToURL{
				URL: "https://example.com/3ds",
			},
		},
	}
}

func TestMapPaymentIntentBasicFields(t *testing.T) {
	p := &Provider{}
	pay := p.mapPaymentIntent(newFullPaymentIntent())

	if pay.ID != "pi_test" {
		t.Errorf("ID = %s, want pi_test", pay.ID)
	}
	if pay.Amount.Value != 1999 {
		t.Errorf("Amount = %d, want 1999", pay.Amount.Value)
	}
	if pay.Amount.Currency != "USD" {
		t.Errorf("Currency = %s, want USD", pay.Amount.Currency)
	}
	if pay.Status != gopay.PaymentStatusSucceeded {
		t.Errorf("Status = %s, want succeeded", pay.Status)
	}
	if pay.Provider != "stripe" {
		t.Errorf("Provider = %s, want stripe", pay.Provider)
	}
	if pay.CreatedAt != time.Unix(1700000000, 0) {
		t.Errorf("CreatedAt = %v, want %v", pay.CreatedAt, time.Unix(1700000000, 0))
	}
}

func TestMapPaymentIntentRelations(t *testing.T) {
	p := &Provider{}
	pay := p.mapPaymentIntent(newFullPaymentIntent())

	if pay.CustomerID != "cus_123" {
		t.Errorf("CustomerID = %s, want cus_123", pay.CustomerID)
	}
	if pay.PaymentMethodID != "pm_456" {
		t.Errorf("PaymentMethodID = %s, want pm_456", pay.PaymentMethodID)
	}
}

func TestMapPaymentIntentAmounts(t *testing.T) {
	p := &Provider{}
	pay := p.mapPaymentIntent(newFullPaymentIntent())

	if pay.CaptureMethod != gopay.CaptureManual {
		t.Errorf("CaptureMethod = %s, want manual", pay.CaptureMethod)
	}
	if pay.AmountCaptured != 1999 {
		t.Errorf("AmountCaptured = %d, want 1999", pay.AmountCaptured)
	}
	if pay.AmountRefunded != 200 {
		t.Errorf("AmountRefunded = %d, want 200", pay.AmountRefunded)
	}
	if pay.ClientSecret != "cs_secret" {
		t.Errorf("ClientSecret = %s, want cs_secret", pay.ClientSecret)
	}
}

func TestMapPaymentIntentFailure(t *testing.T) {
	p := &Provider{}
	pay := p.mapPaymentIntent(newFullPaymentIntent())

	if pay.FailureCode != "card_declined" {
		t.Errorf("FailureCode = %s, want card_declined", pay.FailureCode)
	}
	if pay.FailureMessage != "declined" {
		t.Errorf("FailureMessage = %s, want declined", pay.FailureMessage)
	}
	if pay.RedirectURL != "https://example.com/3ds" {
		t.Errorf("RedirectURL = %s, want https://example.com/3ds", pay.RedirectURL)
	}
	if pay.Metadata["key"] != "val" {
		t.Errorf("Metadata[key] = %s, want val", pay.Metadata["key"])
	}
}

func TestMapPaymentIntentMinimal(t *testing.T) {
	p := &Provider{}

	pi := &stripe.PaymentIntent{
		ID:            "pi_minimal",
		Amount:        500,
		Currency:      "eur",
		Status:        stripe.PaymentIntentStatusRequiresPaymentMethod,
		CaptureMethod: stripe.PaymentIntentCaptureMethodAutomatic,
		Created:       1700000000,
	}

	pay := p.mapPaymentIntent(pi)

	if pay.CustomerID != "" {
		t.Errorf("CustomerID = %s, want empty", pay.CustomerID)
	}
	if pay.PaymentMethodID != "" {
		t.Errorf("PaymentMethodID = %s, want empty", pay.PaymentMethodID)
	}
	if pay.CaptureMethod != gopay.CaptureAutomatic {
		t.Errorf("CaptureMethod = %s, want automatic", pay.CaptureMethod)
	}
	if pay.AmountRefunded != 0 {
		t.Errorf("AmountRefunded = %d, want 0", pay.AmountRefunded)
	}
	if pay.FailureCode != "" {
		t.Errorf("FailureCode = %s, want empty", pay.FailureCode)
	}
	if pay.RedirectURL != "" {
		t.Errorf("RedirectURL = %s, want empty", pay.RedirectURL)
	}
}

func TestMapRefund(t *testing.T) {
	p := &Provider{}

	pi := &stripe.PaymentIntent{ID: "pi_123"}
	r := &stripe.Refund{
		ID:            "re_test",
		Amount:        500,
		Currency:      "usd",
		Status:        stripe.RefundStatusSucceeded,
		PaymentIntent: pi,
		Reason:        "duplicate",
		FailureReason: "expired_or_canceled_card",
		Metadata:      map[string]string{"note": "test"},
		Created:       1700000000,
	}

	ref := p.mapRefund(r)

	if ref.ID != "re_test" {
		t.Errorf("ID = %s, want re_test", ref.ID)
	}
	if ref.PaymentID != "pi_123" {
		t.Errorf("PaymentID = %s, want pi_123", ref.PaymentID)
	}
	if ref.Amount.Value != 500 {
		t.Errorf("Amount = %d, want 500", ref.Amount.Value)
	}
	if ref.Status != gopay.RefundStatusSucceeded {
		t.Errorf("Status = %s, want succeeded", ref.Status)
	}
	if ref.Reason != gopay.RefundReasonDuplicate {
		t.Errorf("Reason = %s, want duplicate", ref.Reason)
	}
	if ref.FailureReason != "expired_or_canceled_card" {
		t.Errorf("FailureReason = %s, want expired_or_canceled_card", ref.FailureReason)
	}
	if ref.Provider != "stripe" {
		t.Errorf("Provider = %s, want stripe", ref.Provider)
	}
}

func TestMapRefundNilPaymentIntent(t *testing.T) {
	p := &Provider{}

	r := &stripe.Refund{
		ID:       "re_orphan",
		Amount:   100,
		Currency: "usd",
		Status:   stripe.RefundStatusPending,
		Created:  1700000000,
	}

	ref := p.mapRefund(r)
	if ref.PaymentID != "" {
		t.Errorf("PaymentID = %s, want empty", ref.PaymentID)
	}
}

func TestMapCustomer(t *testing.T) {
	p := &Provider{}

	defaultPM := &stripe.PaymentMethod{ID: "pm_default"}
	c := &stripe.Customer{
		ID:          "cus_test",
		Email:       "test@example.com",
		Name:        "Test User",
		Phone:       "+1234567890",
		Description: "A test customer",
		Metadata:    map[string]string{"tier": "gold"},
		Created:     1700000000,
		InvoiceSettings: &stripe.CustomerInvoiceSettings{
			DefaultPaymentMethod: defaultPM,
		},
	}

	cust := p.mapCustomer(c)

	if cust.ID != "cus_test" {
		t.Errorf("ID = %s, want cus_test", cust.ID)
	}
	if cust.Email != "test@example.com" {
		t.Errorf("Email = %s, want test@example.com", cust.Email)
	}
	if cust.Name != "Test User" {
		t.Errorf("Name = %s, want Test User", cust.Name)
	}
	if cust.DefaultPaymentMethodID != "pm_default" {
		t.Errorf("DefaultPaymentMethodID = %s, want pm_default", cust.DefaultPaymentMethodID)
	}
	if cust.Provider != "stripe" {
		t.Errorf("Provider = %s, want stripe", cust.Provider)
	}
}

func TestMapPaymentMethod(t *testing.T) {
	p := &Provider{}

	pm := &stripe.PaymentMethod{
		ID:   "pm_test",
		Type: stripe.PaymentMethodTypeCard,
		Customer: &stripe.Customer{
			ID: "cus_123",
		},
		Card: &stripe.PaymentMethodCard{
			Brand:    stripe.PaymentMethodCardBrandVisa,
			Last4:    "4242",
			ExpMonth: 12,
			ExpYear:  2030,
			Funding:  stripe.CardFundingCredit,
			Country:  "US",
		},
		BillingDetails: &stripe.PaymentMethodBillingDetails{
			Name:  "John Doe",
			Email: "john@example.com",
			Phone: "+1234567890",
			Address: &stripe.Address{
				Line1:      "123 Main St",
				Line2:      "Apt 4",
				City:       "New York",
				State:      "NY",
				PostalCode: "10001",
				Country:    "US",
			},
		},
		Created: 1700000000,
	}

	method := p.mapPaymentMethod(pm)

	if method.ID != "pm_test" {
		t.Errorf("ID = %s, want pm_test", method.ID)
	}
	if method.Type != gopay.PaymentMethodCard {
		t.Errorf("Type = %s, want card", method.Type)
	}
	if method.CustomerID != "cus_123" {
		t.Errorf("CustomerID = %s, want cus_123", method.CustomerID)
	}
	if method.Card == nil {
		t.Fatal("Card is nil")
	}
	if method.Card.Brand != string(stripe.PaymentMethodCardBrandVisa) {
		t.Errorf("Card.Brand = %s, want visa", method.Card.Brand)
	}
	if method.Card.Last4 != "4242" {
		t.Errorf("Card.Last4 = %s, want 4242", method.Card.Last4)
	}
	if method.Card.ExpMonth != 12 {
		t.Errorf("Card.ExpMonth = %d, want 12", method.Card.ExpMonth)
	}
	if method.Card.ExpYear != 2030 {
		t.Errorf("Card.ExpYear = %d, want 2030", method.Card.ExpYear)
	}
	if method.BillingDetails == nil {
		t.Fatal("BillingDetails is nil")
	}
	if method.BillingDetails.Name != "John Doe" {
		t.Errorf("BillingDetails.Name = %s, want John Doe", method.BillingDetails.Name)
	}
	if method.BillingDetails.Address == nil {
		t.Fatal("Address is nil")
	}
	if method.BillingDetails.Address.City != "New York" {
		t.Errorf("Address.City = %s, want New York", method.BillingDetails.Address.City)
	}
}

func TestMapPaymentMethodMinimal(t *testing.T) {
	p := &Provider{}

	pm := &stripe.PaymentMethod{
		ID:      "pm_minimal",
		Type:    stripe.PaymentMethodTypeUSBankAccount,
		Created: 1700000000,
	}

	method := p.mapPaymentMethod(pm)

	if method.CustomerID != "" {
		t.Errorf("CustomerID = %s, want empty", method.CustomerID)
	}
	if method.Card != nil {
		t.Error("Card should be nil")
	}
	if method.BillingDetails != nil {
		t.Error("BillingDetails should be nil")
	}
	if method.Type != gopay.PaymentMethodBankAccount {
		t.Errorf("Type = %s, want bank_account", method.Type)
	}
}

func TestVerifyWebhookErrors(t *testing.T) {
	p := &Provider{config: Config{SecretKey: "sk_test_123"}}
	ctx := context.Background()

	// Missing signature header
	_, err := p.VerifyWebhook(ctx, []byte("{}"), map[string]string{})
	if !errors.Is(err, gopay.ErrProviderError) {
		t.Errorf("expected ErrProviderError for missing sig, got %v", err)
	}

	// Missing webhook secret
	_, err = p.VerifyWebhook(ctx, []byte("{}"), map[string]string{
		"Stripe-Signature": "t=123,v1=abc",
	})
	if !errors.Is(err, gopay.ErrInvalidConfig) {
		t.Errorf("expected ErrInvalidConfig for missing secret, got %v", err)
	}

	// Invalid signature (webhook secret set but signature is bad)
	p.config.WebhookSecret = "whsec_test"
	_, err = p.VerifyWebhook(ctx, []byte("{}"), map[string]string{
		"Stripe-Signature": "t=123,v1=invalid",
	})
	if err == nil {
		t.Error("expected error for invalid signature")
	}
	if !errors.Is(err, gopay.ErrProviderError) {
		t.Errorf("expected ErrProviderError for invalid sig, got %v", err)
	}
}
