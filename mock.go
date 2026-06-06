package gopay

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// MockProvider is a mock payment provider for testing.
type MockProvider struct {
	mu             sync.RWMutex
	payments       map[string]*Payment
	refunds        map[string]*Refund
	customers      map[string]*Customer
	paymentMethods map[string]*PaymentMethod
	createError    error
	captureError   error
	refundError    error
	webhookError   error
	autoCapture    bool
	autoSucceed    bool
}

// NewMockProvider creates a new mock provider.
func NewMockProvider() *MockProvider {
	return &MockProvider{
		payments:       make(map[string]*Payment),
		refunds:        make(map[string]*Refund),
		customers:      make(map[string]*Customer),
		paymentMethods: make(map[string]*PaymentMethod),
		autoCapture:    true,
		autoSucceed:    true,
	}
}

// Name returns the provider name.
func (p *MockProvider) Name() string {
	return "mock"
}

// CreatePayment creates a mock payment.
func (p *MockProvider) CreatePayment(ctx context.Context, req *PaymentRequest) (*Payment, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.createError != nil {
		return nil, p.createError
	}

	id := "pi_" + uuid.New().String()[:8]

	status := PaymentStatusPending
	if req.PaymentMethodID != "" && p.autoSucceed {
		if req.CaptureMethod == CaptureManual {
			status = PaymentStatusRequiresCapture
		} else {
			status = PaymentStatusSucceeded
		}
	}

	// Deep copy amount and metadata to avoid aliasing with caller.
	amt := &Amount{Value: req.Amount.Value, Currency: req.Amount.Currency}
	meta := make(map[string]string, len(req.Metadata))
	for k, v := range req.Metadata {
		meta[k] = v
	}

	payment := &Payment{
		ID:              id,
		Amount:          amt,
		Status:          status,
		Description:     req.Description,
		CustomerID:      req.CustomerID,
		PaymentMethodID: req.PaymentMethodID,
		CaptureMethod:   req.CaptureMethod,
		ClientSecret:    "cs_" + uuid.New().String()[:16],
		Metadata:        meta,
		CreatedAt:       time.Now(),
		Provider:        p.Name(),
		Raw:             map[string]any{"mock": true},
	}

	if status == PaymentStatusSucceeded && p.autoCapture {
		payment.AmountCaptured = req.Amount.Value
	}

	p.payments[id] = payment

	return payment, nil
}

// GetPayment retrieves a mock payment.
func (p *MockProvider) GetPayment(ctx context.Context, paymentID string) (*Payment, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	payment, ok := p.payments[paymentID]
	if !ok {
		return nil, ErrNotFound
	}

	return payment, nil
}

// CapturePayment captures a mock payment.
func (p *MockProvider) CapturePayment(ctx context.Context, paymentID string, amount *Amount) (*Payment, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.captureError != nil {
		return nil, p.captureError
	}

	payment, ok := p.payments[paymentID]
	if !ok {
		return nil, ErrNotFound
	}

	if payment.Status != PaymentStatusRequiresCapture {
		return nil, ErrAlreadyCaptured
	}

	captureAmount := payment.Amount.Value
	if amount != nil {
		captureAmount = amount.Value
	}

	payment.AmountCaptured = captureAmount
	payment.Status = PaymentStatusSucceeded

	return payment, nil
}

// CancelPayment cancels a mock payment.
func (p *MockProvider) CancelPayment(ctx context.Context, paymentID string) (*Payment, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	payment, ok := p.payments[paymentID]
	if !ok {
		return nil, ErrNotFound
	}

	switch payment.Status {
	case PaymentStatusPending, PaymentStatusRequiresAction, PaymentStatusRequiresCapture:
		// These statuses are cancelable.
	default:
		return nil, fmt.Errorf("%w: cannot cancel payment with status %s", ErrPaymentFailed, payment.Status)
	}

	payment.Status = PaymentStatusCanceled

	return payment, nil
}

// Refund creates a mock refund.
func (p *MockProvider) Refund(ctx context.Context, req *RefundRequest) (*Refund, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.refundError != nil {
		return nil, p.refundError
	}

	payment, ok := p.payments[req.PaymentID]
	if !ok {
		return nil, ErrNotFound
	}

	if payment.Status != PaymentStatusSucceeded {
		return nil, fmt.Errorf("%w: payment not in refundable state", ErrRefundFailed)
	}

	refundAmount := &Amount{Value: payment.Amount.Value, Currency: payment.Amount.Currency}
	if req.Amount != nil {
		refundAmount = &Amount{Value: req.Amount.Value, Currency: req.Amount.Currency}
	}

	if payment.AmountRefunded+refundAmount.Value > payment.AmountCaptured {
		return nil, fmt.Errorf("%w: refund amount exceeds captured amount", ErrRefundFailed)
	}

	meta := make(map[string]string, len(req.Metadata))
	for k, v := range req.Metadata {
		meta[k] = v
	}

	id := "re_" + uuid.New().String()[:8]

	refund := &Refund{
		ID:        id,
		PaymentID: req.PaymentID,
		Amount:    refundAmount,
		Status:    RefundStatusSucceeded,
		Reason:    req.Reason,
		Metadata:  meta,
		CreatedAt: time.Now(),
		Provider:  p.Name(),
		Raw:       map[string]any{"mock": true},
	}

	p.refunds[id] = refund
	payment.AmountRefunded += refundAmount.Value

	return refund, nil
}

// GetRefund retrieves a mock refund.
func (p *MockProvider) GetRefund(ctx context.Context, refundID string) (*Refund, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	refund, ok := p.refunds[refundID]
	if !ok {
		return nil, ErrNotFound
	}

	return refund, nil
}

// CreateCustomer creates a mock customer.
func (p *MockProvider) CreateCustomer(ctx context.Context, req *CustomerRequest) (*Customer, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	id := "cus_" + uuid.New().String()[:8]

	meta := make(map[string]string, len(req.Metadata))
	for k, v := range req.Metadata {
		meta[k] = v
	}

	customer := &Customer{
		ID:          id,
		Email:       req.Email,
		Name:        req.Name,
		Phone:       req.Phone,
		Description: req.Description,
		Metadata:    meta,
		CreatedAt:   time.Now(),
		Provider:    p.Name(),
		Raw:         map[string]any{"mock": true},
	}

	p.customers[id] = customer

	return customer, nil
}

// GetCustomer retrieves a mock customer.
func (p *MockProvider) GetCustomer(ctx context.Context, customerID string) (*Customer, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	customer, ok := p.customers[customerID]
	if !ok {
		return nil, ErrNotFound
	}

	return customer, nil
}

// UpdateCustomer updates a mock customer.
func (p *MockProvider) UpdateCustomer(ctx context.Context, customerID string, req *CustomerRequest) (*Customer, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	customer, ok := p.customers[customerID]
	if !ok {
		return nil, ErrNotFound
	}

	if req.Email != "" {
		customer.Email = req.Email
	}
	if req.Name != "" {
		customer.Name = req.Name
	}
	if req.Phone != "" {
		customer.Phone = req.Phone
	}
	if req.Description != "" {
		customer.Description = req.Description
	}
	for k, v := range req.Metadata {
		customer.Metadata[k] = v
	}

	return customer, nil
}

// DeleteCustomer deletes a mock customer.
func (p *MockProvider) DeleteCustomer(ctx context.Context, customerID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if _, ok := p.customers[customerID]; !ok {
		return ErrNotFound
	}

	delete(p.customers, customerID)
	return nil
}

// AttachPaymentMethod attaches a payment method to a customer.
func (p *MockProvider) AttachPaymentMethod(ctx context.Context, customerID, paymentMethodID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	pm, ok := p.paymentMethods[paymentMethodID]
	if !ok {
		// Create a mock payment method
		pm = &PaymentMethod{
			ID:         paymentMethodID,
			Type:       PaymentMethodCard,
			CustomerID: customerID,
			Card: &CardDetails{
				Brand:    "visa",
				Last4:    "4242",
				ExpMonth: 12,
				ExpYear:  2030,
				Funding:  "credit",
			},
			CreatedAt: time.Now(),
			Provider:  p.Name(),
		}
		p.paymentMethods[paymentMethodID] = pm
	}

	pm.CustomerID = customerID
	return nil
}

// DetachPaymentMethod detaches a payment method.
func (p *MockProvider) DetachPaymentMethod(ctx context.Context, paymentMethodID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	pm, ok := p.paymentMethods[paymentMethodID]
	if !ok {
		return ErrNotFound
	}

	pm.CustomerID = ""
	return nil
}

// ListPaymentMethods lists payment methods for a customer.
func (p *MockProvider) ListPaymentMethods(ctx context.Context, customerID string) ([]*PaymentMethod, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var methods []*PaymentMethod
	for _, pm := range p.paymentMethods {
		if pm.CustomerID == customerID {
			methods = append(methods, pm)
		}
	}

	return methods, nil
}

// VerifyWebhook verifies and parses a mock webhook event.
// It simply parses the payload as JSON and returns it as a WebhookEvent.
// Use WithWebhookError to simulate verification failures.
func (p *MockProvider) VerifyWebhook(_ context.Context, payload []byte, _ map[string]string) (*WebhookEvent, error) {
	p.mu.RLock()
	webhookErr := p.webhookError
	p.mu.RUnlock()

	if webhookErr != nil {
		return nil, webhookErr
	}

	var event struct {
		ID   string `json:"id"`
		Type string `json:"type"`
	}
	if err := json.Unmarshal(payload, &event); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrProviderError, err)
	}

	return &WebhookEvent{
		ID:       event.ID,
		Type:     event.Type,
		Provider: "mock",
		Raw:      payload,
	}, nil
}

// WithWebhookError sets the error to return on VerifyWebhook.
func (p *MockProvider) WithWebhookError(err error) *MockProvider {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.webhookError = err
	return p
}

// WithCreateError sets the error to return on CreatePayment.
func (p *MockProvider) WithCreateError(err error) *MockProvider {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.createError = err
	return p
}

// WithCaptureError sets the error to return on CapturePayment.
func (p *MockProvider) WithCaptureError(err error) *MockProvider {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.captureError = err
	return p
}

// WithRefundError sets the error to return on Refund.
func (p *MockProvider) WithRefundError(err error) *MockProvider {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.refundError = err
	return p
}

// WithAutoCapture sets whether payments are auto-captured.
func (p *MockProvider) WithAutoCapture(auto bool) *MockProvider {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.autoCapture = auto
	return p
}

// WithAutoSucceed sets whether payments auto-succeed.
func (p *MockProvider) WithAutoSucceed(auto bool) *MockProvider {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.autoSucceed = auto
	return p
}

// SetPayment manually sets a payment.
func (p *MockProvider) SetPayment(payment *Payment) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.payments[payment.ID] = payment
}

// SetRefund manually sets a refund.
func (p *MockProvider) SetRefund(refund *Refund) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.refunds[refund.ID] = refund
}

// SetCustomer manually sets a customer.
func (p *MockProvider) SetCustomer(customer *Customer) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.customers[customer.ID] = customer
}

// Payments returns all payments.
func (p *MockProvider) Payments() map[string]*Payment {
	p.mu.RLock()
	defer p.mu.RUnlock()

	result := make(map[string]*Payment)
	for k, v := range p.payments {
		result[k] = v
	}
	return result
}

// Refunds returns all refunds.
func (p *MockProvider) Refunds() map[string]*Refund {
	p.mu.RLock()
	defer p.mu.RUnlock()

	result := make(map[string]*Refund)
	for k, v := range p.refunds {
		result[k] = v
	}
	return result
}

// Customers returns all customers.
func (p *MockProvider) Customers() map[string]*Customer {
	p.mu.RLock()
	defer p.mu.RUnlock()

	result := make(map[string]*Customer)
	for k, v := range p.customers {
		result[k] = v
	}
	return result
}

// Reset clears all data.
func (p *MockProvider) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.payments = make(map[string]*Payment)
	p.refunds = make(map[string]*Refund)
	p.customers = make(map[string]*Customer)
	p.paymentMethods = make(map[string]*PaymentMethod)
	p.createError = nil
	p.captureError = nil
	p.refundError = nil
	p.webhookError = nil
	p.autoCapture = true
	p.autoSucceed = true
}
