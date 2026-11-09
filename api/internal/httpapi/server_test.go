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

type memSession struct {
	userID  string
	expires time.Time
}

// memStore is an in-memory Store for handler tests.
type memStore struct {
	mu       sync.Mutex
	items    map[string]store.Item
	users    map[string]store.Pengguna // by id
	sessions map[string]memSession     // by token hash
	next     int
	pingErr  error
}

func newMemStore() *memStore {
	return &memStore{items: map[string]store.Item{}, users: map[string]store.Pengguna{}, sessions: map[string]memSession{}}
}

func (m *memStore) newID() string {
	m.next++
	return fmt.Sprintf("00000000-0000-4000-8000-%012d", m.next)
}

func (m *memStore) Ping(context.Context) error { return m.pingErr }

func (m *memStore) List(_ context.Context, userID string, f store.Filter, now time.Time) ([]store.Item, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []store.Item{}
	for _, it := range m.items {
		if it.PenggunaID != userID {
			continue
		}
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

func (m *memStore) Get(_ context.Context, userID, id string) (store.Item, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	it, ok := m.items[id]
	if !ok || it.PenggunaID != userID {
		return store.Item{}, store.ErrNotFound
	}
	return it, nil
}

func (m *memStore) Create(_ context.Context, userID string, in store.Input) (store.Item, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	it := fromInput(m.newID(), userID, in)
	m.items[it.ID] = it
	return it, nil
}

func (m *memStore) Update(_ context.Context, userID, id string, in store.Input) (store.Item, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if it, ok := m.items[id]; !ok || it.PenggunaID != userID {
		return store.Item{}, store.ErrNotFound
	}
	it := fromInput(id, userID, in)
	m.items[id] = it
	return it, nil
}

func (m *memStore) Delete(_ context.Context, userID, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if it, ok := m.items[id]; !ok || it.PenggunaID != userID {
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

func (m *memStore) Summary(ctx context.Context, userID string, now time.Time) (store.Summary, error) {
	items, _ := m.List(ctx, userID, store.Filter{}, now)
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

func (m *memStore) CreateUser(_ context.Context, nama, email, hash string) (store.Pengguna, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, u := range m.users {
		if u.Email == email {
			return store.Pengguna{}, store.ErrEmailTaken
		}
	}
	u := store.Pengguna{ID: m.newID(), Nama: nama, Email: email, HashSandi: hash, Dibuat: testNow}
	m.users[u.ID] = u
	return u, nil
}

func (m *memStore) UserByEmail(_ context.Context, email string) (store.Pengguna, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, u := range m.users {
		if u.Email == email {
			return u, nil
		}
	}
	return store.Pengguna{}, store.ErrNotFound
}

func (m *memStore) CreateSession(_ context.Context, hash, userID string, expires time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[hash] = memSession{userID, expires}
	return nil
}

func (m *memStore) UserBySession(_ context.Context, hash string, now time.Time) (store.Pengguna, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[hash]
	if !ok || !s.expires.After(now) {
		return store.Pengguna{}, store.ErrNotFound
	}
	u, ok := m.users[s.userID]
	if !ok {
		return store.Pengguna{}, store.ErrNotFound
	}
	return u, nil
}

func (m *memStore) DeleteSession(_ context.Context, hash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, hash)
	return nil
}

func (m *memStore) DeleteUser(_ context.Context, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.users[userID]; !ok {
		return store.ErrNotFound
	}
	delete(m.users, userID)
	for h, s := range m.sessions {
		if s.userID == userID {
			delete(m.sessions, h)
		}
	}
	for id, it := range m.items {
		if it.PenggunaID == userID {
			delete(m.items, id)
		}
	}
	return nil
}

func (m *memStore) DeleteExpiredSessions(context.Context, time.Time) (int64, error) { return 0, nil }

func fromInput(id, userID string, in store.Input) store.Item {
	return store.Item{ID: id, PenggunaID: userID, Nama: in.Nama, Jenis: in.Jenis, Klien: in.Klien, Host: in.Host,
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

// do sends a request, with the session cookie when one is given.
func do(t *testing.T, h http.Handler, method, path, body string, cookie ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for _, c := range cookie {
		if c != nil {
			req.AddCookie(c)
		}
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func sessionOf(rec *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookie && c.Value != "" {
			return c
		}
	}
	return nil
}

// signUp creates an account and returns its session cookie.
func signUp(t *testing.T, h http.Handler, email string) *http.Cookie {
	t.Helper()
	rec := do(t, h, "POST", "/api/akun", `{"nama":"Pengguna Uji","email":"`+email+`","kata_sandi":"rahasia-panjang"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("sign up %s: %d %s", email, rec.Code, rec.Body)
	}
	c := sessionOf(rec)
	if c == nil {
		t.Fatalf("sign up %s: no session cookie", email)
	}
	return c
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

func TestEverythingNeedsASession(t *testing.T) {
	s, _ := newTestServer()
	h := s.Routes()
	for _, r := range []struct{ method, path string }{
		{"GET", "/api/item"}, {"POST", "/api/item"}, {"GET", "/api/item/00000000-0000-4000-8000-000000000001"},
		{"PUT", "/api/item/00000000-0000-4000-8000-000000000001"}, {"DELETE", "/api/item/00000000-0000-4000-8000-000000000001"},
		{"POST", "/api/item/00000000-0000-4000-8000-000000000001/cek"}, {"GET", "/api/sertifikat?host=example.com"},
		{"GET", "/api/ringkasan"}, {"GET", "/api/kalender.ics"}, {"GET", "/api/saya"}, {"DELETE", "/api/saya"},
	} {
		if rec := do(t, h, r.method, r.path, `{}`); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s without session: %d, want 401", r.method, r.path, rec.Code)
		}
	}
	bogus := &http.Cookie{Name: sessionCookie, Value: "bukan-token"}
	if rec := do(t, h, "GET", "/api/item", "", bogus); rec.Code != http.StatusUnauthorized {
		t.Errorf("unknown token: %d", rec.Code)
	}
}

func TestSignUpLoginLogout(t *testing.T) {
	s, _ := newTestServer()
	h := s.Routes()
	c := signUp(t, h, "Rifki@Contoh.ID")
	if !c.HttpOnly || c.SameSite != http.SameSiteLaxMode || c.Path != "/" {
		t.Errorf("cookie flags: %+v", c)
	}

	me := decode[accountJSON](t, do(t, h, "GET", "/api/saya", "", c))
	if me.Pengguna.Email != "rifki@contoh.id" || me.Pengguna.Nama != "Pengguna Uji" {
		t.Errorf("me = %+v", me.Pengguna)
	}
	if strings.Contains(do(t, h, "GET", "/api/saya", "", c).Body.String(), "argon2") {
		t.Error("password hash leaked in /api/saya")
	}

	if rec := do(t, h, "POST", "/api/akun", `{"nama":"Lain","email":"rifki@contoh.id","kata_sandi":"rahasia-panjang"}`); rec.Code != http.StatusConflict {
		t.Errorf("duplicate email: %d", rec.Code)
	}
	bad := do(t, h, "POST", "/api/akun", `{"nama":"A","email":"bukan-email","kata_sandi":"pendek"}`)
	if e := decode[apiError](t, bad); bad.Code != http.StatusUnprocessableEntity || len(e.Field) != 3 {
		t.Errorf("invalid sign-up: %d %v", bad.Code, e.Field)
	}

	if rec := do(t, h, "POST", "/api/sesi", `{"email":"rifki@contoh.id","kata_sandi":"salah-sandi"}`); rec.Code != http.StatusUnauthorized {
		t.Errorf("wrong password: %d", rec.Code)
	}
	if rec := do(t, h, "POST", "/api/sesi", `{"email":"tidak.ada@contoh.id","kata_sandi":"rahasia-panjang"}`); rec.Code != http.StatusUnauthorized {
		t.Errorf("unknown email: %d", rec.Code)
	}
	login := do(t, h, "POST", "/api/sesi", `{"email":" RIFKI@contoh.id ","kata_sandi":"rahasia-panjang"}`)
	c2 := sessionOf(login)
	if login.Code != http.StatusOK || c2 == nil {
		t.Fatalf("login: %d", login.Code)
	}

	if rec := do(t, h, "DELETE", "/api/sesi", "", c2); rec.Code != http.StatusNoContent {
		t.Errorf("logout: %d", rec.Code)
	}
	if rec := do(t, h, "GET", "/api/saya", "", c2); rec.Code != http.StatusUnauthorized {
		t.Errorf("after logout: %d", rec.Code)
	}
	if rec := do(t, h, "GET", "/api/saya", "", c); rec.Code != http.StatusOK {
		t.Errorf("other session ended too: %d", rec.Code)
	}
}

func TestUsersSeeOnlyTheirOwnAssets(t *testing.T) {
	s, _ := newTestServer()
	h := s.Routes()
	a := signUp(t, h, "a@contoh.id")
	b := signUp(t, h, "b@contoh.id")

	created := decode[itemJSON](t, do(t, h, "POST", "/api/item", `{"nama":"VPS A","jenis":"langganan","kedaluwarsa":"2026-12-01"}`, a))
	for _, r := range []struct {
		method, path, body string
	}{
		{"GET", "/api/item/" + created.ID, ""},
		{"PUT", "/api/item/" + created.ID, `{"nama":"Dicuri","jenis":"langganan","kedaluwarsa":"2027-01-01"}`},
		{"DELETE", "/api/item/" + created.ID, ""},
		{"POST", "/api/item/" + created.ID + "/cek", ""},
	} {
		if rec := do(t, h, r.method, r.path, r.body, b); rec.Code != http.StatusNotFound {
			t.Errorf("user B %s %s: %d, want 404", r.method, r.path, rec.Code)
		}
	}
	if list := decode[struct{ Item []itemJSON }](t, do(t, h, "GET", "/api/item", "", b)); len(list.Item) != 0 {
		t.Errorf("user B sees %d items", len(list.Item))
	}
	if list := decode[struct{ Item []itemJSON }](t, do(t, h, "GET", "/api/item", "", a)); len(list.Item) != 1 || list.Item[0].Nama != "VPS A" {
		t.Errorf("user A list = %+v", list.Item)
	}
}

func TestDeleteAccountRemovesData(t *testing.T) {
	s, st := newTestServer()
	h := s.Routes()
	c := signUp(t, h, "hapus@contoh.id")
	do(t, h, "POST", "/api/item", `{"nama":"Token","jenis":"token","kedaluwarsa":"2027-01-01"}`, c)
	if rec := do(t, h, "DELETE", "/api/saya", "", c); rec.Code != http.StatusNoContent {
		t.Fatalf("delete account: %d", rec.Code)
	}
	if len(st.items) != 0 || len(st.users) != 0 || len(st.sessions) != 0 {
		t.Errorf("left behind: %d items, %d users, %d sessions", len(st.items), len(st.users), len(st.sessions))
	}
	if rec := do(t, h, "GET", "/api/saya", "", c); rec.Code != http.StatusUnauthorized {
		t.Errorf("after delete: %d", rec.Code)
	}
}

func TestOtherSitesCannotWrite(t *testing.T) {
	s, _ := newTestServer()
	h := s.Routes()
	c := signUp(t, h, "origin@contoh.id")
	body := `{"nama":"VPS","jenis":"langganan","kedaluwarsa":"2026-12-01"}`

	req := httptest.NewRequest("POST", "http://tenggat.mrramdn.com/api/item", strings.NewReader(body))
	req.Header.Set("Origin", "https://jahat.example")
	req.AddCookie(c)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("cross-site POST: %d, want 403", rec.Code)
	}

	req = httptest.NewRequest("POST", "http://tenggat.mrramdn.com/api/item", strings.NewReader(body))
	req.Header.Set("Origin", "https://tenggat.mrramdn.com")
	req.AddCookie(c)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Errorf("same-site POST: %d, want 201", rec.Code)
	}
}

func TestCreateManualItem(t *testing.T) {
	s, _ := newTestServer()
	h := s.Routes()
	c := signUp(t, h, "manual@contoh.id")
	rec := do(t, h, "POST", "/api/item",
		`{"nama":"Domain tokokuemawar.id","jenis":"domain","klien":"Toko Kue Mawar","host":"tokokuemawar.id","kedaluwarsa":"2026-11-13","biaya":285000}`, c)
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
	if rec := do(t, h, "GET", "/api/item/"+got.ID, "", c); rec.Code != http.StatusOK {
		t.Errorf("get: %d", rec.Code)
	}
	list := decode[struct{ Item []itemJSON }](t, do(t, h, "GET", "/api/item?status=kritis", "", c))
	if len(list.Item) != 1 {
		t.Errorf("filter kritis: %d items", len(list.Item))
	}
}

func TestCreateCertificateReadsServer(t *testing.T) {
	s, _ := newTestServer()
	h := s.Routes()
	c := signUp(t, h, "cert@contoh.id")
	rec := do(t, h, "POST", "/api/item", `{"jenis":"sertifikat","klien":"Studio Arsitek Rana","host":"https://StudioRana.co.id/"}`, c)
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
	c := signUp(t, h, "bad@contoh.id")
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
	for _, tc := range cases {
		rec := do(t, h, "POST", "/api/item", tc.body, c)
		if rec.Code != tc.code {
			t.Errorf("%s: status %d, want %d", tc.body, rec.Code, tc.code)
			continue
		}
		if tc.field != "" {
			if e := decode[apiError](t, rec); e.Field[tc.field] == "" {
				t.Errorf("%s: no message for field %q in %v", tc.body, tc.field, e.Field)
			}
		}
	}
	e := decode[apiError](t, do(t, h, "POST", "/api/item", `{"jenis":"sertifikat","host":"lokal.example"}`, c))
	if !strings.HasPrefix(e.Field["host"], "Host ini mengarah ke alamat jaringan lokal") {
		t.Errorf("blocked host message = %q", e.Field["host"])
	}
}

func TestUpdateDeleteAndNotFound(t *testing.T) {
	s, _ := newTestServer()
	h := s.Routes()
	c := signUp(t, h, "ubah@contoh.id")
	created := decode[itemJSON](t, do(t, h, "POST", "/api/item", `{"nama":"VPS","jenis":"langganan","kedaluwarsa":"2026-12-01"}`, c))

	rec := do(t, h, "PUT", "/api/item/"+created.ID, `{"nama":"VPS","jenis":"langganan","kedaluwarsa":"2027-12-01"}`, c)
	if rec.Code != http.StatusOK || decode[itemJSON](t, rec).Status != expiry.Aman {
		t.Errorf("renew: %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, "DELETE", "/api/item/"+created.ID, "", c); rec.Code != http.StatusNoContent {
		t.Errorf("delete: %d", rec.Code)
	}
	for _, r := range []struct{ method, path string }{
		{"GET", "/api/item/" + created.ID},
		{"DELETE", "/api/item/" + created.ID},
		{"GET", "/api/item/bukan-uuid"},
		{"PUT", "/api/item/00000000-0000-4000-8000-999999999999"},
	} {
		body := ""
		if r.method == "PUT" {
			body = `{"nama":"VPS","jenis":"langganan","kedaluwarsa":"2027-12-01"}`
		}
		if rec := do(t, h, r.method, r.path, body, c); rec.Code != http.StatusNotFound {
			t.Errorf("%s %s: %d, want 404", r.method, r.path, rec.Code)
		}
	}
	if rec := do(t, h, "GET", "/api/tidak-ada", ""); rec.Code != http.StatusNotFound {
		t.Errorf("unknown path: %d", rec.Code)
	}
}

func TestRecheck(t *testing.T) {
	s, st := newTestServer()
	h := s.Routes()
	c := signUp(t, h, "cek@contoh.id")
	manual := decode[itemJSON](t, do(t, h, "POST", "/api/item", `{"nama":"Token","jenis":"token","kedaluwarsa":"2027-01-01"}`, c))
	if rec := do(t, h, "POST", "/api/item/"+manual.ID+"/cek", "", c); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("recheck manual item: %d", rec.Code)
	}

	cert := decode[itemJSON](t, do(t, h, "POST", "/api/item", `{"jenis":"sertifikat","host":"studiorana.co.id"}`, c))
	rec := do(t, h, "POST", "/api/item/"+cert.ID+"/cek", "", c)
	if rec.Code != http.StatusOK || decode[itemJSON](t, rec).GalatCek != nil {
		t.Errorf("recheck: %d %s", rec.Code, rec.Body)
	}

	// The host stops answering: the check is recorded, the last date kept.
	it := st.items[cert.ID]
	dead := "mati.example"
	it.Host = &dead
	st.items[cert.ID] = it
	got := decode[itemJSON](t, do(t, h, "POST", "/api/item/"+cert.ID+"/cek", "", c))
	if got.GalatCek == nil || got.Kedaluwarsa != "2026-11-28" {
		t.Errorf("unreachable host: galat %v, kedaluwarsa %s", got.GalatCek, got.Kedaluwarsa)
	}
}

func TestSummaryAndCalendar(t *testing.T) {
	s, _ := newTestServer()
	h := s.Routes()
	c := signUp(t, h, "kalender@contoh.id")
	do(t, h, "POST", "/api/item", `{"nama":"Sertifikat lama, kedaluwarsa","jenis":"domain","kedaluwarsa":"2026-11-06"}`, c)
	do(t, h, "POST", "/api/item", `{"nama":"Token Cloudflare DNS; zona utama","jenis":"token","klien":"Pribadi","kedaluwarsa":"2027-05-09"}`, c)

	sum := decode[store.Summary](t, do(t, h, "GET", "/api/ringkasan", "", c))
	if sum.Lewat != 1 || sum.Aman != 1 || sum.Total != 2 {
		t.Errorf("summary = %+v", sum)
	}

	rec := do(t, h, "GET", "/api/kalender.ics", "", c)
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

// memCache is an in-memory Cache for handler tests.
type memCache struct {
	mu       sync.Mutex
	data     map[string][]byte
	versions map[string]int64
	counts   map[string]int
}

func newMemCache() *memCache {
	return &memCache{data: map[string][]byte{}, versions: map[string]int64{}, counts: map[string]int{}}
}

func (c *memCache) Ping(context.Context) error { return nil }
func (c *memCache) Get(_ context.Context, key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	b, ok := c.data[key]
	return b, ok
}
func (c *memCache) Set(_ context.Context, key string, value []byte, _ time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[key] = value
}
func (c *memCache) Version(_ context.Context, scope string) int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.versions[scope]
}
func (c *memCache) Bump(_ context.Context, scope string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.versions[scope]++
}
func (c *memCache) Allow(_ context.Context, bucket, client string, limit int, _ time.Duration) (bool, time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.counts[bucket+client]++
	return c.counts[bucket+client] <= limit, 42 * time.Second
}

// countingChecker counts real lookups.
type countingChecker struct {
	fakeChecker
	calls int
}

func (c *countingChecker) Check(ctx context.Context, host string) (*tlscheck.Result, error) {
	c.calls++
	return c.fakeChecker.Check(ctx, host)
}

func TestCachedReadsArePerUser(t *testing.T) {
	s, _ := newTestServer()
	s.Cache = newMemCache()
	h := s.Routes()
	a := signUp(t, h, "cache-a@contoh.id")
	b := signUp(t, h, "cache-b@contoh.id")

	first := do(t, h, "GET", "/api/item", "", a)
	second := do(t, h, "GET", "/api/item", "", a)
	if first.Header().Get("X-Cache") != "MISS" || second.Header().Get("X-Cache") != "HIT" {
		t.Fatalf("X-Cache = %q then %q", first.Header().Get("X-Cache"), second.Header().Get("X-Cache"))
	}

	do(t, h, "POST", "/api/item", `{"nama":"VPS","jenis":"langganan","kedaluwarsa":"2026-12-01"}`, a)
	after := do(t, h, "GET", "/api/item", "", a)
	if after.Header().Get("X-Cache") != "MISS" || len(decode[struct{ Item []itemJSON }](t, after).Item) != 1 {
		t.Errorf("after a write: X-Cache %q, body %s", after.Header().Get("X-Cache"), after.Body)
	}
	// B's first read must not be served A's cached list.
	other := do(t, h, "GET", "/api/item", "", b)
	if other.Header().Get("X-Cache") != "MISS" || len(decode[struct{ Item []itemJSON }](t, other).Item) != 0 {
		t.Errorf("user B: X-Cache %q, body %s", other.Header().Get("X-Cache"), other.Body)
	}
}

func TestCertificateLookupIsCached(t *testing.T) {
	s, _ := newTestServer()
	s.Cache = newMemCache()
	checker := &countingChecker{fakeChecker: fakeChecker{}}
	s.Checker = checker
	h := s.Routes()
	c := signUp(t, h, "pratinjau@contoh.id")

	rec := do(t, h, "GET", "/api/sertifikat?host=studiorana.co.id", "", c)
	if rec.Code != http.StatusOK {
		t.Fatalf("preview: %d %s", rec.Code, rec.Body)
	}
	if p := decode[certificatePreview](t, rec); p.Kedaluwarsa != "2026-11-28" || p.SisaHari != 19 || p.Status != expiry.Segera {
		t.Errorf("preview = %+v", p)
	}
	if rec := do(t, h, "POST", "/api/item", `{"jenis":"sertifikat","host":"studiorana.co.id"}`, c); rec.Code != http.StatusCreated {
		t.Fatalf("create: %d", rec.Code)
	}
	if checker.calls != 1 {
		t.Errorf("lookups = %d, want 1", checker.calls)
	}
	if rec := do(t, h, "GET", "/api/sertifikat?host=localhost", "", c); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("preview localhost: %d", rec.Code)
	}
}

func TestRateLimits(t *testing.T) {
	s, _ := newTestServer()
	s.Cache = newMemCache()
	s.LimitPerMinute = 3
	h := s.Routes()
	c := signUp(t, h, "batas@contoh.id")
	body := `{"nama":"Token","jenis":"token","kedaluwarsa":"2027-01-01"}`
	for i := 0; i < 3; i++ {
		if rec := do(t, h, "POST", "/api/item", body, c); rec.Code != http.StatusCreated {
			t.Fatalf("request %d: %d", i+1, rec.Code)
		}
	}
	rec := do(t, h, "POST", "/api/item", body, c)
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") != "43" {
		t.Errorf("4th request: %d, Retry-After %q", rec.Code, rec.Header().Get("Retry-After"))
	}
	if rec := do(t, h, "GET", "/api/item", "", c); rec.Code != http.StatusOK {
		t.Errorf("read after limit: %d", rec.Code)
	}

	// Logins have their own bucket of 10 a minute.
	for i := 0; i < 10; i++ {
		do(t, h, "POST", "/api/sesi", `{"email":"batas@contoh.id","kata_sandi":"salah-sandi"}`)
	}
	if rec := do(t, h, "POST", "/api/sesi", `{"email":"batas@contoh.id","kata_sandi":"rahasia-panjang"}`); rec.Code != http.StatusTooManyRequests {
		t.Errorf("11th login: %d, want 429", rec.Code)
	}
}

func TestClientIP(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "172.20.0.4:51234"
	if got := clientIP(r); got != "172.20.0.4" {
		t.Errorf("no header: %s", got)
	}
	r.Header.Set("X-Forwarded-For", "203.0.113.9, 10.0.0.7")
	if got := clientIP(r); got != "10.0.0.7" {
		t.Errorf("forwarded: %s", got)
	}
}
