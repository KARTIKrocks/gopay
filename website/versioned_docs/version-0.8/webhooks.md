---
id: webhooks
title: Webhooks
description: Signature-verified, provider-normalized webhook events via VerifyWebhook and WebhookEventKind.
---

# Webhooks

`WebhookProvider.VerifyWebhook` verifies signatures and parses events into a
unified `WebhookEvent`. All three providers implement it — pass the raw
request body and the relevant signature header(s):

```go
event, err := client.VerifyWebhook(ctx, payload, map[string]string{
    "Stripe-Signature": signatureHeader, // for Stripe
    // "X-Razorpay-Signature": sig,      // for Razorpay
    // "PAYPAL-TRANSMISSION-SIG": sig,   // for PayPal (+ other PAYPAL-* headers)
})
if err != nil {
    // signature verification failed
}

fmt.Println(event.Type)     // e.g. "payment_intent.succeeded"
fmt.Println(event.Provider) // e.g. "stripe"
```

Configure each provider's webhook secret at construction time —
`stripe.DefaultConfig().WithWebhookSecret(...)`,
`razorpay.DefaultConfig().WithWebhookSecret(...)`,
`paypal.DefaultConfig().WithWebhookID(...)`.

## Acting on normalized events

Beyond the raw `Type`/`Raw`, every event carries provider-normalized fields
so you can act without parsing the raw payload — switch on `event.Kind`:

```go
switch event.Kind {
case payment.WebhookPaymentSucceeded:
    // event.PaymentID and event.OrderID are populated; event.Amount may be nil
    if event.Amount != nil {
        markPaid(event.PaymentID, event.Amount)
    }
case payment.WebhookPaymentFailed:
    markFailed(event.PaymentID)
case payment.WebhookRefundSucceeded:
    recordRefund(event.RefundID, event.PaymentID, event.Amount)
case payment.WebhookSetupSucceeded:
    // event.SetupIntentID is populated; the payment method is ready for reuse
    activateSavedCard(event.SetupIntentID)
case payment.WebhookSetupFailed:
    notifySetupFailed(event.SetupIntentID)
case payment.WebhookInvoicePaymentSucceeded:
    // recurring charge paid; event.SubscriptionID/InvoiceID and Amount are set
    extendSubscription(event.SubscriptionID, event.Amount)
case payment.WebhookInvoicePaymentFailed:
    flagPastDue(event.SubscriptionID)
case payment.WebhookSubscriptionCanceled:
    revokeAccess(event.SubscriptionID)
case payment.WebhookUnknown:
    // not a normalized event; fall back to event.Type / event.Raw
}
```

## `WebhookEventKind` values

`WebhookPaymentCreated`, `WebhookPaymentSucceeded`, `WebhookPaymentFailed`,
`WebhookPaymentCanceled`, `WebhookRefundSucceeded`, `WebhookRefundFailed`,
`WebhookSetupSucceeded`, `WebhookSetupFailed`,
`WebhookSubscriptionCreated`, `WebhookSubscriptionUpdated`,
`WebhookSubscriptionCanceled`, `WebhookInvoicePaymentSucceeded`,
`WebhookInvoicePaymentFailed`, and `WebhookUnknown` for anything that
doesn't map to a normalized category.

## `WebhookEvent` fields

`ID`, `Type` (raw provider event type), `Provider`, `Raw` (raw payload
bytes), `Kind`, and the normalized identifiers that apply to the event —
`PaymentID`, `OrderID`, `RefundID`, `SetupIntentID`, `SubscriptionID`,
`InvoiceID` — each empty when it doesn't apply. `Amount` is the normalized
amount in integer minor units (`nil` when the event carries no amount).
PayPal's major-unit decimal strings (e.g. `"10.00"`) are converted to minor
units automatically via `ParseMajorUnitAmount`.

## Debugging: `ParseWebhook`

PayPal and Razorpay each export a `ParseWebhook` that parses an event
**without** verifying its signature. It exists for local debugging only —
never call it in production, since it accepts an unsigned, unauthenticated
payload.

Stripe's `ParseWebhook` is different: despite the shared name, it still
verifies the signature — it takes `signature` and `webhookSecret` as explicit
parameters instead of reading them from `Config` and a headers map the way
`VerifyWebhook` does. It's a convenience for callers who already have the
raw `Stripe-Signature` header value in hand, not an unverified debugging
path.
