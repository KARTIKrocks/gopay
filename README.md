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
- Webhook verification with signature validation
- Mock provider for testing
- Builder pattern for requests
- Thread-safe (safe for concurrent use)

## Supported Providers

| Provider  | Payments | Refunds | Customers | Payment Methods | Webhooks |
|-----------|----------|---------|-----------|-----------------|----------|
| Stripe    | Yes      | Yes     | Yes       | Yes             | Yes      |
| PayPal    | Yes      | Yes     | No        | No              | Yes      |
| Razorpay  | Yes      | Yes     | Yes       | No              | Yes      |

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
