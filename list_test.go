package gopay

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestListParamsValidate(t *testing.T) {
	if err := (*ListParams)(nil).Validate(); err != nil {
		t.Errorf("nil params should validate, got %v", err)
	}
	if err := NewListParams().Validate(); err != nil {
		t.Errorf("zero params should validate, got %v", err)
	}
	if err := NewListParams().WithLimit(MaxListLimit).Validate(); err != nil {
		t.Errorf("max limit should validate, got %v", err)
	}
	if err := NewListParams().WithLimit(MaxListLimit + 1).Validate(); err == nil {
		t.Error("limit over max should fail validation")
	}
}

func TestListParamsEffectiveLimit(t *testing.T) {
	cases := []struct {
		name   string
		params *ListParams
		want   int
	}{
		{"nil", nil, DefaultListLimit},
		{"zero", NewListParams(), DefaultListLimit},
		{"negative", NewListParams().WithLimit(-5), DefaultListLimit},
		{"set", NewListParams().WithLimit(7), 7},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.params.EffectiveLimit(); got != c.want {
				t.Errorf("EffectiveLimit() = %d, want %d", got, c.want)
			}
		})
	}
}

func TestListParamsBuilder(t *testing.T) {
	p := NewListParams().WithLimit(5).WithCursor("abc")
	if p.Limit != 5 || p.Cursor != "abc" {
		t.Errorf("builder produced %+v", p)
	}
}

// seedPayments creates n succeeded payments on the mock with strictly increasing
// CreatedAt so list ordering is deterministic.
func seedPayments(t *testing.T, mock *MockProvider, n int) {
	t.Helper()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := range n {
		mock.SetPayment(&Payment{
			ID:        "pi_" + string(rune('a'+i)),
			Amount:    USD(int64((i + 1) * 100)),
			Status:    PaymentStatusSucceeded,
			CreatedAt: base.Add(time.Duration(i) * time.Hour),
			Provider:  "mock",
		})
	}
}

func TestClientListPaymentsPagination(t *testing.T) {
	mock := NewMockProvider()
	seedPayments(t, mock, 5)
	client, err := NewClient(mock)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	ctx := context.Background()

	// First page of 2.
	page1, err := client.ListPayments(ctx, NewListParams().WithLimit(2))
	if err != nil {
		t.Fatalf("ListPayments page1: %v", err)
	}
	if len(page1.Items) != 2 {
		t.Fatalf("page1 len = %d, want 2", len(page1.Items))
	}
	if !page1.HasMore || page1.NextCursor == "" {
		t.Errorf("page1 should have more; got HasMore=%v cursor=%q", page1.HasMore, page1.NextCursor)
	}
	// Newest-first ordering: pi_e (i=4) then pi_d (i=3).
	if page1.Items[0].ID != "pi_e" || page1.Items[1].ID != "pi_d" {
		t.Errorf("page1 order = %s,%s want pi_e,pi_d", page1.Items[0].ID, page1.Items[1].ID)
	}

	// Walk all pages and confirm we see every payment exactly once, in order.
	var got []string
	cursor := ""
	for {
		page, err := client.ListPayments(ctx, NewListParams().WithLimit(2).WithCursor(cursor))
		if err != nil {
			t.Fatalf("ListPayments: %v", err)
		}
		for _, p := range page.Items {
			got = append(got, p.ID)
		}
		if !page.HasMore {
			break
		}
		cursor = page.NextCursor
	}
	want := []string{"pi_e", "pi_d", "pi_c", "pi_b", "pi_a"}
	if len(got) != len(want) {
		t.Fatalf("walked %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("walked %v, want %v", got, want)
		}
	}
}

func TestClientListPaymentsLastPageNoMore(t *testing.T) {
	mock := NewMockProvider()
	seedPayments(t, mock, 3)
	client, _ := NewClient(mock)

	page, err := client.ListPayments(context.Background(), NewListParams().WithLimit(10))
	if err != nil {
		t.Fatalf("ListPayments: %v", err)
	}
	if len(page.Items) != 3 {
		t.Errorf("len = %d, want 3", len(page.Items))
	}
	if page.HasMore || page.NextCursor != "" {
		t.Errorf("single page should not have more; got HasMore=%v cursor=%q", page.HasMore, page.NextCursor)
	}
}

func TestClientListPaymentsEmpty(t *testing.T) {
	mock := NewMockProvider()
	client, _ := NewClient(mock)

	page, err := client.ListPayments(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListPayments: %v", err)
	}
	if len(page.Items) != 0 || page.HasMore {
		t.Errorf("empty provider should yield empty page; got %+v", page)
	}
}

func TestClientListRefundsAndCustomers(t *testing.T) {
	mock := NewMockProvider()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	mock.SetRefund(&Refund{ID: "re_a", Amount: USD(100), Status: RefundStatusSucceeded, CreatedAt: base, Provider: "mock"})
	mock.SetRefund(&Refund{ID: "re_b", Amount: USD(200), Status: RefundStatusSucceeded, CreatedAt: base.Add(time.Hour), Provider: "mock"})
	mock.SetCustomer(&Customer{ID: "cus_a", Email: "a@example.com", CreatedAt: base, Provider: "mock"})

	client, _ := NewClient(mock)
	ctx := context.Background()

	refunds, err := client.ListRefunds(ctx, nil)
	if err != nil {
		t.Fatalf("ListRefunds: %v", err)
	}
	if len(refunds.Items) != 2 || refunds.Items[0].ID != "re_b" {
		t.Errorf("refunds = %+v, want newest-first [re_b, re_a]", refunds.Items)
	}

	customers, err := client.ListCustomers(ctx, nil)
	if err != nil {
		t.Fatalf("ListCustomers: %v", err)
	}
	if len(customers.Items) != 1 || customers.Items[0].ID != "cus_a" {
		t.Errorf("customers = %+v, want [cus_a]", customers.Items)
	}
}

func TestClientListValidationError(t *testing.T) {
	mock := NewMockProvider()
	client, _ := NewClient(mock)

	_, err := client.ListPayments(context.Background(), NewListParams().WithLimit(MaxListLimit+1))
	if err == nil {
		t.Error("expected validation error for over-limit params")
	}
}

// baseOnlyProvider implements just the base Provider interface (not ListProvider)
// to exercise the ErrUnsupported path.
type baseOnlyProvider struct{}

func (baseOnlyProvider) Name() string { return "base" }
func (baseOnlyProvider) CreatePayment(context.Context, *PaymentRequest) (*Payment, error) {
	return nil, nil
}
func (baseOnlyProvider) GetPayment(context.Context, string) (*Payment, error) { return nil, nil }
func (baseOnlyProvider) CapturePayment(context.Context, string, *Amount) (*Payment, error) {
	return nil, nil
}
func (baseOnlyProvider) CancelPayment(context.Context, string) (*Payment, error) { return nil, nil }
func (baseOnlyProvider) Refund(context.Context, *RefundRequest) (*Refund, error) { return nil, nil }
func (baseOnlyProvider) GetRefund(context.Context, string) (*Refund, error)      { return nil, nil }

func TestClientListUnsupported(t *testing.T) {
	client, err := NewClient(baseOnlyProvider{})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	ctx := context.Background()

	if _, err := client.ListPayments(ctx, nil); !errors.Is(err, ErrUnsupported) {
		t.Errorf("ListPayments err = %v, want ErrUnsupported", err)
	}
	if _, err := client.ListRefunds(ctx, nil); !errors.Is(err, ErrUnsupported) {
		t.Errorf("ListRefunds err = %v, want ErrUnsupported", err)
	}
	if _, err := client.ListCustomers(ctx, nil); !errors.Is(err, ErrUnsupported) {
		t.Errorf("ListCustomers err = %v, want ErrUnsupported", err)
	}
}
