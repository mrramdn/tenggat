package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/mrramdn/tenggat/api/internal/expiry"
	"github.com/mrramdn/tenggat/api/internal/rdap"
	"github.com/mrramdn/tenggat/api/internal/store"
	"github.com/mrramdn/tenggat/api/internal/tlscheck"
	"github.com/mrramdn/tenggat/api/internal/uang"
)

// itemJSON is the API form of an item, with the days left and status worked out.
type itemJSON struct {
	ID            string         `json:"id"`
	Nama          string         `json:"nama"`
	Jenis         string         `json:"jenis"`
	Klien         string         `json:"klien"`
	KlienID       *string        `json:"klien_id"`
	Host          *string        `json:"host"`
	Kedaluwarsa   string         `json:"kedaluwarsa"`
	SisaHari      int            `json:"sisa_hari"`
	Status        expiry.Status  `json:"status"`
	Sumber        string         `json:"sumber"`
	Biaya         *json.Number   `json:"biaya"`
	MataUang      string         `json:"mata_uang"`
	UlangJumlah   *int           `json:"ulang_jumlah"`
	UlangSatuan   *string        `json:"ulang_satuan"`
	SelesaiPada   *time.Time     `json:"selesai_pada"`
	Catatan       string         `json:"catatan"`
	TLS           *store.TLSInfo `json:"tls"`
	TerakhirDicek *time.Time     `json:"terakhir_dicek"`
	GalatCek      *string        `json:"galat_cek"`
	Dibuat        time.Time      `json:"dibuat"`
	Diubah        time.Time      `json:"diubah"`

	// Registri is what the last registry lookup said; null unless sumber is "registri".
	Registri *store.RegistryInfo `json:"registri"`
}

func uangJSON(minor *int64, mu string) *json.Number {
	if minor == nil {
		return nil
	}
	if mu == "" {
		mu = "IDR"
	}
	n := uang.JSON(*minor, mu)
	return &n
}

func toJSON(it store.Item, now time.Time) itemJSON {
	days := expiry.DaysLeft(it.Kedaluwarsa, now)
	mu := it.MataUang
	if mu == "" {
		mu = "IDR"
	}
	var ulangJumlah *int
	var ulangSatuan *string
	if it.UlangJumlah > 0 && it.UlangSatuan != "" {
		n, u := it.UlangJumlah, it.UlangSatuan
		ulangJumlah, ulangSatuan = &n, &u
	}
	return itemJSON{
		ID: it.ID, Nama: it.Nama, Jenis: it.Jenis, Klien: it.Klien, KlienID: it.KlienID, Host: it.Host,
		Kedaluwarsa: it.Kedaluwarsa.Format(time.DateOnly), SisaHari: days, Status: expiry.For(days),
		Sumber: it.Sumber, Biaya: uangJSON(it.Biaya, mu), MataUang: mu,
		UlangJumlah: ulangJumlah, UlangSatuan: ulangSatuan, SelesaiPada: it.SelesaiPada,
		Catatan: it.Catatan, TLS: it.InfoTLS, Registri: it.InfoRegistri,
		TerakhirDicek: it.TerakhirDicek, GalatCek: it.GalatCek, Dibuat: it.Dibuat, Diubah: it.Diubah,
	}
}

// itemRequest is the body of POST /api/item and PUT /api/item/{id}.
type itemRequest struct {
	Nama        string       `json:"nama"`
	Jenis       string       `json:"jenis"`
	Klien       string       `json:"klien"`    // a client's name; ignored when klien_id is set
	KlienID     *string      `json:"klien_id"` // one of the user's clients
	Host        string       `json:"host"`
	Kedaluwarsa string       `json:"kedaluwarsa"`
	Biaya       *json.Number `json:"biaya"`
	MataUang    *string      `json:"mata_uang"`
	UlangJumlah *int         `json:"ulang_jumlah"`
	UlangSatuan *string      `json:"ulang_satuan"`
	Catatan     string       `json:"catatan"`
}

var jenisValid = map[string]bool{
	"sertifikat": true, "domain": true, "token": true, "langganan": true, "tagihan": true, "lainnya": true,
}

// prepare validates a request and, for a certificate, reads its date from the
// server. On PUT, old carries the stored item so omitted currency and
// recurrence fields keep their existing values.
func (s *Server) prepare(ctx context.Context, req itemRequest, old *store.Item) (store.Input, map[string]string) {
	fields := map[string]string{}
	in := store.Input{
		Nama:     strings.TrimSpace(req.Nama),
		Jenis:    strings.TrimSpace(req.Jenis),
		Klien:    strings.TrimSpace(req.Klien),
		Catatan:  strings.TrimSpace(req.Catatan),
		MataUang: "IDR",
		Sumber:   "manual",
	}
	if old != nil && old.MataUang != "" {
		in.MataUang = old.MataUang
	}
	if req.MataUang != nil && strings.TrimSpace(*req.MataUang) != "" {
		in.MataUang = strings.TrimSpace(*req.MataUang)
	}

	if !jenisValid[in.Jenis] {
		fields["jenis"] = "Pilih jenis: sertifikat, domain, token, langganan, tagihan, atau lainnya."
	}
	if utf8.RuneCountInString(in.Klien) > 80 {
		fields["klien"] = "Nama klien maksimal 80 karakter."
	}
	if utf8.RuneCountInString(in.Catatan) > 500 {
		fields["catatan"] = "Catatan maksimal 500 karakter."
	}
	parseMu := in.MataUang
	if !uang.Valid(in.MataUang) {
		fields["mata_uang"] = "Pilih IDR atau USD."
		parseMu = "IDR"
	}
	if req.Biaya != nil {
		minor, err := uang.Parse(*req.Biaya, parseMu)
		if err != nil {
			fields["biaya"] = "Biaya harus angka 0 atau lebih; rupiah tanpa desimal, USD paling banyak 2 desimal."
		} else {
			in.Biaya = &minor
		}
	}

	var host string
	rawHost := req.Host
	if in.Jenis == "domain" {
		rawHost = rdap.ToASCII(rawHost) // a domain may be typed in Unicode
	}
	if in.Jenis == "tagihan" {
		if strings.TrimSpace(rawHost) != "" {
			fields["host"] = "Tagihan tidak memakai host."
		}
	} else if strings.TrimSpace(rawHost) != "" || in.Jenis == "sertifikat" {
		h, err := tlscheck.NormalizeHost(rawHost)
		if err != nil {
			fields["host"] = sentence(err.Error())
		} else {
			host = h
			in.Host = &host
		}
	}

	// A domain with a host and no date is read from the registry.
	fromRegistry := in.Jenis == "domain" && in.Host != nil && strings.TrimSpace(req.Kedaluwarsa) == ""
	if in.Jenis == "sertifikat" {
		if in.Nama == "" {
			in.Nama = host
		}
	} else if fromRegistry {
		registryDefaults(&in, fields)
	} else {
		d, err := time.Parse(time.DateOnly, strings.TrimSpace(req.Kedaluwarsa))
		switch {
		case err != nil:
			fields["kedaluwarsa"] = "Isi tanggal berakhir dengan format TTTT-BB-HH."
		case d.Year() < 2000 || d.Year() > 2100:
			fields["kedaluwarsa"] = "Tanggal berakhir di luar rentang yang masuk akal."
		default:
			in.Kedaluwarsa = d
		}
	}

	autoDate := in.Jenis == "sertifikat" || fromRegistry
	switch {
	case req.UlangJumlah == nil && req.UlangSatuan == nil:
		if old != nil && !autoDate {
			in.UlangJumlah, in.UlangSatuan = old.UlangJumlah, old.UlangSatuan
		}
	case req.UlangSatuan != nil && strings.TrimSpace(*req.UlangSatuan) == "" && (req.UlangJumlah == nil || *req.UlangJumlah == 0):
		// An empty ulang_satuan clears recurrence.
	case autoDate:
		fields["ulang_satuan"] = "Tanggal sertifikat dan domain dari registri dibaca otomatis, jadi tidak diulang."
	default:
		if req.UlangJumlah == nil || *req.UlangJumlah < 1 || *req.UlangJumlah > 365 {
			fields["ulang_jumlah"] = "Isi angka 1 sampai 365."
		} else {
			in.UlangJumlah = *req.UlangJumlah
		}
		satuan := ""
		if req.UlangSatuan != nil {
			satuan = strings.TrimSpace(*req.UlangSatuan)
		}
		if !expiry.Units[satuan] {
			fields["ulang_satuan"] = "Pilih satuan: hari, minggu, bulan, atau tahun."
		} else {
			in.UlangSatuan = satuan
		}
	}

	// A certificate without a valid host has no default name yet; the host
	// message already explains what to fix.
	if n := utf8.RuneCountInString(in.Nama); (n < 2 || n > 120) && !(in.Jenis == "sertifikat" && n == 0) {
		fields["nama"] = "Nama minimal 2 dan maksimal 120 karakter."
	}
	if len(fields) > 0 {
		return in, fields
	}

	// Only now, with every other field valid, contact the server.
	if fromRegistry {
		return s.fromRegistry(ctx, in)
	}
	if in.Jenis == "sertifikat" {
		res, err := s.checkHost(ctx, host, false)
		if err != nil {
			return in, map[string]string{"host": checkMessage(err)}
		}
		now := s.Now()
		info := tlsInfo(res)
		in.Kedaluwarsa = expiry.DateOf(res.NotAfter)
		in.Sumber = "server"
		in.InfoTLS = &info
		in.Dicek = &now
	}
	return in, nil
}

func tlsInfo(res *tlscheck.Result) store.TLSInfo {
	return store.TLSInfo{
		Penerbit: res.Issuer, Subjek: res.Subject, Alamat: res.Address,
		BerlakuSejak: res.NotBefore, Berakhir: res.NotAfter,
		RantaiValid: res.ChainValid, GalatRantai: res.ChainError,
	}
}

// checkMessage turns a lookup error into a sentence for the user. Handshake
// errors carry Go's own detail, which is dropped here.
func checkMessage(err error) string {
	for _, known := range []error{tlscheck.ErrInvalidHost, tlscheck.ErrBlockedHost, tlscheck.ErrNotFound,
		tlscheck.ErrUnreachable, tlscheck.ErrHandshake, tlscheck.ErrNoCertificate} {
		if errors.Is(err, known) {
			return sentence(known.Error())
		}
	}
	return "Sertifikat tidak dapat dibaca."
}

// sentence capitalises the first letter and ends with a full stop.
func sentence(s string) string {
	if s == "" {
		return s
	}
	r, size := utf8.DecodeRuneInString(s)
	s = string(unicode.ToUpper(r)) + s[size:]
	if !strings.HasSuffix(s, ".") {
		s += "."
	}
	return s
}

type apiError struct {
	Galat string            `json:"galat"`
	Field map[string]string `json:"field,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string, fields map[string]string) {
	writeJSON(w, status, apiError{Galat: message, Field: fields})
}

func writeNotFound(w http.ResponseWriter) {
	writeError(w, http.StatusNotFound, "Aset tidak ditemukan.", nil)
}

// decodeJSON reads a single JSON object of at most 16 KB with no unknown
// fields. It writes a 400 response and returns false when the body is unusable.
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	dec.DisallowUnknownFields()
	err := dec.Decode(v)
	if err == nil && dec.Decode(&struct{}{}) != io.EOF {
		err = errors.New("lebih dari satu objek JSON")
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, "Isi permintaan harus berupa JSON yang valid.", nil)
		return false
	}
	return true
}

// statusRecorder remembers the status code a handler wrote.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// logRequests writes one JSON log line per request. Successful health checks,
// which Docker sends every few seconds, are not logged.
func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		if r.URL.Path == "/health" && rec.status == http.StatusOK {
			return
		}
		s.Log.Info("permintaan", "method", r.Method, "path", redactPath(r.URL.Path), "status", rec.status,
			"durasi_ms", time.Since(start).Milliseconds())
	})
}

func (s *Server) recoverPanics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				s.Log.Error("panic", "path", redactPath(r.URL.Path), "nilai", v)
				writeError(w, http.StatusInternalServerError, "Terjadi kesalahan di server. Coba beberapa saat lagi.", nil)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
