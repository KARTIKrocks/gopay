# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.1] - 2026-06-07

Curated quality and correctness pass (informed by an automated full-codebase
review). All changes are backward compatible — no exported signatures changed,
and sentinel errors remain matchable with `errors.Is`.

### Fixed

- **PayPal**: `toDecimal` now preserves the sign for negative amounts in the
  `(-1.00, 0)` range (e.g. `-50` now renders `-0.50` instead of `0.50`).
- **PayPal**: `VerifyWebhook` surfaces provider errors on HTTP 4xx/5xx responses
  instead of misinterpreting them as a verification result.
- **Stripe**: `Stripe-Signature` header lookup is now case-insensitive, matching
  HTTP header semantics.

### Changed

- **Core**: provider call results are wrapped with operation context
  (`fmt.Errorf("...: %w", err)`); sentinel errors stay unwrappable via `errors.Is`.
- **Core**: customer and payment-method `Client` methods validate empty IDs and
  return `ErrNotFound`, consistent with the existing payment/refund methods.
- **Core**: `Amount.Validate` canonicalizes the currency in place (trim +
  uppercase) so providers always receive a clean ISO 4217 code.
- **Providers**: nil-request guards and `json.Marshal` error checks across
  Stripe, PayPal, and Razorpay.
- **PayPal / Razorpay**: a default 30s HTTP timeout is applied on a *copy* of the
  caller's `*http.Client`, never mutating the client the caller still owns.
- **Providers**: unparseable error responses include a length-bounded (256-byte)
  body snippet instead of an unbounded dump.

### Tooling & Tests

- Stricter linting: `revive` exported-symbol docs enabled and `errcheck` now
  applies to tests; added the corresponding doc comments and error checks.
- `make lint-fix` runs per-module, matching the multi-module layout.
- Expanded provider test coverage (zero-amount, invalid-currency,
  transport-failure, and empty-email paths).
- Issue template reminds reporters to redact secrets from logs/screenshots.

## [0.1.0] - 2025-03-18

### Added

- Core payment interfaces: `Provider`, `CustomerProvider`, `PaymentMethodProvider`, `WebhookProvider`
- `Client` with validation, convenience methods (`FullRefund`, `VerifyWebhook`), and interface detection
- Builder pattern for `PaymentRequest`, `RefundRequest`, and `CustomerRequest`
- Currency helpers: `USD()`, `EUR()`, `GBP()`, `INR()`
- Amount validation with ISO 4217 currency code checking
- Sentinel errors for consistent cross-provider error handling (`ErrCardDeclined`, `ErrNotFound`, etc.)
- `MockProvider` for unit testing with configurable behavior

### Providers

- **Stripe** (`github.com/KARTIKrocks/gopay/stripe`)
  - Payments, refunds, customers, payment methods
  - Webhook signature verification via Stripe SDK
  - Per-instance `client.API` (safe for concurrent use with multiple keys)

- **PayPal** (`github.com/KARTIKrocks/gopay/paypal`)
  - Payments (orders), refunds, capture, void authorization
  - Webhook verification via PayPal verification endpoint
  - OAuth2 token management with thread-safe caching
  - Integer-only money math (no float precision issues)

- **Razorpay** (`github.com/KARTIKrocks/gopay/razorpay`)
  - Payments (orders), refunds, customers
  - HMAC-SHA256 webhook signature verification

### Project

- Multi-module structure for dependency isolation (each provider is a separate Go module)
- CI workflow with test matrix (Go 1.24), coverage, linting, benchmarks
- CodeQL security scanning
- Dependabot configuration for all modules
- golangci-lint v2 configuration

[0.1.0]: https://github.com/KARTIKrocks/gopay/releases/tag/v0.1.0
