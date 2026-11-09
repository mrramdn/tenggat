package httpapi

import (
	"net/http"
	"strings"
	"testing"

	"github.com/mrramdn/tenggat/api/internal/rdap"
	"github.com/mrramdn/tenggat/api/internal/tlscheck"
)

type tugasList struct{ Tugas []tugasJSON }

type aktivitasList struct {
	Aktivitas []aktivitasJSON
	Lagi      bool
}

func TestTugasCreateListUpdateDelete(t *testing.T) {
	s, st := newTestServer()
	h := s.Routes()
	c := signUp(t, h, "tugas@contoh.id")

	rec := do(t, h, "POST", "/api/tugas", `{"judul":" Backup bulanan ","klien":"Studio Rana","jatuh_tempo":"2026-11-12","biaya":150000}`, c)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body)
	}
	tg := decode[tugasJSON](t, rec)
	if tg.Judul != "Backup bulanan" || tg.Klien != "Studio Rana" || tg.KlienID == nil || tg.UlangJumlah != 1 ||
		tg.UlangSatuan != "bulan" || tg.SisaHari != 3 || tg.Status != "kritis" || tg.TerakhirSelesai != nil {
		t.Errorf("created %+v", tg)
	}
	if loc := rec.Header().Get("Location"); loc != "/api/tugas/"+tg.ID {
		t.Errorf("Location = %q", loc)
	}
	do(t, h, "POST", "/api/tugas", `{"judul":"Perbarui plugin","klien_id":"`+*tg.KlienID+`","ulang_jumlah":2,"ulang_satuan":"minggu","jatuh_tempo":"2027-03-01"}`, c)
	do(t, h, "POST", "/api/tugas", `{"judul":"Cek uptime","jatuh_tempo":"2026-10-01","ulang_satuan":"hari"}`, c)

	list := decode[tugasList](t, do(t, h, "GET", "/api/tugas", "", c)).Tugas
	if len(list) != 3 || list[0].Judul != "Cek uptime" || list[0].Status != "lewat" || list[2].Judul != "Perbarui plugin" {
		t.Fatalf("list %+v", list)
	}
	if got := decode[tugasList](t, do(t, h, "GET", "/api/tugas?dalam_hari=14", "", c)).Tugas; len(got) != 2 {
		t.Errorf("dalam_hari=14: %d tasks, want the overdue one and the one due in 3 days", len(got))
	}
	if got := decode[tugasList](t, do(t, h, "GET", "/api/tugas?klien_id="+*tg.KlienID, "", c)).Tugas; len(got) != 2 {
		t.Errorf("klien filter: %d", len(got))
	}
	for _, q := range []string{"dalam_hari=-1", "dalam_hari=x", "klien_id=bukan"} {
		if rec := do(t, h, "GET", "/api/tugas?"+q, "", c); rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("GET ?%s: %d", q, rec.Code)
		}
	}

	rec = do(t, h, "PUT", "/api/tugas/"+tg.ID, `{"judul":"Backup mingguan","ulang_satuan":"minggu","jatuh_tempo":"2026-11-16"}`, c)
	up := decode[tugasJSON](t, rec)
	if rec.Code != http.StatusOK || up.Judul != "Backup mingguan" || up.KlienID != nil || up.UlangSatuan != "minggu" {
		t.Errorf("update: %d %+v", rec.Code, up)
	}

	if rec := do(t, h, "DELETE", "/api/tugas/"+tg.ID, "", c); rec.Code != http.StatusNoContent {
		t.Errorf("delete: %d", rec.Code)
	}
	if rec := do(t, h, "DELETE", "/api/tugas/"+tg.ID, "", c); rec.Code != http.StatusNotFound {
		t.Errorf("delete twice: %d", rec.Code)
	}
	if rec := do(t, h, "PUT", "/api/tugas/bukan-id", `{}`, c); rec.Code != http.StatusNotFound {
		t.Errorf("bad id: %d", rec.Code)
	}

	baru, dihapus := st.aktivitasOf("tugas_baru"), st.aktivitasOf("tugas_dihapus")
	if len(baru) != 3 || baru[0].Teks != "Tugas baru: Backup bulanan, jatuh tempo 12 Nov 2026" || deref(baru[0].TugasID) != tg.ID ||
		deref(baru[0].KlienID) != *tg.KlienID {
		t.Errorf("tugas_baru entries %+v", baru)
	}
	if len(dihapus) != 1 || dihapus[0].Teks != "Tugas dihapus: Backup mingguan" || dihapus[0].TugasID != nil ||
		!strings.Contains(string(dihapus[0].Data), tg.ID) {
		t.Errorf("tugas_dihapus entries %+v", dihapus)
	}
}

func TestTugasValidation(t *testing.T) {
	s, _ := newTestServer()
	h := s.Routes()
	c := signUp(t, h, "tugas@contoh.id")
	for body, field := range map[string]string{
		`{"judul":"A","jatuh_tempo":"2026-12-01"}`:                                                        "judul",
		`{"judul":"Backup"}`:                                                                              "jatuh_tempo",
		`{"judul":"Backup","jatuh_tempo":"01-12-2026"}`:                                                   "jatuh_tempo",
		`{"judul":"Backup","jatuh_tempo":"1999-12-01"}`:                                                   "jatuh_tempo",
		`{"judul":"Backup","jatuh_tempo":"2026-12-01","ulang_jumlah":0}`:                                  "ulang_jumlah",
		`{"judul":"Backup","jatuh_tempo":"2026-12-01","ulang_jumlah":366}`:                                "ulang_jumlah",
		`{"judul":"Backup","jatuh_tempo":"2026-12-01","ulang_satuan":"jam"}`:                              "ulang_satuan",
		`{"judul":"Backup","jatuh_tempo":"2026-12-01","biaya":-1}`:                                        "biaya",
		`{"judul":"Backup","jatuh_tempo":"2026-12-01","catatan":"` + strings.Repeat("x", 501) + `"}`:      "catatan",
		`{"judul":"Backup","jatuh_tempo":"2026-12-01","klien_id":"00000000-0000-4000-8000-000000000999"}`: "klien",
	} {
		rec := do(t, h, "POST", "/api/tugas", body, c)
		if e := decode[apiError](t, rec); rec.Code != http.StatusUnprocessableEntity || e.Field[field] == "" {
			t.Errorf("%s: %d %v, want 422 on %s", body, rec.Code, e.Field, field)
		}
	}
	if rec := do(t, h, "POST", "/api/tugas", `{"judul":"Backup","jatuh_tempo":"2026-12-01","lain":1}`, c); rec.Code != http.StatusBadRequest {
		t.Errorf("unknown field: %d", rec.Code)
	}
}

func TestTugasSelesai(t *testing.T) {
	s, st := newTestServer()
	h := s.Routes()
	c := signUp(t, h, "tugas@contoh.id")
	// Due on 31 Aug, monthly, long overdue on 9 Nov: the next date is 30 Nov.
	tg := decode[tugasJSON](t, do(t, h, "POST", "/api/tugas", `{"judul":"Backup","klien":"Apotek Sehat","jatuh_tempo":"2026-08-31"}`, c))

	rec := do(t, h, "POST", "/api/tugas/"+tg.ID+"/selesai", `{"catatan":"Backup ke S3, 2 GB","durasi_menit":30,"tanggal":"2026-11-08"}`, c)
	done := decode[tugasJSON](t, rec)
	if rec.Code != http.StatusOK || done.JatuhTempo != "2026-11-30" || done.TerakhirSelesai == nil || !done.TerakhirSelesai.Equal(testNow) {
		t.Fatalf("selesai: %d %+v", rec.Code, done)
	}
	got := st.aktivitasOf("tugas_selesai")
	if len(got) != 1 {
		t.Fatalf("tugas_selesai entries: %d", len(got))
	}
	a := got[0]
	if a.Teks != "Selesai: Backup" || a.Tanggal.Format("2006-01-02") != "2026-11-08" || a.DurasiMenit == nil || *a.DurasiMenit != 30 ||
		deref(a.TugasID) != tg.ID || deref(a.KlienID) != *tg.KlienID ||
		string(a.Data) != `{"catatan":"Backup ke S3, 2 GB","jatuh_tempo_baru":"2026-11-30","jatuh_tempo_lama":"2026-08-31"}` {
		t.Errorf("entry %+v data %s", a, a.Data)
	}

	// No body at all is fine; the date moves one more month from 30 Nov.
	rec = do(t, h, "POST", "/api/tugas/"+tg.ID+"/selesai", "", c)
	if rec.Code != http.StatusOK || decode[tugasJSON](t, rec).JatuhTempo != "2026-12-30" {
		t.Errorf("selesai without body: %d %s", rec.Code, rec.Body)
	}
	for body, field := range map[string]string{
		`{"durasi_menit":0}`:                             "durasi_menit",
		`{"durasi_menit":1441}`:                          "durasi_menit",
		`{"tanggal":"2026-11-10"}`:                       "tanggal",
		`{"tanggal":"kemarin"}`:                          "tanggal",
		`{"catatan":"` + strings.Repeat("x", 501) + `"}`: "catatan",
	} {
		rec := do(t, h, "POST", "/api/tugas/"+tg.ID+"/selesai", body, c)
		if e := decode[apiError](t, rec); rec.Code != http.StatusUnprocessableEntity || e.Field[field] == "" {
			t.Errorf("selesai %s: %d %v", body, rec.Code, e.Field)
		}
	}
	if rec := do(t, h, "POST", "/api/tugas/"+tg.ID+"/selesai", `{"x":1}`, c); rec.Code != http.StatusBadRequest {
		t.Errorf("unknown field: %d", rec.Code)
	}
}

func TestTugasAndAktivitasArePerUser(t *testing.T) {
	s, _ := newTestServer()
	h := s.Routes()
	a := signUp(t, h, "a@contoh.id")
	b := signUp(t, h, "b@contoh.id")
	tg := decode[tugasJSON](t, do(t, h, "POST", "/api/tugas", `{"judul":"Backup","klien":"Klien A","jatuh_tempo":"2026-12-01"}`, a))
	note := decode[aktivitasJSON](t, do(t, h, "POST", "/api/aktivitas", `{"klien_id":"`+*tg.KlienID+`","jenis":"catatan","teks":"Rahasia A"}`, a))

	for _, r := range []struct{ method, path, body string }{
		{"PUT", "/api/tugas/" + tg.ID, `{"judul":"Dicuri","jatuh_tempo":"2027-01-01"}`},
		{"DELETE", "/api/tugas/" + tg.ID, ""},
		{"POST", "/api/tugas/" + tg.ID + "/selesai", ""},
		{"DELETE", "/api/aktivitas/" + note.ID, ""},
	} {
		if rec := do(t, h, r.method, r.path, r.body, b); rec.Code != http.StatusNotFound {
			t.Errorf("user B %s %s: %d, want 404", r.method, r.path, rec.Code)
		}
	}
	// B cannot file entries or tasks under A's client.
	rec := do(t, h, "POST", "/api/aktivitas", `{"klien_id":"`+*tg.KlienID+`","jenis":"catatan","teks":"Halo"}`, b)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("B notes on A's client: %d", rec.Code)
	}
	rec = do(t, h, "POST", "/api/tugas", `{"judul":"Backup","klien_id":"`+*tg.KlienID+`","jatuh_tempo":"2026-12-01"}`, b)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("B task on A's client: %d", rec.Code)
	}
	if got := decode[tugasList](t, do(t, h, "GET", "/api/tugas", "", b)).Tugas; len(got) != 0 {
		t.Errorf("B sees %d tasks", len(got))
	}
	if got := decode[aktivitasList](t, do(t, h, "GET", "/api/aktivitas", "", b)).Aktivitas; len(got) != 0 {
		t.Errorf("B sees %d entries", len(got))
	}
	if got := decode[aktivitasList](t, do(t, h, "GET", "/api/aktivitas?klien_id="+*tg.KlienID, "", b)).Aktivitas; len(got) != 0 {
		t.Errorf("B sees %d entries of A's client", len(got))
	}
	for _, path := range []string{"/api/tugas", "/api/aktivitas"} {
		if rec := do(t, h, "GET", path, ""); rec.Code != http.StatusUnauthorized {
			t.Errorf("GET %s without session: %d", path, rec.Code)
		}
	}
}

func TestAutomaticAssetEntries(t *testing.T) {
	s, st, reg := newRegistryServer()
	h := s.Routes()
	c := signUp(t, h, "aset@contoh.id")

	vps := decode[itemJSON](t, do(t, h, "POST", "/api/item", `{"nama":"VPS","jenis":"langganan","klien":"Studio Rana","kedaluwarsa":"2026-11-20"}`, c))
	// An edit that keeps or moves the date earlier is no renewal; one that moves it later is.
	do(t, h, "PUT", "/api/item/"+vps.ID, `{"nama":"VPS","jenis":"langganan","klien":"Studio Rana","kedaluwarsa":"2026-11-19"}`, c)
	do(t, h, "PUT", "/api/item/"+vps.ID, `{"nama":"VPS","jenis":"langganan","klien":"Studio Rana","kedaluwarsa":"2027-11-14"}`, c)
	do(t, h, "DELETE", "/api/item/"+vps.ID, "", c)

	if got := st.aktivitasOf("aset_baru"); len(got) != 1 || got[0].Teks != "VPS ditambahkan, berakhir 20 Nov 2026" ||
		deref(got[0].ItemID) != vps.ID || deref(got[0].KlienID) != deref(vps.KlienID) {
		t.Errorf("aset_baru %+v", got)
	}
	if got := st.aktivitasOf("aset_diperpanjang"); len(got) != 1 || got[0].Teks != "VPS diperpanjang sampai 14 Nov 2027" ||
		string(got[0].Data) != `{"dari":"2026-11-19","sampai":"2027-11-14"}` {
		t.Errorf("aset_diperpanjang %+v", got)
	}
	if got := st.aktivitasOf("aset_dihapus"); len(got) != 1 || got[0].Teks != "VPS dihapus" || got[0].ItemID != nil ||
		deref(got[0].KlienID) != deref(vps.KlienID) {
		t.Errorf("aset_dihapus %+v", got)
	}

	// A registry domain renewed outside Tenggat: the recheck finds the later date.
	dom := decode[itemJSON](t, do(t, h, "POST", "/api/item", `{"jenis":"domain","host":"toko.co.id"}`, c))
	reg.results["toko.co.id"] = domainResult("toko.co.id", "2028-02-01", "PT Registrar Uji")
	if rec := do(t, h, "POST", "/api/item/"+dom.ID+"/cek", "", c); rec.Code != http.StatusOK {
		t.Fatalf("recheck: %d %s", rec.Code, rec.Body)
	}
	do(t, h, "POST", "/api/item/"+dom.ID+"/cek", "", c) // same date again: nothing new
	if got := st.aktivitasOf("cek_diperbarui"); len(got) != 1 ||
		got[0].Teks != "Domain toko.co.id diperpanjang di registri sampai 1 Feb 2028" {
		t.Errorf("cek_diperbarui %+v", got)
	}

	// A failing check is recorded once per asset per day, however often it runs.
	reg.errs["toko.co.id"] = rdap.ErrUnreachable
	for range 3 {
		do(t, h, "POST", "/api/item/"+dom.ID+"/cek", "", c)
	}
	cert := decode[itemJSON](t, do(t, h, "POST", "/api/item", `{"jenis":"sertifikat","host":"api.example"}`, c))
	s.Checker = fakeChecker{"api.example": tlscheck.ErrUnreachable}
	do(t, h, "POST", "/api/item/"+cert.ID+"/cek", "", c)
	got := st.aktivitasOf("cek_gagal")
	if len(got) != 2 || !strings.HasPrefix(got[0].Teks, "Cek toko.co.id gagal. Registri .id tidak dapat dihubungi") ||
		got[1].Teks != "Cek api.example gagal. Server tidak dapat dihubungi di port 443." {
		t.Errorf("cek_gagal %+v", got)
	}
}
