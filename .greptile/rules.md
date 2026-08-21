# Style and pattern rationale

Context for the scoped rules in `config.json`. This file is freeform prose read
alongside the diff; `config.json` is what actually gates comment scope and
severity.

## Sign loss near zero — a recurring bug class

`decimal-sign-near-zero` exists because the exact same bug has been fixed
twice in this codebase, in both conversion directions:

- v0.1.1: PayPal's `toDecimal` rendered `-50` (minor units) as `0.50` instead
  of `-0.50` — the sign was derived from the integer whole-units part, which
  is `0` for any amount smaller than one major unit, silently dropping the
  sign for the entire `(-1.00, 0)` range.
- v0.6.1: PayPal's `fromDecimal` had the inverse bug going the other way —
  `"-0.99"` parsed back to `99` instead of `-99` minor units, for the same
  reason: the sign lived on the now-zero whole part.

Any code converting between a decimal string and a minor-unit integer
`Amount` needs to extract the sign once, up front, independent of whether the
whole-number part happens to be zero — and needs a test case in the
`(-1.00, 0)` range specifically, since that's exactly the range prior tests
missed both times.

## Case-sensitive header lookups breaking signature verification

`webhook-signature-verification`'s header-casing clause also comes from two
separate fixes:

- v0.1.1: Stripe's `Stripe-Signature` lookup was case-sensitive and could miss
  the header after Go's `net/http` canonicalizes incoming header names.
- v0.6.1: PayPal's `PAYPAL-*` transmission-header lookup had the same bug.

Both are now fixed with explicit case-insensitive comparison (see
`headerValue` in `paypal.go`, `strings.EqualFold` in `stripe.go`). A new or
modified header lookup for anything security-relevant (signatures, auth
tokens) needs the same treatment — direct map indexing (`headers["X-Foo"]`)
on caller-supplied header maps is the shape of this bug.

## Idempotency keys are already wired — keep new call sites wired too

`PaymentRequest`, `RefundRequest`, `PlanRequest`, and `SubscriptionRequest`
all carry `IdempotencyKey` via `WithIdempotencyKey`, and every provider
threads it to the underlying API differently: Stripe calls
`params.SetIdempotencyKey`, PayPal sets the `PayPal-Request-Id` header,
Razorpay uses the order's `Receipt` field (Razorpay has no native idempotency
header — `Receipt` is the closest analogue and is what the existing code
uses). A new mutating operation on an existing or new provider needs the same
treatment; if the underlying provider truly has no idempotency mechanism, say
so explicitly rather than silently dropping the key.

## Provider parity is the contract, same as objstore's backend parity

`Client` lets callers write provider-agnostic code by dispatching through
optional interfaces (`CustomerProvider`, `WebhookProvider`, `ListProvider`,
etc.) and normalizing results into shared types (`WebhookEvent`, `List[T]`).
A field populated by one provider's mapping but left zero by another's for
the equivalent event is invisible in that provider's own tests (which only
check that provider) but breaks a caller who switches providers — exactly
what happened with `WebhookEvent.InvoiceID`, populated for Stripe but missed
for Razorpay's `subscription.charged` until a later fix. When reviewing a
change to one provider's mapping/normalization logic, check whether the other
providers implementing the same optional interface need the equivalent
change.

## Deliberate gaps are documented, not oversights

`feature_gaps.md` explains, per capability, which providers intentionally
return `ErrUnsupported` and why — e.g. PayPal has no charge-free card-save
flow so it doesn't implement `SetupIntentProvider` at all, and forcing the
abstraction would risk silently authorizing or charging a customer who
expected no charge. Don't flag a provider's non-implementation of an optional
interface as a missing feature without checking `feature_gaps.md` first.

## Multi-module boundaries

Four Go modules (root, `stripe/`, `paypal/`, `razorpay/`), each its own
`go.mod`, tied together at dev time by the committed `go.work`. A change to a
core type, interface, or sentinel error in the root module needs the matching
change in whichever provider module(s) implement the affected surface, in the
same PR — the root module's own tests won't catch a mismatch, since each
provider module only pulls in the root via its pinned `require` (overridden
locally by `go.work`, never by a `replace`).
