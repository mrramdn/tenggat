package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/mrramdn/tenggat/api/internal/expiry"
	"github.com/mrramdn/tenggat/api/internal/rdap"
	"github.com/mrramdn/tenggat/api/internal/store"
)

// Registry reads a domain's expiry date from its registry over RDAP.
type Registry interface {
	Lookup(ctx context.Context, host string) (*rdap.Result, error)
}

// RegistryStore is the part of Store that keeps registry lookups.
type RegistryStore interface {
	SaveRegistryCheck(ctx context.Context, id string, date time.Time, info store.RegistryInfo, at time.Time) (store.Item, error)
}

const (
	registryTTL     = 12 * time.Hour // cached registry answer per domain
	registryMissTTL = time.Hour      // cached "not found" or "no expiration date"
)

// cachedLookup is a registry answer as kept in Redis: a result, or the kind of
// negative answer.
type cachedLookup struct {
	Hasil *rdap.Result `json:"hasil,omitempty"`
	Galat string       `json:"galat,omitempty"`
}

var negativeAnswers = map[string]error{"tidak_ditemukan": rdap.ErrNotFound, "tanpa_tanggal": rdap.ErrNoExpiration}

// lookupDomain asks the registry about a registrable domain, from Redis when
// the same domain was looked up less than 12 hours ago (an hour for a negative
// answer), unless fresh is set. Network errors are never kept.
func (s *Server) lookupDomain(ctx context.Context, domain string, fresh bool) (*rdap.Result, error) {
	key := "rdap:" + domain
	if s.Cache != nil && !fresh {
		if body, ok := s.Cache.Get(ctx, key); ok {
			var c cachedLookup
			if json.Unmarshal(body, &c) == nil {
				if c.Hasil != nil {
					return c.Hasil, nil
				}
				if err, ok := negativeAnswers[c.Galat]; ok {
					return nil, err
				}
			}
		}
	}
	if s.Registry == nil {
		return nil, rdap.ErrUnreachable
	}
	res, err := s.Registry.Lookup(ctx, domain)
	if s.Cache == nil {
		return res, err
	}
	switch {
	case err == nil:
		if body, err := json.Marshal(cachedLookup{Hasil: res}); err == nil {
			s.Cache.Set(ctx, key, body, registryTTL)
		}
	case errors.Is(err, rdap.ErrNotFound):
		s.Cache.Set(ctx, key, []byte(`{"galat":"tidak_ditemukan"}`), registryMissTTL)
	case errors.Is(err, rdap.ErrNoExpiration):
		s.Cache.Set(ctx, key, []byte(`{"galat":"tanpa_tanggal"}`), registryMissTTL)
	}
	return res, err
}

// registryMessage turns a lookup error into a sentence that says what to do.
func registryMessage(err error, domain string) string {
	var noRDAP *rdap.NoRDAPError
	switch {
	case errors.As(err, &noRDAP):
		return "Registri ." + noRDAP.TLD + " belum menyediakan RDAP. Isi tanggalnya sendiri."
	case errors.Is(err, rdap.ErrInvalidDomain):
		return "Nama domain tidak valid. Tulis nama domain, misalnya tokokuemawar.id."
	case errors.Is(err, rdap.ErrNotFound):
		return "Domain " + domain + " tidak ditemukan di registri. Periksa ejaannya, atau isi tanggalnya sendiri."
	case errors.Is(err, rdap.ErrNoExpiration):
		return "Registri tidak mencantumkan tanggal berakhir " + domain + ". Isi tanggalnya sendiri."
	case errors.Is(err, rdap.ErrUnreachable):
		return "Registri ." + rdap.TLD(domain) + " tidak dapat dihubungi saat ini. Coba lagi sebentar lagi, atau isi tanggalnya sendiri."
	default:
		return "Jawaban registri tidak dapat dibaca. Isi tanggalnya sendiri."
	}
}

// domainPreview is what "Ambil dari registri" shows before an item is saved.
type domainPreview struct {
	Host        string            `json:"host"`
	Domain      string            `json:"domain"`
	Kedaluwarsa string            `json:"kedaluwarsa"`
	SisaHari    int               `json:"sisa_hari"`
	Status      expiry.Status     `json:"status"`
	Registri    map[string]string `json:"registri"`
}

// previewDomain asks the registry about a domain without storing anything.
func (s *Server) previewDomain(w http.ResponseWriter, r *http.Request, _ store.Pengguna) {
	host, domain, err := rdap.Normalize(r.URL.Query().Get("host"))
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "Periksa kembali isian Anda.",
			map[string]string{"host": registryMessage(err, "")})
		return
	}
	res, err := s.lookupDomain(r.Context(), domain, false)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "Tanggal domain tidak dapat diambil dari registri.",
			map[string]string{"host": registryMessage(err, domain)})
		return
	}
	days := expiry.DaysLeft(res.Date, s.Now())
	writeJSON(w, http.StatusOK, domainPreview{
		Host:        host,
		Domain:      domain,
		Kedaluwarsa: res.Date.Format(time.DateOnly),
		SisaHari:    days,
		Status:      expiry.For(days),
		Registri:    map[string]string{"registrar": res.Registrar, "server": res.Server},
	})
}

// registryDefaults runs during validation of a domain that will be read from
// the registry: it names the item after the domain when no name was given.
func registryDefaults(in *store.Input, fields map[string]string) {
	_, domain, err := rdap.Normalize(*in.Host)
	if err != nil {
		fields["host"] = registryMessage(err, "")
		return
	}
	if in.Nama == "" {
		in.Nama = domain
	}
}

// fromRegistry fills the date of a domain item from the registry.
func (s *Server) fromRegistry(ctx context.Context, in store.Input) (store.Input, map[string]string) {
	_, domain, err := rdap.Normalize(*in.Host)
	if err != nil {
		return in, map[string]string{"host": registryMessage(err, "")}
	}
	res, err := s.lookupDomain(ctx, domain, false)
	if err != nil {
		return in, map[string]string{"host": registryMessage(err, domain)}
	}
	now := s.Now()
	in.Kedaluwarsa = res.Date
	in.Sumber = "registri"
	in.Registri = &store.RegistryInfo{Domain: res.Domain, Registrar: res.Registrar, Server: res.Server}
	in.Dicek = &now
	return in, nil
}

// recheckDomain asks the registry again and stores either the new date or the
// error, keeping the last known date.
func (s *Server) recheckDomain(ctx context.Context, it store.Item) (store.Item, error) {
	now := s.Now()
	defer s.bump(ctx, it.PenggunaID)
	_, domain, err := rdap.Normalize(*it.Host)
	if err != nil {
		return s.Store.SaveCheckError(ctx, it.ID, registryMessage(err, ""), now)
	}
	res, err := s.lookupDomain(ctx, domain, true)
	if err != nil {
		return s.Store.SaveCheckError(ctx, it.ID, registryMessage(err, domain), now)
	}
	return s.Store.SaveRegistryCheck(ctx, it.ID, res.Date,
		store.RegistryInfo{Domain: res.Domain, Registrar: res.Registrar, Server: res.Server}, now)
}
