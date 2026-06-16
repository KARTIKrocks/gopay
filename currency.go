package gopay

import (
	"math"
	"strconv"
	"strings"
)

// currencyMinorUnits maps each accepted ISO 4217 currency to the number of
// decimal places in its minor unit (its exponent): 2 for most currencies such
// as USD cents, and 0 for currencies with no minor unit such as JPY.
//
// It is the single source of truth for converting between major and minor
// units. Every currency in validCurrencies must have an entry here; the
// TestCurrencyExponentCoverage test enforces that the two stay in sync so a
// newly accepted currency can't silently inherit a wrong exponent.
var currencyMinorUnits = map[string]int{
	"USD": 2, "EUR": 2, "GBP": 2, "INR": 2, "JPY": 0,
	"CAD": 2, "AUD": 2, "CHF": 2, "CNY": 2, "HKD": 2,
	"SGD": 2, "SEK": 2, "NOK": 2, "DKK": 2, "NZD": 2,
	"ZAR": 2, "MXN": 2, "BRL": 2, "PLN": 2, "THB": 2,
	"MYR": 2, "IDR": 2, "PHP": 2, "CZK": 2, "HUF": 2,
	"ILS": 2, "KRW": 0, "TRY": 2, "RUB": 2, "AED": 2,
	"SAR": 2, "TWD": 2, "ARS": 2, "CLP": 0, "COP": 2,
	"PEN": 2, "NGN": 2, "EGP": 2, "KES": 2, "GHS": 2,
	"BDT": 2, "PKR": 2, "LKR": 2, "MMK": 2, "VND": 0,
}

// minorUnitExponent returns the number of decimal places in a currency's minor
// unit (e.g. 2 for USD, 0 for JPY). It returns (exp, true) for known
// currencies and (0, false) for currencies not in the accepted set.
func minorUnitExponent(currency string) (int, bool) {
	exp, ok := currencyMinorUnits[strings.ToUpper(strings.TrimSpace(currency))]
	return exp, ok
}

// ParseMajorUnitAmount converts a major-unit decimal amount string (e.g.
// "10.00") in the given currency into a normalized minor-unit Amount (e.g.
// 1000 for USD, 500 for "500" JPY). Fractional digits beyond the currency's
// minor unit are rounded half-up.
//
// It returns (nil, false) if the currency is unknown or the string is not a
// valid decimal number, so callers can leave WebhookEvent.Amount nil rather
// than recording a wrong value. Providers that already report integer minor
// units (Stripe, Razorpay) should use NewAmount directly instead.
func ParseMajorUnitAmount(value, currency string) (*Amount, bool) {
	return parseMajorUnitAmount(value, currency)
}

func parseMajorUnitAmount(value, currency string) (*Amount, bool) {
	cur := strings.ToUpper(strings.TrimSpace(currency))
	exp, ok := minorUnitExponent(cur)
	if !ok {
		return nil, false
	}

	neg, intPart, fracPart, ok := splitDecimal(strings.TrimSpace(value))
	if !ok {
		return nil, false
	}

	// Scale the fractional part to exactly exp digits, rounding half-up.
	roundUp := false
	if len(fracPart) > exp {
		roundUp = fracPart[exp] >= '5'
		fracPart = fracPart[:exp]
	} else {
		fracPart += strings.Repeat("0", exp-len(fracPart))
	}

	minor, err := strconv.ParseInt(intPart+fracPart, 10, 64)
	if err != nil {
		return nil, false // overflow or invalid
	}
	if roundUp {
		if minor == math.MaxInt64 {
			return nil, false // rounding would overflow int64
		}
		minor++
	}
	if neg {
		minor = -minor
	}
	return NewAmount(minor, cur), true
}

// splitDecimal validates a decimal amount string and splits it into its sign
// and integer/fractional digit parts. It reports ok=false for empty, sign-only
// ("+"/"-"), dot-only ("."), multi-dot, or non-numeric input. A leading-dot
// form such as ".50" yields intPart "0".
func splitDecimal(s string) (neg bool, intPart, fracPart string, ok bool) {
	if s == "" {
		return false, "", "", false
	}
	switch s[0] {
	case '+':
		s = s[1:]
	case '-':
		neg = true
		s = s[1:]
	}
	if s == "" {
		return false, "", "", false // sign with no digits
	}

	intPart, fracPart, hasDot := strings.Cut(s, ".")
	switch {
	case hasDot && strings.IndexByte(fracPart, '.') >= 0:
		return false, "", "", false // more than one decimal point
	case intPart == "" && fracPart == "":
		return false, "", "", false // a lone "."
	}
	if intPart == "" {
		intPart = "0" // leading-dot form like ".50"
	}
	if !isDigits(intPart) || (fracPart != "" && !isDigits(fracPart)) {
		return false, "", "", false
	}
	return neg, intPart, fracPart, true
}

// isDigits reports whether s is non-empty and contains only ASCII digits.
func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
