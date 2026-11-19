package httpapi

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/mrramdn/tenggat/api/internal/store"
)

func TestTagihanAndCurrencyValidation(t *testing.T) {
	s, st, _ := newRegistryServer()
	h := s.Routes()
	c := signUp(t, h, "tagihan@contoh.id")

	// USD 20.50 round trip: stored as 2050 cents, emitted as 20.5 in JSON.
	rec := do(t, h, "POST", "/api/item",
		`{"nama":"Claude Pro","jenis":"tagihan","klien":"Studio Rana","kedaluwarsa":"2026-11-20","biaya":20.50,"mata_uang":"USD","ulang_jumlah":1,"ulang_satuan":"bulan"}`, c)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create USD bill: %d %s", rec.Code, rec.Body)
	}
	got := decode[itemJSON](t, rec)
	if got.MataUang != "USD" || got.Biaya == nil || string(*got.Biaya) != "20.5" ||
		got.UlangJumlah == nil || *got.UlangJumlah != 1 || got.UlangSatuan == nil || *got.UlangSatuan != "bulan" ||
		got.SelesaiPada != nil {
		t.Errorf("created JSON = %+v (body %s)", got, rec.Body)
	}
	if stored := st.items[got.ID]; stored.Biaya == nil || *stored.Biaya != 2050 || stored.MataUang != "USD" {
		t.Errorf("stored item = %+v", stored)
	}
	fetched := decode[itemJSON](t, do(t, h, "GET", "/api/item/"+got.ID, "", c))
	if fetched.MataUang != "USD" || fetched.Biaya == nil || string(*fetched.Biaya) != "20.5" {
		t.Errorf("GET item = %+v", fetched)
	}

	// PUT without mata_uang keeps USD, and without ulang_* keeps recurrence.
	rec = do(t, h, "PUT", "/api/item/"+got.ID,
		`{"nama":"Claude Pro Team","jenis":"tagihan","klien":"Studio Rana","kedaluwarsa":"2026-11-20","biaya":25.75}`, c)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT without mata_uang/ulang: %d %s", rec.Code, rec.Body)
	}
	upd := decode[itemJSON](t, rec)
	if upd.Nama != "Claude Pro Team" || upd.MataUang != "USD" || upd.Biaya == nil || string(*upd.Biaya) != "25.75" ||
		upd.UlangJumlah == nil || *upd.UlangJumlah != 1 || upd.UlangSatuan == nil || *upd.UlangSatuan != "bulan" {
		t.Errorf("PUT kept fields = %+v", upd)
	}
	if stored := st.items[got.ID]; stored.Biaya == nil || *stored.Biaya != 2575 || stored.MataUang != "USD" {
		t.Errorf("stored after PUT = %+v", stored)
	}

	// PUT with "ulang_satuan": "" removes recurrence while keeping USD.
	rec = do(t, h, "PUT", "/api/item/"+got.ID,
		`{"nama":"Claude Pro Team","jenis":"tagihan","klien":"Studio Rana","kedaluwarsa":"2026-11-20","biaya":25.75,"ulang_satuan":""}`, c)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT clear recurrence: %d %s", rec.Code, rec.Body)
	}
	cleared := decode[itemJSON](t, rec)
	if cleared.MataUang != "USD" || cleared.UlangJumlah != nil || cleared.UlangSatuan != nil {
		t.Errorf("PUT cleared recurrence = %+v", cleared)
	}

	// Validation rejections: host on a bill, 1.5 IDR, recurrence on a certificate or registry domain, bad currency/recurrence.
	cases := []struct {
		body  string
		field string
		want  string
	}{
		{
			`{"nama":"Tagihan Listrik","jenis":"tagihan","host":"pln.co.id","kedaluwarsa":"2026-12-01"}`,
			"host", "Tagihan tidak memakai host.",
		},
		{
			`{"nama":"Domain","jenis":"domain","kedaluwarsa":"2026-12-01","biaya":1.5}`,
			"biaya", "Biaya harus angka 0 atau lebih; rupiah tanpa desimal, USD paling banyak 2 desimal.",
		},
		{
			`{"nama":"VPS","jenis":"langganan","kedaluwarsa":"2026-12-01","biaya":10.123,"mata_uang":"USD"}`,
			"biaya", "Biaya harus angka 0 atau lebih; rupiah tanpa desimal, USD paling banyak 2 desimal.",
		},
		{
			`{"nama":"VPS","jenis":"langganan","kedaluwarsa":"2026-12-01","mata_uang":"EUR"}`,
			"mata_uang", "Pilih IDR atau USD.",
		},
		{
			`{"nama":"Aset","jenis":"tidak_ada","kedaluwarsa":"2026-12-01"}`,
			"jenis", "Pilih jenis: sertifikat, domain, token, langganan, tagihan, atau lainnya.",
		},
		{
			`{"jenis":"sertifikat","host":"studiorana.co.id","ulang_jumlah":1,"ulang_satuan":"bulan"}`,
			"ulang_satuan", "Tanggal sertifikat dan domain dari registri dibaca otomatis, jadi tidak diulang.",
		},
		{
			`{"jenis":"domain","host":"toko.co.id","ulang_jumlah":1,"ulang_satuan":"tahun"}`,
			"ulang_satuan", "Tanggal sertifikat dan domain dari registri dibaca otomatis, jadi tidak diulang.",
		},
		{
			`{"nama":"Tagihan","jenis":"tagihan","kedaluwarsa":"2026-12-01","ulang_jumlah":1}`,
			"ulang_satuan", "Pilih satuan: hari, minggu, bulan, atau tahun.",
		},
		{
			`{"nama":"Tagihan","jenis":"tagihan","kedaluwarsa":"2026-12-01","ulang_satuan":"bulan"}`,
			"ulang_jumlah", "Isi angka 1 sampai 365.",
		},
		{
			`{"nama":"Tagihan","jenis":"tagihan","kedaluwarsa":"2026-12-01","ulang_jumlah":400,"ulang_satuan":"bulan"}`,
			"ulang_jumlah", "Isi angka 1 sampai 365.",
		},
	}
	for _, tc := range cases {
		rec := do(t, h, "POST", "/api/item", tc.body, c)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s: status %d, want 422 (%s)", tc.body, rec.Code, rec.Body)
			continue
		}
		if e := decode[apiError](t, rec); e.Field[tc.field] != tc.want {
			t.Errorf("%s: field[%q] = %q, want %q", tc.body, tc.field, e.Field[tc.field], tc.want)
		}
	}
}

func TestPerpanjangAndClosedItems(t *testing.T) {
	s, st := newTestServer()
	h := s.Routes()
	c := signUp(t, h, "perpanjang@contoh.id")

	// Recurring monthly bill due today (9 Nov 2026): /perpanjang adds one month and logs tagihan_dibayar.
	bill := decode[itemJSON](t, do(t, h, "POST", "/api/item",
		`{"nama":"Listrik","jenis":"tagihan","klien":"Studio Rana","kedaluwarsa":"2026-11-09","biaya":350000,"ulang_jumlah":1,"ulang_satuan":"bulan"}`, c))

	// Wrong dari is 409 and changes nothing.
	rec := do(t, h, "POST", "/api/item/"+bill.ID+"/perpanjang", `{"dari":"2026-10-09"}`, c)
	if rec.Code != http.StatusConflict {
		t.Fatalf("wrong dari: %d %s, want 409", rec.Code, rec.Body)
	}
	if got := decode[itemJSON](t, do(t, h, "GET", "/api/item/"+bill.ID, "", c)); got.Kedaluwarsa != "2026-11-09" {
		t.Errorf("item changed after 409: %+v", got)
	}
	if len(st.aktivitasOf("tagihan_dibayar")) != 0 {
		t.Errorf("wrong dari logged tagihan_dibayar")
	}

	// Paying the bill moves it by one month to 2026-12-09 and logs tagihan_dibayar with the amount.
	rec = do(t, h, "POST", "/api/item/"+bill.ID+"/perpanjang", `{"dari":"2026-11-09","catatan":"Bayar lewat m-banking"}`, c)
	if rec.Code != http.StatusOK {
		t.Fatalf("perpanjang bill: %d %s", rec.Code, rec.Body)
	}
	paid := decode[itemJSON](t, rec)
	if paid.Kedaluwarsa != "2026-12-09" || paid.SelesaiPada != nil {
		t.Errorf("paid bill = %+v", paid)
	}
	akt := decode[aktivitasList](t, do(t, h, "GET", "/api/aktivitas", "", c)).Aktivitas
	if len(akt) == 0 || akt[0].Jenis != "tagihan_dibayar" ||
		akt[0].Teks != "Tagihan Listrik dibayar Rp 350.000, berikutnya 9 Des 2026" ||
		akt[0].Jumlah == nil || string(*akt[0].Jumlah) != "350000" || deref(akt[0].MataUang) != "IDR" {
		t.Fatalf("tagihan_dibayar activity = %+v", akt)
	}
	var aktData map[string]any
	if err := json.Unmarshal(akt[0].Data, &aktData); err != nil ||
		aktData["dari"] != "2026-11-09" || aktData["sampai"] != "2026-12-09" ||
		aktData["lunas"] != false || aktData["catatan"] != "Bayar lewat m-banking" {
		t.Errorf("tagihan_dibayar data = %s", akt[0].Data)
	}
	if got := st.aktivitasOf("aset_diperpanjang"); len(got) != 0 {
		t.Errorf("perpanjang bill logged aset_diperpanjang: %+v", got)
	}

	// Overdue recurring bill advances by exactly one period even when the new date is still in the past.
	overdue := decode[itemJSON](t, do(t, h, "POST", "/api/item",
		`{"nama":"Internet","jenis":"tagihan","kedaluwarsa":"2026-09-09","biaya":400000,"ulang_jumlah":1,"ulang_satuan":"bulan"}`, c))
	rec = do(t, h, "POST", "/api/item/"+overdue.ID+"/perpanjang", "", c)
	if rec.Code != http.StatusOK || decode[itemJSON](t, rec).Kedaluwarsa != "2026-10-09" {
		t.Errorf("overdue perpanjang: %d %s, want 2026-10-09", rec.Code, rec.Body)
	}

	// One-off bill without sampai is closed (lunas), disappears from GET /api/item and GET /api/ringkasan,
	// and shows with GET /api/item?selesai=1.
	once := decode[itemJSON](t, do(t, h, "POST", "/api/item",
		`{"nama":"Desain Logo","jenis":"tagihan","kedaluwarsa":"2026-11-15","biaya":1500000}`, c))
	rec = do(t, h, "POST", "/api/item/"+once.ID+"/perpanjang", "", c)
	if rec.Code != http.StatusOK {
		t.Fatalf("perpanjang one-off bill: %d %s", rec.Code, rec.Body)
	}
	closed := decode[itemJSON](t, rec)
	if closed.SelesaiPada == nil {
		t.Fatalf("one-off bill not closed: %+v", closed)
	}
	akt = decode[aktivitasList](t, do(t, h, "GET", "/api/aktivitas", "", c)).Aktivitas
	if akt[0].Jenis != "tagihan_dibayar" || akt[0].Teks != "Tagihan Desain Logo dibayar Rp 1.500.000, lunas" {
		t.Errorf("closed bill activity = %+v", akt[0])
	}

	activeList := decode[struct{ Item []itemJSON }](t, do(t, h, "GET", "/api/item", "", c)).Item
	for _, it := range activeList {
		if it.ID == once.ID {
			t.Errorf("closed bill still in GET /api/item")
		}
	}
	if sum := decode[store.Summary](t, do(t, h, "GET", "/api/ringkasan", "", c)); sum.Total != 2 {
		t.Errorf("summary Total = %d, want 2 active items", sum.Total)
	}
	closedList := decode[struct{ Item []itemJSON }](t, do(t, h, "GET", "/api/item?selesai=1", "", c)).Item
	if len(closedList) != 1 || closedList[0].ID != once.ID || closedList[0].SelesaiPada == nil {
		t.Errorf("GET /api/item?selesai=1 = %+v", closedList)
	}

	// PUT on a closed item is 409.
	rec = do(t, h, "PUT", "/api/item/"+once.ID,
		`{"nama":"Desain Logo","jenis":"tagihan","kedaluwarsa":"2026-11-20","biaya":1500000}`, c)
	if rec.Code != http.StatusConflict || decode[apiError](t, rec).Galat != "Aset ini sudah ditutup." {
		t.Errorf("PUT on closed item: %d %s", rec.Code, rec.Body)
	}

	// A certificate cannot be renewed via /perpanjang (422).
	cert := decode[itemJSON](t, do(t, h, "POST", "/api/item", `{"jenis":"sertifikat","host":"studiorana.co.id"}`, c))
	rec = do(t, h, "POST", "/api/item/"+cert.ID+"/perpanjang", `{"sampai":"2027-11-28"}`, c)
	if rec.Code != http.StatusUnprocessableEntity ||
		decode[apiError](t, rec).Galat != "Sertifikat dan domain dari registri diperbarui lewat cek ulang." {
		t.Errorf("perpanjang certificate: %d %s", rec.Code, rec.Body)
	}

	// Non-recurring manual non-bill without sampai is 422 on sampai; with sampai it logs aset_diperpanjang once.
	dom := decode[itemJSON](t, do(t, h, "POST", "/api/item",
		`{"nama":"toko.io","jenis":"domain","host":"toko.io","kedaluwarsa":"2026-11-20","biaya":12.50,"mata_uang":"USD"}`, c))
	oldDom := st.items[dom.ID]
	rec = do(t, h, "POST", "/api/item/"+dom.ID+"/perpanjang", "", c)
	if rec.Code != http.StatusUnprocessableEntity || decode[apiError](t, rec).Field["sampai"] != "Isi tanggal berakhir yang baru." {
		t.Errorf("non-recurring domain without sampai: %d %s", rec.Code, rec.Body)
	}
	rec = do(t, h, "POST", "/api/item/"+dom.ID+"/perpanjang", `{"sampai":"2027-11-20"}`, c)
	if rec.Code != http.StatusOK || decode[itemJSON](t, rec).Kedaluwarsa != "2027-11-20" {
		t.Fatalf("perpanjang domain: %d %s", rec.Code, rec.Body)
	}
	// catatAsetDiubah must not log a second aset_diperpanjang for a change made through /perpanjang.
	s.catatAsetDiubah(t.Context(), oldDom, st.items[dom.ID])
	if got := st.aktivitasOf("aset_diperpanjang"); len(got) != 1 || got[0].Teks != "toko.io diperpanjang sampai 20 Nov 2027" ||
		got[0].Jumlah == nil || *got[0].Jumlah != 1250 || deref(got[0].MataUang) != "USD" {
		t.Errorf("aset_diperpanjang from /perpanjang = %+v", got)
	}

	// Auth and 404 checks on /perpanjang.
	if rec := do(t, h, "POST", "/api/item/"+bill.ID+"/perpanjang", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("perpanjang without session: %d, want 401", rec.Code)
	}
	if rec := do(t, h, "POST", "/api/item/00000000-0000-4000-8000-999999999999/perpanjang", "", c); rec.Code != http.StatusNotFound {
		t.Errorf("perpanjang unknown id: %d, want 404", rec.Code)
	}
	for body, field := range map[string]string{
		`{"sampai":"2026-12-09"}`:                        "sampai",
		`{"jumlah":1.5}`:                                 "jumlah",
		`{"mata_uang":"EUR"}`:                            "mata_uang",
		`{"tanggal":"2026-11-10"}`:                       "tanggal",
		`{"catatan":"` + strings.Repeat("x", 501) + `"}`: "catatan",
	} {
		rec := do(t, h, "POST", "/api/item/"+bill.ID+"/perpanjang", body, c)
		if e := decode[apiError](t, rec); rec.Code != http.StatusUnprocessableEntity || e.Field[field] == "" {
			t.Errorf("perpanjang %s: %d %v, want 422 on %s", body, rec.Code, e.Field, field)
		}
	}
}

func TestPerCurrencyFiguresAndCSV(t *testing.T) {
	s, _ := newTestServer()
	h := s.Routes()
	c := signUp(t, h, "uang@contoh.id")
	k := decode[klienJSON](t, do(t, h, "POST", "/api/klien", `{"nama":"Studio Rana"}`, c))
	kid := `"klien_id":"` + k.ID + `"`

	// Two IDR items and one USD item due within 30 days, one USD item past 30 days (due in 40 days, inside the
	// 60-day report due-soon window), and one one-off IDR bill that gets closed.
	for _, body := range []string{
		`{"nama":"VPS","jenis":"langganan","kedaluwarsa":"2026-11-20","biaya":350000,` + kid + `}`,
		`{"nama":"Domain","jenis":"domain","host":"studiorana.id","kedaluwarsa":"2026-11-25","biaya":124000,` + kid + `}`,
		`{"nama":"Claude Pro","jenis":"tagihan","kedaluwarsa":"2026-11-18","biaya":20.50,"mata_uang":"USD","ulang_jumlah":1,"ulang_satuan":"bulan",` + kid + `}`,
		`{"nama":"Backup S3","jenis":"langganan","kedaluwarsa":"2026-12-19","biaya":5.25,"mata_uang":"USD",` + kid + `}`,
	} {
		if rec := do(t, h, "POST", "/api/item", body, c); rec.Code != http.StatusCreated {
			t.Fatalf("create item: %d %s", rec.Code, rec.Body)
		}
	}
	once := decode[itemJSON](t, do(t, h, "POST", "/api/item",
		`{"nama":"Setup Awal","jenis":"tagihan","kedaluwarsa":"2026-11-12","biaya":900000,`+kid+`}`, c))
	do(t, h, "POST", "/api/item/"+once.ID+"/perpanjang", "", c)

	// GET /api/klien keeps biaya_30_hari in IDR and adds biaya_30_hari_per_mata_uang in major units.
	clients := decode[klienList](t, do(t, h, "GET", "/api/klien", "", c)).Klien
	if len(clients) != 1 {
		t.Fatalf("clients = %+v", clients)
	}
	if got := clients[0]; got.Biaya30Hari != 474000 ||
		string(got.Biaya30HariPer["IDR"]) != "474000" || string(got.Biaya30HariPer["USD"]) != "20.5" ||
		len(got.Biaya30HariPer) != 2 {
		t.Errorf("klien figures = %+v", got)
	}

	// Client report sums per currency without mixing IDR and USD, and emits mata_uang and major-unit biaya.
	repRec := do(t, h, "GET", "/api/klien/"+k.ID+"/laporan?bulan=2026-11", "", c)
	if repRec.Code != http.StatusOK {
		t.Fatalf("laporan: %d %s", repRec.Code, repRec.Body)
	}
	rep := decode[laporanJSON](t, repRec)
	if rep.Ringkasan.Biaya30Hari != 474000 ||
		string(rep.Ringkasan.Biaya30HariPer["IDR"]) != "474000" ||
		string(rep.Ringkasan.Biaya30HariPer["USD"]) != "20.5" ||
		len(rep.Ringkasan.Biaya30HariPer) != 2 {
		t.Errorf("report ringkasan = %+v", rep.Ringkasan)
	}
	if len(rep.Aset) != 4 || rep.Aset[0].Nama != "Claude Pro" || rep.Aset[0].MataUang != "USD" ||
		rep.Aset[0].Biaya == nil || string(*rep.Aset[0].Biaya) != "20.5" {
		t.Errorf("report aset = %+v", rep.Aset)
	}
	if len(rep.JatuhTempo) != 4 || rep.JatuhTempo[0].Nama != "Claude Pro" || rep.JatuhTempo[0].MataUang != "USD" ||
		rep.JatuhTempo[0].Biaya == nil || string(*rep.JatuhTempo[0].Biaya) != "20.5" ||
		rep.JatuhTempo[3].Nama != "Backup S3" || rep.JatuhTempo[3].MataUang != "USD" ||
		rep.JatuhTempo[3].Biaya == nil || string(*rep.JatuhTempo[3].Biaya) != "5.25" {
		t.Errorf("report jatuh_tempo = %+v", rep.JatuhTempo)
	}

	// CSV export has major-unit biaya and trailing mata_uang and ulang columns.
	csvRec := do(t, h, "GET", "/api/item.csv", "", c)
	if csvRec.Code != http.StatusOK {
		t.Fatalf("csv: %d", csvRec.Code)
	}
	rows, err := csv.NewReader(bytes.NewReader(csvRec.Body.Bytes()[3:])).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 5 || rows[0][8] != "biaya" || rows[0][10] != "mata_uang" || rows[0][11] != "ulang" {
		t.Fatalf("csv header = %q", rows[0])
	}
	if rows[1][0] != "Claude Pro" || rows[1][1] != "tagihan" || rows[1][8] != "20.5" || rows[1][10] != "USD" || rows[1][11] != "1 bulan" {
		t.Errorf("csv USD recurring row = %q", rows[1])
	}
	if rows[2][0] != "VPS" || rows[2][8] != "350000" || rows[2][10] != "IDR" || rows[2][11] != "" {
		t.Errorf("csv IDR non-recurring row = %q", rows[2])
	}

	// iCalendar export labels a tagihan as "Tagihan".
	icsBody := do(t, h, "GET", "/api/kalender.ics", "", c).Body.String()
	if !strings.Contains(icsBody, `DESCRIPTION:Tagihan\nKlien: Studio Rana`) {
		t.Errorf("calendar lacks Tagihan description:\n%s", icsBody)
	}
}
