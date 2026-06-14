package stripe

import (
	"context"
	"testing"

	"github.com/KARTIKrocks/gopay"
	"github.com/stripe/stripe-go/v81"
)

func TestApplyStripeListParams(t *testing.T) {
	// Defaults: no cursor, default limit, single-page mode.
	var lp stripe.ListParams
	applyStripeListParams(&lp, context.Background(), gopay.NewListParams())
	if !lp.Single {
		t.Error("Single should be true to disable auto-pagination")
	}
	if lp.Limit == nil || *lp.Limit != int64(gopay.DefaultListLimit) {
		t.Errorf("Limit = %v, want %d", lp.Limit, gopay.DefaultListLimit)
	}
	if lp.StartingAfter != nil {
		t.Errorf("StartingAfter should be nil without a cursor, got %v", *lp.StartingAfter)
	}
	if lp.Context == nil {
		t.Error("Context should be set")
	}

	// With a cursor and explicit limit.
	var lp2 stripe.ListParams
	applyStripeListParams(&lp2, context.Background(), gopay.NewListParams().WithLimit(5).WithCursor("pi_123"))
	if lp2.Limit == nil || *lp2.Limit != 5 {
		t.Errorf("Limit = %v, want 5", lp2.Limit)
	}
	if lp2.StartingAfter == nil || *lp2.StartingAfter != "pi_123" {
		t.Errorf("StartingAfter = %v, want pi_123", lp2.StartingAfter)
	}
}

func TestBuildStripeList(t *testing.T) {
	id := func(x *gopay.Payment) string { return x.ID }
	items := []*gopay.Payment{{ID: "pi_1"}, {ID: "pi_2"}}

	// HasMore true => cursor is last item's ID.
	more := buildStripeList(items, &stripe.ListMeta{HasMore: true}, id)
	if !more.HasMore || more.NextCursor != "pi_2" {
		t.Errorf("got HasMore=%v cursor=%q, want true / pi_2", more.HasMore, more.NextCursor)
	}

	// HasMore false => no cursor.
	last := buildStripeList(items, &stripe.ListMeta{HasMore: false}, id)
	if last.HasMore || last.NextCursor != "" {
		t.Errorf("got HasMore=%v cursor=%q, want false / empty", last.HasMore, last.NextCursor)
	}

	// Nil meta => no pagination info.
	nilMeta := buildStripeList(items, nil, id)
	if nilMeta.HasMore || nilMeta.NextCursor != "" {
		t.Errorf("nil meta should yield no more pages, got %+v", nilMeta)
	}

	// HasMore with empty items => no cursor (avoid index panic).
	empty := buildStripeList([]*gopay.Payment{}, &stripe.ListMeta{HasMore: true}, id)
	if empty.NextCursor != "" {
		t.Errorf("empty items should have no cursor, got %q", empty.NextCursor)
	}
}
