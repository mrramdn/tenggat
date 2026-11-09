package expiry

import "time"

// Units a recurring task repeats in.
var Units = map[string]bool{"hari": true, "minggu": true, "bulan": true, "tahun": true}

// NextDue moves a recurring task's due date forward by whole intervals of n
// units until it is after today, and always by at least one interval. Counting
// every step from the old due date keeps a month-end date on the month end:
// 31 Jan monthly gives 28 Feb, then 31 Mar when one completion skips a month.
// due and today are dates at midnight UTC (see DateOf).
func NextDue(due, today time.Time, n int, unit string) time.Time {
	if n < 1 {
		n = 1
	}
	step := func(k int) time.Time {
		switch unit {
		case "hari":
			return due.AddDate(0, 0, k*n)
		case "minggu":
			return due.AddDate(0, 0, 7*k*n)
		case "tahun":
			return AddMonths(due, 12*k*n)
		default:
			return AddMonths(due, k*n)
		}
	}
	k := 1
	// Jump close to today first, so a task overdue for years needs few steps.
	if days := int(today.Sub(due).Hours() / 24); days > 0 {
		perStep := map[string]int{"hari": n, "minggu": 7 * n, "bulan": 31 * n, "tahun": 366 * n}[unit]
		if perStep == 0 {
			perStep = 31 * n
		}
		if skip := days / perStep; skip > 1 {
			k = skip
		}
	}
	next := step(k)
	for !next.After(today) {
		k++
		next = step(k)
	}
	return next
}

// AddMonths adds whole months to a date, keeping the day of the month but never
// running into the next month: 31 Jan plus one month is 28 (or 29) Feb.
func AddMonths(d time.Time, months int) time.Time {
	y, m, day := d.Date()
	first := time.Date(y, m+time.Month(months), 1, 0, 0, 0, 0, time.UTC)
	last := first.AddDate(0, 1, -1).Day()
	if day > last {
		day = last
	}
	return time.Date(first.Year(), first.Month(), day, 0, 0, 0, 0, time.UTC)
}
