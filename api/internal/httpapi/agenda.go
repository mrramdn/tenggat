package httpapi

import (
	"cmp"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mrramdn/tenggat/api/internal/expiry"
	"github.com/mrramdn/tenggat/api/internal/kurs"
	"github.com/mrramdn/tenggat/api/internal/store"
	"github.com/mrramdn/tenggat/api/internal/uang"
)

const (
	agendaDefaultBatas = 200
	agendaMaxBatas     = 500
	agendaMaxDays      = 3660
	agendaMaxQuery     = 100
)

var agendaJenisValid = map[string]bool{
	"sertifikat": true,
	"domain":     true,
	"token":      true,
	"langganan":  true,
	"tagihan":    true,
	"lainnya":    true,
	"tugas":      true,
}

// agendaFilter narrows the merged agenda. Zero fields mean no filter.
type agendaFilter struct {
	DalamHari *int
	KlienID   string
	Pribadi   bool
	Status    expiry.Status
	Jenis     []string
	Q         string
}

type agendaUlang struct {
	Jumlah int    `json:"jumlah"`
	Satuan string `json:"satuan"`
}

type agendaAksi struct {
	Kode   string `json:"kode"`
	Label  string `json:"label"`
	Method string `json:"method"`
	Jalur  string `json:"jalur"`
}

// agendaRow is one active item or task in the merged agenda.
type agendaRow struct {
	Asal     string        `json:"asal"`
	ID       string        `json:"id"`
	Judul    string        `json:"judul"`
	Jenis    string        `json:"jenis"`
	Sekali   bool          `json:"sekali"`
	KlienID  *string       `json:"klien_id"`
	Klien    string        `json:"klien"`
	Tanggal  string        `json:"tanggal"`
	SisaHari int           `json:"sisa_hari"`
	Status   expiry.Status `json:"status"`
	Biaya    *json.Number  `json:"biaya"`
	MataUang string        `json:"mata_uang"`
	KiraIDR  *int64        `json:"kira_idr"`
	Ulang    *agendaUlang  `json:"ulang"`
	Aksi     *agendaAksi   `json:"aksi"`

	date     time.Time
	biayaRaw *int64
}

// agendaJumlah counts every row matching the filters, before batas trims the slice.
type agendaJumlah struct {
	Lewat  int `json:"lewat"`
	Kritis int `json:"kritis"`
	Segera int `json:"segera"`
	Aman   int `json:"aman"`
	Total  int `json:"total"`
}

type agendaResponse struct {
	Agenda       []agendaRow  `json:"agenda"`
	Jumlah       agendaJumlah `json:"jumlah"`
	Lagi         bool         `json:"lagi"`
	KiraIDRTotal *int64       `json:"kira_idr_total"`
	KursTanggal  *string      `json:"kurs_tanggal"`
}

func (s *Server) getAgenda(w http.ResponseWriter, r *http.Request, u store.Pengguna) {
	f, batas, fields := parseAgendaQuery(r.URL.Query())
	if len(fields) > 0 {
		writeError(w, http.StatusUnprocessableEntity, "Periksa kembali parameter Anda.", fields)
		return
	}
	key := "agenda:" + canonicalAgendaQuery(f, batas)
	s.cachedRead(w, r, u.ID, key, func(now time.Time) (any, error) {
		rows, jumlah, err := s.buildAgenda(r.Context(), u.ID, now, f)
		if err != nil {
			return nil, err
		}
		rate, hasRate := s.kursKini(r.Context())
		var kursTanggal *string
		if hasRate {
			t := rate.Tanggal
			kursTanggal = &t
		}
		kiraTotal := applyAgendaEstimates(rows, rate)
		lagi := len(rows) > batas
		if lagi {
			rows = rows[:batas]
		}
		return agendaResponse{
			Agenda:       rows,
			Jumlah:       jumlah,
			Lagi:         lagi,
			KiraIDRTotal: kiraTotal,
			KursTanggal:  kursTanggal,
		}, nil
	})
}

// applyAgendaEstimates fills each priced row's kira_idr and returns their sum,
// or nil when any priced row has no estimate (for example, foreign currency
// with rates unavailable).
func applyAgendaEstimates(rows []agendaRow, rate kurs.Kurs) *int64 {
	var total int64
	missing := false
	for i := range rows {
		if rows[i].biayaRaw == nil {
			continue
		}
		est, ok := rate.KeRupiah(*rows[i].biayaRaw, rows[i].MataUang)
		if !ok {
			missing = true
			continue
		}
		rows[i].KiraIDR = &est
		total += est
	}
	if missing {
		return nil
	}
	return &total
}

func parseAgendaQuery(q url.Values) (agendaFilter, int, map[string]string) {
	f := agendaFilter{
		KlienID: q.Get("klien_id"),
		Status:  expiry.Status(q.Get("status")),
		Q:       strings.TrimSpace(q.Get("q")),
	}
	batas := agendaDefaultBatas
	fields := map[string]string{}

	if v := q.Get("dalam_hari"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 || n > agendaMaxDays {
			fields["dalam_hari"] = "Isi jumlah hari 0 sampai 3660."
		} else {
			f.DalamHari = &n
		}
	}
	if f.KlienID != "" && !uuidPattern.MatchString(f.KlienID) {
		fields["klien_id"] = "Klien tidak ditemukan."
	}
	if v := q.Get("pribadi"); v != "" {
		switch v {
		case "1":
			f.Pribadi = true
		case "0":
			f.Pribadi = false
		default:
			fields["pribadi"] = "Gunakan 0 atau 1."
		}
	}
	if f.Pribadi && f.KlienID != "" {
		fields["pribadi"] = "Tidak dapat digabung dengan klien_id."
	}
	if f.Status != "" && !f.Status.Valid() {
		fields["status"] = "Gunakan lewat, kritis, segera, atau aman."
	}
	if v := q.Get("jenis"); v != "" {
		seen := map[string]bool{}
		bad := false
		for part := range strings.SplitSeq(v, ",") {
			p := strings.TrimSpace(part)
			if !agendaJenisValid[p] {
				bad = true
				break
			}
			if !seen[p] {
				seen[p] = true
				f.Jenis = append(f.Jenis, p)
			}
		}
		if bad {
			fields["jenis"] = "Pilih jenis: sertifikat, domain, token, langganan, tagihan, lainnya, atau tugas."
		} else {
			slices.Sort(f.Jenis)
		}
	}
	if utf8.RuneCountInString(f.Q) > agendaMaxQuery {
		fields["q"] = "Pencarian maksimal 100 karakter."
	}
	if v := q.Get("batas"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > agendaMaxBatas {
			fields["batas"] = "Batas harus 1 sampai 500."
		} else {
			batas = n
		}
	}
	return f, batas, fields
}

// canonicalAgendaQuery builds a collision-free cache key suffix where
// equivalent queries (different parameter order, duplicate jenis, case in q,
// or explicit default batas) map to the same key.
func canonicalAgendaQuery(f agendaFilter, batas int) string {
	v := url.Values{}
	if f.DalamHari != nil {
		v.Set("dalam_hari", strconv.Itoa(*f.DalamHari))
	}
	if f.KlienID != "" {
		v.Set("klien_id", f.KlienID)
	}
	if f.Pribadi {
		v.Set("pribadi", "1")
	}
	if f.Status != "" {
		v.Set("status", string(f.Status))
	}
	if len(f.Jenis) > 0 {
		v.Set("jenis", strings.Join(f.Jenis, ","))
	}
	if f.Q != "" {
		v.Set("q", strings.ToLower(f.Q))
	}
	v.Set("batas", strconv.Itoa(batas))
	return v.Encode()
}

// buildAgenda merges a user's active items and active tasks matching f, sorted
// by date and then title, together with counts across all matching rows.
func (s *Server) buildAgenda(ctx context.Context, userID string, now time.Time, f agendaFilter) ([]agendaRow, agendaJumlah, error) {
	wantItems, wantTasks := agendaSources(f.Jenis)
	needle := strings.ToLower(strings.TrimSpace(f.Q))

	rows := []agendaRow{}
	if wantItems {
		items, err := s.Store.List(ctx, userID, store.Filter{Status: f.Status, KlienID: f.KlienID}, now)
		if err != nil {
			return nil, agendaJumlah{}, err
		}
		for _, it := range items {
			if f.Pribadi && it.KlienID != nil {
				continue
			}
			if len(f.Jenis) > 0 && !slices.Contains(f.Jenis, it.Jenis) {
				continue
			}
			days := expiry.DaysLeft(it.Kedaluwarsa, now)
			if f.DalamHari != nil && days > *f.DalamHari {
				continue
			}
			st := expiry.For(days)
			if f.Status != "" && st != f.Status {
				continue
			}
			if needle != "" && !matchesAgendaQuery(needle, it.Nama, it.Klien, it.Catatan, deref(it.Host)) {
				continue
			}
			rows = append(rows, itemAgendaRow(it, days, st))
		}
	}

	if wantTasks {
		tf := store.TugasFilter{KlienID: f.KlienID}
		if f.DalamHari != nil {
			sampai := expiry.DateOf(now).AddDate(0, 0, *f.DalamHari)
			tf.Sampai = &sampai
		}
		tasks, err := s.Store.ListTugas(ctx, userID, tf)
		if err != nil {
			return nil, agendaJumlah{}, err
		}
		for _, t := range tasks {
			if f.Pribadi && t.KlienID != nil {
				continue
			}
			days := expiry.DaysLeft(t.JatuhTempo, now)
			if f.DalamHari != nil && days > *f.DalamHari {
				continue
			}
			st := expiry.For(days)
			if f.Status != "" && st != f.Status {
				continue
			}
			if needle != "" && !matchesAgendaQuery(needle, t.Judul, t.Klien, t.Catatan) {
				continue
			}
			rows = append(rows, tugasAgendaRow(t, days, st))
		}
	}

	slices.SortFunc(rows, func(a, b agendaRow) int {
		return cmp.Or(
			a.date.Compare(b.date),
			strings.Compare(strings.ToLower(a.Judul), strings.ToLower(b.Judul)),
			strings.Compare(a.Judul, b.Judul),
			strings.Compare(a.ID, b.ID),
		)
	})

	var jumlah agendaJumlah
	for _, row := range rows {
		switch row.Status {
		case expiry.Lewat:
			jumlah.Lewat++
		case expiry.Kritis:
			jumlah.Kritis++
		case expiry.Segera:
			jumlah.Segera++
		default:
			jumlah.Aman++
		}
		jumlah.Total++
	}
	return rows, jumlah, nil
}

func agendaSources(jenis []string) (items, tasks bool) {
	if len(jenis) == 0 {
		return true, true
	}
	for _, j := range jenis {
		if j == "tugas" {
			tasks = true
		} else {
			items = true
		}
	}
	return items, tasks
}

func matchesAgendaQuery(needle string, fields ...string) bool {
	for _, field := range fields {
		if field != "" && strings.Contains(strings.ToLower(field), needle) {
			return true
		}
	}
	return false
}

func itemAgendaRow(it store.Item, days int, st expiry.Status) agendaRow {
	mu := it.MataUang
	if mu == "" {
		mu = "IDR"
	}
	var biaya *json.Number
	if it.Biaya != nil {
		n := uang.JSON(*it.Biaya, mu)
		biaya = &n
	}
	var ulang *agendaUlang
	if it.UlangJumlah > 0 && it.UlangSatuan != "" {
		ulang = &agendaUlang{Jumlah: it.UlangJumlah, Satuan: it.UlangSatuan}
	}
	return agendaRow{
		Asal:     "item",
		ID:       it.ID,
		Judul:    it.Nama,
		Jenis:    it.Jenis,
		Sekali:   false,
		KlienID:  it.KlienID,
		Klien:    it.Klien,
		Tanggal:  it.Kedaluwarsa.Format(time.DateOnly),
		SisaHari: days,
		Status:   st,
		Biaya:    biaya,
		MataUang: mu,
		Ulang:    ulang,
		Aksi:     itemAgendaAksi(it),
		date:     it.Kedaluwarsa,
		biayaRaw: it.Biaya,
	}
}

func itemAgendaAksi(it store.Item) *agendaAksi {
	switch {
	case it.Jenis == "tagihan":
		return &agendaAksi{Kode: "bayar", Label: "Tandai dibayar", Method: http.MethodPost, Jalur: "/api/item/" + it.ID + "/perpanjang"}
	case it.Jenis == "sertifikat" || it.Sumber == "registri":
		return &agendaAksi{Kode: "cek", Label: "Cek ulang", Method: http.MethodPost, Jalur: "/api/item/" + it.ID + "/cek"}
	case it.Sumber == "manual" && it.UlangJumlah > 0 && it.UlangSatuan != "":
		return &agendaAksi{Kode: "perpanjang", Label: "Perpanjang", Method: http.MethodPost, Jalur: "/api/item/" + it.ID + "/perpanjang"}
	default:
		return nil
	}
}

func tugasAgendaRow(t store.Tugas, days int, st expiry.Status) agendaRow {
	mu := t.MataUang
	if mu == "" {
		mu = "IDR"
	}
	var biaya *json.Number
	if t.Biaya != nil {
		n := uang.JSON(*t.Biaya, mu)
		biaya = &n
	}
	var ulang *agendaUlang
	recurring := t.UlangJumlah > 0 && t.UlangSatuan != ""
	if recurring {
		ulang = &agendaUlang{Jumlah: t.UlangJumlah, Satuan: t.UlangSatuan}
	}
	return agendaRow{
		Asal:     "tugas",
		ID:       t.ID,
		Judul:    t.Judul,
		Jenis:    "tugas",
		Sekali:   !recurring,
		KlienID:  t.KlienID,
		Klien:    t.Klien,
		Tanggal:  t.JatuhTempo.Format(time.DateOnly),
		SisaHari: days,
		Status:   st,
		Biaya:    biaya,
		MataUang: mu,
		Ulang:    ulang,
		Aksi:     &agendaAksi{Kode: "selesai", Label: "Tandai selesai", Method: http.MethodPost, Jalur: "/api/tugas/" + t.ID + "/selesai"},
		date:     t.JatuhTempo,
		biayaRaw: t.Biaya,
	}
}
