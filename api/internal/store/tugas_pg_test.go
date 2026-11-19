package store

import (
	"errors"
	"slices"
	"testing"
	"time"
)

func idTugas(list []Tugas) []string {
	out := make([]string, len(list))
	for i, t := range list {
		out[i] = t.ID
	}
	return out
}

func TestTugasRoundTrip(t *testing.T) {
	t.Parallel()
	s := storeSiap(t)
	ctx := t.Context()
	u := penggunaUji(t, s, "Rana")
	mawar := klienUji(t, s, u.ID, "Toko Kue Mawar")
	senja := klienUji(t, s, u.ID, "Kopi Senja")

	in := TugasInput{KlienID: &mawar.ID, Judul: "Backup database bulanan", Catatan: "Simpan ke S3",
		UlangJumlah: 1, UlangSatuan: "bulan", JatuhTempo: tanggal(2026, 12, 1), Biaya: ptr[int64](50000)}
	tg, err := s.CreateTugas(ctx, u.ID, in)
	if err != nil {
		t.Fatalf("CreateTugas: %v", err)
	}
	if tg.ID == "" || tg.PenggunaID != u.ID || tg.KlienID == nil || *tg.KlienID != mawar.ID || tg.Klien != mawar.Nama ||
		tg.Judul != in.Judul || tg.Catatan != in.Catatan || tg.UlangJumlah != 1 || tg.UlangSatuan != "bulan" ||
		!tg.JatuhTempo.Equal(tanggal(2026, 12, 1)) || tg.Biaya == nil || *tg.Biaya != 50000 ||
		tg.TerakhirSelesai != nil || tg.Dibuat.IsZero() || tg.Diubah.IsZero() {
		t.Errorf("CreateTugas = %+v", tg)
	}
	got, err := s.GetTugas(ctx, u.ID, tg.ID)
	if err != nil || jsonOf(t, got) != jsonOf(t, tg) {
		t.Errorf("GetTugas = %+v, %v; want %+v", got, err, tg)
	}

	// All repeat units work, with or without a client.
	for _, satuan := range []string{"hari", "minggu", "tahun"} {
		other, err := s.CreateTugas(ctx, u.ID, TugasInput{Judul: "Tugas " + satuan, UlangJumlah: 2,
			UlangSatuan: satuan, JatuhTempo: tanggal(2027, 1, 1)})
		if err != nil || other.UlangSatuan != satuan || other.KlienID != nil || other.Klien != "" || other.Biaya != nil {
			t.Errorf("CreateTugas(%s) = %+v, %v", satuan, other, err)
		}
	}

	// UpdateTugas replaces the editable fields and refreshes the joined client name.
	updIn := TugasInput{KlienID: &senja.ID, Judul: "Backup mingguan", Catatan: "",
		UlangJumlah: 2, UlangSatuan: "minggu", JatuhTempo: tanggal(2026, 12, 15), Biaya: ptr[int64](0)}
	upd, err := s.UpdateTugas(ctx, u.ID, tg.ID, updIn)
	if err != nil {
		t.Fatalf("UpdateTugas: %v", err)
	}
	if upd.KlienID == nil || *upd.KlienID != senja.ID || upd.Klien != senja.Nama || upd.Judul != "Backup mingguan" ||
		upd.Catatan != "" || upd.UlangJumlah != 2 || upd.UlangSatuan != "minggu" ||
		!upd.JatuhTempo.Equal(tanggal(2026, 12, 15)) || upd.Biaya == nil || *upd.Biaya != 0 ||
		!upd.Dibuat.Equal(tg.Dibuat) || upd.Diubah.Before(tg.Diubah) {
		t.Errorf("UpdateTugas = %+v", upd)
	}
	if _, err := s.UpdateTugas(ctx, u.ID, "00000000-0000-4000-8000-000000000999", updIn); !errors.Is(err, ErrNotFound) {
		t.Errorf("UpdateTugas(unknown) err = %v, want ErrNotFound", err)
	}

	// Deleting a task sets tugas_id to NULL on its activity entries.
	ak, err := s.AddAktivitas(ctx, u.ID, AktivitasInput{KlienID: &senja.ID, TugasID: &tg.ID,
		Jenis: "tugas_baru", Teks: "Tugas baru: Backup mingguan", Tanggal: tanggal(2026, 11, 19)})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteTugas(ctx, u.ID, tg.ID); err != nil {
		t.Fatalf("DeleteTugas: %v", err)
	}
	if _, err := s.GetTugas(ctx, u.ID, tg.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetTugas after DeleteTugas err = %v, want ErrNotFound", err)
	}
	if err := s.DeleteTugas(ctx, u.ID, tg.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("second DeleteTugas err = %v, want ErrNotFound", err)
	}
	if feed, err := s.ListAktivitas(ctx, u.ID, AktivitasFilter{}); err != nil || len(feed) != 1 ||
		feed[0].ID != ak.ID || feed[0].TugasID != nil {
		t.Errorf("Aktivitas after DeleteTugas = %+v, %v", feed, err)
	}
}

func TestCompleteTugas(t *testing.T) {
	t.Parallel()
	s := storeSiap(t)
	ctx := t.Context()
	u := penggunaUji(t, s, "Rana")
	other := penggunaUji(t, s, "Budi")
	k := klienUji(t, s, u.ID, "Toko Kue Mawar")

	from := tanggal(2026, 11, 19)
	next := tanggal(2026, 12, 19)
	at := time.Date(2026, 11, 19, 8, 30, 0, 0, time.UTC)

	tg, err := s.CreateTugas(ctx, u.ID, TugasInput{KlienID: &k.ID, Judul: "Backup database",
		UlangJumlah: 1, UlangSatuan: "bulan", JatuhTempo: from})
	if err != nil {
		t.Fatal(err)
	}

	// Wrong from date or another user is rejected before moving the task.
	if _, err := s.CompleteTugas(ctx, u.ID, tg.ID, tanggal(2026, 11, 18), next, at); !errors.Is(err, ErrNotFound) {
		t.Errorf("CompleteTugas(wrong from) err = %v, want ErrNotFound", err)
	}
	if _, err := s.CompleteTugas(ctx, other.ID, tg.ID, from, next, at); !errors.Is(err, ErrNotFound) {
		t.Errorf("CompleteTugas(other user) err = %v, want ErrNotFound", err)
	}

	// Matching from date moves the due date once and records the completion timestamp.
	done, err := s.CompleteTugas(ctx, u.ID, tg.ID, from, next, at)
	if err != nil {
		t.Fatalf("CompleteTugas: %v", err)
	}
	if !done.JatuhTempo.Equal(next) || !sama(done.TerakhirSelesai, &at) || done.Klien != k.Nama {
		t.Errorf("CompleteTugas = %+v", done)
	}

	// A second click with the original from date must not move the task a second time.
	later := at.Add(time.Minute)
	if _, err := s.CompleteTugas(ctx, u.ID, tg.ID, from, tanggal(2027, 1, 19), later); !errors.Is(err, ErrNotFound) {
		t.Errorf("second CompleteTugas(old from) err = %v, want ErrNotFound", err)
	}
	if got, err := s.GetTugas(ctx, u.ID, tg.ID); err != nil || !got.JatuhTempo.Equal(next) || !sama(got.TerakhirSelesai, &at) {
		t.Errorf("GetTugas after duplicate CompleteTugas = %+v, %v", got, err)
	}

	// UpdateTugas preserves TerakhirSelesai.
	upd, err := s.UpdateTugas(ctx, u.ID, tg.ID, TugasInput{KlienID: &k.ID, Judul: "Backup database & aset",
		UlangJumlah: 1, UlangSatuan: "bulan", JatuhTempo: next})
	if err != nil || !sama(upd.TerakhirSelesai, &at) {
		t.Errorf("UpdateTugas after CompleteTugas = %+v, %v; want TerakhirSelesai preserved", upd, err)
	}
}

func TestTugasListAndScoping(t *testing.T) {
	t.Parallel()
	s := storeSiap(t)
	ctx := t.Context()
	u := penggunaUji(t, s, "Rana")
	other := penggunaUji(t, s, "Budi")
	mawar := klienUji(t, s, u.ID, "Toko Kue Mawar")
	senja := klienUji(t, s, u.ID, "Kopi Senja")

	mk := func(userID string, klienID *string, judul string, due time.Time) Tugas {
		t.Helper()
		tg, err := s.CreateTugas(ctx, userID, TugasInput{KlienID: klienID, Judul: judul,
			UlangJumlah: 1, UlangSatuan: "bulan", JatuhTempo: due})
		if err != nil {
			t.Fatal(err)
		}
		return tg
	}
	t1 := mk(u.ID, &mawar.ID, "Backup lama", tanggal(2026, 11, 10))
	t2 := mk(u.ID, &mawar.ID, "Cek SSL B", tanggal(2026, 11, 20))
	t3 := mk(u.ID, &senja.ID, "Cek SSL A", tanggal(2026, 11, 20)) // same date: ordered by title
	t4 := mk(u.ID, nil, "Audit tahunan", tanggal(2026, 12, 1))
	otherTugas := mk(other.ID, nil, "Tugas Budi", tanggal(2026, 11, 15))

	cutoff := tanggal(2026, 11, 20)
	cases := []struct {
		name string
		f    TugasFilter
		want []Tugas
	}{
		{"all, ordered by due date then title", TugasFilter{}, []Tugas{t1, t3, t2, t4}},
		{"by client", TugasFilter{KlienID: mawar.ID}, []Tugas{t1, t2}},
		{"due on or before cutoff", TugasFilter{Sampai: &cutoff}, []Tugas{t1, t3, t2}},
		{"by client and cutoff", TugasFilter{KlienID: mawar.ID, Sampai: &cutoff}, []Tugas{t1, t2}},
		{"cutoff before any task", TugasFilter{Sampai: ptr(tanggal(2026, 11, 1))}, nil},
	}
	for _, c := range cases {
		got, err := s.ListTugas(ctx, u.ID, c.f)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if got == nil {
			t.Errorf("%s: ListTugas returned nil slice, want non-nil", c.name)
		}
		if !slices.Equal(idTugas(got), idTugas(c.want)) {
			t.Errorf("%s: got %v, want %v", c.name, idTugas(got), idTugas(c.want))
		}
	}

	// Scoping: Rana cannot touch Budi's task.
	if _, err := s.GetTugas(ctx, u.ID, otherTugas.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetTugas(other user) err = %v, want ErrNotFound", err)
	}
	if _, err := s.UpdateTugas(ctx, u.ID, otherTugas.ID, TugasInput{Judul: "Ambil", UlangJumlah: 1,
		UlangSatuan: "hari", JatuhTempo: cutoff}); !errors.Is(err, ErrNotFound) {
		t.Errorf("UpdateTugas(other user) err = %v, want ErrNotFound", err)
	}
	if err := s.DeleteTugas(ctx, u.ID, otherTugas.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("DeleteTugas(other user) err = %v, want ErrNotFound", err)
	}
}

func TestTugasConstraints(t *testing.T) {
	t.Parallel()
	s := storeSiap(t)
	ctx := t.Context()
	u := penggunaUji(t, s, "Rana")
	due := tanggal(2026, 12, 1)

	_, err := s.CreateTugas(ctx, u.ID, TugasInput{Judul: "x", UlangJumlah: 1, UlangSatuan: "bulan", JatuhTempo: due})
	mustCheckViolation(t, err, "tugas_judul_check")
	_, err = s.CreateTugas(ctx, u.ID, TugasInput{Judul: "Backup", UlangJumlah: -1, UlangSatuan: "bulan", JatuhTempo: due})
	mustCheckViolation(t, err, "tugas_ulang_jumlah_check")
	_, err = s.CreateTugas(ctx, u.ID, TugasInput{Judul: "Backup", UlangJumlah: 366, UlangSatuan: "hari", JatuhTempo: due})
	mustCheckViolation(t, err, "tugas_ulang_jumlah_check")
	_, err = s.CreateTugas(ctx, u.ID, TugasInput{Judul: "Backup", UlangJumlah: 0, UlangSatuan: "bulan", JatuhTempo: due})
	mustCheckViolation(t, err, "tugas_ulang_lengkap")
	_, err = s.CreateTugas(ctx, u.ID, TugasInput{Judul: "Backup", UlangJumlah: 1, UlangSatuan: "", JatuhTempo: due})
	mustCheckViolation(t, err, "tugas_ulang_lengkap")
	_, err = s.CreateTugas(ctx, u.ID, TugasInput{Judul: "Backup", UlangJumlah: 1, UlangSatuan: "jam", JatuhTempo: due})
	mustCheckViolation(t, err, "tugas_ulang_satuan_check")
	_, err = s.CreateTugas(ctx, u.ID, TugasInput{Judul: "Backup", UlangJumlah: 1, UlangSatuan: "bulan",
		JatuhTempo: due, Biaya: ptr[int64](-100)})
	mustCheckViolation(t, err, "tugas_biaya_check")
	_, err = s.CreateTugas(ctx, u.ID, TugasInput{Judul: "Backup", UlangJumlah: 1, UlangSatuan: "bulan",
		JatuhTempo: due, MataUang: "EUR"})
	mustCheckViolation(t, err, "tugas_mata_uang_check")

	_, err = s.CreateTugas(ctx, u.ID, TugasInput{KlienID: ptr("00000000-0000-4000-8000-000000000999"),
		Judul: "Backup", UlangJumlah: 1, UlangSatuan: "bulan", JatuhTempo: due})
	if code, _ := kodeSQL(err); code != "23503" {
		t.Errorf("CreateTugas(unknown klien_id) err = %v, want foreign_key_violation", err)
	}
	if n := hitung(t, s, `SELECT count(*) FROM tugas`); n != 0 {
		t.Errorf("%d rows written by refused task creates", n)
	}
}

func TestOneOffTugasCloseAndFilter(t *testing.T) {
	t.Parallel()
	s := storeSiap(t)
	ctx := t.Context()
	u := penggunaUji(t, s, "Rana")
	other := penggunaUji(t, s, "Budi")
	k := klienUji(t, s, u.ID, "Toko Kue Mawar")

	due1 := tanggal(2026, 11, 20)
	due2 := tanggal(2026, 11, 25)
	at1 := time.Date(2026, 11, 19, 8, 0, 0, 0, time.UTC)
	at2 := time.Date(2026, 11, 19, 9, 0, 0, 0, time.UTC)

	// One-off task with USD cost.
	once1, err := s.CreateTugas(ctx, u.ID, TugasInput{
		KlienID:    &k.ID,
		Judul:      "Migrasi server produksi",
		Catatan:    "Pindah ke region Singapura",
		JatuhTempo: due1,
		Biaya:      ptr[int64](5000),
		MataUang:   "USD",
	})
	if err != nil {
		t.Fatalf("CreateTugas(one-off): %v", err)
	}
	if once1.UlangJumlah != 0 || once1.UlangSatuan != "" || once1.MataUang != "USD" || once1.SelesaiPada != nil {
		t.Errorf("CreateTugas(one-off) = %+v", once1)
	}
	if n := hitung(t, s, `SELECT count(*) FROM tugas WHERE id = $1 AND ulang_jumlah IS NULL AND ulang_satuan IS NULL`, once1.ID); n != 1 {
		t.Errorf("one-off task recurrence not stored as NULL in SQL")
	}

	once2, err := s.CreateTugas(ctx, u.ID, TugasInput{
		Judul:      "Audit keamanan",
		JatuhTempo: due2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if once2.MataUang != "IDR" {
		t.Errorf("default MataUang = %q, want IDR", once2.MataUang)
	}

	recurring, err := s.CreateTugas(ctx, u.ID, TugasInput{
		Judul:       "Backup mingguan",
		UlangJumlah: 1,
		UlangSatuan: "minggu",
		JatuhTempo:  due1,
	})
	if err != nil {
		t.Fatal(err)
	}

	// CloseTugas guards: recurring task, wrong from date, other user, unknown id all return ErrNotFound.
	if _, err := s.CloseTugas(ctx, u.ID, recurring.ID, due1, at1); !errors.Is(err, ErrNotFound) {
		t.Errorf("CloseTugas(recurring) err = %v, want ErrNotFound", err)
	}
	if _, err := s.CloseTugas(ctx, u.ID, once1.ID, due2, at1); !errors.Is(err, ErrNotFound) {
		t.Errorf("CloseTugas(wrong from) err = %v, want ErrNotFound", err)
	}
	if _, err := s.CloseTugas(ctx, other.ID, once1.ID, due1, at1); !errors.Is(err, ErrNotFound) {
		t.Errorf("CloseTugas(other user) err = %v, want ErrNotFound", err)
	}

	// Close once1 at at1, then once2 at at2 (later).
	closed1, err := s.CloseTugas(ctx, u.ID, once1.ID, due1, at1)
	if err != nil {
		t.Fatalf("CloseTugas(once1): %v", err)
	}
	if !sama(closed1.SelesaiPada, &at1) || !sama(closed1.TerakhirSelesai, &at1) || closed1.Klien != k.Nama {
		t.Errorf("CloseTugas(once1) = %+v", closed1)
	}
	if _, err := s.CloseTugas(ctx, u.ID, once2.ID, due2, at2); err != nil {
		t.Fatalf("CloseTugas(once2): %v", err)
	}

	// Closed task cannot be closed again, completed, or updated.
	if _, err := s.CloseTugas(ctx, u.ID, once1.ID, due1, at2); !errors.Is(err, ErrNotFound) {
		t.Errorf("second CloseTugas err = %v, want ErrNotFound", err)
	}
	if _, err := s.CompleteTugas(ctx, u.ID, once1.ID, due1, due2, at2); !errors.Is(err, ErrNotFound) {
		t.Errorf("CompleteTugas(closed) err = %v, want ErrNotFound", err)
	}
	if _, err := s.UpdateTugas(ctx, u.ID, once1.ID, TugasInput{Judul: "Ubah", JatuhTempo: due2}); !errors.Is(err, ErrNotFound) {
		t.Errorf("UpdateTugas(closed) err = %v, want ErrNotFound", err)
	}

	// ListTugas with Selesai=false returns only active tasks; Selesai=true returns closed tasks ordered by selesai_pada DESC.
	active, err := s.ListTugas(ctx, u.ID, TugasFilter{Selesai: false})
	if err != nil || !slices.Equal(idTugas(active), []string{recurring.ID}) {
		t.Errorf("ListTugas(Selesai=false) = %v, %v; want [%s]", idTugas(active), err, recurring.ID)
	}
	closedList, err := s.ListTugas(ctx, u.ID, TugasFilter{Selesai: true})
	if err != nil || !slices.Equal(idTugas(closedList), []string{once2.ID, once1.ID}) {
		t.Errorf("ListTugas(Selesai=true) = %v, %v; want [%s %s] (newest selesai_pada first)", idTugas(closedList), err, once2.ID, once1.ID)
	}
}
