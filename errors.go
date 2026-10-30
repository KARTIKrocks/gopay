package gopay

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Sentinel errors for payment operations.
var (
	ErrInvalidConfig          = errors.New("gopay: invalid configuration")
	ErrInvalidAmount          = errors.New("gopay: invalid amount")
	ErrInvalidCurrency        = errors.New("gopay: invalid currency")
	ErrInvalidCard            = errors.New("gopay: invalid card")
	ErrCardDeclined           = errors.New("gopay: card declined")
	ErrInsufficientFunds      = errors.New("gopay: insufficient funds")
	ErrExpiredCard            = errors.New("gopay: expired card")
	ErrPaymentFailed          = errors.New("gopay: payment failed")
	ErrRefundFailed           = errors.New("gopay: refund failed")
	ErrSetupFailed            = errors.New("gopay: setup failed")
	ErrSubscriptionFailed     = errors.New("gopay: subscription failed")
	ErrNotFound               = errors.New("gopay: not found")
	ErrAlreadyRefunded        = errors.New("gopay: already refunded")
	ErrAlreadyCaptured        = errors.New("gopay: already captured")
	ErrAuthenticationRequired = errors.New("gopay: authentication required")
	ErrProviderError          = errors.New("gopay: provider error")
	ErrUnsupported            = errors.New("gopay: operation not supported")
)

// Refinements of [ErrProviderError]. Each wraps ErrProviderError, so code that
// matches errors.Is(err, ErrProviderError) keeps matching these failures, which
// were all reported as a plain ErrProviderError before they were split out.
var (
	// ErrRateLimited reports that the provider throttled the request (HTTP 429).
	// It is transient; see [IsRetryable] and [RetryAfter].
	ErrRateLimited = fmt.Errorf("%w: rate limited", ErrProviderError)

	// ErrProviderUnavailable reports that the provider could not be reached or
	// failed on its side: a network error, a per-attempt timeout, or an HTTP 5xx.
	// It is transient; see [IsRetryable].
	ErrProviderUnavailable = fmt.Errorf("%w: provider unavailable", ErrProviderError)

	// ErrInvalidCredentials reports that the provider rejected the configured
	// API credentials (HTTP 401). Retrying will not help.
	ErrInvalidCredentials = fmt.Errorf("%w: invalid credentials", ErrProviderError)
)

// Error is the structured error a provider returns when a provider call fails.
// It carries the provider's own diagnostics alongside a normalized Kind.
//
// Match the Kind with [errors.Is] and reach the details with [errors.As]:
//
//	if errors.Is(err, gopay.ErrCardDeclined) { ... }
//
//	var perr *gopay.Error
//	if errors.As(err, &perr) {
//	    log.Printf("provider=%s code=%s request_id=%s", perr.Provider, perr.Code, perr.RequestID)
//	}
//
// Errors raised by gopay itself before any provider call, such as request
// validation failures, are plain sentinel errors, not *Error.
type Error struct {
	// Kind is the sentinel error that classifies the failure, such as
	// [ErrCardDeclined] or [ErrRateLimited]. A nil Kind is treated as
	// [ErrProviderError].
	Kind error

	// Provider is the name of the provider that returned the error, as
	// reported by [Provider.Name].
	Provider string

	// StatusCode is the HTTP status of the provider response, or 0 when no
	// response was received (for example, on a network error).
	StatusCode int

	// Code is the provider's most specific error code, such as Stripe's
	// "card_declined" or Razorpay's "BAD_REQUEST_ERROR". It is provider-specific
	// and intended for logging and diagnostics, not for control flow; match on
	// Kind instead.
	Code string

	// Message is the provider's human-readable error message.
	Message string

	// RequestID is the provider's identifier for the failed request, when
	// available. Include it when contacting the provider's support.
	RequestID string

	// RetryAfter is how long the provider asked the caller to wait before
	// retrying, or 0 when it gave no hint.
	RetryAfter time.Duration

	// Err is the underlying cause, such as a network or SDK error, when there
	// is one.
	Err error
}

// Error returns the Kind's message followed by the provider's message, for
// example "gopay: card declined: Your card was declined.". Without a message
// it falls back to the underlying cause, then to the provider's code.
func (e *Error) Error() string {
	msg := e.kind().Error()
	switch {
	case e.Message != "":
		return msg + ": " + e.Message
	case e.Err != nil:
		return msg + ": " + e.Err.Error()
	case e.Code != "":
		return msg + ": " + e.Code
	default:
		return msg
	}
}

// Unwrap returns the Kind and, when present, the underlying cause, so that
// [errors.Is] and [errors.As] match either of them.
func (e *Error) Unwrap() []error {
	if e.Err == nil {
		return []error{e.kind()}
	}
	return []error{e.kind(), e.Err}
}

func (e *Error) kind() error {
	if e.Kind == nil {
		return ErrProviderError
	}
	return e.Kind
}

// IsRetryable reports whether err is a transient provider failure, one that
// may succeed if the same request is sent again: [ErrRateLimited] or
// [ErrProviderUnavailable].
//
// It classifies the failure only. Whether resending is safe also depends on
// the operation; a create without an idempotency key may run twice.
func IsRetryable(err error) bool {
	return errors.Is(err, ErrRateLimited) || errors.Is(err, ErrProviderUnavailable)
}

// RetryAfter returns the wait the provider requested before a retry, taken
// from the first [*Error] in err's chain. It returns 0 when there is no such
// error or the provider gave no hint.
func RetryAfter(err error) time.Duration {
	if perr, ok := errors.AsType[*Error](err); ok {
		return perr.RetryAfter
	}
	return 0
}

// KindForStatus returns the default error Kind for an HTTP status code from a
// provider: [ErrInvalidCredentials] for 401, [ErrNotFound] for 404,
// [ErrRateLimited] for 429, [ErrProviderUnavailable] for 408 and for 5xx other
// than 501 and 505, and [ErrProviderError] for anything else. A 403 is
// ErrProviderError: it usually means the credentials are valid but lack
// permission for the operation, so it must not prompt a key rotation.
//
// It is a helper for [Provider] implementations, which should prefer a more
// specific Kind when the provider's error code identifies one, such as a
// card decline.
func KindForStatus(status int) error {
	switch {
	case status == http.StatusUnauthorized:
		return ErrInvalidCredentials
	case status == http.StatusNotFound:
		return ErrNotFound
	case status == http.StatusTooManyRequests:
		return ErrRateLimited
	case status == http.StatusRequestTimeout:
		return ErrProviderUnavailable
	case status == http.StatusNotImplemented, status == http.StatusHTTPVersionNotSupported:
		// Permanent server-side refusals: resending the request cannot help.
		return ErrProviderError
	case status >= 500 && status <= 599:
		return ErrProviderUnavailable
	default:
		return ErrProviderError
	}
}

// maxRetryAfter caps the wait ParseRetryAfter reports.
const maxRetryAfter = 24 * time.Hour

// ParseRetryAfter parses the value of an HTTP Retry-After header, given either
// as delay-seconds or as an HTTP date relative to now. It returns 0 for an
// empty or malformed value, and for a date that is not in the future. The
// result is capped at 24 hours, so a misbehaving server cannot stall a caller
// that sleeps for it.
//
// It is a helper for [Provider] implementations filling [Error.RetryAfter].
func ParseRetryAfter(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if secs, err := strconv.ParseInt(value, 10, 64); err == nil {
		if secs <= 0 {
			return 0
		}
		// Clamp before converting, so absurd values cannot overflow Duration.
		return time.Duration(min(secs, int64(maxRetryAfter/time.Second))) * time.Second
	}
	if at, err := http.ParseTime(value); err == nil {
		if d := at.Sub(now); d > 0 {
			return min(d, maxRetryAfter)
		}
	}
	return 0
}
