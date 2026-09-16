---
id: subscriptions
title: Subscriptions
description: Plans and recurring billing with the optional SubscriptionProvider capability, including Razorpay's mandate-authorization quirks.
---

# Subscriptions

`SubscriptionProvider` adds recurring billing: plans (a recurring price) and
subscriptions that charge a customer's stored payment method each billing
cycle. It's an optional capability implemented by **Stripe and Razorpay**;
PayPal returns `payment.ErrUnsupported`.

Subscriptions charge a saved off-session payment method, so on Stripe they
build on the [setup-intent flow](./setup-intents.md).

## Creating a plan

```go
plan, err := client.CreatePlan(ctx, payment.NewPlanRequest(payment.USD(1999), payment.BillingIntervalMonth).
    WithName("Pro").
    WithIntervalCount(1))
```

`Interval` is `BillingIntervalDay`, `Week`, `Month`, or `Year`.
`IntervalCount` is the number of intervals between charges — `Month` + count
`3` is quarterly — and defaults to `1`. In Stripe, a plan becomes a recurring
Price; `plan.ID` is what you subscribe customers to.

## Subscribing a customer

```go
sub, err := client.CreateSubscription(ctx, payment.NewSubscriptionRequest(customerID, plan.ID).
    WithPaymentMethod(paymentMethodID).
    WithTrialDays(14))
```

```go
if sub.IsActive() {
    // sub.CurrentPeriodEnd is the next charge date
}
```

`Subscription.Status` is one of `SubscriptionStatusActive`, `Trialing`,
`PastDue`, `Canceled`, `Incomplete`, `IncompleteExpired`, `Unpaid`, or
`Completed` (a finite subscription that ran all its billing cycles — see
below).

## Canceling

```go
// Keep access until the current period ends
sub, err = client.CancelSubscription(ctx, sub.ID, &payment.CancelOptions{AtPeriodEnd: true})

// Cancel immediately
sub, err = client.CancelSubscription(ctx, sub.ID, nil)
```

A `nil` `*CancelOptions` cancels immediately.

## Razorpay's mandate-authorization flow

Razorpay's subscription model differs from Stripe's in ways your code needs
to branch on if you support both:

- **A finite cycle count is required.** Razorpay has no concept of
  open-ended billing — set `SubscriptionRequest.TotalCount` via
  `WithTotalCount`. Stripe ignores `TotalCount` entirely.
- **The customer authorizes the mandate via a redirect.** Razorpay's
  `CreateSubscription` does **not** accept `CustomerID` or
  `PaymentMethodID` on the request — instead, the returned
  `Subscription.AuthURL` is a customer-facing URL the customer must visit to
  authorize the recurring mandate. Until they do, `Status` is
  `SubscriptionStatusIncomplete`. Once authorization completes, Razorpay
  populates the subscription's `CustomerID` for you. `AuthURL` is empty for
  Stripe, which charges a pre-authorized payment method directly.

```go
sub, err := client.CreateSubscription(ctx, payment.NewSubscriptionRequest("", plan.ID).
    WithTotalCount(12)) // Razorpay: required; Stripe: ignored
if err != nil {
    log.Fatal(err)
}
if sub.AuthURL != "" {
    // redirect the customer to sub.AuthURL to authorize the mandate
}
```

## Reacting to billing outcomes

Recurring charges surface as normalized webhook events rather than as
polling — see [Webhooks](./webhooks.md):

- `WebhookInvoicePaymentSucceeded` — a recurring charge was paid (`SubscriptionID`,
  `InvoiceID`, `Amount` populated)
- `WebhookInvoicePaymentFailed` — a recurring charge failed
- `WebhookSubscriptionCanceled` — the subscription ended

## Next steps

Each successful billing cycle produces an [Invoice](./invoices.md) you can
retrieve with `client.GetInvoice`.
