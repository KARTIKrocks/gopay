---
id: setup-intents
title: Setup Intents
description: Save a card for later off-session charges with the optional SetupIntentProvider capability, currently Stripe only.
---

# Setup Intents

A setup intent tokenizes and stores a payment method for future
**off-session** charges, without moving any money. It's the standard
primitive behind subscriptions and one-click checkout.

It's an optional capability implemented by **Stripe only** at present; other
providers return `payment.ErrUnsupported`.

## Creating a setup intent

Without a `PaymentMethodID`, the intent is created awaiting client-side
confirmation — hand `ClientSecret` to your frontend to collect and confirm
the card:

```go
si, err := client.CreateSetupIntent(ctx, payment.NewSetupIntentRequest().
    WithCustomer(customerID).
    WithUsage(payment.SetupIntentUsageOffSession))
if err != nil {
    log.Fatal(err)
}
// hand si.ClientSecret to the frontend
```

If you already hold a payment method (e.g. collected earlier), pass it to
confirm immediately:

```go
si, err := client.CreateSetupIntent(ctx, payment.NewSetupIntentRequest().
    WithCustomer(customerID).
    WithPaymentMethod(paymentMethodID))
```

```go
if si.IsSucceeded() {
    // si.PaymentMethodID is now stored on the customer for later charges
}
```

## `SetupIntentUsage`

- `SetupIntentUsageOffSession` (default) — charged when the customer is not
  present, e.g. recurring subscription billing.
- `SetupIntentUsageOnSession` — charged while the customer is present, e.g.
  one-click checkout.

## Retrieving and canceling

```go
si, err := client.GetSetupIntent(ctx, si.ID)
si, err = client.CancelSetupIntent(ctx, si.ID)
```

## `SetupIntent` fields

`ID`, `Status` (`SetupIntentStatusRequiresPaymentMethod`,
`RequiresConfirmation`, `RequiresAction`, `Processing`, `Succeeded`,
`Canceled`), `CustomerID`, `PaymentMethodID`, `Usage`, `Description`,
`ClientSecret`, `RedirectURL` (3DS/next-action), `FailureCode`,
`FailureMessage`, `Metadata`, `CreatedAt`, `Provider`, `Raw`.

`si.RequiresAction()` reports whether additional authentication (e.g. 3DS)
is needed before the setup can complete.

## Next steps

Once a card is saved, attach it to future payments with `WithPaymentMethod`
(see [Payments](./payments.md)), or use it to back a
[Subscription](./subscriptions.md).
