package razorpay

import (
	"encoding/json"
	"testing"
)

// Razorpay serialises an empty notes bag as `"notes": []` — a JSON array, not an
// object. A plain map[string]string rejects that ("cannot unmarshal array into Go
// value of type map[string]string") and the error takes the whole response with
// it, so any entity created without notes was unreadable.
//
// This was found live against Razorpay test mode: fetching the invoice for a
// subscription charge failed, and the hosted invoice URL never reached the
// database.
func TestNotesAcceptsRazorpayEmptyArray(t *testing.T) {
	// Trimmed from the real payload Razorpay returned for GET /invoices/{id}.
	const body = `{
		"id": "inv_TCt05taRnxgMGF",
		"subscription_id": "sub_TCt058cATmDGb3",
		"status": "paid",
		"amount": 99900,
		"currency": "INR",
		"short_url": "https://rzp.io/i/abc123",
		"notes": []
	}`

	var inv invoice
	if err := json.Unmarshal([]byte(body), &inv); err != nil {
		t.Fatalf("empty notes array failed to decode: %v", err)
	}
	if inv.ShortURL != "https://rzp.io/i/abc123" {
		t.Errorf("short_url = %q, want the hosted invoice URL", inv.ShortURL)
	}
	if inv.Notes != nil {
		t.Errorf("notes = %v, want nil for an empty bag", inv.Notes)
	}
}

func TestNotesRoundTrips(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want map[string]string
	}{
		{"empty array (Razorpay's empty bag)", `[]`, nil},
		{"null", `null`, nil},
		{"empty object", `{}`, map[string]string{}},
		{"populated", `{"salon_id":"abc"}`, map[string]string{"salon_id": "abc"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var n notes
			if err := json.Unmarshal([]byte(tt.in), &n); err != nil {
				t.Fatalf("unmarshal %s: %v", tt.in, err)
			}
			if len(n) != len(tt.want) {
				t.Fatalf("notes = %v, want %v", n, tt.want)
			}
			for k, v := range tt.want {
				if n[k] != v {
					t.Errorf("notes[%q] = %q, want %q", k, n[k], v)
				}
			}
		})
	}
}

// A populated array is not something Razorpay sends, and it is not a shape we
// should guess at: silently discarding it would drop real metadata. Fail loudly.
func TestNotesRejectsPopulatedArray(t *testing.T) {
	var n notes
	if err := json.Unmarshal([]byte(`["unexpected"]`), &n); err == nil {
		t.Fatal("a non-empty notes array decoded silently; real metadata would be dropped without a trace")
	}
}

// The metadata we send must still survive the round trip — the fix must not have
// broken writing notes.
func TestNotesMarshalsBackToObject(t *testing.T) {
	b, err := json.Marshal(notes{"salon_id": "abc"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(b) != `{"salon_id":"abc"}` {
		t.Errorf("marshalled to %s, want a JSON object", b)
	}
}
