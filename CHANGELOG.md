# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
