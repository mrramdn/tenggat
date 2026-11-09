package expiry

import (
	"testing"
	"time"
)

func date(s string) time.Time {
	d, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return d
}

func TestNextDue(t *testing.T) {
	for _, c := range []struct {
		due, today string
		n          int
		unit       string
		want       string
	}{
		{"2026-11-09", "2026-11-09", 1, "bulan", "2026-12-09"},  // done on the due date
		{"2026-12-01", "2026-11-09", 1, "bulan", "2027-01-01"},  // done early still moves one interval
		{"2026-11-05", "2026-11-09", 1, "minggu", "2026-11-12"}, // a few days late
		{"2026-11-01", "2026-11-09", 3, "hari", "2026-11-10"},   // 4, 7, then 10
		{"2026-01-31", "2026-02-01", 1, "bulan", "2026-02-28"},  // month end, no leap year
		{"2028-01-31", "2028-02-01", 1, "bulan", "2028-02-29"},  // leap year
		{"2026-01-31", "2026-03-01", 1, "bulan", "2026-03-31"},  // counted from the old date, not from 28 Feb
		{"2026-08-31", "2026-09-01", 1, "bulan", "2026-09-30"},
		{"2024-02-29", "2024-03-01", 1, "tahun", "2025-02-28"},
		{"2026-11-30", "2026-11-30", 3, "bulan", "2027-02-28"},
		{"2019-03-15", "2026-11-09", 1, "bulan", "2026-11-15"}, // overdue for years
		{"2016-11-09", "2026-11-09", 1, "hari", "2026-11-10"},  // overdue for ten years, daily
		{"2001-06-01", "2026-11-09", 2, "tahun", "2027-06-01"},
		{"2026-10-09", "2026-11-09", 1, "bulan", "2026-12-09"}, // exactly one interval late
	} {
		got := NextDue(date(c.due), date(c.today), c.n, c.unit).Format(time.DateOnly)
		if got != c.want {
			t.Errorf("NextDue(%s, today %s, %d %s) = %s, want %s", c.due, c.today, c.n, c.unit, got, c.want)
		}
	}
}

func TestAddMonths(t *testing.T) {
	for _, c := range []struct {
		from   string
		months int
		want   string
	}{
		{"2026-01-31", 1, "2026-02-28"}, {"2026-03-31", -1, "2026-02-28"}, {"2026-12-15", 1, "2027-01-15"},
		{"2026-05-31", 1, "2026-06-30"}, {"2026-01-30", 13, "2027-02-28"},
	} {
		if got := AddMonths(date(c.from), c.months).Format(time.DateOnly); got != c.want {
			t.Errorf("AddMonths(%s, %d) = %s, want %s", c.from, c.months, got, c.want)
		}
	}
}
