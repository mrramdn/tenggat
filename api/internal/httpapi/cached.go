package httpapi

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mrramdn/tenggat/api/internal/expiry"
	"github.com/mrramdn/tenggat/api/internal/store"
	"github.com/mrramdn/tenggat/api/internal/tlscheck"
)

const (
	readTTL = 60 * time.Second // cached list and summary
	certTTL = time.Hour        // cached certificate lookup per host
)

// cachedRead answers a GET from Redis when it can. The key holds the user, that
// user's cache version (bumped by every write of theirs) and today's date in
// Jakarta (statuses change at midnight), so one user never reads another's
// cached answer and a cached answer is never stale for longer than readTTL.
// The X-Cache header tells which path answered.
func (s *Server) cachedRead(w http.ResponseWriter, r *http.Request, userID, name string, load func(now time.Time) (any, error)) {
	now := s.Now()
	var key string
	if s.Cache != nil {
		key = "baca:" + userID + ":" + strconv.FormatInt(s.Cache.Version(r.Context(), userID), 10) + ":" +
			expiry.DateOf(now).Format(time.DateOnly) + ":" + name
		if body, ok := s.Cache.Get(r.Context(), key); ok {
			w.Header().Set("X-Cache", "HIT")
			writeRaw(w, http.StatusOK, body)
			return
		}
	}
	v, err := load(now)
	if err != nil {
		s.internal(w, r, err)
		return
	}
	body, err := json.Marshal(v)
	if err != nil {
		s.internal(w, r, err)
		return
	}
	if s.Cache != nil {
		s.Cache.Set(r.Context(), key, body, readTTL)
		w.Header().Set("X-Cache", "MISS")
	}
	writeRaw(w, http.StatusOK, body)
}

func writeRaw(w http.ResponseWriter, status int, body []byte) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(append(body, '\n'))
}

// bump invalidates a user's cached reads after a write.
func (s *Server) bump(ctx context.Context, userID string) {
	if s.Cache != nil {
		s.Cache.Bump(ctx, userID)
	}
}

// checkHost reads a certificate, from Redis when a lookup of the same host is
// less than an hour old, unless fresh is set. Only successful lookups are kept.
func (s *Server) checkHost(ctx context.Context, host string, fresh bool) (*tlscheck.Result, error) {
	key := "tls:" + host
	if s.Cache != nil && !fresh {
		if body, ok := s.Cache.Get(ctx, key); ok {
			var res tlscheck.Result
			if json.Unmarshal(body, &res) == nil {
				return &res, nil
			}
		}
	}
	res, err := s.Checker.Check(ctx, host)
	if err != nil {
		return nil, err
	}
	if s.Cache != nil {
		if body, err := json.Marshal(res); err == nil {
			s.Cache.Set(ctx, key, body, certTTL)
		}
	}
	return res, nil
}

// certificatePreview is what "Ambil dari server" shows before an item is saved.
type certificatePreview struct {
	Host        string         `json:"host"`
	Kedaluwarsa string         `json:"kedaluwarsa"`
	SisaHari    int            `json:"sisa_hari"`
	Status      expiry.Status  `json:"status"`
	TLS         map[string]any `json:"tls"`
}

// previewCertificate reads a host's certificate without storing anything.
func (s *Server) previewCertificate(w http.ResponseWriter, r *http.Request, _ store.Pengguna) {
	host, err := tlscheck.NormalizeHost(r.URL.Query().Get("host"))
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "Periksa kembali isian Anda.",
			map[string]string{"host": sentence(err.Error())})
		return
	}
	res, err := s.checkHost(r.Context(), host, false)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "Sertifikat tidak dapat dibaca.",
			map[string]string{"host": checkMessage(err)})
		return
	}
	date := expiry.DateOf(res.NotAfter)
	days := expiry.DaysLeft(date, s.Now())
	writeJSON(w, http.StatusOK, certificatePreview{
		Host:        host,
		Kedaluwarsa: date.Format(time.DateOnly),
		SisaHari:    days,
		Status:      expiry.For(days),
		TLS: map[string]any{
			"penerbit":      res.Issuer,
			"subjek":        res.Subject,
			"berlaku_sejak": res.NotBefore,
			"berakhir":      res.NotAfter,
			"rantai_valid":  res.ChainValid,
			"galat_rantai":  res.ChainError,
		},
	})
}

// limit allows each client LimitPerMinute requests per minute (30 by default)
// on endpoints that write or open connections to other servers.
func (s *Server) limit(next http.HandlerFunc) http.HandlerFunc {
	perMinute := s.LimitPerMinute
	if perMinute <= 0 {
		perMinute = 30
	}
	return s.limitTo("tulis", perMinute, next)
}

// limitTo counts requests per client in its own bucket; over the limit it
// answers 429 with Retry-After.
func (s *Server) limitTo(bucket string, perMinute int, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.Cache != nil {
			ok, retry := s.Cache.Allow(r.Context(), bucket, clientIP(r), perMinute, time.Minute)
			if !ok {
				w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
				writeError(w, http.StatusTooManyRequests,
					"Terlalu banyak permintaan dalam satu menit. Coba lagi sebentar lagi.", nil)
				return
			}
		}
		next(w, r)
	}
}

// clientIP takes the last address in X-Forwarded-For, which is the one our own
// nginx appended; the API port is not published, so only nginx can reach it.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[len(parts)-1])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
