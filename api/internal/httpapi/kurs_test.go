package httpapi

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/mrramdn/tenggat/api/internal/kurs"
	"github.com/mrramdn/tenggat/api/internal/store"
)

type fakeKurs struct {
	mu    sync.Mutex
	val   kurs.Kurs
	err   error
	calls int
}

func (f *fakeKurs) Ambil(context.Context) (kurs.Kurs, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.err != nil {
		return kurs.Kurs{}, f.err
	}
	return f.val, nil
}

func (f *fakeKurs) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func (f *fakeKurs) set(val kurs.Kurs, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.val, f.err = val, err
}

func sampleKurs() kurs.Kurs {
	return kurs.Kurs{
		Tanggal: "2026-11-18",
		PerIDR: map[string]float64{
			"USD": 18254,
			"SGD": 14155.2,
		},
	}
}

func TestKursEndpointCachingAndBackoff(t *testing.T) {
	// 1. Feature off (s.Kurs == nil): 200 with null tanggal and per_idr.
	sOff, _ := newTestServer()
	hOff := sOff.Routes()
	if rec := do(t, hOff, "GET", "/api/kurs", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("GET /api/kurs without session: %d, want 401", rec.Code)
	}
	cOff := signUp(t, hOff, "off@contoh.id")
	rec := do(t, hOff, "GET", "/api/kurs", "", cOff)
	if rec.Code != http.StatusOK {
		t.Fatalf("feature off GET /api/kurs: %d %s", rec.Code, rec.Body)
	}
	raw := decodeRawMap(t, rec.Body.Bytes())
	if raw["tanggal"] != nil || raw["per_idr"] != nil || raw["sumber"] != kurs.NamaSumber {
		t.Errorf("feature off /api/kurs = %v", raw)
	}

	// 2. Rates present and globally cached across two different users.
	s, _ := newTestServer()
	mc := newMemCache()
	s.Cache = mc
	fk := &fakeKurs{val: sampleKurs()}
	s.Kurs = fk
	h := s.Routes()
	a := signUp(t, h, "kurs-a@contoh.id")
	b := signUp(t, h, "kurs-b@contoh.id")

	rec = do(t, h, "GET", "/api/kurs", "", a)
	if rec.Code != http.StatusOK {
		t.Fatalf("user A /api/kurs: %d %s", rec.Code, rec.Body)
	}
	got := decode[kursJSON](t, rec)
	if got.Tanggal == nil || *got.Tanggal != "2026-11-18" ||
		got.PerIDR["USD"] != 18254 || got.PerIDR["SGD"] != 14155.2 ||
		got.Sumber != kurs.NamaSumber {
		t.Errorf("user A /api/kurs = %+v", got)
	}

	// User B reads the same global cache key without calling the source again.
	rec = do(t, h, "GET", "/api/kurs", "", b)
	if rec.Code != http.StatusOK {
		t.Fatalf("user B /api/kurs: %d %s", rec.Code, rec.Body)
	}
	if fk.count() != 1 {
		t.Errorf("source calls across two users = %d, want 1", fk.count())
	}

	// 3. Source failing backs off for 10 minutes without calling source twice, then retries and recovers.
	now := testNow
	sFail, _ := newTestServer()
	sFail.Cache = newMemCache()
	sFail.Now = func() time.Time { return now }
	fkFail := &fakeKurs{err: errors.New("upstream down")}
	sFail.Kurs = fkFail
	hFail := sFail.Routes()
	cFail := signUp(t, hFail, "gagal@contoh.id")

	rec = do(t, hFail, "GET", "/api/kurs", "", cFail)
	if rec.Code != http.StatusOK {
		t.Fatalf("failing /api/kurs 1: %d %s", rec.Code, rec.Body)
	}
	if m := decodeRawMap(t, rec.Body.Bytes()); m["tanggal"] != nil || m["per_idr"] != nil || m["sumber"] != kurs.NamaSumber {
		t.Errorf("failing /api/kurs 1 = %v", m)
	}
	if fkFail.count() != 1 {
		t.Fatalf("calls after first failure = %d, want 1", fkFail.count())
	}

	// Still inside the 10-minute window (5 minutes later): must not call source again.
	now = now.Add(5 * time.Minute)
	rec = do(t, hFail, "GET", "/api/kurs", "", cFail)
	if rec.Code != http.StatusOK {
		t.Fatalf("failing /api/kurs 2: %d %s", rec.Code, rec.Body)
	}
	if fkFail.count() != 1 {
		t.Errorf("calls within 10-minute backoff = %d, want 1", fkFail.count())
	}

	// After 10 minutes total, the source is retried and succeeds.
	fkFail.set(sampleKurs(), nil)
	now = now.Add(5 * time.Minute)
	rec = do(t, hFail, "GET", "/api/kurs", "", cFail)
	if rec.Code != http.StatusOK {
		t.Fatalf("recovered /api/kurs: %d %s", rec.Code, rec.Body)
	}
	recov := decode[kursJSON](t, rec)
	if fkFail.count() != 2 || recov.Tanggal == nil || *recov.Tanggal != "2026-11-18" || recov.PerIDR["USD"] != 18254 {
		t.Errorf("after recovery: calls=%d resp=%+v", fkFail.count(), recov)
	}
}

func TestAgendaKiraIDR(t *testing.T) {
	d := func(y int, m time.Month, day int) time.Time { return time.Date(y, m, day, 0, 0, 0, 0, time.UTC) }

	seed := func(t *testing.T, s *Server, st *memStore) (http.Handler, *http.Cookie, string, string, string, string) {
		t.Helper()
		h := s.Routes()
		c := signUp(t, h, "ag-kurs@contoh.id")
		uid := userIDOf(t, h, c)
		ctx := t.Context()

		idrBill, _ := st.Create(ctx, uid, store.Input{
			Nama: "Listrik", Jenis: "tagihan", Kedaluwarsa: d(2026, 11, 10),
			Sumber: "manual", Biaya: i64(500000), MataUang: "IDR",
		})
		usdBill, _ := st.Create(ctx, uid, store.Input{
			Nama: "Claude Pro", Jenis: "tagihan", Kedaluwarsa: d(2026, 11, 12),
			Sumber: "manual", Biaya: i64(2050), MataUang: "USD",
		})
		sgdTask, _ := st.CreateTugas(ctx, uid, store.TugasInput{
			Judul: "VPS SG", JatuhTempo: d(2026, 11, 15), Biaya: i64(1000), MataUang: "SGD",
		})
		freeTask, _ := st.CreateTugas(ctx, uid, store.TugasInput{
			Judul: "Cek log", JatuhTempo: d(2026, 11, 18),
		})
		return h, c, idrBill.ID, usdBill.ID, sgdTask.ID, freeTask.ID
	}

	// 1. Rates present: IDR keeps exact amount, USD and SGD get rounded estimates, unpriced is null.
	s, st := newTestServer()
	s.Kurs = &fakeKurs{val: sampleKurs()}
	h, c, idrID, usdID, sgdID, freeID := seed(t, s, st)

	rec := do(t, h, "GET", "/api/agenda", "", c)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/agenda with rates: %d %s", rec.Code, rec.Body)
	}
	got := decode[agendaResponse](t, rec)
	if got.KursTanggal == nil || *got.KursTanggal != "2026-11-18" {
		t.Errorf("kurs_tanggal = %v, want 2026-11-18", got.KursTanggal)
	}
	// 500000 + round(20.50*18254=374207) + round(10.00*14155.2=141552) = 1015759
	if got.KiraIDRTotal == nil || *got.KiraIDRTotal != 1015759 {
		t.Errorf("kira_idr_total = %v, want 1015759", got.KiraIDRTotal)
	}
	byID := map[string]agendaRow{}
	for _, r := range got.Agenda {
		byID[r.ID] = r
	}
	if byID[idrID].KiraIDR == nil || *byID[idrID].KiraIDR != 500000 {
		t.Errorf("IDR row kira_idr = %v, want 500000", byID[idrID].KiraIDR)
	}
	if byID[usdID].KiraIDR == nil || *byID[usdID].KiraIDR != 374207 {
		t.Errorf("USD row kira_idr = %v, want 374207", byID[usdID].KiraIDR)
	}
	if byID[sgdID].KiraIDR == nil || *byID[sgdID].KiraIDR != 141552 {
		t.Errorf("SGD row kira_idr = %v, want 141552", byID[sgdID].KiraIDR)
	}
	if byID[freeID].KiraIDR != nil {
		t.Errorf("unpriced row kira_idr = %v, want nil", byID[freeID].KiraIDR)
	}

	// Filtered to only unpriced rows: kira_idr_total is 0.
	onlyFree := decode[agendaResponse](t, do(t, h, "GET", "/api/agenda?q=cek+log", "", c))
	if onlyFree.KiraIDRTotal == nil || *onlyFree.KiraIDRTotal != 0 {
		t.Errorf("unpriced filter kira_idr_total = %v, want 0", onlyFree.KiraIDRTotal)
	}

	// 2. Source failing: never fails request; IDR row keeps estimate, foreign rows and total become null.
	sFail, stFail := newTestServer()
	sFail.Kurs = &fakeKurs{err: errors.New("dead")}
	hFail, cFail, idrFail, usdFail, sgdFail, _ := seed(t, sFail, stFail)

	recFail := do(t, hFail, "GET", "/api/agenda", "", cFail)
	if recFail.Code != http.StatusOK {
		t.Fatalf("GET /api/agenda source failing: %d %s", recFail.Code, recFail.Body)
	}
	gotFail := decode[agendaResponse](t, recFail)
	if gotFail.KursTanggal != nil || gotFail.KiraIDRTotal != nil {
		t.Errorf("failing agenda: kurs_tanggal=%v kira_idr_total=%v, want nil/nil", gotFail.KursTanggal, gotFail.KiraIDRTotal)
	}
	failByID := map[string]agendaRow{}
	for _, r := range gotFail.Agenda {
		failByID[r.ID] = r
	}
	if failByID[idrFail].KiraIDR == nil || *failByID[idrFail].KiraIDR != 500000 {
		t.Errorf("failing agenda IDR row kira_idr = %v, want 500000", failByID[idrFail].KiraIDR)
	}
	if failByID[usdFail].KiraIDR != nil || failByID[sgdFail].KiraIDR != nil {
		t.Errorf("failing agenda foreign rows kira_idr = %v, %v; want nil", failByID[usdFail].KiraIDR, failByID[sgdFail].KiraIDR)
	}

	// When filtered to only IDR rows, kira_idr_total is still known even without rates.
	idrOnly := decode[agendaResponse](t, do(t, hFail, "GET", "/api/agenda?q=listrik", "", cFail))
	if idrOnly.KiraIDRTotal == nil || *idrOnly.KiraIDRTotal != 500000 || idrOnly.KursTanggal != nil {
		t.Errorf("failing agenda IDR-only filter = %+v", idrOnly)
	}

	// 3. Feature off (s.Kurs == nil): same graceful degradation.
	sOff, stOff := newTestServer()
	hOff, cOff, idrOff, usdOff, _, _ := seed(t, sOff, stOff)
	gotOff := decode[agendaResponse](t, do(t, hOff, "GET", "/api/agenda", "", cOff))
	if gotOff.KursTanggal != nil || gotOff.KiraIDRTotal != nil {
		t.Errorf("feature off agenda: kurs_tanggal=%v kira_idr_total=%v, want nil/nil", gotOff.KursTanggal, gotOff.KiraIDRTotal)
	}
	offByID := map[string]agendaRow{}
	for _, r := range gotOff.Agenda {
		offByID[r.ID] = r
	}
	if offByID[idrOff].KiraIDR == nil || *offByID[idrOff].KiraIDR != 500000 || offByID[usdOff].KiraIDR != nil {
		t.Errorf("feature off agenda rows: IDR=%v USD=%v", offByID[idrOff].KiraIDR, offByID[usdOff].KiraIDR)
	}
}

func TestKlienBiaya30HariKiraIDR(t *testing.T) {
	seedClients := func(t *testing.T, s *Server) (http.Handler, *http.Cookie, string) {
		t.Helper()
		h := s.Routes()
		c := signUp(t, h, "kl-kurs@contoh.id")

		kCampur := decode[klienJSON](t, do(t, h, "POST", "/api/klien", `{"nama":"Alfa Campur"}`, c))
		kIDR := decode[klienJSON](t, do(t, h, "POST", "/api/klien", `{"nama":"Beta Rupiah"}`, c))
		do(t, h, "POST", "/api/klien", `{"nama":"Citra Kosong"}`, c)

		for _, body := range []string{
			`{"nama":"VPS IDR","jenis":"tagihan","kedaluwarsa":"2026-11-20","biaya":350000,"klien_id":"` + kCampur.ID + `"}`,
			`{"nama":"Claude","jenis":"tagihan","kedaluwarsa":"2026-11-18","biaya":20.50,"mata_uang":"USD","klien_id":"` + kCampur.ID + `"}`,
			`{"nama":"DO SG","jenis":"tagihan","kedaluwarsa":"2026-11-22","biaya":10.00,"mata_uang":"SGD","klien_id":"` + kCampur.ID + `"}`,
			`{"nama":"Domain","jenis":"domain","kedaluwarsa":"2026-11-25","biaya":200000,"klien_id":"` + kIDR.ID + `"}`,
		} {
			if rec := do(t, h, "POST", "/api/item", body, c); rec.Code != http.StatusCreated {
				t.Fatalf("create item: %d %s", rec.Code, rec.Body)
			}
		}
		return h, c, kCampur.ID
	}

	// 1. Rates present.
	s, _ := newTestServer()
	s.Kurs = &fakeKurs{val: sampleKurs()}
	h, c, campurID := seedClients(t, s)

	list := decode[klienList](t, do(t, h, "GET", "/api/klien", "", c)).Klien
	if len(list) != 3 {
		t.Fatalf("clients = %+v", list)
	}
	// Alfa Campur: 350000 + 374207 + 141552 = 865759
	if list[0].Biaya30HariKiraIDR == nil || *list[0].Biaya30HariKiraIDR != 865759 {
		t.Errorf("Alfa Campur biaya_30_hari_kira_idr = %v, want 865759", list[0].Biaya30HariKiraIDR)
	}
	if list[1].Biaya30HariKiraIDR == nil || *list[1].Biaya30HariKiraIDR != 200000 {
		t.Errorf("Beta Rupiah biaya_30_hari_kira_idr = %v, want 200000", list[1].Biaya30HariKiraIDR)
	}
	if list[2].Biaya30HariKiraIDR == nil || *list[2].Biaya30HariKiraIDR != 0 {
		t.Errorf("Citra Kosong biaya_30_hari_kira_idr = %v, want 0", list[2].Biaya30HariKiraIDR)
	}

	// PUT /api/klien/{id} returns the same estimate as GET /api/klien.
	upd := decode[klienJSON](t, do(t, h, "PUT", "/api/klien/"+campurID, `{"nama":"Alfa Campur"}`, c))
	if upd.Biaya30HariKiraIDR == nil || *upd.Biaya30HariKiraIDR != 865759 {
		t.Errorf("PUT Alfa Campur biaya_30_hari_kira_idr = %v, want 865759", upd.Biaya30HariKiraIDR)
	}

	// 2. Source failing: mixed client gets null, IDR-only keeps 200000, empty keeps 0.
	sFail, _ := newTestServer()
	sFail.Kurs = &fakeKurs{err: errors.New("dead")}
	hFail, cFail, _ := seedClients(t, sFail)

	failList := decode[klienList](t, do(t, hFail, "GET", "/api/klien", "", cFail)).Klien
	if failList[0].Biaya30HariKiraIDR != nil {
		t.Errorf("failing Alfa Campur biaya_30_hari_kira_idr = %v, want nil", failList[0].Biaya30HariKiraIDR)
	}
	if failList[1].Biaya30HariKiraIDR == nil || *failList[1].Biaya30HariKiraIDR != 200000 {
		t.Errorf("failing Beta Rupiah biaya_30_hari_kira_idr = %v, want 200000", failList[1].Biaya30HariKiraIDR)
	}
	if failList[2].Biaya30HariKiraIDR == nil || *failList[2].Biaya30HariKiraIDR != 0 {
		t.Errorf("failing Citra Kosong biaya_30_hari_kira_idr = %v, want 0", failList[2].Biaya30HariKiraIDR)
	}

	// 3. Feature off (s.Kurs == nil): same as failing.
	sOff, _ := newTestServer()
	hOff, cOff, _ := seedClients(t, sOff)
	offList := decode[klienList](t, do(t, hOff, "GET", "/api/klien", "", cOff)).Klien
	if offList[0].Biaya30HariKiraIDR != nil ||
		offList[1].Biaya30HariKiraIDR == nil || *offList[1].Biaya30HariKiraIDR != 200000 ||
		offList[2].Biaya30HariKiraIDR == nil || *offList[2].Biaya30HariKiraIDR != 0 {
		t.Errorf("feature off clients = %+v", offList)
	}
}
