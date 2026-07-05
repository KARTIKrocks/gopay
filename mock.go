package gopay

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
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
	setupIntents   map[string]*SetupIntent
	plans          map[string]*Plan
	subscriptions  map[string]*Subscription
	invoices       map[string]*Invoice
	createError    error
	captureError   error
	refundError    error
	webhookError   error
	setupError     error
	subError       error
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
		setupIntents:   make(map[string]*SetupIntent),
		plans:          make(map[string]*Plan),
		subscriptions:  make(map[string]*Subscription),
		invoices:       make(map[string]*Invoice),
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

// CreateSetupIntent creates a mock setup intent. When a PaymentMethodID is
// supplied and auto-succeed is on, the intent is marked succeeded; otherwise it
// awaits confirmation.
func (p *MockProvider) CreateSetupIntent(_ context.Context, req *SetupIntentRequest) (*SetupIntent, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.setupError != nil {
		return nil, p.setupError
	}

	usage := req.Usage
	if usage == "" {
		usage = SetupIntentUsageOffSession
	}

	status := SetupIntentStatusRequiresPaymentMethod
	if req.PaymentMethodID != "" {
		if p.autoSucceed {
			status = SetupIntentStatusSucceeded
		} else {
			status = SetupIntentStatusRequiresConfirmation
		}
	}

	meta := make(map[string]string, len(req.Metadata))
	for k, v := range req.Metadata {
		meta[k] = v
	}

	id := "seti_" + uuid.New().String()[:8]
	si := &SetupIntent{
		ID:              id,
		Status:          status,
		CustomerID:      req.CustomerID,
		PaymentMethodID: req.PaymentMethodID,
		Usage:           usage,
		Description:     req.Description,
		ClientSecret:    id + "_secret_" + uuid.New().String()[:16],
		Metadata:        meta,
		CreatedAt:       time.Now(),
		Provider:        p.Name(),
		Raw:             map[string]any{"mock": true},
	}

	p.setupIntents[id] = si
	return si, nil
}

// GetSetupIntent retrieves a mock setup intent.
func (p *MockProvider) GetSetupIntent(_ context.Context, setupIntentID string) (*SetupIntent, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	si, ok := p.setupIntents[setupIntentID]
	if !ok {
		return nil, ErrNotFound
	}
	return si, nil
}

// CancelSetupIntent cancels a mock setup intent.
func (p *MockProvider) CancelSetupIntent(_ context.Context, setupIntentID string) (*SetupIntent, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	si, ok := p.setupIntents[setupIntentID]
	if !ok {
		return nil, ErrNotFound
	}

	if si.Status == SetupIntentStatusSucceeded {
		return nil, fmt.Errorf("%w: cannot cancel a succeeded setup intent", ErrSetupFailed)
	}

	si.Status = SetupIntentStatusCanceled
	return si, nil
}

// AddInvoice seeds an invoice so GetInvoice can return it. It exists because the
// mock has no invoice-creation flow; tests use it to stage retrievable invoices.
func (p *MockProvider) AddInvoice(inv *Invoice) *MockProvider {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.invoices[inv.ID] = inv
	return p
}

// GetInvoice retrieves a seeded mock invoice.
func (p *MockProvider) GetInvoice(_ context.Context, invoiceID string) (*Invoice, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	inv, ok := p.invoices[invoiceID]
	if !ok {
		return nil, ErrNotFound
	}
	return inv, nil
}

// CreatePlan creates a mock plan.
func (p *MockProvider) CreatePlan(_ context.Context, req *PlanRequest) (*Plan, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.subError != nil {
		return nil, p.subError
	}

	count := req.IntervalCount
	if count <= 0 {
		count = 1
	}

	meta := make(map[string]string, len(req.Metadata))
	for k, v := range req.Metadata {
		meta[k] = v
	}

	id := "plan_" + uuid.New().String()[:8]
	plan := &Plan{
		ID:            id,
		Name:          req.Name,
		Amount:        &Amount{Value: req.Amount.Value, Currency: req.Amount.Currency},
		Interval:      req.Interval,
		IntervalCount: count,
		Metadata:      meta,
		CreatedAt:     time.Now(),
		Provider:      p.Name(),
		Raw:           map[string]any{"mock": true},
	}

	p.plans[id] = plan
	return plan, nil
}

// GetPlan retrieves a mock plan.
func (p *MockProvider) GetPlan(_ context.Context, planID string) (*Plan, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	plan, ok := p.plans[planID]
	if !ok {
		return nil, ErrNotFound
	}
	return plan, nil
}

// CreateSubscription creates a mock subscription. The plan must exist. When
// auto-succeed is on the subscription is active (or trialing when TrialDays is
// set); otherwise it is incomplete.
func (p *MockProvider) CreateSubscription(_ context.Context, req *SubscriptionRequest) (*Subscription, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.subError != nil {
		return nil, p.subError
	}

	plan, ok := p.plans[req.PlanID]
	if !ok {
		return nil, fmt.Errorf("%w: unknown plan", ErrSubscriptionFailed)
	}

	status := SubscriptionStatusIncomplete
	if p.autoSucceed {
		if req.TrialDays > 0 {
			status = SubscriptionStatusTrialing
		} else {
			status = SubscriptionStatusActive
		}
	}

	meta := make(map[string]string, len(req.Metadata))
	for k, v := range req.Metadata {
		meta[k] = v
	}

	now := time.Now()
	periodEnd := addInterval(now, plan.Interval, plan.IntervalCount)
	if req.TrialDays > 0 {
		periodEnd = now.AddDate(0, 0, req.TrialDays)
	}

	id := "sub_" + uuid.New().String()[:8]
	sub := &Subscription{
		ID:                 id,
		CustomerID:         req.CustomerID,
		PlanID:             req.PlanID,
		PaymentMethodID:    req.PaymentMethodID,
		Status:             status,
		CurrentPeriodStart: now,
		CurrentPeriodEnd:   periodEnd,
		Metadata:           meta,
		CreatedAt:          now,
		Provider:           p.Name(),
		Raw:                map[string]any{"mock": true},
	}

	p.subscriptions[id] = sub
	return sub, nil
}

// GetSubscription retrieves a mock subscription.
func (p *MockProvider) GetSubscription(_ context.Context, subscriptionID string) (*Subscription, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	sub, ok := p.subscriptions[subscriptionID]
	if !ok {
		return nil, ErrNotFound
	}
	return sub, nil
}

// CancelSubscription cancels a mock subscription, immediately or at period end.
func (p *MockProvider) CancelSubscription(_ context.Context, subscriptionID string, opts *CancelOptions) (*Subscription, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	sub, ok := p.subscriptions[subscriptionID]
	if !ok {
		return nil, ErrNotFound
	}

	if opts != nil && opts.AtPeriodEnd {
		sub.CancelAtPeriodEnd = true
		return sub, nil
	}

	sub.Status = SubscriptionStatusCanceled
	sub.CanceledAt = time.Now()
	return sub, nil
}

// addInterval advances t by count units of the given billing interval. It is a
// helper for computing the mock's next billing period.
func addInterval(t time.Time, interval BillingInterval, count int) time.Time {
	if count <= 0 {
		count = 1
	}
	switch interval {
	case BillingIntervalDay:
		return t.AddDate(0, 0, count)
	case BillingIntervalWeek:
		return t.AddDate(0, 0, 7*count)
	case BillingIntervalMonth:
		return t.AddDate(0, count, 0)
	case BillingIntervalYear:
		return t.AddDate(count, 0, 0)
	default:
		return t.AddDate(0, count, 0)
	}
}

// ListPayments lists mock payments with cursor-based pagination. Results are
// ordered newest-first (by CreatedAt, then ID for determinism); the opaque
// cursor is the ID of the last item on the previous page.
func (p *MockProvider) ListPayments(_ context.Context, params *ListParams) (*List[*Payment], error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	all := make([]*Payment, 0, len(p.payments))
	for _, pay := range p.payments {
		all = append(all, pay)
	}
	sortByCreatedDesc(all, func(x *Payment) (time.Time, string) { return x.CreatedAt, x.ID })

	items, hasMore, next := paginate(all, params, func(x *Payment) string { return x.ID })
	return &List[*Payment]{Items: items, HasMore: hasMore, NextCursor: next}, nil
}

// ListRefunds lists mock refunds with cursor-based pagination.
func (p *MockProvider) ListRefunds(_ context.Context, params *ListParams) (*List[*Refund], error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	all := make([]*Refund, 0, len(p.refunds))
	for _, r := range p.refunds {
		all = append(all, r)
	}
	sortByCreatedDesc(all, func(x *Refund) (time.Time, string) { return x.CreatedAt, x.ID })

	items, hasMore, next := paginate(all, params, func(x *Refund) string { return x.ID })
	return &List[*Refund]{Items: items, HasMore: hasMore, NextCursor: next}, nil
}

// ListCustomers lists mock customers with cursor-based pagination.
func (p *MockProvider) ListCustomers(_ context.Context, params *ListParams) (*List[*Customer], error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	all := make([]*Customer, 0, len(p.customers))
	for _, c := range p.customers {
		all = append(all, c)
	}
	sortByCreatedDesc(all, func(x *Customer) (time.Time, string) { return x.CreatedAt, x.ID })

	items, hasMore, next := paginate(all, params, func(x *Customer) string { return x.ID })
	return &List[*Customer]{Items: items, HasMore: hasMore, NextCursor: next}, nil
}

// sortByCreatedDesc sorts items newest-first by their (CreatedAt, ID) key, using
// ID as a stable tiebreak so pagination is deterministic even when timestamps
// collide.
func sortByCreatedDesc[T any](items []T, key func(T) (time.Time, string)) {
	sort.Slice(items, func(i, j int) bool {
		ti, idi := key(items[i])
		tj, idj := key(items[j])
		if ti.Equal(tj) {
			return idi > idj
		}
		return ti.After(tj)
	})
}

// paginate returns the page of items following the cursor in params, along with
// whether more remain and the cursor for the next page. The cursor is the id of
// the last item on the previous page; items must be pre-sorted.
func paginate[T any](items []T, params *ListParams, id func(T) string) ([]T, bool, string) {
	start := 0
	if params != nil && params.Cursor != "" {
		for i, it := range items {
			if id(it) == params.Cursor {
				start = i + 1
				break
			}
		}
	}

	end := min(start+params.EffectiveLimit(), len(items))

	page := items[start:end]
	hasMore := end < len(items)
	next := ""
	if hasMore && len(page) > 0 {
		next = id(page[len(page)-1])
	}
	return page, hasMore, next
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
		ID        string           `json:"id"`
		Type      string           `json:"type"`
		Kind      WebhookEventKind `json:"kind"`
		PaymentID string           `json:"payment_id"`
		OrderID   string           `json:"order_id"`
		RefundID  string           `json:"refund_id"`
		Amount    int64            `json:"amount"`
		Currency  string           `json:"currency"`
	}
	if err := json.Unmarshal(payload, &event); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrProviderError, err)
	}

	ev := &WebhookEvent{
		ID:        event.ID,
		Type:      event.Type,
		Provider:  "mock",
		Raw:       payload,
		Kind:      event.Kind,
		PaymentID: event.PaymentID,
		OrderID:   event.OrderID,
		RefundID:  event.RefundID,
	}
	// The mock emits already-normalized minor-unit amounts directly.
	if event.Currency != "" {
		ev.Amount = NewAmount(event.Amount, event.Currency)
	}
	return ev, nil
}

// WithWebhookError sets the error to return on VerifyWebhook.
func (p *MockProvider) WithWebhookError(err error) *MockProvider {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.webhookError = err
	return p
}

// WithSetupError sets the error to return on CreateSetupIntent.
func (p *MockProvider) WithSetupError(err error) *MockProvider {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.setupError = err
	return p
}

// WithSubscriptionError sets the error to return on CreatePlan and
// CreateSubscription.
func (p *MockProvider) WithSubscriptionError(err error) *MockProvider {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.subError = err
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

// SetSetupIntent manually sets a setup intent.
func (p *MockProvider) SetSetupIntent(si *SetupIntent) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.setupIntents[si.ID] = si
}

// SetPlan manually sets a plan.
func (p *MockProvider) SetPlan(plan *Plan) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.plans[plan.ID] = plan
}

// SetSubscription manually sets a subscription.
func (p *MockProvider) SetSubscription(sub *Subscription) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.subscriptions[sub.ID] = sub
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
	p.setupIntents = make(map[string]*SetupIntent)
	p.plans = make(map[string]*Plan)
	p.subscriptions = make(map[string]*Subscription)
	p.invoices = make(map[string]*Invoice)
	p.createError = nil
	p.captureError = nil
	p.refundError = nil
	p.webhookError = nil
	p.setupError = nil
	p.subError = nil
	p.autoCapture = true
	p.autoSucceed = true
}
