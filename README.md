# gopay

[![Go Reference](https://pkg.go.dev/badge/github.com/KARTIKrocks/gopay.svg)](https://pkg.go.dev/github.com/KARTIKrocks/gopay)
[![Go Report Card](https://goreportcard.com/badge/github.com/KARTIKrocks/gopay)](https://goreportcard.com/report/github.com/KARTIKrocks/gopay)
[![Go Version](https://img.shields.io/github/go-mod/go-version/KARTIKrocks/gopay)](go.mod)
[![CI](https://github.com/KARTIKrocks/gopay/actions/workflows/ci.yml/badge.svg)](https://github.com/KARTIKrocks/gopay/actions/workflows/ci.yml)
[![GitHub tag](https://img.shields.io/github/v/tag/KARTIKrocks/gopay)](https://github.com/KARTIKrocks/gopay/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![codecov](https://codecov.io/gh/KARTIKrocks/gopay/branch/main/graph/badge.svg)](https://codecov.io/gh/KARTIKrocks/gopay)

A unified payment processing library for Go with support for Stripe, PayPal, and Razorpay.

Each provider is a separate Go module, so you only pull in the dependencies you need.

## Installation

```bash
# Core library (interfaces, types, mock provider)
go get github.com/KARTIKrocks/gopay

# Install only the providers you need:
go get github.com/KARTIKrocks/gopay/stripe
go get github.com/KARTIKrocks/gopay/paypal
go get github.com/KARTIKrocks/gopay/razorpay
```

## Features

- Unified interface for multiple payment providers
- Support for Stripe, PayPal, and Razorpay
- Dependency isolation: each provider is a separate module
- Payment creation with automatic/manual capture
- Refund processing (full and partial)
- Customer management
- Payment method management
- Setup intents (save a card for later off-session charges)
- Subscriptions and recurring billing (plans + subscriptions)
- Webhook verification with signature validation
- Mock provider for testing
- Builder pattern for requests
- Thread-safe (safe for concurrent use)

## Supported Providers

| Provider | Payments | Refunds | Customers | Payment Methods | Setup Intents | Subscriptions | Webhooks | Listing |
| -------- | -------- | ------- | --------- | --------------- | ------------- | ------------- | -------- | ------- |
| Stripe   | Yes      | Yes     | Yes       | Yes             | Yes           | Yes           | Yes      | Yes     |
| PayPal   | Yes      | Yes     | No        | No              | No            | No            | Yes      | No      |
| Razorpay | Yes      | Yes     | Yes       | No              | No            | No            | Yes      | Yes     |

PayPal's Orders API has no list endpoint, so listing calls return `ErrUnsupported`
for the PayPal provider. Setup intents (save-card-without-charging) and
subscriptions (recurring billing) are currently implemented for Stripe only;
Razorpay and PayPal return `ErrUnsupported`.

## Quick Start

### Stripe

```go
import (
    payment "github.com/KARTIKrocks/gopay"
    "github.com/KARTIKrocks/gopay/stripe"
)

config := stripe.DefaultConfig().
    WithSecretKey("sk_test_...")

provider, err := stripe.NewProvider(config)
if err != nil {
    log.Fatal(err)
}

client, err := payment.NewClient(provider)
if err != nil {
    log.Fatal(err)
}

// Create a payment
p, err := client.CreatePayment(ctx, payment.NewPaymentRequest(payment.USD(1999)).
    WithDescription("Order #123").
    WithPaymentMethod("pm_card_visa"))
```

### PayPal

```go
import (
    payment "github.com/KARTIKrocks/gopay"
    "github.com/KARTIKrocks/gopay/paypal"
)

config := paypal.DefaultConfig().
    WithCredentials("client_id", "client_secret").
    WithSandbox(true)

provider, err := paypal.NewProvider(config)
if err != nil {
    log.Fatal(err)
}

client, err := payment.NewClient(provider)
if err != nil {
    log.Fatal(err)
}

p, err := client.CreatePayment(ctx, payment.NewPaymentRequest(payment.USD(2500)).
    WithDescription("Order #456"))
```

### Razorpay

```go
import (
    payment "github.com/KARTIKrocks/gopay"
    "github.com/KARTIKrocks/gopay/razorpay"
)

config := razorpay.DefaultConfig().
    WithCredentials("key_id", "key_secret")

provider, err := razorpay.NewProvider(config)
if err != nil {
    log.Fatal(err)
}

client, err := payment.NewClient(provider)
if err != nil {
    log.Fatal(err)
}

p, err := client.CreatePayment(ctx, payment.NewPaymentRequest(payment.INR(50000)).
    WithDescription("Order #789"))
```

## Currency Helpers

```go
payment.USD(1999)  // $19.99 in cents
payment.EUR(500)   // €5.00 in cents
payment.GBP(250)   // £2.50 in pence
payment.INR(10000) // ₹100.00 in paise
```

## Refunds

```go
// Full refund
refund, err := client.FullRefund(ctx, paymentID)

// Partial refund
refund, err := client.Refund(ctx, payment.NewRefundRequest(paymentID).
    WithAmount(payment.USD(500)).
    WithReason(payment.RefundReasonRequestedByCustomer))
```

## Setup Intents (save a card without charging)

A setup intent tokenizes and stores a payment method for future off-session
charges — the standard primitive behind subscriptions and one-click checkout. It
is an optional capability (Stripe only at present; other providers return
`ErrUnsupported`).

```go
// Create a setup intent. Without a PaymentMethodID, hand si.ClientSecret to the
// frontend to collect and confirm the card.
si, err := client.CreateSetupIntent(ctx, payment.NewSetupIntentRequest().
    WithCustomer(customerID).
    WithUsage(payment.SetupIntentUsageOffSession))
if err != nil {
    log.Fatal(err)
}

// If you already hold a payment method, pass it to confirm immediately.
si, err = client.CreateSetupIntent(ctx, payment.NewSetupIntentRequest().
    WithCustomer(customerID).
    WithPaymentMethod(paymentMethodID))
if err != nil {
    log.Fatal(err)
}

if si.IsSucceeded() {
    // si.PaymentMethodID is now stored on the customer for later charges.
}

// Retrieve or cancel later.
si, err = client.GetSetupIntent(ctx, si.ID)
if err != nil {
    log.Fatal(err)
}
si, err = client.CancelSetupIntent(ctx, si.ID)
if err != nil {
    log.Fatal(err)
}
```

## Subscriptions (recurring billing)

Create a recurring plan, then subscribe a customer to it. The subscription
charges the customer's payment method each billing cycle. This is an optional
capability (Stripe only at present; other providers return `ErrUnsupported`).

```go
// Create a recurring plan (amount + interval). In Stripe this becomes a
// recurring Price; plan.ID is what you subscribe customers to.
plan, err := client.CreatePlan(ctx, payment.NewPlanRequest(payment.USD(1999), payment.BillingIntervalMonth).
    WithName("Pro").
    WithIntervalCount(1))
if err != nil {
    log.Fatal(err)
}

// Subscribe a customer, charging a saved payment method each cycle.
sub, err := client.CreateSubscription(ctx, payment.NewSubscriptionRequest(customerID, plan.ID).
    WithPaymentMethod(paymentMethodID).
    WithTrialDays(14))
if err != nil {
    log.Fatal(err)
}
if sub.IsActive() {
    // sub.CurrentPeriodEnd is the next charge date.
}

// Cancel at the end of the current period (keeps access until then)...
sub, err = client.CancelSubscription(ctx, sub.ID, &payment.CancelOptions{AtPeriodEnd: true})
// ...or immediately (nil opts).
sub, err = client.CancelSubscription(ctx, sub.ID, nil)
```

React to billing outcomes via normalized webhook events (`WebhookInvoicePaymentSucceeded`,
`WebhookInvoicePaymentFailed`, `WebhookSubscriptionCanceled`, …) — see below.

## Webhooks

All providers support webhook signature verification through a unified interface:

```go
event, err := client.VerifyWebhook(ctx, payload, map[string]string{
    "Stripe-Signature": signatureHeader, // for Stripe
    // "X-Razorpay-Signature": sig,      // for Razorpay
    // "PAYPAL-TRANSMISSION-SIG": sig,    // for PayPal (+ other PAYPAL-* headers)
})
if err != nil {
    // signature verification failed
}

fmt.Println(event.Type)     // e.g., "payment_intent.succeeded"
fmt.Println(event.Provider) // e.g., "stripe"
```

The event is also **provider-normalized**, so you can act on it without parsing the
raw provider payload. Switch on `event.Kind` and read the normalized identifiers and
amount directly:

```go
switch event.Kind {
case payment.WebhookPaymentSucceeded:
    // event.PaymentID and event.OrderID are populated; event.Amount may be nil.
    if event.Amount != nil {
        markPaid(event.PaymentID, event.Amount)
    }
case payment.WebhookPaymentFailed:
    markFailed(event.PaymentID)
case payment.WebhookRefundSucceeded:
    recordRefund(event.RefundID, event.PaymentID, event.Amount)
case payment.WebhookSetupSucceeded:
    // event.SetupIntentID is populated; the payment method is ready for reuse.
    activateSavedCard(event.SetupIntentID)
case payment.WebhookSetupFailed:
    notifySetupFailed(event.SetupIntentID)
case payment.WebhookInvoicePaymentSucceeded:
    // Recurring charge paid; event.SubscriptionID/InvoiceID and Amount are set.
    extendSubscription(event.SubscriptionID, event.Amount)
case payment.WebhookInvoicePaymentFailed:
    flagPastDue(event.SubscriptionID)
case payment.WebhookSubscriptionCanceled:
    revokeAccess(event.SubscriptionID)
case payment.WebhookUnknown:
    // not a normalized event; fall back to event.Type / event.Raw
}
```

`event.Amount` is in integer minor units (like everywhere else in the library) and is
`nil` when the event carries no amount. Amounts are normalized across providers — PayPal's
major-unit decimal strings (e.g. `"10.00"`) are converted to minor units automatically.
Unmapped events keep `Kind == WebhookUnknown` with `Type` and `Raw` intact.

## Listing & Pagination

Providers that support it (Stripe, Razorpay) can list payments, refunds, and
customers with cursor-based pagination. Pass an opaque `NextCursor` from one page
back via `WithCursor` to fetch the next:

```go
params := payment.NewListParams().WithLimit(50)
for {
    page, err := client.ListPayments(ctx, params)
    if err != nil {
        // errors.Is(err, payment.ErrUnsupported) if the provider can't list
        break
    }
    for _, p := range page.Items {
        process(p)
    }
    if !page.HasMore {
        break
    }
    params = params.WithCursor(page.NextCursor)
}
```

`ListRefunds` and `ListCustomers` follow the same shape. `Limit` defaults to
`DefaultListLimit` (20) and may not exceed `MaxListLimit` (100). The cursor is
provider-specific and must be treated as opaque (Stripe uses an object ID;
Razorpay uses a skip offset) — always pass back the `NextCursor` you received.

## Error Handling

All provider errors are mapped to sentinel errors for consistent handling:

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

## Mock Provider for Testing

```go
mock := payment.NewMockProvider()
client, _ := payment.NewClient(mock)

// Configure behavior
mock.WithAutoSucceed(true)
mock.WithCreateError(payment.ErrCardDeclined) // simulate failures

// Use client as normal in tests
p, err := client.CreatePayment(ctx, payment.NewPaymentRequest(payment.USD(1000)).
    WithPaymentMethod("pm_test"))

// Inspect state
payments := mock.Payments()
mock.Reset()
```

## License

[MIT](LICENSE)
