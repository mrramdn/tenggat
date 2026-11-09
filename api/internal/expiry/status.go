// Package expiry decides how urgent an expiry date is.
package expiry

import "time"

// Status is the urgency of an item, derived from the days left.
type Status string

const (
	Lewat  Status = "lewat"  // already expired
	Kritis Status = "kritis" // 0 to 7 days left
	Segera Status = "segera" // 8 to 30 days left
	Aman   Status = "aman"   // more than 30 days left
)

// Statuses lists every status from most to least urgent.
var Statuses = []Status{Lewat, Kritis, Segera, Aman}

// Valid reports whether s is one of the known statuses.
func (s Status) Valid() bool {
	for _, v := range Statuses {
		if s == v {
			return true
		}
	}
	return false
}

// Jakarta decides which calendar day it is. Asia/Jakarta has been UTC+7 without
// daylight saving since 1964, so a fixed zone is an exact fallback when the
// time zone database is missing.
var Jakarta = loadJakarta()

func loadJakarta() *time.Location {
	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		return time.FixedZone("WIB", 7*60*60)
	}
	return loc
}

// DateOf returns the calendar date of t in Jakarta, as midnight UTC. Dates read
// from PostgreSQL use the same form, so the two can be compared directly.
func DateOf(t time.Time) time.Time {
	y, m, d := t.In(Jakarta).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// DaysLeft counts whole calendar days from today in Jakarta to the expiry date.
// It is 0 on the expiry day itself and negative once the date has passed.
func DaysLeft(expiry, now time.Time) int {
	y, m, d := expiry.Date()
	exp := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	return int(exp.Sub(DateOf(now)).Hours() / 24)
}

// For maps the days left to a status.
func For(daysLeft int) Status {
	switch {
	case daysLeft < 0:
		return Lewat
	case daysLeft <= 7:
		return Kritis
	case daysLeft <= 30:
		return Segera
	default:
		return Aman
	}
}
