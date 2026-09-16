---
id: razorpay
title: Razorpay
description: Configuring the Razorpay provider, including its mandate-authorization subscription flow.
---

# Razorpay

`github.com/KARTIKrocks/gopay/razorpay` covers payments, refunds, customers,
listing, webhooks, subscriptions, and invoices — everything except payment
methods and setup intents.

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
```

`NewProvider` returns `payment.ErrInvalidConfig` if `KeyID` or `KeySecret` is
empty.

## `Config`

| Field | Set via | Notes |
| --- | --- | --- |
| `KeyID`, `KeySecret` | `WithCredentials` | Required |
| `WebhookSecret` | `WithWebhookSecret` | Required to call `VerifyWebhook` |
| `HTTPClient` | `WithHTTPClient` | Optional; `DefaultConfig()` sets a 30s timeout |

## Support matrix

| Capability | Supported |
| --- | --- |
| Payments, Refunds | ✓ |
| Customers | ✓ |
| Payment Methods | – |
| Setup Intents | – (Stripe only) |
| Subscriptions | ✓ — mandate-authorization flow, see below |
| Invoices | ✓ |
| Webhooks | ✓ (`X-Razorpay-Signature` header) |
| Listing | ✓ (cursor = skip offset) |

## Subscriptions authorize via a customer-facing redirect

Razorpay's subscription model is the one place its behavior diverges
sharply from Stripe's — see [Subscriptions](./subscriptions.md#razorpays-mandate-authorization-flow)
for the full explanation. In short:

- `SubscriptionRequest.TotalCount` (a finite billing-cycle count) is
  **required** — set it via `WithTotalCount`.
- `CreateSubscription` ignores the request's `CustomerID` and
  `PaymentMethodID`; instead, the customer authorizes the recurring mandate
  by visiting the returned `Subscription.AuthURL`. The subscription's status
  is `SubscriptionStatusIncomplete` until they do, at which point Razorpay
  populates `CustomerID` for you.

## Notes

- `Metadata` on Razorpay's `notes` bag decodes safely whether Razorpay sends
  it as an empty JSON object or (as it sometimes does for empty bags) an
  empty JSON array — both normalize to a nil `Metadata` map.
