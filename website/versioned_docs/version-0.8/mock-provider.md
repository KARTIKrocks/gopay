---
id: mock-provider
title: Mock Provider
description: A full-featured in-memory Provider for tests, implementing every optional capability with no network calls.
---

# Mock Provider

`payment.MockProvider` implements every gopay interface — the base
`Provider` plus every optional capability — entirely in memory. Use it to
test code that calls `Client` without hitting a real provider or needing API
keys.

```go
mock := payment.NewMockProvider()
client, _ := payment.NewClient(mock)

p, err := client.CreatePayment(ctx, payment.NewPaymentRequest(payment.USD(1000)).
    WithPaymentMethod("pm_test"))
```

By default (`WithAutoSucceed(true)`, `WithAutoCapture(true)`), a payment
created with a `PaymentMethodID` succeeds immediately and, unless
`CaptureMethod` is `CaptureManual`, is captured immediately too.

## Simulating failures

Each operation has its own injectable error, checked before any other logic
runs:

```go
mock.WithCreateError(payment.ErrCardDeclined)
mock.WithCaptureError(payment.ErrProviderError)
mock.WithRefundError(payment.ErrRefundFailed)
mock.WithSetupError(payment.ErrSetupFailed)
mock.WithSubscriptionError(payment.ErrSubscriptionFailed)
mock.WithWebhookError(errors.New("bad signature"))
```

```go
p, err := client.CreatePayment(ctx, req)
// err wraps payment.ErrCardDeclined
```

Clear an injected error the same way — call the `With...Error` method with
`nil`.

## Controlling success/capture behavior

```go
mock.WithAutoSucceed(false) // payments stay PaymentStatusPending
mock.WithAutoCapture(false) // successful payments aren't auto-captured
```

## Staging state directly

For tests that need a resource to already exist (e.g. testing `GetPayment`
in isolation), set it directly instead of going through `Create...`:

```go
mock.SetPayment(&payment.Payment{ID: "pi_test", Status: payment.PaymentStatusSucceeded})
mock.SetRefund(&payment.Refund{ID: "re_test", PaymentID: "pi_test"})
mock.SetCustomer(&payment.Customer{ID: "cus_test", Email: "jane@example.com"})
mock.SetSetupIntent(&payment.SetupIntent{ID: "seti_test"})
mock.SetPlan(&payment.Plan{ID: "plan_test"})
mock.SetSubscription(&payment.Subscription{ID: "sub_test"})
mock.AddInvoice(&payment.Invoice{ID: "inv_test"})
```

## Inspecting and resetting state

```go
payments := mock.Payments()   // map[string]*Payment
refunds := mock.Refunds()     // map[string]*Refund
customers := mock.Customers() // map[string]*Customer

mock.Reset() // clears all staged/created state and injected errors
```

`MockProvider` is safe for concurrent use, like every other provider.
