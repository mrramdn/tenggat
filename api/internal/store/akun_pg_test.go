package store

import (
	"errors"
	"os"
	"testing"
	"time"
)

func TestOpenPingClose(t *testing.T) {
	t.Parallel()
	if os.Getenv("TENGGAT_UJI_PG") != "1" {
		t.Skip("set TENGGAT_UJI_PG=1 and the PG* variables to run against PostgreSQL")
	}
	ctx := t.Context()
	s, err := Open(ctx)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()
	if err := s.Ping(ctx); err != nil {
		t.Errorf("Ping: %v", err)
	}
}

func TestUserRoundTripAndEmailTaken(t *testing.T) {
	t.Parallel()
	s := storeSiap(t)
	ctx := t.Context()

	u, err := s.CreateUser(ctx, "Rana Pratiwi", "rana@studiorana.co.id", "$argon2id$v=19$hash")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if u.ID == "" || u.Nama != "Rana Pratiwi" || u.Email != "rana@studiorana.co.id" ||
		u.HashSandi != "$argon2id$v=19$hash" || u.Dibuat.IsZero() {
		t.Errorf("CreateUser = %+v", u)
	}

	got, err := s.UserByEmail(ctx, "rana@studiorana.co.id")
	if err != nil || got != u {
		t.Errorf("UserByEmail = %+v, %v; want %+v", got, err, u)
	}
	if _, err := s.UserByEmail(ctx, "tidak.ada@contoh.id"); !errors.Is(err, ErrNotFound) {
		t.Errorf("UserByEmail(unknown) err = %v, want ErrNotFound", err)
	}

	// Duplicate email returns ErrEmailTaken.
	if _, err := s.CreateUser(ctx, "Rana Lain", "rana@studiorana.co.id", "$argon2id$lain"); !errors.Is(err, ErrEmailTaken) {
		t.Errorf("CreateUser(duplicate email) err = %v, want ErrEmailTaken", err)
	}

	// Schema constraints on nama and email.
	_, err = s.CreateUser(ctx, "Rana", "Rana@Contoh.id", "x")
	mustCheckViolation(t, err, "pengguna_email_check")
	_, err = s.CreateUser(ctx, "Rana", "a@", "x")
	mustCheckViolation(t, err, "pengguna_email_check")
	_, err = s.CreateUser(ctx, "R", "r@contoh.id", "x")
	mustCheckViolation(t, err, "pengguna_nama_check")
}

func TestSessionsAndExpiry(t *testing.T) {
	t.Parallel()
	s := storeSiap(t)
	ctx := t.Context()
	u := penggunaUji(t, s, "Rana")
	now := time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC)

	if err := s.CreateSession(ctx, "hash-aktif", u.ID, now.Add(time.Hour)); err != nil {
		t.Fatalf("CreateSession(aktif): %v", err)
	}
	if err := s.CreateSession(ctx, "hash-aktif-2", u.ID, now.Add(24*time.Hour)); err != nil {
		t.Fatalf("CreateSession(aktif-2): %v", err)
	}
	if err := s.CreateSession(ctx, "hash-tepat", u.ID, now); err != nil {
		t.Fatalf("CreateSession(tepat): %v", err)
	}
	if err := s.CreateSession(ctx, "hash-lewat", u.ID, now.Add(-time.Minute)); err != nil {
		t.Fatalf("CreateSession(lewat): %v", err)
	}

	got, err := s.UserBySession(ctx, "hash-aktif", now)
	if err != nil || got != u {
		t.Errorf("UserBySession(aktif) = %+v, %v; want %+v", got, err, u)
	}
	// At or past kedaluwarsa, UserBySession reports ErrNotFound.
	for _, h := range []string{"hash-tepat", "hash-lewat", "hash-tidak-ada"} {
		if _, err := s.UserBySession(ctx, h, now); !errors.Is(err, ErrNotFound) {
			t.Errorf("UserBySession(%q) err = %v, want ErrNotFound", h, err)
		}
	}

	// DeleteSession ends only the targeted session; deleting an unknown token does not fail.
	if err := s.DeleteSession(ctx, "hash-aktif-2"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	if err := s.DeleteSession(ctx, "hash-tidak-ada"); err != nil {
		t.Errorf("DeleteSession(unknown): %v", err)
	}
	if _, err := s.UserBySession(ctx, "hash-aktif-2", now); !errors.Is(err, ErrNotFound) {
		t.Errorf("UserBySession after DeleteSession err = %v, want ErrNotFound", err)
	}
	if _, err := s.UserBySession(ctx, "hash-aktif", now); err != nil {
		t.Errorf("DeleteSession affected another session: %v", err)
	}

	// DeleteExpiredSessions removes sessions with kedaluwarsa <= now (both hash-tepat and hash-lewat).
	deleted, err := s.DeleteExpiredSessions(ctx, now)
	if err != nil || deleted != 2 {
		t.Errorf("DeleteExpiredSessions = %d, %v; want 2", deleted, err)
	}
	if deleted, err := s.DeleteExpiredSessions(ctx, now); err != nil || deleted != 0 {
		t.Errorf("second DeleteExpiredSessions = %d, %v; want 0", deleted, err)
	}
	if _, err := s.UserBySession(ctx, "hash-aktif", now); err != nil {
		t.Errorf("active session removed by DeleteExpiredSessions: %v", err)
	}
}

func TestDeleteUserCascade(t *testing.T) {
	t.Parallel()
	s := storeSiap(t)
	ctx := t.Context()
	a := penggunaUji(t, s, "Rana")
	b := penggunaUji(t, s, "Budi")
	now := time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC)

	populate := func(u Pengguna, suffix string) {
		t.Helper()
		if err := s.CreateSession(ctx, "sesi-"+suffix, u.ID, now.Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
		k := klienUji(t, s, u.ID, "Klien "+suffix)
		if err := s.SetCatatanLaporan(ctx, u.ID, k.ID, tanggal(2026, 11, 1), "Laporan "+suffix); err != nil {
			t.Fatal(err)
		}
		it, err := s.Create(ctx, u.ID, Input{Nama: "Aset " + suffix, Jenis: "lainnya", KlienID: &k.ID,
			Kedaluwarsa: tanggal(2027, 1, 1), Sumber: "manual"})
		if err != nil {
			t.Fatal(err)
		}
		tg, err := s.CreateTugas(ctx, u.ID, TugasInput{KlienID: &k.ID, Judul: "Tugas " + suffix,
			UlangJumlah: 1, UlangSatuan: "bulan", JatuhTempo: tanggal(2026, 12, 1)})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.AddAktivitas(ctx, u.ID, AktivitasInput{KlienID: &k.ID, ItemID: &it.ID, TugasID: &tg.ID,
			Jenis: "pekerjaan", Teks: "Kerja " + suffix, Tanggal: tanggal(2026, 11, 19), DurasiMenit: ptr(30)}); err != nil {
			t.Fatal(err)
		}
	}
	populate(a, "A")
	populate(b, "B")

	if err := s.DeleteUser(ctx, a.ID); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	if err := s.DeleteUser(ctx, a.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("second DeleteUser err = %v, want ErrNotFound", err)
	}
	if _, err := s.UserByEmail(ctx, a.Email); !errors.Is(err, ErrNotFound) {
		t.Errorf("UserByEmail after DeleteUser err = %v, want ErrNotFound", err)
	}

	// Every table now holds only Budi's 1 row.
	for _, table := range []string{"pengguna", "sesi", "klien", "catatan_laporan", "item", "tugas", "aktivitas"} {
		if got := hitung(t, s, `SELECT count(*) FROM `+table); got != 1 {
			t.Errorf("%s has %d rows after DeleteUser(a), want 1 (Budi's)", table, got)
		}
	}
	if _, err := s.UserBySession(ctx, "sesi-B", now); err != nil {
		t.Errorf("Budi's session lost after deleting Rana: %v", err)
	}
}
