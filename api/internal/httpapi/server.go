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
	List(ctx context.Context, f store.Filter, now time.Time) ([]store.Item, error)
	Get(ctx context.Context, id string) (store.Item, error)
	Create(ctx context.Context, in store.Input) (store.Item, error)
	Update(ctx context.Context, id string, in store.Input) (store.Item, error)
	Delete(ctx context.Context, id string) error
	SaveCheck(ctx context.Context, id string, date time.Time, info store.TLSInfo, at time.Time) (store.Item, error)
	SaveCheckError(ctx context.Context, id, message string, at time.Time) (store.Item, error)
	DueForCheck(ctx context.Context, before time.Time) ([]store.Item, error)
	Summary(ctx context.Context, now time.Time) (store.Summary, error)
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
	Version(ctx context.Context) int64
	Bump(ctx context.Context)
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
}

// Routes returns the API handler with request logging and panic recovery.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.health)
	mux.HandleFunc("GET /api/item", s.listItems)
	mux.HandleFunc("POST /api/item", s.limit(s.createItem))
	mux.HandleFunc("GET /api/item/{id}", s.getItem)
	mux.HandleFunc("PUT /api/item/{id}", s.limit(s.updateItem))
	mux.HandleFunc("DELETE /api/item/{id}", s.limit(s.deleteItem))
	mux.HandleFunc("POST /api/item/{id}/cek", s.limit(s.recheckItem))
	mux.HandleFunc("GET /api/sertifikat", s.limit(s.previewCertificate))
	mux.HandleFunc("GET /api/ringkasan", s.summary)
	mux.HandleFunc("GET /api/kalender.ics", s.calendar)
	mux.HandleFunc("GET /", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "Alamat tidak ditemukan.", nil)
	})
	return s.recoverPanics(s.logRequests(mux))
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

func (s *Server) listItems(w http.ResponseWriter, r *http.Request) {
	f := store.Filter{Status: expiry.Status(r.URL.Query().Get("status")), Klien: r.URL.Query().Get("klien")}
	if f.Status != "" && !f.Status.Valid() {
		writeError(w, http.StatusUnprocessableEntity, "Status tidak dikenal.",
			map[string]string{"status": "Gunakan lewat, kritis, segera, atau aman."})
		return
	}
	s.cachedRead(w, r, "daftar:"+string(f.Status)+":"+f.Klien, func(now time.Time) (any, error) {
		items, err := s.Store.List(r.Context(), f, now)
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

func (s *Server) getItem(w http.ResponseWriter, r *http.Request) {
	id, ok := itemID(r)
	if !ok {
		writeNotFound(w)
		return
	}
	it, err := s.Store.Get(r.Context(), id)
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

func (s *Server) createItem(w http.ResponseWriter, r *http.Request) {
	var req itemRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	in, fields := s.prepare(r.Context(), req)
	if fields != nil {
		writeError(w, http.StatusUnprocessableEntity, "Periksa kembali isian Anda.", fields)
		return
	}
	it, err := s.Store.Create(r.Context(), in)
	if err != nil {
		s.internal(w, r, err)
		return
	}
	s.bump(r.Context())
	w.Header().Set("Location", "/api/item/"+it.ID)
	writeJSON(w, http.StatusCreated, toJSON(it, s.Now()))
}

func (s *Server) updateItem(w http.ResponseWriter, r *http.Request) {
	id, ok := itemID(r)
	if !ok {
		writeNotFound(w)
		return
	}
	if _, err := s.Store.Get(r.Context(), id); errors.Is(err, store.ErrNotFound) {
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
	in, fields := s.prepare(r.Context(), req)
	if fields != nil {
		writeError(w, http.StatusUnprocessableEntity, "Periksa kembali isian Anda.", fields)
		return
	}
	it, err := s.Store.Update(r.Context(), id, in)
	if errors.Is(err, store.ErrNotFound) {
		writeNotFound(w)
		return
	}
	if err != nil {
		s.internal(w, r, err)
		return
	}
	s.bump(r.Context())
	writeJSON(w, http.StatusOK, toJSON(it, s.Now()))
}

func (s *Server) deleteItem(w http.ResponseWriter, r *http.Request) {
	id, ok := itemID(r)
	if !ok {
		writeNotFound(w)
		return
	}
	err := s.Store.Delete(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeNotFound(w)
		return
	}
	if err != nil {
		s.internal(w, r, err)
		return
	}
	s.bump(r.Context())
	w.WriteHeader(http.StatusNoContent)
}

// recheckItem reads the certificate again now. A server that cannot be reached
// is a result, not a failed request: the item comes back with galat_cek set and
// its last known date kept.
func (s *Server) recheckItem(w http.ResponseWriter, r *http.Request) {
	id, ok := itemID(r)
	if !ok {
		writeNotFound(w)
		return
	}
	it, err := s.Store.Get(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeNotFound(w)
		return
	}
	if err != nil {
		s.internal(w, r, err)
		return
	}
	if it.Jenis != "sertifikat" || it.Host == nil {
		writeError(w, http.StatusUnprocessableEntity, "Hanya sertifikat TLS yang dapat dicek ulang dari server.", nil)
		return
	}
	updated, err := s.recheck(r.Context(), it)
	if err != nil {
		s.internal(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toJSON(updated, s.Now()))
}

// recheck reads one certificate and stores either the new date or the error.
func (s *Server) recheck(ctx context.Context, it store.Item) (store.Item, error) {
	now := s.Now()
	defer s.bump(ctx)
	res, err := s.checkHost(ctx, *it.Host, true)
	if err != nil {
		return s.Store.SaveCheckError(ctx, it.ID, checkMessage(err), now)
	}
	return s.Store.SaveCheck(ctx, it.ID, expiry.DateOf(res.NotAfter), tlsInfo(res), now)
}

// RunRechecks re-reads certificates that were last checked more than one
// interval ago: once shortly after start, then on every tick, until ctx ends.
func (s *Server) RunRechecks(ctx context.Context, interval time.Duration) {
	timer := time.NewTimer(30 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
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

func (s *Server) summary(w http.ResponseWriter, r *http.Request) {
	s.cachedRead(w, r, "ringkasan", func(now time.Time) (any, error) {
		return s.Store.Summary(r.Context(), now)
	})
}

func (s *Server) calendar(w http.ResponseWriter, r *http.Request) {
	items, err := s.Store.List(r.Context(), store.Filter{}, s.Now())
	if err != nil {
		s.internal(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="tenggat.ics"`)
	writeICS(w, items, s.Now())
}

func (s *Server) internal(w http.ResponseWriter, r *http.Request, err error) {
	s.Log.Error("galat internal", "method", r.Method, "path", r.URL.Path, "galat", err)
	writeError(w, http.StatusInternalServerError, "Terjadi kesalahan di server. Coba lagi sebentar lagi.", nil)
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
