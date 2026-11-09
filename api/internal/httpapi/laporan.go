package httpapi

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mrramdn/tenggat/api/internal/auth"
	"github.com/mrramdn/tenggat/api/internal/expiry"
	"github.com/mrramdn/tenggat/api/internal/store"
)

// LaporanStore is the part of Store that keeps monthly reports and their links.
type LaporanStore interface {
	CatatanLaporan(ctx context.Context, klienID string, bulan time.Time) (string, error)
	SetCatatanLaporan(ctx context.Context, userID, klienID string, bulan time.Time, teks string) error
	SetReportToken(ctx context.Context, userID, klienID, tokenHash string) error
	DeleteReportToken(ctx context.Context, userID, klienID string) error
	KlienByReportToken(ctx context.Context, tokenHash string) (store.Klien, store.Pengguna, error)
}

// reportPerMinute caps public report requests per client; a person reading a
// report needs a handful, so this only stops someone guessing tokens.
const reportPerMinute = 60

const reportPrefix = "/api/laporan/"

// jatuhTempoHari is how far ahead the report's "due soon" list looks.
const jatuhTempoHari = 60

type namaJSON struct {
	Nama string `json:"nama"`
}

type laporanRingkasan struct {
	JumlahAset  int            `json:"jumlah_aset"`
	PerStatus   map[string]int `json:"per_status"`
	Pekerjaan   int            `json:"pekerjaan"`
	Menit       int            `json:"menit"`
	Biaya30Hari int64          `json:"biaya_30_hari"`
}

type laporanPekerjaan struct {
	Tanggal     string `json:"tanggal"`
	Teks        string `json:"teks"`
	DurasiMenit *int   `json:"durasi_menit"`
	Jenis       string `json:"jenis"` // pekerjaan or tugas_selesai
}

type laporanPerpanjangan struct {
	Tanggal string `json:"tanggal"`
	Teks    string `json:"teks"`
}

type laporanAset struct {
	Nama        string        `json:"nama"`
	Jenis       string        `json:"jenis"`
	Host        *string       `json:"host"`
	Kedaluwarsa string        `json:"kedaluwarsa"`
	SisaHari    int           `json:"sisa_hari"`
	Status      expiry.Status `json:"status"`
	Sumber      string        `json:"sumber"`
	Biaya       *int64        `json:"biaya"`
}

type laporanJatuhTempo struct {
	Nama     string        `json:"nama"`
	Jenis    string        `json:"jenis"` // an asset's jenis, or "tugas"
	Tanggal  string        `json:"tanggal"`
	SisaHari int           `json:"sisa_hari"`
	Status   expiry.Status `json:"status"`
	Biaya    *int64        `json:"biaya"`
}

// laporanJSON is one client's monthly report. The owner and the read-only link
// see the same thing: internal notes (activity notes, asset, task and client
// notes) never go in; catatan is the owner's words written for the client.
type laporanJSON struct {
	Klien        namaJSON              `json:"klien"`
	Penyusun     namaJSON              `json:"penyusun"`
	Bulan        string                `json:"bulan"`
	LabelBulan   string                `json:"label_bulan"`
	Dibuat       time.Time             `json:"dibuat"`
	Catatan      string                `json:"catatan"`
	Ringkasan    laporanRingkasan      `json:"ringkasan"`
	Pekerjaan    []laporanPekerjaan    `json:"pekerjaan"`
	Perpanjangan []laporanPerpanjangan `json:"perpanjangan"`
	Aset         []laporanAset         `json:"aset"`
	JatuhTempo   []laporanJatuhTempo   `json:"jatuh_tempo"`
}

// reportMonth reads ?bulan=TTTT-BB, the current month in Jakarta when empty,
// as its first day.
func (s *Server) reportMonth(v string) (time.Time, bool) {
	if v == "" {
		y, m, _ := expiry.DateOf(s.Now()).Date()
		return time.Date(y, m, 1, 0, 0, 0, 0, time.UTC), true
	}
	d, err := time.Parse("2006-01", v)
	if err != nil || d.Year() < 2000 || d.Year() > 2100 {
		return time.Time{}, false
	}
	return d, true
}

func writeBadMonth(w http.ResponseWriter) {
	writeError(w, http.StatusUnprocessableEntity, "Bulan tidak valid.",
		map[string]string{"bulan": "Tulis bulan dengan format TTTT-BB, misalnya 2026-11."})
}

// buildLaporan gathers one client's report for one month.
func (s *Server) buildLaporan(ctx context.Context, owner store.Pengguna, k store.Klien, bulan, now time.Time) (laporanJSON, error) {
	end := bulan.AddDate(0, 1, -1)
	rep := laporanJSON{
		Klien: namaJSON{k.Nama}, Penyusun: namaJSON{owner.Nama},
		Bulan: bulan.Format("2006-01"), LabelBulan: bulanPanjang[bulan.Month()-1] + " " + strconv.Itoa(bulan.Year()),
		Dibuat: now,
		Ringkasan: laporanRingkasan{PerStatus: map[string]int{
			string(expiry.Lewat): 0, string(expiry.Kritis): 0, string(expiry.Segera): 0, string(expiry.Aman): 0}},
		Pekerjaan: []laporanPekerjaan{}, Perpanjangan: []laporanPerpanjangan{},
		Aset: []laporanAset{}, JatuhTempo: []laporanJatuhTempo{},
	}
	var err error
	if rep.Catatan, err = s.Store.CatatanLaporan(ctx, k.ID, bulan); err != nil {
		return rep, err
	}

	entries, err := s.Store.ListAktivitas(ctx, owner.ID, store.AktivitasFilter{
		KlienID: k.ID, Dari: &bulan, Sampai: &end, Batas: 2000,
		Jenis: []string{"pekerjaan", "tugas_selesai", "aset_diperpanjang", "cek_diperbarui"},
	})
	if err != nil {
		return rep, err
	}
	slices.Reverse(entries) // oldest first
	slices.SortStableFunc(entries, func(a, b store.Aktivitas) int { return a.Tanggal.Compare(b.Tanggal) })
	for _, a := range entries {
		day := a.Tanggal.Format(time.DateOnly)
		switch a.Jenis {
		case "pekerjaan", "tugas_selesai":
			rep.Pekerjaan = append(rep.Pekerjaan, laporanPekerjaan{Tanggal: day, Teks: a.Teks, DurasiMenit: a.DurasiMenit, Jenis: a.Jenis})
			rep.Ringkasan.Pekerjaan++
			if a.DurasiMenit != nil {
				rep.Ringkasan.Menit += *a.DurasiMenit
			}
		default:
			rep.Perpanjangan = append(rep.Perpanjangan, laporanPerpanjangan{Tanggal: day, Teks: a.Teks})
		}
	}

	items, err := s.Store.List(ctx, owner.ID, store.Filter{KlienID: k.ID}, now)
	if err != nil {
		return rep, err
	}
	for _, it := range items {
		days := expiry.DaysLeft(it.Kedaluwarsa, now)
		st := expiry.For(days)
		day := it.Kedaluwarsa.Format(time.DateOnly)
		rep.Aset = append(rep.Aset, laporanAset{Nama: it.Nama, Jenis: it.Jenis, Host: it.Host, Kedaluwarsa: day,
			SisaHari: days, Status: st, Sumber: it.Sumber, Biaya: it.Biaya})
		rep.Ringkasan.JumlahAset++
		rep.Ringkasan.PerStatus[string(st)]++
		if days <= 30 && it.Biaya != nil {
			rep.Ringkasan.Biaya30Hari += *it.Biaya
		}
		if days <= jatuhTempoHari {
			rep.JatuhTempo = append(rep.JatuhTempo, laporanJatuhTempo{Nama: it.Nama, Jenis: it.Jenis, Tanggal: day,
				SisaHari: days, Status: st, Biaya: it.Biaya})
		}
	}
	limit := expiry.DateOf(now).AddDate(0, 0, jatuhTempoHari)
	tasks, err := s.Store.ListTugas(ctx, owner.ID, store.TugasFilter{KlienID: k.ID, Sampai: &limit})
	if err != nil {
		return rep, err
	}
	for _, t := range tasks {
		days := expiry.DaysLeft(t.JatuhTempo, now)
		rep.JatuhTempo = append(rep.JatuhTempo, laporanJatuhTempo{Nama: t.Judul, Jenis: "tugas",
			Tanggal: t.JatuhTempo.Format(time.DateOnly), SisaHari: days, Status: expiry.For(days), Biaya: t.Biaya})
	}
	slices.SortStableFunc(rep.JatuhTempo, func(a, b laporanJatuhTempo) int { return strings.Compare(a.Tanggal, b.Tanggal) })
	return rep, nil
}

// ownKlien reads the {id} client of the signed-in user, writing a 404 when it
// is not theirs.
func (s *Server) ownKlien(w http.ResponseWriter, r *http.Request, u store.Pengguna) (store.Klien, bool) {
	id, ok := itemID(r)
	if !ok {
		writeKlienNotFound(w)
		return store.Klien{}, false
	}
	k, err := s.Store.GetKlien(r.Context(), u.ID, id)
	if errors.Is(err, store.ErrNotFound) {
		writeKlienNotFound(w)
		return k, false
	}
	if err != nil {
		s.internal(w, r, err)
		return k, false
	}
	return k, true
}

func (s *Server) getLaporan(w http.ResponseWriter, r *http.Request, u store.Pengguna) {
	bulan, ok := s.reportMonth(r.URL.Query().Get("bulan"))
	if !ok {
		writeBadMonth(w)
		return
	}
	k, ok := s.ownKlien(w, r, u)
	if !ok {
		return
	}
	s.cachedRead(w, r, u.ID, "laporan:"+k.ID+":"+bulan.Format("2006-01"), func(now time.Time) (any, error) {
		return s.buildLaporan(r.Context(), u, k, bulan, now)
	})
}

// putCatatanLaporan stores the owner's summary or recommendations for one
// month and answers with that month's report.
func (s *Server) putCatatanLaporan(w http.ResponseWriter, r *http.Request, u store.Pengguna) {
	var req struct {
		Bulan string `json:"bulan"`
		Teks  string `json:"teks"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	teks := strings.TrimSpace(req.Teks)
	fields := map[string]string{}
	bulan, ok := s.reportMonth(strings.TrimSpace(req.Bulan))
	if !ok || strings.TrimSpace(req.Bulan) == "" {
		fields["bulan"] = "Tulis bulan dengan format TTTT-BB, misalnya 2026-11."
	}
	if utf8.RuneCountInString(teks) > 2000 {
		fields["teks"] = "Catatan laporan paling panjang 2000 karakter."
	}
	if len(fields) > 0 {
		writeError(w, http.StatusUnprocessableEntity, "Periksa kembali isian Anda.", fields)
		return
	}
	k, ok := s.ownKlien(w, r, u)
	if !ok {
		return
	}
	err := s.Store.SetCatatanLaporan(r.Context(), u.ID, k.ID, bulan, teks)
	if errors.Is(err, store.ErrNotFound) {
		writeKlienNotFound(w)
		return
	}
	if err != nil {
		s.internal(w, r, err)
		return
	}
	s.bump(r.Context(), u.ID)
	rep, err := s.buildLaporan(r.Context(), u, k, bulan, s.Now())
	if err != nil {
		s.internal(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

// createReportLink makes a new read-only link token for a client's report and
// drops the old one. The token is shown only in this response.
func (s *Server) createReportLink(w http.ResponseWriter, r *http.Request, u store.Pengguna) {
	k, ok := s.ownKlien(w, r, u)
	if !ok {
		return
	}
	token, hash, err := auth.NewToken()
	if err != nil {
		s.internal(w, r, err)
		return
	}
	if err := s.Store.SetReportToken(r.Context(), u.ID, k.ID, hash); errors.Is(err, store.ErrNotFound) {
		writeKlienNotFound(w)
		return
	} else if err != nil {
		s.internal(w, r, err)
		return
	}
	s.catat(r.Context(), u.ID, store.AktivitasInput{KlienID: &k.ID, Jenis: "laporan_dibagikan",
		Teks: "Tautan laporan dibuat"})
	s.bump(r.Context(), u.ID)
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusCreated, map[string]string{"jalur": "/laporan/" + token})
}

func (s *Server) deleteReportLink(w http.ResponseWriter, r *http.Request, u store.Pengguna) {
	id, ok := itemID(r)
	if !ok {
		writeKlienNotFound(w)
		return
	}
	err := s.Store.DeleteReportToken(r.Context(), u.ID, id)
	if errors.Is(err, store.ErrNotFound) {
		writeKlienNotFound(w)
		return
	}
	if err != nil {
		s.internal(w, r, err)
		return
	}
	s.bump(r.Context(), u.ID)
	w.WriteHeader(http.StatusNoContent)
}

// publicReport serves a client's report to whoever holds the link: no session,
// the token in the path is the key.
func (s *Server) publicReport(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Robots-Tag", "noindex")
	w.Header().Set("Referrer-Policy", "no-referrer")
	notFound := func() {
		writeError(w, http.StatusNotFound, "Laporan tidak ditemukan atau tautannya sudah dicabut.", nil)
	}
	token := r.PathValue("token")
	if !calendarToken.MatchString(token) {
		notFound()
		return
	}
	bulan, ok := s.reportMonth(r.URL.Query().Get("bulan"))
	if !ok {
		writeBadMonth(w)
		return
	}
	k, owner, err := s.Store.KlienByReportToken(r.Context(), auth.HashToken(token))
	if errors.Is(err, store.ErrNotFound) {
		notFound()
		return
	}
	if err != nil {
		s.internal(w, r, err)
		return
	}
	rep, err := s.buildLaporan(r.Context(), owner, k, bulan, s.Now())
	if err != nil {
		s.internal(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, rep)
}
