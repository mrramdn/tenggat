package httpapi

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/mrramdn/tenggat/api/internal/auth"
	"github.com/mrramdn/tenggat/api/internal/store"
)

// CalendarStore is the part of Store that keeps calendar subscription links.
type CalendarStore interface {
	SetCalendarToken(ctx context.Context, userID, tokenHash string) error
	DeleteCalendarToken(ctx context.Context, userID string) error
	UserByCalendarToken(ctx context.Context, tokenHash string) (store.Pengguna, error)
	CalendarActive(ctx context.Context, userID string) (bool, error)
}

// calendarPerMinute caps feed requests per client. Calendar apps poll every
// few hours at most, so this only stops someone guessing tokens.
const calendarPerMinute = 60

const calendarPrefix = "/api/kalender/"

// calendarToken matches what auth.NewToken makes: 32 random bytes in
// unpadded base64url.
var calendarToken = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)

// createCalendarLink makes a new subscription token and drops the old one, so
// an old link stops working. The token is shown only in this response.
func (s *Server) createCalendarLink(w http.ResponseWriter, r *http.Request, u store.Pengguna) {
	token, hash, err := auth.NewToken()
	if err != nil {
		s.internal(w, r, err)
		return
	}
	if err := s.Store.SetCalendarToken(r.Context(), u.ID, hash); err != nil {
		s.internal(w, r, err)
		return
	}
	s.bump(r.Context(), u.ID)
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusCreated, map[string]string{"jalur": calendarPrefix + token + ".ics"})
}

// deleteCalendarLink turns the subscription link off.
func (s *Server) deleteCalendarLink(w http.ResponseWriter, r *http.Request, u store.Pengguna) {
	if err := s.Store.DeleteCalendarToken(r.Context(), u.ID); err != nil {
		s.internal(w, r, err)
		return
	}
	s.bump(r.Context(), u.ID)
	w.WriteHeader(http.StatusNoContent)
}

// subscribedCalendar serves the same calendar as /api/kalender.ics to a
// calendar app, which has no session: the token in the path is the key.
func (s *Server) subscribedCalendar(w http.ResponseWriter, r *http.Request) {
	token, ok := strings.CutSuffix(r.PathValue("berkas"), ".ics")
	if !ok || !calendarToken.MatchString(token) {
		writeError(w, http.StatusNotFound, "Tautan kalender tidak ditemukan atau sudah dicabut.", nil)
		return
	}
	u, err := s.Store.UserByCalendarToken(r.Context(), auth.HashToken(token))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "Tautan kalender tidak ditemukan atau sudah dicabut.", nil)
		return
	}
	if err != nil {
		s.internal(w, r, err)
		return
	}
	items, err := s.Store.List(r.Context(), u.ID, store.Filter{}, s.Now())
	if err != nil {
		s.internal(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", `inline; filename="tenggat.ics"`)
	w.Header().Set("Cache-Control", "private, max-age=300")
	w.Header().Set("X-Robots-Tag", "noindex")
	w.Header().Set("Referrer-Policy", "no-referrer")
	writeICS(w, items, s.Now())
}

// me answers GET /api/saya: the account and whether it has a calendar link.
func (s *Server) me(w http.ResponseWriter, r *http.Request, u store.Pengguna) {
	active, err := s.Store.CalendarActive(r.Context(), u.ID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		s.internal(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		accountJSON
		KalenderAktif bool `json:"kalender_aktif"`
	}{accountJSON{Pengguna: u}, active})
}

// redactPath hides a calendar token before a path is written to the log: the
// token works like a password for the feed.
func redactPath(p string) string {
	if rest, ok := strings.CutPrefix(p, calendarPrefix); ok && strings.HasSuffix(rest, ".ics") {
		return calendarPrefix + "…" + ".ics"
	}
	return p
}
