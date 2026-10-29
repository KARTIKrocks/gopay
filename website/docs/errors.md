---
id: errors
title: Errors
description: Sentinel errors and the error-translation pattern each provider uses to translate SDK errors consistently.
---

# Errors

Every provider translates its SDK's own errors into gopay's sentinel errors,
so callers write one `errors.Is` check that works across Stripe, PayPal, and
Razorpay instead of branching on each SDK's error types.

```go
p, err := client.CreatePayment(ctx, req)
if errors.Is(err, payment.ErrCardDeclined) {
    // handle declined card
} else if errors.Is(err, payment.ErrInsufficientFunds) {
    // handle insufficient funds
} else if errors.Is(err, payment.ErrInvalidAmount) {
    // handle invalid amount
}
```

Errors returned by `Client` methods are wrapped with `fmt.Errorf("...: %w",
err)` (e.g. `"create payment: gopay: card declined"`), so `errors.Is` still
matches through the wrapping.

## Sentinel errors

| Error | Meaning |
| --- | --- |
| `ErrInvalidConfig` | Provider configuration is invalid (e.g. missing API key) |
| `ErrInvalidAmount` | Amount is negative or fails validation |
| `ErrInvalidCurrency` | Currency is empty or not in the accepted ISO 4217 set |
| `ErrInvalidCard` | Card details are invalid |
| `ErrCardDeclined` | The card was declined |
| `ErrInsufficientFunds` | The card had insufficient funds |
| `ErrExpiredCard` | The card has expired |
| `ErrPaymentFailed` | The payment failed for another reason |
| `ErrRefundFailed` | The refund failed |
| `ErrSetupFailed` | The setup intent failed |
| `ErrSubscriptionFailed` | The subscription operation failed |
| `ErrNotFound` | The requested resource doesn't exist (also returned for an empty ID) |
| `ErrAlreadyRefunded` | The payment was already fully refunded |
| `ErrAlreadyCaptured` | The payment was already captured |
| `ErrAuthenticationRequired` | Additional authentication (e.g. 3DS) is required |
| `ErrProviderError` | An unmapped provider-side error |
| `ErrRateLimited` | The provider throttled the request (HTTP 429). Transient |
| `ErrProviderUnavailable` | Network failure, timeout, or provider 5xx. Transient |
| `ErrInvalidCredentials` | The provider rejected the API credentials (HTTP 401) |
| `ErrUnsupported` | The provider doesn't implement the optional capability called |

`ErrRateLimited`, `ErrProviderUnavailable`, and `ErrInvalidCredentials` are
refinements of `ErrProviderError` and wrap it, so an existing
`errors.Is(err, payment.ErrProviderError)` check still matches them.

## Transient errors and retries

`payment.IsRetryable(err)` reports whether a failure is transient — rate
limited or provider unavailable — and may succeed if the same request is sent
again. `payment.RetryAfter(err)` returns the wait the provider asked for, or 0.

```go
_, err := client.GetPayment(ctx, id)
if payment.IsRetryable(err) {
    time.Sleep(max(payment.RetryAfter(err), time.Second))
    // retry
}
```

`IsRetryable` classifies the failure only. Whether a retry is _safe_ depends
on the operation: reads always are, but a create without an idempotency key
can run twice. Declines, validation failures, and bad credentials are never
retryable. A call whose own `ctx` was canceled or timed out returns the context
error, which is not retryable either.

## Structured error details

Errors that come from a provider call are a `*payment.Error`, which carries
the provider's own diagnostics alongside the normalized kind:

```go
var perr *payment.Error
if errors.As(err, &perr) {
    log.Printf("provider=%s status=%d code=%s request_id=%s",
        perr.Provider, perr.StatusCode, perr.Code, perr.RequestID)
}
```

| Field | Content |
| --- | --- |
| `Kind` | The sentinel above that classifies the failure |
| `Provider` | `"stripe"`, `"paypal"`, or `"razorpay"` |
| `StatusCode` | HTTP status, or 0 when no response arrived |
| `Code` | The provider's most specific code (Stripe's decline code, PayPal's issue, Razorpay's error code) — for logs, not control flow |
| `Message` | The provider's human-readable message |
| `RequestID` | Stripe's request ID or PayPal's debug ID, when available |
| `RetryAfter` | The provider's `Retry-After` hint, or 0. PayPal and Razorpay only: stripe-go does not expose response headers |
| `Err` | The underlying cause: the SDK error or network error |

Validation errors raised by gopay before any provider call are plain sentinels,
not `*payment.Error`.

## `ErrUnsupported` is the one to check across providers

Because optional capabilities (customers, subscriptions, listing, ...) are
gated per-provider (see [Client](./client.md)), `ErrUnsupported` is the error
you'll see most often when writing provider-agnostic code:

```go
_, err := client.ListPayments(ctx, params)
if errors.Is(err, payment.ErrUnsupported) {
    // this provider (e.g. PayPal) has no list endpoint
}
```

## The error-translation pattern

Each provider package implements its own unexported error-translation method
— `mapError` in Stripe, `parseError` in PayPal and Razorpay — that returns a
`*payment.Error`. Its `Kind` starts from the HTTP status via
`payment.KindForStatus` (401 → `ErrInvalidCredentials`, 404 →
`ErrNotFound`, 429 → `ErrRateLimited`, 5xx → `ErrProviderUnavailable`, else
`ErrProviderError`) and is refined by the provider's own decline or error
code. A decline is never classified as transient, whatever status accompanies
it. Network failures become `ErrProviderUnavailable`.

Every provider method funnels its errors through that function, so callers
branch on gopay's kinds, never on a provider's error type. The original SDK
error stays reachable with `errors.As` through `Err` as an escape hatch.
Third-party providers can use `payment.KindForStatus` and
`payment.ParseRetryAfter` to classify errors the same way.
