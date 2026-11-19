package store

import (
	"errors"
	"testing"
	"time"

	"github.com/mrramdn/tenggat/api/internal/expiry"
)

func TestKlienRoundTrip(t *testing.T) {
	t.Parallel()
	s := storeSiap(t)
	ctx := t.Context()
	u := penggunaUji(t, s, "Rana")

	// Empty status defaults to aktif.
	k, err := s.CreateKlien(ctx, u.ID, "Toko Kue Mawar", "Pemilik: Bu Sari", "")
	if err != nil {
		t.Fatalf("CreateKlien: %v", err)
	}
	if k.ID == "" || k.PenggunaID != u.ID || k.Nama != "Toko Kue Mawar" || k.Catatan != "Pemilik: Bu Sari" ||
		k.Status != "aktif" || k.LaporanAktif || k.Dibuat.IsZero() {
		t.Errorf("CreateKlien = %+v", k)
	}
	got, err := s.GetKlien(ctx, u.ID, k.ID)
	if err != nil || got != k {
		t.Errorf("GetKlien = %+v, %v; want %+v", got, err, k)
	}

	// Explicit status values.
	jeda, err := s.CreateKlien(ctx, u.ID, "Kopi Senja", "", "jeda")
	if err != nil || jeda.Status != "jeda" {
		t.Fatalf("CreateKlien(jeda) = %+v, %v", jeda, err)
	}
	selesai, err := s.CreateKlien(ctx, u.ID, "Studio Lama", "", "selesai")
	if err != nil || selesai.Status != "selesai" {
		t.Fatalf("CreateKlien(selesai) = %+v, %v", selesai, err)
	}

	// Attach an item, a task, an activity entry, and a report note to k.
	it, err := s.Create(ctx, u.ID, Input{Nama: "tokokuemawar.id", Jenis: "domain", KlienID: &k.ID,
		Host: ptr("tokokuemawar.id"), Kedaluwarsa: tanggal(2027, 1, 1), Sumber: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	tg, err := s.CreateTugas(ctx, u.ID, TugasInput{KlienID: &k.ID, Judul: "Backup bulanan",
		UlangJumlah: 1, UlangSatuan: "bulan", JatuhTempo: tanggal(2026, 12, 1)})
	if err != nil {
		t.Fatal(err)
	}
	ak, err := s.AddAktivitas(ctx, u.ID, AktivitasInput{KlienID: &k.ID, Jenis: "catatan",
		Teks: "Kontrak diperpanjang", Tanggal: tanggal(2026, 11, 19)})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetCatatanLaporan(ctx, u.ID, k.ID, tanggal(2026, 11, 1), "Semua lancar."); err != nil {
		t.Fatal(err)
	}

	// UpdateKlien changes name, note, and status; an empty status keeps the current one.
	upd, err := s.UpdateKlien(ctx, u.ID, k.ID, "Mawar Bakery", "Catatan baru", "jeda")
	if err != nil {
		t.Fatalf("UpdateKlien: %v", err)
	}
	if upd.Nama != "Mawar Bakery" || upd.Catatan != "Catatan baru" || upd.Status != "jeda" || !upd.Dibuat.Equal(k.Dibuat) {
		t.Errorf("UpdateKlien = %+v", upd)
	}
	upd2, err := s.UpdateKlien(ctx, u.ID, k.ID, "Mawar Bakery & Cafe", "", "")
	if err != nil || upd2.Nama != "Mawar Bakery & Cafe" || upd2.Catatan != "" || upd2.Status != "jeda" {
		t.Errorf("UpdateKlien(empty status) = %+v, %v", upd2, err)
	}
	if _, err := s.UpdateKlien(ctx, u.ID, "00000000-0000-4000-8000-000000000999", "Baru", "", "aktif"); !errors.Is(err, ErrNotFound) {
		t.Errorf("UpdateKlien(unknown) err = %v, want ErrNotFound", err)
	}

	// The new client name is visible immediately on joined items, tasks, and activity entries.
	if gotIt, err := s.Get(ctx, u.ID, it.ID); err != nil || gotIt.Klien != "Mawar Bakery & Cafe" {
		t.Errorf("Item.Klien after rename = %q, %v", gotIt.Klien, err)
	}
	if gotTg, err := s.GetTugas(ctx, u.ID, tg.ID); err != nil || gotTg.Klien != "Mawar Bakery & Cafe" {
		t.Errorf("Tugas.Klien after rename = %q, %v", gotTg.Klien, err)
	}
	if feed, err := s.ListAktivitas(ctx, u.ID, AktivitasFilter{KlienID: k.ID}); err != nil || len(feed) != 1 ||
		feed[0].ID != ak.ID || feed[0].Klien != "Mawar Bakery & Cafe" {
		t.Errorf("Aktivitas.Klien after rename = %+v, %v", feed, err)
	}

	// DeleteKlien drops the client and its report notes, while its assets, tasks, and activity entries stay without a client.
	if err := s.DeleteKlien(ctx, u.ID, k.ID); err != nil {
		t.Fatalf("DeleteKlien: %v", err)
	}
	if _, err := s.GetKlien(ctx, u.ID, k.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetKlien after DeleteKlien err = %v, want ErrNotFound", err)
	}
	if err := s.DeleteKlien(ctx, u.ID, k.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("second DeleteKlien err = %v, want ErrNotFound", err)
	}
	if gotIt, err := s.Get(ctx, u.ID, it.ID); err != nil || gotIt.KlienID != nil || gotIt.Klien != "" {
		t.Errorf("Item after DeleteKlien = %+v, %v", gotIt, err)
	}
	if gotTg, err := s.GetTugas(ctx, u.ID, tg.ID); err != nil || gotTg.KlienID != nil || gotTg.Klien != "" {
		t.Errorf("Tugas after DeleteKlien = %+v, %v", gotTg, err)
	}
	if feed, err := s.ListAktivitas(ctx, u.ID, AktivitasFilter{}); err != nil || len(feed) != 1 ||
		feed[0].KlienID != nil || feed[0].Klien != "" {
		t.Errorf("Aktivitas after DeleteKlien = %+v, %v", feed, err)
	}
	if n := hitung(t, s, `SELECT count(*) FROM catatan_laporan WHERE klien_id = $1`, k.ID); n != 0 {
		t.Errorf("catatan_laporan has %d rows after DeleteKlien, want 0", n)
	}
}

func TestKlienTakenAndScoping(t *testing.T) {
	t.Parallel()
	s := storeSiap(t)
	ctx := t.Context()
	a := penggunaUji(t, s, "Rana")
	b := penggunaUji(t, s, "Budi")

	mawar, err := s.CreateKlien(ctx, a.ID, "Toko Kue Mawar", "", "aktif")
	if err != nil {
		t.Fatal(err)
	}
	senja, err := s.CreateKlien(ctx, a.ID, "Kopi Senja", "", "aktif")
	if err != nil {
		t.Fatal(err)
	}

	// Duplicate names for the same user fail regardless of letter case.
	for _, dup := range []string{"Toko Kue Mawar", "toko kue mawar", "TOKO KUE MAWAR"} {
		if _, err := s.CreateKlien(ctx, a.ID, dup, "", "aktif"); !errors.Is(err, ErrKlienTaken) {
			t.Errorf("CreateKlien(%q) err = %v, want ErrKlienTaken", dup, err)
		}
		if _, err := s.UpdateKlien(ctx, a.ID, senja.ID, dup, "", "aktif"); !errors.Is(err, ErrKlienTaken) {
			t.Errorf("UpdateKlien(%q) err = %v, want ErrKlienTaken", dup, err)
		}
	}
	// Updating a client to a different casing of its own name is allowed.
	if got, err := s.UpdateKlien(ctx, a.ID, mawar.ID, "toko kue Mawar", "Catatan", ""); err != nil || got.Nama != "toko kue Mawar" {
		t.Errorf("UpdateKlien(same client case change) = %+v, %v", got, err)
	}

	// Another user cannot read, edit, or delete Rana's client.
	if _, err := s.GetKlien(ctx, b.ID, mawar.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetKlien by another user err = %v, want ErrNotFound", err)
	}
	if _, err := s.UpdateKlien(ctx, b.ID, mawar.ID, "Dibajak", "", "selesai"); !errors.Is(err, ErrNotFound) {
		t.Errorf("UpdateKlien by another user err = %v, want ErrNotFound", err)
	}
	if err := s.DeleteKlien(ctx, b.ID, mawar.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("DeleteKlien by another user err = %v, want ErrNotFound", err)
	}
	if list, err := s.ListKlien(ctx, b.ID, time.Now()); err != nil || list == nil || len(list) != 0 {
		t.Errorf("ListKlien(empty user) = %v, %v; want an empty, non-nil slice", list, err)
	}

	// Another user may use the same client name in their own account.
	if kBudi, err := s.CreateKlien(ctx, b.ID, "Toko Kue Mawar", "", "aktif"); err != nil || kBudi.PenggunaID != b.ID {
		t.Errorf("CreateKlien(same name, different user) = %+v, %v", kBudi, err)
	}
}

func TestListKlienAggregates(t *testing.T) {
	t.Parallel()
	s := storeSiap(t)
	ctx := t.Context()
	u := penggunaUji(t, s, "Rana")
	other := penggunaUji(t, s, "Budi")
	now := time.Date(2026, 11, 19, 7, 0, 0, 0, time.UTC) // 2026-11-19 in Jakarta
	today := expiry.DateOf(now)

	beta := klienUji(t, s, u.ID, "beta")
	alfa := klienUji(t, s, u.ID, "Alfa")
	kosong := klienUji(t, s, u.ID, "Kosong")
	otherKlien := klienUji(t, s, other.ID, "Klien Budi")

	if err := s.SetReportToken(ctx, u.ID, alfa.ID, "hash-laporan-alfa"); err != nil {
		t.Fatal(err)
	}

	addItem := func(userID string, klienID *string, nama string, days int, biaya *int64) {
		t.Helper()
		_, err := s.Create(ctx, userID, Input{Nama: nama, Jenis: "lainnya", KlienID: klienID,
			Kedaluwarsa: today.AddDate(0, 0, days), Sumber: "manual", Biaya: biaya})
		if err != nil {
			t.Fatal(err)
		}
	}
	// Alfa: overdue (-5), today (0), +30 days (counted), +31 days (excluded from Biaya30Hari), and one without cost.
	addItem(u.ID, &alfa.ID, "Alfa lewat", -5, ptr[int64](100000))
	addItem(u.ID, &alfa.ID, "Alfa hari ini", 0, ptr[int64](200000))
	addItem(u.ID, &alfa.ID, "Alfa 30 hari", 30, ptr[int64](50000))
	addItem(u.ID, &alfa.ID, "Alfa 31 hari", 31, ptr[int64](999999))
	addItem(u.ID, &alfa.ID, "Alfa gratis", 10, nil)

	// beta: only an asset 45 days out, so Biaya30Hari is 0.
	addItem(u.ID, &beta.ID, "Beta jauh", 45, ptr[int64](500000))
	// Unassigned asset and another user's asset must not affect Rana's clients.
	addItem(u.ID, nil, "Tanpa klien", 1, ptr[int64](700000))
	addItem(other.ID, &otherKlien.ID, "Aset Budi", 1, ptr[int64](800000))

	got, err := s.ListKlien(ctx, u.ID, now)
	if err != nil {
		t.Fatalf("ListKlien: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("ListKlien returned %d rows, want 3: %+v", len(got), got)
	}

	// Sorted by lower(nama), nama: Alfa, beta, Kosong.
	wantEarliestAlfa := today.AddDate(0, 0, -5)
	if got[0].ID != alfa.ID || !got[0].LaporanAktif || got[0].JumlahAset != 5 ||
		!sama(got[0].Terdekat, &wantEarliestAlfa) || got[0].Biaya30Hari != 350000 {
		t.Errorf("ListKlien[0] (Alfa) = %+v", got[0])
	}
	wantEarliestBeta := today.AddDate(0, 0, 45)
	if got[1].ID != beta.ID || got[1].LaporanAktif || got[1].JumlahAset != 1 ||
		!sama(got[1].Terdekat, &wantEarliestBeta) || got[1].Biaya30Hari != 0 {
		t.Errorf("ListKlien[1] (beta) = %+v", got[1])
	}
	if got[2].ID != kosong.ID || got[2].LaporanAktif || got[2].JumlahAset != 0 ||
		got[2].Terdekat != nil || got[2].Biaya30Hari != 0 {
		t.Errorf("ListKlien[2] (Kosong) = %+v", got[2])
	}
}

func TestEnsureKlien(t *testing.T) {
	t.Parallel()
	s := storeSiap(t)
	ctx := t.Context()
	a := penggunaUji(t, s, "Rana")
	b := penggunaUji(t, s, "Budi")

	first, err := s.EnsureKlien(ctx, a.ID, "Toko Kue Mawar")
	if err != nil {
		t.Fatalf("EnsureKlien(new): %v", err)
	}
	if first.ID == "" || first.Nama != "Toko Kue Mawar" || first.Status != "aktif" {
		t.Errorf("EnsureKlien(new) = %+v", first)
	}
	if _, err := s.UpdateKlien(ctx, a.ID, first.ID, first.Nama, "Catatan penting", "jeda"); err != nil {
		t.Fatal(err)
	}

	// A second EnsureKlien with a different case returns the existing row unchanged.
	again, err := s.EnsureKlien(ctx, a.ID, "toko KUE mawar")
	if err != nil {
		t.Fatalf("EnsureKlien(existing): %v", err)
	}
	if again.ID != first.ID || again.Nama != "Toko Kue Mawar" || again.Catatan != "Catatan penting" || again.Status != "jeda" {
		t.Errorf("EnsureKlien(existing) = %+v, want original row %s", again, first.ID)
	}

	// Another user gets their own client row.
	other, err := s.EnsureKlien(ctx, b.ID, "Toko Kue Mawar")
	if err != nil || other.ID == first.ID || other.PenggunaID != b.ID {
		t.Errorf("EnsureKlien(other user) = %+v, %v", other, err)
	}
}

func TestKlienConstraintsAndCrossOwnerFK(t *testing.T) {
	t.Parallel()
	s := storeSiap(t)
	ctx := t.Context()
	a := penggunaUji(t, s, "Rana")
	b := penggunaUji(t, s, "Budi")

	_, err := s.CreateKlien(ctx, a.ID, "", "", "aktif")
	mustCheckViolation(t, err, "klien_nama_check")
	_, err = s.CreateKlien(ctx, a.ID, "Arsip", "", "arsip")
	mustCheckViolation(t, err, "klien_status_check")

	// The database foreign keys on item.klien_id, tugas.klien_id, and aktivitas.klien_id
	// reference klien(id) without checking pengguna_id (handlers enforce ownership).
	budiKlien := klienUji(t, s, b.ID, "Toko Budi")
	it, err := s.Create(ctx, a.ID, Input{Nama: "Aset silang", Jenis: "lainnya", KlienID: &budiKlien.ID,
		Kedaluwarsa: tanggal(2026, 11, 20), Sumber: "manual", Biaya: ptr[int64](12345)})
	if err != nil {
		t.Fatalf("Create with another user's klien_id failed unexpectedly: %v", err)
	}
	if it.Klien != "Toko Budi" {
		t.Errorf("Item.Klien = %q, want Toko Budi from the joined client row", it.Klien)
	}
	// ListKlien joins item on i.klien_id = k.id without filtering i.pengguna_id.
	budiList, err := s.ListKlien(ctx, b.ID, tanggal(2026, 11, 19))
	if err != nil || len(budiList) != 1 || budiList[0].JumlahAset != 1 || budiList[0].Biaya30Hari != 12345 {
		t.Errorf("ListKlien(budi) with cross-owner item = %+v, %v", budiList, err)
	}
}
