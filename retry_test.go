package gopay

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
)

// flakyProvider fails its first `failures` calls to each overridden method
// with err, then succeeds. It embeds MockProvider for the rest of Provider.
type flakyProvider struct {
	*MockProvider
	honorsKeys bool

	mu       sync.Mutex
	failures int
	err      error
	calls    int
	deadline time.Time // deadline of the last ctx seen, if any
}

func newFlaky(failures int, err error) *flakyProvider {
	return &flakyProvider{MockProvider: NewMockProvider(), failures: failures, err: err}
}

func (p *flakyProvider) attempt(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	p.deadline, _ = ctx.Deadline()
	if p.calls <= p.failures {
		return p.err
	}
	return nil
}

func (p *flakyProvider) callCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

func (p *flakyProvider) GetPayment(ctx context.Context, id string) (*Payment, error) {
	if err := p.attempt(ctx); err != nil {
		return nil, err
	}
	return &Payment{ID: id}, nil
}

func (p *flakyProvider) CreatePayment(ctx context.Context, _ *PaymentRequest) (*Payment, error) {
	if err := p.attempt(ctx); err != nil {
		return nil, err
	}
	return &Payment{ID: "pay_new"}, nil
}

func (p *flakyProvider) CapturePayment(ctx context.Context, id string, _ *Amount) (*Payment, error) {
	if err := p.attempt(ctx); err != nil {
		return nil, err
	}
	return &Payment{ID: id}, nil
}

// idempotentFlaky is a flakyProvider that implements IdempotentProvider.
type idempotentFlaky struct{ *flakyProvider }

func (p idempotentFlaky) HonorsIdempotencyKeys() bool { return p.honorsKeys }

// newTestClient builds a Client that records waits instead of sleeping and
// uses the maximum jitter, so each wait equals the full backoff.
func newTestClient(t *testing.T, p Provider, opts ...ClientOption) (*Client, *[]time.Duration) {
	t.Helper()
	c, err := NewClient(p, opts...)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	var waits []time.Duration
	c.sleep = func(ctx context.Context, d time.Duration) error {
		waits = append(waits, d)
		return ctx.Err()
	}
	c.jitter = func(n int64) int64 { return n - 1 }
	return c, &waits
}

var testPolicy = RetryPolicy{MaxAttempts: 3, BaseDelay: 100 * time.Millisecond, MaxDelay: time.Second}

func TestRetryReadSucceedsAfterTransientFailures(t *testing.T) {
	t.Parallel()
	p := newFlaky(2, &Error{Kind: ErrProviderUnavailable})
	c, waits := newTestClient(t, p, WithRetry(testPolicy))

	got, err := c.GetPayment(context.Background(), "pay_1")
	if err != nil {
		t.Fatalf("GetPayment() error = %v", err)
	}
	if got.ID != "pay_1" || p.callCount() != 3 {
		t.Errorf("payment %q after %d calls, want pay_1 after 3", got.ID, p.callCount())
	}
	want := []time.Duration{100 * time.Millisecond, 200 * time.Millisecond}
	if !equalDurations(*waits, want) {
		t.Errorf("waits = %v, want %v", *waits, want)
	}
}

func TestRetryStopsAtMaxAttempts(t *testing.T) {
	t.Parallel()
	p := newFlaky(10, &Error{Kind: ErrRateLimited})
	c, _ := newTestClient(t, p, WithRetry(testPolicy))

	_, err := c.GetPayment(context.Background(), "pay_1")
	if !errors.Is(err, ErrRateLimited) || !strings.HasPrefix(err.Error(), "get payment: ") {
		t.Errorf("GetPayment() error = %v, want prefixed ErrRateLimited", err)
	}
	if p.callCount() != 3 {
		t.Errorf("calls = %d, want 3", p.callCount())
	}
}

func TestNoRetryWithoutOption(t *testing.T) {
	t.Parallel()
	p := newFlaky(1, ErrProviderUnavailable)
	c, waits := newTestClient(t, p)

	if _, err := c.GetPayment(context.Background(), "pay_1"); !errors.Is(err, ErrProviderUnavailable) {
		t.Errorf("GetPayment() error = %v, want ErrProviderUnavailable", err)
	}
	if p.callCount() != 1 || len(*waits) != 0 {
		t.Errorf("calls = %d, waits = %v; want one call and no waits", p.callCount(), *waits)
	}
}

func TestNoRetryOnPermanentError(t *testing.T) {
	t.Parallel()
	p := newFlaky(1, &Error{Kind: ErrCardDeclined})
	c, _ := newTestClient(t, p, WithRetry(testPolicy))

	if _, err := c.GetPayment(context.Background(), "pay_1"); !errors.Is(err, ErrCardDeclined) {
		t.Errorf("GetPayment() error = %v, want ErrCardDeclined", err)
	}
	if p.callCount() != 1 {
		t.Errorf("calls = %d, want 1", p.callCount())
	}
}

func TestWriteRetryRequiresHonoredIdempotencyKey(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		idempotent bool // provider implements IdempotentProvider
		honors     bool
		key        string
		wantCalls  int
	}{
		{"honored key", true, true, "idem_1", 3},
		{"no key", true, true, "", 1},
		{"provider does not honor keys", true, false, "idem_1", 1},
		{"provider without the interface", false, false, "idem_1", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			flaky := newFlaky(10, ErrProviderUnavailable)
			flaky.honorsKeys = tt.honors
			var p Provider = flaky
			if tt.idempotent {
				p = idempotentFlaky{flaky}
			}
			c, _ := newTestClient(t, p, WithRetry(testPolicy))

			req := NewPaymentRequest(USD(1000))
			if tt.key != "" {
				req.WithIdempotencyKey(tt.key)
			}
			_, _ = c.CreatePayment(context.Background(), req)
			if flaky.callCount() != tt.wantCalls {
				t.Errorf("calls = %d, want %d", flaky.callCount(), tt.wantCalls)
			}
		})
	}
}

func TestCaptureIsNeverRetried(t *testing.T) {
	t.Parallel()
	flaky := newFlaky(10, ErrProviderUnavailable)
	flaky.honorsKeys = true
	c, _ := newTestClient(t, idempotentFlaky{flaky}, WithRetry(testPolicy))

	// A partial capture resent after a lost response could capture twice.
	_, _ = c.CapturePayment(context.Background(), "pay_1", USD(500))
	if flaky.callCount() != 1 {
		t.Errorf("calls = %d, want 1", flaky.callCount())
	}
}

func TestRetryHonorsRetryAfter(t *testing.T) {
	t.Parallel()
	p := newFlaky(1, &Error{Kind: ErrRateLimited, RetryAfter: 700 * time.Millisecond})
	c, waits := newTestClient(t, p, WithRetry(testPolicy))

	if _, err := c.GetPayment(context.Background(), "pay_1"); err != nil {
		t.Fatalf("GetPayment() error = %v", err)
	}
	if !equalDurations(*waits, []time.Duration{700 * time.Millisecond}) {
		t.Errorf("waits = %v, want [700ms]", *waits)
	}
}

func TestRetryGivesUpWhenRetryAfterExceedsMaxDelay(t *testing.T) {
	t.Parallel()
	p := newFlaky(1, &Error{Kind: ErrRateLimited, RetryAfter: 30 * time.Second})
	c, waits := newTestClient(t, p, WithRetry(testPolicy))

	_, err := c.GetPayment(context.Background(), "pay_1")
	if !errors.Is(err, ErrRateLimited) || RetryAfter(err) != 30*time.Second {
		t.Errorf("GetPayment() error = %v, want ErrRateLimited carrying the 30s hint", err)
	}
	if p.callCount() != 1 || len(*waits) != 0 {
		t.Errorf("calls = %d, waits = %v; want one call and no waits", p.callCount(), *waits)
	}
}

func TestRetryGivesUpBeforeDeadline(t *testing.T) {
	t.Parallel()
	p := newFlaky(1, ErrProviderUnavailable)
	c, waits := newTestClient(t, p, WithRetry(RetryPolicy{BaseDelay: time.Minute, MaxDelay: time.Minute}))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err := c.GetPayment(ctx, "pay_1"); !errors.Is(err, ErrProviderUnavailable) {
		t.Errorf("GetPayment() error = %v, want ErrProviderUnavailable", err)
	}
	if p.callCount() != 1 || len(*waits) != 0 {
		t.Errorf("calls = %d, waits = %v; want no wait past the deadline", p.callCount(), *waits)
	}
}

func TestRetryCanceledDuringWait(t *testing.T) {
	t.Parallel()
	p := newFlaky(10, ErrProviderUnavailable)
	c, _ := newTestClient(t, p, WithRetry(testPolicy))
	c.sleep = func(context.Context, time.Duration) error { return context.Canceled }

	_, err := c.GetPayment(context.Background(), "pay_1")
	if !errors.Is(err, context.Canceled) || IsRetryable(err) {
		t.Errorf("GetPayment() error = %v, want non-retryable context.Canceled", err)
	}
	if !strings.Contains(err.Error(), "last provider error") {
		t.Errorf("error %q should keep the provider error for diagnostics", err)
	}
}

func TestRetryBackoffIsCapped(t *testing.T) {
	t.Parallel()
	p := newFlaky(10, ErrProviderUnavailable)
	c, waits := newTestClient(t, p, WithRetry(RetryPolicy{MaxAttempts: 6, BaseDelay: time.Second, MaxDelay: 3 * time.Second}))

	_, _ = c.GetPayment(context.Background(), "pay_1")
	want := []time.Duration{time.Second, 2 * time.Second, 3 * time.Second, 3 * time.Second, 3 * time.Second}
	if !equalDurations(*waits, want) {
		t.Errorf("waits = %v, want %v", *waits, want)
	}
}

func TestRetryWaitJitterBounds(t *testing.T) {
	t.Parallel()
	c, err := NewClient(NewMockProvider(), WithRetry(RetryPolicy{BaseDelay: 200 * time.Millisecond}))
	if err != nil {
		t.Fatal(err)
	}
	for range 1000 {
		wait, ok := c.retryWait(context.Background(), 1, ErrProviderUnavailable)
		if !ok || wait < 100*time.Millisecond || wait > 200*time.Millisecond {
			t.Fatalf("retryWait() = %v, %v; want within [100ms, 200ms]", wait, ok)
		}
	}
}

func TestWithTimeoutBoundsCallWithoutDeadline(t *testing.T) {
	t.Parallel()
	p := newFlaky(0, nil)
	c, _ := newTestClient(t, p, WithTimeout(time.Minute))

	before := time.Now()
	if _, err := c.GetPayment(context.Background(), "pay_1"); err != nil {
		t.Fatal(err)
	}
	if p.deadline.IsZero() || p.deadline.Before(before.Add(time.Minute)) || p.deadline.After(time.Now().Add(time.Minute)) {
		t.Errorf("provider saw deadline %v, want about one minute from the call", p.deadline)
	}
}

func TestWithTimeoutKeepsCallerDeadline(t *testing.T) {
	t.Parallel()
	p := newFlaky(0, nil)
	c, _ := newTestClient(t, p, WithTimeout(time.Minute))

	ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
	defer cancel()
	want, _ := ctx.Deadline()
	if _, err := c.GetPayment(ctx, "pay_1"); err != nil {
		t.Fatal(err)
	}
	if !p.deadline.Equal(want) {
		t.Errorf("provider saw deadline %v, want the caller's %v", p.deadline, want)
	}
}

func TestWithLoggerReportsRetries(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	p := newFlaky(1, &Error{Kind: ErrProviderUnavailable, StatusCode: 503, RequestID: "req_9"})
	c, _ := newTestClient(t, p, WithRetry(testPolicy), WithLogger(logger))

	if _, err := c.GetPayment(context.Background(), "pay_1"); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"level=WARN", `msg="gopay: retrying provider call"`, `op="get payment"`, "attempt=1", "status=503", "request_id=req_9"} {
		if !strings.Contains(out, want) {
			t.Errorf("log %q missing %q", out, want)
		}
	}
}

func TestNewClientOptionValidation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		opt     ClientOption
		wantErr bool
	}{
		{"defaults", WithRetry(RetryPolicy{}), false},
		{"retries disabled", WithRetry(RetryPolicy{MaxAttempts: 1}), false},
		{"negative attempts", WithRetry(RetryPolicy{MaxAttempts: -1}), true},
		{"negative delay", WithRetry(RetryPolicy{BaseDelay: -time.Second}), true},
		{"max below base", WithRetry(RetryPolicy{BaseDelay: time.Second, MaxDelay: time.Millisecond}), true},
		{"negative timeout", WithTimeout(-time.Second), true},
		{"nil logger ignored", WithLogger(nil), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := NewClient(NewMockProvider(), tt.opt)
			if gotErr := errors.Is(err, ErrInvalidConfig); gotErr != tt.wantErr {
				t.Errorf("NewClient() error = %v, want ErrInvalidConfig: %v", err, tt.wantErr)
			}
		})
	}

	c, err := NewClient(NewMockProvider(), WithRetry(RetryPolicy{}))
	if err != nil {
		t.Fatal(err)
	}
	if c.retry != DefaultRetryPolicy() {
		t.Errorf("zero policy = %+v, want DefaultRetryPolicy()", c.retry)
	}
}

func equalDurations(a, b []time.Duration) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
