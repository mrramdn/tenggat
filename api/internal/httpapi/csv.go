package httpapi

import (
	"encoding/csv"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mrramdn/tenggat/api/internal/expiry"
	"github.com/mrramdn/tenggat/api/internal/store"
)

var csvHeader = []string{"nama", "jenis", "klien", "host", "kedaluwarsa", "sisa_hari", "status", "sumber", "biaya", "catatan"}

// exportCSV writes all of a user's items as a spreadsheet file. It starts with
// a UTF-8 byte order mark so Excel reads the file as UTF-8.
func (s *Server) exportCSV(w http.ResponseWriter, r *http.Request, u store.Pengguna) {
	now := s.Now()
	items, err := s.Store.List(r.Context(), u.ID, store.Filter{}, now)
	if err != nil {
		s.internal(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition",
		`attachment; filename="tenggat-`+expiry.DateOf(now).Format(time.DateOnly)+`.csv"`)
	w.Header().Set("Cache-Control", "private, no-store")
	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF}) // UTF-8 byte order mark
	cw := csv.NewWriter(w)
	cw.UseCRLF = true
	_ = cw.Write(csvHeader)
	for _, it := range items {
		days := expiry.DaysLeft(it.Kedaluwarsa, now)
		biaya := ""
		if it.Biaya != nil {
			biaya = strconv.FormatInt(*it.Biaya, 10)
		}
		_ = cw.Write([]string{
			csvText(it.Nama), it.Jenis, csvText(it.Klien), csvText(deref(it.Host)),
			it.Kedaluwarsa.Format(time.DateOnly), strconv.Itoa(days), string(expiry.For(days)), it.Sumber,
			biaya, csvText(it.Catatan),
		})
	}
	cw.Flush()
}

// csvText stops a spreadsheet from running user text as a formula (CSV
// injection): a cell that starts with = + - @, a tab or a carriage return gets
// a leading apostrophe. Numbers Tenggat writes itself, such as a negative
// sisa_hari, are not passed through here.
func csvText(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}
