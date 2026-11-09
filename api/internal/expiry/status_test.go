package expiry

import (
	"testing"
	"time"
)

func TestForBoundaries(t *testing.T) {
	cases := []struct {
		days int
		want Status
	}{
		{-30, Lewat}, {-1, Lewat},
		{0, Kritis}, {7, Kritis},
		{8, Segera}, {30, Segera},
		{31, Aman}, {365, Aman},
	}
	for _, c := range cases {
		if got := For(c.days); got != c.want {
			t.Errorf("For(%d) = %s, want %s", c.days, got, c.want)
		}
	}
}

func TestDaysLeftUsesJakartaDate(t *testing.T) {
	expiry := time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC)

	// 16:30 UTC on 9 Nov is 23:30 WIB, still 9 Nov in Jakarta.
	if got := DaysLeft(expiry, time.Date(2026, 11, 9, 16, 30, 0, 0, time.UTC)); got != 4 {
		t.Errorf("late evening: got %d days, want 4", got)
	}
	// 17:30 UTC on 9 Nov is 00:30 WIB on 10 Nov.
	if got := DaysLeft(expiry, time.Date(2026, 11, 9, 17, 30, 0, 0, time.UTC)); got != 3 {
		t.Errorf("after midnight WIB: got %d days, want 3", got)
	}
	// On the expiry day itself.
	if got := DaysLeft(expiry, time.Date(2026, 11, 13, 1, 0, 0, 0, time.UTC)); got != 0 {
		t.Errorf("expiry day: got %d days, want 0", got)
	}
	// The day after.
	if got := DaysLeft(expiry, time.Date(2026, 11, 14, 1, 0, 0, 0, time.UTC)); got != -1 {
		t.Errorf("day after: got %d days, want -1", got)
	}
}

func TestDateOf(t *testing.T) {
	// A certificate ending at 20:00 UTC on 6 Nov ends on 7 Nov in Jakarta.
	got := DateOf(time.Date(2026, 11, 6, 20, 0, 0, 0, time.UTC))
	want := time.Date(2026, 11, 7, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("DateOf = %v, want %v", got, want)
	}
}

func TestValid(t *testing.T) {
	if !Kritis.Valid() || Status("hampir").Valid() {
		t.Error("Valid does not match the known statuses")
	}
}
