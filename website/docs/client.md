---
id: client
title: Client
description: How Client wraps a Provider, validates requests, and gates optional capabilities behind ErrUnsupported.
---

# Client

`Client` (`payment.go`) is the type every application talks to. It wraps any
`Provider`, validates requests before they reach the provider, and gates
optional capabilities with a runtime type assertion.

```go
client, err := payment.NewClient(provider)
```

`NewClient` rejects a `nil` provider. `provider` can be a real provider
(`stripe.NewProvider(...)`, `paypal.NewProvider(...)`,
`razorpay.NewProvider(...)`) or `payment.NewMockProvider()` for tests — see
[Mock Provider](./mock-provider.md).

## The base surface

Every provider implements `Provider`, so these `Client` methods always work
regardless of which provider you passed in:

- `CreatePayment`, `GetPayment`, `CapturePayment`, `CancelPayment`
- `Refund`, `FullRefund`, `GetRefund`

## Optional capabilities

Everything past the base surface — customers, payment methods, setup
intents, subscriptions, invoices, listing, webhooks — is an **optional
interface** that extends `Provider`. A provider implements an optional
interface only when its API actually supports that capability; PayPal, for
example, has no list endpoint, so it does not implement `ListProvider`.

`Client` follows the same pattern for every optional method: **validate →
type-assert → `ErrUnsupported` → call → wrap error**. `CreateCustomer` is
representative:

```go
func (c *Client) CreateCustomer(ctx context.Context, req *CustomerRequest) (*Customer, error) {
    if err := req.Validate(); err != nil {
        return nil, err
    }
    cp, ok := c.provider.(CustomerProvider)
    if !ok {
        return nil, ErrUnsupported
    }
    customer, err := cp.CreateCustomer(ctx, req)
    if err != nil {
        return nil, fmt.Errorf("create customer: %w", err)
    }
    return customer, nil
}
```

That means every optional call can fail with `payment.ErrUnsupported`, and
callers that work across providers should check for it:

```go
_, err := client.CreateCustomer(ctx, req)
if errors.Is(err, payment.ErrUnsupported) {
    // this provider doesn't support customer management (e.g. PayPal)
}
```

See [Stripe](./stripe.md), [PayPal](./paypal.md), and [Razorpay](./razorpay.md)
for which provider implements which optional interface.

## Optional interfaces at a glance

| Interface | Adds | Defined in |
| --- | --- | --- |
| `CustomerProvider` | Create/get/update/delete customers | `payment.go` |
| `PaymentMethodProvider` | Attach/detach/list payment methods | `payment.go` |
| `WebhookProvider` | `VerifyWebhook` | `payment.go` |
| `ListProvider` | Cursor-paginated `ListPayments`/`ListRefunds`/`ListCustomers` | `list.go` |
| `SetupIntentProvider` | Save a card without charging | `setupintent.go` |
| `SubscriptionProvider` | Plans + recurring billing | `subscription.go` |
| `InvoiceProvider` | Read-only invoice retrieval | `invoice.go` |

## Escape hatches

`client.Provider()` returns the underlying `Provider` if you need to type-assert
to a concrete provider type or an optional interface yourself.
`client.ProviderName()` returns the provider's name (`"stripe"`, `"paypal"`,
`"razorpay"`, `"mock"`) for logging and metrics.
