package httpapi

import (
	"bufio"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mrramdn/tenggat/api/internal/store"
)

var jenisLabel = map[string]string{
	"sertifikat": "Sertifikat TLS",
	"domain":     "Domain",
	"token":      "Token API",
	"langganan":  "Langganan",
	"lainnya":    "Lainnya",
}

// writeICS writes an iCalendar file (RFC 5545) with one all-day event per item
// on its expiry date and two reminders: 30 and 7 days before, at 09.00.
func writeICS(w io.Writer, items []store.Item, now time.Time) {
	b := bufio.NewWriter(w)
	line := func(s string) { writeFolded(b, s) }

	line("BEGIN:VCALENDAR")
	line("VERSION:2.0")
	line("PRODID:-//Tenggat//Tenggat//ID")
	line("CALSCALE:GREGORIAN")
	line("METHOD:PUBLISH")
	line("X-WR-CALNAME:Tenggat")
	stamp := now.UTC().Format("20060102T150405Z")
	for _, it := range items {
		day := it.Kedaluwarsa
		desc := jenisLabel[it.Jenis]
		if it.Klien != "" {
			desc += "\nKlien: " + it.Klien
		}
		if it.Host != nil {
			desc += "\nHost: " + *it.Host
		}
		line("BEGIN:VEVENT")
		line("UID:" + it.ID + "@tenggat")
		line("DTSTAMP:" + stamp)
		line("DTSTART;VALUE=DATE:" + day.Format("20060102"))
		line("DTEND;VALUE=DATE:" + day.AddDate(0, 0, 1).Format("20060102"))
		line("SUMMARY:" + escapeText("Berakhir: "+it.Nama))
		line("DESCRIPTION:" + escapeText(desc))
		line("TRANSP:TRANSPARENT")
		for _, a := range []struct{ trigger, when string }{
			{"-P29DT15H", "30 hari lagi"}, // 09.00, thirty days before
			{"-P6DT15H", "7 hari lagi"},   // 09.00, seven days before
		} {
			line("BEGIN:VALARM")
			line("ACTION:DISPLAY")
			line("DESCRIPTION:" + escapeText(it.Nama+" berakhir "+a.when))
			line("TRIGGER:" + a.trigger)
			line("END:VALARM")
		}
		line("END:VEVENT")
	}
	line("END:VCALENDAR")
	_ = b.Flush()
}

// escapeText escapes the characters RFC 5545 reserves in TEXT values.
func escapeText(s string) string {
	return strings.NewReplacer(`\`, `\\`, ";", `\;`, ",", `\,`, "\r\n", `\n`, "\n", `\n`).Replace(s)
}

// writeFolded writes a content line ending in CRLF, folding it so no physical
// line is longer than 75 octets and no UTF-8 character is split.
func writeFolded(w *bufio.Writer, s string) {
	limit := 75
	for len(s) > limit {
		cut := limit
		for cut > 0 && !utf8.RuneStart(s[cut]) {
			cut--
		}
		_, _ = w.WriteString(s[:cut] + "\r\n ")
		s = s[cut:]
		limit = 74 // continuation lines start with a space
	}
	_, _ = w.WriteString(s + "\r\n")
}
