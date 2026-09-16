---
id: payments
title: Payments
description: Create, capture, get, and cancel payments with PaymentRequest and the builder pattern.
---

# Payments

`CreatePayment`, `GetPayment`, `CapturePayment`, and `CancelPayment` are part
of the base `Provider` interface, so they work with every provider.

## Creating a payment

Requests are built with a `New...` constructor plus chained `With...`
methods, and validated by the `Client` before the provider ever sees them:

```go
p, err := client.CreatePayment(ctx, payment.NewPaymentRequest(payment.USD(1999)).
    WithDescription("Order #123").
    WithCustomer(customerID).
    WithPaymentMethod("pm_card_visa").
    WithMetadata("order_id", "123"))
```

`PaymentRequest.Validate()` checks the amount (non-negative value, known ISO
4217 currency) and the capture method; `Client.CreatePayment` calls it for
you and returns the validation error unwrapped, before making any network
call.

## Automatic vs. manual capture

```go
payment.NewPaymentRequest(amount).WithCaptureMethod(payment.CaptureManual)
```

- `CaptureAutomatic` (default) — funds are captured immediately.
- `CaptureManual` — the payment is authorized only; call `CapturePayment`
  separately (optionally for a smaller amount than authorized) once you're
  ready to take the funds:

```go
p, err := client.CapturePayment(ctx, paymentID, payment.USD(1500)) // partial capture
// or pass nil to capture the full authorized amount
p, err := client.CapturePayment(ctx, paymentID, nil)
```

## Canceling an authorized payment

```go
p, err := client.CancelPayment(ctx, paymentID)
```

Only payments that haven't been captured yet can be canceled; the provider
returns a mapped error otherwise (see [Errors](./errors.md)).

## Reading payment state

```go
p, err := client.GetPayment(ctx, paymentID)

p.IsSuccessful()   // Status == PaymentStatusSucceeded
p.RequiresAction() // Status == PaymentStatusRequiresAction (e.g. 3DS)
p.IsCaptured()     // AmountCaptured > 0
```

`Payment.Status` is one of `PaymentStatusPending`,
`PaymentStatusRequiresAction`, `PaymentStatusProcessing`,
`PaymentStatusSucceeded`, `PaymentStatusFailed`, `PaymentStatusCanceled`, or
`PaymentStatusRequiresCapture`.

`Payment.Raw` carries the raw provider response as `map[string]any` for
anything not surfaced on the normalized struct.

## Refunds

See [Refunds](./refunds.md) for `client.Refund` and `client.FullRefund`.
