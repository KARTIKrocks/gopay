package razorpay

import (
	"context"
	"net/http"
	"testing"

	"github.com/KARTIKrocks/gopay"
)

func TestCursorToSkip(t *testing.T) {
	cases := []struct {
		name   string
		params *gopay.ListParams
		want   int
	}{
		{"nil", nil, 0},
		{"empty", gopay.NewListParams(), 0},
		{"valid", gopay.NewListParams().WithCursor("40"), 40},
		{"negative", gopay.NewListParams().WithCursor("-5"), 0},
		{"garbage", gopay.NewListParams().WithCursor("abc"), 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := cursorToSkip(c.params); got != c.want {
				t.Errorf("cursorToSkip() = %d, want %d", got, c.want)
			}
		})
	}
}

func TestBuildRazorpayList(t *testing.T) {
	// Full page => more available, cursor advances.
	full := buildRazorpayList([]int{1, 2}, 0, 2)
	if !full.HasMore || full.NextCursor != "2" {
		t.Errorf("full page = %+v, want HasMore and cursor 2", full)
	}
	// Partial page => last page.
	partial := buildRazorpayList([]int{1}, 2, 2)
	if partial.HasMore || partial.NextCursor != "" {
		t.Errorf("partial page = %+v, want last page", partial)
	}
}

func TestListPaymentsHTTP(t *testing.T) {
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/payments" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		q := r.URL.Query()
		if q.Get("count") != "2" {
			t.Errorf("count = %q, want 2", q.Get("count"))
		}
		if q.Get("skip") != "10" {
			t.Errorf("skip = %q, want 10", q.Get("skip"))
		}
		user, pass, ok := r.BasicAuth()
		if !ok || user != "key_test" || pass != "secret_test" {
			t.Errorf("BasicAuth = (%s, %s, %v)", user, pass, ok)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"entity":"collection","count":2,"items":[
			{"id":"pay_1","amount":1000,"currency":"INR","status":"captured","created_at":1700000000},
			{"id":"pay_2","amount":2000,"currency":"INR","status":"authorized","created_at":1700000100}
		]}`))
	})

	list, err := p.ListPayments(context.Background(), gopay.NewListParams().WithLimit(2).WithCursor("10"))
	if err != nil {
		t.Fatalf("ListPayments: %v", err)
	}
	if len(list.Items) != 2 {
		t.Fatalf("len = %d, want 2", len(list.Items))
	}
	if list.Items[0].ID != "pay_1" || list.Items[1].ID != "pay_2" {
		t.Errorf("items = %s,%s", list.Items[0].ID, list.Items[1].ID)
	}
	if list.Items[0].Status != gopay.PaymentStatusSucceeded {
		t.Errorf("status[0] = %s, want succeeded", list.Items[0].Status)
	}
	// Full page (count==len) => more pages, cursor advances to skip+len.
	if !list.HasMore || list.NextCursor != "12" {
		t.Errorf("HasMore=%v cursor=%q, want true / 12", list.HasMore, list.NextCursor)
	}
}

func TestListRefundsHTTP(t *testing.T) {
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/refunds" {
			t.Errorf("path = %s, want /refunds", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"entity":"collection","count":1,"items":[
			{"id":"rfnd_1","payment_id":"pay_1","amount":500,"currency":"INR","status":"processed","created_at":1700000000}
		]}`))
	})

	list, err := p.ListRefunds(context.Background(), gopay.NewListParams().WithLimit(10))
	if err != nil {
		t.Fatalf("ListRefunds: %v", err)
	}
	if len(list.Items) != 1 || list.Items[0].ID != "rfnd_1" {
		t.Fatalf("items = %+v", list.Items)
	}
	if list.Items[0].Status != gopay.RefundStatusSucceeded {
		t.Errorf("status = %s, want succeeded", list.Items[0].Status)
	}
	// Partial page (1 < 10) => last page.
	if list.HasMore || list.NextCursor != "" {
		t.Errorf("HasMore=%v cursor=%q, want last page", list.HasMore, list.NextCursor)
	}
}

func TestListCustomersHTTP(t *testing.T) {
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/customers" {
			t.Errorf("path = %s, want /customers", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"entity":"collection","count":1,"items":[
			{"id":"cust_1","name":"Asha","email":"asha@example.com","contact":"+91999","created_at":1700000000}
		]}`))
	})

	list, err := p.ListCustomers(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListCustomers: %v", err)
	}
	if len(list.Items) != 1 || list.Items[0].Email != "asha@example.com" {
		t.Fatalf("items = %+v", list.Items)
	}
}

func TestListPaymentsErrorHTTP(t *testing.T) {
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"code":"SERVER_ERROR","description":"boom"}}`))
	})

	_, err := p.ListPayments(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error from server failure")
	}
}
