---
id: getting-started
title: Getting Started
description: Install gopay and create your first payment with Stripe, PayPal, or Razorpay.
---

# Getting Started

## Installation

```bash
# Core library (interfaces, types, mock provider)
go get github.com/KARTIKrocks/gopay

# Install only the providers you need
go get github.com/KARTIKrocks/gopay/stripe
go get github.com/KARTIKrocks/gopay/paypal
go get github.com/KARTIKrocks/gopay/razorpay
```

Each provider is its own Go module with its own `go.mod`, so `go get`ing
`stripe` never pulls in PayPal's or Razorpay's SDK.

## Quick Start

Every provider follows the same shape: build a provider `Config`, construct
the provider, wrap it in a `Client`, then call `Client` methods. Here it is
with Stripe:

```go
package main

import (
    "context"
    "log"

    payment "github.com/KARTIKrocks/gopay"
    "github.com/KARTIKrocks/gopay/stripe"
)

func main() {
    ctx := context.Background()

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

    p, err := client.CreatePayment(ctx, payment.NewPaymentRequest(payment.USD(1999)).
        WithDescription("Order #123").
        WithPaymentMethod("pm_card_visa"))
    if err != nil {
        log.Fatal(err)
    }

    log.Printf("payment %s: %s", p.ID, p.Status)
}
```

Swapping providers is a matter of swapping the `stripe.*` calls for
`paypal.*` or `razorpay.*` — see [Stripe](./stripe.md), [PayPal](./paypal.md),
and [Razorpay](./razorpay.md) for each one's `Config` and quirks.

## Currency helpers

Amounts are always integer **minor units** (cents, pence, paise — never
floats):

```go
payment.USD(1999)  // $19.99 in cents
payment.EUR(500)   // €5.00 in cents
payment.GBP(250)   // £2.50 in pence
payment.INR(10000) // ₹100.00 in paise
```

For any other ISO 4217 currency, use `payment.NewAmount(value, "JPY")`
directly.

## Next steps

- **[Client](./client.md)** — how optional capabilities (customers, webhooks,
  subscriptions...) are gated and dispatched
- **[Payments](./payments.md)** — the full payment lifecycle
- **[Mock Provider](./mock-provider.md)** — writing tests without hitting a
  real provider
