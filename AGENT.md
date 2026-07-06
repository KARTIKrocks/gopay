# AGENT.md

This file provides guidance to AI agents when working with code in this repository.

## Overview

`gopay` is a unified payment-processing library for Go supporting Stripe, PayPal, and Razorpay. The core package (`github.com/KARTIKrocks/gopay`) defines provider-agnostic interfaces, types, sentinel errors, a `Client`, and a `MockProvider`. Each provider lives in its **own Go sub-module** so importing one provider doesn't pull in the SDKs of the others.

## Multi-module layout

This is a **multi-module workspace** — four separate `go.mod` files tied together by `go.work`:

- `.` — core (`github.com/KARTIKrocks/gopay`), depends only on `google/uuid`
- `stripe/`, `paypal/`, `razorpay/` — each its own module importing the core + that provider's SDK

`go.work` makes local edits to the core module resolve immediately inside the sub-modules during development. The sub-module `go.mod` files `require` a published core version (e.g. `gopay v0.2.0`) rather than using `replace` directives — those are toggled only around releases (see `make release-prep` / `make release-local`). Because of this layout, `go` commands must be run **per module** (the Makefile loops over all four); a bare `go test ./...` from the root only covers the core package.

## Commands

Use the Makefile; it iterates over all four modules. Requires Go 1.24+ and golangci-lint v2 (installed automatically by `make setup`).

```bash
make all          # tidy, fmt, vet, lint, build, test — run before requesting review
make test         # go test ./... in each module
make test-race    # tests with -race -count=1
make lint         # golangci-lint across all modules
make fix          # auto-fix formatting + lint issues
make fmt-check vet # what CI runs (plus test-race) via `make ci`
make coverage     # merged coverage report across modules
make bench        # benchmarks
```

Run a single test (cd into the owning module first):

```bash
cd stripe && go test -run TestCreatePayment ./...
go test -run TestAmountValidate ./...   # core package, from repo root
```

## Architecture

**Interface segregation.** `Provider` (payment.go) is the base interface every provider implements: create/get/capture/cancel payment, refund, get refund. Optional interfaces extend it: `CustomerProvider`, `PaymentMethodProvider`, `WebhookProvider`, `ListProvider` (list.go), `SetupIntentProvider` (setupintent.go, save-card-without-charging — Stripe only), `SubscriptionProvider` (subscription.go, plans + recurring billing — Stripe and Razorpay), and `InvoiceProvider` (invoice.go, read-only invoice retrieval — Stripe and Razorpay). Providers only implement what they support — e.g. PayPal omits customer/payment-method management, only Stripe implements setup intents, and subscriptions/invoices are implemented by Stripe and Razorpay (see the support matrix in README.md).

**Client capability dispatch.** `Client` (payment.go) wraps any `Provider`, adds request validation, and gates optional features with runtime type assertions: methods like `CreateCustomer` do `provider.(CustomerProvider)` and return `ErrUnsupported` if the provider doesn't implement it. When adding a `Client` method for an optional capability, follow this validate → type-assert → `ErrUnsupported` → call → wrap-error pattern.

**Error mapping.** All provider-specific SDK errors are translated to the package's sentinel errors (e.g. `ErrCardDeclined`, `ErrInsufficientFunds`, `ErrNotFound`) via each provider's `mapError` method, so callers use `errors.Is` against the core sentinels regardless of provider. New provider methods must funnel SDK errors through `mapError`.

**Builder pattern.** Requests (`PaymentRequest`, `RefundRequest`, `CustomerRequest`) are constructed via `New…` constructors plus chained `With…` methods, each with a `Validate()` method the `Client` calls before dispatch. Amounts use integer minor units via helpers (`USD`, `EUR`, `GBP`, `INR`), validated against an ISO-4217 allowlist (`validCurrencies`). `currency.go` holds the matching minor-unit exponent table (`currencyMinorUnits`) plus `ParseMajorUnitAmount`, which converts a major-unit decimal string (e.g. PayPal's `"10.00"`) to minor units; every `validCurrencies` entry must have an exponent, enforced by `TestCurrencyExponentCoverage`.

**Listing & pagination.** `ListProvider` (list.go) is an optional interface
(`ListPayments`/`ListRefunds`/`ListCustomers`) gated by the `Client` like the
other optional capabilities. Pagination is cursor-based via `ListParams`
(`Limit`, opaque `Cursor`) returning a generic `List[T]` (`Items`, `HasMore`,
`NextCursor`). The `Cursor` is provider-specific but opaque to callers: Stripe
maps it to `starting_after` (object ID); Razorpay encodes a skip offset. PayPal
does not implement `ListProvider` (its Orders API has no list endpoint), so those
calls return `ErrUnsupported`. New providers should use `ListParams.EffectiveLimit`
for the default page size and set `NextCursor` only when `HasMore`.

**Webhooks.** `WebhookProvider.VerifyWebhook` verifies signatures and parses events into a unified `WebhookEvent`. Beyond the raw `Type`/`Raw`, each provider normalizes the event into `Kind` (a `WebhookEventKind`), `PaymentID`, `OrderID`, `RefundID`, `SubscriptionID`, and `Amount` (minor units, may be nil) so callers can act without parsing `Raw`. When adding or extending a provider's webhook handling, map its event types to `WebhookEventKind` via a `mapWebhookKind` helper and pull the identifiers/amount out of the verified payload — refund success/failure should be derived from the refund object's status, not assumed; unmapped events stay `WebhookUnknown` with `Type`/`Raw` intact. Each provider also exports a `ParseWebhook` that parses _without_ verification — for debugging only, never production.

**Testing.** `MockProvider` (mock.go) implements all interfaces and is configurable (`WithAutoSucceed`, `WithCreateError`, `Reset`, `Payments()`) for unit tests without hitting external APIs.

## Conventions

- Every exported type and function needs a doc comment.
- Sentinel errors wrap with `%w` and the `gopay:` prefix; preserve `errors.Is` chains.
- Keep PRs focused; include tests; ensure `make all` passes.
