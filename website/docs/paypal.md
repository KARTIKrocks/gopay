---
id: paypal
title: PayPal
description: Configuring the PayPal provider — payments, refunds, and webhooks via the Orders API.
---

# PayPal

`github.com/KARTIKrocks/gopay/paypal` wraps PayPal's Orders API. It's the
narrowest provider: no customer object, no payment methods, no setup
intents, no subscriptions/invoices, and no list endpoint.

```go
import (
    payment "github.com/KARTIKrocks/gopay"
    "github.com/KARTIKrocks/gopay/paypal"
)

config := paypal.DefaultConfig().
    WithCredentials("client_id", "client_secret").
    WithSandbox(true)

provider, err := paypal.NewProvider(config)
if err != nil {
    log.Fatal(err)
}

client, err := payment.NewClient(provider)
```

`NewProvider` returns `payment.ErrInvalidConfig` if `ClientID` or
`ClientSecret` is empty.

## `Config`

| Field | Set via | Notes |
| --- | --- | --- |
| `ClientID`, `ClientSecret` | `WithCredentials` | Required |
| `Sandbox` | `WithSandbox` | **`true` by default** in `DefaultConfig()` — call `WithSandbox(false)` for production |
| `WebhookID` | `WithWebhookID` | Required to call `VerifyWebhook` |
| `HTTPClient` | `WithHTTPClient` | Optional; `DefaultConfig()` sets a 30s timeout |

## Support matrix

| Capability | Supported |
| --- | --- |
| Payments, Refunds | ✓ |
| Customers | – |
| Payment Methods | – |
| Setup Intents | – |
| Subscriptions | – |
| Invoices | – |
| Webhooks | ✓ (`PAYPAL-TRANSMISSION-SIG` + related `PAYPAL-*` headers) |
| Listing | – (the Orders API has no list endpoint) |

Every unsupported capability above returns `payment.ErrUnsupported` from the
`Client` rather than panicking or silently no-op'ing — see [Errors](./errors.md).

## Notes

- Amounts sent to PayPal are major-unit decimal strings (e.g. `"19.99"`).
  gopay converts to/from these automatically, including on webhook payloads
  via `ParseMajorUnitAmount` — you always work in integer minor units.
- Access tokens are fetched and cached internally; you don't need to manage
  OAuth yourself.
