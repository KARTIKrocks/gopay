---
id: intro
title: Overview
sidebar_label: Overview
description: A unified payment-processing library for Go with support for Stripe, PayPal, and Razorpay.
slug: /
---

# gopay

A unified payment-processing library for Go with support for **Stripe**,
**PayPal**, and **Razorpay**.

The core package (`github.com/KARTIKrocks/gopay`) defines provider-agnostic
interfaces, types, sentinel errors, a `Client`, and a `MockProvider`. Each
provider lives in its own Go module, so importing one never pulls in the
SDKs of the others.

```bash
go get github.com/KARTIKrocks/gopay
go get github.com/KARTIKrocks/gopay/stripe   # or paypal, razorpay
```

## What you get

| | |
| --- | --- |
| **Unified interface** | One `Client` API across Stripe, PayPal, and Razorpay |
| **Dependency isolation** | Each provider is a separate Go module |
| **Payments** | Create, capture (automatic or manual), get, and cancel |
| **Refunds** | Full and partial refund processing |
| **Customers & payment methods** | Create, manage, attach, detach |
| **Setup intents** | Save a card for later off-session charges |
| **Subscriptions** | Plans and recurring billing |
| **Invoices** | Read-only invoice retrieval with hosted URLs |
| **Webhooks** | Signature-verified, normalized events |
| **Cursor pagination** | A consistent `List`/paginate pattern |
| **Mock provider** | In-memory provider for tests, no network |
| **Thread safe** | Every method is safe for concurrent use |

## Where to go next

- **[Getting Started](./getting-started.md)** — install and create your first payment
- **[Client](./client.md)** — the capability-dispatch pattern every optional feature follows
- **[Stripe](./stripe.md), [PayPal](./paypal.md), [Razorpay](./razorpay.md)** — each provider's support matrix and quirks
- **[API Reference](https://pkg.go.dev/github.com/KARTIKrocks/gopay)** — full
  generated godoc on pkg.go.dev

## Documentation layout

These guides explain concepts, patterns, and provider quirks. For exact type
signatures, method sets, and struct fields, use
[pkg.go.dev](https://pkg.go.dev/github.com/KARTIKrocks/gopay) — it is
generated from the source and is always authoritative.
