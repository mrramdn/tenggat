package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mrramdn/tenggat/api/internal/expiry"
	"github.com/mrramdn/tenggat/api/internal/store"
	"github.com/mrramdn/tenggat/api/internal/tlscheck"
)

// memStore is an in-memory Store for handler tests.
type memStore struct {
	mu      sync.Mutex
	items   map[string]store.Item
	next    int
	pingErr error
}

func newMemStore() *memStore { return &memStore{items: map[string]store.Item{}} }

func (m *memStore) Ping(context.Context) error { return m.pingErr }

func (m *memStore) List(_ context.Context, f store.Filter, now time.Time) ([]store.Item, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []store.Item{}
	for _, it := range m.items {
		if f.Status != "" && expiry.For(expiry.DaysLeft(it.Kedaluwarsa, now)) != f.Status {
			continue
		}
		if f.Klien != "" && it.Klien != f.Klien {
			continue
		}
		out = append(out, it)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Kedaluwarsa.Before(out[j].Kedaluwarsa) })
	return out, nil
}

func (m *memStore) Get(_ context.Context, id string) (store.Item, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	it, ok := m.items[id]
	if !ok {
		return it, store.ErrNotFound
	}
	return it, nil
}

func (m *memStore) Create(_ context.Context, in store.Input) (store.Item, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.next++
	it := fromInput(fmt.Sprintf("00000000-0000-4000-8000-%012d", m.next), in)
	m.items[it.ID] = it
	return it, nil
}

func (m *memStore) Update(_ context.Context, id string, in store.Input) (store.Item, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.items[id]; !ok {
		return store.Item{}, store.ErrNotFound
	}
	it := fromInput(id, in)
	m.items[id] = it
	return it, nil
}

func (m *memStore) Delete(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.items[id]; !ok {
		return store.ErrNotFound
	}
	delete(m.items, id)
	return nil
}

func (m *memStore) SaveCheck(_ context.Context, id string, date time.Time, info store.TLSInfo, at time.Time) (store.Item, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	it := m.items[id]
	it.Kedaluwarsa, it.InfoTLS, it.TerakhirDicek, it.GalatCek = date, &info, &at, nil
	m.items[id] = it
	return it, nil
}

func (m *memStore) SaveCheckError(_ context.Context, id, message string, at time.Time) (store.Item, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	it := m.items[id]
	it.TerakhirDicek, it.GalatCek = &at, &message
	m.items[id] = it
	return it, nil
}

func (m *memStore) DueForCheck(context.Context, time.Time) ([]store.Item, error) { return nil, nil }

func (m *memStore) Summary(_ context.Context, now time.Time) (store.Summary, error) {
	items, _ := m.List(context.Background(), store.Filter{}, now)
	sum := store.Summary{Klien: []string{}}
	for _, it := range items {
		switch expiry.For(expiry.DaysLeft(it.Kedaluwarsa, now)) {
		case expiry.Lewat:
			sum.Lewat++
		case expiry.Kritis:
			sum.Kritis++
		case expiry.Segera:
			sum.Segera++
		default:
			sum.Aman++
		}
		sum.Total++
	}
	return sum, nil
}

func fromInput(id string, in store.Input) store.Item {
	return store.Item{ID: id, Nama: in.Nama, Jenis: in.Jenis, Klien: in.Klien, Host: in.Host,
		Kedaluwarsa: in.Kedaluwarsa, Sumber: in.Sumber, Biaya: in.Biaya, Catatan: in.Catatan,
		InfoTLS: in.InfoTLS, TerakhirDicek: in.Dicek}
}

// fakeChecker answers by host name.
type fakeChecker map[string]error

var certEnd = time.Date(2026, 11, 28, 12, 0, 0, 0, time.UTC)

func (f fakeChecker) Check(_ context.Context, host string) (*tlscheck.Result, error) {
	if err := f[host]; err != nil {
		return nil, err
	}
	return &tlscheck.Result{Host: host, Issuer: "Let's Encrypt R12", NotAfter: certEnd, ChainValid: true}, nil
}

var testNow = time.Date(2026, 11, 9, 3, 0, 0, 0, time.UTC) // 10.00 WIB, Monday 9 Nov 2026

func newTestServer() (*Server, *memStore) {
	st := newMemStore()
	return &Server{
		Store:   st,
		Checker: fakeChecker{"lokal.example": tlscheck.ErrBlockedHost, "mati.example": tlscheck.ErrUnreachable},
		Now:     func() time.Time { return testNow },
		Log:     slog.New(slog.NewTextHandler(io.Discard, nil)),
	}, st
}

func do(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	return v
}

func TestHealth(t *testing.T) {
	s, st := newTestServer()
	h := s.Routes()
	if rec := do(t, h, "GET", "/health", ""); rec.Code != http.StatusOK {
		t.Errorf("healthy: %d", rec.Code)
	}
	st.pingErr = errors.New("down")
	if rec := do(t, h, "GET", "/health", ""); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("db down: %d", rec.Code)
	}
}

func TestCreateManualItem(t *testing.T) {
	s, _ := newTestServer()
	h := s.Routes()
	rec := do(t, h, "POST", "/api/item",
		`{"nama":"Domain tokokuemawar.id","jenis":"domain","klien":"Toko Kue Mawar","host":"tokokuemawar.id","kedaluwarsa":"2026-11-13","biaya":285000}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	got := decode[itemJSON](t, rec)
	if rec.Header().Get("Location") != "/api/item/"+got.ID {
		t.Errorf("Location = %q", rec.Header().Get("Location"))
	}
	if got.SisaHari != 4 || got.Status != expiry.Kritis || got.Sumber != "manual" {
		t.Errorf("got %d days, %s, %s", got.SisaHari, got.Status, got.Sumber)
	}

	if rec := do(t, h, "GET", "/api/item/"+got.ID, ""); rec.Code != http.StatusOK {
		t.Errorf("get: %d", rec.Code)
	}
	list := decode[struct{ Item []itemJSON }](t, do(t, h, "GET", "/api/item?status=kritis", ""))
	if len(list.Item) != 1 {
		t.Errorf("filter kritis: %d items", len(list.Item))
	}
}

func TestCreateCertificateReadsServer(t *testing.T) {
	s, _ := newTestServer()
	rec := do(t, s.Routes(), "POST", "/api/item", `{"jenis":"sertifikat","klien":"Studio Arsitek Rana","host":"https://StudioRana.co.id/"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	got := decode[itemJSON](t, rec)
	if got.Nama != "studiorana.co.id" || got.Sumber != "server" || got.Kedaluwarsa != "2026-11-28" {
		t.Errorf("got nama %q, sumber %q, kedaluwarsa %q", got.Nama, got.Sumber, got.Kedaluwarsa)
	}
	if got.TLS == nil || got.TLS.Penerbit != "Let's Encrypt R12" || got.TerakhirDicek == nil {
		t.Errorf("certificate details missing: %+v", got.TLS)
	}
}

func TestCreateRejectsBadInput(t *testing.T) {
	s, _ := newTestServer()
	h := s.Routes()
	cases := []struct {
		body  string
		code  int
		field string
	}{
		{`{"nama":`, http.StatusBadRequest, ""},
		{`{"nama":"x","jenis":"domain","rahasia":"abc"}`, http.StatusBadRequest, ""},
		{`{"nama":"Token","jenis":"token"}`, http.StatusUnprocessableEntity, "kedaluwarsa"},
		{`{"nama":"Token","jenis":"kunci","kedaluwarsa":"2027-01-01"}`, http.StatusUnprocessableEntity, "jenis"},
		{`{"jenis":"sertifikat","host":"localhost"}`, http.StatusUnprocessableEntity, "host"},
		{`{"jenis":"sertifikat","host":"lokal.example"}`, http.StatusUnprocessableEntity, "host"},
		{`{"jenis":"sertifikat","host":"mati.example"}`, http.StatusUnprocessableEntity, "host"},
		{`{"nama":"A","jenis":"domain","kedaluwarsa":"2027-01-01","biaya":-5}`, http.StatusUnprocessableEntity, "biaya"},
	}
	for _, c := range cases {
		rec := do(t, h, "POST", "/api/item", c.body)
		if rec.Code != c.code {
			t.Errorf("%s: status %d, want %d", c.body, rec.Code, c.code)
			continue
		}
		if c.field != "" {
			if e := decode[apiError](t, rec); e.Field[c.field] == "" {
				t.Errorf("%s: no message for field %q in %v", c.body, c.field, e.Field)
			}
		}
	}
	e := decode[apiError](t, do(t, h, "POST", "/api/item", `{"jenis":"sertifikat","host":"lokal.example"}`))
	if !strings.HasPrefix(e.Field["host"], "Host ini mengarah ke alamat jaringan lokal") {
		t.Errorf("blocked host message = %q", e.Field["host"])
	}
}

func TestUpdateDeleteAndNotFound(t *testing.T) {
	s, _ := newTestServer()
	h := s.Routes()
	created := decode[itemJSON](t, do(t, h, "POST", "/api/item", `{"nama":"VPS","jenis":"langganan","kedaluwarsa":"2026-12-01"}`))

	rec := do(t, h, "PUT", "/api/item/"+created.ID, `{"nama":"VPS","jenis":"langganan","kedaluwarsa":"2027-12-01"}`)
	if rec.Code != http.StatusOK || decode[itemJSON](t, rec).Status != expiry.Aman {
		t.Errorf("renew: %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, "DELETE", "/api/item/"+created.ID, ""); rec.Code != http.StatusNoContent {
		t.Errorf("delete: %d", rec.Code)
	}
	for _, r := range []struct{ method, path string }{
		{"GET", "/api/item/" + created.ID},
		{"DELETE", "/api/item/" + created.ID},
		{"GET", "/api/item/bukan-uuid"},
		{"PUT", "/api/item/00000000-0000-4000-8000-999999999999"},
		{"GET", "/api/tidak-ada"},
	} {
		body := ""
		if r.method == "PUT" {
			body = `{"nama":"VPS","jenis":"langganan","kedaluwarsa":"2027-12-01"}`
		}
		if rec := do(t, h, r.method, r.path, body); rec.Code != http.StatusNotFound {
			t.Errorf("%s %s: %d, want 404", r.method, r.path, rec.Code)
		}
	}
}

func TestRecheck(t *testing.T) {
	s, st := newTestServer()
	h := s.Routes()
	manual := decode[itemJSON](t, do(t, h, "POST", "/api/item", `{"nama":"Token","jenis":"token","kedaluwarsa":"2027-01-01"}`))
	if rec := do(t, h, "POST", "/api/item/"+manual.ID+"/cek", ""); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("recheck manual item: %d", rec.Code)
	}

	cert := decode[itemJSON](t, do(t, h, "POST", "/api/item", `{"jenis":"sertifikat","host":"studiorana.co.id"}`))
	rec := do(t, h, "POST", "/api/item/"+cert.ID+"/cek", "")
	if rec.Code != http.StatusOK || decode[itemJSON](t, rec).GalatCek != nil {
		t.Errorf("recheck: %d %s", rec.Code, rec.Body)
	}

	// The host stops answering: the check is recorded, the last date kept.
	it := st.items[cert.ID]
	dead := "mati.example"
	it.Host = &dead
	st.items[cert.ID] = it
	got := decode[itemJSON](t, do(t, h, "POST", "/api/item/"+cert.ID+"/cek", ""))
	if got.GalatCek == nil || got.Kedaluwarsa != "2026-11-28" {
		t.Errorf("unreachable host: galat %v, kedaluwarsa %s", got.GalatCek, got.Kedaluwarsa)
	}
}

func TestSummaryAndCalendar(t *testing.T) {
	s, _ := newTestServer()
	h := s.Routes()
	do(t, h, "POST", "/api/item", `{"nama":"Sertifikat lama, kedaluwarsa","jenis":"domain","kedaluwarsa":"2026-11-06"}`)
	do(t, h, "POST", "/api/item", `{"nama":"Token Cloudflare DNS; zona utama","jenis":"token","klien":"Pribadi","kedaluwarsa":"2027-05-09"}`)

	sum := decode[store.Summary](t, do(t, h, "GET", "/api/ringkasan", ""))
	if sum.Lewat != 1 || sum.Aman != 1 || sum.Total != 2 {
		t.Errorf("summary = %+v", sum)
	}

	rec := do(t, h, "GET", "/api/kalender.ics", "")
	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/calendar") {
		t.Fatalf("calendar: %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}
	for _, want := range []string{"BEGIN:VCALENDAR\r\n", "DTSTART;VALUE=DATE:20270509\r\n",
		`SUMMARY:Berakhir: Token Cloudflare DNS\; zona utama`, "TRIGGER:-P6DT15H\r\n", "END:VCALENDAR\r\n"} {
		if !strings.Contains(body, want) {
			t.Errorf("calendar lacks %q", want)
		}
	}
	for _, l := range strings.Split(body, "\r\n") {
		if len(l) > 75 {
			t.Errorf("line longer than 75 octets: %q", l)
		}
	}
}
