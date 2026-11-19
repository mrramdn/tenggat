package httpapi

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/mrramdn/tenggat/api/internal/expiry"
	"github.com/mrramdn/tenggat/api/internal/store"
)

func i64(v int64) *int64    { return &v }
func strp(s string) *string { return &s }

func userIDOf(t *testing.T, h http.Handler, c *http.Cookie) string {
	t.Helper()
	return decode[accountJSON](t, do(t, h, "GET", "/api/saya", "", c)).Pengguna.ID
}

func TestAgendaEmptyAndAuth(t *testing.T) {
	s, _ := newTestServer()
	h := s.Routes()

	if rec := do(t, h, "GET", "/api/agenda", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("without session: %d, want 401", rec.Code)
	}

	c := signUp(t, h, "kosong@contoh.id")
	rec := do(t, h, "GET", "/api/agenda", "", c)
	if rec.Code != http.StatusOK {
		t.Fatalf("empty agenda: %d %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), `"agenda":[]`) {
		t.Errorf("empty agenda should encode [], got %s", rec.Body)
	}
	got := decode[agendaResponse](t, rec)
	if len(got.Agenda) != 0 || got.Jumlah != (agendaJumlah{}) || got.Lagi {
		t.Errorf("empty agenda = %+v", got)
	}
}

func TestAgendaMergeOrderAndAksi(t *testing.T) {
	s, st := newTestServer()
	h := s.Routes()
	c := signUp(t, h, "agenda@contoh.id")
	uid := userIDOf(t, h, c)
	ctx := t.Context()

	k, err := st.CreateKlien(ctx, uid, "Studio Rana", "", "")
	if err != nil {
		t.Fatal(err)
	}

	// testNow is Monday 9 Nov 2026.
	d := func(y int, m time.Month, day int) time.Time { return time.Date(y, m, day, 0, 0, 0, 0, time.UTC) }

	// 1. Overdue one-off bill (IDR, no client) -> bayar
	billOneOff, _ := st.Create(ctx, uid, store.Input{
		Nama: "Tagihan Listrik", Jenis: "tagihan", Kedaluwarsa: d(2026, 11, 7),
		Sumber: "manual", Biaya: i64(500000), MataUang: "IDR",
	})
	// 2. Recurring bill (USD, no client) -> bayar
	billUSD, _ := st.Create(ctx, uid, store.Input{
		Nama: "Claude Pro", Jenis: "tagihan", Kedaluwarsa: d(2026, 11, 13),
		Sumber: "manual", Biaya: i64(2050), MataUang: "USD", UlangJumlah: 1, UlangSatuan: "bulan",
	})
	// 3. Same date as Claude Pro, earlier title (case-insensitive) -> recurring task -> selesai
	recTask, _ := st.CreateTugas(ctx, uid, store.TugasInput{
		KlienID: &k.ID, Judul: "backup mingguan", UlangJumlah: 1, UlangSatuan: "minggu",
		JatuhTempo: d(2026, 11, 13), Biaya: i64(150000), MataUang: "IDR",
	})
	// 4. Same date as Claude Pro, later title -> one-off task -> selesai, sekali=true
	oneTask, _ := st.CreateTugas(ctx, uid, store.TugasInput{
		Judul: "Kirim invoice", JatuhTempo: d(2026, 11, 13),
	})
	// 5. Manual recurring item (non-bill) -> perpanjang
	manRec, _ := st.Create(ctx, uid, store.Input{
		Nama: "VPS Jakarta", Jenis: "langganan", KlienID: &k.ID, Klien: k.Nama,
		Kedaluwarsa: d(2026, 11, 20), Sumber: "manual", Biaya: i64(250000), MataUang: "IDR",
		UlangJumlah: 1, UlangSatuan: "bulan",
	})
	// 6. Certificate (server) -> cek
	cert, _ := st.Create(ctx, uid, store.Input{
		Nama: "studiorana.co.id", Jenis: "sertifikat", KlienID: &k.ID, Klien: k.Nama,
		Host: strp("studiorana.co.id"), Kedaluwarsa: d(2026, 11, 28), Sumber: "server",
	})
	// 7. Registry domain -> cek
	regDom, _ := st.Create(ctx, uid, store.Input{
		Nama: "ranastudio.com", Jenis: "domain", KlienID: &k.ID, Klien: k.Nama,
		Host: strp("ranastudio.com"), Kedaluwarsa: d(2026, 12, 15), Sumber: "registri",
	})
	// 8. Manual item without recurrence -> aksi nil
	manPlain, _ := st.Create(ctx, uid, store.Input{
		Nama: "Token API", Jenis: "token", Kedaluwarsa: d(2026, 12, 20), Sumber: "manual",
	})

	rec := do(t, h, "GET", "/api/agenda", "", c)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/agenda: %d %s", rec.Code, rec.Body)
	}
	got := decode[agendaResponse](t, rec)

	wantIDs := []string{
		billOneOff.ID,
		recTask.ID, // 2026-11-13 "backup mingguan"
		billUSD.ID, // 2026-11-13 "Claude Pro"
		oneTask.ID, // 2026-11-13 "Kirim invoice"
		manRec.ID,
		cert.ID,
		regDom.ID,
		manPlain.ID,
	}
	if len(got.Agenda) != len(wantIDs) {
		t.Fatalf("got %d rows, want %d: %+v", len(got.Agenda), len(wantIDs), got.Agenda)
	}
	for i, id := range wantIDs {
		if got.Agenda[i].ID != id {
			t.Errorf("row %d id = %s (%s), want %s", i, got.Agenda[i].ID, got.Agenda[i].Judul, id)
		}
	}

	if got.Jumlah != (agendaJumlah{Lewat: 1, Kritis: 3, Segera: 2, Aman: 2, Total: 8}) || got.Lagi {
		t.Errorf("jumlah = %+v, lagi = %v", got.Jumlah, got.Lagi)
	}

	byID := map[string]agendaRow{}
	for _, row := range got.Agenda {
		byID[row.ID] = row
	}

	// Verify fields on recurring USD bill.
	r := byID[billUSD.ID]
	if r.Asal != "item" || r.Jenis != "tagihan" || r.Sekali || r.KlienID != nil || r.Klien != "" ||
		r.Tanggal != "2026-11-13" || r.SisaHari != 4 || r.Status != expiry.Kritis ||
		r.Biaya == nil || string(*r.Biaya) != "20.5" || r.MataUang != "USD" ||
		r.Ulang == nil || r.Ulang.Jumlah != 1 || r.Ulang.Satuan != "bulan" {
		t.Errorf("billUSD row = %+v", r)
	}
	// Verify fields on one-off task vs recurring task.
	if rt := byID[recTask.ID]; rt.Asal != "tugas" || rt.Jenis != "tugas" || rt.Sekali ||
		rt.Klien != "Studio Rana" || rt.Biaya == nil || string(*rt.Biaya) != "150000" ||
		rt.Ulang == nil || rt.Ulang.Jumlah != 1 || rt.Ulang.Satuan != "minggu" {
		t.Errorf("recTask row = %+v", rt)
	}
	if ot := byID[oneTask.ID]; ot.Asal != "tugas" || ot.Jenis != "tugas" || !ot.Sekali ||
		ot.Biaya != nil || ot.Ulang != nil {
		t.Errorf("oneTask row = %+v", ot)
	}

	// Verify aksi for every kind.
	checkAksi := func(id string, want *agendaAksi) {
		t.Helper()
		gotAksi := byID[id].Aksi
		if want == nil {
			if gotAksi != nil {
				t.Errorf("row %s aksi = %+v, want nil", id, gotAksi)
			}
			return
		}
		if gotAksi == nil || *gotAksi != *want {
			t.Errorf("row %s aksi = %+v, want %+v", id, gotAksi, want)
		}
	}
	checkAksi(billOneOff.ID, &agendaAksi{Kode: "bayar", Label: "Tandai dibayar", Method: "POST", Jalur: "/api/item/" + billOneOff.ID + "/perpanjang"})
	checkAksi(billUSD.ID, &agendaAksi{Kode: "bayar", Label: "Tandai dibayar", Method: "POST", Jalur: "/api/item/" + billUSD.ID + "/perpanjang"})
	checkAksi(manRec.ID, &agendaAksi{Kode: "perpanjang", Label: "Perpanjang", Method: "POST", Jalur: "/api/item/" + manRec.ID + "/perpanjang"})
	checkAksi(cert.ID, &agendaAksi{Kode: "cek", Label: "Cek ulang", Method: "POST", Jalur: "/api/item/" + cert.ID + "/cek"})
	checkAksi(regDom.ID, &agendaAksi{Kode: "cek", Label: "Cek ulang", Method: "POST", Jalur: "/api/item/" + regDom.ID + "/cek"})
	checkAksi(manPlain.ID, nil)
	checkAksi(recTask.ID, &agendaAksi{Kode: "selesai", Label: "Tandai selesai", Method: "POST", Jalur: "/api/tugas/" + recTask.ID + "/selesai"})
	checkAksi(oneTask.ID, &agendaAksi{Kode: "selesai", Label: "Tandai selesai", Method: "POST", Jalur: "/api/tugas/" + oneTask.ID + "/selesai"})
}

func TestAgendaFiltersAndValidation(t *testing.T) {
	s, st := newTestServer()
	h := s.Routes()
	c := signUp(t, h, "filter@contoh.id")
	uid := userIDOf(t, h, c)
	ctx := t.Context()

	kA, _ := st.CreateKlien(ctx, uid, "Toko Mawar", "", "")
	kPribadi, _ := st.CreateKlien(ctx, uid, "Pribadi", "", "") // a client literally named "Pribadi" is still a client

	d := func(y int, m time.Month, day int) time.Time { return time.Date(y, m, day, 0, 0, 0, 0, time.UTC) }

	// Overdue personal bill
	iBill, _ := st.Create(ctx, uid, store.Input{
		Nama: "Netflix", Jenis: "tagihan", Kedaluwarsa: d(2026, 11, 8), Sumber: "manual",
		Biaya: i64(186000), Catatan: "Paket keluarga",
	})
	// Today client domain with host
	iDom, _ := st.Create(ctx, uid, store.Input{
		Nama: "Domain utama", Jenis: "domain", KlienID: &kA.ID, Klien: kA.Nama,
		Host: strp("tokomawar.co.id"), Kedaluwarsa: d(2026, 11, 9), Sumber: "registri",
	})
	// 5 days client "Pribadi" token (has KlienID, so pribadi=1 must NOT include it)
	iTok, _ := st.Create(ctx, uid, store.Input{
		Nama: "Token GitHub", Jenis: "token", KlienID: &kPribadi.ID, Klien: kPribadi.Nama,
		Kedaluwarsa: d(2026, 11, 14), Sumber: "manual",
	})
	// 20 days personal task
	tPriv, _ := st.CreateTugas(ctx, uid, store.TugasInput{
		Judul: "Bayar pajak", Catatan: "Bawa NPWP", JatuhTempo: d(2026, 11, 29),
	})
	// 40 days client task
	tClient, _ := st.CreateTugas(ctx, uid, store.TugasInput{
		KlienID: &kA.ID, Judul: "Audit keamanan", UlangJumlah: 3, UlangSatuan: "bulan",
		JatuhTempo: d(2026, 12, 19),
	})

	getIDs := func(query string) []string {
		t.Helper()
		rec := do(t, h, "GET", "/api/agenda"+query, "", c)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /api/agenda%s: %d %s", query, rec.Code, rec.Body)
		}
		res := decode[agendaResponse](t, rec)
		ids := make([]string, len(res.Agenda))
		for i, r := range res.Agenda {
			ids[i] = r.ID
		}
		return ids
	}

	assertIDs := func(query string, want ...string) {
		t.Helper()
		got := getIDs(query)
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("GET /api/agenda%s = %v, want %v", query, got, want)
		}
	}

	// dalam_hari: 0 includes overdue + today; 5 adds iTok.
	assertIDs("?dalam_hari=0", iBill.ID, iDom.ID)
	assertIDs("?dalam_hari=5", iBill.ID, iDom.ID, iTok.ID)

	// klien_id vs pribadi=1.
	assertIDs("?klien_id="+kA.ID, iDom.ID, tClient.ID)
	assertIDs("?pribadi=1", iBill.ID, tPriv.ID)
	assertIDs("?pribadi=0", iBill.ID, iDom.ID, iTok.ID, tPriv.ID, tClient.ID)

	// status.
	assertIDs("?status=lewat", iBill.ID)
	assertIDs("?status=kritis", iDom.ID, iTok.ID)
	assertIDs("?status=segera", tPriv.ID)
	assertIDs("?status=aman", tClient.ID)

	// jenis comma-separated.
	assertIDs("?jenis=tagihan,tugas", iBill.ID, tPriv.ID, tClient.ID)
	assertIDs("?jenis=tugas", tPriv.ID, tClient.ID)
	assertIDs("?jenis=domain,token", iDom.ID, iTok.ID)
	assertIDs("?jenis=tugas,tugas", tPriv.ID, tClient.ID)

	// q: title, client name, notes, host (case-insensitive).
	assertIDs("?q=NETFLIX", iBill.ID)          // item title
	assertIDs("?q=pajak", tPriv.ID)            // task title
	assertIDs("?q=mawar", iDom.ID, tClient.ID) // client name on item and task
	assertIDs("?q=keluarga", iBill.ID)         // item note
	assertIDs("?q=npwp", tPriv.ID)             // task note
	assertIDs("?q=TOKOMAWAR.CO", iDom.ID)      // item host

	// Exactly 100 runes is valid.
	assertIDs("?q=" + strings.Repeat("a", 100))

	// Invalid queries return 422 with per-field messages.
	for _, tc := range []struct{ query, field string }{
		{"?dalam_hari=-1", "dalam_hari"},
		{"?dalam_hari=3661", "dalam_hari"},
		{"?dalam_hari=tujuh", "dalam_hari"},
		{"?klien_id=bukan-uuid", "klien_id"},
		{"?pribadi=ya", "pribadi"},
		{"?pribadi=1&klien_id=" + kA.ID, "pribadi"},
		{"?status=darurat", "status"},
		{"?jenis=asing", "jenis"},
		{"?jenis=tagihan,", "jenis"},
		{"?jenis=,tugas", "jenis"},
		{"?q=" + strings.Repeat("a", 101), "q"},
		{"?batas=0", "batas"},
		{"?batas=501", "batas"},
		{"?batas=banyak", "batas"},
	} {
		rec := do(t, h, "GET", "/api/agenda"+tc.query, "", c)
		e := decode[apiError](t, rec)
		if rec.Code != http.StatusUnprocessableEntity || e.Field[tc.field] == "" {
			t.Errorf("GET /api/agenda%s: status %d, fields %v, want 422 with field %q", tc.query, rec.Code, e.Field, tc.field)
		}
	}
}

func TestAgendaExcludesClosedRows(t *testing.T) {
	s, st := newTestServer()
	h := s.Routes()
	c := signUp(t, h, "tutup@contoh.id")
	uid := userIDOf(t, h, c)
	ctx := t.Context()

	due := time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC)
	activeBill, _ := st.Create(ctx, uid, store.Input{
		Nama: "Tagihan Aktif", Jenis: "tagihan", Kedaluwarsa: due, Sumber: "manual", Biaya: i64(100000),
	})
	closedBill, _ := st.Create(ctx, uid, store.Input{
		Nama: "Tagihan Lunas", Jenis: "tagihan", Kedaluwarsa: due, Sumber: "manual", Biaya: i64(200000),
	})
	if _, err := st.PerpanjangItem(ctx, uid, closedBill.ID, store.Perpanjangan{
		Dari: due, Lunas: true, At: testNow,
		Aktivitas: store.AktivitasInput{Jenis: "tagihan_dibayar", Teks: "Lunas"},
	}); err != nil {
		t.Fatal(err)
	}

	activeTask, _ := st.CreateTugas(ctx, uid, store.TugasInput{Judul: "Tugas Aktif", JatuhTempo: due})
	closedTask, _ := st.CreateTugas(ctx, uid, store.TugasInput{Judul: "Tugas Selesai", JatuhTempo: due})
	if _, err := st.CloseTugas(ctx, uid, closedTask.ID, due, testNow); err != nil {
		t.Fatal(err)
	}

	got := decode[agendaResponse](t, do(t, h, "GET", "/api/agenda", "", c))
	if got.Jumlah.Total != 2 || len(got.Agenda) != 2 {
		t.Fatalf("got %d rows (total %d), want 2: %+v", len(got.Agenda), got.Jumlah.Total, got.Agenda)
	}
	if got.Agenda[0].ID != activeBill.ID || got.Agenda[1].ID != activeTask.ID {
		t.Errorf("agenda = %+v", got.Agenda)
	}
}

func TestAgendaBatasLagiAndJumlah(t *testing.T) {
	s, st := newTestServer()
	h := s.Routes()
	c := signUp(t, h, "batas@contoh.id")
	uid := userIDOf(t, h, c)
	ctx := t.Context()

	dates := []time.Time{
		time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC),  // lewat
		time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC), // kritis
		time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC), // kritis
		time.Date(2026, 11, 25, 0, 0, 0, 0, time.UTC), // segera
		time.Date(2026, 12, 20, 0, 0, 0, 0, time.UTC), // aman
	}
	for i, dt := range dates {
		if i%2 == 0 {
			_, _ = st.Create(ctx, uid, store.Input{
				Nama: "Item " + string(rune('A'+i)), Jenis: "lainnya", Kedaluwarsa: dt, Sumber: "manual",
			})
		} else {
			_, _ = st.CreateTugas(ctx, uid, store.TugasInput{
				Judul: "Tugas " + string(rune('A'+i)), JatuhTempo: dt,
			})
		}
	}

	cut := decode[agendaResponse](t, do(t, h, "GET", "/api/agenda?batas=2", "", c))
	wantJumlah := agendaJumlah{Lewat: 1, Kritis: 2, Segera: 1, Aman: 1, Total: 5}
	if len(cut.Agenda) != 2 || !cut.Lagi || cut.Jumlah != wantJumlah {
		t.Errorf("batas=2: len=%d lagi=%v jumlah=%+v", len(cut.Agenda), cut.Lagi, cut.Jumlah)
	}

	exact := decode[agendaResponse](t, do(t, h, "GET", "/api/agenda?batas=5", "", c))
	if len(exact.Agenda) != 5 || exact.Lagi || exact.Jumlah != wantJumlah {
		t.Errorf("batas=5: len=%d lagi=%v jumlah=%+v", len(exact.Agenda), exact.Lagi, exact.Jumlah)
	}
}

func TestAgendaIsolationAndCache(t *testing.T) {
	s, st := newTestServer()
	s.Cache = newMemCache()
	h := s.Routes()
	a := signUp(t, h, "ag-a@contoh.id")
	b := signUp(t, h, "ag-b@contoh.id")
	uidA := userIDOf(t, h, a)
	uidB := userIDOf(t, h, b)
	ctx := t.Context()

	kA, _ := st.CreateKlien(ctx, uidA, "Klien A", "", "")
	due := time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC)
	_, _ = st.Create(ctx, uidA, store.Input{
		Nama: "Netflix A", Jenis: "tagihan", KlienID: &kA.ID, Klien: kA.Nama, Kedaluwarsa: due, Sumber: "manual",
	})
	_, _ = st.CreateTugas(ctx, uidA, store.TugasInput{KlienID: &kA.ID, Judul: "Tugas A", JatuhTempo: due})
	_, _ = st.Create(ctx, uidB, store.Input{
		Nama: "Hosting B", Jenis: "langganan", Kedaluwarsa: due, Sumber: "manual",
	})

	// Cache MISS then HIT for A.
	r1 := do(t, h, "GET", "/api/agenda", "", a)
	r2 := do(t, h, "GET", "/api/agenda?batas=200", "", a)
	if r1.Header().Get("X-Cache") != "MISS" || r2.Header().Get("X-Cache") != "HIT" {
		t.Fatalf("X-Cache A = %q then %q, want MISS then HIT", r1.Header().Get("X-Cache"), r2.Header().Get("X-Cache"))
	}
	if res := decode[agendaResponse](t, r2); res.Jumlah.Total != 2 {
		t.Errorf("user A total = %d, want 2", res.Jumlah.Total)
	}

	// Canonical query: different order / case hits the same cache entry.
	q1 := do(t, h, "GET", "/api/agenda?jenis=tugas,tagihan&q=NETFLIX", "", a)
	q2 := do(t, h, "GET", "/api/agenda?q=+netflix+&jenis=tagihan,tugas&batas=200", "", a)
	if q1.Header().Get("X-Cache") != "MISS" || q2.Header().Get("X-Cache") != "HIT" {
		t.Errorf("canonical cache = %q then %q, want MISS then HIT", q1.Header().Get("X-Cache"), q2.Header().Get("X-Cache"))
	}

	// User B never receives A's rows or A's cached response.
	rb := do(t, h, "GET", "/api/agenda", "", b)
	if rb.Header().Get("X-Cache") != "MISS" {
		t.Errorf("user B X-Cache = %q, want MISS", rb.Header().Get("X-Cache"))
	}
	if res := decode[agendaResponse](t, rb); res.Jumlah.Total != 1 || res.Agenda[0].Judul != "Hosting B" {
		t.Errorf("user B agenda = %+v", res)
	}
	// Passing A's klien_id as B returns nothing.
	if res := decode[agendaResponse](t, do(t, h, "GET", "/api/agenda?klien_id="+kA.ID, "", b)); res.Jumlah.Total != 0 {
		t.Errorf("user B with A klien_id = %+v", res)
	}

	// Writing invalidates A's cache.
	do(t, h, "POST", "/api/item", `{"nama":"Baru A","jenis":"token","kedaluwarsa":"2026-11-18"}`, a)
	r3 := do(t, h, "GET", "/api/agenda", "", a)
	if r3.Header().Get("X-Cache") != "MISS" || decode[agendaResponse](t, r3).Jumlah.Total != 3 {
		t.Errorf("after write: X-Cache=%q body=%s", r3.Header().Get("X-Cache"), r3.Body)
	}
}

func TestDigestWithBillsAndUnchangedFixture(t *testing.T) {
	s, st := newTestServer()
	s.AppURL = "https://tenggat.example"
	h := s.Routes()
	c := signUp(t, h, "digest@contoh.id")
	uid := userIDOf(t, h, c)
	ctx := context.Background()

	// Monday 9 Nov 2026, 08.00 WIB.
	at8 := time.Date(2026, 11, 9, 1, 0, 0, 0, time.UTC)
	d := func(y int, m time.Month, day int) time.Time { return time.Date(y, m, day, 0, 0, 0, 0, time.UTC) }

	// Same fixture as TestDailyDigest: must remain byte-for-byte identical.
	for _, body := range []string{
		`{"nama":"Domain lama","jenis":"lainnya","klien":"Studio Rana","kedaluwarsa":"2026-11-06","biaya":250000}`,
		`{"nama":"VPS","jenis":"langganan","kedaluwarsa":"2026-11-16","biaya":300000}`,
		`{"nama":"Hosting","jenis":"langganan","kedaluwarsa":"2026-11-17"}`,
		`{"nama":"Token API","jenis":"token","kedaluwarsa":"2026-11-09"}`,
	} {
		do(t, h, "POST", "/api/item", body, c)
	}
	do(t, h, "POST", "/api/tugas", `{"judul":"Backup","klien":"Studio Rana","jatuh_tempo":"2026-11-10"}`, c)
	do(t, h, "POST", "/api/tugas", `{"judul":"Audit","jatuh_tempo":"2026-12-10"}`, c)

	baseText, err := s.digestText(ctx, uid, at8)
	if err != nil {
		t.Fatal(err)
	}
	wantBase := "Ringkasan harian Tenggat, 9 Nov 2026\n\nLewat:\n- Domain lama (Studio Rana): lewat 3 hari, 6 Nov 2026" +
		"\n\nJatuh tempo 7 hari ke depan:\n- Token API: hari ini\n- Tugas Backup (Studio Rana): besok, 10 Nov 2026" +
		"\n- VPS: 7 hari lagi, 16 Nov 2026\n\nBuka Tenggat: https://tenggat.example/app"
	if baseText != wantBase {
		t.Fatalf("base digest:\n%s\nwant:\n%s", baseText, wantBase)
	}

	// Add bills (IDR personal, USD with client, already prefixed, without cost, and closed).
	k, _ := st.CreateKlien(ctx, uid, "Klien B", "", "")
	_, _ = st.Create(ctx, uid, store.Input{
		Nama: "Netflix", Jenis: "tagihan", Kedaluwarsa: d(2026, 11, 13), Sumber: "manual",
		Biaya: i64(186000), MataUang: "IDR", UlangJumlah: 1, UlangSatuan: "bulan",
	})
	_, _ = st.Create(ctx, uid, store.Input{
		Nama: "Claude Pro", Jenis: "tagihan", KlienID: &k.ID, Klien: k.Nama,
		Kedaluwarsa: d(2026, 11, 10), Sumber: "manual", Biaya: i64(2050), MataUang: "USD",
	})
	_, _ = st.Create(ctx, uid, store.Input{
		Nama: "Tagihan Listrik", Jenis: "tagihan", Kedaluwarsa: d(2026, 11, 8), Sumber: "manual",
		Biaya: i64(500000), MataUang: "IDR",
	})
	_, _ = st.Create(ctx, uid, store.Input{
		Nama: "Internet", Jenis: "tagihan", Kedaluwarsa: d(2026, 11, 9), Sumber: "manual",
	})
	closedBill, _ := st.Create(ctx, uid, store.Input{
		Nama: "Sudah Lunas", Jenis: "tagihan", Kedaluwarsa: d(2026, 11, 10), Sumber: "manual",
		Biaya: i64(99000), MataUang: "IDR",
	})
	_, _ = st.PerpanjangItem(ctx, uid, closedBill.ID, store.Perpanjangan{
		Dari: d(2026, 11, 10), Lunas: true, At: testNow,
		Aktivitas: store.AktivitasInput{Jenis: "tagihan_dibayar", Teks: "Lunas"},
	})

	withBills, err := s.digestText(ctx, uid, at8)
	if err != nil {
		t.Fatal(err)
	}
	for _, wantLine := range []string{
		"- Tagihan Listrik: lewat 1 hari, 8 Nov 2026, Rp 500.000",
		"- Tagihan Internet: hari ini",
		"- Tagihan Claude Pro (Klien B): besok, 10 Nov 2026, US$20.50",
		"- Tagihan Netflix: 4 hari lagi, 13 Nov 2026, Rp 186.000",
	} {
		if !strings.Contains(withBills, "\n"+wantLine) {
			t.Errorf("digest missing %q:\n%s", wantLine, withBills)
		}
	}
	if strings.Contains(withBills, "Sudah Lunas") {
		t.Errorf("closed bill appeared in digest:\n%s", withBills)
	}
	if strings.Contains(withBills, "Tagihan Tagihan") {
		t.Errorf("duplicate Tagihan prefix in digest:\n%s", withBills)
	}
	if strings.Contains(strings.ToLower(withBills), "total") {
		t.Errorf("digest should never print a total:\n%s", withBills)
	}
}
