---
id: retries
title: Retries & timeouts
description: Opt-in retries of transient provider failures, a default call timeout, and retry logging — and which operations are safe to resend.
---

# Retries & timeouts

_Added in 0.10._ `Client` can retry transient provider failures — rate limits,
network errors, provider 5xx — and bound calls with a default timeout. Both
are off unless you ask for them:

```go
client, err := payment.NewClient(provider,
    payment.WithRetry(payment.RetryPolicy{}), // defaults: 3 attempts, 250ms–5s backoff
    payment.WithTimeout(30*time.Second),
    payment.WithLogger(slog.Default()),
)
```

`NewClient` returns `ErrInvalidConfig` for a negative or inconsistent policy.

## What gets retried

A call is retried only when its error is transient — `payment.IsRetryable(err)`
is true, meaning `ErrRateLimited` or `ErrProviderUnavailable` (see
[Errors](./errors.md)) — **and** resending it is safe:

| Operation | Retried? |
| --- | --- |
| Reads: `Get…`, `List…` | Always |
| `CreatePayment`, `Refund`, `CreateSetupIntent`, `CreatePlan`, `CreateSubscription` | Only with an `IdempotencyKey`, and only on a provider that honors it |
| `CapturePayment`, `CancelPayment`, `FullRefund`, customer and payment-method writes, cancellations, `VerifyWebhook` | Never |

Declines, validation failures, and bad credentials are never retried.

**Set an idempotency key on every write you want retried.** When a response is
lost, the write may already have happened; the provider only recognizes the
resend as a repeat if it carries the same key:

```go
req := payment.NewPaymentRequest(payment.USD(1999)).
    WithIdempotencyKey(orderID) // stable per logical payment, not per attempt
p, err := client.CreatePayment(ctx, req)
```

Writes without a key are never resent: a capture or cancellation whose
response was lost could otherwise run twice — a PayPal partial capture really
would capture twice.

### Providers and idempotency

A provider declares that it honors idempotency keys by implementing
`payment.IdempotentProvider`:

| Provider | Honors keys | Effect |
| --- | --- | --- |
| Stripe | Yes — `Idempotency-Key` header | Keyed writes are retried |
| PayPal | Yes — `PayPal-Request-Id` header | Keyed writes are retried |
| Razorpay | No — its API has no idempotency key | Writes are never retried; reads are |

A third-party provider should implement `IdempotentProvider` only if it
forwards the key of every request type it implements to an API that
deduplicates on it.

## Backoff

The wait before retry _n_ is `BaseDelay × 2ⁿ⁻¹`, capped at `MaxDelay`, then
jittered to between half and all of that value so concurrent clients spread
out. `RetryPolicy` fields left at zero take their value from
`payment.DefaultRetryPolicy()`, except that a default delay never contradicts
one you set: `RetryPolicy{BaseDelay: 10 * time.Second}` raises `MaxDelay` to
10s, and `RetryPolicy{MaxDelay: 100 * time.Millisecond}` lowers `BaseDelay` to
100ms.

A provider's `Retry-After` hint is honored when it is longer than the backoff.
If it asks for more than `MaxDelay`, the client returns the error instead of
waiting; read the requested wait with `payment.RetryAfter(err)` and schedule
the retry yourself.

The client never waits past the context's deadline: if the next wait would end
after it, the client returns the last error at once. If the context is
canceled during a wait, the call returns the context error, with the last
provider error kept in its message.

## Timeouts

`WithTimeout(d)` bounds each call whose context has **no** deadline, covering
every attempt and the waits between them. A context that already has a
deadline is used as is. Each provider also sets a per-request HTTP timeout of
its own (30s by default).

## Logging

`WithLogger` reports each retry at `slog.LevelWarn` as
`gopay: retrying provider call`, with `op`, `provider`, `attempt`, `wait`,
`error`, and, for provider errors, `status` and `request_id`. Final errors are
returned, not logged. Without a logger the client logs nothing.

## Stripe's built-in retries

stripe-go retries failed requests itself, twice by default. Under a retrying
`Client`, turn that off so the two layers do not multiply (3 × 3 = 9 requests):

```go
provider, err := stripe.NewProvider(stripe.DefaultConfig().
    WithSecretKey(key).
    WithMaxNetworkRetries(0))
```

Stripe stores the response to a keyed write for 24 hours, including a `500`,
and replays it for any request with the same key. A keyed write that fails
with a `500` therefore fails the same way on every retry. That costs only the
extra attempts, never a duplicate charge. stripe-go doesn't expose the
`Stripe-Should-Retry` header that would let gopay tell these failures apart.
`429`s and network errors are not stored, so retrying them works as intended.
