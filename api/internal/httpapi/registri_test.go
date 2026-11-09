package httpapi

import (
	"bytes"
	"context"
	"encoding/csv"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mrramdn/tenggat/api/internal/rdap"
	"github.com/mrramdn/tenggat/api/internal/store"
)

// Calendar tokens of each memStore, kept beside it so the memStore type in
// server_test.go stays as it is.
var (
	memCalendarMu sync.Mutex
	memCalendars  = map[*memStore]map[string]string{} // token hash → user id
)

func (m *memStore) calendars() map[string]string {
	if memCalendars[m] == nil {
		memCalendars[m] = map[string]string{}
	}
	return memCalendars[m]
}

func (m *memStore) SetCalendarToken(_ context.Context, userID, hash string) error {
	memCalendarMu.Lock()
	defer memCalendarMu.Unlock()
	cal := m.calendars()
	for h, id := range cal {
		if id == userID {
			delete(cal, h)
		}
	}
	cal[hash] = userID
	return nil
}

func (m *memStore) DeleteCalendarToken(_ context.Context, userID string) error {
	memCalendarMu.Lock()
	defer memCalendarMu.Unlock()
	cal := m.calendars()
	for h, id := range cal {
		if id == userID {
			delete(cal, h)
		}
	}
	return nil
}

func (m *memStore) UserByCalendarToken(_ context.Context, hash string) (store.Pengguna, error) {
	memCalendarMu.Lock()
	id, ok := m.calendars()[hash]
	memCalendarMu.Unlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	u, found := m.users[id]
	if !ok || !found {
		return store.Pengguna{}, store.ErrNotFound
	}
	return u, nil
}

func (m *memStore) CalendarActive(_ context.Context, userID string) (bool, error) {
	memCalendarMu.Lock()
	defer memCalendarMu.Unlock()
	for _, id := range m.calendars() {
		if id == userID {
			return true, nil
		}
	}
	return false, nil
}

func (m *memStore) SaveRegistryCheck(_ context.Context, id string, date time.Time, info store.RegistryInfo, at time.Time) (store.Item, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	it := m.items[id]
	it.Kedaluwarsa, it.InfoRegistri, it.TerakhirDicek, it.GalatCek = date, &info, &at, nil
	m.items[id] = it
	return it, nil
}

// fakeRegistry answers by registrable domain and counts real lookups.
type fakeRegistry struct {
	mu      sync.Mutex
	results map[string]*rdap.Result
	errs    map[string]error
	calls   int
}

func (f *fakeRegistry) Lookup(_ context.Context, host string) (*rdap.Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	_, domain, err := rdap.Normalize(host)
	if err != nil {
		return nil, err
	}
	if _, ok := rdap.New().Servers[rdap.TLD(domain)]; !ok {
		return nil, &rdap.NoRDAPError{TLD: rdap.TLD(domain)}
	}
	if err := f.errs[domain]; err != nil {
		return nil, err
	}
	if res := f.results[domain]; res != nil {
		copied := *res
		return &copied, nil
	}
	return nil, rdap.ErrNotFound
}

func domainResult(domain, date, registrar string) *rdap.Result {
	d, _ := time.Parse(time.DateOnly, date)
	return &rdap.Result{Domain: domain, Expiration: d.Add(-7 * time.Hour), Date: d, Registrar: registrar, Server: "rdap.example"}
}

func newRegistryServer() (*Server, *memStore, *fakeRegistry) {
	s, st := newTestServer()
	reg := &fakeRegistry{
		results: map[string]*rdap.Result{
			"github.com": domainResult("github.com", "2026-10-10", "MarkMonitor Inc."),
			"toko.co.id": domainResult("toko.co.id", "2027-02-01", "PT Registrar Uji"),
		},
		errs: map[string]error{
			"tanpatanggal.com": rdap.ErrNoExpiration,
			"lambat.com":       rdap.ErrUnreachable,
		},
	}
	s.Registry = reg
	return s, st, reg
}

func TestDomainPreview(t *testing.T) {
	s, _, reg := newRegistryServer()
	s.Cache = newMemCache()
	h := s.Routes()
	c := signUp(t, h, "domain@contoh.id")

	rec := do(t, h, "GET", "/api/domain?host=https://www.Toko.co.id/", "", c)
	if rec.Code != http.StatusOK {
		t.Fatalf("preview: %d %s", rec.Code, rec.Body)
	}
	got := decode[map[string]any](t, rec)
	reg2, _ := got["registri"].(map[string]any)
	if got["host"] != "www.toko.co.id" || got["domain"] != "toko.co.id" || got["kedaluwarsa"] != "2027-02-01" ||
		got["sisa_hari"] != float64(84) || got["status"] != "aman" || reg2["registrar"] != "PT Registrar Uji" ||
		reg2["server"] != "rdap.example" {
		t.Errorf("preview = %v", got)
	}
	// The second lookup of the same domain comes from the cache.
	do(t, h, "GET", "/api/domain?host=toko.co.id", "", c)
	if reg.calls != 1 {
		t.Errorf("registry asked %d times, want 1", reg.calls)
	}

	for host, want := range map[string]string{
		"toko.io":          "Registri .io belum menyediakan RDAP. Isi tanggalnya sendiri.",
		"hilang.com":       "Domain hilang.com tidak ditemukan di registri. Periksa ejaannya, atau isi tanggalnya sendiri.",
		"tanpatanggal.com": "Registri tidak mencantumkan tanggal berakhir tanpatanggal.com. Isi tanggalnya sendiri.",
		"lambat.com":       "Registri .com tidak dapat dihubungi saat ini. Coba lagi sebentar lagi, atau isi tanggalnya sendiri.",
		"localhost":        "Nama domain tidak valid. Tulis nama domain, misalnya tokokuemawar.id.",
		"10.1.2.3":         "Nama domain tidak valid. Tulis nama domain, misalnya tokokuemawar.id.",
	} {
		rec := do(t, h, "GET", "/api/domain?host="+host, "", c)
		if e := decode[apiError](t, rec); rec.Code != http.StatusUnprocessableEntity || e.Field["host"] != want {
			t.Errorf("%s: %d %q", host, rec.Code, e.Field["host"])
		}
	}
	// A negative answer is cached too; a network error is not.
	before := reg.calls
	do(t, h, "GET", "/api/domain?host=hilang.com", "", c)
	do(t, h, "GET", "/api/domain?host=lambat.com", "", c)
	if reg.calls != before+1 {
		t.Errorf("registry asked %d more times, want 1 (only lambat.com)", reg.calls-before)
	}
	if rec := do(t, h, "GET", "/api/domain?host=github.com", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("preview without session: %d", rec.Code)
	}
}

func TestCreateDomainFromRegistry(t *testing.T) {
	s, _, _ := newRegistryServer()
	h := s.Routes()
	c := signUp(t, h, "registri@contoh.id")

	rec := do(t, h, "POST", "/api/item", `{"jenis":"domain","host":"www.toko.co.id","klien":"Toko"}`, c)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body)
	}
	got := decode[itemJSON](t, rec)
	if got.Nama != "toko.co.id" || got.Sumber != "registri" || got.Kedaluwarsa != "2027-02-01" ||
		got.Registri == nil || got.Registri.Domain != "toko.co.id" || got.Registri.Registrar != "PT Registrar Uji" ||
		got.TerakhirDicek == nil || *got.Host != "www.toko.co.id" {
		t.Errorf("created = %+v registri %+v", got, got.Registri)
	}
	if !strings.Contains(rec.Body.String(), `"registri":{"domain":"toko.co.id","registrar":"PT Registrar Uji","server":"rdap.example"}`) {
		t.Errorf("JSON shape: %s", rec.Body)
	}

	// A domain sent with a date stays manual and has no registry info.
	manual := decode[itemJSON](t, do(t, h, "POST", "/api/item",
		`{"nama":"Domain lama","jenis":"domain","host":"toko.io","kedaluwarsa":"2027-05-01"}`, c))
	if manual.Sumber != "manual" || manual.Registri != nil || !strings.Contains(
		do(t, h, "GET", "/api/item/"+manual.ID, "", c).Body.String(), `"registri":null`) {
		t.Errorf("manual domain = %+v", manual)
	}

	// No RDAP for .io: the client is told to fill the date in.
	rec = do(t, h, "POST", "/api/item", `{"jenis":"domain","host":"toko.io"}`, c)
	if e := decode[apiError](t, rec); rec.Code != http.StatusUnprocessableEntity ||
		e.Field["host"] != "Registri .io belum menyediakan RDAP. Isi tanggalnya sendiri." {
		t.Errorf(".io: %d %v", rec.Code, e.Field)
	}
	// A domain with neither host nor date still needs the date.
	rec = do(t, h, "POST", "/api/item", `{"nama":"Tanpa host","jenis":"domain"}`, c)
	if e := decode[apiError](t, rec); rec.Code != http.StatusUnprocessableEntity || e.Field["kedaluwarsa"] == "" {
		t.Errorf("no host, no date: %d %v", rec.Code, e.Field)
	}
	// A public suffix is not a domain.
	rec = do(t, h, "POST", "/api/item", `{"jenis":"domain","host":"co.id"}`, c)
	if e := decode[apiError](t, rec); rec.Code != http.StatusUnprocessableEntity || e.Field["host"] == "" {
		t.Errorf("co.id: %d %v", rec.Code, e.Field)
	}

	// Editing with a date turns the item manual; editing without one reads the registry again.
	upd := decode[itemJSON](t, do(t, h, "PUT", "/api/item/"+got.ID,
		`{"nama":"Toko","jenis":"domain","host":"toko.co.id","kedaluwarsa":"2030-01-01"}`, c))
	if upd.Sumber != "manual" || upd.Registri != nil || upd.Kedaluwarsa != "2030-01-01" {
		t.Errorf("update with date = %+v", upd)
	}
	upd = decode[itemJSON](t, do(t, h, "PUT", "/api/item/"+got.ID, `{"nama":"Toko","jenis":"domain","host":"toko.co.id"}`, c))
	if upd.Sumber != "registri" || upd.Kedaluwarsa != "2027-02-01" {
		t.Errorf("update without date = %+v", upd)
	}
}

func TestRecheckRegistryDomain(t *testing.T) {
	s, st, reg := newRegistryServer()
	s.Cache = newMemCache()
	h := s.Routes()
	c := signUp(t, h, "cekdomain@contoh.id")
	it := decode[itemJSON](t, do(t, h, "POST", "/api/item", `{"jenis":"domain","host":"github.com"}`, c))

	// The registry now has a renewed date; a recheck skips the cache.
	reg.mu.Lock()
	reg.results["github.com"] = domainResult("github.com", "2027-10-10", "MarkMonitor Inc.")
	reg.mu.Unlock()
	rec := do(t, h, "POST", "/api/item/"+it.ID+"/cek", "", c)
	got := decode[itemJSON](t, rec)
	if rec.Code != http.StatusOK || got.Kedaluwarsa != "2027-10-10" || got.GalatCek != nil || got.Registri == nil {
		t.Errorf("recheck: %d %s", rec.Code, rec.Body)
	}

	// The registry stops answering: the error is recorded, the date kept.
	reg.mu.Lock()
	reg.errs["github.com"] = rdap.ErrUnreachable
	reg.mu.Unlock()
	got = decode[itemJSON](t, do(t, h, "POST", "/api/item/"+it.ID+"/cek", "", c))
	if got.GalatCek == nil || !strings.HasPrefix(*got.GalatCek, "Registri .com tidak dapat dihubungi") || got.Kedaluwarsa != "2027-10-10" {
		t.Errorf("unreachable registry: galat %v, kedaluwarsa %s", got.GalatCek, got.Kedaluwarsa)
	}

	// The periodic job takes the same path.
	reg.mu.Lock()
	delete(reg.errs, "github.com")
	reg.mu.Unlock()
	updated, err := s.recheck(context.Background(), st.items[it.ID])
	if err != nil || updated.GalatCek != nil || updated.Kedaluwarsa.Format(time.DateOnly) != "2027-10-10" {
		t.Errorf("periodic recheck: %v %+v", err, updated)
	}
}

func TestCalendarLinkLifecycle(t *testing.T) {
	s, _ := newTestServer()
	h := s.Routes()
	c := signUp(t, h, "kalender@contoh.id")
	do(t, h, "POST", "/api/item", `{"nama":"Domain toko","jenis":"domain","kedaluwarsa":"2026-12-24"}`, c)

	if me := decode[map[string]any](t, do(t, h, "GET", "/api/saya", "", c)); me["kalender_aktif"] != false {
		t.Errorf("before: %v", me)
	}
	rec := do(t, h, "POST", "/api/kalender/tautan", "", c)
	link := decode[map[string]string](t, rec)["jalur"]
	if rec.Code != http.StatusCreated || !strings.HasPrefix(link, "/api/kalender/") || !strings.HasSuffix(link, ".ics") {
		t.Fatalf("create link: %d %s", rec.Code, rec.Body)
	}
	if me := decode[map[string]any](t, do(t, h, "GET", "/api/saya", "", c)); me["kalender_aktif"] != true {
		t.Errorf("after: %v", me)
	}

	// The feed needs no cookie and no Origin.
	feed := do(t, h, "GET", link, "")
	if feed.Code != http.StatusOK || !strings.Contains(feed.Body.String(), "SUMMARY:Berakhir: Domain toko") ||
		feed.Header().Get("Cache-Control") != "private, max-age=300" ||
		!strings.HasPrefix(feed.Header().Get("Content-Type"), "text/calendar") {
		t.Errorf("feed: %d %v %s", feed.Code, feed.Header(), feed.Body)
	}

	// A new link replaces the old one.
	link2 := decode[map[string]string](t, do(t, h, "POST", "/api/kalender/tautan", "", c))["jalur"]
	if link2 == link {
		t.Fatal("same link twice")
	}
	if rec := do(t, h, "GET", link, ""); rec.Code != http.StatusNotFound {
		t.Errorf("old link after rotation: %d", rec.Code)
	}
	if rec := do(t, h, "GET", link2, ""); rec.Code != http.StatusOK {
		t.Errorf("new link: %d", rec.Code)
	}

	if rec := do(t, h, "DELETE", "/api/kalender/tautan", "", c); rec.Code != http.StatusNoContent {
		t.Errorf("delete link: %d", rec.Code)
	}
	if rec := do(t, h, "GET", link2, ""); rec.Code != http.StatusNotFound {
		t.Errorf("link after delete: %d", rec.Code)
	}
	if me := decode[map[string]any](t, do(t, h, "GET", "/api/saya", "", c)); me["kalender_aktif"] != false {
		t.Errorf("after delete: %v", me)
	}

	for _, p := range []string{"/api/kalender/bukan-token.ics", "/api/kalender/tautan", "/api/kalender/" + strings.Repeat("A", 43)} {
		if rec := do(t, h, "GET", p, ""); rec.Code != http.StatusNotFound {
			t.Errorf("GET %s: %d", p, rec.Code)
		}
	}
	for _, m := range []string{"POST", "DELETE"} {
		if rec := do(t, h, m, "/api/kalender/tautan", ""); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s link without session: %d", m, rec.Code)
		}
	}
	if got := redactPath(link2); strings.Contains(got, strings.TrimSuffix(strings.TrimPrefix(link2, "/api/kalender/"), ".ics")) {
		t.Errorf("token not redacted: %s", got)
	}
}

func TestCalendarFeedRateLimit(t *testing.T) {
	s, _ := newTestServer()
	s.Cache = newMemCache()
	h := s.Routes()
	var last int
	for range calendarPerMinute + 1 {
		last = do(t, h, "GET", "/api/kalender/"+strings.Repeat("A", 43)+".ics", "").Code
	}
	if last != http.StatusTooManyRequests {
		t.Errorf("after %d feed requests: %d", calendarPerMinute+1, last)
	}
}

func TestExportCSV(t *testing.T) {
	s, _ := newTestServer()
	h := s.Routes()
	if rec := do(t, h, "GET", "/api/item.csv", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("without session: %d", rec.Code)
	}
	c := signUp(t, h, "csv@contoh.id")
	do(t, h, "POST", "/api/item", `{"nama":"=HYPERLINK(\"http://jahat\")","jenis":"token","klien":"+Klien, \"A\"",
		"kedaluwarsa":"2026-11-01","biaya":150000,"catatan":"@baris\ndua"}`, c)
	do(t, h, "POST", "/api/item", `{"nama":"Domain toko","jenis":"domain","host":"toko.id","kedaluwarsa":"2027-01-01"}`, c)

	rec := do(t, h, "GET", "/api/item.csv", "", c)
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "text/csv; charset=utf-8" ||
		rec.Header().Get("Content-Disposition") != `attachment; filename="tenggat-2026-11-09.csv"` {
		t.Fatalf("csv: %d %v", rec.Code, rec.Header())
	}
	body := rec.Body.Bytes()
	if !bytes.HasPrefix(body, []byte{0xEF, 0xBB, 0xBF}) {
		t.Fatal("no byte order mark")
	}
	rows, err := csv.NewReader(bytes.NewReader(body[3:])).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"nama", "jenis", "klien", "host", "kedaluwarsa", "sisa_hari", "status", "sumber", "biaya", "catatan"},
		{`'=HYPERLINK("http://jahat")`, "token", `'+Klien, "A"`, "", "2026-11-01", "-8", "lewat", "manual", "150000", "'@baris\ndua"},
		{"Domain toko", "domain", "", "toko.id", "2027-01-01", "53", "aman", "manual", "", ""},
	}
	if len(rows) != len(want) {
		t.Fatalf("rows = %q", rows)
	}
	for i := range want {
		if strings.Join(rows[i], "|") != strings.Join(want[i], "|") {
			t.Errorf("row %d = %q, want %q", i, rows[i], want[i])
		}
	}
}
