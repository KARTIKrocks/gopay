# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.5.1] - 2026-06-16

Quality and correctness pass triaging an automated full-repository review. All
changes are backward compatible — no exported signatures changed.

### Fixed

- **Stripe**: a caller-supplied `*http.Client` with no timeout now receives a
  default 30s timeout, applied on a copy so the caller's client is never mutated,
  preventing a custom client from blocking requests indefinitely. This brings
  Stripe in line with PayPal and Razorpay, which already did this.

### Changed

- **Razorpay**: `Config.WithHTTPClient` now copies the caller's `*http.Client`
  and applies a default 30s timeout at the builder level, mirroring PayPal's
  defensive pattern (previously the default was only applied later in
  `NewProvider`).

### Docs

- **Core**: corrected the `currencyMinorUnits` doc comment, which referenced a
  3-decimal exponent (e.g. KWD) the table never contained — it holds only
  exponents 0 (e.g. JPY) and 2.

## [0.5.0] - 2026-06-15

Subscriptions and recurring billing. Providers that support it can now create
plans and subscribe customers to them through a unified `Client` API, building on
the setup-intent primitive from v0.4.0. All changes are backward compatible: only
additive symbols, a new optional interface, and new `WebhookEvent` fields;
existing code is unaffected.

### Added

- **Core**: `SubscriptionProvider` — a new optional provider interface
  (`CreatePlan`, `GetPlan`, `CreateSubscription`, `GetSubscription`,
  `CancelSubscription`) gated by the `Client` with a runtime type assertion,
  returning `ErrUnsupported` for providers that don't implement it.
- **Core**: matching `Client` methods following the existing validate →
  type-assert → `ErrUnsupported` → call → wrap-error pattern.
- **Core**: `PlanRequest` builder (`WithName`/`WithIntervalCount`/`WithMetadata`/
  `WithIdempotencyKey` + `Validate`) and `Plan` result type;
  `SubscriptionRequest` builder (`WithPaymentMethod`/`WithTrialDays`/
  `WithMetadata`/`WithIdempotencyKey` + `Validate`) and `Subscription` result
  type (`IsActive`, current-period fields, `CancelAtPeriodEnd`).
- **Core**: `BillingInterval` (day/week/month/year) and `SubscriptionStatus`
  enums, `CancelOptions` (immediate vs at-period-end), and the
  `ErrSubscriptionFailed` sentinel error.
- **Core**: webhook event kinds `WebhookSubscriptionCreated`/`Updated`/`Canceled`
  and `WebhookInvoicePaymentSucceeded`/`Failed`, plus `WebhookEvent.SubscriptionID`
  and `WebhookEvent.InvoiceID` fields.
- **Stripe**: implements `SubscriptionProvider` (plans map to recurring Prices
  with an inline Product; subscriptions support trials, a default payment method,
  and immediate or at-period-end cancellation) and normalizes
  `customer.subscription.*` and `invoice.payment_*` webhook events.
- **MockProvider**: implements `SubscriptionProvider` (honoring `WithAutoSucceed`)
  with `WithSubscriptionError`, `SetPlan`, and `SetSubscription` helpers.

### Notes

- **Razorpay** and **PayPal**: do not yet implement `SubscriptionProvider`, so
  subscription calls return `ErrUnsupported`. Plan changes/proration
  (`UpdateSubscription`), `ListSubscriptions`, and full invoice CRUD are planned
  follow-ups.

### Tooling & Tests

- Core, mock, and Stripe subscription tests covering plan/subscription
  validation, `Client` dispatch, trial/cancel semantics, status/interval mapping,
  webhook normalization, and the `ErrUnsupported` path.

## [0.4.0] - 2026-06-15

Setup intents (save-card-without-charging). Providers that support it can now
tokenize and store a payment method for future off-session charges — the standard
primitive behind subscriptions and one-click checkout — through a unified
`Client` API. All changes are backward compatible: only additive symbols, a new
optional interface, and a new `WebhookEvent` field; existing code is unaffected.

### Added

- **Core**: `SetupIntentProvider` — a new optional provider interface
  (`CreateSetupIntent`, `GetSetupIntent`, `CancelSetupIntent`) gated by the
  `Client` with a runtime type assertion, returning `ErrUnsupported` for
  providers that don't implement it.
- **Core**: `Client.CreateSetupIntent`, `Client.GetSetupIntent`, and
  `Client.CancelSetupIntent`, following the existing validate → type-assert →
  `ErrUnsupported` → call → wrap-error pattern.
- **Core**: `SetupIntentRequest` (builder with `WithCustomer`,
  `WithPaymentMethod`, `WithUsage`, `WithDescription`, `WithReturnURL`,
  `WithMetadata`, `WithIdempotencyKey`, plus `Validate`) and the `SetupIntent`
  result type (`IsSucceeded`, `RequiresAction`). Confirmation is folded into
  creation: setting `PaymentMethodID` confirms immediately, mirroring
  `CreatePayment`.
- **Core**: `SetupIntentStatus` and `SetupIntentUsage` (`off_session` default /
  `on_session`) enums, and the `ErrSetupFailed` sentinel error.
- **Core**: `WebhookSetupSucceeded` / `WebhookSetupFailed` webhook event kinds and
  a new `WebhookEvent.SetupIntentID` field.
- **Stripe**: implements `SetupIntentProvider` using the native SetupIntents API,
  and normalizes `setup_intent.succeeded` / `setup_intent.setup_failed` /
  `setup_intent.canceled` webhook events.
- **MockProvider**: implements `SetupIntentProvider` (honoring `WithAutoSucceed`)
  with `WithSetupError` and `SetSetupIntent` helpers for tests.

### Notes

- **Razorpay** and **PayPal**: do not yet implement `SetupIntentProvider`, so
  setup-intent calls return `ErrUnsupported`. Razorpay tokens / PayPal Vault
  setup-tokens are planned as follow-ups.

### Tooling & Tests

- Core, mock, and Stripe setup-intent tests covering request validation, `Client`
  dispatch, status/usage mapping, webhook normalization, and the `ErrUnsupported`
  path.

## [0.3.0] - 2026-06-14

Listing and pagination. Providers that support it can now list payments, refunds,
and customers with cursor-based pagination through a unified `Client` API. All
changes are backward compatible — only additive symbols and a new optional
interface; existing code is unaffected.

### Added

- **Core**: `ListProvider` — a new optional provider interface
  (`ListPayments`, `ListRefunds`, `ListCustomers`) gated by the `Client` with a
  runtime type assertion, returning `ErrUnsupported` for providers that don't
  implement it.
- **Core**: `Client.ListPayments`, `Client.ListRefunds`, and
  `Client.ListCustomers`, following the existing validate → type-assert →
  `ErrUnsupported` → call → wrap-error pattern.
- **Core**: `ListParams` (builder with `WithLimit`/`WithCursor`, plus `Validate`
  and `EffectiveLimit`) and a generic `List[T]` page type (`Items`, `HasMore`,
  `NextCursor`). Pagination is cursor-based with an opaque, provider-specific
  cursor. `DefaultListLimit` (20) and `MaxListLimit` (100) bound the page size.
- **Stripe**: implements `ListProvider` using cursor pagination
  (`starting_after`), one page per call, with `has_more` taken from the list
  metadata.
- **Razorpay**: implements `ListProvider` using `skip`/`count` offsets, encoded
  into the opaque cursor.
- **MockProvider**: implements `ListProvider` with deterministic newest-first
  ordering for tests.

### Notes

- **PayPal**: does not implement `ListProvider` (its Orders API has no list
  endpoint), so listing calls return `ErrUnsupported`.

### Tooling & Tests

- Core, Stripe, and Razorpay listing tests covering pagination walks, cursor
  encoding, `HasMore`/empty/last-page edges, and the `ErrUnsupported` path.

## [0.2.0] - 2026-06-11

Provider-normalized webhook events. `WebhookEvent` now carries typed, unified
fields so consumers can act on an event without hand-parsing each provider's raw
JSON. All changes are backward compatible — only additive fields and new
exported symbols; existing `Raw`-based code keeps working.

### Added

- **Core**: `WebhookEvent` gains normalized fields — `Kind` (a new
  `WebhookEventKind`), `PaymentID`, `OrderID`, `RefundID`, and `Amount`
  (`*Amount`, in minor units). Fields are zero-valued when not applicable to the
  event; unmapped events use `Kind == WebhookUnknown` with `Type`/`Raw` preserved.
- **Core**: `WebhookEventKind` enum — `WebhookPaymentCreated`,
  `WebhookPaymentSucceeded`, `WebhookPaymentFailed`, `WebhookPaymentCanceled`,
  `WebhookRefundSucceeded`, `WebhookRefundFailed`, and `WebhookUnknown`.
- **Core**: `ParseMajorUnitAmount` converts a major-unit decimal string (e.g.
  PayPal's `"10.00"`) to a minor-unit `Amount`, backed by an ISO 4217
  minor-unit exponent table kept in sync with the accepted-currency allowlist.
  Rounds half-up; returns `(nil, false)` for unknown currencies or malformed input.

### Changed

- **Stripe / PayPal / Razorpay**: `VerifyWebhook` and `ParseWebhook` now populate
  the normalized fields. Provider event types are mapped to `WebhookEventKind`,
  and identifiers/amounts are extracted from the verified payload (PayPal's
  major-unit amounts are converted to minor units; `Amount` is left nil for
  currencies outside the accepted set).
- **MockProvider**: `VerifyWebhook` reads optional `kind`, `payment_id`,
  `order_id`, `refund_id`, `amount`, and `currency` fields so the test harness can
  emit fully-normalized events.

### Tooling & Tests

- Currency-coverage guard test ensures every accepted currency has an explicit
  minor-unit exponent.
- Added per-provider webhook normalization tests and `ParseMajorUnitAmount`
  rounding/edge-case coverage.

## [0.1.1] - 2026-06-07

Curated quality and correctness pass (informed by an automated full-codebase
review). All changes are backward compatible — no exported signatures changed,
and sentinel errors remain matchable with `errors.Is`.

### Fixed

- **PayPal**: `toDecimal` now preserves the sign for negative amounts in the
  `(-1.00, 0)` range (e.g. `-50` now renders `-0.50` instead of `0.50`).
- **PayPal**: `VerifyWebhook` surfaces provider errors on HTTP 4xx/5xx responses
  instead of misinterpreting them as a verification result.
- **PayPal**: malformed decimal amounts in provider responses now surface as an
  error instead of silently mapping to a zero amount.
- **Stripe**: `Stripe-Signature` header lookup is now case-insensitive, matching
  HTTP header semantics.

### Changed

- **Core**: provider call results are wrapped with operation context
  (`fmt.Errorf("...: %w", err)`); sentinel errors stay unwrappable via `errors.Is`.
- **Core**: customer and payment-method `Client` methods validate empty IDs and
  return `ErrNotFound`, consistent with the existing payment/refund methods.
- **Core**: `Amount.Validate` canonicalizes the currency in place (trim +
  uppercase) so providers always receive a clean ISO 4217 code.
- **Providers**: nil-request and nil-amount guards plus `json.Marshal` error
  checks across Stripe, PayPal, and Razorpay.
- **PayPal / Razorpay**: a default 30s HTTP timeout is applied on a _copy_ of the
  caller's `*http.Client`, never mutating the client the caller still owns.
- **Providers**: unparseable error responses return a generic message and never
  echo response-body bytes, avoiding any risk of leaking secrets.

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

[0.5.1]: https://github.com/KARTIKrocks/gopay/releases/tag/v0.5.1
[0.5.0]: https://github.com/KARTIKrocks/gopay/releases/tag/v0.5.0
[0.4.0]: https://github.com/KARTIKrocks/gopay/releases/tag/v0.4.0
[0.3.0]: https://github.com/KARTIKrocks/gopay/releases/tag/v0.3.0
[0.2.0]: https://github.com/KARTIKrocks/gopay/releases/tag/v0.2.0
[0.1.1]: https://github.com/KARTIKrocks/gopay/releases/tag/v0.1.1
[0.1.0]: https://github.com/KARTIKrocks/gopay/releases/tag/v0.1.0
