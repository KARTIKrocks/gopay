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
| `ErrUnsupported` | The provider doesn't implement the optional capability called |

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
— `mapError` in Stripe, `parseError` in PayPal and Razorpay — that inspects
the underlying SDK error (HTTP status code, decline code, provider-specific
error type) and returns the matching gopay sentinel, falling back to
`ErrProviderError` for anything it doesn't recognize. Every provider method
funnels its SDK error through that function before returning, so no raw
provider error type ever reaches a caller through the `Client`.
