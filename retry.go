package gopay

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"time"
)

// ClientOption configures a [Client]. Pass options to [NewClient].
type ClientOption func(*Client)

// IdempotentProvider is implemented by providers that forward the
// IdempotencyKey of every request type they implement to their API, which
// then performs repeated requests with the same key at most once.
//
// A [Client] configured with [WithRetry] resends a failed write only when its
// provider implements this interface, HonorsIdempotencyKeys returns true, and
// the request carries a key. Without that guarantee a resent write could
// charge or refund twice.
type IdempotentProvider interface {
	Provider

	// HonorsIdempotencyKeys reports whether the provider deduplicates
	// requests by their IdempotencyKey.
	HonorsIdempotencyKeys() bool
}

// RetryPolicy configures how a [Client] retries transient provider failures,
// those for which [IsRetryable] is true.
//
// Reads are always eligible for a retry. A write is eligible only when its
// request carries an IdempotencyKey and the provider honors it (see
// [IdempotentProvider]); writes without one, such as captures, cancellations,
// and deletions, are never retried.
type RetryPolicy struct {
	// MaxAttempts is the total number of attempts, including the first. A
	// value of 1 disables retries.
	MaxAttempts int

	// BaseDelay is the backoff before the first retry. Each later retry
	// doubles it, up to MaxDelay. The actual wait is jittered between half
	// the backoff and the full backoff, so concurrent clients spread out.
	BaseDelay time.Duration

	// MaxDelay caps the backoff. It also caps how long the client will honor
	// a provider's Retry-After: when the provider asks for a longer wait, the
	// client returns the error instead of waiting, and the caller can read
	// the requested wait with [RetryAfter].
	MaxDelay time.Duration
}

// DefaultRetryPolicy returns the policy [WithRetry] fills zero fields from:
// 3 attempts, 250ms base delay, 5s maximum delay.
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{MaxAttempts: 3, BaseDelay: 250 * time.Millisecond, MaxDelay: 5 * time.Second}
}

// WithRetry enables retries of transient provider failures under policy.
// Zero fields take their value from [DefaultRetryPolicy], so
// WithRetry(RetryPolicy{}) uses the defaults, except that a default delay
// never contradicts the one you set: a BaseDelay above the default MaxDelay
// raises MaxDelay to match, and a MaxDelay below the default BaseDelay lowers
// BaseDelay to match. Retries are disabled unless this option is given.
//
// The retries happen in gopay, above the provider SDK. Some SDKs retry on
// their own as well; see the provider package for how to turn that off so
// attempts do not multiply.
func WithRetry(policy RetryPolicy) ClientOption {
	return func(c *Client) {
		def := DefaultRetryPolicy()
		if policy.MaxAttempts == 0 {
			policy.MaxAttempts = def.MaxAttempts
		}
		switch {
		case policy.BaseDelay == 0 && policy.MaxDelay == 0:
			policy.BaseDelay, policy.MaxDelay = def.BaseDelay, def.MaxDelay
		case policy.BaseDelay == 0:
			policy.BaseDelay = min(def.BaseDelay, policy.MaxDelay)
		case policy.MaxDelay == 0:
			policy.MaxDelay = max(def.MaxDelay, policy.BaseDelay)
		}
		c.retry = policy
	}
}

// WithTimeout bounds each [Client] call whose context has no deadline,
// covering every attempt and the waits between them. A context that already
// has a deadline is left as is.
func WithTimeout(d time.Duration) ClientOption {
	return func(c *Client) { c.timeout = d }
}

// WithLogger sets the logger the [Client] reports retries to, at
// [slog.LevelWarn]. Without it the client logs nothing.
func WithLogger(logger *slog.Logger) ClientOption {
	return func(c *Client) {
		if logger != nil {
			c.logger = logger
		}
	}
}

// validate reports whether the options applied to c are usable.
func (c *Client) validate() error {
	switch r := c.retry; {
	case r.MaxAttempts < 0, r.BaseDelay < 0, r.MaxDelay < 0:
		return fmt.Errorf("%w: retry policy values must not be negative", ErrInvalidConfig)
	case r.MaxDelay < r.BaseDelay:
		return fmt.Errorf("%w: retry MaxDelay is shorter than BaseDelay", ErrInvalidConfig)
	case c.timeout < 0:
		return fmt.Errorf("%w: timeout must not be negative", ErrInvalidConfig)
	}
	return nil
}

// Resend safety of a Client operation, as passed to do.
const (
	safeToResend = true  // a read: resending it cannot change anything
	neverResend  = false // a write the provider cannot deduplicate
)

// resendable reports whether a write carrying idempotencyKey may be resent.
func (c *Client) resendable(idempotencyKey string) bool {
	ip, ok := c.provider.(IdempotentProvider)
	return idempotencyKey != "" && ok && ip.HonorsIdempotencyKeys()
}

// do runs fn under the client's timeout and, when resendable, its retry
// policy, and prefixes a returned error with op.
func do[T any](ctx context.Context, c *Client, op string, resendable bool, fn func(context.Context) (T, error)) (T, error) {
	if c.timeout > 0 {
		if _, ok := ctx.Deadline(); !ok {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, c.timeout)
			defer cancel()
		}
	}

	attempts := 1
	if resendable {
		attempts = c.retry.MaxAttempts // at least 1: NewClient and WithRetry ensure it
	}

	var zero T
	for attempt := 1; ; attempt++ {
		v, err := fn(ctx)
		if err == nil {
			return v, nil
		}
		if attempt >= attempts || !IsRetryable(err) {
			return zero, fmt.Errorf("%s: %w", op, err)
		}
		wait, ok := c.retryWait(ctx, attempt, err)
		if !ok {
			return zero, fmt.Errorf("%s: %w", op, err)
		}
		c.logRetry(ctx, op, attempt, wait, err)
		if sleepErr := c.sleep(ctx, wait); sleepErr != nil {
			// The caller gave up while we waited: report that, as providers
			// do, keeping the provider's error for diagnostics only.
			return zero, fmt.Errorf("%s: %w (last provider error: %s)", op, sleepErr, err.Error())
		}
	}
}

// doErr is do for operations that return only an error. None of them is safe
// to resend, so only the timeout applies.
func doErr(ctx context.Context, c *Client, op string, fn func(context.Context) error) error {
	_, err := do(ctx, c, op, neverResend, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, fn(ctx)
	})
	return err
}

// retryWait returns how long to wait before the attempt after attempt, or
// false when retrying is pointless: the provider asked for a longer wait
// than the policy allows, or ctx's deadline will pass first.
func (c *Client) retryWait(ctx context.Context, attempt int, err error) (time.Duration, bool) {
	// BaseDelay << shift, capped at MaxDelay. Comparing against
	// MaxDelay >> shift first keeps the shift from overflowing.
	backoff := c.retry.MaxDelay
	if shift := attempt - 1; shift < 63 && c.retry.BaseDelay <= c.retry.MaxDelay>>shift {
		backoff = c.retry.BaseDelay << shift
	}
	wait := backoff/2 + time.Duration(c.jitter(int64(backoff/2)+1))

	if hint := RetryAfter(err); hint > 0 {
		if hint > c.retry.MaxDelay {
			return 0, false
		}
		wait = max(wait, hint)
	}
	if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) < wait {
		return 0, false
	}
	return wait, true
}

func (c *Client) logRetry(ctx context.Context, op string, attempt int, wait time.Duration, err error) {
	attrs := []slog.Attr{
		slog.String("op", op),
		slog.String("provider", c.provider.Name()),
		slog.Int("attempt", attempt),
		slog.Duration("wait", wait),
		slog.String("error", err.Error()),
	}
	if perr, ok := errors.AsType[*Error](err); ok {
		attrs = append(attrs, slog.Int("status", perr.StatusCode), slog.String("request_id", perr.RequestID))
	}
	c.logger.LogAttrs(ctx, slog.LevelWarn, "gopay: retrying provider call", attrs...)
}

// sleepContext waits for d, or until ctx is done, in which case it returns
// ctx's error.
func sleepContext(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// defaultJitter returns a uniform random duration in [0, n).
func defaultJitter(n int64) int64 {
	return rand.Int64N(n) //nolint:gosec // jitter only spreads retries out; it needs no cryptographic randomness
}
