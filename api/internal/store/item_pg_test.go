package store

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/mrramdn/tenggat/api/internal/expiry"
)

func idItem(items []Item) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.ID
	}
	return out
}

func TestItemRoundTrip(t *testing.T) {
	t.Parallel()
	s := storeSiap(t)
	ctx := t.Context()
	u := penggunaUji(t, s, "Rana")
	k := klienUji(t, s, u.ID, "Toko Kue Mawar")

	at := time.Date(2026, 11, 19, 1, 2, 3, 0, time.UTC)
	info := &TLSInfo{Penerbit: "Let's Encrypt · R12", Subjek: "tokokuemawar.id", Alamat: "203.0.113.10",
		BerlakuSejak: time.Date(2026, 10, 16, 8, 0, 0, 0, time.UTC), Berakhir: time.Date(2027, 1, 14, 7, 59, 59, 0, time.UTC),
		RantaiValid: false, GalatRantai: "x509: certificate signed by unknown authority"}
	in := Input{Nama: "Sertifikat toko", Jenis: "sertifikat", KlienID: &k.ID, Host: ptr("tokokuemawar.id"),
		Kedaluwarsa: tanggal(2027, 1, 14), Sumber: "server", Catatan: "Auto-renew lewat certbot", InfoTLS: info, Dicek: &at}
	it, err := s.Create(ctx, u.ID, in)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if it.ID == "" || it.PenggunaID != u.ID || it.Nama != in.Nama || it.Jenis != "sertifikat" || it.Klien != k.Nama ||
		it.KlienID == nil || *it.KlienID != k.ID || str(it.Host) != `"tokokuemawar.id"` ||
		!it.Kedaluwarsa.Equal(tanggal(2027, 1, 14)) || it.Sumber != "server" || it.Biaya != nil || it.Catatan != in.Catatan ||
		it.InfoTLS == nil || jsonOf(t, *it.InfoTLS) != jsonOf(t, *info) || it.InfoRegistri != nil ||
		!sama(it.TerakhirDicek, &at) || it.GalatCek != nil || it.Dibuat.IsZero() || it.Diubah.IsZero() {
		t.Errorf("Create = %+v", it)
	}
	got, err := s.Get(ctx, u.ID, it.ID)
	if err != nil || jsonOf(t, got) != jsonOf(t, it) {
		t.Errorf("Get = %+v, %v; want %+v", got, err, it)
	}

	// A registry domain with a cost, without a client.
	reg := &RegistryInfo{Domain: "tokokuemawar.id", Registrar: "PT Registrar Nusantara", Server: "https://rdap.pandi.id/rdap/"}
	dom, err := s.Create(ctx, u.ID, Input{Nama: "tokokuemawar.id", Jenis: "domain", Host: ptr("tokokuemawar.id"),
		Kedaluwarsa: tanggal(2027, 3, 2), Sumber: "registri", Biaya: ptr[int64](250000), Registri: reg, Dicek: &at})
	if err != nil {
		t.Fatalf("Create(domain): %v", err)
	}
	if dom.Klien != "" || dom.KlienID != nil || dom.Biaya == nil || *dom.Biaya != 250000 ||
		dom.InfoRegistri == nil || *dom.InfoRegistri != *reg || dom.InfoTLS != nil {
		t.Errorf("Create(domain) = %+v", dom)
	}

	// Update replaces the editable fields; a nil Dicek keeps the last check time.
	upd := Input{Nama: "Sertifikat toko (baru)", Jenis: "sertifikat", Host: ptr("www.tokokuemawar.id"),
		Kedaluwarsa: tanggal(2027, 4, 1), Sumber: "server", Biaya: ptr[int64](0), Catatan: ""}
	it2, err := s.Update(ctx, u.ID, it.ID, upd)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if it2.Nama != upd.Nama || it2.KlienID != nil || it2.Klien != "" || str(it2.Host) != `"www.tokokuemawar.id"` ||
		!it2.Kedaluwarsa.Equal(tanggal(2027, 4, 1)) || it2.Biaya == nil || *it2.Biaya != 0 || it2.InfoTLS != nil ||
		!sama(it2.TerakhirDicek, &at) || !it2.Dibuat.Equal(it.Dibuat) || it2.Diubah.Before(it.Diubah) {
		t.Errorf("Update = %+v", it2)
	}
	if _, err := s.Update(ctx, u.ID, "00000000-0000-4000-8000-000000000999", upd); !errors.Is(err, ErrNotFound) {
		t.Errorf("Update(unknown) err = %v, want ErrNotFound", err)
	}

	if err := s.Delete(ctx, u.ID, it.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.Get(ctx, u.ID, it.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get after Delete err = %v, want ErrNotFound", err)
	}
	if err := s.Delete(ctx, u.ID, it.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("second Delete err = %v, want ErrNotFound", err)
	}
	if _, err := s.Get(ctx, u.ID, dom.ID); err != nil {
		t.Errorf("Delete removed another item: %v", err)
	}
}

// TestItemUpdateCheckError pins how Update treats the last check's error.
func TestItemUpdateCheckError(t *testing.T) {
	t.Parallel()
	s := storeSiap(t)
	ctx := t.Context()
	u := penggunaUji(t, s, "Rana")
	at := time.Date(2026, 11, 19, 1, 0, 0, 0, time.UTC)

	cert := Input{Nama: "Sertifikat", Jenis: "sertifikat", Host: ptr("contoh.id"), Kedaluwarsa: tanggal(2027, 1, 1), Sumber: "server"}
	it, err := s.Create(ctx, u.ID, cert)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveCheckError(ctx, it.ID, "koneksi ditolak", at); err != nil {
		t.Fatal(err)
	}
	// An edit that did not read the server keeps the error.
	got, err := s.Update(ctx, u.ID, it.ID, cert)
	if err != nil || got.GalatCek == nil || *got.GalatCek != "koneksi ditolak" || !sama(got.TerakhirDicek, &at) {
		t.Errorf("Update without a check = %+v, %v; want the error kept", got, err)
	}
	// An edit that just read the server clears it.
	later := at.Add(time.Hour)
	cert.Dicek = &later
	got, err = s.Update(ctx, u.ID, it.ID, cert)
	if err != nil || got.GalatCek != nil || !sama(got.TerakhirDicek, &later) {
		t.Errorf("Update with a check = %+v, %v; want the error cleared", got, err)
	}

	// Turning a registry domain into a manual one clears its error too.
	dom := Input{Nama: "contoh.id", Jenis: "domain", Host: ptr("contoh.id"), Kedaluwarsa: tanggal(2027, 1, 1), Sumber: "registri"}
	d, err := s.Create(ctx, u.ID, dom)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveCheckError(ctx, d.ID, "RDAP 503", at); err != nil {
		t.Fatal(err)
	}
	dom.Sumber = "manual"
	got, err = s.Update(ctx, u.ID, d.ID, dom)
	if err != nil || got.GalatCek != nil || !sama(got.TerakhirDicek, &at) {
		t.Errorf("Update to manual = %+v, %v; want the error cleared and the check time kept", got, err)
	}
}

func TestItemScopedToUser(t *testing.T) {
	t.Parallel()
	s := storeSiap(t)
	ctx := t.Context()
	a := penggunaUji(t, s, "Rana")
	b := penggunaUji(t, s, "Budi")
	it := itemUji(t, s, a.ID, "Domain A", "domain", tanggal(2027, 1, 1))

	if _, err := s.Get(ctx, b.ID, it.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get by another user err = %v, want ErrNotFound", err)
	}
	if _, err := s.Update(ctx, b.ID, it.ID, Input{Nama: "Dicuri", Jenis: "domain", Kedaluwarsa: tanggal(2030, 1, 1),
		Sumber: "manual"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("Update by another user err = %v, want ErrNotFound", err)
	}
	if err := s.Delete(ctx, b.ID, it.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Delete by another user err = %v, want ErrNotFound", err)
	}
	list, err := s.List(ctx, b.ID, Filter{}, time.Now())
	if err != nil || list == nil || len(list) != 0 {
		t.Errorf("List(another user) = %v, %v; want an empty, non-nil list", list, err)
	}
	if sum, err := s.Summary(ctx, b.ID, time.Now()); err != nil || sum.Total != 0 || sum.Klien == nil {
		t.Errorf("Summary(another user) = %+v, %v", sum, err)
	}
	if got, err := s.Get(ctx, a.ID, it.ID); err != nil || got.Nama != "Domain A" || !got.Kedaluwarsa.Equal(tanggal(2027, 1, 1)) {
		t.Errorf("owner's item changed: %+v, %v", got, err)
	}
}

func TestItemListFilters(t *testing.T) {
	t.Parallel()
	s := storeSiap(t)
	ctx := t.Context()
	u := penggunaUji(t, s, "Rana")
	other := penggunaUji(t, s, "Budi")
	mawar := klienUji(t, s, u.ID, "Toko Kue Mawar")
	senja := klienUji(t, s, u.ID, "Kopi Senja")
	now := time.Date(2026, 11, 19, 7, 0, 0, 0, time.UTC) // 14:00 in Jakarta
	today := expiry.DateOf(now)

	mk := func(nama string, klienID *string, days int) Item {
		t.Helper()
		it, err := s.Create(ctx, u.ID, Input{Nama: nama, Jenis: "lainnya", KlienID: klienID,
			Kedaluwarsa: today.AddDate(0, 0, days), Sumber: "manual"})
		if err != nil {
			t.Fatal(err)
		}
		return it
	}
	m1 := mk("Mawar B", &mawar.ID, 3)
	m2 := mk("Mawar A", &mawar.ID, 3) // same date: ordered by name
	m3 := mk("Mawar lama", &mawar.ID, -5)
	s1 := mk("Senja", &senja.ID, 20)
	n1 := mk("Tanpa klien", nil, 100)
	itemUji(t, s, other.ID, "Punya Budi", "lainnya", today)

	cases := []struct {
		name string
		f    Filter
		want []Item
	}{
		{"all, nearest first", Filter{}, []Item{m3, m2, m1, s1, n1}},
		{"client name, any case", Filter{Klien: "toko KUE mawar"}, []Item{m3, m2, m1}},
		{"client id", Filter{KlienID: senja.ID}, []Item{s1}},
		{"unknown client name", Filter{Klien: "Tidak Ada"}, nil},
		{"status and client", Filter{Status: expiry.Kritis, KlienID: mawar.ID}, []Item{m2, m1}},
		{"status lewat", Filter{Status: expiry.Lewat}, []Item{m3}},
		{"status segera", Filter{Status: expiry.Segera}, []Item{s1}},
		{"status aman", Filter{Status: expiry.Aman}, []Item{n1}},
		{"status of another client", Filter{Status: expiry.Aman, Klien: "Kopi Senja"}, nil},
	}
	for _, c := range cases {
		got, err := s.List(ctx, u.ID, c.f, now)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if !slices.Equal(idItem(got), idItem(c.want)) {
			t.Errorf("%s: got %v, want %v", c.name, idItem(got), idItem(c.want))
		}
	}
	got, _ := s.List(ctx, u.ID, Filter{KlienID: mawar.ID}, now)
	for _, it := range got {
		if it.Klien != "Toko Kue Mawar" {
			t.Errorf("List row %s has client %q", it.Nama, it.Klien)
		}
	}
}

func TestSummary(t *testing.T) {
	t.Parallel()
	s := storeSiap(t)
	ctx := t.Context()
	u := penggunaUji(t, s, "Rana")
	other := penggunaUji(t, s, "Budi")
	now := time.Date(2026, 11, 19, 7, 0, 0, 0, time.UTC)
	today := expiry.DateOf(now)

	for _, nama := range []string{"beta", "Alfa", "alfa dua"} {
		klienUji(t, s, u.ID, nama)
	}
	klienUji(t, s, other.ID, "Klien Budi")
	for i, days := range []int{-10, -1, 0, 5, 7, 8, 30, 31, 400} {
		itemUji(t, s, u.ID, "Aset "+string(rune('a'+i)), "lainnya", today.AddDate(0, 0, days))
	}
	itemUji(t, s, other.ID, "Aset Budi", "lainnya", today)

	sum, err := s.Summary(ctx, u.ID, now)
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	want := Summary{Lewat: 2, Kritis: 3, Segera: 2, Aman: 2, Total: 9, Klien: []string{"Alfa", "alfa dua", "beta"}}
	if jsonOf(t, sum) != jsonOf(t, want) {
		t.Errorf("Summary = %+v, want %+v", sum, want)
	}

	empty := penggunaUji(t, s, "Kosong")
	sum, err = s.Summary(ctx, empty.ID, now)
	if err != nil || jsonOf(t, sum) != `{"lewat":0,"kritis":0,"segera":0,"aman":0,"total":0,"klien":[]}` {
		t.Errorf("Summary(no items) = %s, %v", jsonOf(t, sum), err)
	}
}

// TestStatusParitySQLAndExpiry makes sure the date ranges in SQL (Summary and
// List with a status) put every item in the bucket expiry.For puts it in,
// including at the edges of the Jakarta day.
func TestStatusParitySQLAndExpiry(t *testing.T) {
	t.Parallel()
	s := storeSiap(t)
	instants := map[string]time.Time{
		"now":                       time.Now(),
		"last second of a WIB day":  time.Date(2026, 11, 19, 16, 59, 59, 0, time.UTC),
		"first second of a WIB day": time.Date(2026, 11, 19, 17, 0, 0, 0, time.UTC),
		"WIB new year, UTC old":     time.Date(2026, 12, 31, 17, 30, 0, 0, time.UTC),
		"leap day in WIB":           time.Date(2028, 2, 28, 18, 0, 0, 0, time.UTC),
		"UTC midnight":              time.Date(2027, 3, 1, 0, 0, 0, 0, time.UTC),
	}
	offsets := []int{-400, -31, -2, -1, 0, 1, 6, 7, 8, 9, 29, 30, 31, 32, 365}

	for name, now := range instants {
		t.Run(name, func(t *testing.T) {
			ctx := t.Context()
			u := penggunaUji(t, s, "Rana")
			today := expiry.DateOf(now)
			want := map[expiry.Status][]string{}
			for _, off := range offsets {
				it := itemUji(t, s, u.ID, "Aset", "lainnya", today.AddDate(0, 0, off))
				days := expiry.DaysLeft(it.Kedaluwarsa, now)
				if days != off {
					t.Fatalf("DaysLeft(%s) = %d, want %d", it.Kedaluwarsa.Format(time.DateOnly), days, off)
				}
				st := expiry.For(days)
				want[st] = append(want[st], it.ID)
			}

			sum, err := s.Summary(ctx, u.ID, now)
			if err != nil {
				t.Fatal(err)
			}
			counts := map[expiry.Status]int{expiry.Lewat: sum.Lewat, expiry.Kritis: sum.Kritis, expiry.Segera: sum.Segera,
				expiry.Aman: sum.Aman}
			if sum.Total != len(offsets) {
				t.Errorf("Total = %d, want %d", sum.Total, len(offsets))
			}
			for _, st := range expiry.Statuses {
				if counts[st] != len(want[st]) {
					t.Errorf("Summary %s = %d, expiry.For says %d", st, counts[st], len(want[st]))
				}
				got, err := s.List(ctx, u.ID, Filter{Status: st}, now)
				if err != nil {
					t.Fatal(err)
				}
				ids := idItem(got)
				for _, it := range got {
					if f := expiry.For(expiry.DaysLeft(it.Kedaluwarsa, now)); f != st {
						t.Errorf("List(%s) returned %s, which expiry.For calls %s", st, it.Kedaluwarsa.Format(time.DateOnly), f)
					}
				}
				w := slices.Clone(want[st])
				slices.Sort(ids)
				slices.Sort(w)
				if !slices.Equal(ids, w) {
					t.Errorf("List(%s) = %d items, expiry.For says %d", st, len(ids), len(w))
				}
			}
		})
	}
}

func TestSaveCheck(t *testing.T) {
	t.Parallel()
	s := storeSiap(t)
	ctx := t.Context()
	u := penggunaUji(t, s, "Rana")
	k := klienUji(t, s, u.ID, "Toko Kue Mawar")
	cert, err := s.Create(ctx, u.ID, Input{Nama: "Sertifikat", Jenis: "sertifikat", KlienID: &k.ID, Host: ptr("contoh.id"),
		Kedaluwarsa: tanggal(2026, 12, 1), Sumber: "server", Catatan: "catatan tetap"})
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 11, 19, 1, 0, 0, 0, time.UTC)
	if _, err := s.SaveCheckError(ctx, cert.ID, "timeout", at); err != nil {
		t.Fatal(err)
	}

	info := TLSInfo{Penerbit: "Google Trust Services · WE1", Subjek: "contoh.id", Alamat: "198.51.100.7",
		BerlakuSejak: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), Berakhir: time.Date(2027, 1, 30, 23, 59, 59, 0, time.UTC),
		RantaiValid: true}
	later := at.Add(time.Hour)
	got, err := s.SaveCheck(ctx, cert.ID, tanggal(2027, 1, 30), info, later)
	if err != nil {
		t.Fatalf("SaveCheck: %v", err)
	}
	if !got.Kedaluwarsa.Equal(tanggal(2027, 1, 30)) || got.InfoTLS == nil || jsonOf(t, *got.InfoTLS) != jsonOf(t, info) ||
		!sama(got.TerakhirDicek, &later) || got.GalatCek != nil || got.Klien != k.Nama || got.Catatan != "catatan tetap" {
		t.Errorf("SaveCheck = %+v", got)
	}
	if again, err := s.Get(ctx, u.ID, cert.ID); err != nil || jsonOf(t, again) != jsonOf(t, got) {
		t.Errorf("Get after SaveCheck = %+v, %v", again, err)
	}

	// Only certificates take a certificate check.
	dom, err := s.Create(ctx, u.ID, Input{Nama: "contoh.id", Jenis: "domain", Host: ptr("contoh.id"),
		Kedaluwarsa: tanggal(2027, 1, 1), Sumber: "registri"})
	if err != nil {
		t.Fatal(err)
	}
	token := itemUji(t, s, u.ID, "Token", "token", tanggal(2027, 1, 1))
	for _, id := range []string{dom.ID, token.ID, "00000000-0000-4000-8000-000000000999"} {
		if _, err := s.SaveCheck(ctx, id, tanggal(2030, 1, 1), info, later); !errors.Is(err, ErrNotFound) {
			t.Errorf("SaveCheck(%s) err = %v, want ErrNotFound", id, err)
		}
	}
	if d, _ := s.Get(ctx, u.ID, dom.ID); !d.Kedaluwarsa.Equal(tanggal(2027, 1, 1)) || d.InfoTLS != nil {
		t.Errorf("SaveCheck changed a domain: %+v", d)
	}
}

func TestSaveCheckError(t *testing.T) {
	t.Parallel()
	s := storeSiap(t)
	ctx := t.Context()
	u := penggunaUji(t, s, "Rana")
	info := &TLSInfo{Penerbit: "R12", Subjek: "contoh.id", RantaiValid: true}
	at := time.Date(2026, 11, 19, 1, 0, 0, 0, time.UTC)
	cert, err := s.Create(ctx, u.ID, Input{Nama: "Sertifikat", Jenis: "sertifikat", Host: ptr("contoh.id"),
		Kedaluwarsa: tanggal(2026, 12, 1), Sumber: "server", InfoTLS: info, Dicek: &at})
	if err != nil {
		t.Fatal(err)
	}
	reg, err := s.Create(ctx, u.ID, Input{Nama: "contoh.id", Jenis: "domain", Host: ptr("contoh.id"),
		Kedaluwarsa: tanggal(2027, 2, 1), Sumber: "registri"})
	if err != nil {
		t.Fatal(err)
	}

	later := at.Add(6 * time.Hour)
	got, err := s.SaveCheckError(ctx, cert.ID, "tls: handshake failure", later)
	if err != nil {
		t.Fatalf("SaveCheckError(cert): %v", err)
	}
	if got.GalatCek == nil || *got.GalatCek != "tls: handshake failure" || !sama(got.TerakhirDicek, &later) ||
		!got.Kedaluwarsa.Equal(tanggal(2026, 12, 1)) || got.InfoTLS == nil || got.InfoTLS.Penerbit != "R12" {
		t.Errorf("SaveCheckError(cert) = %+v; want the error, the new time and the last known date", got)
	}
	got, err = s.SaveCheckError(ctx, reg.ID, "RDAP 404", later)
	if err != nil || got.GalatCek == nil || *got.GalatCek != "RDAP 404" || !got.Kedaluwarsa.Equal(tanggal(2027, 2, 1)) {
		t.Errorf("SaveCheckError(registry) = %+v, %v", got, err)
	}

	manual, err := s.Create(ctx, u.ID, Input{Nama: "contoh.id", Jenis: "domain", Host: ptr("contoh.id"),
		Kedaluwarsa: tanggal(2027, 2, 1), Sumber: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{manual.ID, itemUji(t, s, u.ID, "Token", "token", tanggal(2027, 1, 1)).ID} {
		if _, err := s.SaveCheckError(ctx, id, "x", later); !errors.Is(err, ErrNotFound) {
			t.Errorf("SaveCheckError(manual item) err = %v, want ErrNotFound", err)
		}
	}
}

func TestDueForCheck(t *testing.T) {
	t.Parallel()
	s := storeSiap(t)
	ctx := t.Context()
	a := penggunaUji(t, s, "Rana")
	b := penggunaUji(t, s, "Budi")
	now := time.Date(2026, 11, 19, 7, 0, 0, 0, time.UTC)
	before := now.Add(-24 * time.Hour)

	mk := func(userID, nama, jenis, sumber string, host *string, dicek *time.Time) Item {
		t.Helper()
		it, err := s.Create(ctx, userID, Input{Nama: nama, Jenis: jenis, Host: host, Kedaluwarsa: tanggal(2027, 1, 1),
			Sumber: sumber, Dicek: dicek})
		if err != nil {
			t.Fatalf("Create(%s): %v", nama, err)
		}
		return it
	}
	never := mk(a.ID, "Sertifikat baru", "sertifikat", "server", ptr("baru.contoh.id"), nil)
	old := mk(a.ID, "Sertifikat lama", "sertifikat", "server", ptr("lama.contoh.id"), ptr(now.Add(-48*time.Hour)))
	mk(a.ID, "Sertifikat segar", "sertifikat", "server", ptr("segar.contoh.id"), ptr(now.Add(-time.Hour)))
	reg := mk(a.ID, "contoh.id", "domain", "registri", ptr("contoh.id"), ptr(now.Add(-30*time.Hour)))
	mk(a.ID, "segar.id", "domain", "registri", ptr("segar.id"), ptr(now.Add(-2*time.Hour)))
	mk(a.ID, "manual.id", "domain", "manual", ptr("manual.id"), nil)
	mk(a.ID, "Token", "token", "manual", nil, nil)
	otherUser := mk(b.ID, "Sertifikat Budi", "sertifikat", "server", ptr("tokobudi.id"), ptr(now.Add(-25*time.Hour)))
	// At exactly the cut-off a check counts as recent.
	mk(b.ID, "Sertifikat tepat", "sertifikat", "server", ptr("tepat.tokobudi.id"), &before)

	got, err := s.DueForCheck(ctx, before)
	if err != nil {
		t.Fatalf("DueForCheck: %v", err)
	}
	want := []string{never.ID, old.ID, reg.ID, otherUser.ID}
	if !slices.Equal(idItem(got), want) {
		t.Errorf("DueForCheck = %v, want %v (never checked first, then oldest)", idItem(got), want)
	}

	if got, err := s.DueForCheck(ctx, now.Add(-72*time.Hour)); err != nil || !slices.Equal(idItem(got), []string{never.ID}) {
		t.Errorf("DueForCheck(3 days ago) = %v, %v", idItem(got), err)
	}
}

func TestItemConstraintsThroughStore(t *testing.T) {
	t.Parallel()
	s := storeSiap(t)
	ctx := t.Context()
	u := penggunaUji(t, s, "Rana")

	_, err := s.Create(ctx, u.ID, Input{Nama: "Sertifikat", Jenis: "sertifikat", Kedaluwarsa: tanggal(2027, 1, 1), Sumber: "server"})
	mustCheckViolation(t, err, "sertifikat_punya_host")
	_, err = s.Create(ctx, u.ID, Input{Nama: "Token", Jenis: "token", Host: ptr("contoh.id"), Kedaluwarsa: tanggal(2027, 1, 1),
		Sumber: "registri"})
	mustCheckViolation(t, err, "registri_punya_domain")
	_, err = s.Create(ctx, u.ID, Input{Nama: "Aset", Jenis: "kendaraan", Kedaluwarsa: tanggal(2027, 1, 1), Sumber: "manual"})
	mustCheckViolation(t, err, "item_jenis_check")
	_, err = s.Create(ctx, u.ID, Input{Nama: "Aset", Jenis: "lainnya", KlienID: ptr("00000000-0000-4000-8000-000000000999"),
		Kedaluwarsa: tanggal(2027, 1, 1), Sumber: "manual"})
	if code, _ := kodeSQL(err); code != "23503" {
		t.Errorf("Create with an unknown client err = %v, want foreign_key_violation", err)
	}
	if n := hitung(t, s, `SELECT count(*) FROM item`); n != 0 {
		t.Errorf("%d rows written by refused creates", n)
	}
}

func TestItemV2ColumnsAndPerpanjang(t *testing.T) {
	t.Parallel()
	s := storeSiap(t)
	ctx := t.Context()
	u := penggunaUji(t, s, "Rana")
	other := penggunaUji(t, s, "Budi")
	k := klienUji(t, s, u.ID, "Toko Kue Mawar")
	now := time.Date(2026, 11, 19, 7, 0, 0, 0, time.UTC)

	// Recurring USD bill.
	due1 := tanggal(2026, 11, 20)
	due2 := tanggal(2026, 12, 20)
	bill, err := s.Create(ctx, u.ID, Input{
		Nama:        "Claude Pro",
		Jenis:       "tagihan",
		KlienID:     &k.ID,
		Kedaluwarsa: due1,
		Sumber:      "manual",
		Biaya:       ptr[int64](2000),
		MataUang:    "USD",
		UlangJumlah: 1,
		UlangSatuan: "bulan",
	})
	if err != nil {
		t.Fatalf("Create(tagihan USD): %v", err)
	}
	if bill.MataUang != "USD" || bill.UlangJumlah != 1 || bill.UlangSatuan != "bulan" || bill.SelesaiPada != nil {
		t.Errorf("Create(tagihan USD) = %+v", bill)
	}

	// One-off IDR bill (empty MataUang defaults to IDR, zero recurrence stored as NULL).
	onceDue := tanggal(2026, 11, 25)
	once, err := s.Create(ctx, u.ID, Input{
		Nama:        "Pelunasan Desain",
		Jenis:       "tagihan",
		KlienID:     &k.ID,
		Kedaluwarsa: onceDue,
		Sumber:      "manual",
		Biaya:       ptr[int64](1500000),
	})
	if err != nil {
		t.Fatalf("Create(one-off tagihan): %v", err)
	}
	if once.MataUang != "IDR" || once.UlangJumlah != 0 || once.UlangSatuan != "" || once.SelesaiPada != nil {
		t.Errorf("Create(one-off tagihan) = %+v", once)
	}
	if n := hitung(t, s, `SELECT count(*) FROM item WHERE id = $1 AND ulang_jumlah IS NULL AND ulang_satuan IS NULL`, once.ID); n != 1 {
		t.Errorf("one-off item recurrence not stored as NULL in SQL")
	}

	// Wrong Dari or another user writes nothing and no activity row.
	badP := Perpanjangan{
		Dari:   tanggal(2026, 11, 19),
		Sampai: &due2,
		At:     now,
		Aktivitas: AktivitasInput{
			KlienID:  &k.ID,
			Jenis:    "tagihan_dibayar",
			Teks:     "Tidak boleh tercatat",
			Tanggal:  tanggal(2026, 11, 19),
			Jumlah:   ptr[int64](2000),
			MataUang: ptr("USD"),
		},
	}
	if _, err := s.PerpanjangItem(ctx, u.ID, bill.ID, badP); !errors.Is(err, ErrNotFound) {
		t.Errorf("PerpanjangItem(wrong Dari) err = %v, want ErrNotFound", err)
	}
	badP.Dari = due1
	if _, err := s.PerpanjangItem(ctx, other.ID, bill.ID, badP); !errors.Is(err, ErrNotFound) {
		t.Errorf("PerpanjangItem(other user) err = %v, want ErrNotFound", err)
	}
	if n := hitung(t, s, `SELECT count(*) FROM aktivitas`); n != 0 {
		t.Errorf("failed PerpanjangItem wrote %d activity rows, want 0", n)
	}

	// Matching Dari moves the date once and writes the activity row atomically.
	pay1 := Perpanjangan{
		Dari:   due1,
		Sampai: &due2,
		At:     now,
		Aktivitas: AktivitasInput{
			KlienID:  &k.ID,
			Jenis:    "tagihan_dibayar",
			Teks:     "Tagihan Claude Pro dibayar US$20.00",
			Tanggal:  tanggal(2026, 11, 19),
			Jumlah:   ptr[int64](2000),
			MataUang: ptr("USD"),
		},
	}
	moved, err := s.PerpanjangItem(ctx, u.ID, bill.ID, pay1)
	if err != nil {
		t.Fatalf("PerpanjangItem: %v", err)
	}
	if !moved.Kedaluwarsa.Equal(due2) || moved.SelesaiPada != nil || moved.Klien != k.Nama {
		t.Errorf("PerpanjangItem = %+v", moved)
	}
	// Repeating the same PerpanjangItem with the old Dari fails and does not write a second activity row.
	if _, err := s.PerpanjangItem(ctx, u.ID, bill.ID, pay1); !errors.Is(err, ErrNotFound) {
		t.Errorf("second PerpanjangItem(old Dari) err = %v, want ErrNotFound", err)
	}
	feed, err := s.ListAktivitas(ctx, u.ID, AktivitasFilter{})
	if err != nil || len(feed) != 1 || feed[0].Jenis != "tagihan_dibayar" ||
		feed[0].ItemID == nil || *feed[0].ItemID != bill.ID ||
		feed[0].Jumlah == nil || *feed[0].Jumlah != 2000 ||
		feed[0].MataUang == nil || *feed[0].MataUang != "USD" {
		t.Errorf("ListAktivitas after PerpanjangItem = %+v, %v", feed, err)
	}

	// Lunas=true closes the one-off bill, sets selesai_pada = At, and records payment.
	closeAt := now.Add(time.Hour)
	closed, err := s.PerpanjangItem(ctx, u.ID, once.ID, Perpanjangan{
		Dari:  onceDue,
		Lunas: true,
		At:    closeAt,
		Aktivitas: AktivitasInput{
			Jenis:    "tagihan_dibayar",
			Teks:     "Tagihan Pelunasan Desain lunas Rp 1.500.000",
			Tanggal:  tanggal(2026, 11, 19),
			Jumlah:   ptr[int64](1500000),
			MataUang: ptr("IDR"),
		},
	})
	if err != nil {
		t.Fatalf("PerpanjangItem(Lunas): %v", err)
	}
	if !closed.Kedaluwarsa.Equal(onceDue) || !sama(closed.SelesaiPada, &closeAt) {
		t.Errorf("PerpanjangItem(Lunas) = %+v", closed)
	}

	// Closed item cannot be extended again or updated.
	if _, err := s.PerpanjangItem(ctx, u.ID, once.ID, Perpanjangan{
		Dari:  onceDue,
		Lunas: true,
		At:    closeAt.Add(time.Minute),
		Aktivitas: AktivitasInput{
			Jenis:   "tagihan_dibayar",
			Teks:    "Bayar dua kali",
			Tanggal: tanggal(2026, 11, 19),
		},
	}); !errors.Is(err, ErrNotFound) {
		t.Errorf("PerpanjangItem(already closed) err = %v, want ErrNotFound", err)
	}
	if _, err := s.Update(ctx, u.ID, once.ID, Input{
		Nama:        "Diubah setelah lunas",
		Jenis:       "tagihan",
		Kedaluwarsa: onceDue,
		Sumber:      "manual",
	}); !errors.Is(err, ErrNotFound) {
		t.Errorf("Update(closed item) err = %v, want ErrNotFound", err)
	}
	if n := hitung(t, s, `SELECT count(*) FROM aktivitas`); n != 2 {
		t.Errorf("aktivitas has %d rows, want 2", n)
	}

	// Filter.Selesai, Summary, and DueForCheck respect selesai_pada.
	reg, err := s.Create(ctx, u.ID, Input{
		Nama:        "habis.id",
		Jenis:       "domain",
		Host:        ptr("habis.id"),
		Kedaluwarsa: tanggal(2026, 11, 22),
		Sumber:      "registri",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.PerpanjangItem(ctx, u.ID, reg.ID, Perpanjangan{
		Dari:  tanggal(2026, 11, 22),
		Lunas: true,
		At:    closeAt,
		Aktivitas: AktivitasInput{
			Jenis:   "catatan",
			Teks:    "Ditutup",
			Tanggal: tanggal(2026, 11, 19),
		},
	}); err != nil {
		t.Fatal(err)
	}

	active, err := s.List(ctx, u.ID, Filter{Selesai: false}, now)
	if err != nil || !slices.Equal(idItem(active), []string{bill.ID}) {
		t.Errorf("List(Selesai=false) = %v, %v; want [%s]", idItem(active), err, bill.ID)
	}
	doneList, err := s.List(ctx, u.ID, Filter{Selesai: true}, now)
	if err != nil || !slices.Equal(idItem(doneList), []string{reg.ID, once.ID}) {
		t.Errorf("List(Selesai=true) = %v, %v; want [%s %s]", idItem(doneList), err, reg.ID, once.ID)
	}
	sum, err := s.Summary(ctx, u.ID, now)
	if err != nil || sum.Total != 1 || sum.Aman != 1 {
		t.Errorf("Summary with closed items = %+v, %v; want Total=1, Aman=1", sum, err)
	}
	dueCheck, err := s.DueForCheck(ctx, now.Add(time.Hour))
	if err != nil || len(dueCheck) != 0 {
		t.Errorf("DueForCheck with closed registry domain = %+v, %v; want empty", dueCheck, err)
	}
}

func TestItemTugasAktivitasSGDRoundTrip(t *testing.T) {
	t.Parallel()
	s := storeSiap(t)
	ctx := t.Context()
	u := penggunaUji(t, s, "Rana")
	k := klienUji(t, s, u.ID, "Klien Singapura")
	now := time.Date(2026, 11, 19, 7, 0, 0, 0, time.UTC)
	due1 := tanggal(2026, 11, 25)
	due2 := tanggal(2026, 12, 25)

	bill, err := s.Create(ctx, u.ID, Input{
		Nama:        "DigitalOcean SG",
		Jenis:       "tagihan",
		KlienID:     &k.ID,
		Kedaluwarsa: due1,
		Sumber:      "manual",
		Biaya:       ptr[int64](2050),
		MataUang:    "SGD",
		UlangJumlah: 1,
		UlangSatuan: "bulan",
	})
	if err != nil {
		t.Fatalf("Create SGD item: %v", err)
	}
	if bill.MataUang != "SGD" || bill.Biaya == nil || *bill.Biaya != 2050 {
		t.Fatalf("created SGD item = %+v", bill)
	}

	got, err := s.Get(ctx, u.ID, bill.ID)
	if err != nil || got.MataUang != "SGD" || got.Biaya == nil || *got.Biaya != 2050 {
		t.Fatalf("Get SGD item = %+v, %v", got, err)
	}

	clients, err := s.ListKlien(ctx, u.ID, now)
	if err != nil || len(clients) != 1 || clients[0].Biaya30Hari != 0 || clients[0].Biaya30HariPer["SGD"] != 2050 {
		t.Fatalf("ListKlien SGD = %+v, %v", clients, err)
	}

	moved, err := s.PerpanjangItem(ctx, u.ID, bill.ID, Perpanjangan{
		Dari:   due1,
		Sampai: &due2,
		At:     now,
		Aktivitas: AktivitasInput{
			KlienID:  &k.ID,
			Jenis:    "tagihan_dibayar",
			Teks:     "Tagihan DigitalOcean SG dibayar S$20.50",
			Tanggal:  tanggal(2026, 11, 19),
			Jumlah:   ptr[int64](2050),
			MataUang: ptr("SGD"),
		},
	})
	if err != nil || !moved.Kedaluwarsa.Equal(due2) || moved.MataUang != "SGD" {
		t.Fatalf("PerpanjangItem SGD = %+v, %v", moved, err)
	}

	feed, err := s.ListAktivitas(ctx, u.ID, AktivitasFilter{})
	if err != nil || len(feed) != 1 || feed[0].MataUang == nil || *feed[0].MataUang != "SGD" ||
		feed[0].Jumlah == nil || *feed[0].Jumlah != 2050 {
		t.Fatalf("ListAktivitas SGD = %+v, %v", feed, err)
	}

	tg, err := s.CreateTugas(ctx, u.ID, TugasInput{
		KlienID:    &k.ID,
		Judul:      "Audit server SG",
		JatuhTempo: due1,
		Biaya:      ptr[int64](1500),
		MataUang:   "SGD",
	})
	if err != nil || tg.MataUang != "SGD" || tg.Biaya == nil || *tg.Biaya != 1500 {
		t.Fatalf("CreateTugas SGD = %+v, %v", tg, err)
	}
}
