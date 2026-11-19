package uang

import (
	"encoding/json"
	"testing"
)

func TestValidAndExponent(t *testing.T) {
	for _, c := range []struct {
		mu    string
		valid bool
		exp   int
	}{
		{"IDR", true, 0},
		{"USD", true, 2},
		{"SGD", true, 2},
		{"", false, 0},
		{"idr", false, 0},
		{"sgd", false, 0},
		{"EUR", false, 0},
	} {
		if got := Valid(c.mu); got != c.valid {
			t.Errorf("Valid(%q) = %v, want %v", c.mu, got, c.valid)
		}
		if got := Exponent(c.mu); got != c.exp {
			t.Errorf("Exponent(%q) = %d, want %d", c.mu, got, c.exp)
		}
	}
}

func TestParseJSONAndFormat(t *testing.T) {
	for _, c := range []struct {
		in      json.Number
		mu      string
		minor   int64
		wantJ   json.Number
		wantFmt string
	}{
		{"0", "IDR", 0, "0", "Rp 0"},
		{"500", "IDR", 500, "500", "Rp 500"},
		{"1000", "IDR", 1000, "1000", "Rp 1.000"},
		{"189000", "IDR", 189000, "189000", "Rp 189.000"},
		{"1250000", "IDR", 1250000, "1250000", "Rp 1.250.000"},
		{"1000000000000", "IDR", 1_000_000_000_000, "1000000000000", "Rp 1.000.000.000.000"},
		{"0", "USD", 0, "0", "US$0.00"},
		{"0.05", "USD", 5, "0.05", "US$0.05"},
		{"0.5", "USD", 50, "0.5", "US$0.50"},
		{"0.50", "USD", 50, "0.5", "US$0.50"},
		{"20", "USD", 2000, "20", "US$20.00"},
		{"20.00", "USD", 2000, "20", "US$20.00"},
		{"20.5", "USD", 2050, "20.5", "US$20.50"},
		{"20.50", "USD", 2050, "20.5", "US$20.50"},
		{"20.25", "USD", 2025, "20.25", "US$20.25"},
		{"1000000000000", "USD", 100_000_000_000_000, "1000000000000", "US$1000000000000.00"},
		{"1000000000000.00", "USD", 100_000_000_000_000, "1000000000000", "US$1000000000000.00"},
		{"0", "SGD", 0, "0", "S$0.00"},
		{"0.05", "SGD", 5, "0.05", "S$0.05"},
		{"0.5", "SGD", 50, "0.5", "S$0.50"},
		{"20", "SGD", 2000, "20", "S$20.00"},
		{"20.5", "SGD", 2050, "20.5", "S$20.50"},
		{"20.50", "SGD", 2050, "20.5", "S$20.50"},
		{"20.25", "SGD", 2025, "20.25", "S$20.25"},
		{"1000000000000.00", "SGD", 100_000_000_000_000, "1000000000000", "S$1000000000000.00"},
	} {
		got, err := Parse(c.in, c.mu)
		if err != nil || got != c.minor {
			t.Errorf("Parse(%q, %q) = %d, %v; want %d, nil", c.in, c.mu, got, err, c.minor)
		}
		if j := JSON(c.minor, c.mu); j != c.wantJ {
			t.Errorf("JSON(%d, %q) = %q, want %q", c.minor, c.mu, j, c.wantJ)
		}
		if f := Format(c.minor, c.mu); f != c.wantFmt {
			t.Errorf("Format(%d, %q) = %q, want %q", c.minor, c.mu, f, c.wantFmt)
		}
	}
}

func TestParseRejects(t *testing.T) {
	for _, c := range []struct {
		in json.Number
		mu string
	}{
		{"", "IDR"},
		{"", "USD"},
		{"100", ""},
		{"100", "EUR"},
		{"100", "idr"},
		{"-1", "IDR"},
		{"+1", "IDR"},
		{"-0", "USD"},
		{"+20.50", "USD"},
		{"189000.0", "IDR"},
		{"189000.5", "IDR"},
		{"189.000", "IDR"},
		{"20.", "USD"},
		{".5", "USD"},
		{".50", "USD"},
		{"20.555", "USD"},
		{"20.500", "USD"},
		{"20.1.2", "USD"},
		{"1e5", "IDR"},
		{"1E5", "USD"},
		{"2e-1", "USD"},
		{"20,50", "USD"},
		{" 100", "IDR"},
		{"100 ", "IDR"},
		{"abc", "IDR"},
		{"12a", "USD"},
		{"20.5a", "USD"},
		{"1000000000001", "IDR"},
		{"1000000000000.01", "USD"},
		{"1000000000001", "USD"},
		{"", "SGD"},
		{"-1", "SGD"},
		{"20.555", "SGD"},
		{"1000000000000.01", "SGD"},
		{"99999999999999999999999999", "IDR"},
	} {
		if got, err := Parse(c.in, c.mu); err == nil {
			t.Errorf("Parse(%q, %q) = %d, nil; want error", c.in, c.mu, got)
		}
	}
}
