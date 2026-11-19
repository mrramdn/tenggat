package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/mrramdn/tenggat/api/internal/kurs"
	"github.com/mrramdn/tenggat/api/internal/store"
)

const (
	kursCacheKey   = "kurs:terkini"
	kursCacheTTL   = 12 * time.Hour
	kursRetryAfter = 10 * time.Minute
)

type kursJSON struct {
	Tanggal *string            `json:"tanggal"`
	PerIDR  map[string]float64 `json:"per_idr"`
	Sumber  string             `json:"sumber"`
}

func (s *Server) getKurs(w http.ResponseWriter, r *http.Request, _ store.Pengguna) {
	resp := kursJSON{Sumber: kurs.NamaSumber}
	if k, ok := s.kursKini(r.Context()); ok {
		t := k.Tanggal
		resp.Tanggal = &t
		resp.PerIDR = k.PerIDR
	}
	writeJSON(w, http.StatusOK, resp)
}

// kursKini returns the current daily exchange rates through Cache under one
// global key for 12 hours, fetching on a miss. Failures are never fatal to a
// request and back off in memory for 10 minutes so a dead upstream is not hit
// on every request.
func (s *Server) kursKini(ctx context.Context) (kurs.Kurs, bool) {
	if s.Kurs == nil {
		return kurs.Kurs{}, false
	}
	if k, ok := s.kursFromCache(ctx); ok {
		return k, true
	}

	s.kursMu.Lock()
	defer s.kursMu.Unlock()

	if k, ok := s.kursFromCache(ctx); ok {
		return k, true
	}
	now := s.Now()
	if !s.kursRetryUntil.IsZero() && now.Before(s.kursRetryUntil) {
		return kurs.Kurs{}, false
	}

	k, err := s.Kurs.Ambil(ctx)
	if err != nil {
		if s.Log != nil {
			s.Log.Warn("gagal mengambil kurs", "galat", err)
		}
		// Only back off on an upstream failure, not when the caller cancelled its own request.
		if ctx.Err() == nil {
			s.kursRetryUntil = s.Now().Add(kursRetryAfter)
		}
		return kurs.Kurs{}, false
	}

	s.kursRetryUntil = time.Time{}
	if s.Cache != nil {
		if body, err := json.Marshal(k); err == nil {
			s.Cache.Set(ctx, kursCacheKey, body, kursCacheTTL)
		}
	}
	return k, true
}

func (s *Server) kursFromCache(ctx context.Context) (kurs.Kurs, bool) {
	if s.Cache == nil {
		return kurs.Kurs{}, false
	}
	raw, ok := s.Cache.Get(ctx, kursCacheKey)
	if !ok {
		return kurs.Kurs{}, false
	}
	var k kurs.Kurs
	if json.Unmarshal(raw, &k) != nil || k.Tanggal == "" || len(k.PerIDR) == 0 {
		return kurs.Kurs{}, false
	}
	return k, true
}
