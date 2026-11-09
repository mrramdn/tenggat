package httpapi

import (
	"bytes"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/mrramdn/tenggat/api/internal/tlscheck"
)

// reportFixture makes a client with assets, a task and work entries, every one
// carrying an internal note that must never reach a report.
func reportFixture(t *testing.T, h http.Handler, c *http.Cookie) klienJSON {
	t.Helper()
	k := decode[klienJSON](t, do(t, h, "POST", "/api/klien", `{"nama":"Studio Rana","catatan":"rahasia klien"}`, c))
	kid := `"klien_id":"` + k.ID + `"`
	for _, body := range []string{
		`{"nama":"VPS","jenis":"langganan","kedaluwarsa":"2026-11-05","biaya":300000,"catatan":"rahasia aset",` + kid + `}`,
		`{"nama":"studiorana.id","jenis":"domain","kedaluwarsa":"2026-12-20","biaya":250000,` + kid + `}`,
		`{"nama":"Hosting","jenis":"langganan","kedaluwarsa":"2027-06-01","biaya":900000,` + kid + `}`,
	} {
		if rec := do(t, h, "POST", "/api/item", body, c); rec.Code != http.StatusCreated {
			t.Fatalf("item: %d %s", rec.Code, rec.Body)
		}
	}
	vps := decode[struct{ Item []itemJSON }](t, do(t, h, "GET", "/api/item?klien_id="+k.ID, "", c)).Item[0]
	do(t, h, "PUT", "/api/item/"+vps.ID, `{"nama":"VPS","jenis":"langganan","kedaluwarsa":"2027-11-05","biaya":300000,"catatan":"rahasia aset",`+kid+`}`, c)
	tg := decode[tugasJSON](t, do(t, h, "POST", "/api/tugas", `{"judul":"Backup bulanan","catatan":"rahasia tugas","jatuh_tempo":"2026-11-01",`+kid+`}`, c))
	do(t, h, "POST", "/api/tugas/"+tg.ID+"/selesai", `{"catatan":"rahasia selesai","durasi_menit":30}`, c)
	for _, body := range []string{
		`{"jenis":"pekerjaan","teks":"Ganti tema","tanggal":"2026-11-02","durasi_menit":90,` + kid + `}`,
		`{"jenis":"pekerjaan","teks":"Bulan lalu","tanggal":"2026-10-20","durasi_menit":45,` + kid + `}`,
		`{"jenis":"catatan","teks":"rahasia aktivitas","tanggal":"2026-11-03",` + kid + `}`,
	} {
		if rec := do(t, h, "POST", "/api/aktivitas", body, c); rec.Code != http.StatusCreated {
			t.Fatalf("aktivitas: %d %s", rec.Code, rec.Body)
		}
	}
	return k
}

func TestLaporanContents(t *testing.T) {
	s, _ := newTestServer()
	h := s.Routes()
	c := signUp(t, h, "laporan@contoh.id")
	k := reportFixture(t, h, c)

	rec := do(t, h, "PUT", "/api/klien/"+k.ID+"/laporan/catatan", `{"bulan":"2026-11","teks":" Semua aman. Domain diperpanjang Desember. "}`, c)
	if rec.Code != http.StatusOK {
		t.Fatalf("catatan: %d %s", rec.Code, rec.Body)
	}
	rec = do(t, h, "GET", "/api/klien/"+k.ID+"/laporan?bulan=2026-11", "", c)
	if rec.Code != http.StatusOK {
		t.Fatalf("laporan: %d %s", rec.Code, rec.Body)
	}
	if strings.Contains(rec.Body.String(), "rahasia") {
		t.Errorf("an internal note reached the report: %s", rec.Body)
	}
	rep := decode[laporanJSON](t, rec)
	if rep.Klien.Nama != "Studio Rana" || rep.Penyusun.Nama != "Pengguna Uji" || rep.Bulan != "2026-11" ||
		rep.LabelBulan != "November 2026" || rep.Catatan != "Semua aman. Domain diperpanjang Desember." {
		t.Errorf("header %+v", rep)
	}
	// Today is 9 Nov 2026: VPS was renewed to 2027, the domain is due in 41 days, hosting next year.
	r := rep.Ringkasan
	if r.JumlahAset != 3 || r.PerStatus["aman"] != 3 || r.PerStatus["lewat"] != 0 || r.Pekerjaan != 2 || r.Menit != 120 ||
		r.Biaya30Hari != 0 {
		t.Errorf("ringkasan %+v", r)
	}
	if len(rep.Pekerjaan) != 2 || rep.Pekerjaan[0].Teks != "Ganti tema" || rep.Pekerjaan[0].Tanggal != "2026-11-02" ||
		rep.Pekerjaan[1].Teks != "Selesai: Backup bulanan" || rep.Pekerjaan[1].Jenis != "tugas_selesai" {
		t.Errorf("pekerjaan %+v", rep.Pekerjaan)
	}
	if len(rep.Perpanjangan) != 1 || rep.Perpanjangan[0].Teks != "VPS diperpanjang sampai 5 Nov 2027" {
		t.Errorf("perpanjangan %+v", rep.Perpanjangan)
	}
	if len(rep.Aset) != 3 || rep.Aset[0].Nama != "studiorana.id" {
		t.Errorf("aset %+v", rep.Aset)
	}
	// The task moved to 1 Dec (22 days) and the domain is due in 41 days; hosting is past 60 days.
	if len(rep.JatuhTempo) != 2 || rep.JatuhTempo[0].Jenis != "tugas" || rep.JatuhTempo[0].Tanggal != "2026-12-01" ||
		rep.JatuhTempo[1].Nama != "studiorana.id" || rep.JatuhTempo[1].SisaHari != 41 {
		t.Errorf("jatuh_tempo %+v", rep.JatuhTempo)
	}

	oct := decode[laporanJSON](t, do(t, h, "GET", "/api/klien/"+k.ID+"/laporan?bulan=2026-10", "", c))
	if len(oct.Pekerjaan) != 1 || oct.Pekerjaan[0].Teks != "Bulan lalu" || oct.Catatan != "" || len(oct.Perpanjangan) != 0 {
		t.Errorf("October %+v", oct)
	}
	// October has ended: its work is its own, the due-soon list and the 30-day cost are left out.
	if !oct.Lampau || oct.PerTanggal != "2026-11-09" || len(oct.JatuhTempo) != 0 || oct.Ringkasan.Biaya30Hari != 0 ||
		len(oct.Aset) != 3 || oct.BulanPertama != "2026-10" || oct.BulanTerakhir != "2026-11" {
		t.Errorf("October bounds %+v", oct)
	}
	if rep.Lampau || rep.BulanPertama != "2026-10" || rep.BulanTerakhir != "2026-11" {
		t.Errorf("November bounds lampau=%v %s..%s", rep.Lampau, rep.BulanPertama, rep.BulanTerakhir)
	}
	// Before the first entry and after this month there is no report.
	for _, b := range []string{"2026-09", "2026-12", "2020-01"} {
		rec := do(t, h, "GET", "/api/klien/"+k.ID+"/laporan?bulan="+b, "", c)
		if e := decode[apiError](t, rec); rec.Code != http.StatusUnprocessableEntity ||
			e.Field["bulan"] != "Laporan tersedia dari Oktober 2026 sampai November 2026." {
			t.Errorf("?bulan=%s: %d %+v", b, rec.Code, e)
		}
		rec = do(t, h, "PUT", "/api/klien/"+k.ID+"/laporan/catatan", `{"bulan":"`+b+`","teks":"x"}`, c)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("PUT bulan %s: %d", b, rec.Code)
		}
	}
	// No ?bulan is this month in Jakarta.
	if got := decode[laporanJSON](t, do(t, h, "GET", "/api/klien/"+k.ID+"/laporan", "", c)); got.Bulan != "2026-11" {
		t.Errorf("default month %s", got.Bulan)
	}
	for _, q := range []string{"bulan=2026-13", "bulan=11-2026", "bulan=1999-01"} {
		if rec := do(t, h, "GET", "/api/klien/"+k.ID+"/laporan?"+q, "", c); rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("?%s: %d", q, rec.Code)
		}
	}
	for body, field := range map[string]string{
		`{"teks":"x"}`:                  "bulan",
		`{"bulan":"2026-1","teks":"x"}`: "bulan",
		`{"bulan":"2026-11","teks":"` + strings.Repeat("x", 2001) + `"}`: "teks",
	} {
		rec := do(t, h, "PUT", "/api/klien/"+k.ID+"/laporan/catatan", body, c)
		if e := decode[apiError](t, rec); rec.Code != http.StatusUnprocessableEntity || e.Field[field] == "" {
			t.Errorf("PUT %s: %d %v", body, rec.Code, e.Field)
		}
	}
}

func TestLaporanLink(t *testing.T) {
	s, st := newTestServer()
	var logs bytes.Buffer
	s.Log = slog.New(slog.NewJSONHandler(&logs, nil))
	h := s.Routes()
	c := signUp(t, h, "laporan@contoh.id")
	k := reportFixture(t, h, c)
	do(t, h, "PUT", "/api/klien/"+k.ID+"/laporan/catatan", `{"bulan":"2026-11","teks":"Ringkasan untuk klien"}`, c)

	rec := do(t, h, "POST", "/api/klien/"+k.ID+"/laporan/tautan", "", c)
	if rec.Code != http.StatusCreated || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("create link: %d %s", rec.Code, rec.Body)
	}
	jalur := decode[map[string]string](t, rec)["jalur"]
	token := strings.TrimPrefix(jalur, "/laporan/")
	if !calendarToken.MatchString(token) {
		t.Fatalf("jalur %q", jalur)
	}
	if got := decode[klienList](t, do(t, h, "GET", "/api/klien", "", c)).Klien; len(got) != 1 || !got[0].LaporanAktif {
		t.Errorf("laporan_aktif after create: %+v", got)
	}
	if got := st.aktivitasOf("laporan_dibagikan"); len(got) != 1 || got[0].Teks != "Tautan laporan dibuat" || deref(got[0].KlienID) != k.ID {
		t.Errorf("laporan_dibagikan %+v", got)
	}

	pub := do(t, h, "GET", "/api/laporan/"+token+"?bulan=2026-11", "")
	if pub.Code != http.StatusOK {
		t.Fatalf("public: %d %s", pub.Code, pub.Body)
	}
	for header, want := range map[string]string{"Cache-Control": "private, no-store", "X-Robots-Tag": "noindex",
		"Referrer-Policy": "no-referrer"} {
		if got := pub.Header().Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
	if strings.Contains(pub.Body.String(), "rahasia") {
		t.Errorf("an internal note reached the public report: %s", pub.Body)
	}
	rep := decode[laporanJSON](t, pub)
	if rep.Klien.Nama != "Studio Rana" || rep.Penyusun.Nama != "Pengguna Uji" || rep.Catatan != "Ringkasan untuk klien" ||
		len(rep.Pekerjaan) != 2 || len(rep.Aset) != 3 {
		t.Errorf("public report %+v", rep)
	}
	if rec := do(t, h, "GET", "/api/laporan/"+token+"?bulan=2027-01", ""); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("public report for a future month: %d", rec.Code)
	}
	if strings.Contains(logs.String(), token) {
		t.Error("the report token was written to the log")
	}

	// A new link replaces the old one; deleting turns it off.
	second := strings.TrimPrefix(decode[map[string]string](t, do(t, h, "POST", "/api/klien/"+k.ID+"/laporan/tautan", "", c))["jalur"], "/laporan/")
	if rec := do(t, h, "GET", "/api/laporan/"+token, ""); rec.Code != http.StatusNotFound {
		t.Errorf("replaced token: %d", rec.Code)
	}
	if rec := do(t, h, "GET", "/api/laporan/"+second, ""); rec.Code != http.StatusOK {
		t.Errorf("new token: %d", rec.Code)
	}
	if rec := do(t, h, "DELETE", "/api/klien/"+k.ID+"/laporan/tautan", "", c); rec.Code != http.StatusNoContent {
		t.Errorf("delete link: %d", rec.Code)
	}
	if rec := do(t, h, "GET", "/api/laporan/"+second, ""); rec.Code != http.StatusNotFound {
		t.Errorf("revoked token: %d", rec.Code)
	}
	if got := decode[klienList](t, do(t, h, "GET", "/api/klien", "", c)).Klien; got[0].LaporanAktif {
		t.Error("laporan_aktif after delete")
	}
	for _, tok := range []string{"pendek", strings.Repeat("A", 43), strings.Repeat("A", 44)} {
		if rec := do(t, h, "GET", "/api/laporan/"+tok, ""); rec.Code != http.StatusNotFound || rec.Header().Get("X-Robots-Tag") != "noindex" {
			t.Errorf("token %q: %d", tok, rec.Code)
		}
	}
	if got := redactPath("/api/laporan/" + second); got != "/api/laporan/…" {
		t.Errorf("redactPath = %q", got)
	}
}

func TestLaporanIsPerUser(t *testing.T) {
	s, _ := newTestServer()
	h := s.Routes()
	a := signUp(t, h, "a@contoh.id")
	b := signUp(t, h, "b@contoh.id")
	k := reportFixture(t, h, a)
	for _, r := range []struct{ method, path, body string }{
		{"GET", "/api/klien/" + k.ID + "/laporan", ""},
		{"PUT", "/api/klien/" + k.ID + "/laporan/catatan", `{"bulan":"2026-11","teks":"x"}`},
		{"POST", "/api/klien/" + k.ID + "/laporan/tautan", ""},
		{"DELETE", "/api/klien/" + k.ID + "/laporan/tautan", ""},
	} {
		if rec := do(t, h, r.method, r.path, r.body, b); rec.Code != http.StatusNotFound {
			t.Errorf("user B %s %s: %d, want 404", r.method, r.path, rec.Code)
		}
		if rec := do(t, h, r.method, r.path, r.body); rec.Code != http.StatusUnauthorized {
			t.Errorf("no session %s %s: %d, want 401", r.method, r.path, rec.Code)
		}
	}
}

func TestLaporanRateLimit(t *testing.T) {
	s, _ := newTestServer()
	s.Cache = newMemCache()
	h := s.Routes()
	for i := range reportPerMinute {
		if rec := do(t, h, "GET", "/api/laporan/"+strings.Repeat("A", 43), ""); rec.Code != http.StatusNotFound {
			t.Fatalf("request %d: %d", i, rec.Code)
		}
	}
	if rec := do(t, h, "GET", "/api/laporan/"+strings.Repeat("A", 43), ""); rec.Code != http.StatusTooManyRequests {
		t.Errorf("over the limit: %d", rec.Code)
	}
}

func TestCalendarForOneClient(t *testing.T) {
	s, _ := newTestServer()
	h := s.Routes()
	c := signUp(t, h, "kal@contoh.id")
	do(t, h, "POST", "/api/item", `{"nama":"VPS Rana","jenis":"langganan","klien":"Studio Rana","kedaluwarsa":"2026-12-01"}`, c)
	do(t, h, "POST", "/api/item", `{"nama":"VPS Apotek","jenis":"langganan","klien":"Apotek","kedaluwarsa":"2026-12-02"}`, c)
	list := decode[klienList](t, do(t, h, "GET", "/api/klien", "", c)).Klien
	body := do(t, h, "GET", "/api/kalender.ics?klien_id="+list[1].ID, "", c).Body.String()
	if !strings.Contains(body, "VPS Rana") || strings.Contains(body, "VPS Apotek") {
		t.Errorf("calendar for Studio Rana:\n%s", body)
	}
	if rec := do(t, h, "GET", "/api/kalender.ics?klien_id=x", "", c); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("bad klien_id: %d", rec.Code)
	}
}

type publikResult struct {
	Host   string
	Domain *struct {
		Domain, Kedaluwarsa, Status, Registrar string
		SisaHari                               int `json:"sisa_hari"`
	}
	DomainGalat *string `json:"domain_galat"`
	Sertifikat  *struct {
		Kedaluwarsa, Status, Penerbit string
		SisaHari                      int `json:"sisa_hari"`
	}
	SertifikatGalat *string `json:"sertifikat_galat"`
}

func TestPublicCheck(t *testing.T) {
	s, _, _ := newRegistryServer()
	s.Checker = fakeChecker{"mati.toko.co.id": tlscheck.ErrUnreachable, "lokal.toko.co.id": tlscheck.ErrBlockedHost}
	h := s.Routes()

	rec := do(t, h, "GET", "/api/publik/cek?host=https://WWW.Toko.co.id/kontak", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("check: %d %s", rec.Code, rec.Body)
	}
	got := decode[publikResult](t, rec)
	if got.Host != "www.toko.co.id" || got.Domain == nil || got.Domain.Domain != "toko.co.id" ||
		got.Domain.Kedaluwarsa != "2027-02-01" || got.Domain.Registrar != "PT Registrar Uji" || got.DomainGalat != nil ||
		got.Sertifikat == nil || got.Sertifikat.Kedaluwarsa != "2026-11-28" || got.Sertifikat.SisaHari != 19 ||
		got.Sertifikat.Status != "segera" || got.Sertifikat.Penerbit != "Let's Encrypt R12" || got.SertifikatGalat != nil {
		t.Errorf("result %s", rec.Body)
	}

	// Each part fails on its own and says why.
	got = decode[publikResult](t, do(t, h, "GET", "/api/publik/cek?host=mati.toko.co.id", ""))
	if got.Domain == nil || got.Sertifikat != nil || got.SertifikatGalat == nil ||
		*got.SertifikatGalat != "Server tidak dapat dihubungi di port 443." {
		t.Errorf("unreachable server: %+v", got)
	}
	got = decode[publikResult](t, do(t, h, "GET", "/api/publik/cek?host=lokal.toko.co.id", ""))
	if got.SertifikatGalat == nil || !strings.Contains(*got.SertifikatGalat, "lokal atau privat") {
		t.Errorf("blocked host: %+v", got)
	}
	got = decode[publikResult](t, do(t, h, "GET", "/api/publik/cek?host=tidakada.com", ""))
	if got.Domain != nil || got.DomainGalat == nil ||
		*got.DomainGalat != "Domain tidakada.com tidak ditemukan di registri. Periksa ejaannya." {
		t.Errorf("unknown domain: %+v", got)
	}
	got = decode[publikResult](t, do(t, h, "GET", "/api/publik/cek?host=co.id", ""))
	if got.Domain != nil || got.DomainGalat == nil {
		t.Errorf("public suffix: %+v", got)
	}

	for _, host := range []string{"", "localhost", "127.0.0.1", "toko.co.id:8443", "a..b", "user@toko.id"} {
		rec := do(t, h, "GET", "/api/publik/cek?host="+host, "")
		if e := decode[apiError](t, rec); rec.Code != http.StatusUnprocessableEntity || e.Field["host"] == "" {
			t.Errorf("host %q: %d %s", host, rec.Code, rec.Body)
		}
	}
}

func TestPublicCheckRateLimit(t *testing.T) {
	s, _, _ := newRegistryServer()
	s.Cache = newMemCache()
	h := s.Routes()
	for i := range publicCheckPerMinute {
		if rec := do(t, h, "GET", "/api/publik/cek?host=toko.co.id", ""); rec.Code != http.StatusOK {
			t.Fatalf("request %d: %d", i, rec.Code)
		}
	}
	rec := do(t, h, "GET", "/api/publik/cek?host=toko.co.id", "")
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" {
		t.Errorf("over the limit: %d", rec.Code)
	}
	// Its own bucket: signing in still works.
	if rec := do(t, h, "POST", "/api/akun", `{"nama":"Pengguna Uji","email":"x@contoh.id","kata_sandi":"rahasia-panjang"}`); rec.Code != http.StatusCreated {
		t.Errorf("sign-up after the demo limit: %d", rec.Code)
	}
}
