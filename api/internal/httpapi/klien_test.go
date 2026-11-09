package httpapi

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/mrramdn/tenggat/api/internal/expiry"
	"github.com/mrramdn/tenggat/api/internal/store"
)

// The memStore's client methods. Items keep a copy of their client's name, so
// renaming and deleting a client update the items as the SQL join would.

func (m *memStore) klienTaken(userID, nama, except string) bool {
	for _, k := range m.klien {
		if k.PenggunaID == userID && k.ID != except && strings.EqualFold(k.Nama, nama) {
			return true
		}
	}
	return false
}

func (m *memStore) ListKlien(_ context.Context, userID string, now time.Time) ([]store.KlienRingkas, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []store.KlienRingkas{}
	for _, k := range m.klien {
		if k.PenggunaID != userID {
			continue
		}
		r := store.KlienRingkas{Klien: k}
		for _, it := range m.items {
			if deref(it.KlienID) != k.ID {
				continue
			}
			r.JumlahAset++
			if r.Terdekat == nil || it.Kedaluwarsa.Before(*r.Terdekat) {
				d := it.Kedaluwarsa
				r.Terdekat = &d
			}
			if expiry.DaysLeft(it.Kedaluwarsa, now) <= 30 && it.Biaya != nil {
				r.Biaya30Hari += *it.Biaya
			}
		}
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Nama) < strings.ToLower(out[j].Nama) })
	return out, nil
}

func (m *memStore) GetKlien(_ context.Context, userID, id string) (store.Klien, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k, ok := m.klien[id]
	if !ok || k.PenggunaID != userID {
		return store.Klien{}, store.ErrNotFound
	}
	return k, nil
}

func (m *memStore) CreateKlien(_ context.Context, userID, nama, catatan string) (store.Klien, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.klienTaken(userID, nama, "") {
		return store.Klien{}, store.ErrKlienTaken
	}
	if m.klien == nil {
		m.klien = map[string]store.Klien{}
	}
	k := store.Klien{ID: m.newID(), PenggunaID: userID, Nama: nama, Catatan: catatan, Dibuat: testNow}
	m.klien[k.ID] = k
	return k, nil
}

func (m *memStore) UpdateKlien(_ context.Context, userID, id, nama, catatan string) (store.Klien, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k, ok := m.klien[id]
	if !ok || k.PenggunaID != userID {
		return store.Klien{}, store.ErrNotFound
	}
	if m.klienTaken(userID, nama, id) {
		return store.Klien{}, store.ErrKlienTaken
	}
	k.Nama, k.Catatan = nama, catatan
	m.klien[id] = k
	for itemID, it := range m.items {
		if deref(it.KlienID) == id {
			it.Klien = nama
			m.items[itemID] = it
		}
	}
	return k, nil
}

func (m *memStore) DeleteKlien(_ context.Context, userID, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if k, ok := m.klien[id]; !ok || k.PenggunaID != userID {
		return store.ErrNotFound
	}
	delete(m.klien, id)
	for itemID, it := range m.items {
		if deref(it.KlienID) == id {
			it.Klien, it.KlienID = "", nil
			m.items[itemID] = it
		}
	}
	return nil
}

func (m *memStore) EnsureKlien(ctx context.Context, userID, nama string) (store.Klien, error) {
	m.mu.Lock()
	for _, k := range m.klien {
		if k.PenggunaID == userID && strings.EqualFold(k.Nama, nama) {
			m.mu.Unlock()
			return k, nil
		}
	}
	m.mu.Unlock()
	return m.CreateKlien(ctx, userID, nama, "")
}

type klienList struct{ Klien []klienJSON }

func TestKlienCreateListRenameDelete(t *testing.T) {
	s, _ := newTestServer()
	h := s.Routes()
	c := signUp(t, h, "klien@contoh.id")

	rec := do(t, h, "POST", "/api/klien", `{"nama":"  Studio Rana ","catatan":"Bu Rani, bayar lewat transfer"}`, c)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body)
	}
	k := decode[klienJSON](t, rec)
	if k.Nama != "Studio Rana" || k.Catatan != "Bu Rani, bayar lewat transfer" || k.JumlahAset != 0 || k.Terdekat != nil {
		t.Errorf("created %+v", k)
	}
	if loc := rec.Header().Get("Location"); loc != "/api/klien/"+k.ID {
		t.Errorf("Location = %q", loc)
	}
	do(t, h, "POST", "/api/klien", `{"nama":"Apotek Sehat"}`, c)

	// Three assets for Studio Rana: one overdue and one due in 9 days, both with a cost
	// (the 30-day cost counts overdue assets too, like the dashboard), and one next year.
	for _, body := range []string{
		`{"nama":"Lisensi font","jenis":"lainnya","kedaluwarsa":"2026-11-01","biaya":450000,"klien_id":"` + k.ID + `"}`,
		`{"nama":"studiorana.co.id","jenis":"domain","kedaluwarsa":"2026-11-18","biaya":250000,"klien_id":"` + k.ID + `"}`,
		`{"nama":"Hosting","jenis":"langganan","kedaluwarsa":"2027-06-01","biaya":900000,"klien_id":"` + k.ID + `"}`,
	} {
		if rec := do(t, h, "POST", "/api/item", body, c); rec.Code != http.StatusCreated {
			t.Fatalf("item: %d %s", rec.Code, rec.Body)
		}
	}

	list := decode[klienList](t, do(t, h, "GET", "/api/klien", "", c)).Klien
	if len(list) != 2 || list[0].Nama != "Apotek Sehat" || list[1].Nama != "Studio Rana" {
		t.Fatalf("list %+v", list)
	}
	if got := list[1]; got.JumlahAset != 3 || got.Terdekat == nil || *got.Terdekat != "2026-11-01" || got.Biaya30Hari != 700000 {
		t.Errorf("figures %+v", got)
	}

	rec = do(t, h, "PUT", "/api/klien/"+k.ID, `{"nama":"Studio Rana Kreatif","catatan":""}`, c)
	if rec.Code != http.StatusOK || decode[klienJSON](t, rec).JumlahAset != 3 {
		t.Fatalf("rename: %d %s", rec.Code, rec.Body)
	}
	items := decode[struct{ Item []itemJSON }](t, do(t, h, "GET", "/api/item?klien_id="+k.ID, "", c)).Item
	if len(items) != 3 || items[0].Klien != "Studio Rana Kreatif" || deref(items[0].KlienID) != k.ID {
		t.Errorf("items after rename %+v", items)
	}
	if n := len(decode[struct{ Item []itemJSON }](t, do(t, h, "GET", "/api/item?klien=studio%20rana%20kreatif", "", c)).Item); n != 3 {
		t.Errorf("filter by name finds %d items", n)
	}

	if rec := do(t, h, "PUT", "/api/klien/"+k.ID, `{"nama":""}`, c); rec.Code != http.StatusUnprocessableEntity ||
		!strings.Contains(rec.Body.String(), `"nama"`) {
		t.Errorf("empty name: %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, "POST", "/api/klien", `{"nama":"`+strings.Repeat("a", 81)+`"}`, c); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("long name: %d", rec.Code)
	}
	if rec := do(t, h, "PUT", "/api/klien/00000000-0000-4000-8000-999999999999", `{"nama":"X"}`, c); rec.Code != http.StatusNotFound {
		t.Errorf("unknown id: %d", rec.Code)
	}

	// Deleting the client keeps its assets, without a client.
	if rec := do(t, h, "DELETE", "/api/klien/"+k.ID, "", c); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body)
	}
	items = decode[struct{ Item []itemJSON }](t, do(t, h, "GET", "/api/item", "", c)).Item
	if len(items) != 3 || items[0].Klien != "" || items[0].KlienID != nil || items[1].KlienID != nil || items[2].KlienID != nil {
		t.Errorf("items after delete %+v", items)
	}
	if list := decode[klienList](t, do(t, h, "GET", "/api/klien", "", c)).Klien; len(list) != 1 {
		t.Errorf("list after delete %+v", list)
	}
	if rec := do(t, h, "DELETE", "/api/klien/"+k.ID, "", c); rec.Code != http.StatusNotFound {
		t.Errorf("delete twice: %d", rec.Code)
	}
}

func TestKlienNameIsUniqueIgnoringCase(t *testing.T) {
	s, _ := newTestServer()
	h := s.Routes()
	c := signUp(t, h, "unik@contoh.id")

	do(t, h, "POST", "/api/klien", `{"nama":"Toko Budi"}`, c)
	rec := do(t, h, "POST", "/api/klien", `{"nama":"TOKO budi"}`, c)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), `"nama"`) {
		t.Errorf("duplicate create: %d %s", rec.Code, rec.Body)
	}
	other := decode[klienJSON](t, do(t, h, "POST", "/api/klien", `{"nama":"Kopi Senja"}`, c))
	if rec := do(t, h, "PUT", "/api/klien/"+other.ID, `{"nama":"toko BUDI"}`, c); rec.Code != http.StatusConflict {
		t.Errorf("duplicate rename: %d %s", rec.Code, rec.Body)
	}
	// Changing only the case of its own name is allowed.
	if rec := do(t, h, "PUT", "/api/klien/"+other.ID, `{"nama":"KOPI SENJA"}`, c); rec.Code != http.StatusOK {
		t.Errorf("own name in other case: %d %s", rec.Code, rec.Body)
	}
	// Another user may use the same name.
	b := signUp(t, h, "unik-b@contoh.id")
	if rec := do(t, h, "POST", "/api/klien", `{"nama":"Toko Budi"}`, b); rec.Code != http.StatusCreated {
		t.Errorf("same name for another user: %d %s", rec.Code, rec.Body)
	}
}

func TestKlienBelongsToOneUser(t *testing.T) {
	s, _ := newTestServer()
	h := s.Routes()
	a := signUp(t, h, "pemilik@contoh.id")
	b := signUp(t, h, "tamu@contoh.id")
	k := decode[klienJSON](t, do(t, h, "POST", "/api/klien", `{"nama":"Rahasia A"}`, a))

	if list := decode[klienList](t, do(t, h, "GET", "/api/klien", "", b)).Klien; len(list) != 0 {
		t.Errorf("B sees %+v", list)
	}
	if rec := do(t, h, "PUT", "/api/klien/"+k.ID, `{"nama":"Diambil"}`, b); rec.Code != http.StatusNotFound {
		t.Errorf("B renames: %d", rec.Code)
	}
	if rec := do(t, h, "DELETE", "/api/klien/"+k.ID, "", b); rec.Code != http.StatusNotFound {
		t.Errorf("B deletes: %d", rec.Code)
	}
	rec := do(t, h, "POST", "/api/item", `{"nama":"Domain B","jenis":"domain","kedaluwarsa":"2027-01-01","klien_id":"`+k.ID+`"}`, b)
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), `"klien"`) {
		t.Errorf("B attaches A's client: %d %s", rec.Code, rec.Body)
	}
	// klien_id wins over a name, so a valid name does not rescue a foreign id.
	rec = do(t, h, "POST", "/api/item", `{"nama":"Domain B","jenis":"domain","kedaluwarsa":"2027-01-01","klien":"Rahasia A","klien_id":"`+k.ID+`"}`, b)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("B attaches A's client with a name: %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, "GET", "/api/item?klien_id=bukan-uuid", "", b); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("bad klien_id filter: %d", rec.Code)
	}
}

func TestItemWithClientName(t *testing.T) {
	s, _ := newTestServer()
	h := s.Routes()
	c := signUp(t, h, "nama-klien@contoh.id")

	rec := do(t, h, "POST", "/api/item", `{"nama":"Domain toko","jenis":"domain","kedaluwarsa":"2027-01-01","klien":"Toko Budi"}`, c)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body)
	}
	it := decode[itemJSON](t, rec)
	list := decode[klienList](t, do(t, h, "GET", "/api/klien", "", c)).Klien
	if len(list) != 1 || list[0].Nama != "Toko Budi" || deref(it.KlienID) != list[0].ID || it.Klien != "Toko Budi" {
		t.Fatalf("new name: item %+v, clients %+v", it, list)
	}

	// The same name in another case reuses the client and keeps its spelling.
	rec = do(t, h, "POST", "/api/item", `{"nama":"Email toko","jenis":"langganan","kedaluwarsa":"2027-02-01","klien":"toko budi"}`, c)
	if got := decode[itemJSON](t, rec); got.Klien != "Toko Budi" || deref(got.KlienID) != list[0].ID {
		t.Errorf("existing name in other case: %+v", got)
	}
	if list := decode[klienList](t, do(t, h, "GET", "/api/klien", "", c)).Klien; len(list) != 1 || list[0].JumlahAset != 2 {
		t.Errorf("clients %+v", list)
	}

	// An empty name removes the client from the asset.
	rec = do(t, h, "PUT", "/api/item/"+it.ID, `{"nama":"Domain toko","jenis":"domain","kedaluwarsa":"2027-01-01","klien":""}`, c)
	if got := decode[itemJSON](t, rec); rec.Code != http.StatusOK || got.KlienID != nil || got.Klien != "" {
		t.Errorf("clear client: %d %s", rec.Code, rec.Body)
	}
	// klien_id wins over a name.
	rec = do(t, h, "PUT", "/api/item/"+it.ID, `{"nama":"Domain toko","jenis":"domain","kedaluwarsa":"2027-01-01","klien":"Lain","klien_id":"`+list[0].ID+`"}`, c)
	if got := decode[itemJSON](t, rec); got.Klien != "Toko Budi" {
		t.Errorf("klien_id over name: %s", rec.Body)
	}
	if list := decode[klienList](t, do(t, h, "GET", "/api/klien", "", c)).Klien; len(list) != 1 {
		t.Errorf("a name next to klien_id created a client: %+v", list)
	}
}

func TestKlienWritesRefreshCachedReads(t *testing.T) {
	s, _ := newTestServer()
	s.Cache = newMemCache()
	h := s.Routes()
	c := signUp(t, h, "cache-klien@contoh.id")

	read := func(path string) string {
		t.Helper()
		return do(t, h, "GET", path, "", c).Header().Get("X-Cache")
	}
	paths := []string{"/api/klien", "/api/item", "/api/ringkasan"}
	warm := func(step string) {
		t.Helper()
		for _, p := range paths {
			if got := read(p); got != "MISS" {
				t.Errorf("%s: first read of %s after the write is %q, want MISS", step, p, got)
			}
			if got := read(p); got != "HIT" {
				t.Errorf("%s: second read of %s is %q, want HIT", step, p, got)
			}
		}
	}

	warm("start")
	k := decode[klienJSON](t, do(t, h, "POST", "/api/klien", `{"nama":"Toko Budi"}`, c))
	warm("create")
	do(t, h, "POST", "/api/item", `{"nama":"Domain","jenis":"domain","kedaluwarsa":"2027-01-01","klien_id":"`+k.ID+`"}`, c)
	warm("item")
	do(t, h, "PUT", "/api/klien/"+k.ID, `{"nama":"Toko Budi Jaya"}`, c)
	warm("rename")
	do(t, h, "DELETE", "/api/klien/"+k.ID, "", c)
	warm("delete")
}
