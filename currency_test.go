package gopay

import "testing"

// TestCurrencyExponentCoverage enforces that currencyMinorUnits and
// validCurrencies stay in sync: every accepted currency must have an explicit
// minor-unit exponent (so a newly accepted currency can't silently inherit a
// wrong default), and the table must not carry exponents for currencies the
// library doesn't accept.
func TestCurrencyExponentCoverage(t *testing.T) {
	for cur := range validCurrencies {
		if _, ok := currencyMinorUnits[cur]; !ok {
			t.Errorf("currency %q is in validCurrencies but missing from currencyMinorUnits", cur)
		}
	}
	for cur := range currencyMinorUnits {
		if !validCurrencies[cur] {
			t.Errorf("currency %q has an exponent but is not in validCurrencies", cur)
		}
	}
}

func TestMinorUnitExponent(t *testing.T) {
	tests := []struct {
		currency string
		wantExp  int
		wantOK   bool
	}{
		{"USD", 2, true},
		{"usd", 2, true}, // case-insensitive
		{" EUR ", 2, true},
		{"JPY", 0, true},
		{"KRW", 0, true},
		{"CLP", 0, true},
		{"VND", 0, true},
		{"XYZ", 0, false}, // unknown
		{"", 0, false},
	}
	for _, tt := range tests {
		exp, ok := minorUnitExponent(tt.currency)
		if exp != tt.wantExp || ok != tt.wantOK {
			t.Errorf("minorUnitExponent(%q) = (%d, %v), want (%d, %v)", tt.currency, exp, ok, tt.wantExp, tt.wantOK)
		}
	}
}

func TestParseMajorUnitAmount(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		currency  string
		wantValue int64
		wantCur   string
		wantOK    bool
	}{
		{"two-decimal exact", "10.00", "USD", 1000, "USD", true},
		{"two-decimal cents", "19.99", "USD", 1999, "USD", true},
		{"one fractional digit", "5.5", "EUR", 550, "EUR", true},
		{"no fractional part", "7", "GBP", 700, "GBP", true},
		{"leading dot", ".50", "USD", 50, "USD", true},
		{"zero-decimal integer", "500", "JPY", 500, "JPY", true},
		{"zero-decimal trailing zeros", "500.00", "JPY", 500, "JPY", true},
		{"zero-decimal rounds extra digits", "500.6", "JPY", 501, "JPY", true},
		{"round half up", "1.005", "USD", 101, "USD", true},
		{"round down", "1.004", "USD", 100, "USD", true},
		{"truncate then round", "1.999", "USD", 200, "USD", true},
		{"negative", "-3.50", "USD", -350, "USD", true},
		{"currency normalized to upper", "1.00", "usd", 100, "USD", true},
		{"unknown currency", "1.00", "XYZ", 0, "", false},
		{"empty value", "", "USD", 0, "", false},
		{"non-numeric", "abc", "USD", 0, "", false},
		{"two dots", "1.0.0", "USD", 0, "", false},
		{"trailing junk", "1.0x", "USD", 0, "", false},
		{"sign only plus", "+", "USD", 0, "", false},
		{"sign only minus", "-", "USD", 0, "", false},
		{"dot only", ".", "USD", 0, "", false},
		{"rounding overflow", "92233720368547758.075", "USD", 0, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			amount, ok := parseMajorUnitAmount(tt.value, tt.currency)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !tt.wantOK {
				if amount != nil {
					t.Fatalf("amount = %v, want nil on failure", amount)
				}
				return
			}
			if amount.Value != tt.wantValue {
				t.Errorf("Value = %d, want %d", amount.Value, tt.wantValue)
			}
			if amount.Currency != tt.wantCur {
				t.Errorf("Currency = %s, want %s", amount.Currency, tt.wantCur)
			}
		})
	}
}
