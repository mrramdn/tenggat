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
	kursTunggu     = 300 * time.Millisecond // how long a request gives a fetch in flight
	kursAmbilMaks  = 20 * time.Second       // the fetch itself, apart from any request
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

// kursKini returns the current daily exchange rates, or false when there are
// none yet. A request never waits for the rate source: the fetch runs on its own
// in the background, the request gives it a moment (enough for a fast answer)
// and otherwise goes on without estimates; the next request finds the result.
// Rates are kept in memory and in Cache under one global key for 12 hours, and a
// failed fetch is not repeated for 10 minutes.
func (s *Server) kursKini(ctx context.Context) (kurs.Kurs, bool) {
	if s.Kurs == nil {
		return kurs.Kurs{}, false
	}
	if k, ok := s.kursMemori(); ok {
		return k, true
	}
	if k, ok := s.kursFromCache(ctx); ok {
		s.kursMu.Lock()
		s.kursMem, s.kursMemSampai = k, s.Now().Add(time.Hour)
		s.kursMu.Unlock()
		return k, true
	}

	s.kursMu.Lock()
	if now := s.Now(); !s.kursRetryUntil.IsZero() && now.Before(s.kursRetryUntil) {
		s.kursMu.Unlock()
		return kurs.Kurs{}, false
	}
	if s.kursJalan == nil {
		s.kursJalan = make(chan struct{})
		go s.ambilKurs(s.kursJalan)
	}
	selesai := s.kursJalan
	s.kursMu.Unlock()

	tunggu := time.NewTimer(kursTunggu)
	defer tunggu.Stop()
	select {
	case <-selesai:
	case <-tunggu.C:
	case <-ctx.Done():
	}
	return s.kursMemori()
}

// kursMemori returns the rates held in memory while they are still fresh.
func (s *Server) kursMemori() (kurs.Kurs, bool) {
	s.kursMu.Lock()
	defer s.kursMu.Unlock()
	if s.kursMem.Tanggal == "" || !s.Now().Before(s.kursMemSampai) {
		return kurs.Kurs{}, false
	}
	return s.kursMem, true
}

// ambilKurs fetches the rates once, apart from any request, and closes selesai.
func (s *Server) ambilKurs(selesai chan struct{}) {
	ctx, cancel := context.WithTimeout(context.Background(), kursAmbilMaks)
	defer cancel()
	k, err := s.Kurs.Ambil(ctx)

	s.kursMu.Lock()
	defer s.kursMu.Unlock()
	if err != nil {
		if s.Log != nil {
			s.Log.Warn("gagal mengambil kurs", "galat", err)
		}
		s.kursRetryUntil = s.Now().Add(kursRetryAfter)
	} else {
		s.kursRetryUntil = time.Time{}
		s.kursMem, s.kursMemSampai = k, s.Now().Add(kursCacheTTL)
		if s.Cache != nil {
			if body, err := json.Marshal(k); err == nil {
				s.Cache.Set(ctx, kursCacheKey, body, kursCacheTTL)
			}
		}
	}
	s.kursJalan = nil
	close(selesai)
}

// PanaskanKurs starts fetching the rates at start-up, so the first visitor
// already gets estimates.
func (s *Server) PanaskanKurs() {
	if s.Kurs == nil {
		return
	}
	go s.kursKini(context.Background())
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
