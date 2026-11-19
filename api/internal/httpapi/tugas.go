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
	"github.com/mrramdn/tenggat/api/internal/uang"
)

// TugasStore is the part of Store that keeps recurring and one-off tasks.
type TugasStore interface {
	ListTugas(ctx context.Context, userID string, f store.TugasFilter) ([]store.Tugas, error)
	GetTugas(ctx context.Context, userID, id string) (store.Tugas, error)
	CreateTugas(ctx context.Context, userID string, in store.TugasInput) (store.Tugas, error)
	UpdateTugas(ctx context.Context, userID, id string, in store.TugasInput) (store.Tugas, error)
	CompleteTugas(ctx context.Context, userID, id string, from, next, at time.Time) (store.Tugas, error)
	CloseTugas(ctx context.Context, userID, id string, from, at time.Time) (store.Tugas, error)
	DeleteTugas(ctx context.Context, userID, id string) error
}

func isSekali(t store.Tugas) bool {
	return t.UlangJumlah == 0 && t.UlangSatuan == ""
}

// tugasJSON is the API form of a task, with the days left and status worked out.
type tugasJSON struct {
	ID              string        `json:"id"`
	KlienID         *string       `json:"klien_id"`
	Klien           string        `json:"klien"`
	Judul           string        `json:"judul"`
	Catatan         string        `json:"catatan"`
	Sekali          bool          `json:"sekali"`
	UlangJumlah     int           `json:"ulang_jumlah"`
	UlangSatuan     string        `json:"ulang_satuan"`
	JatuhTempo      string        `json:"jatuh_tempo"`
	SisaHari        int           `json:"sisa_hari"`
	Status          expiry.Status `json:"status"`
	Biaya           *json.Number  `json:"biaya"`
	MataUang        string        `json:"mata_uang"`
	TerakhirSelesai *time.Time    `json:"terakhir_selesai"`
	SelesaiPada     *time.Time    `json:"selesai_pada"`
	Dibuat          time.Time     `json:"dibuat"`
	Diubah          time.Time     `json:"diubah"`
}

// MarshalJSON emits ulang_jumlah and ulang_satuan as null for one-off tasks
// while keeping the Go fields non-pointer for callers.
func (t tugasJSON) MarshalJSON() ([]byte, error) {
	type alias tugasJSON
	var jumlah *int
	var satuan *string
	if !t.Sekali {
		jumlah, satuan = &t.UlangJumlah, &t.UlangSatuan
	}
	return json.Marshal(struct {
		alias
		UlangJumlah *int    `json:"ulang_jumlah"`
		UlangSatuan *string `json:"ulang_satuan"`
	}{alias(t), jumlah, satuan})
}

func toTugasJSON(t store.Tugas, now time.Time) tugasJSON {
	days := expiry.DaysLeft(t.JatuhTempo, now)
	mu := t.MataUang
	if mu == "" {
		mu = "IDR"
	}
	var biaya *json.Number
	if t.Biaya != nil {
		n := uang.JSON(*t.Biaya, mu)
		biaya = &n
	}
	return tugasJSON{
		ID: t.ID, KlienID: t.KlienID, Klien: t.Klien, Judul: t.Judul, Catatan: t.Catatan,
		Sekali: isSekali(t), UlangJumlah: t.UlangJumlah, UlangSatuan: t.UlangSatuan,
		JatuhTempo: t.JatuhTempo.Format(time.DateOnly), SisaHari: days, Status: expiry.For(days),
		Biaya: biaya, MataUang: mu, TerakhirSelesai: t.TerakhirSelesai, SelesaiPada: t.SelesaiPada,
		Dibuat: t.Dibuat, Diubah: t.Diubah,
	}
}

// tugasRequest is the body of POST /api/tugas and PUT /api/tugas/{id}.
type tugasRequest struct {
	Judul       string       `json:"judul"`
	Catatan     string       `json:"catatan"`
	Klien       string       `json:"klien"`    // a client's name; ignored when klien_id is set
	KlienID     *string      `json:"klien_id"` // one of the user's clients
	Sekali      bool         `json:"sekali"`
	UlangJumlah *int         `json:"ulang_jumlah"`
	UlangSatuan string       `json:"ulang_satuan"`
	JatuhTempo  string       `json:"jatuh_tempo"`
	Biaya       *json.Number `json:"biaya"`
	MataUang    string       `json:"mata_uang"`
}

// selesaiRequest is the optional body of POST /api/tugas/{id}/selesai.
type selesaiRequest struct {
	Catatan     string `json:"catatan"`
	DurasiMenit *int   `json:"durasi_menit"`
	Tanggal     string `json:"tanggal"`
	Dari        string `json:"dari"`
}

func writeTugasNotFound(w http.ResponseWriter) {
	writeError(w, http.StatusNotFound, "Tugas tidak ditemukan.", nil)
}

// prepareTugas validates a task request and resolves its client. It writes the
// error response and returns false when the task cannot be stored.
func (s *Server) prepareTugas(w http.ResponseWriter, r *http.Request, userID string, req tugasRequest, defaultMataUang string) (store.TugasInput, bool) {
	in := store.TugasInput{
		Judul:    strings.TrimSpace(req.Judul),
		Catatan:  strings.TrimSpace(req.Catatan),
		MataUang: strings.TrimSpace(req.MataUang),
	}
	if in.MataUang == "" {
		in.MataUang = defaultMataUang
	}
	if in.MataUang == "" {
		in.MataUang = "IDR"
	}
	klien := strings.TrimSpace(req.Klien)
	fields := map[string]string{}
	if n := utf8.RuneCountInString(in.Judul); n < 2 || n > 120 {
		fields["judul"] = "Judul minimal 2 dan maksimal 120 karakter."
	}
	if utf8.RuneCountInString(in.Catatan) > 500 {
		fields["catatan"] = "Catatan maksimal 500 karakter."
	}
	if utf8.RuneCountInString(klien) > 80 {
		fields["klien"] = "Nama klien maksimal 80 karakter."
	}
	satuan := strings.TrimSpace(req.UlangSatuan)
	if req.Sekali {
		if req.UlangJumlah != nil || satuan != "" {
			fields["ulang_satuan"] = "Tugas sekali jalan tidak berulang."
		}
	} else {
		in.UlangJumlah = 1
		if req.UlangJumlah != nil {
			in.UlangJumlah = *req.UlangJumlah
		}
		if in.UlangJumlah < 1 || in.UlangJumlah > 365 {
			fields["ulang_jumlah"] = "Isi angka 1 sampai 365."
		}
		in.UlangSatuan = satuan
		if in.UlangSatuan == "" {
			in.UlangSatuan = "bulan"
		}
		if !expiry.Units[in.UlangSatuan] {
			fields["ulang_satuan"] = "Pilih satuan: hari, minggu, bulan, atau tahun."
		}
	}
	if !uang.Valid(in.MataUang) {
		fields["mata_uang"] = "Pilih IDR atau USD."
	} else if req.Biaya != nil {
		minor, err := uang.Parse(*req.Biaya, in.MataUang)
		if err != nil {
			fields["biaya"] = "Biaya harus angka 0 atau lebih; rupiah tanpa desimal, USD paling banyak 2 desimal."
		} else {
			in.Biaya = &minor
		}
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
	switch q.Get("selesai") {
	case "", "0":
	case "1":
		f.Selesai = true
	default:
		fields["selesai"] = "Gunakan 0 atau 1."
	}
	if len(fields) > 0 {
		writeError(w, http.StatusUnprocessableEntity, "Periksa kembali parameter Anda.", fields)
		return
	}
	selesaiKey := "0"
	if f.Selesai {
		selesaiKey = "1"
	}
	s.cachedRead(w, r, u.ID, "tugas:"+f.KlienID+":"+dalam+":"+selesaiKey, func(now time.Time) (any, error) {
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
	in, ok := s.prepareTugas(w, r, u.ID, req, "IDR")
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
	old, err := s.Store.GetTugas(r.Context(), u.ID, id)
	if errors.Is(err, store.ErrNotFound) {
		writeTugasNotFound(w)
		return
	} else if err != nil {
		s.internal(w, r, err)
		return
	}
	if old.SelesaiPada != nil {
		writeError(w, http.StatusConflict, "Tugas ini sudah selesai.", nil)
		return
	}
	var req tugasRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	in, ok := s.prepareTugas(w, r, u.ID, req, old.MataUang)
	if !ok {
		return
	}
	t, err := s.Store.UpdateTugas(r.Context(), u.ID, id, in)
	if errors.Is(err, store.ErrNotFound) {
		if cur, err2 := s.Store.GetTugas(r.Context(), u.ID, id); err2 == nil && cur.SelesaiPada != nil {
			writeError(w, http.StatusConflict, "Tugas ini sudah selesai.", nil)
			return
		}
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

// completeTugas marks a task done: a recurring task's due date moves forward by
// whole intervals from the old due date until it is after today, while a
// one-off task is closed. Either way the work is recorded as a tugas_selesai
// entry.
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
		fields["catatan"] = "Catatan maksimal 500 karakter."
	}
	validDuration(req.DurasiMenit, fields)
	tanggal := s.workDate(req.Tanggal, fields)
	var dari time.Time
	hasDari := false
	if rawDari := strings.TrimSpace(req.Dari); rawDari != "" {
		d, err := time.Parse(time.DateOnly, rawDari)
		if err != nil {
			fields["dari"] = "Isi tanggal dengan format TTTT-BB-HH."
		} else {
			dari, hasDari = d, true
		}
	}
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
	if old.SelesaiPada != nil {
		writeError(w, http.StatusConflict, "Tugas ini sudah selesai.", nil)
		return
	}
	if hasDari && !dari.Equal(old.JatuhTempo) {
		writeError(w, http.StatusConflict, "Tugas ini baru saja diubah. Muat ulang lalu coba lagi.", nil)
		return
	}
	now := s.Now()
	extra := map[string]any{"jatuh_tempo_lama": old.JatuhTempo.Format(time.DateOnly)}
	var t store.Tugas
	if isSekali(old) {
		t, err = s.Store.CloseTugas(r.Context(), u.ID, id, old.JatuhTempo, now)
		extra["sekali"] = true
	} else {
		next := expiry.NextDue(old.JatuhTempo, expiry.DateOf(now), old.UlangJumlah, old.UlangSatuan)
		t, err = s.Store.CompleteTugas(r.Context(), u.ID, id, old.JatuhTempo, next, now)
		extra["jatuh_tempo_baru"] = next.Format(time.DateOnly)
	}
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusConflict, "Tugas ini baru saja diubah. Muat ulang lalu coba lagi.", nil)
		return
	}
	if err != nil {
		s.internal(w, r, err)
		return
	}
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
