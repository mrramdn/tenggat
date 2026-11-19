package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func decodeRawMap(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("unmarshal %q: %v", string(body), err)
	}
	return m
}

func TestTugasSekaliCreateAndComplete(t *testing.T) {
	s, st := newTestServer()
	s.Cache = newMemCache()
	h := s.Routes()
	c := signUp(t, h, "sekali@contoh.id")

	// One-off with a client.
	rec := do(t, h, "POST", "/api/tugas",
		`{"judul":"Migrasi server","klien":"Studio Rana","sekali":true,"jatuh_tempo":"2026-11-20","biaya":250000}`, c)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create with client: %d %s", rec.Code, rec.Body)
	}
	raw := decodeRawMap(t, rec.Body.Bytes())
	if raw["sekali"] != true || raw["ulang_jumlah"] != nil || raw["ulang_satuan"] != nil ||
		raw["mata_uang"] != "IDR" || raw["selesai_pada"] != nil {
		t.Errorf("raw one-off json = %v", raw)
	}
	withClient := decode[tugasJSON](t, rec)
	if !withClient.Sekali || withClient.KlienID == nil || withClient.Klien != "Studio Rana" ||
		withClient.MataUang != "IDR" || withClient.Biaya == nil || *withClient.Biaya != "250000" ||
		withClient.SelesaiPada != nil {
		t.Errorf("created with client = %+v", withClient)
	}
	if loc := rec.Header().Get("Location"); loc != "/api/tugas/"+withClient.ID {
		t.Errorf("Location = %q", loc)
	}

	// One-off without a client, explicitly passing null recurrence fields.
	rec = do(t, h, "POST", "/api/tugas",
		`{"judul":"Audit DNS","sekali":true,"ulang_jumlah":null,"ulang_satuan":null,"jatuh_tempo":"2026-11-25"}`, c)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create without client: %d %s", rec.Code, rec.Body)
	}
	noClient := decode[tugasJSON](t, rec)
	if !noClient.Sekali || noClient.KlienID != nil || noClient.Klien != "" || noClient.Biaya != nil {
		t.Errorf("created without client = %+v", noClient)
	}

	// A recurring task alongside the one-offs.
	rec = do(t, h, "POST", "/api/tugas",
		`{"judul":"Backup bulanan","klien_id":"`+*withClient.KlienID+`","jatuh_tempo":"2026-12-01"}`, c)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create recurring: %d %s", rec.Code, rec.Body)
	}
	recurring := decode[tugasJSON](t, rec)
	if recurring.Sekali || recurring.UlangJumlah != 1 || recurring.UlangSatuan != "bulan" || recurring.MataUang != "IDR" {
		t.Errorf("recurring = %+v", recurring)
	}

	// Active list has all three; closed list is empty.
	if got := decode[tugasList](t, do(t, h, "GET", "/api/tugas", "", c)).Tugas; len(got) != 3 {
		t.Fatalf("active list before close: %d %+v", len(got), got)
	}
	if got := decode[tugasList](t, do(t, h, "GET", "/api/tugas?selesai=1", "", c)).Tugas; len(got) != 0 {
		t.Fatalf("closed list before close: %d %+v", len(got), got)
	}

	// Complete the client's one-off task.
	rec = do(t, h, "POST", "/api/tugas/"+withClient.ID+"/selesai",
		`{"catatan":"Pindah ke VPS baru","durasi_menit":45,"tanggal":"2026-11-09","dari":"2026-11-20"}`, c)
	if rec.Code != http.StatusOK {
		t.Fatalf("complete one-off: %d %s", rec.Code, rec.Body)
	}
	closed := decode[tugasJSON](t, rec)
	if !closed.Sekali || closed.JatuhTempo != "2026-11-20" || closed.SelesaiPada == nil || !closed.SelesaiPada.Equal(testNow) ||
		closed.TerakhirSelesai == nil || !closed.TerakhirSelesai.Equal(testNow) {
		t.Errorf("closed = %+v", closed)
	}

	// Activity entry recorded and included in the client's monthly report.
	entries := st.aktivitasOf("tugas_selesai")
	if len(entries) != 1 || entries[0].Teks != "Selesai: Migrasi server" ||
		deref(entries[0].TugasID) != withClient.ID || deref(entries[0].KlienID) != *withClient.KlienID ||
		entries[0].DurasiMenit == nil || *entries[0].DurasiMenit != 45 ||
		strings.Contains(string(entries[0].Data), "jatuh_tempo_baru") {
		t.Errorf("tugas_selesai = %+v", entries)
	}
	repRec := do(t, h, "GET", "/api/klien/"+*withClient.KlienID+"/laporan?bulan=2026-11", "", c)
	if repRec.Code != http.StatusOK {
		t.Fatalf("laporan: %d %s", repRec.Code, repRec.Body)
	}
	rep := decode[laporanJSON](t, repRec)
	if len(rep.Pekerjaan) != 1 || rep.Pekerjaan[0].Teks != "Selesai: Migrasi server" || rep.Pekerjaan[0].Jenis != "tugas_selesai" {
		t.Errorf("laporan pekerjaan = %+v", rep.Pekerjaan)
	}

	// Gone from active list (default and ?selesai=0), present in ?selesai=1.
	active := decode[tugasList](t, do(t, h, "GET", "/api/tugas", "", c)).Tugas
	if len(active) != 2 {
		t.Fatalf("active list after close: %d %+v", len(active), active)
	}
	for _, tg := range active {
		if tg.ID == withClient.ID {
			t.Errorf("closed one-off still in active list: %+v", tg)
		}
	}
	if active0 := decode[tugasList](t, do(t, h, "GET", "/api/tugas?selesai=0", "", c)).Tugas; len(active0) != 2 {
		t.Errorf("?selesai=0 list = %d, want 2", len(active0))
	}
	closedList := decode[tugasList](t, do(t, h, "GET", "/api/tugas?selesai=1", "", c)).Tugas
	if len(closedList) != 1 || closedList[0].ID != withClient.ID || closedList[0].SelesaiPada == nil {
		t.Fatalf("?selesai=1 list = %+v", closedList)
	}
	if bad := do(t, h, "GET", "/api/tugas?selesai=ya", "", c); bad.Code != http.StatusUnprocessableEntity {
		t.Errorf("?selesai=ya: %d, want 422", bad.Code)
	}

	// Second completion is 409.
	if rec := do(t, h, "POST", "/api/tugas/"+withClient.ID+"/selesai", "", c); rec.Code != http.StatusConflict {
		t.Errorf("second completion: %d %s, want 409", rec.Code, rec.Body)
	}
	// PUT on a closed task is 409 with the expected Indonesian message.
	putRec := do(t, h, "PUT", "/api/tugas/"+withClient.ID,
		`{"judul":"Ubah judul","sekali":true,"jatuh_tempo":"2026-12-01"}`, c)
	if e := decode[apiError](t, putRec); putRec.Code != http.StatusConflict || e.Galat != "Tugas ini sudah selesai." {
		t.Errorf("PUT closed: %d %+v, want 409 'Tugas ini sudah selesai.'", putRec.Code, e)
	}

	// Complete the clientless one-off with an empty body.
	if rec := do(t, h, "POST", "/api/tugas/"+noClient.ID+"/selesai", "", c); rec.Code != http.StatusOK {
		t.Fatalf("complete clientless one-off: %d %s", rec.Code, rec.Body)
	}
	if got := decode[tugasList](t, do(t, h, "GET", "/api/tugas?selesai=1", "", c)).Tugas; len(got) != 2 {
		t.Errorf("?selesai=1 after second close = %d, want 2", len(got))
	}
}

func TestTugasSekaliValidation(t *testing.T) {
	s, _ := newTestServer()
	h := s.Routes()
	c := signUp(t, h, "valid@contoh.id")

	const (
		errSekali   = "Tugas sekali jalan tidak berulang."
		errBiaya    = "Biaya harus angka 0 atau lebih; rupiah tanpa desimal, USD paling banyak 2 desimal."
		errMataUang = "Pilih IDR atau USD."
	)
	for _, tc := range []struct {
		body, field, msg string
	}{
		{`{"judul":"Audit","sekali":true,"jatuh_tempo":"2026-12-01","ulang_jumlah":1}`, "ulang_satuan", errSekali},
		{`{"judul":"Audit","sekali":true,"jatuh_tempo":"2026-12-01","ulang_jumlah":0}`, "ulang_satuan", errSekali},
		{`{"judul":"Audit","sekali":true,"jatuh_tempo":"2026-12-01","ulang_satuan":"bulan"}`, "ulang_satuan", errSekali},
		{`{"judul":"Audit","sekali":true,"jatuh_tempo":"2026-12-01","ulang_jumlah":2,"ulang_satuan":"minggu"}`, "ulang_satuan", errSekali},
		{`{"judul":"Audit","sekali":true,"jatuh_tempo":"2026-12-01","mata_uang":"EUR"}`, "mata_uang", errMataUang},
		{`{"judul":"Audit","sekali":true,"jatuh_tempo":"2026-12-01","mata_uang":"usd"}`, "mata_uang", errMataUang},
		{`{"judul":"Audit","sekali":true,"jatuh_tempo":"2026-12-01","biaya":150000.5}`, "biaya", errBiaya},
		{`{"judul":"Audit","sekali":true,"jatuh_tempo":"2026-12-01","biaya":-1}`, "biaya", errBiaya},
		{`{"judul":"Audit","sekali":true,"jatuh_tempo":"2026-12-01","biaya":1e5}`, "biaya", errBiaya},
		{`{"judul":"Audit","sekali":true,"jatuh_tempo":"2026-12-01","mata_uang":"USD","biaya":12.345}`, "biaya", errBiaya},
		{`{"judul":"Audit","sekali":true,"jatuh_tempo":"2026-12-01","mata_uang":"USD","biaya":-5}`, "biaya", errBiaya},
	} {
		rec := do(t, h, "POST", "/api/tugas", tc.body, c)
		e := decode[apiError](t, rec)
		if rec.Code != http.StatusUnprocessableEntity || e.Field[tc.field] != tc.msg {
			t.Errorf("%s: %d %v, want 422 %s=%q", tc.body, rec.Code, e.Field, tc.field, tc.msg)
		}
	}

	// Bad currency with a numeric biaya reports only mata_uang, not a spurious biaya error.
	rec := do(t, h, "POST", "/api/tugas",
		`{"judul":"Audit","sekali":true,"jatuh_tempo":"2026-12-01","mata_uang":"EUR","biaya":100}`, c)
	if e := decode[apiError](t, rec); rec.Code != http.StatusUnprocessableEntity || e.Field["mata_uang"] != errMataUang || e.Field["biaya"] != "" {
		t.Errorf("bad currency with biaya: %d %v", rec.Code, e.Field)
	}
}

func TestTugasSelesaiDari(t *testing.T) {
	s, st := newTestServer()
	h := s.Routes()
	c := signUp(t, h, "dari@contoh.id")

	oneOff := decode[tugasJSON](t, do(t, h, "POST", "/api/tugas",
		`{"judul":"Kirim faktur","sekali":true,"jatuh_tempo":"2026-11-20"}`, c))
	recurring := decode[tugasJSON](t, do(t, h, "POST", "/api/tugas",
		`{"judul":"Cek backup","jatuh_tempo":"2026-11-20","ulang_jumlah":1,"ulang_satuan":"bulan"}`, c))

	// Invalid dari format is 422.
	rec := do(t, h, "POST", "/api/tugas/"+oneOff.ID+"/selesai", `{"dari":"kemarin"}`, c)
	if e := decode[apiError](t, rec); rec.Code != http.StatusUnprocessableEntity || e.Field["dari"] == "" {
		t.Errorf("bad dari format: %d %v", rec.Code, e.Field)
	}

	// Mismatched dari is 409 for one-off, leaving it open with no activity logged.
	if rec := do(t, h, "POST", "/api/tugas/"+oneOff.ID+"/selesai", `{"dari":"2026-11-19"}`, c); rec.Code != http.StatusConflict {
		t.Errorf("one-off dari mismatch: %d %s, want 409", rec.Code, rec.Body)
	}
	if len(st.aktivitasOf("tugas_selesai")) != 0 {
		t.Errorf("mismatch logged activity")
	}

	// Mismatched dari is 409 for recurring, leaving its due date unchanged.
	if rec := do(t, h, "POST", "/api/tugas/"+recurring.ID+"/selesai", `{"dari":"2026-11-19"}`, c); rec.Code != http.StatusConflict {
		t.Errorf("recurring dari mismatch: %d %s, want 409", rec.Code, rec.Body)
	}
	if got := decode[tugasJSON](t, do(t, h, "GET", "/api/tugas/"+recurring.ID, "", c)); got.JatuhTempo != "2026-11-20" {
		t.Errorf("recurring moved after mismatch: %+v", got)
	}

	// Matching dari succeeds for both kinds.
	if rec := do(t, h, "POST", "/api/tugas/"+oneOff.ID+"/selesai", `{"dari":"2026-11-20"}`, c); rec.Code != http.StatusOK {
		t.Fatalf("one-off matching dari: %d %s", rec.Code, rec.Body)
	}
	rec = do(t, h, "POST", "/api/tugas/"+recurring.ID+"/selesai", `{"dari":"2026-11-20"}`, c)
	if rec.Code != http.StatusOK || decode[tugasJSON](t, rec).JatuhTempo != "2026-12-20" {
		t.Fatalf("recurring matching dari: %d %s", rec.Code, rec.Body)
	}
	// Replaying the old dari on the recurring task is 409.
	if rec := do(t, h, "POST", "/api/tugas/"+recurring.ID+"/selesai", `{"dari":"2026-11-20"}`, c); rec.Code != http.StatusConflict {
		t.Errorf("recurring replayed dari: %d %s, want 409", rec.Code, rec.Body)
	}
}

func TestTugasGet(t *testing.T) {
	s, _ := newTestServer()
	h := s.Routes()
	a := signUp(t, h, "get-a@contoh.id")
	b := signUp(t, h, "get-b@contoh.id")

	oneOff := decode[tugasJSON](t, do(t, h, "POST", "/api/tugas",
		`{"judul":"Pasang sertifikat","sekali":true,"jatuh_tempo":"2026-11-18","biaya":100000}`, a))

	rec := do(t, h, "GET", "/api/tugas/"+oneOff.ID, "", a)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET own task: %d %s", rec.Code, rec.Body)
	}
	raw := decodeRawMap(t, rec.Body.Bytes())
	if raw["id"] != oneOff.ID || raw["sekali"] != true || raw["ulang_jumlah"] != nil || raw["ulang_satuan"] != nil {
		t.Errorf("GET raw = %v", raw)
	}

	// Closed task is still readable via GET /api/tugas/{id}.
	do(t, h, "POST", "/api/tugas/"+oneOff.ID+"/selesai", "", a)
	rec = do(t, h, "GET", "/api/tugas/"+oneOff.ID, "", a)
	if rec.Code != http.StatusOK || decode[tugasJSON](t, rec).SelesaiPada == nil {
		t.Errorf("GET closed task: %d %s", rec.Code, rec.Body)
	}

	for _, tc := range []struct {
		name, path string
		cookie     *http.Cookie
		want       int
	}{
		{"unknown uuid", "/api/tugas/00000000-0000-4000-8000-000000000999", a, http.StatusNotFound},
		{"bad uuid", "/api/tugas/bukan-uuid", a, http.StatusNotFound},
		{"another user", "/api/tugas/" + oneOff.ID, b, http.StatusNotFound},
		{"no session", "/api/tugas/" + oneOff.ID, nil, http.StatusUnauthorized},
	} {
		if rec := do(t, h, "GET", tc.path, "", tc.cookie); rec.Code != tc.want {
			t.Errorf("%s: %d, want %d", tc.name, rec.Code, tc.want)
		}
	}
}

func TestTugasBiayaUSDAndPutKeepsMataUang(t *testing.T) {
	s, st := newTestServer()
	h := s.Routes()
	c := signUp(t, h, "usd@contoh.id")

	// Create a USD task with cents and verify minor-unit storage and major-unit JSON.
	rec := do(t, h, "POST", "/api/tugas",
		`{"judul":"Audit cloud","sekali":true,"jatuh_tempo":"2026-12-01","biaya":20.50,"mata_uang":"USD"}`, c)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create USD: %d %s", rec.Code, rec.Body)
	}
	tg := decode[tugasJSON](t, rec)
	if tg.MataUang != "USD" || tg.Biaya == nil || *tg.Biaya != "20.5" {
		t.Fatalf("created USD = %+v", tg)
	}
	// Stored in minor units (cents).
	stored := st.ruang().tugas[tg.ID]
	if stored.MataUang != "USD" || stored.Biaya == nil || *stored.Biaya != 2050 {
		t.Errorf("stored USD = %+v, want 2050 cents", stored)
	}

	// Round trip through GET /api/tugas/{id}.
	got := decode[tugasJSON](t, do(t, h, "GET", "/api/tugas/"+tg.ID, "", c))
	if got.MataUang != "USD" || got.Biaya == nil || *got.Biaya != "20.5" {
		t.Errorf("GET USD = %+v", got)
	}

	// PUT without mata_uang keeps USD and parses the new biaya in USD.
	rec = do(t, h, "PUT", "/api/tugas/"+tg.ID,
		`{"judul":"Audit cloud lanjutan","sekali":true,"jatuh_tempo":"2026-12-10","biaya":35.75}`, c)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT without mata_uang: %d %s", rec.Code, rec.Body)
	}
	up := decode[tugasJSON](t, rec)
	if up.MataUang != "USD" || up.Biaya == nil || *up.Biaya != "35.75" {
		t.Errorf("updated without mata_uang = %+v", up)
	}
	stored = st.ruang().tugas[tg.ID]
	if stored.MataUang != "USD" || stored.Biaya == nil || *stored.Biaya != 3575 {
		t.Errorf("stored after PUT = %+v, want 3575 cents", stored)
	}

	// PUT without mata_uang and without biaya still keeps USD.
	rec = do(t, h, "PUT", "/api/tugas/"+tg.ID,
		`{"judul":"Audit cloud gratis","sekali":true,"jatuh_tempo":"2026-12-10"}`, c)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT without mata_uang or biaya: %d %s", rec.Code, rec.Body)
	}
	up = decode[tugasJSON](t, rec)
	if up.MataUang != "USD" || up.Biaya != nil {
		t.Errorf("updated no biaya = %+v", up)
	}

	// PUT with explicit mata_uang changes the currency.
	rec = do(t, h, "PUT", "/api/tugas/"+tg.ID,
		`{"judul":"Audit lokal","sekali":true,"jatuh_tempo":"2026-12-10","biaya":500000,"mata_uang":"IDR"}`, c)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT switch to IDR: %d %s", rec.Code, rec.Body)
	}
	up = decode[tugasJSON](t, rec)
	if up.MataUang != "IDR" || up.Biaya == nil || *up.Biaya != "500000" {
		t.Errorf("updated IDR = %+v", up)
	}
}
