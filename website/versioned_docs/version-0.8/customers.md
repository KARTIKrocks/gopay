---
id: customers
title: Customers
description: Create, retrieve, update, and delete customers via the optional CustomerProvider capability.
---

# Customers

Customer management is an optional capability: providers implement
`CustomerProvider` only when their API supports it. PayPal does not (its
Orders API has no customer object), so those calls return
`payment.ErrUnsupported` for PayPal. See [Stripe](./stripe.md),
[PayPal](./paypal.md), and [Razorpay](./razorpay.md) for the full picture.

```go
customer, err := client.CreateCustomer(ctx, payment.NewCustomerRequest("jane@example.com").
    WithName("Jane Doe").
    WithPhone("+1-555-0100").
    WithMetadata("account_id", "acct_42"))
```

`CustomerRequest.Validate()` requires a non-empty `Email`.

## Retrieving, updating, and deleting

```go
customer, err := client.GetCustomer(ctx, customerID)

customer, err = client.UpdateCustomer(ctx, customerID, payment.NewCustomerRequest("jane@example.com").
    WithName("Jane D."))

err = client.DeleteCustomer(ctx, customerID)
```

An empty ID returns `payment.ErrNotFound` without a network call.

## `Customer` fields

`ID`, `Email`, `Name`, `Phone`, `Description`, `DefaultPaymentMethodID`,
`Metadata`, `CreatedAt`, `Provider`, and `Raw` (the unmodified provider
response, for anything not normalized).

## Next steps

- **[Payment Methods](./payment-methods.md)** — attach a payment method to a customer
- **[Setup Intents](./setup-intents.md)** — save a card to a customer for later
- **[Subscriptions](./subscriptions.md)** — subscribe a customer to a recurring plan
