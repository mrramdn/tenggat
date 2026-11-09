// Package httpapi serves the JSON API under /api and the health check.
package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"time"

	"github.com/mrramdn/tenggat/api/internal/expiry"
	"github.com/mrramdn/tenggat/api/internal/store"
	"github.com/mrramdn/tenggat/api/internal/tlscheck"
)

// Store is the part of store.Store the handlers use.
type Store interface {
	Ping(ctx context.Context) error
	List(ctx context.Context, userID string, f store.Filter, now time.Time) ([]store.Item, error)
	Get(ctx context.Context, userID, id string) (store.Item, error)
	Create(ctx context.Context, userID string, in store.Input) (store.Item, error)
	Update(ctx context.Context, userID, id string, in store.Input) (store.Item, error)
	Delete(ctx context.Context, userID, id string) error
	SaveCheck(ctx context.Context, id string, date time.Time, info store.TLSInfo, at time.Time) (store.Item, error)
	SaveCheckError(ctx context.Context, id, message string, at time.Time) (store.Item, error)
	DueForCheck(ctx context.Context, before time.Time) ([]store.Item, error)
	Summary(ctx context.Context, userID string, now time.Time) (store.Summary, error)

	CreateUser(ctx context.Context, nama, email, hash string) (store.Pengguna, error)
	UserByEmail(ctx context.Context, email string) (store.Pengguna, error)
	CreateSession(ctx context.Context, tokenHash, userID string, expires time.Time) error
	UserBySession(ctx context.Context, tokenHash string, now time.Time) (store.Pengguna, error)
	DeleteSession(ctx context.Context, tokenHash string) error
	DeleteUser(ctx context.Context, userID string) error
	DeleteExpiredSessions(ctx context.Context, now time.Time) (int64, error)

	RegistryStore
	CalendarStore
	KlienStore
	TugasStore
	AktivitasStore
	LaporanStore
	NotifikasiStore
}

// Checker reads a host's certificate.
type Checker interface {
	Check(ctx context.Context, host string) (*tlscheck.Result, error)
}

// Cache is the part of cache.Cache the handlers use. A nil Cache turns caching
// and rate limiting off.
type Cache interface {
	Ping(ctx context.Context) error
	Get(ctx context.Context, key string) ([]byte, bool)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration)
	Version(ctx context.Context, scope string) int64
	Bump(ctx context.Context, scope string)
	Allow(ctx context.Context, bucket, client string, limit int, window time.Duration) (bool, time.Duration)
}

// Server holds what the handlers need.
type Server struct {
	Store   Store
	Checker Checker
	Cache   Cache
	Now     func() time.Time
	Log     *slog.Logger
	// LimitPerMinute caps writes and certificate lookups per client; 0 means 30.
	LimitPerMinute int
	// CookieSecure marks the session cookie Secure; set it when the site is served over HTTPS.
	CookieSecure bool

	// Registry reads domain expiry dates over RDAP; nil turns registry lookups off.
	Registry Registry

	// Webhook posts notifications; nil means a client with an 8 second timeout
	// that never follows redirects. Tests replace it.
	Webhook *http.Client
	// AppURL is the site's address without a trailing slash, for links in
	// notifications; empty leaves the links out.
	AppURL string
}

// Routes returns the API handler with request logging, panic recovery and the
// same-origin check for requests that change data.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.health)

	mux.HandleFunc("POST /api/akun", s.limitTo("daftar", 5, s.createAccount))
	mux.HandleFunc("POST /api/sesi", s.limitTo("masuk", 10, s.login))
	mux.HandleFunc("DELETE /api/sesi", s.logout)
	mux.HandleFunc("GET /api/saya", s.user(s.me))
	mux.HandleFunc("DELETE /api/saya", s.limit(s.user(s.deleteMe)))

	mux.HandleFunc("GET /api/item", s.user(s.listItems))
	mux.HandleFunc("POST /api/item", s.limit(s.user(s.createItem)))
	mux.HandleFunc("GET /api/item/{id}", s.user(s.getItem))
	mux.HandleFunc("PUT /api/item/{id}", s.limit(s.user(s.updateItem)))
	mux.HandleFunc("DELETE /api/item/{id}", s.limit(s.user(s.deleteItem)))
	mux.HandleFunc("POST /api/item/{id}/cek", s.limit(s.user(s.recheckItem)))
	mux.HandleFunc("GET /api/sertifikat", s.limit(s.user(s.previewCertificate)))
	mux.HandleFunc("GET /api/domain", s.limit(s.user(s.previewDomain)))
	mux.HandleFunc("GET /api/item.csv", s.user(s.exportCSV))
	mux.HandleFunc("GET /api/ringkasan", s.user(s.summary))
	mux.HandleFunc("GET /api/klien", s.user(s.listKlien))
	mux.HandleFunc("POST /api/klien", s.limit(s.user(s.createKlien)))
	mux.HandleFunc("PUT /api/klien/{id}", s.limit(s.user(s.updateKlien)))
	mux.HandleFunc("DELETE /api/klien/{id}", s.limit(s.user(s.deleteKlien)))
	mux.HandleFunc("GET /api/kalender.ics", s.user(s.calendar))
	mux.HandleFunc("POST /api/kalender/tautan", s.limit(s.user(s.createCalendarLink)))
	mux.HandleFunc("DELETE /api/kalender/tautan", s.limit(s.user(s.deleteCalendarLink)))
	mux.HandleFunc("GET /api/kalender/{berkas}", s.limitTo("kalender", calendarPerMinute, s.subscribedCalendar))
	mux.HandleFunc("GET /api/tugas", s.user(s.listTugas))
	mux.HandleFunc("POST /api/tugas", s.limit(s.user(s.createTugas)))
	mux.HandleFunc("PUT /api/tugas/{id}", s.limit(s.user(s.updateTugas)))
	mux.HandleFunc("DELETE /api/tugas/{id}", s.limit(s.user(s.deleteTugas)))
	mux.HandleFunc("POST /api/tugas/{id}/selesai", s.limit(s.user(s.completeTugas)))
	mux.HandleFunc("GET /api/aktivitas", s.user(s.listAktivitas))
	mux.HandleFunc("POST /api/aktivitas", s.limit(s.user(s.createAktivitas)))
	mux.HandleFunc("DELETE /api/aktivitas/{id}", s.limit(s.user(s.deleteAktivitas)))
	mux.HandleFunc("GET /api/klien/{id}/laporan", s.user(s.getLaporan))
	mux.HandleFunc("PUT /api/klien/{id}/laporan/catatan", s.limit(s.user(s.putCatatanLaporan)))
	mux.HandleFunc("POST /api/klien/{id}/laporan/tautan", s.limit(s.user(s.createReportLink)))
	mux.HandleFunc("DELETE /api/klien/{id}/laporan/tautan", s.limit(s.user(s.deleteReportLink)))
	mux.HandleFunc("GET /api/laporan/{token}", s.limitTo("laporan", reportPerMinute, s.publicReport))
	mux.HandleFunc("GET /api/publik/cek", s.limitTo("publik", publicCheckPerMinute, s.publicCheck))
	mux.HandleFunc("GET /api/notifikasi", s.user(s.getNotifikasi))
	mux.HandleFunc("PUT /api/notifikasi", s.limit(s.user(s.putNotifikasi)))
	mux.HandleFunc("DELETE /api/notifikasi", s.limit(s.user(s.deleteNotifikasi)))
	mux.HandleFunc("POST /api/notifikasi/uji", s.limitTo("webhook", webhookTestLimit, s.user(s.testNotifikasi)))
	mux.HandleFunc("GET /", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "Alamat tidak ditemukan.", nil)
	})
	return s.recoverPanics(s.logRequests(s.sameOrigin(mux)))
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	status := map[string]string{"status": "ok", "db": "ok"}
	if s.Cache != nil {
		// The API keeps working without Redis, so a missing cache is reported
		// but does not make the service unhealthy.
		status["cache"] = "ok"
		if err := s.Cache.Ping(ctx); err != nil {
			status["cache"] = "tidak terhubung"
		}
	}
	if err := s.Store.Ping(ctx); err != nil {
		status["status"], status["db"] = "gagal", "tidak terhubung"
		writeJSON(w, http.StatusServiceUnavailable, status)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) listItems(w http.ResponseWriter, r *http.Request, u store.Pengguna) {
	q := r.URL.Query()
	f := store.Filter{Status: expiry.Status(q.Get("status")), Klien: q.Get("klien"), KlienID: q.Get("klien_id")}
	if f.Status != "" && !f.Status.Valid() {
		writeError(w, http.StatusUnprocessableEntity, "Status tidak dikenal.",
			map[string]string{"status": "Gunakan lewat, kritis, segera, atau aman."})
		return
	}
	if f.KlienID != "" && !uuidPattern.MatchString(f.KlienID) {
		writeError(w, http.StatusUnprocessableEntity, "Klien tidak dikenal.", map[string]string{"klien_id": "Klien tidak ditemukan."})
		return
	}
	s.cachedRead(w, r, u.ID, "daftar:"+string(f.Status)+":"+f.KlienID+":"+f.Klien, func(now time.Time) (any, error) {
		items, err := s.Store.List(r.Context(), u.ID, f, now)
		if err != nil {
			return nil, err
		}
		out := make([]itemJSON, 0, len(items))
		for _, it := range items {
			out = append(out, toJSON(it, now))
		}
		return map[string]any{"item": out}, nil
	})
}

var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// itemID returns the id path value, or false when it cannot be an item id.
func itemID(r *http.Request) (string, bool) {
	id := r.PathValue("id")
	return id, uuidPattern.MatchString(id)
}

func (s *Server) getItem(w http.ResponseWriter, r *http.Request, u store.Pengguna) {
	id, ok := itemID(r)
	if !ok {
		writeNotFound(w)
		return
	}
	it, err := s.Store.Get(r.Context(), u.ID, id)
	if errors.Is(err, store.ErrNotFound) {
		writeNotFound(w)
		return
	}
	if err != nil {
		s.internal(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toJSON(it, s.Now()))
}

func (s *Server) createItem(w http.ResponseWriter, r *http.Request, u store.Pengguna) {
	var req itemRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	in, ok := s.prepareItem(w, r, u.ID, req)
	if !ok {
		return
	}
	it, err := s.Store.Create(r.Context(), u.ID, in)
	if err != nil {
		s.internal(w, r, err)
		return
	}
	s.catatAsetBaru(r.Context(), it)
	s.bump(r.Context(), u.ID)
	w.Header().Set("Location", "/api/item/"+it.ID)
	writeJSON(w, http.StatusCreated, toJSON(it, s.Now()))
}

func (s *Server) updateItem(w http.ResponseWriter, r *http.Request, u store.Pengguna) {
	id, ok := itemID(r)
	if !ok {
		writeNotFound(w)
		return
	}
	old, err := s.Store.Get(r.Context(), u.ID, id)
	if errors.Is(err, store.ErrNotFound) {
		writeNotFound(w)
		return
	} else if err != nil {
		s.internal(w, r, err)
		return
	}
	var req itemRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	in, ok := s.prepareItem(w, r, u.ID, req)
	if !ok {
		return
	}
	it, err := s.Store.Update(r.Context(), u.ID, id, in)
	if errors.Is(err, store.ErrNotFound) {
		writeNotFound(w)
		return
	}
	if err != nil {
		s.internal(w, r, err)
		return
	}
	s.catatAsetDiubah(r.Context(), old, it)
	s.bump(r.Context(), u.ID)
	writeJSON(w, http.StatusOK, toJSON(it, s.Now()))
}

func (s *Server) deleteItem(w http.ResponseWriter, r *http.Request, u store.Pengguna) {
	id, ok := itemID(r)
	if !ok {
		writeNotFound(w)
		return
	}
	it, err := s.Store.Get(r.Context(), u.ID, id)
	if err == nil {
		err = s.Store.Delete(r.Context(), u.ID, id)
	}
	if errors.Is(err, store.ErrNotFound) {
		writeNotFound(w)
		return
	}
	if err != nil {
		s.internal(w, r, err)
		return
	}
	s.catatAsetDihapus(r.Context(), it)
	s.bump(r.Context(), u.ID)
	w.WriteHeader(http.StatusNoContent)
}

// recheckItem reads the certificate again now. A server that cannot be reached
// is a result, not a failed request: the item comes back with galat_cek set and
// its last known date kept.
func (s *Server) recheckItem(w http.ResponseWriter, r *http.Request, u store.Pengguna) {
	id, ok := itemID(r)
	if !ok {
		writeNotFound(w)
		return
	}
	it, err := s.Store.Get(r.Context(), u.ID, id)
	if errors.Is(err, store.ErrNotFound) {
		writeNotFound(w)
		return
	}
	if err != nil {
		s.internal(w, r, err)
		return
	}
	if (it.Jenis != "sertifikat" && it.Sumber != "registri") || it.Host == nil {
		writeError(w, http.StatusUnprocessableEntity,
			"Hanya sertifikat TLS dan domain dari registri yang dapat dicek ulang.", nil)
		return
	}
	updated, err := s.recheck(r.Context(), it)
	if err != nil {
		s.internal(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toJSON(updated, s.Now()))
}

// recheck reads one certificate, or asks the registry about one domain, and
// stores either the new date or the error. A later date or a failure goes into
// the activity feed.
func (s *Server) recheck(ctx context.Context, it store.Item) (store.Item, error) {
	updated, err := s.recheckOnce(ctx, it)
	if err != nil {
		return updated, err
	}
	s.catatCek(ctx, it, updated)
	s.bump(ctx, it.PenggunaID)
	return updated, nil
}

func (s *Server) recheckOnce(ctx context.Context, it store.Item) (store.Item, error) {
	if it.Sumber == "registri" {
		return s.recheckDomain(ctx, it)
	}
	now := s.Now()
	defer s.bump(ctx, it.PenggunaID)
	res, err := s.checkHost(ctx, *it.Host, true)
	if err != nil {
		return s.Store.SaveCheckError(ctx, it.ID, checkMessage(err), now)
	}
	return s.Store.SaveCheck(ctx, it.ID, expiry.DateOf(res.NotAfter), tlsInfo(res), now)
}

// RunRechecks re-reads certificates that were last checked more than one
// interval ago, once shortly after start and then on every tick, and removes
// expired sessions, until ctx ends.
func (s *Server) RunRechecks(ctx context.Context, interval time.Duration) {
	timer := time.NewTimer(30 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		if n, err := s.Store.DeleteExpiredSessions(ctx, s.Now()); err != nil {
			s.Log.Error("gagal menghapus sesi kedaluwarsa", "galat", err)
		} else if n > 0 {
			s.Log.Info("sesi kedaluwarsa dihapus", "jumlah", n)
		}
		due, err := s.Store.DueForCheck(ctx, s.Now().Add(-interval))
		if err != nil {
			s.Log.Error("cek berkala gagal membaca daftar", "galat", err)
		}
		for _, it := range due {
			if ctx.Err() != nil {
				return
			}
			updated, err := s.recheck(ctx, it)
			if err != nil {
				s.Log.Error("cek berkala gagal menyimpan", "id", it.ID, "galat", err)
				continue
			}
			s.Log.Info("cek berkala", "host", *it.Host, "kedaluwarsa", updated.Kedaluwarsa.Format(time.DateOnly),
				"galat_cek", deref(updated.GalatCek))
			time.Sleep(200 * time.Millisecond) // be gentle with other people's servers
		}
		timer.Reset(interval / 6)
	}
}

func (s *Server) summary(w http.ResponseWriter, r *http.Request, u store.Pengguna) {
	s.cachedRead(w, r, u.ID, "ringkasan", func(now time.Time) (any, error) {
		return s.Store.Summary(r.Context(), u.ID, now)
	})
}

// calendar downloads the user's calendar, or one client's with ?klien_id=.
func (s *Server) calendar(w http.ResponseWriter, r *http.Request, u store.Pengguna) {
	f := store.Filter{KlienID: r.URL.Query().Get("klien_id")}
	if f.KlienID != "" && !uuidPattern.MatchString(f.KlienID) {
		writeError(w, http.StatusUnprocessableEntity, "Klien tidak dikenal.", map[string]string{"klien_id": "Klien tidak ditemukan."})
		return
	}
	items, err := s.Store.List(r.Context(), u.ID, f, s.Now())
	if err != nil {
		s.internal(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="tenggat.ics"`)
	writeICS(w, items, s.Now())
}

func (s *Server) internal(w http.ResponseWriter, r *http.Request, err error) {
	s.Log.Error("galat internal", "method", r.Method, "path", redactPath(r.URL.Path), "galat", err)
	writeError(w, http.StatusInternalServerError, "Terjadi kesalahan di server. Coba beberapa saat lagi.", nil)
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
