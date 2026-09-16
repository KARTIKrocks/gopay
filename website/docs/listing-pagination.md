---
id: listing-pagination
title: Listing & Pagination
description: Cursor-based pagination for payments, refunds, and customers via the optional ListProvider capability.
---

# Listing & Pagination

`ListProvider` adds cursor-paginated listing for payments, refunds, and
customers. It's an optional capability implemented by **Stripe and
Razorpay**; PayPal's Orders API has no list endpoint and returns
`payment.ErrUnsupported`.

```go
params := payment.NewListParams().WithLimit(50)
for {
    page, err := client.ListPayments(ctx, params)
    if err != nil {
        // errors.Is(err, payment.ErrUnsupported) if the provider can't list
        break
    }
    for _, p := range page.Items {
        process(p)
    }
    if !page.HasMore {
        break
    }
    params = params.WithCursor(page.NextCursor)
}
```

`client.ListRefunds` and `client.ListCustomers` follow the exact same shape.

## `ListParams`

- `Limit` — defaults to `DefaultListLimit` (20) when zero or negative;
  `Validate()` rejects a `Limit` above `MaxListLimit` (100).
- `Cursor` — an opaque string from a previous page's `NextCursor`. Empty
  starts from the first page.

## `List[T]`

```go
type List[T any] struct {
    Items      []T
    HasMore    bool
    NextCursor string
}
```

## The cursor is opaque — always pass back what you received

The cursor's actual shape is provider-specific and must not be constructed
or parsed by callers:

- **Stripe** encodes it as an object ID (`starting_after`).
- **Razorpay** encodes it as a skip offset.

Always pass the `NextCursor` you were given straight into the next
`WithCursor` call rather than trying to build one yourself.
