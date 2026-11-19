package store

import (
	"errors"
	"testing"
)

func TestCatatanLaporan(t *testing.T) {
	t.Parallel()
	s := storeSiap(t)
	ctx := t.Context()
	u := penggunaUji(t, s, "Rana")
	other := penggunaUji(t, s, "Budi")
	k := klienUji(t, s, u.ID, "Toko Kue Mawar")
	okt := tanggal(2026, 10, 1)
	nov := tanggal(2026, 11, 1)

	// Missing note returns an empty string without error.
	if teks, err := s.CatatanLaporan(ctx, k.ID, okt); err != nil || teks != "" {
		t.Errorf("CatatanLaporan(empty) = %q, %v; want \"\", nil", teks, err)
	}

	if err := s.SetCatatanLaporan(ctx, u.ID, k.ID, okt, "Semua aset aman di Oktober."); err != nil {
		t.Fatalf("SetCatatanLaporan(okt): %v", err)
	}
	if err := s.SetCatatanLaporan(ctx, u.ID, k.ID, nov, "Catatan November."); err != nil {
		t.Fatalf("SetCatatanLaporan(nov): %v", err)
	}
	if teks, err := s.CatatanLaporan(ctx, k.ID, okt); err != nil || teks != "Semua aset aman di Oktober." {
		t.Errorf("CatatanLaporan(okt) = %q, %v", teks, err)
	}

	// Upsert replaces the month's text without touching other months.
	if err := s.SetCatatanLaporan(ctx, u.ID, k.ID, okt, "Revisi Oktober."); err != nil {
		t.Fatalf("SetCatatanLaporan(upsert): %v", err)
	}
	if teks, err := s.CatatanLaporan(ctx, k.ID, okt); err != nil || teks != "Revisi Oktober." {
		t.Errorf("CatatanLaporan(okt after upsert) = %q, %v", teks, err)
	}
	if teks, err := s.CatatanLaporan(ctx, k.ID, nov); err != nil || teks != "Catatan November." {
		t.Errorf("CatatanLaporan(nov) = %q, %v", teks, err)
	}

	// Another user or an unknown client gets ErrNotFound and leaves the note intact.
	if err := s.SetCatatanLaporan(ctx, other.ID, k.ID, okt, "Bajakan"); !errors.Is(err, ErrNotFound) {
		t.Errorf("SetCatatanLaporan(other user) err = %v, want ErrNotFound", err)
	}
	if err := s.SetCatatanLaporan(ctx, u.ID, "00000000-0000-4000-8000-000000000999", okt, "x"); !errors.Is(err, ErrNotFound) {
		t.Errorf("SetCatatanLaporan(unknown client) err = %v, want ErrNotFound", err)
	}
	if teks, err := s.CatatanLaporan(ctx, k.ID, okt); err != nil || teks != "Revisi Oktober." {
		t.Errorf("CatatanLaporan after rejected write = %q, %v", teks, err)
	}

	// Bulan must be the first day of the month.
	err := s.SetCatatanLaporan(ctx, u.ID, k.ID, tanggal(2026, 10, 15), "Pertengahan bulan")
	mustCheckViolation(t, err, "catatan_laporan_bulan_check")
}

func TestReportToken(t *testing.T) {
	t.Parallel()
	s := storeSiap(t)
	ctx := t.Context()
	u := penggunaUji(t, s, "Rana")
	other := penggunaUji(t, s, "Budi")
	mawar := klienUji(t, s, u.ID, "Toko Kue Mawar")
	senja := klienUji(t, s, u.ID, "Kopi Senja")

	if _, _, err := s.KlienByReportToken(ctx, "token-belum-ada"); !errors.Is(err, ErrNotFound) {
		t.Errorf("KlienByReportToken(unknown) err = %v, want ErrNotFound", err)
	}

	if err := s.SetReportToken(ctx, u.ID, mawar.ID, "hash-laporan-1"); err != nil {
		t.Fatalf("SetReportToken: %v", err)
	}
	if k, err := s.GetKlien(ctx, u.ID, mawar.ID); err != nil || !k.LaporanAktif {
		t.Errorf("GetKlien after SetReportToken = %+v, %v; want LaporanAktif=true", k, err)
	}
	gotK, gotU, err := s.KlienByReportToken(ctx, "hash-laporan-1")
	if err != nil || gotK.ID != mawar.ID || !gotK.LaporanAktif || gotU.ID != u.ID || gotU.Email != u.Email {
		t.Errorf("KlienByReportToken = %+v, %+v, %v", gotK, gotU, err)
	}

	// Replacing the token invalidates the previous link.
	if err := s.SetReportToken(ctx, u.ID, mawar.ID, "hash-laporan-2"); err != nil {
		t.Fatalf("SetReportToken(replace): %v", err)
	}
	if _, _, err := s.KlienByReportToken(ctx, "hash-laporan-1"); !errors.Is(err, ErrNotFound) {
		t.Errorf("KlienByReportToken(old token) err = %v, want ErrNotFound", err)
	}
	if gotK, _, err := s.KlienByReportToken(ctx, "hash-laporan-2"); err != nil || gotK.ID != mawar.ID {
		t.Errorf("KlienByReportToken(new token) = %+v, %v", gotK, err)
	}

	// Two clients cannot share the same report token hash.
	if err := s.SetReportToken(ctx, u.ID, senja.ID, "hash-laporan-2"); err == nil {
		t.Errorf("SetReportToken(duplicate hash) succeeded")
	} else if code, _ := kodeSQL(err); code != "23505" {
		t.Errorf("SetReportToken(duplicate hash) SQLSTATE = %q, want 23505", code)
	}

	// Another user or an unknown client cannot set or delete the report token.
	if err := s.SetReportToken(ctx, other.ID, mawar.ID, "hash-bajakan"); !errors.Is(err, ErrNotFound) {
		t.Errorf("SetReportToken(other user) err = %v, want ErrNotFound", err)
	}
	if err := s.DeleteReportToken(ctx, other.ID, mawar.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("DeleteReportToken(other user) err = %v, want ErrNotFound", err)
	}
	if err := s.SetReportToken(ctx, u.ID, "00000000-0000-4000-8000-000000000999", "hash-x"); !errors.Is(err, ErrNotFound) {
		t.Errorf("SetReportToken(unknown) err = %v, want ErrNotFound", err)
	}
	if err := s.DeleteReportToken(ctx, u.ID, "00000000-0000-4000-8000-000000000999"); !errors.Is(err, ErrNotFound) {
		t.Errorf("DeleteReportToken(unknown) err = %v, want ErrNotFound", err)
	}

	// DeleteReportToken turns the link off.
	if err := s.DeleteReportToken(ctx, u.ID, mawar.ID); err != nil {
		t.Fatalf("DeleteReportToken: %v", err)
	}
	if k, err := s.GetKlien(ctx, u.ID, mawar.ID); err != nil || k.LaporanAktif {
		t.Errorf("GetKlien after DeleteReportToken = %+v, %v; want LaporanAktif=false", k, err)
	}
	if _, _, err := s.KlienByReportToken(ctx, "hash-laporan-2"); !errors.Is(err, ErrNotFound) {
		t.Errorf("KlienByReportToken after DeleteReportToken err = %v, want ErrNotFound", err)
	}
}

func TestAwalLaporan(t *testing.T) {
	t.Parallel()
	s := storeSiap(t)
	ctx := t.Context()
	u := penggunaUji(t, s, "Rana")

	if _, err := s.AwalLaporan(ctx, "00000000-0000-4000-8000-000000000999"); !errors.Is(err, ErrNotFound) {
		t.Errorf("AwalLaporan(unknown) err = %v, want ErrNotFound", err)
	}

	// 2026-11-18 19:00 UTC is 2026-11-19 02:00 in Asia/Jakarta (WIB).
	k := klienUji(t, s, u.ID, "Toko Kue Mawar")
	exec(t, s, `UPDATE klien SET dibuat = '2026-11-18 19:00:00+00' WHERE id = $1`, k.ID)

	awal, err := s.AwalLaporan(ctx, k.ID)
	if err != nil || !awal.Equal(tanggal(2026, 11, 19)) {
		t.Errorf("AwalLaporan(no activity) = %v, %v; want 2026-11-19", awal, err)
	}

	// An earlier entry of a kind not shown on the report (e.g. catatan, aset_baru) is ignored.
	for _, jenis := range []string{"catatan", "aset_baru", "cek_gagal", "tugas_baru", "laporan_dibagikan"} {
		if _, err := s.AddAktivitas(ctx, u.ID, AktivitasInput{KlienID: &k.ID, Jenis: jenis,
			Teks: "Abaikan " + jenis, Tanggal: tanggal(2026, 9, 1)}); err != nil {
			t.Fatal(err)
		}
	}
	if awal, err = s.AwalLaporan(ctx, k.ID); err != nil || !awal.Equal(tanggal(2026, 11, 19)) {
		t.Errorf("AwalLaporan(with non-report activity) = %v, %v; want 2026-11-19", awal, err)
	}

	// A report-worthy entry after dibuat keeps dibuat.
	if _, err := s.AddAktivitas(ctx, u.ID, AktivitasInput{KlienID: &k.ID, Jenis: "pekerjaan",
		Teks: "Kerja baru", Tanggal: tanggal(2026, 11, 25), DurasiMenit: ptr(60)}); err != nil {
		t.Fatal(err)
	}
	if awal, err = s.AwalLaporan(ctx, k.ID); err != nil || !awal.Equal(tanggal(2026, 11, 19)) {
		t.Errorf("AwalLaporan(later work) = %v, %v; want 2026-11-19", awal, err)
	}

	// Backdated entries of the four report kinds push AwalLaporan back to the earliest one.
	for i, tc := range []struct {
		jenis string
		tgl   int
	}{
		{"cek_diperbarui", 15},
		{"aset_diperpanjang", 12},
		{"tugas_selesai", 10},
		{"pekerjaan", 5},
	} {
		want := tanggal(2026, 10, tc.tgl)
		if _, err := s.AddAktivitas(ctx, u.ID, AktivitasInput{KlienID: &k.ID, Jenis: tc.jenis,
			Teks: "Riwayat " + tc.jenis, Tanggal: want}); err != nil {
			t.Fatalf("step %d (%s): %v", i, tc.jenis, err)
		}
		if awal, err = s.AwalLaporan(ctx, k.ID); err != nil || !awal.Equal(want) {
			t.Errorf("AwalLaporan after %s = %v, %v; want %s", tc.jenis, awal, err, want.Format("2006-01-02"))
		}
	}
}
