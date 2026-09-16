---
id: payment-methods
title: Payment Methods
description: Attach, detach, and list a customer's saved payment methods via the optional PaymentMethodProvider capability.
---

# Payment Methods

Payment method management is an optional capability: providers implement
`PaymentMethodProvider` only when their API supports it. See
[Stripe](./stripe.md), [PayPal](./paypal.md), and [Razorpay](./razorpay.md)
for which providers do.

```go
err := client.AttachPaymentMethod(ctx, customerID, paymentMethodID)
```

`AttachPaymentMethod` associates an existing payment method (created
provider-side, e.g. via Stripe.js on the frontend, or collected through a
[Setup Intent](./setup-intents.md)) with a customer.

## Detaching and listing

```go
err := client.DetachPaymentMethod(ctx, paymentMethodID)

methods, err := client.ListPaymentMethods(ctx, customerID)
for _, m := range methods {
    fmt.Println(m.ID, m.Type, m.Card.Brand, m.Card.Last4)
}
```

An empty `customerID` or `paymentMethodID` returns `payment.ErrNotFound`
without a network call.

## `PaymentMethod` fields

`ID`, `Type` (`PaymentMethodCard`, `PaymentMethodBankAccount`,
`PaymentMethodWallet`, `PaymentMethodUPI`, `PaymentMethodNetBanking`),
`CustomerID`, `Card` (`*CardDetails` when `Type` is card — brand, last 4,
expiry, funding, country), `BillingDetails`, `CreatedAt`, `Provider`, `Raw`.
