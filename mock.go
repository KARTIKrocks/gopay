package gopay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

// MockProvider is a mock payment provider for testing.
type MockProvider struct {
	mu               sync.RWMutex
	payments         map[string]*Payment
	refunds          map[string]*Refund
	customers        map[string]*Customer
	paymentMethods   map[string][]string // customerID -> []paymentMethodID
	paymentCounter   int
	refundCounter    int
	customerCounter  int
	createError      error
	captureError     error
	cancelError      error
	refundError      error
	webhookError     error
	autoSucceed      bool
	autoCapture      bool
}

// NewMockProvider creates a new mock provider.
func NewMockProvider() *MockProvider {
	return &MockProvider{
		payments:       make(map[string]*Payment),
		refunds:        make(map[string]*Refund),
		customers:      make(map[string]*Customer),
		paymentMethods: make(map[string][]string),
		autoSucceed:    true,
		autoCapture:    true,
	}
}

// Name returns the provider name.
func (m *MockProvider) Name() string {
	return "mock"
}

// CreatePayment creates a mock payment.
func (m *MockProvider) CreatePayment(ctx context.Context, req *PaymentRequest) (*Payment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.createError != nil {
		return nil, m.createError
	}

	m.paymentCounter++
	payment := &Payment{
		ID:              fmt.Sprintf("pi_mock_%d", m.paymentCounter),
		Amount:          req.Amount,
		Description:     req.Description,
		CustomerID:      req.CustomerID,
		PaymentMethodID: req.PaymentMethodID,
		CaptureMethod:   req.CaptureMethod,
		Metadata:        req.Metadata,
		CreatedAt:       time.Now(),
		Provider:        "mock",
		Raw:             make(map[string]any),
	}

	// Set status based on capture method and auto settings
	if req.CaptureMethod == CaptureManual {
		payment.Status = PaymentStatusRequiresCapture
		payment.AmountCaptured = 0
	} else {
		if m.autoSucceed {
			payment.Status = PaymentStatusSucceeded
			if m.autoCapture {
				payment.AmountCaptured = req.Amount.Value
			}
		} else {
			payment.Status = PaymentStatusPending
		}
	}

	m.payments[payment.ID] = payment
	return payment, nil
}

// GetPayment retrieves a mock payment.
func (m *MockProvider) GetPayment(ctx context.Context, paymentID string) (*Payment, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	payment, ok := m.payments[paymentID]
	if !ok {
		return nil, ErrNotFound
	}
	return payment, nil
}

// CapturePayment captures a mock payment.
func (m *MockProvider) CapturePayment(ctx context.Context, paymentID string, amount *Amount) (*Payment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.captureError != nil {
		return nil, m.captureError
	}

	payment, ok := m.payments[paymentID]
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

	payment.Status = PaymentStatusSucceeded
	payment.AmountCaptured = captureAmount

	return payment, nil
}

// CancelPayment cancels a mock payment.
func (m *MockProvider) CancelPayment(ctx context.Context, paymentID string) (*Payment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cancelError != nil {
		return nil, m.cancelError
	}

	payment, ok := m.payments[paymentID]
	if !ok {
		return nil, ErrNotFound
	}

	// Only allow canceling payments that can be canceled
	if payment.Status == PaymentStatusSucceeded || payment.Status == PaymentStatusCanceled {
		return nil, errors.New("gopay: payment cannot be canceled")
	}

	payment.Status = PaymentStatusCanceled
	return payment, nil
}

// Refund creates a mock refund.
func (m *MockProvider) Refund(ctx context.Context, req *RefundRequest) (*Refund, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.refundError != nil {
		return nil, m.refundError
	}

	payment, ok := m.payments[req.PaymentID]
	if !ok {
		return nil, ErrNotFound
	}

	m.refundCounter++
	refundAmount := payment.Amount
	if req.Amount != nil {
		refundAmount = req.Amount
	}

	refund := &Refund{
		ID:        fmt.Sprintf("re_mock_%d", m.refundCounter),
		PaymentID: req.PaymentID,
		Amount:    refundAmount,
		Status:    RefundStatusSucceeded,
		Reason:    req.Reason,
		Metadata:  req.Metadata,
		CreatedAt: time.Now(),
		Provider:  "mock",
		Raw:       make(map[string]any),
	}

	payment.AmountRefunded += refundAmount.Value
	m.refunds[refund.ID] = refund
	return refund, nil
}

// GetRefund retrieves a mock refund.
func (m *MockProvider) GetRefund(ctx context.Context, refundID string) (*Refund, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	refund, ok := m.refunds[refundID]
	if !ok {
		return nil, ErrNotFound
	}
	return refund, nil
}

// CreateCustomer creates a mock customer.
func (m *MockProvider) CreateCustomer(ctx context.Context, req *CustomerRequest) (*Customer, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.customerCounter++
	customer := &Customer{
		ID:          fmt.Sprintf("cus_mock_%d", m.customerCounter),
		Email:       req.Email,
		Name:        req.Name,
		Phone:       req.Phone,
		Description: req.Description,
		Metadata:    req.Metadata,
		CreatedAt:   time.Now(),
		Provider:    "mock",
		Raw:         make(map[string]any),
	}

	m.customers[customer.ID] = customer
	return customer, nil
}

// GetCustomer retrieves a mock customer.
func (m *MockProvider) GetCustomer(ctx context.Context, customerID string) (*Customer, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	customer, ok := m.customers[customerID]
	if !ok {
		return nil, ErrNotFound
	}
	return customer, nil
}

// UpdateCustomer updates a mock customer.
func (m *MockProvider) UpdateCustomer(ctx context.Context, customerID string, req *CustomerRequest) (*Customer, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	customer, ok := m.customers[customerID]
	if !ok {
		return nil, ErrNotFound
	}

	customer.Email = req.Email
	customer.Name = req.Name
	customer.Phone = req.Phone
	customer.Description = req.Description
	if req.Metadata != nil {
		customer.Metadata = req.Metadata
	}

	return customer, nil
}

// DeleteCustomer deletes a mock customer.
func (m *MockProvider) DeleteCustomer(ctx context.Context, customerID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.customers[customerID]; !ok {
		return ErrNotFound
	}

	delete(m.customers, customerID)
	delete(m.paymentMethods, customerID)
	return nil
}

// AttachPaymentMethod attaches a payment method to a customer.
func (m *MockProvider) AttachPaymentMethod(ctx context.Context, customerID, paymentMethodID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.customers[customerID]; !ok {
		return ErrNotFound
	}

	methods := m.paymentMethods[customerID]
	methods = append(methods, paymentMethodID)
	m.paymentMethods[customerID] = methods
	return nil
}

// DetachPaymentMethod detaches a payment method from a customer.
func (m *MockProvider) DetachPaymentMethod(ctx context.Context, paymentMethodID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for customerID, methods := range m.paymentMethods {
		for i, id := range methods {
			if id == paymentMethodID {
				m.paymentMethods[customerID] = append(methods[:i], methods[i+1:]...)
				return nil
			}
		}
	}
	return ErrNotFound
}

// ListPaymentMethods lists payment methods for a customer.
func (m *MockProvider) ListPaymentMethods(ctx context.Context, customerID string) ([]*PaymentMethod, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if _, ok := m.customers[customerID]; !ok {
		return nil, ErrNotFound
	}

	methodIDs := m.paymentMethods[customerID]
	methods := make([]*PaymentMethod, 0, len(methodIDs))
	for _, id := range methodIDs {
		methods = append(methods, &PaymentMethod{
			ID:         id,
			Type:       PaymentMethodCard,
			CustomerID: customerID,
			CreatedAt:  time.Now(),
			Provider:   "mock",
			Raw:        make(map[string]any),
		})
	}
	return methods, nil
}

// VerifyWebhook verifies and parses a mock webhook event.
func (m *MockProvider) VerifyWebhook(ctx context.Context, payload []byte, headers map[string]string) (*WebhookEvent, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.webhookError != nil {
		return nil, m.webhookError
	}

	// Parse the JSON payload
	var data map[string]any
	if err := json.Unmarshal(payload, &data); err != nil {
		return nil, fmt.Errorf("gopay: invalid webhook payload: %w", err)
	}

	event := &WebhookEvent{
		Provider: "mock",
		Raw:      payload,
	}

	if id, ok := data["id"].(string); ok {
		event.ID = id
	}
	if typ, ok := data["type"].(string); ok {
		event.Type = typ
	}

	return event, nil
}

// WithCreateError sets an error for CreatePayment.
func (m *MockProvider) WithCreateError(err error) *MockProvider {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.createError = err
	return m
}

// WithCaptureError sets an error for CapturePayment.
func (m *MockProvider) WithCaptureError(err error) *MockProvider {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.captureError = err
	return m
}

// WithRefundError sets an error for Refund.
func (m *MockProvider) WithRefundError(err error) *MockProvider {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.refundError = err
	return m
}

// WithWebhookError sets an error for VerifyWebhook.
func (m *MockProvider) WithWebhookError(err error) *MockProvider {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.webhookError = err
	return m
}

// WithAutoSucceed sets whether payments automatically succeed.
func (m *MockProvider) WithAutoSucceed(autoSucceed bool) *MockProvider {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.autoSucceed = autoSucceed
	return m
}

// WithAutoCapture sets whether payments are automatically captured.
func (m *MockProvider) WithAutoCapture(autoCapture bool) *MockProvider {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.autoCapture = autoCapture
	return m
}

// Reset resets the mock provider state.
func (m *MockProvider) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.payments = make(map[string]*Payment)
	m.refunds = make(map[string]*Refund)
	m.customers = make(map[string]*Customer)
	m.paymentMethods = make(map[string][]string)
	m.paymentCounter = 0
	m.refundCounter = 0
	m.customerCounter = 0
	m.createError = nil
	m.captureError = nil
	m.cancelError = nil
	m.refundError = nil
	m.webhookError = nil
	m.autoSucceed = true
	m.autoCapture = true
}

// SetPayment manually sets a payment in the mock store.
func (m *MockProvider) SetPayment(payment *Payment) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.payments[payment.ID] = payment
}

// SetRefund manually sets a refund in the mock store.
func (m *MockProvider) SetRefund(refund *Refund) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.refunds[refund.ID] = refund
}

// SetCustomer manually sets a customer in the mock store.
func (m *MockProvider) SetCustomer(customer *Customer) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.customers[customer.ID] = customer
}

// Payments returns a copy of the payments map.
func (m *MockProvider) Payments() map[string]*Payment {
	m.mu.RLock()
	defer m.mu.RUnlock()
	copy := make(map[string]*Payment, len(m.payments))
	for k, v := range m.payments {
		copy[k] = v
	}
	return copy
}

// Refunds returns a copy of the refunds map.
func (m *MockProvider) Refunds() map[string]*Refund {
	m.mu.RLock()
	defer m.mu.RUnlock()
	copy := make(map[string]*Refund, len(m.refunds))
	for k, v := range m.refunds {
		copy[k] = v
	}
	return copy
}

// Customers returns a copy of the customers map.
func (m *MockProvider) Customers() map[string]*Customer {
	m.mu.RLock()
	defer m.mu.RUnlock()
	copy := make(map[string]*Customer, len(m.customers))
	for k, v := range m.customers {
		copy[k] = v
	}
	return copy
}
