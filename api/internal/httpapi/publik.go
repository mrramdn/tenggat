package httpapi

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/mrramdn/tenggat/api/internal/expiry"
	"github.com/mrramdn/tenggat/api/internal/rdap"
	"github.com/mrramdn/tenggat/api/internal/tlscheck"
)

// publicCheckPerMinute caps the landing page demo per client. Each request may
// open a TLS connection and ask a registry, so it is kept low.
const publicCheckPerMinute = 10

type publikDomain struct {
	Domain      string        `json:"domain"`
	Kedaluwarsa string        `json:"kedaluwarsa"`
	SisaHari    int           `json:"sisa_hari"`
	Status      expiry.Status `json:"status"`
	Registrar   string        `json:"registrar"`
}

type publikSertifikat struct {
	Kedaluwarsa string        `json:"kedaluwarsa"`
	SisaHari    int           `json:"sisa_hari"`
	Status      expiry.Status `json:"status"`
	Penerbit    string        `json:"penerbit"`
}

type publikCek struct {
	Host            string            `json:"host"`
	Domain          *publikDomain     `json:"domain"`
	DomainGalat     *string           `json:"domain_galat"`
	Sertifikat      *publikSertifikat `json:"sertifikat"`
	SertifikatGalat *string           `json:"sertifikat_galat"`
}

// publicCheck is the landing page demo: a host's domain expiry date from its
// registry and its certificate's end date, without an account. It goes through
// the same caches and network guards as the signed-in lookups. One part failing
// is a result, not an error.
func (s *Server) publicCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	host, err := tlscheck.NormalizeHost(rdap.ToASCII(r.URL.Query().Get("host")))
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "Periksa kembali isian Anda.",
			map[string]string{"host": sentence(err.Error())})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	now := s.Now()
	out := publikCek{Host: host}
	var wg sync.WaitGroup
	wg.Go(func() {
		_, domain, err := rdap.Normalize(host)
		if err != nil {
			msg := withoutManualHint(registryMessage(err, ""))
			out.DomainGalat = &msg
			return
		}
		res, err := s.lookupDomain(ctx, domain, false)
		if err != nil {
			msg := withoutManualHint(registryMessage(err, domain))
			out.DomainGalat = &msg
			return
		}
		days := expiry.DaysLeft(res.Date, now)
		out.Domain = &publikDomain{Domain: domain, Kedaluwarsa: res.Date.Format(time.DateOnly), SisaHari: days,
			Status: expiry.For(days), Registrar: res.Registrar}
	})
	wg.Go(func() {
		res, err := s.checkHost(ctx, host, false)
		if err != nil {
			msg := checkMessage(err)
			out.SertifikatGalat = &msg
			return
		}
		date := expiry.DateOf(res.NotAfter)
		days := expiry.DaysLeft(date, now)
		out.Sertifikat = &publikSertifikat{Kedaluwarsa: date.Format(time.DateOnly), SisaHari: days,
			Status: expiry.For(days), Penerbit: res.Issuer}
	})
	wg.Wait()
	writeJSON(w, http.StatusOK, out)
}

// withoutManualHint drops the advice to type the date by hand from a registry
// message: a visitor of the landing page has no form to type it in.
func withoutManualHint(msg string) string {
	msg = strings.Replace(msg, ", atau isi tanggalnya sendiri.", ".", 1)
	msg = strings.Replace(msg, " Isi tanggalnya sendiri.", "", 1)
	return msg
}
