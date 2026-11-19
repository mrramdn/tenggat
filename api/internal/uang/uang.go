// Package uang converts money amounts between their JSON form (major units)
// and the integer minor units stored in PostgreSQL (rupiah for IDR, cents for
// USD), without floating-point arithmetic.
package uang

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// MaxMajor is the largest amount accepted, in major units.
const MaxMajor int64 = 1_000_000_000_000

var errInvalid = errors.New("jumlah uang tidak valid")

// Valid reports whether mu is a supported currency code.
func Valid(mu string) bool {
	return mu == "IDR" || mu == "USD"
}

// Exponent returns the number of decimal places a currency has in minor units.
func Exponent(mu string) int {
	if mu == "USD" {
		return 2
	}
	return 0
}

// Parse converts a JSON number in major units into minor units. It accepts only
// plain ASCII digits with an optional fractional part of at most Exponent(mu)
// digits, no sign, no scientific exponent, and at most MaxMajor major units.
func Parse(n json.Number, mu string) (int64, error) {
	if !Valid(mu) {
		return 0, errInvalid
	}
	s := string(n)
	whole, frac, hasDot := strings.Cut(s, ".")
	if whole == "" || !allDigits(whole) {
		return 0, errInvalid
	}
	exp := Exponent(mu)
	if hasDot && (exp == 0 || frac == "" || len(frac) > exp || !allDigits(frac)) {
		return 0, errInvalid
	}
	var major int64
	for i := 0; i < len(whole); i++ {
		major = major*10 + int64(whole[i]-'0')
		if major > MaxMajor {
			return 0, errInvalid
		}
	}
	scale := int64(1)
	for range exp {
		scale *= 10
	}
	var sub int64
	for i := 0; i < exp; i++ {
		sub *= 10
		if i < len(frac) {
			sub += int64(frac[i] - '0')
		}
	}
	minor := major*scale + sub
	if minor > MaxMajor*scale {
		return 0, errInvalid
	}
	return minor, nil
}

func allDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// JSON turns minor units back into a JSON number in major units, dropping
// trailing fractional zeros (2050 USD becomes 20.5, 2000 USD becomes 20).
func JSON(minor int64, mu string) json.Number {
	if Exponent(mu) == 0 {
		return json.Number(strconv.FormatInt(minor, 10))
	}
	whole := strconv.FormatInt(minor/100, 10)
	rem := minor % 100
	if rem == 0 {
		return json.Number(whole)
	}
	if rem%10 == 0 {
		return json.Number(whole + "." + strconv.FormatInt(rem/10, 10))
	}
	return json.Number(fmt.Sprintf("%s.%02d", whole, rem))
}

// Format turns minor units into a display string: "Rp 189.000" or "US$20.50".
func Format(minor int64, mu string) string {
	if mu == "USD" {
		return fmt.Sprintf("US$%d.%02d", minor/100, minor%100)
	}
	return "Rp " + ribuan(minor)
}

func ribuan(n int64) string {
	s := strconv.FormatInt(n, 10)
	if len(s) <= 3 {
		return s
	}
	var b strings.Builder
	rem := len(s) % 3
	if rem > 0 {
		b.WriteString(s[:rem])
	}
	for i := rem; i < len(s); i += 3 {
		if b.Len() > 0 {
			b.WriteByte('.')
		}
		b.WriteString(s[i : i+3])
	}
	return b.String()
}
