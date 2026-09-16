---
id: refunds
title: Refunds
description: Full and partial refund processing with RefundRequest.
---

# Refunds

`Refund` and `GetRefund` are part of the base `Provider` interface, so
refunds work with every provider.

## Full refund

```go
refund, err := client.FullRefund(ctx, paymentID)
```

`FullRefund` builds a `RefundRequest` for you with no `Amount` set, which
every provider treats as "refund the full remaining amount".

## Partial refund

```go
refund, err := client.Refund(ctx, payment.NewRefundRequest(paymentID).
    WithAmount(payment.USD(500)).
    WithReason(payment.RefundReasonRequestedByCustomer))
```

`RefundRequest.Validate()` requires a non-empty `PaymentID` and, when
`Amount` is set, validates it the same way `PaymentRequest` does.

## Refund reasons

```go
payment.RefundReasonDuplicate
payment.RefundReasonFraudulent
payment.RefundReasonRequestedByCustomer
payment.RefundReasonOther
```

`Reason` is optional and purely informational — providers that support it
attach it to their own refund record; it does not change refund behavior.

## Reading refund state

```go
refund, err := client.GetRefund(ctx, refundID)
refund.IsSuccessful() // Status == RefundStatusSucceeded
```

`Refund.Status` is one of `RefundStatusPending`, `RefundStatusSucceeded`,
`RefundStatusFailed`, or `RefundStatusCanceled`.

## Errors

A refund attempt on an already-fully-refunded payment maps to
`payment.ErrAlreadyRefunded`. See [Errors](./errors.md) for the full sentinel
list.
