package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mrramdn/tenggat/api/internal/expiry"
	"github.com/mrramdn/tenggat/api/internal/store"
)

// TugasStore is the part of Store that keeps recurring tasks.
type TugasStore interface {
	ListTugas(ctx context.Context, userID string, f store.TugasFilter) ([]store.Tugas, error)
	GetTugas(ctx context.Context, userID, id string) (store.Tugas, error)
	CreateTugas(ctx context.Context, userID string, in store.TugasInput) (store.Tugas, error)
	UpdateTugas(ctx context.Context, userID, id string, in store.TugasInput) (store.Tugas, error)
	CompleteTugas(ctx context.Context, userID, id string, from, next, at time.Time) (store.Tugas, error)
	DeleteTugas(ctx context.Context, userID, id string) error
}

// tugasJSON is the API form of a task, with the days left and status worked out.
type tugasJSON struct {
	ID              string        `json:"id"`
	KlienID         *string       `json:"klien_id"`
	Klien           string        `json:"klien"`
	Judul           string        `json:"judul"`
	Catatan         string        `json:"catatan"`
	UlangJumlah     int           `json:"ulang_jumlah"`
	UlangSatuan     string        `json:"ulang_satuan"`
	JatuhTempo      string        `json:"jatuh_tempo"`
	SisaHari        int           `json:"sisa_hari"`
	Status          expiry.Status `json:"status"`
	Biaya           *int64        `json:"biaya"`
	TerakhirSelesai *time.Time    `json:"terakhir_selesai"`
	Dibuat          time.Time     `json:"dibuat"`
	Diubah          time.Time     `json:"diubah"`
}

func toTugasJSON(t store.Tugas, now time.Time) tugasJSON {
	days := expiry.DaysLeft(t.JatuhTempo, now)
	return tugasJSON{
		ID: t.ID, KlienID: t.KlienID, Klien: t.Klien, Judul: t.Judul, Catatan: t.Catatan,
		UlangJumlah: t.UlangJumlah, UlangSatuan: t.UlangSatuan, JatuhTempo: t.JatuhTempo.Format(time.DateOnly),
		SisaHari: days, Status: expiry.For(days), Biaya: t.Biaya, TerakhirSelesai: t.TerakhirSelesai,
		Dibuat: t.Dibuat, Diubah: t.Diubah,
	}
}

// tugasRequest is the body of POST /api/tugas and PUT /api/tugas/{id}.
type tugasRequest struct {
	Judul       string  `json:"judul"`
	Catatan     string  `json:"catatan"`
	Klien       string  `json:"klien"`    // a client's name; ignored when klien_id is set
	KlienID     *string `json:"klien_id"` // one of the user's clients
	UlangJumlah *int    `json:"ulang_jumlah"`
	UlangSatuan string  `json:"ulang_satuan"`
	JatuhTempo  string  `json:"jatuh_tempo"`
	Biaya       *int64  `json:"biaya"`
}

// selesaiRequest is the optional body of POST /api/tugas/{id}/selesai.
type selesaiRequest struct {
	Catatan     string `json:"catatan"`
	DurasiMenit *int   `json:"durasi_menit"`
	Tanggal     string `json:"tanggal"`
}

func writeTugasNotFound(w http.ResponseWriter) {
	writeError(w, http.StatusNotFound, "Tugas tidak ditemukan.", nil)
}

// prepareTugas validates a task request and resolves its client. It writes the
// error response and returns false when the task cannot be stored.
func (s *Server) prepareTugas(w http.ResponseWriter, r *http.Request, userID string, req tugasRequest) (store.TugasInput, bool) {
	in := store.TugasInput{
		Judul:       strings.TrimSpace(req.Judul),
		Catatan:     strings.TrimSpace(req.Catatan),
		UlangJumlah: 1,
		UlangSatuan: strings.TrimSpace(req.UlangSatuan),
		Biaya:       req.Biaya,
	}
	klien := strings.TrimSpace(req.Klien)
	fields := map[string]string{}
	if n := utf8.RuneCountInString(in.Judul); n < 2 || n > 120 {
		fields["judul"] = "Judul harus 2 sampai 120 karakter."
	}
	if utf8.RuneCountInString(in.Catatan) > 500 {
		fields["catatan"] = "Catatan paling panjang 500 karakter."
	}
	if utf8.RuneCountInString(klien) > 80 {
		fields["klien"] = "Nama klien paling panjang 80 karakter."
	}
	if req.UlangJumlah != nil {
		in.UlangJumlah = *req.UlangJumlah
	}
	if in.UlangJumlah < 1 || in.UlangJumlah > 365 {
		fields["ulang_jumlah"] = "Isi angka 1 sampai 365."
	}
	if in.UlangSatuan == "" {
		in.UlangSatuan = "bulan"
	}
	if !expiry.Units[in.UlangSatuan] {
		fields["ulang_satuan"] = "Pilih satuan: hari, minggu, bulan, atau tahun."
	}
	if in.Biaya != nil && (*in.Biaya < 0 || *in.Biaya > 1_000_000_000_000) {
		fields["biaya"] = "Biaya harus angka rupiah 0 atau lebih."
	}
	d, err := time.Parse(time.DateOnly, strings.TrimSpace(req.JatuhTempo))
	switch {
	case err != nil:
		fields["jatuh_tempo"] = "Isi tanggal jatuh tempo dengan format TTTT-BB-HH."
	case d.Year() < 2000 || d.Year() > 2100:
		fields["jatuh_tempo"] = "Tanggal jatuh tempo di luar rentang yang masuk akal."
	default:
		in.JatuhTempo = d
	}
	if len(fields) == 0 {
		id, _, f, err := s.findKlien(r.Context(), userID, req.KlienID, klien)
		if err != nil {
			s.internal(w, r, err)
			return in, false
		}
		fields, in.KlienID = f, id
	}
	if len(fields) > 0 {
		writeError(w, http.StatusUnprocessableEntity, "Periksa kembali isian Anda.", fields)
		return in, false
	}
	return in, true
}

func (s *Server) listTugas(w http.ResponseWriter, r *http.Request, u store.Pengguna) {
	q := r.URL.Query()
	f := store.TugasFilter{KlienID: q.Get("klien_id")}
	fields := map[string]string{}
	if f.KlienID != "" && !uuidPattern.MatchString(f.KlienID) {
		fields["klien_id"] = "Klien tidak ditemukan."
	}
	dalam := q.Get("dalam_hari")
	if dalam != "" {
		n, err := strconv.Atoi(dalam)
		if err != nil || n < 0 || n > 3660 {
			fields["dalam_hari"] = "Isi jumlah hari 0 sampai 3660."
		} else {
			d := expiry.DateOf(s.Now()).AddDate(0, 0, n)
			f.Sampai = &d
		}
	}
	if len(fields) > 0 {
		writeError(w, http.StatusUnprocessableEntity, "Periksa kembali parameter Anda.", fields)
		return
	}
	s.cachedRead(w, r, u.ID, "tugas:"+f.KlienID+":"+dalam, func(now time.Time) (any, error) {
		list, err := s.Store.ListTugas(r.Context(), u.ID, f)
		if err != nil {
			return nil, err
		}
		out := make([]tugasJSON, 0, len(list))
		for _, t := range list {
			out = append(out, toTugasJSON(t, now))
		}
		return map[string]any{"tugas": out}, nil
	})
}

func (s *Server) createTugas(w http.ResponseWriter, r *http.Request, u store.Pengguna) {
	var req tugasRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	in, ok := s.prepareTugas(w, r, u.ID, req)
	if !ok {
		return
	}
	t, err := s.Store.CreateTugas(r.Context(), u.ID, in)
	if err != nil {
		s.internal(w, r, err)
		return
	}
	id := t.ID
	s.catat(r.Context(), u.ID, store.AktivitasInput{
		KlienID: t.KlienID, TugasID: &id, Jenis: "tugas_baru",
		Teks: "Tugas baru: " + t.Judul + ", jatuh tempo " + tanggalID(t.JatuhTempo),
		Data: data(map[string]any{"judul": t.Judul, "jatuh_tempo": t.JatuhTempo.Format(time.DateOnly)}),
	})
	s.bump(r.Context(), u.ID)
	w.Header().Set("Location", "/api/tugas/"+t.ID)
	writeJSON(w, http.StatusCreated, toTugasJSON(t, s.Now()))
}

func (s *Server) updateTugas(w http.ResponseWriter, r *http.Request, u store.Pengguna) {
	id, ok := itemID(r)
	if !ok {
		writeTugasNotFound(w)
		return
	}
	if _, err := s.Store.GetTugas(r.Context(), u.ID, id); errors.Is(err, store.ErrNotFound) {
		writeTugasNotFound(w)
		return
	} else if err != nil {
		s.internal(w, r, err)
		return
	}
	var req tugasRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	in, ok := s.prepareTugas(w, r, u.ID, req)
	if !ok {
		return
	}
	t, err := s.Store.UpdateTugas(r.Context(), u.ID, id, in)
	if errors.Is(err, store.ErrNotFound) {
		writeTugasNotFound(w)
		return
	}
	if err != nil {
		s.internal(w, r, err)
		return
	}
	s.bump(r.Context(), u.ID)
	writeJSON(w, http.StatusOK, toTugasJSON(t, s.Now()))
}

func (s *Server) deleteTugas(w http.ResponseWriter, r *http.Request, u store.Pengguna) {
	id, ok := itemID(r)
	if !ok {
		writeTugasNotFound(w)
		return
	}
	t, err := s.Store.GetTugas(r.Context(), u.ID, id)
	if err == nil {
		err = s.Store.DeleteTugas(r.Context(), u.ID, id)
	}
	if errors.Is(err, store.ErrNotFound) {
		writeTugasNotFound(w)
		return
	}
	if err != nil {
		s.internal(w, r, err)
		return
	}
	// Recorded after the delete, so the task id lives only in the data.
	s.catat(r.Context(), u.ID, store.AktivitasInput{
		KlienID: t.KlienID, Jenis: "tugas_dihapus", Teks: "Tugas dihapus: " + t.Judul,
		Data: data(map[string]any{"tugas_id": t.ID, "judul": t.Judul}),
	})
	s.bump(r.Context(), u.ID)
	w.WriteHeader(http.StatusNoContent)
}

// completeTugas marks a task done: the due date moves forward by whole
// intervals from the old due date until it is after today, and the work is
// recorded as a tugas_selesai entry.
func (s *Server) completeTugas(w http.ResponseWriter, r *http.Request, u store.Pengguna) {
	id, ok := itemID(r)
	if !ok {
		writeTugasNotFound(w)
		return
	}
	var req selesaiRequest
	if !decodeOptionalJSON(w, r, &req) {
		return
	}
	req.Catatan = strings.TrimSpace(req.Catatan)
	fields := map[string]string{}
	if utf8.RuneCountInString(req.Catatan) > 500 {
		fields["catatan"] = "Catatan paling panjang 500 karakter."
	}
	validDuration(req.DurasiMenit, fields)
	tanggal := s.workDate(req.Tanggal, fields)
	if len(fields) > 0 {
		writeError(w, http.StatusUnprocessableEntity, "Periksa kembali isian Anda.", fields)
		return
	}
	old, err := s.Store.GetTugas(r.Context(), u.ID, id)
	if errors.Is(err, store.ErrNotFound) {
		writeTugasNotFound(w)
		return
	}
	if err != nil {
		s.internal(w, r, err)
		return
	}
	now := s.Now()
	next := expiry.NextDue(old.JatuhTempo, expiry.DateOf(now), old.UlangJumlah, old.UlangSatuan)
	t, err := s.Store.CompleteTugas(r.Context(), u.ID, id, old.JatuhTempo, next, now)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusConflict, "Tugas ini baru saja diubah. Muat ulang lalu coba lagi.", nil)
		return
	}
	if err != nil {
		s.internal(w, r, err)
		return
	}
	extra := map[string]any{"jatuh_tempo_lama": old.JatuhTempo.Format(time.DateOnly),
		"jatuh_tempo_baru": next.Format(time.DateOnly)}
	if req.Catatan != "" {
		extra["catatan"] = req.Catatan
	}
	s.catat(r.Context(), u.ID, store.AktivitasInput{
		KlienID: t.KlienID, TugasID: &t.ID, Jenis: "tugas_selesai", Teks: "Selesai: " + t.Judul,
		Tanggal: tanggal, DurasiMenit: req.DurasiMenit, Data: data(extra),
	})
	s.bump(r.Context(), u.ID)
	writeJSON(w, http.StatusOK, toTugasJSON(t, now))
}

// decodeOptionalJSON is decodeJSON for a body that may be left out entirely.
func decodeOptionalJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 16<<10))
	if err != nil {
		writeError(w, http.StatusBadRequest, "Isi permintaan harus berupa JSON yang valid.", nil)
		return false
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return true
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	err = dec.Decode(v)
	if err == nil && dec.Decode(&struct{}{}) != io.EOF {
		err = errors.New("lebih dari satu objek JSON")
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, "Isi permintaan harus berupa JSON yang valid.", nil)
		return false
	}
	return true
}
