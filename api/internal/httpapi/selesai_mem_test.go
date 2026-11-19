package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/mrramdn/tenggat/api/internal/store"
)

func (m *memStore) CloseTugas(_ context.Context, userID, id string, from, at time.Time) (store.Tugas, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.ruang().tugas[id]
	if !ok || t.PenggunaID != userID || !t.JatuhTempo.Equal(from) ||
		t.UlangJumlah != 0 || t.UlangSatuan != "" || t.SelesaiPada != nil {
		return store.Tugas{}, store.ErrNotFound
	}
	t.SelesaiPada, t.TerakhirSelesai, t.Diubah = &at, &at, at
	m.ruang().tugas[id] = t
	t.Klien = m.klienName(t.KlienID)
	return t, nil
}

func TestStubRoutes(t *testing.T) {
	s, _ := newTestServer()
	h := s.Routes()
	c := signUp(t, h, "stub@contoh.id")

	for _, r := range []struct {
		method, path string
	}{
		{"GET", "/api/tugas/00000000-0000-4000-8000-000000000001"},
		{"GET", "/api/agenda"},
		{"GET", "/api/token"},
		{"POST", "/api/token"},
		{"DELETE", "/api/token/00000000-0000-4000-8000-000000000001"},
	} {
		if rec := do(t, h, r.method, r.path, ""); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s without session: %d, want 401", r.method, r.path, rec.Code)
		}
		rec := do(t, h, r.method, r.path, "", c)
		if rec.Code != http.StatusNotImplemented {
			t.Errorf("%s %s with session: %d, want 501", r.method, r.path, rec.Code)
		}
		if e := decode[apiError](t, rec); e.Galat != "Belum tersedia." {
			t.Errorf("%s %s message = %q, want %q", r.method, r.path, e.Galat, "Belum tersedia.")
		}
	}
}

func TestMemStoreV2Contracts(t *testing.T) {
	st := newMemStore()
	ctx := t.Context()
	u, err := st.CreateUser(ctx, "Rana", "rana@contoh.id", "h")
	if err != nil {
		t.Fatal(err)
	}
	k, err := st.CreateKlien(ctx, u.ID, "Mawar", "", "")
	if err != nil {
		t.Fatal(err)
	}

	// Items, PerpanjangItem, Lunas, and closed filter.
	due1 := time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)
	due2 := time.Date(2026, 12, 20, 0, 0, 0, 0, time.UTC)
	costUSD := int64(2050)
	costIDR := int64(300000)
	itUSD, _ := st.Create(ctx, u.ID, store.Input{Nama: "Claude", Jenis: "tagihan", KlienID: &k.ID,
		Kedaluwarsa: due1, Sumber: "manual", Biaya: &costUSD, MataUang: "USD", UlangJumlah: 1, UlangSatuan: "bulan"})
	itIDR, _ := st.Create(ctx, u.ID, store.Input{Nama: "Desain", Jenis: "tagihan", KlienID: &k.ID,
		Kedaluwarsa: due1, Sumber: "manual", Biaya: &costIDR})

	if _, err := st.PerpanjangItem(ctx, u.ID, itUSD.ID, store.Perpanjangan{Dari: due2, Sampai: &due2, At: testNow}); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("PerpanjangItem(wrong Dari) = %v, want ErrNotFound", err)
	}
	moved, err := st.PerpanjangItem(ctx, u.ID, itUSD.ID, store.Perpanjangan{
		Dari: due1, Sampai: &due2, At: testNow,
		Aktivitas: store.AktivitasInput{Jenis: "tagihan_dibayar", Teks: "Bayar", Jumlah: &costUSD},
	})
	if err != nil || !moved.Kedaluwarsa.Equal(due2) {
		t.Fatalf("PerpanjangItem = %+v, %v", moved, err)
	}
	if _, err := st.PerpanjangItem(ctx, u.ID, itIDR.ID, store.Perpanjangan{
		Dari: due1, Lunas: true, At: testNow,
		Aktivitas: store.AktivitasInput{Jenis: "tagihan_dibayar", Teks: "Lunas", Jumlah: &costIDR},
	}); err != nil {
		t.Fatalf("PerpanjangItem(Lunas): %v", err)
	}
	if _, err := st.Update(ctx, u.ID, itIDR.ID, store.Input{Nama: "X", Jenis: "tagihan", Kedaluwarsa: due1}); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("Update(closed item) = %v, want ErrNotFound", err)
	}
	if act, _ := st.List(ctx, u.ID, store.Filter{Selesai: false}, testNow); len(act) != 1 || act[0].ID != itUSD.ID {
		t.Errorf("List(Selesai=false) = %+v", act)
	}
	if cls, _ := st.List(ctx, u.ID, store.Filter{Selesai: true}, testNow); len(cls) != 1 || cls[0].ID != itIDR.ID {
		t.Errorf("List(Selesai=true) = %+v", cls)
	}
	if sum, _ := st.Summary(ctx, u.ID, testNow); sum.Total != 1 {
		t.Errorf("Summary = %+v, want Total=1", sum)
	}
	if kl, _ := st.ListKlien(ctx, u.ID, due1); len(kl) != 1 || kl[0].JumlahAset != 1 || kl[0].Biaya30Hari != 0 || kl[0].Biaya30HariPer["USD"] != 2050 {
		t.Errorf("ListKlien = %+v", kl)
	}

	// Tasks and CloseTugas.
	recT, _ := st.CreateTugas(ctx, u.ID, store.TugasInput{Judul: "Rutin", UlangJumlah: 1, UlangSatuan: "bulan", JatuhTempo: due1})
	one1, _ := st.CreateTugas(ctx, u.ID, store.TugasInput{Judul: "Sekali 1", JatuhTempo: due1})
	one2, _ := st.CreateTugas(ctx, u.ID, store.TugasInput{Judul: "Sekali 2", JatuhTempo: due2})
	if _, err := st.CloseTugas(ctx, u.ID, recT.ID, due1, testNow); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("CloseTugas(recurring) = %v, want ErrNotFound", err)
	}
	if _, err := st.CloseTugas(ctx, u.ID, one1.ID, due1, testNow); err != nil {
		t.Fatalf("CloseTugas(one1): %v", err)
	}
	if _, err := st.CloseTugas(ctx, u.ID, one2.ID, due2, testNow.Add(time.Hour)); err != nil {
		t.Fatalf("CloseTugas(one2): %v", err)
	}
	if _, err := st.CloseTugas(ctx, u.ID, one1.ID, due1, testNow); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("second CloseTugas = %v, want ErrNotFound", err)
	}
	if clsT, _ := st.ListTugas(ctx, u.ID, store.TugasFilter{Selesai: true}); len(clsT) != 2 || clsT[0].ID != one2.ID {
		t.Errorf("ListTugas(Selesai=true) = %+v", clsT)
	}

	// Tokens: 20 limit, expiry, and once-a-minute terakhir_dipakai.
	for i := range store.MaxTokensPerUser {
		if _, err := st.CreateToken(ctx, u.ID, store.TokenInput{
			Nama: "T" + strconv.Itoa(i), Hash: "h" + strconv.Itoa(i), Awalan: "tgt_0000", Cakupan: "baca",
		}); err != nil {
			t.Fatalf("CreateToken #%d: %v", i, err)
		}
	}
	if _, err := st.CreateToken(ctx, u.ID, store.TokenInput{Nama: "Over", Hash: "hover", Awalan: "tgt_over", Cakupan: "baca"}); !errors.Is(err, store.ErrTokenLimit) {
		t.Errorf("21st CreateToken = %v, want ErrTokenLimit", err)
	}
	_, tok, err := st.UserByToken(ctx, "h0", testNow)
	if err != nil || tok.TerakhirDipakai == nil || !tok.TerakhirDipakai.Equal(testNow) {
		t.Fatalf("UserByToken = %+v, %v", tok, err)
	}
	if _, tok, _ = st.UserByToken(ctx, "h0", testNow.Add(30*time.Second)); !tok.TerakhirDipakai.Equal(testNow) {
		t.Errorf("UserByToken(+30s) updated TerakhirDipakai = %v", tok.TerakhirDipakai)
	}
	if _, tok, _ = st.UserByToken(ctx, "h0", testNow.Add(time.Minute)); !tok.TerakhirDipakai.Equal(testNow.Add(time.Minute)) {
		t.Errorf("UserByToken(+1m) TerakhirDipakai = %v", tok.TerakhirDipakai)
	}
}
