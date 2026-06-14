package gopay

import (
	"context"
	"errors"
)

// Default and maximum page sizes for list operations. When ListParams.Limit is
// zero or negative, DefaultListLimit is used; Limit may not exceed MaxListLimit.
const (
	DefaultListLimit = 20
	MaxListLimit     = 100
)

// ListParams controls pagination for list operations.
//
// Pagination is cursor-based: each List result carries an opaque NextCursor that
// is passed back via WithCursor to fetch the following page. The cursor is
// provider-specific and must be treated as opaque by callers.
type ListParams struct {
	// Limit is the maximum number of items to return per page. When zero or
	// negative, the provider's default page size (DefaultListLimit) is used.
	Limit int

	// Cursor is an opaque pagination cursor returned as NextCursor by a previous
	// page. Empty starts from the first page.
	Cursor string
}

// NewListParams creates a new ListParams with default pagination.
func NewListParams() *ListParams {
	return &ListParams{}
}

// WithLimit sets the maximum number of items per page.
func (p *ListParams) WithLimit(limit int) *ListParams {
	p.Limit = limit
	return p
}

// WithCursor sets the pagination cursor (the NextCursor from a previous page).
func (p *ListParams) WithCursor(cursor string) *ListParams {
	p.Cursor = cursor
	return p
}

// Validate validates the list parameters.
func (p *ListParams) Validate() error {
	if p == nil {
		return nil
	}
	if p.Limit > MaxListLimit {
		return errors.New("gopay: list limit exceeds maximum")
	}
	return nil
}

// EffectiveLimit returns the page size to request, applying DefaultListLimit when
// Limit is unset (zero or negative). It is a helper for provider implementations.
func (p *ListParams) EffectiveLimit() int {
	if p == nil || p.Limit <= 0 {
		return DefaultListLimit
	}
	return p.Limit
}

// List is a single page of results from a paginated list operation.
//
// HasMore reports whether further pages exist; when true, NextCursor holds the
// opaque cursor to pass to the next request (via ListParams.WithCursor).
type List[T any] struct {
	// Items are the results on this page.
	Items []T

	// HasMore reports whether additional pages are available.
	HasMore bool

	// NextCursor is the opaque cursor for the next page; empty when HasMore is
	// false.
	NextCursor string
}

// ListProvider extends Provider with listing and pagination capabilities.
//
// It is an optional interface: providers implement it only when their API
// supports listing. The Client gates each method with a runtime type assertion
// and returns ErrUnsupported for providers that don't implement it.
type ListProvider interface {
	Provider

	// ListPayments lists payments with cursor-based pagination.
	ListPayments(ctx context.Context, params *ListParams) (*List[*Payment], error)

	// ListRefunds lists refunds with cursor-based pagination.
	ListRefunds(ctx context.Context, params *ListParams) (*List[*Refund], error)

	// ListCustomers lists customers with cursor-based pagination.
	ListCustomers(ctx context.Context, params *ListParams) (*List[*Customer], error)
}
