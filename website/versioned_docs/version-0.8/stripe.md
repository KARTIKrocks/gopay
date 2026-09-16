---
id: stripe
title: Stripe
description: Configuring the Stripe provider — the only provider that supports every gopay capability.
---

# Stripe

`github.com/KARTIKrocks/gopay/stripe` is the most complete provider — it
implements every optional interface gopay defines.

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
```

`NewProvider` returns `payment.ErrInvalidConfig` if `SecretKey` is empty.

## `Config`

| Field | Set via | Notes |
| --- | --- | --- |
| `SecretKey` | `WithSecretKey` | Required |
| `WebhookSecret` | `WithWebhookSecret` | Required to call `VerifyWebhook` |
| `HTTPClient` | `WithHTTPClient` | Optional, for custom timeouts/transport |

## Support matrix

| Capability | Supported |
| --- | --- |
| Payments, Refunds | ✓ |
| Customers | ✓ |
| Payment Methods | ✓ |
| Setup Intents | ✓ — the only provider that does |
| Subscriptions | ✓ (open-ended — `TotalCount` is ignored) |
| Invoices | ✓ |
| Webhooks | ✓ (`Stripe-Signature` header) |
| Listing | ✓ (cursor = object ID, `starting_after`) |

## Notes

- `CaptureMethod` maps directly onto Stripe's own manual/automatic capture on
  a PaymentIntent.
- Subscriptions are open-ended by default — Stripe ignores
  `SubscriptionRequest.TotalCount` (see [Subscriptions](./subscriptions.md)
  for how this differs from Razorpay).
- `Payment.Raw`, `Refund.Raw`, etc. carry the full Stripe API response as
  `map[string]any` for anything not surfaced on the normalized struct.
