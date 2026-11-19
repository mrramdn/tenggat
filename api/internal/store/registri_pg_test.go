package store

import (
	"errors"
	"testing"
	"time"
)

func TestSaveRegistryCheck(t *testing.T) {
	t.Parallel()
	s := storeSiap(t)
	ctx := t.Context()
	u := penggunaUji(t, s, "Rana")
	k := klienUji(t, s, u.ID, "Toko Kue Mawar")

	dom, err := s.Create(ctx, u.ID, Input{Nama: "tokokuemawar.id", Jenis: "domain", KlienID: &k.ID,
		Host: ptr("tokokuemawar.id"), Kedaluwarsa: tanggal(2026, 12, 1), Sumber: "registri", Catatan: "Perpanjang via PANDI"})
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 11, 19, 1, 0, 0, 0, time.UTC)
	if _, err := s.SaveCheckError(ctx, dom.ID, "RDAP 503", at); err != nil {
		t.Fatal(err)
	}

	info := RegistryInfo{Domain: "tokokuemawar.id", Registrar: "PT Registrar Nusantara", Server: "https://rdap.pandi.id/rdap/"}
	later := at.Add(2 * time.Hour)
	got, err := s.SaveRegistryCheck(ctx, dom.ID, tanggal(2027, 3, 2), info, later)
	if err != nil {
		t.Fatalf("SaveRegistryCheck: %v", err)
	}
	if !got.Kedaluwarsa.Equal(tanggal(2027, 3, 2)) || got.InfoRegistri == nil || *got.InfoRegistri != info ||
		!sama(got.TerakhirDicek, &later) || got.GalatCek != nil || got.Klien != k.Nama || got.Catatan != "Perpanjang via PANDI" {
		t.Errorf("SaveRegistryCheck = %+v", got)
	}
	if again, err := s.Get(ctx, u.ID, dom.ID); err != nil || jsonOf(t, again) != jsonOf(t, got) {
		t.Errorf("Get after SaveRegistryCheck = %+v, %v", again, err)
	}

	// Only registry-sourced items accept a registry check.
	manualDom, err := s.Create(ctx, u.ID, Input{Nama: "manual.id", Jenis: "domain", Host: ptr("manual.id"),
		Kedaluwarsa: tanggal(2027, 1, 1), Sumber: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	cert, err := s.Create(ctx, u.ID, Input{Nama: "Sertifikat", Jenis: "sertifikat", Host: ptr("contoh.id"),
		Kedaluwarsa: tanggal(2027, 1, 1), Sumber: "server"})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{manualDom.ID, cert.ID, "00000000-0000-4000-8000-000000000999"} {
		if _, err := s.SaveRegistryCheck(ctx, id, tanggal(2030, 1, 1), info, later); !errors.Is(err, ErrNotFound) {
			t.Errorf("SaveRegistryCheck(%s) err = %v, want ErrNotFound", id, err)
		}
	}
	if d, _ := s.Get(ctx, u.ID, manualDom.ID); !d.Kedaluwarsa.Equal(tanggal(2027, 1, 1)) || d.InfoRegistri != nil {
		t.Errorf("SaveRegistryCheck modified a manual domain: %+v", d)
	}
}

func TestCalendarToken(t *testing.T) {
	t.Parallel()
	s := storeSiap(t)
	ctx := t.Context()
	u := penggunaUji(t, s, "Rana")
	other := penggunaUji(t, s, "Budi")

	if active, err := s.CalendarActive(ctx, u.ID); err != nil || active {
		t.Errorf("CalendarActive(initial) = %v, %v; want false, nil", active, err)
	}
	if _, err := s.UserByCalendarToken(ctx, "kalender-belum-ada"); !errors.Is(err, ErrNotFound) {
		t.Errorf("UserByCalendarToken(unknown) err = %v, want ErrNotFound", err)
	}

	if err := s.SetCalendarToken(ctx, u.ID, "hash-kalender-1"); err != nil {
		t.Fatalf("SetCalendarToken: %v", err)
	}
	if active, err := s.CalendarActive(ctx, u.ID); err != nil || !active {
		t.Errorf("CalendarActive(after set) = %v, %v; want true, nil", active, err)
	}
	if got, err := s.UserByCalendarToken(ctx, "hash-kalender-1"); err != nil || got != u {
		t.Errorf("UserByCalendarToken = %+v, %v; want %+v", got, err, u)
	}

	// Replacing the token invalidates the old link.
	if err := s.SetCalendarToken(ctx, u.ID, "hash-kalender-2"); err != nil {
		t.Fatalf("SetCalendarToken(replace): %v", err)
	}
	if _, err := s.UserByCalendarToken(ctx, "hash-kalender-1"); !errors.Is(err, ErrNotFound) {
		t.Errorf("UserByCalendarToken(old) err = %v, want ErrNotFound", err)
	}
	if got, err := s.UserByCalendarToken(ctx, "hash-kalender-2"); err != nil || got != u {
		t.Errorf("UserByCalendarToken(new) = %+v, %v", got, err)
	}

	// Two users cannot share the same calendar token hash.
	if err := s.SetCalendarToken(ctx, other.ID, "hash-kalender-2"); err == nil {
		t.Errorf("SetCalendarToken(duplicate hash) succeeded")
	} else if code, _ := kodeSQL(err); code != "23505" {
		t.Errorf("SetCalendarToken(duplicate hash) SQLSTATE = %q, want 23505", code)
	}

	// DeleteCalendarToken disables the link.
	if err := s.DeleteCalendarToken(ctx, u.ID); err != nil {
		t.Fatalf("DeleteCalendarToken: %v", err)
	}
	if active, err := s.CalendarActive(ctx, u.ID); err != nil || active {
		t.Errorf("CalendarActive(after delete) = %v, %v; want false, nil", active, err)
	}
	if _, err := s.UserByCalendarToken(ctx, "hash-kalender-2"); !errors.Is(err, ErrNotFound) {
		t.Errorf("UserByCalendarToken(after delete) err = %v, want ErrNotFound", err)
	}

	unknown := "00000000-0000-4000-8000-000000000999"
	if err := s.SetCalendarToken(ctx, unknown, "hash-x"); !errors.Is(err, ErrNotFound) {
		t.Errorf("SetCalendarToken(unknown) err = %v, want ErrNotFound", err)
	}
	if _, err := s.CalendarActive(ctx, unknown); !errors.Is(err, ErrNotFound) {
		t.Errorf("CalendarActive(unknown) err = %v, want ErrNotFound", err)
	}
	// DeleteCalendarToken does not check RowsAffected and returns nil for an unknown user ID.
	if err := s.DeleteCalendarToken(ctx, unknown); err != nil {
		t.Errorf("DeleteCalendarToken(unknown) = %v, want nil", err)
	}
}
