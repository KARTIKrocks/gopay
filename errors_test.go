package gopay

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestRefinedKindsWrapProviderError(t *testing.T) {
	t.Parallel()
	for _, kind := range []error{ErrRateLimited, ErrProviderUnavailable, ErrInvalidCredentials} {
		if !errors.Is(kind, ErrProviderError) {
			t.Errorf("errors.Is(%v, ErrProviderError) = false, want true", kind)
		}
	}
}

func TestErrorMessage(t *testing.T) {
	t.Parallel()
	cause := errors.New("connection reset")
	tests := []struct {
		name string
		err  *Error
		want string
	}{
		{"kind and message", &Error{Kind: ErrCardDeclined, Message: "Your card was declined."}, "gopay: card declined: Your card was declined."},
		{"message preferred over cause", &Error{Kind: ErrProviderUnavailable, Message: "upstream down", Err: cause}, "gopay: provider error: provider unavailable: upstream down"},
		{"cause when no message", &Error{Kind: ErrProviderUnavailable, Err: cause}, "gopay: provider error: provider unavailable: connection reset"},
		{"code when no message or cause", &Error{Kind: ErrPaymentFailed, Code: "BAD_REQUEST_ERROR"}, "gopay: payment failed: BAD_REQUEST_ERROR"},
		{"kind only", &Error{Kind: ErrNotFound}, "gopay: not found"},
		{"nil kind defaults", &Error{Message: "boom"}, "gopay: provider error: boom"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.err.Error(); got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestErrorUnwrap(t *testing.T) {
	t.Parallel()
	cause := &causeError{}
	perr := &Error{Kind: ErrProviderUnavailable, Provider: "stripe", Err: cause}
	wrapped := fmt.Errorf("create payment: %w", perr)

	if !errors.Is(wrapped, ErrProviderUnavailable) {
		t.Error("errors.Is(wrapped, ErrProviderUnavailable) = false, want true")
	}
	if !errors.Is(wrapped, ErrProviderError) {
		t.Error("errors.Is(wrapped, ErrProviderError) = false, want true")
	}
	if got, ok := errors.AsType[*causeError](wrapped); !ok || got != cause {
		t.Error("errors.As did not reach the underlying cause")
	}
	if got, ok := errors.AsType[*Error](wrapped); !ok || got.Provider != "stripe" {
		t.Error("errors.As did not reach *Error")
	}

	if !errors.Is(&Error{}, ErrProviderError) {
		t.Error("nil Kind should unwrap to ErrProviderError")
	}
}

type causeError struct{}

func (*causeError) Error() string { return "i/o timeout" }

func TestIsRetryable(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"rate limited sentinel", ErrRateLimited, true},
		{"unavailable sentinel", ErrProviderUnavailable, true},
		{"rate limited structured", &Error{Kind: ErrRateLimited}, true},
		{"unavailable wrapped", fmt.Errorf("op: %w", &Error{Kind: ErrProviderUnavailable}), true},
		{"generic provider error", &Error{Kind: ErrProviderError}, false},
		{"invalid credentials", &Error{Kind: ErrInvalidCredentials}, false},
		{"card declined", &Error{Kind: ErrCardDeclined}, false},
		{"not found", ErrNotFound, false},
		{"validation", ErrInvalidAmount, false},
		{"context canceled", context.Canceled, false},
		{"unclassified error", errors.New("boom"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := IsRetryable(tt.err); got != tt.want {
				t.Errorf("IsRetryable(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestRetryAfter(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		err  error
		want time.Duration
	}{
		{"nil", nil, 0},
		{"plain sentinel", ErrRateLimited, 0},
		{"structured", &Error{Kind: ErrRateLimited, RetryAfter: 3 * time.Second}, 3 * time.Second},
		{"wrapped", fmt.Errorf("op: %w", &Error{Kind: ErrRateLimited, RetryAfter: time.Second}), time.Second},
		{"no hint", &Error{Kind: ErrRateLimited}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := RetryAfter(tt.err); got != tt.want {
				t.Errorf("RetryAfter() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestKindForStatus(t *testing.T) {
	t.Parallel()
	tests := []struct {
		status int
		want   error
	}{
		{http.StatusBadRequest, ErrProviderError},
		{http.StatusUnauthorized, ErrInvalidCredentials},
		{http.StatusPaymentRequired, ErrProviderError},
		{http.StatusForbidden, ErrProviderError},
		{http.StatusNotFound, ErrNotFound},
		{http.StatusRequestTimeout, ErrProviderUnavailable},
		{http.StatusConflict, ErrProviderError},
		{http.StatusUnprocessableEntity, ErrProviderError},
		{http.StatusTooManyRequests, ErrRateLimited},
		{http.StatusInternalServerError, ErrProviderUnavailable},
		{http.StatusNotImplemented, ErrProviderError},
		{http.StatusBadGateway, ErrProviderUnavailable},
		{http.StatusServiceUnavailable, ErrProviderUnavailable},
		{http.StatusGatewayTimeout, ErrProviderUnavailable},
		{http.StatusHTTPVersionNotSupported, ErrProviderError},
		{599, ErrProviderUnavailable},
		{0, ErrProviderError},
	}
	for _, tt := range tests {
		t.Run(http.StatusText(tt.status), func(t *testing.T) {
			t.Parallel()
			// Compare identity, not errors.Is: every refined kind also Is
			// ErrProviderError, which would hide a wrong mapping.
			if got := KindForStatus(tt.status); got != tt.want {
				t.Errorf("KindForStatus(%d) = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}

func TestParseRetryAfter(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name  string
		value string
		want  time.Duration
	}{
		{"empty", "", 0},
		{"seconds", "5", 5 * time.Second},
		{"padded seconds", " 2 ", 2 * time.Second},
		{"zero", "0", 0},
		{"negative", "-3", 0},
		{"clamped", "999999999999", 24 * time.Hour},
		{"http date future", now.Add(90 * time.Second).Format(http.TimeFormat), 90 * time.Second},
		{"http date past", now.Add(-time.Minute).Format(http.TimeFormat), 0},
		{"http date clamped", now.AddDate(73, 0, 0).Format(http.TimeFormat), 24 * time.Hour},
		{"malformed", "soon", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := ParseRetryAfter(tt.value, now); got != tt.want {
				t.Errorf("ParseRetryAfter(%q) = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}
