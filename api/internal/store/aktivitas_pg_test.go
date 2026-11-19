package store

import (
	"encoding/json"
	"errors"
	"slices"
	"testing"
	"time"
)

func idAktivitas(list []Aktivitas) []string {
	out := make([]string, len(list))
	for i, a := range list {
		out[i] = a.ID
	}
	return out
}

func TestAktivitasRoundTripAndDelete(t *testing.T) {
	t.Parallel()
	s := storeSiap(t)
	ctx := t.Context()
	u := penggunaUji(t, s, "Rana")
	other := penggunaUji(t, s, "Budi")
	k := klienUji(t, s, u.ID, "Toko Kue Mawar")
	it := itemUji(t, s, u.ID, "tokokuemawar.id", "domain", tanggal(2027, 3, 2))

	catatan, err := s.AddAktivitas(ctx, u.ID, AktivitasInput{KlienID: &k.ID, Jenis: "catatan",
		Teks: "Bu Sari minta laporan tiap awal bulan", Tanggal: tanggal(2026, 11, 2)})
	if err != nil {
		t.Fatalf("AddAktivitas(catatan): %v", err)
	}
	if catatan.ID == "" || catatan.PenggunaID != u.ID || catatan.KlienID == nil || *catatan.KlienID != k.ID ||
		catatan.Klien != k.Nama || catatan.ItemID != nil || catatan.TugasID != nil || catatan.Jenis != "catatan" ||
		!catatan.Tanggal.Equal(tanggal(2026, 11, 2)) || catatan.DurasiMenit != nil || catatan.Data != nil ||
		catatan.Dibuat.IsZero() {
		t.Errorf("AddAktivitas(catatan) = %+v", catatan)
	}

	kerja, err := s.AddAktivitas(ctx, u.ID, AktivitasInput{KlienID: &k.ID, Jenis: "pekerjaan",
		Teks: "Optimasi gambar katalog", Tanggal: tanggal(2026, 11, 3), DurasiMenit: ptr(90)})
	if err != nil {
		t.Fatalf("AddAktivitas(pekerjaan): %v", err)
	}
	if kerja.DurasiMenit == nil || *kerja.DurasiMenit != 90 {
		t.Errorf("AddAktivitas(pekerjaan) = %+v", kerja)
	}

	// Manual entries (catatan and pekerjaan) can be deleted by their owner, but not by another user.
	if err := s.DeleteAktivitas(ctx, other.ID, catatan.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("DeleteAktivitas(other user) err = %v, want ErrNotFound", err)
	}
	if err := s.DeleteAktivitas(ctx, u.ID, catatan.ID); err != nil {
		t.Fatalf("DeleteAktivitas(catatan): %v", err)
	}
	if err := s.DeleteAktivitas(ctx, u.ID, catatan.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("second DeleteAktivitas(catatan) err = %v, want ErrNotFound", err)
	}
	if err := s.DeleteAktivitas(ctx, u.ID, kerja.ID); err != nil {
		t.Fatalf("DeleteAktivitas(pekerjaan): %v", err)
	}

	// Automatic entries cannot be deleted by the user (ErrAktivitasOtomatis), and another user gets ErrNotFound.
	otomatisKinds := []string{"aset_baru", "aset_diperpanjang", "aset_dihapus", "cek_diperbarui", "cek_gagal",
		"tugas_baru", "tugas_selesai", "tugas_dihapus", "laporan_dibagikan"}
	var firstAuto Aktivitas
	for i, jenis := range otomatisKinds {
		a, err := s.AddAktivitas(ctx, u.ID, AktivitasInput{KlienID: &k.ID, ItemID: &it.ID, Jenis: jenis,
			Teks: "Otomatis " + jenis, Tanggal: tanggal(2026, 11, 10),
			Data: json.RawMessage(`{"jenis":"` + jenis + `"}`)})
		if err != nil {
			t.Fatalf("AddAktivitas(%s): %v", jenis, err)
		}
		if i == 0 {
			firstAuto = a
		}
		if err := s.DeleteAktivitas(ctx, u.ID, a.ID); !errors.Is(err, ErrAktivitasOtomatis) {
			t.Errorf("DeleteAktivitas(%s) err = %v, want ErrAktivitasOtomatis", jenis, err)
		}
		if err := s.DeleteAktivitas(ctx, other.ID, a.ID); !errors.Is(err, ErrNotFound) {
			t.Errorf("DeleteAktivitas(%s by other user) err = %v, want ErrNotFound", jenis, err)
		}
	}

	// Deleting the referenced item sets item_id to NULL on the activity entries.
	if err := s.Delete(ctx, u.ID, it.ID); err != nil {
		t.Fatal(err)
	}
	feed, err := s.ListAktivitas(ctx, u.ID, AktivitasFilter{Batas: 20})
	if err != nil || len(feed) != len(otomatisKinds) {
		t.Fatalf("ListAktivitas = %d entries, %v; want %d", len(feed), err, len(otomatisKinds))
	}
	for _, a := range feed {
		if a.ItemID != nil {
			t.Errorf("entry %s still has ItemID %v after item deletion", a.ID, *a.ItemID)
		}
		if a.ID == firstAuto.ID && jsonOf(t, a.Data) != `{"jenis":"aset_baru"}` {
			t.Errorf("entry Data = %s", string(a.Data))
		}
	}
}

func TestAddAktivitasSekaliSehari(t *testing.T) {
	t.Parallel()
	s := storeSiap(t)
	ctx := t.Context()
	u := penggunaUji(t, s, "Rana")
	itA := itemUji(t, s, u.ID, "Sertifikat A", "lainnya", tanggal(2027, 1, 1))
	itB := itemUji(t, s, u.ID, "Sertifikat B", "lainnya", tanggal(2027, 1, 1))
	day1 := tanggal(2026, 11, 19)
	day2 := tanggal(2026, 11, 20)

	in := AktivitasInput{ItemID: &itA.ID, Jenis: "cek_gagal", Teks: "Cek gagal: timeout",
		Tanggal: day1, SekaliSehari: true}
	first, err := s.AddAktivitas(ctx, u.ID, in)
	if err != nil || first.ID == "" {
		t.Fatalf("first AddAktivitas(SekaliSehari): %+v, %v", first, err)
	}

	// Same item, same kind, same date with SekaliSehari=true is skipped with ErrNotFound.
	if _, err := s.AddAktivitas(ctx, u.ID, in); !errors.Is(err, ErrNotFound) {
		t.Errorf("duplicate AddAktivitas(SekaliSehari) err = %v, want ErrNotFound", err)
	}

	// Next day, another kind on the same day, or another item on the same day all succeed.
	nextDay := in
	nextDay.Tanggal = day2
	if _, err := s.AddAktivitas(ctx, u.ID, nextDay); err != nil {
		t.Errorf("AddAktivitas(next day): %v", err)
	}
	otherKind := in
	otherKind.Jenis = "cek_diperbarui"
	if _, err := s.AddAktivitas(ctx, u.ID, otherKind); err != nil {
		t.Errorf("AddAktivitas(other kind): %v", err)
	}
	otherItem := in
	otherItem.ItemID = &itB.ID
	if _, err := s.AddAktivitas(ctx, u.ID, otherItem); err != nil {
		t.Errorf("AddAktivitas(other item): %v", err)
	}

	// With SekaliSehari=false, a second entry on the same (item, kind, date) is stored.
	notLimited := in
	notLimited.SekaliSehari = false
	if _, err := s.AddAktivitas(ctx, u.ID, notLimited); err != nil {
		t.Errorf("AddAktivitas(SekaliSehari=false): %v", err)
	}

	// When ItemID is nil, SQL's "item_id = NULL" is unknown, so SekaliSehari does not deduplicate.
	noItem := AktivitasInput{Jenis: "catatan", Teks: "Catatan tanpa item", Tanggal: day1, SekaliSehari: true}
	if _, err := s.AddAktivitas(ctx, u.ID, noItem); err != nil {
		t.Fatalf("first AddAktivitas(nil ItemID): %v", err)
	}
	if _, err := s.AddAktivitas(ctx, u.ID, noItem); err != nil {
		t.Errorf("second AddAktivitas(nil ItemID, SekaliSehari=true) err = %v; SQL item_id = NULL does not match", err)
	}
}

func TestListAktivitasFiltersAndPagination(t *testing.T) {
	t.Parallel()
	s := storeSiap(t)
	ctx := t.Context()
	u := penggunaUji(t, s, "Rana")
	other := penggunaUji(t, s, "Budi")
	mawar := klienUji(t, s, u.ID, "Toko Kue Mawar")
	senja := klienUji(t, s, u.ID, "Kopi Senja")

	base := time.Date(2026, 11, 10, 8, 0, 0, 0, time.UTC)
	insertAt := func(id, userID string, klienID *string, jenis, teks string, tgl, dibuat time.Time) Aktivitas {
		t.Helper()
		exec(t, s, `INSERT INTO aktivitas (id, pengguna_id, klien_id, jenis, teks, tanggal, dibuat)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`, id, userID, klienID, jenis, teks, tgl, dibuat)
		return Aktivitas{ID: id}
	}

	a1 := insertAt("10000000-0000-4000-8000-000000000001", u.ID, &mawar.ID, "catatan", "Catatan 1",
		tanggal(2026, 11, 1), base)
	// a2 and a3 share the exact same dibuat timestamp to test the (dibuat, id) DESC tie-breaker and cursor.
	sameTime := base.Add(time.Hour)
	a2 := insertAt("10000000-0000-4000-8000-000000000002", u.ID, &mawar.ID, "pekerjaan", "Kerja 2",
		tanggal(2026, 11, 5), sameTime)
	a3 := insertAt("10000000-0000-4000-8000-000000000003", u.ID, &senja.ID, "pekerjaan", "Kerja 3",
		tanggal(2026, 11, 10), sameTime)
	a4 := insertAt("10000000-0000-4000-8000-000000000004", u.ID, &senja.ID, "aset_baru", "Aset 4",
		tanggal(2026, 11, 15), base.Add(2*time.Hour))
	a5 := insertAt("10000000-0000-4000-8000-000000000005", u.ID, nil, "tugas_selesai", "Tugas 5",
		tanggal(2026, 11, 20), base.Add(3*time.Hour))
	otherEntry := insertAt("10000000-0000-4000-8000-000000000099", other.ID, nil, "catatan", "Punya Budi",
		tanggal(2026, 11, 12), base.Add(4*time.Hour))

	beforeA4 := base.Add(2 * time.Hour)
	cases := []struct {
		name string
		f    AktivitasFilter
		want []Aktivitas
	}{
		{"all, newest first", AktivitasFilter{}, []Aktivitas{a5, a4, a3, a2, a1}},
		{"by client", AktivitasFilter{KlienID: mawar.ID}, []Aktivitas{a2, a1}},
		{"by kinds", AktivitasFilter{Jenis: []string{"catatan", "aset_baru"}}, []Aktivitas{a4, a1}},
		{"tanggal range Dari..Sampai inclusive", AktivitasFilter{
			Dari: ptr(tanggal(2026, 11, 5)), Sampai: ptr(tanggal(2026, 11, 15)),
		}, []Aktivitas{a4, a3, a2}},
		{"SebelumWaktu strict <", AktivitasFilter{SebelumWaktu: &beforeA4}, []Aktivitas{a3, a2, a1}},
		{"SebelumID cursor from a4", AktivitasFilter{SebelumID: a4.ID}, []Aktivitas{a3, a2, a1}},
		{"SebelumID cursor across equal dibuat (from a3)", AktivitasFilter{SebelumID: a3.ID}, []Aktivitas{a2, a1}},
		{"SebelumID of another user's entry returns empty", AktivitasFilter{SebelumID: otherEntry.ID}, nil},
		{"Batas limit", AktivitasFilter{Batas: 2}, []Aktivitas{a5, a4}},
	}
	for _, c := range cases {
		got, err := s.ListAktivitas(ctx, u.ID, c.f)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if got == nil {
			t.Errorf("%s: ListAktivitas returned nil slice, want non-nil", c.name)
		}
		if !slices.Equal(idAktivitas(got), idAktivitas(c.want)) {
			t.Errorf("%s: got %v, want %v", c.name, idAktivitas(got), idAktivitas(c.want))
		}
	}

	// Default Batas (0) caps the result at 50 entries.
	extra := penggunaUji(t, s, "Banyak")
	for i := range 55 {
		if _, err := s.AddAktivitas(ctx, extra.ID, AktivitasInput{Jenis: "catatan", Teks: "Catatan",
			Tanggal: tanggal(2026, 11, 1).AddDate(0, 0, i)}); err != nil {
			t.Fatal(err)
		}
	}
	if got, err := s.ListAktivitas(ctx, extra.ID, AktivitasFilter{}); err != nil || len(got) != 50 {
		t.Errorf("ListAktivitas(Batas=0 with 55 rows) = %d rows, %v; want 50", len(got), err)
	}
}

func TestAktivitasConstraints(t *testing.T) {
	t.Parallel()
	s := storeSiap(t)
	ctx := t.Context()
	u := penggunaUji(t, s, "Rana")
	tgl := tanggal(2026, 11, 19)

	_, err := s.AddAktivitas(ctx, u.ID, AktivitasInput{Jenis: "telepon", Teks: "Telepon klien", Tanggal: tgl})
	mustCheckViolation(t, err, "aktivitas_jenis_check")
	_, err = s.AddAktivitas(ctx, u.ID, AktivitasInput{Jenis: "catatan", Teks: "", Tanggal: tgl})
	mustCheckViolation(t, err, "aktivitas_teks_check")
	_, err = s.AddAktivitas(ctx, u.ID, AktivitasInput{Jenis: "pekerjaan", Teks: "Kerja", Tanggal: tgl, DurasiMenit: ptr(0)})
	mustCheckViolation(t, err, "aktivitas_durasi_menit_check")
	_, err = s.AddAktivitas(ctx, u.ID, AktivitasInput{Jenis: "pekerjaan", Teks: "Kerja", Tanggal: tgl, DurasiMenit: ptr(1441)})
	mustCheckViolation(t, err, "aktivitas_durasi_menit_check")
}
