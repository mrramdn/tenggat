package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mrramdn/tenggat/api/internal/expiry"
	"github.com/mrramdn/tenggat/api/internal/store"
)

// AktivitasStore is the part of Store that keeps the activity feed.
type AktivitasStore interface {
	ListAktivitas(ctx context.Context, userID string, f store.AktivitasFilter) ([]store.Aktivitas, error)
	AddAktivitas(ctx context.Context, userID string, in store.AktivitasInput) (store.Aktivitas, error)
	DeleteAktivitas(ctx context.Context, userID, id string) error
}

// aktivitasJSON is the API form of an activity entry.
type aktivitasJSON struct {
	ID          string          `json:"id"`
	KlienID     *string         `json:"klien_id"`
	Klien       string          `json:"klien"`
	ItemID      *string         `json:"item_id"`
	TugasID     *string         `json:"tugas_id"`
	Jenis       string          `json:"jenis"`
	Teks        string          `json:"teks"`
	Tanggal     string          `json:"tanggal"`
	DurasiMenit *int            `json:"durasi_menit"`
	Data        json.RawMessage `json:"data"`
	Dibuat      time.Time       `json:"dibuat"`
}

func toAktivitasJSON(a store.Aktivitas) aktivitasJSON {
	data := a.Data
	if data == nil {
		data = json.RawMessage("null")
	}
	return aktivitasJSON{
		ID: a.ID, KlienID: a.KlienID, Klien: a.Klien, ItemID: a.ItemID, TugasID: a.TugasID, Jenis: a.Jenis,
		Teks: a.Teks, Tanggal: a.Tanggal.Format(time.DateOnly), DurasiMenit: a.DurasiMenit, Data: data, Dibuat: a.Dibuat,
	}
}

// aktivitasRequest is the body of POST /api/aktivitas.
type aktivitasRequest struct {
	KlienID     *string `json:"klien_id"`
	Jenis       string  `json:"jenis"`
	Teks        string  `json:"teks"`
	Tanggal     string  `json:"tanggal"`
	DurasiMenit *int    `json:"durasi_menit"`
}

func writeAktivitasNotFound(w http.ResponseWriter) {
	writeError(w, http.StatusNotFound, "Aktivitas tidak ditemukan.", nil)
}

func (s *Server) listAktivitas(w http.ResponseWriter, r *http.Request, u store.Pengguna) {
	q := r.URL.Query()
	f := store.AktivitasFilter{KlienID: q.Get("klien_id"), Batas: 50}
	fields := map[string]string{}
	if f.KlienID != "" && !uuidPattern.MatchString(f.KlienID) {
		fields["klien_id"] = "Klien tidak ditemukan."
	}
	if v := q.Get("sebelum"); v != "" {
		if uuidPattern.MatchString(v) {
			f.SebelumID = v
		} else if t, err := time.Parse(time.RFC3339, v); err == nil {
			f.SebelumWaktu = &t
		} else {
			fields["sebelum"] = "Gunakan id aktivitas atau waktu RFC 3339."
		}
	}
	if v := q.Get("batas"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 100 {
			fields["batas"] = "Batas harus 1 sampai 100."
		}
		f.Batas = n
	}
	if len(fields) > 0 {
		writeError(w, http.StatusUnprocessableEntity, "Periksa kembali parameter Anda.", fields)
		return
	}
	batas := f.Batas
	f.Batas++ // one more tells whether there is another page
	s.cachedRead(w, r, u.ID, "aktivitas:"+f.KlienID+":"+q.Get("sebelum")+":"+strconv.Itoa(batas), func(time.Time) (any, error) {
		list, err := s.Store.ListAktivitas(r.Context(), u.ID, f)
		if err != nil {
			return nil, err
		}
		lagi := len(list) > batas
		if lagi {
			list = list[:batas]
		}
		out := make([]aktivitasJSON, 0, len(list))
		for _, a := range list {
			out = append(out, toAktivitasJSON(a))
		}
		return map[string]any{"aktivitas": out, "lagi": lagi}, nil
	})
}

// createAktivitas stores a note or a work entry the user wrote.
func (s *Server) createAktivitas(w http.ResponseWriter, r *http.Request, u store.Pengguna) {
	var req aktivitasRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	in := store.AktivitasInput{Jenis: req.Jenis, Teks: strings.TrimSpace(req.Teks), DurasiMenit: req.DurasiMenit}
	fields := map[string]string{}
	if in.Jenis != "catatan" && in.Jenis != "pekerjaan" {
		fields["jenis"] = "Pilih jenis: catatan atau pekerjaan."
	}
	switch n := utf8.RuneCountInString(in.Teks); {
	case n == 0:
		fields["teks"] = "Isi teksnya."
	case n > 2000:
		fields["teks"] = "Teks paling panjang 2000 karakter."
	}
	in.Tanggal = s.workDate(req.Tanggal, fields)
	validDuration(req.DurasiMenit, fields)
	if len(fields) == 0 && req.KlienID != nil && *req.KlienID != "" {
		id, _, f, err := s.findKlien(r.Context(), u.ID, req.KlienID, "")
		if err != nil {
			s.internal(w, r, err)
			return
		}
		if f != nil {
			fields["klien_id"] = f["klien"]
		}
		in.KlienID = id
	}
	if len(fields) > 0 {
		writeError(w, http.StatusUnprocessableEntity, "Periksa kembali isian Anda.", fields)
		return
	}
	a, err := s.Store.AddAktivitas(r.Context(), u.ID, in)
	if err != nil {
		s.internal(w, r, err)
		return
	}
	s.bump(r.Context(), u.ID)
	w.Header().Set("Location", "/api/aktivitas/"+a.ID)
	writeJSON(w, http.StatusCreated, toAktivitasJSON(a))
}

func (s *Server) deleteAktivitas(w http.ResponseWriter, r *http.Request, u store.Pengguna) {
	id, ok := itemID(r)
	if !ok {
		writeAktivitasNotFound(w)
		return
	}
	err := s.Store.DeleteAktivitas(r.Context(), u.ID, id)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeAktivitasNotFound(w)
		return
	case errors.Is(err, store.ErrAktivitasOtomatis):
		writeError(w, http.StatusUnprocessableEntity, "Hanya catatan dan pekerjaan yang dapat dihapus.", nil)
		return
	case err != nil:
		s.internal(w, r, err)
		return
	}
	s.bump(r.Context(), u.ID)
	w.WriteHeader(http.StatusNoContent)
}

// workDate reads the optional date work was done: today in Jakarta when empty,
// never after today.
func (s *Server) workDate(v string, fields map[string]string) time.Time {
	today := expiry.DateOf(s.Now())
	v = strings.TrimSpace(v)
	if v == "" {
		return today
	}
	d, err := time.Parse(time.DateOnly, v)
	switch {
	case err != nil:
		fields["tanggal"] = "Isi tanggal dengan format TTTT-BB-HH."
	case d.Year() < 2000:
		fields["tanggal"] = "Tanggal di luar rentang yang masuk akal."
	case d.After(today):
		fields["tanggal"] = "Tanggal tidak boleh setelah hari ini."
	}
	return d
}

func validDuration(d *int, fields map[string]string) {
	if d != nil && (*d < 1 || *d > 1440) {
		fields["durasi_menit"] = "Durasi harus 1 sampai 1440 menit."
	}
}

// catat records an event the app noticed. A failure is logged and never fails
// the request that caused it: the write itself already happened.
func (s *Server) catat(ctx context.Context, userID string, in store.AktivitasInput) {
	if in.Tanggal.IsZero() {
		in.Tanggal = expiry.DateOf(s.Now())
	}
	if utf8.RuneCountInString(in.Teks) > 2000 {
		in.Teks = string([]rune(in.Teks)[:1999]) + "…"
	}
	_, err := s.Store.AddAktivitas(ctx, userID, in)
	if err != nil && !errors.Is(err, store.ErrNotFound) { // ErrNotFound: already recorded today
		s.Log.Error("gagal mencatat aktivitas", "jenis", in.Jenis, "galat", err)
	}
}

// data encodes an entry's extra fields.
func data(v map[string]any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return b
}

var bulanPendek = [...]string{"Jan", "Feb", "Mar", "Apr", "Mei", "Jun", "Jul", "Agu", "Sep", "Okt", "Nov", "Des"}

var bulanPanjang = [...]string{"Januari", "Februari", "Maret", "April", "Mei", "Juni", "Juli", "Agustus", "September",
	"Oktober", "November", "Desember"}

// tanggalID writes a date the Indonesian way: 14 Nov 2027.
func tanggalID(d time.Time) string {
	return strconv.Itoa(d.Day()) + " " + bulanPendek[d.Month()-1] + " " + strconv.Itoa(d.Year())
}

// The automatic entries for assets. Each goes to the asset's client.

func (s *Server) catatAsetBaru(ctx context.Context, it store.Item) {
	id := it.ID
	s.catat(ctx, it.PenggunaID, store.AktivitasInput{
		KlienID: it.KlienID, ItemID: &id, Jenis: "aset_baru",
		Teks: it.Nama + " ditambahkan, berakhir " + tanggalID(it.Kedaluwarsa),
		Data: data(map[string]any{"nama": it.Nama, "jenis": it.Jenis, "kedaluwarsa": it.Kedaluwarsa.Format(time.DateOnly)}),
	})
}

// catatAsetDiubah records a renewal when an edit moved the date later.
func (s *Server) catatAsetDiubah(ctx context.Context, old, it store.Item) {
	if !it.Kedaluwarsa.After(old.Kedaluwarsa) {
		return
	}
	id := it.ID
	s.catat(ctx, it.PenggunaID, store.AktivitasInput{
		KlienID: it.KlienID, ItemID: &id, Jenis: "aset_diperpanjang",
		Teks: it.Nama + " diperpanjang sampai " + tanggalID(it.Kedaluwarsa),
		Data: data(map[string]any{"dari": old.Kedaluwarsa.Format(time.DateOnly), "sampai": it.Kedaluwarsa.Format(time.DateOnly)}),
	})
}

// catatAsetDihapus runs after the delete, so the entry keeps the id only in
// its data (item_id would point at a row that is gone).
func (s *Server) catatAsetDihapus(ctx context.Context, it store.Item) {
	s.catat(ctx, it.PenggunaID, store.AktivitasInput{
		KlienID: it.KlienID, Jenis: "aset_dihapus",
		Teks: it.Nama + " dihapus",
		Data: data(map[string]any{"item_id": it.ID, "nama": it.Nama, "jenis": it.Jenis}),
	})
}

// catatCek records what a manual or periodic check found: a later date (the
// certificate or domain was renewed outside Tenggat), or an error, at most
// once per asset per day.
func (s *Server) catatCek(ctx context.Context, old, it store.Item) {
	id := it.ID
	if it.GalatCek != nil {
		s.catat(ctx, it.PenggunaID, store.AktivitasInput{
			KlienID: it.KlienID, ItemID: &id, Jenis: "cek_gagal", SekaliSehari: true,
			Teks: "Cek " + it.Nama + " gagal. " + *it.GalatCek,
			Data: data(map[string]any{"galat": *it.GalatCek}),
		})
		return
	}
	if !it.Kedaluwarsa.After(old.Kedaluwarsa) {
		return
	}
	teks := "Sertifikat " + it.Nama + " diperbarui, berlaku sampai " + tanggalID(it.Kedaluwarsa)
	if it.Sumber == "registri" {
		teks = "Domain " + it.Nama + " diperpanjang di registri sampai " + tanggalID(it.Kedaluwarsa)
	}
	s.catat(ctx, it.PenggunaID, store.AktivitasInput{
		KlienID: it.KlienID, ItemID: &id, Jenis: "cek_diperbarui", Teks: teks,
		Data: data(map[string]any{"dari": old.Kedaluwarsa.Format(time.DateOnly),
			"sampai": it.Kedaluwarsa.Format(time.DateOnly), "sumber": it.Sumber}),
	})
}
