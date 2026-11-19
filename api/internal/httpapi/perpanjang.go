package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mrramdn/tenggat/api/internal/expiry"
	"github.com/mrramdn/tenggat/api/internal/store"
	"github.com/mrramdn/tenggat/api/internal/uang"
)

// PerpanjangStore is the part of Store that records item renewals and bill payments.
type PerpanjangStore interface {
	PerpanjangItem(ctx context.Context, userID, id string, p store.Perpanjangan) (store.Item, error)
}

// perpanjangRequest is the optional body of POST /api/item/{id}/perpanjang.
type perpanjangRequest struct {
	Dari     string       `json:"dari"`
	Sampai   string       `json:"sampai"`
	Jumlah   *json.Number `json:"jumlah"`
	MataUang string       `json:"mata_uang"`
	Catatan  string       `json:"catatan"`
	Tanggal  string       `json:"tanggal"`
	// SampaiKini moves a recurring item to its first date after today instead of
	// one period on: "I am paid up", for a bill left unticked for several periods.
	SampaiKini bool `json:"sampai_kini"`
}

// perpanjangItem pays a bill or renews a manual asset, moving its date forward
// by one period (or closing a one-off bill) and logging the activity entry
// atomically through Store.PerpanjangItem.
func (s *Server) perpanjangItem(w http.ResponseWriter, r *http.Request, u store.Pengguna) {
	id, ok := itemID(r)
	if !ok {
		writeNotFound(w)
		return
	}
	var req perpanjangRequest
	if !decodeOptionalJSON(w, r, &req) {
		return
	}
	old, err := s.Store.Get(r.Context(), u.ID, id)
	if errors.Is(err, store.ErrNotFound) {
		writeNotFound(w)
		return
	}
	if err != nil {
		s.internal(w, r, err)
		return
	}
	if old.SelesaiPada != nil {
		writeError(w, http.StatusConflict, "Aset ini sudah ditutup.", nil)
		return
	}
	if old.Sumber != "manual" {
		writeError(w, http.StatusUnprocessableEntity,
			"Sertifikat dan domain dari registri diperbarui lewat cek ulang.", nil)
		return
	}

	fields := map[string]string{}
	if v := strings.TrimSpace(req.Dari); v != "" {
		d, err := time.Parse(time.DateOnly, v)
		if err != nil {
			fields["dari"] = "Isi tanggal dengan format TTTT-BB-HH."
		} else if !d.Equal(old.Kedaluwarsa) {
			writeError(w, http.StatusConflict, "Aset ini baru saja diubah. Muat ulang lalu coba lagi.", nil)
			return
		}
	}

	var sampai *time.Time
	var lunas bool
	if v := strings.TrimSpace(req.Sampai); v != "" {
		d, err := time.Parse(time.DateOnly, v)
		switch {
		case err != nil:
			fields["sampai"] = "Isi tanggal berakhir dengan format TTTT-BB-HH."
		case d.Year() < 2000 || d.Year() > 2100:
			fields["sampai"] = "Tanggal berakhir di luar rentang yang masuk akal."
		case !d.After(old.Kedaluwarsa):
			fields["sampai"] = "Tanggal berakhir yang baru harus setelah tanggal saat ini."
		default:
			sampai = &d
		}
	} else if old.UlangJumlah > 0 && old.UlangSatuan != "" {
		next := expiry.AddInterval(old.Kedaluwarsa, old.UlangJumlah, old.UlangSatuan)
		if req.SampaiKini {
			next = expiry.NextDue(old.Kedaluwarsa, expiry.DateOf(s.Now()), old.UlangJumlah, old.UlangSatuan)
		}
		sampai = &next
	} else if old.Jenis == "tagihan" {
		lunas = true
	} else {
		fields["sampai"] = "Isi tanggal berakhir yang baru."
	}

	if req.SampaiKini && (strings.TrimSpace(req.Sampai) != "" || old.UlangJumlah == 0) {
		fields["sampai_kini"] = "Hanya untuk yang berulang, dan tanpa tanggal baru."
	}

	mu := strings.TrimSpace(req.MataUang)
	if mu == "" {
		mu = old.MataUang
		if mu == "" {
			mu = "IDR"
		}
	} else if !uang.Valid(mu) {
		fields["mata_uang"] = "Pilih IDR atau USD."
	}
	jumlah := old.Biaya
	if req.Jumlah != nil {
		parseMu := mu
		if !uang.Valid(parseMu) {
			parseMu = "IDR"
		}
		minor, err := uang.Parse(*req.Jumlah, parseMu)
		if err != nil {
			fields["jumlah"] = "Biaya harus angka 0 atau lebih; rupiah tanpa desimal, USD paling banyak 2 desimal."
		} else {
			jumlah = &minor
		}
	}

	catatan := strings.TrimSpace(req.Catatan)
	if utf8.RuneCountInString(catatan) > 500 {
		fields["catatan"] = "Catatan maksimal 500 karakter."
	}
	tanggal := s.workDate(req.Tanggal, fields)
	if len(fields) > 0 {
		writeError(w, http.StatusUnprocessableEntity, "Periksa kembali isian Anda.", fields)
		return
	}

	jenisAkt := "aset_diperpanjang"
	var teks string
	if old.Jenis == "tagihan" {
		jenisAkt = "tagihan_dibayar"
		teks = denganJenis("Tagihan", old.Nama) + " dibayar"
		if jumlah != nil {
			teks += " " + uang.Format(*jumlah, mu)
		}
		if lunas {
			teks += ", lunas"
		} else {
			teks += ", berikutnya " + tanggalID(*sampai)
		}
	} else {
		teks = old.Nama + " diperpanjang sampai " + tanggalID(*sampai)
	}

	extra := map[string]any{"dari": old.Kedaluwarsa.Format(time.DateOnly), "lunas": lunas}
	if sampai != nil {
		extra["sampai"] = sampai.Format(time.DateOnly)
	}
	if catatan != "" {
		extra["catatan"] = catatan
	}
	if req.SampaiKini {
		extra["sampai_kini"] = true
	}
	var aktMataUang *string
	if jumlah != nil {
		aktMataUang = &mu
	}
	now := s.Now()
	it, err := s.Store.PerpanjangItem(r.Context(), u.ID, id, store.Perpanjangan{
		Dari:   old.Kedaluwarsa,
		Sampai: sampai,
		Lunas:  lunas,
		At:     now,
		Aktivitas: store.AktivitasInput{
			KlienID:  old.KlienID,
			ItemID:   &old.ID,
			Jenis:    jenisAkt,
			Teks:     teks,
			Tanggal:  tanggal,
			Jumlah:   jumlah,
			MataUang: aktMataUang,
			Data:     data(extra),
		},
	})
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusConflict, "Aset ini baru saja diubah. Muat ulang lalu coba lagi.", nil)
		return
	}
	if err != nil {
		s.internal(w, r, err)
		return
	}
	s.bump(r.Context(), u.ID)
	writeJSON(w, http.StatusOK, toJSON(it, now))
}
