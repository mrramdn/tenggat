package store

import (
	"encoding/json"
	"errors"
	"io/fs"
	"slices"
	"testing"
	"time"
)

// The fixture ids. Fixed ids keep the expectations readable; every test has its own schema.
const (
	fxRana = "0a000000-0000-4000-8000-000000000001"
	fxBudi = "0b000000-0000-4000-8000-000000000002"

	fxMawar   = "0c000000-0000-4000-8000-000000000001" // aktif, report shared
	fxSenja   = "0c000000-0000-4000-8000-000000000002" // jeda
	fxLama    = "0c000000-0000-4000-8000-000000000003" // selesai
	fxTokBudi = "0c000000-0000-4000-8000-000000000004" // Budi's

	fxSertifikat  = "0d000000-0000-4000-8000-000000000001" // sertifikat, server, info_tls
	fxRegistri    = "0d000000-0000-4000-8000-000000000002" // domain, registri, info_registri
	fxDomain      = "0d000000-0000-4000-8000-000000000003" // domain, manual
	fxToken       = "0d000000-0000-4000-8000-000000000004" // token
	fxLangganan   = "0d000000-0000-4000-8000-000000000005" // langganan
	fxLainnya     = "0d000000-0000-4000-8000-000000000006" // lainnya, no client
	fxSertGagal   = "0d000000-0000-4000-8000-000000000007" // sertifikat whose last check failed
	fxDomainBudi  = "0d000000-0000-4000-8000-000000000008" // Budi's
	fxTugasBackup = "0e000000-0000-4000-8000-000000000001"
	fxTugasPlugin = "0e000000-0000-4000-8000-000000000002"
	fxTugasBudi   = "0e000000-0000-4000-8000-000000000003"
)

const fxInfoTLS = `{"penerbit": "Let's Encrypt · R12", "subjek": "tokokuemawar.id", "alamat": "203.0.113.10",
	"berlaku_sejak": "2026-10-16T08:00:00Z", "berakhir": "2027-01-14T07:59:59Z", "rantai_valid": true}`

const fxInfoRegistri = `{"domain": "tokokuemawar.id", "registrar": "PT Registrar Nusantara", "server": "https://rdap.pandi.id/rdap/"}`

// isiDataRilis005 inserts rows shaped like a database the release with migration
// 005_ruang left behind. It uses that release's columns only, so later migrations
// run over it unchanged.
func isiDataRilis005(t *testing.T, s *Store) {
	t.Helper()
	exec(t, s, `INSERT INTO pengguna (id, nama, email, hash_sandi, dibuat, hash_token_kalender, webhook_url, webhook_terakhir) VALUES
		($1, 'Rana Pratiwi', 'rana@studiorana.co.id', '$argon2id$v=19$m=65536,t=3,p=2$c2FsdA$aGFzaA', '2026-01-05 03:00:00+00',
		 'kalender-hash-rana', 'https://hooks.slack.com/services/T000/B000/XXXX', '2026-11-18'),
		($2, 'Budi Santoso', 'budi@tokobudi.id', '$argon2id$v=19$m=65536,t=3,p=2$c2FsdA$aGFzaA', '2026-02-10 04:00:00+00',
		 NULL, NULL, NULL)`, fxRana, fxBudi)
	exec(t, s, `INSERT INTO sesi (hash_token, pengguna_id, dibuat, kedaluwarsa) VALUES
		('sesi-hash-rana', $1, '2026-11-01 00:00:00+00', '2099-01-01 00:00:00+00')`, fxRana)
	exec(t, s, `INSERT INTO klien (id, pengguna_id, nama, catatan, status, dibuat, hash_token_laporan) VALUES
		($1, $5, 'Toko Kue Mawar', 'Pemilik: Bu Sari', 'aktif', '2026-01-06 02:00:00+00', 'laporan-hash-mawar'),
		($2, $5, 'Kopi Senja', '', 'jeda', '2026-03-01 02:00:00+00', NULL),
		($3, $5, 'Studio Lama', '', 'selesai', '2026-01-07 02:00:00+00', NULL),
		($4, $6, 'Toko Budi', '', 'aktif', '2026-02-11 02:00:00+00', NULL)`,
		fxMawar, fxSenja, fxLama, fxTokBudi, fxRana, fxBudi)
	exec(t, s, `INSERT INTO item (id, pengguna_id, klien_id, nama, jenis, host, kedaluwarsa, sumber, biaya, catatan,
		info_tls, info_registri, terakhir_dicek, galat_cek) VALUES
		($1, $9, $11, 'Sertifikat tokokuemawar.id', 'sertifikat', 'tokokuemawar.id', '2027-01-14', 'server', NULL, '',
		 $14::jsonb, NULL, '2026-11-18 20:00:00+00', NULL),
		($2, $9, $11, 'tokokuemawar.id', 'domain', 'tokokuemawar.id', '2027-03-02', 'registri', 250000, 'Perpanjang via PANDI',
		 NULL, $15::jsonb, '2026-11-18 20:05:00+00', NULL),
		($3, $9, $12, 'kopisenja.com', 'domain', 'kopisenja.com', '2026-11-25', 'manual', 180000, '', NULL, NULL, NULL, NULL),
		($4, $9, $12, 'Token API Midtrans', 'token', NULL, '2026-11-10', 'manual', NULL, 'Ganti sebelum habis', NULL, NULL, NULL, NULL),
		($5, $9, $13, 'Hosting Niagahoster', 'langganan', NULL, '2026-12-15', 'manual', 1200000, '', NULL, NULL, NULL, NULL),
		($6, $9, NULL, 'Sewa ruang kerja', 'lainnya', NULL, '2027-06-30', 'manual', 9000000, '', NULL, NULL, NULL, NULL),
		($7, $9, $12, 'Sertifikat api.kopisenja.com', 'sertifikat', 'api.kopisenja.com', '2026-12-01', 'server', NULL, '',
		 NULL, NULL, '2026-11-18 20:10:00+00', 'koneksi ditolak'),
		($8, $10, NULL, 'tokobudi.id', 'domain', 'tokobudi.id', '2027-02-01', 'manual', NULL, '', NULL, NULL, NULL, NULL)`,
		fxSertifikat, fxRegistri, fxDomain, fxToken, fxLangganan, fxLainnya, fxSertGagal, fxDomainBudi,
		fxRana, fxBudi, fxMawar, fxSenja, fxLama, fxInfoTLS, fxInfoRegistri)
	exec(t, s, `UPDATE item SET klien_id = $2 WHERE id = $1`, fxDomainBudi, fxTokBudi)
	exec(t, s, `INSERT INTO tugas (id, pengguna_id, klien_id, judul, catatan, ulang_jumlah, ulang_satuan, jatuh_tempo,
		biaya, terakhir_selesai) VALUES
		($1, $4, $6, 'Backup database bulanan', 'Simpan ke S3', 1, 'bulan', '2026-12-01', 50000, '2026-11-01 03:00:00+00'),
		($2, $4, NULL, 'Update plugin WordPress', '', 2, 'minggu', '2026-11-20', NULL, NULL),
		($3, $5, $7, 'Cek backup toko', '', 1, 'tahun', '2027-01-01', NULL, NULL)`,
		fxTugasBackup, fxTugasPlugin, fxTugasBudi, fxRana, fxBudi, fxMawar, fxTokBudi)
	exec(t, s, `INSERT INTO aktivitas (pengguna_id, klien_id, item_id, tugas_id, jenis, teks, tanggal, durasi_menit, data, dibuat) VALUES
		($1, $2, NULL, NULL, 'catatan', 'Bu Sari minta laporan tiap awal bulan', '2026-11-02', NULL, NULL, '2026-11-02 03:00:00+00'),
		($1, $2, NULL, NULL, 'pekerjaan', 'Optimasi gambar katalog', '2026-11-03', 90, NULL, '2026-11-04 03:00:00+00'),
		($1, $2, $3, NULL, 'aset_baru', 'tokokuemawar.id ditambahkan, berakhir 2 Mar 2027', '2026-11-05', NULL,
		 '{"nama": "tokokuemawar.id", "jenis": "domain", "kedaluwarsa": "2027-03-02"}', '2026-11-05 03:00:00+00'),
		($1, $2, $3, NULL, 'aset_diperpanjang', 'tokokuemawar.id diperpanjang sampai 2 Mar 2027', '2026-11-06', NULL,
		 '{"dari": "2026-03-02", "sampai": "2027-03-02"}', '2026-11-06 03:00:00+00'),
		($1, $2, NULL, NULL, 'aset_dihapus', 'Domain lama dihapus', '2026-11-07', NULL,
		 '{"item_id": "0d000000-0000-4000-8000-0000000000ff", "nama": "Domain lama", "jenis": "domain"}', '2026-11-07 03:00:00+00'),
		($1, $2, $4, NULL, 'cek_diperbarui', 'Sertifikat tokokuemawar.id diperbarui, berlaku sampai 14 Jan 2027', '2026-11-08', NULL,
		 '{"dari": "2026-11-20", "sampai": "2027-01-14", "sumber": "server"}', '2026-11-08 03:00:00+00'),
		($1, $5, $6, NULL, 'cek_gagal', 'Cek Sertifikat api.kopisenja.com gagal. koneksi ditolak', '2026-11-09', NULL,
		 '{"galat": "koneksi ditolak"}', '2026-11-09 03:00:00+00'),
		($1, $2, NULL, $7, 'tugas_baru', 'Tugas baru: Backup database bulanan', '2026-11-10', NULL, NULL, '2026-11-10 03:00:00+00'),
		($1, $2, NULL, $7, 'tugas_selesai', 'Selesai: Backup database bulanan', '2026-11-11', NULL, NULL, '2026-11-11 03:00:00+00'),
		($1, $5, NULL, NULL, 'tugas_dihapus', 'Tugas dihapus: Cek uptime', '2026-11-12', NULL, NULL, '2026-11-12 03:00:00+00'),
		($1, $2, NULL, NULL, 'laporan_dibagikan', 'Tautan laporan dibagikan', '2026-11-13', NULL, NULL, '2026-11-13 03:00:00+00')`,
		fxRana, fxMawar, fxRegistri, fxSertifikat, fxSenja, fxSertGagal, fxTugasBackup)
	exec(t, s, `INSERT INTO catatan_laporan (klien_id, bulan, teks, diubah) VALUES
		($1, '2026-10-01', 'Bulan ini semua aset aman.', '2026-10-31 10:00:00+00')`, fxMawar)
}

var fxJumlah = map[string]int{"pengguna": 2, "sesi": 1, "klien": 4, "item": 8, "tugas": 3, "aktivitas": 11, "catatan_laporan": 1}

func cekJumlahTabel(t *testing.T, s *Store) {
	t.Helper()
	for table, want := range fxJumlah {
		if got := hitung(t, s, `SELECT count(*) FROM `+table); got != want {
			t.Errorf("%s has %d rows, want %d", table, got, want)
		}
	}
}

type versiMigrasi struct {
	Versi      string
	Dijalankan time.Time
}

func daftarMigrasi(t *testing.T, s *Store) []versiMigrasi {
	t.Helper()
	rows, err := s.pool.Query(t.Context(), `SELECT versi, dijalankan FROM schema_migrations ORDER BY versi`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []versiMigrasi
	for rows.Next() {
		var v versiMigrasi
		if err := rows.Scan(&v.Versi, &v.Dijalankan); err != nil {
			t.Fatal(err)
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func semuaVersi(t *testing.T) []string {
	t.Helper()
	names, err := fs.Glob(migrationFiles, "migrations/*.sql")
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(names)
	for i, n := range names {
		names[i] = migrationVersion(n)
	}
	return names
}

func TestMigrateUpTo(t *testing.T) {
	t.Parallel()
	s := storeUji(t)
	ctx := t.Context()

	if err := s.migrateUpTo(ctx, "002_akun"); err != nil {
		t.Fatalf("migrateUpTo(002_akun): %v", err)
	}
	var versi []string
	for _, v := range daftarMigrasi(t, s) {
		versi = append(versi, v.Versi)
	}
	if !slices.Equal(versi, []string{"001_item", "002_akun"}) {
		t.Fatalf("applied %v, want [001_item 002_akun]", versi)
	}
	if n := hitung(t, s, `SELECT count(*) FROM pg_class WHERE oid = to_regclass('klien')`); n != 0 {
		t.Errorf("klien exists after migrateUpTo(002_akun)")
	}

	if err := s.migrateUpTo(ctx, "999_tidak_ada"); err == nil {
		t.Errorf("migrateUpTo of an unknown version succeeded")
	}
	if n := len(daftarMigrasi(t, s)); n != 2 {
		t.Errorf("an unknown version applied migrations: %d now", n)
	}

	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	versi = versi[:0]
	for _, v := range daftarMigrasi(t, s) {
		versi = append(versi, v.Versi)
	}
	if want := semuaVersi(t); !slices.Equal(versi, want) {
		t.Errorf("applied %v, want %v", versi, want)
	}
}

// TestMigrateKeepsData builds a database as the release with 005_ruang left it,
// then runs every later migration over it: no row may be lost or become unreadable.
func TestMigrateKeepsData(t *testing.T) {
	t.Parallel()
	s := storeUji(t)
	ctx := t.Context()
	if err := s.migrateUpTo(ctx, "005_ruang"); err != nil {
		t.Fatalf("migrateUpTo(005_ruang): %v", err)
	}
	isiDataRilis005(t, s)
	cekJumlahTabel(t, s)

	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("Migrate over release-005 data: %v", err)
	}
	cekJumlahTabel(t, s)
	cekDataRilis005(t, s)

	before := daftarMigrasi(t, s)
	var versi []string
	for _, v := range before {
		versi = append(versi, v.Versi)
	}
	if want := semuaVersi(t); !slices.Equal(versi, want) {
		t.Errorf("applied %v, want %v", versi, want)
	}
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
	if after := daftarMigrasi(t, s); !slices.Equal(before, after) {
		t.Errorf("second Migrate changed schema_migrations:\n%v\n%v", before, after)
	}
	cekJumlahTabel(t, s)
	cekDataRilis005(t, s)
}

// cekDataRilis005 reads the release-005 fixture back through the Store.
func cekDataRilis005(t *testing.T, s *Store) {
	t.Helper()
	ctx := t.Context()
	now := time.Date(2026, 11, 19, 7, 0, 0, 0, time.UTC)

	// Accounts, sessions, calendar link, webhook.
	rana, err := s.UserByEmail(ctx, "rana@studiorana.co.id")
	if err != nil || rana.ID != fxRana || rana.Nama != "Rana Pratiwi" || rana.HashSandi == "" {
		t.Errorf("UserByEmail(rana) = %+v, %v", rana, err)
	}
	if u, err := s.UserByEmail(ctx, "budi@tokobudi.id"); err != nil || u.ID != fxBudi {
		t.Errorf("UserByEmail(budi) = %+v, %v", u, err)
	}
	if u, err := s.UserBySession(ctx, "sesi-hash-rana", now); err != nil || u.ID != fxRana {
		t.Errorf("UserBySession = %+v, %v", u, err)
	}
	if u, err := s.UserByCalendarToken(ctx, "kalender-hash-rana"); err != nil || u.ID != fxRana {
		t.Errorf("UserByCalendarToken = %+v, %v", u, err)
	}
	if on, err := s.CalendarActive(ctx, fxBudi); err != nil || on {
		t.Errorf("CalendarActive(budi) = %v, %v", on, err)
	}
	url, last, err := s.Webhook(ctx, fxRana)
	if err != nil || url == nil || *url != "https://hooks.slack.com/services/T000/B000/XXXX" ||
		!sama(last, ptr(tanggal(2026, 11, 18))) {
		t.Errorf("Webhook(rana) = %s, %v, %v", str(url), last, err)
	}

	// Clients and the shared report.
	klien, err := s.ListKlien(ctx, fxRana, now)
	if err != nil {
		t.Fatalf("ListKlien: %v", err)
	}
	status := map[string]string{}
	for _, k := range klien {
		status[k.Nama] = k.Status
		if k.LaporanAktif != (k.ID == fxMawar) {
			t.Errorf("%s: LaporanAktif = %v", k.Nama, k.LaporanAktif)
		}
	}
	if len(klien) != 3 || status["Toko Kue Mawar"] != "aktif" || status["Kopi Senja"] != "jeda" || status["Studio Lama"] != "selesai" {
		t.Errorf("ListKlien(rana) statuses = %v", status)
	}
	if k, u, err := s.KlienByReportToken(ctx, "laporan-hash-mawar"); err != nil || k.ID != fxMawar || u.ID != fxRana ||
		k.Catatan != "Pemilik: Bu Sari" {
		t.Errorf("KlienByReportToken = %+v, %+v, %v", k, u, err)
	}
	if teks, err := s.CatatanLaporan(ctx, fxMawar, tanggal(2026, 10, 1)); err != nil || teks != "Bulan ini semua aset aman." {
		t.Errorf("CatatanLaporan = %q, %v", teks, err)
	}

	// Items: every kind and every source.
	want := []struct {
		id, nama, jenis, sumber, klien string
		host                           *string
		biaya                          *int64
		due                            time.Time
	}{
		{fxSertifikat, "Sertifikat tokokuemawar.id", "sertifikat", "server", "Toko Kue Mawar", ptr("tokokuemawar.id"), nil, tanggal(2027, 1, 14)},
		{fxRegistri, "tokokuemawar.id", "domain", "registri", "Toko Kue Mawar", ptr("tokokuemawar.id"), ptr[int64](250000), tanggal(2027, 3, 2)},
		{fxDomain, "kopisenja.com", "domain", "manual", "Kopi Senja", ptr("kopisenja.com"), ptr[int64](180000), tanggal(2026, 11, 25)},
		{fxToken, "Token API Midtrans", "token", "manual", "Kopi Senja", nil, nil, tanggal(2026, 11, 10)},
		{fxLangganan, "Hosting Niagahoster", "langganan", "manual", "Studio Lama", nil, ptr[int64](1200000), tanggal(2026, 12, 15)},
		{fxLainnya, "Sewa ruang kerja", "lainnya", "manual", "", nil, ptr[int64](9000000), tanggal(2027, 6, 30)},
		{fxSertGagal, "Sertifikat api.kopisenja.com", "sertifikat", "server", "Kopi Senja", ptr("api.kopisenja.com"), nil, tanggal(2026, 12, 1)},
	}
	for _, w := range want {
		it, err := s.Get(ctx, fxRana, w.id)
		if err != nil {
			t.Errorf("Get(%s): %v", w.nama, err)
			continue
		}
		if it.Nama != w.nama || it.Jenis != w.jenis || it.Sumber != w.sumber || it.Klien != w.klien ||
			str(it.Host) != str(w.host) || !it.Kedaluwarsa.Equal(w.due) || (it.Biaya == nil) != (w.biaya == nil) ||
			(it.Biaya != nil && *it.Biaya != *w.biaya) || it.MataUang != "IDR" ||
			it.UlangJumlah != 0 || it.UlangSatuan != "" || it.SelesaiPada != nil {
			t.Errorf("Get(%s) = %+v", w.nama, it)
		}
	}
	if n := hitung(t, s, `SELECT count(*) FROM item WHERE mata_uang = 'IDR' AND ulang_jumlah IS NULL AND ulang_satuan IS NULL AND selesai_pada IS NULL`); n != fxJumlah["item"] {
		t.Errorf("migrated item defaults count = %d, want %d", n, fxJumlah["item"])
	}
	items, err := s.List(ctx, fxRana, Filter{}, now)
	if err != nil || len(items) != len(want) {
		t.Errorf("List(rana) = %d items, %v; want %d", len(items), err, len(want))
	}
	cert, _ := s.Get(ctx, fxRana, fxSertifikat)
	var wantTLS TLSInfo
	if err := json.Unmarshal([]byte(fxInfoTLS), &wantTLS); err != nil {
		t.Fatal(err)
	}
	if cert.InfoTLS == nil || jsonOf(t, *cert.InfoTLS) != jsonOf(t, wantTLS) || cert.TerakhirDicek == nil {
		t.Errorf("certificate InfoTLS = %+v, TerakhirDicek = %v", cert.InfoTLS, cert.TerakhirDicek)
	}
	reg, _ := s.Get(ctx, fxRana, fxRegistri)
	if reg.InfoRegistri == nil || *reg.InfoRegistri != (RegistryInfo{Domain: "tokokuemawar.id",
		Registrar: "PT Registrar Nusantara", Server: "https://rdap.pandi.id/rdap/"}) {
		t.Errorf("registry InfoRegistri = %+v", reg.InfoRegistri)
	}
	gagal, _ := s.Get(ctx, fxRana, fxSertGagal)
	if gagal.GalatCek == nil || *gagal.GalatCek != "koneksi ditolak" {
		t.Errorf("GalatCek = %s", str(gagal.GalatCek))
	}
	if _, err := s.Get(ctx, fxRana, fxDomainBudi); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get(another user's item) err = %v, want ErrNotFound", err)
	}
	if it, err := s.Get(ctx, fxBudi, fxDomainBudi); err != nil || it.Klien != "Toko Budi" ||
		it.MataUang != "IDR" || it.UlangJumlah != 0 || it.UlangSatuan != "" || it.SelesaiPada != nil {
		t.Errorf("Get(budi's item) = %+v, %v", it, err)
	}
	if sum, err := s.Summary(ctx, fxRana, now); err != nil || sum.Total != len(want) ||
		sum.Lewat+sum.Kritis+sum.Segera+sum.Aman != sum.Total || len(sum.Klien) != 3 {
		t.Errorf("Summary(rana) = %+v, %v", sum, err)
	}
	due, err := s.DueForCheck(ctx, now.AddDate(1, 0, 0))
	var dueIDs []string
	for _, it := range due {
		dueIDs = append(dueIDs, it.ID)
	}
	slices.Sort(dueIDs)
	if err != nil || !slices.Equal(dueIDs, []string{fxSertifikat, fxRegistri, fxSertGagal}) {
		t.Errorf("DueForCheck = %v, %v", dueIDs, err)
	}

	// Tasks.
	tugas, err := s.ListTugas(ctx, fxRana, TugasFilter{})
	if err != nil || len(tugas) != 2 {
		t.Errorf("ListTugas(rana) = %d, %v", len(tugas), err)
	}
	backup, err := s.GetTugas(ctx, fxRana, fxTugasBackup)
	if err != nil || backup.Klien != "Toko Kue Mawar" || backup.UlangJumlah != 1 || backup.UlangSatuan != "bulan" ||
		!backup.JatuhTempo.Equal(tanggal(2026, 12, 1)) || backup.Biaya == nil || *backup.Biaya != 50000 ||
		backup.MataUang != "IDR" || backup.SelesaiPada != nil ||
		backup.TerakhirSelesai == nil || backup.Catatan != "Simpan ke S3" {
		t.Errorf("GetTugas(backup) = %+v, %v", backup, err)
	}
	if plugin, err := s.GetTugas(ctx, fxRana, fxTugasPlugin); err != nil || plugin.KlienID != nil ||
		plugin.UlangSatuan != "minggu" || plugin.MataUang != "IDR" || plugin.SelesaiPada != nil {
		t.Errorf("GetTugas(plugin) = %+v, %v", plugin, err)
	}
	if budiTugas, err := s.GetTugas(ctx, fxBudi, fxTugasBudi); err != nil || budiTugas.MataUang != "IDR" || budiTugas.SelesaiPada != nil {
		t.Errorf("GetTugas(budi) = %+v, %v", budiTugas, err)
	}
	if n := hitung(t, s, `SELECT count(*) FROM tugas WHERE mata_uang = 'IDR' AND selesai_pada IS NULL`); n != fxJumlah["tugas"] {
		t.Errorf("migrated tugas defaults count = %d, want %d", n, fxJumlah["tugas"])
	}

	// Activity: one entry of each of the 11 kinds.
	feed, err := s.ListAktivitas(ctx, fxRana, AktivitasFilter{Batas: 100})
	if err != nil {
		t.Fatalf("ListAktivitas: %v", err)
	}
	var jenis []string
	for _, a := range feed {
		jenis = append(jenis, a.Jenis)
		if a.Jumlah != nil || a.MataUang != nil {
			t.Errorf("migrated activity %s has Jumlah=%v MataUang=%v, want nil", a.Jenis, a.Jumlah, a.MataUang)
		}
		switch a.Jenis {
		case "pekerjaan":
			if a.DurasiMenit == nil || *a.DurasiMenit != 90 || !a.Tanggal.Equal(tanggal(2026, 11, 3)) {
				t.Errorf("work entry = %+v", a)
			}
		case "aset_baru":
			if a.ItemID == nil || *a.ItemID != fxRegistri || !json.Valid(a.Data) {
				t.Errorf("aset_baru = %+v", a)
			}
		case "tugas_selesai":
			if a.TugasID == nil || *a.TugasID != fxTugasBackup || a.Data != nil {
				t.Errorf("tugas_selesai = %+v", a)
			}
		}
	}
	slices.Sort(jenis)
	wantJenis := []string{"aset_baru", "aset_dihapus", "aset_diperpanjang", "catatan", "cek_diperbarui", "cek_gagal",
		"laporan_dibagikan", "pekerjaan", "tugas_baru", "tugas_dihapus", "tugas_selesai"}
	if !slices.Equal(jenis, wantJenis) {
		t.Errorf("activity kinds = %v, want %v", jenis, wantJenis)
	}
	if feed[0].Jenis != "laporan_dibagikan" {
		t.Errorf("newest entry = %s, want laporan_dibagikan", feed[0].Jenis)
	}
}

func jsonOf(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestCheckConstraints makes sure the schema itself refuses rows the handlers
// would never write.
func TestCheckConstraints(t *testing.T) {
	t.Parallel()
	s := storeUji(t)
	ctx := t.Context()
	if err := s.migrateUpTo(ctx, "005_ruang"); err != nil {
		t.Fatal(err)
	}
	isiDataRilis005(t, s)
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	item := `INSERT INTO item (pengguna_id, nama, jenis, host, kedaluwarsa, sumber, biaya) VALUES ($1, $2, $3, $4, '2027-01-01', $5, $6)`
	cases := []struct {
		name, constraint, query string
		args                    []any
	}{
		{"unknown jenis", "item_jenis_check", item, []any{fxRana, "Kendaraan", "kendaraan", nil, "manual", nil}},
		{"certificate without host", "sertifikat_punya_host", item, []any{fxRana, "Sertifikat", "sertifikat", nil, "server", nil}},
		{"manual certificate", "sertifikat_punya_host", item, []any{fxRana, "Sertifikat", "sertifikat", "contoh.id", "manual", nil}},
		{"registry source on a token", "registri_punya_domain", item, []any{fxRana, "Token", "token", "contoh.id", "registri", nil}},
		{"registry domain without host", "registri_punya_domain", item, []any{fxRana, "contoh.id", "domain", nil, "registri", nil}},
		{"unknown sumber", "item_sumber_check", item, []any{fxRana, "contoh.id", "domain", "contoh.id", "whois", nil}},
		{"upper-case host", "item_host_check", item, []any{fxRana, "contoh.id", "domain", "Contoh.ID", "manual", nil}},
		{"single-label host", "item_host_check", item, []any{fxRana, "localhost", "domain", "localhost", "manual", nil}},
		{"negative cost", "item_biaya_check", item, []any{fxRana, "Langganan", "langganan", nil, "manual", -1}},
		{"one-letter name", "item_nama_check", item, []any{fxRana, "x", "lainnya", nil, "manual", nil}},
		{"unknown client status", "klien_status_check",
			`INSERT INTO klien (pengguna_id, nama, status) VALUES ($1, 'Arsip', 'arsip')`, []any{fxRana}},
		{"empty client name", "klien_nama_check", `INSERT INTO klien (pengguna_id, nama) VALUES ($1, '')`, []any{fxRana}},
		{"unknown item currency", "item_mata_uang_check",
			`INSERT INTO item (pengguna_id, nama, jenis, kedaluwarsa, sumber, mata_uang) VALUES ($1, 'Tagihan', 'tagihan', '2027-01-01', 'manual', 'EUR')`, []any{fxRana}},
		{"zero item repeat count", "item_ulang_jumlah_check",
			`INSERT INTO item (pengguna_id, nama, jenis, kedaluwarsa, sumber, ulang_jumlah, ulang_satuan) VALUES ($1, 'Tagihan', 'tagihan', '2027-01-01', 'manual', 0, 'bulan')`, []any{fxRana}},
		{"excessive item repeat count", "item_ulang_jumlah_check",
			`INSERT INTO item (pengguna_id, nama, jenis, kedaluwarsa, sumber, ulang_jumlah, ulang_satuan) VALUES ($1, 'Tagihan', 'tagihan', '2027-01-01', 'manual', 366, 'hari')`, []any{fxRana}},
		{"unknown item repeat unit", "item_ulang_satuan_check",
			`INSERT INTO item (pengguna_id, nama, jenis, kedaluwarsa, sumber, ulang_jumlah, ulang_satuan) VALUES ($1, 'Tagihan', 'tagihan', '2027-01-01', 'manual', 1, 'jam')`, []any{fxRana}},
		{"item repeat count without unit", "item_ulang_lengkap",
			`INSERT INTO item (pengguna_id, nama, jenis, kedaluwarsa, sumber, ulang_jumlah) VALUES ($1, 'Tagihan', 'tagihan', '2027-01-01', 'manual', 1)`, []any{fxRana}},
		{"item repeat unit without count", "item_ulang_lengkap",
			`INSERT INTO item (pengguna_id, nama, jenis, kedaluwarsa, sumber, ulang_satuan) VALUES ($1, 'Tagihan', 'tagihan', '2027-01-01', 'manual', 'bulan')`, []any{fxRana}},
		{"recurring non-manual item", "item_ulang_manual",
			`INSERT INTO item (pengguna_id, nama, jenis, host, kedaluwarsa, sumber, ulang_jumlah, ulang_satuan) VALUES ($1, 'contoh.id', 'domain', 'contoh.id', '2027-01-01', 'registri', 1, 'tahun')`, []any{fxRana}},
		{"tagihan with host", "tagihan_manual",
			`INSERT INTO item (pengguna_id, nama, jenis, host, kedaluwarsa, sumber) VALUES ($1, 'Tagihan', 'tagihan', 'contoh.id', '2027-01-01', 'manual')`, []any{fxRana}},
		{"unknown repeat unit", "tugas_ulang_satuan_check",
			`INSERT INTO tugas (pengguna_id, judul, ulang_jumlah, ulang_satuan, jatuh_tempo) VALUES ($1, 'Cek', 1, 'jam', '2027-01-01')`, []any{fxRana}},
		{"zero repeat count", "tugas_ulang_jumlah_check",
			`INSERT INTO tugas (pengguna_id, judul, ulang_jumlah, jatuh_tempo) VALUES ($1, 'Cek', 0, '2027-01-01')`, []any{fxRana}},
		{"unknown task currency", "tugas_mata_uang_check",
			`INSERT INTO tugas (pengguna_id, judul, jatuh_tempo, mata_uang) VALUES ($1, 'Cek', '2027-01-01', 'EUR')`, []any{fxRana}},
		{"task repeat count without unit", "tugas_ulang_lengkap",
			`INSERT INTO tugas (pengguna_id, judul, ulang_jumlah, jatuh_tempo) VALUES ($1, 'Cek', 1, '2027-01-01')`, []any{fxRana}},
		{"task repeat unit without count", "tugas_ulang_lengkap",
			`INSERT INTO tugas (pengguna_id, judul, ulang_satuan, jatuh_tempo) VALUES ($1, 'Cek', 'bulan', '2027-01-01')`, []any{fxRana}},
		{"closed recurring task", "tugas_selesai_sekali",
			`INSERT INTO tugas (pengguna_id, judul, ulang_jumlah, ulang_satuan, jatuh_tempo, selesai_pada) VALUES ($1, 'Cek', 1, 'bulan', '2027-01-01', now())`, []any{fxRana}},
		{"unknown activity kind", "aktivitas_jenis_check",
			`INSERT INTO aktivitas (pengguna_id, jenis, teks, tanggal) VALUES ($1, 'telepon', 'Telepon klien', '2026-11-19')`, []any{fxRana}},
		{"empty activity text", "aktivitas_teks_check",
			`INSERT INTO aktivitas (pengguna_id, jenis, teks, tanggal) VALUES ($1, 'catatan', '', '2026-11-19')`, []any{fxRana}},
		{"zero-minute work entry", "aktivitas_durasi_menit_check",
			`INSERT INTO aktivitas (pengguna_id, jenis, teks, tanggal, durasi_menit) VALUES ($1, 'pekerjaan', 'Kerja', '2026-11-19', 0)`, []any{fxRana}},
		{"negative activity amount", "aktivitas_jumlah_check",
			`INSERT INTO aktivitas (pengguna_id, jenis, teks, tanggal, jumlah, mata_uang) VALUES ($1, 'tagihan_dibayar', 'Bayar', '2026-11-19', -1, 'IDR')`, []any{fxRana}},
		{"activity amount without currency", "aktivitas_uang_lengkap",
			`INSERT INTO aktivitas (pengguna_id, jenis, teks, tanggal, jumlah) VALUES ($1, 'tagihan_dibayar', 'Bayar', '2026-11-19', 1000)`, []any{fxRana}},
		{"activity currency without amount", "aktivitas_uang_lengkap",
			`INSERT INTO aktivitas (pengguna_id, jenis, teks, tanggal, mata_uang) VALUES ($1, 'tagihan_dibayar', 'Bayar', '2026-11-19', 'IDR')`, []any{fxRana}},
		{"activity unknown currency", "aktivitas_uang_lengkap",
			`INSERT INTO aktivitas (pengguna_id, jenis, teks, tanggal, jumlah, mata_uang) VALUES ($1, 'tagihan_dibayar', 'Bayar', '2026-11-19', 1000, 'EUR')`, []any{fxRana}},
		{"empty token name", "token_akses_nama_check",
			`INSERT INTO token_akses (pengguna_id, nama, hash_token, awalan, cakupan) VALUES ($1, '', 'h1', 'tgt_1111', 'baca')`, []any{fxRana}},
		{"unknown token scope", "token_akses_cakupan_check",
			`INSERT INTO token_akses (pengguna_id, nama, hash_token, awalan, cakupan) VALUES ($1, 'Token', 'h2', 'tgt_2222', 'admin')`, []any{fxRana}},
		{"report month not on the first", "catatan_laporan_bulan_check",
			`INSERT INTO catatan_laporan (klien_id, bulan) VALUES ($1, '2026-11-15')`, []any{fxMawar}},
		{"upper-case email", "pengguna_email_check",
			`INSERT INTO pengguna (nama, email, hash_sandi) VALUES ('Siti', 'Siti@Contoh.id', 'x')`, nil},
	}
	for _, c := range cases {
		_, err := s.pool.Exec(ctx, c.query, c.args...)
		if code, constraint := kodeSQL(err); code != "23514" || constraint != c.constraint {
			t.Errorf("%s: err = %v, want check_violation of %s", c.name, err, c.constraint)
		}
	}
	cekJumlahTabel(t, s)
}

// TestMigrate008SGDAcceptsSGD verifies that migrating from 007_token_akses to
// 008_sgd preserves existing IDR/USD rows and allows SGD in item, tugas, and aktivitas.
func TestMigrate008SGDAcceptsSGD(t *testing.T) {
	t.Parallel()
	s := storeUji(t)
	ctx := t.Context()
	if err := s.migrateUpTo(ctx, "007_token_akses"); err != nil {
		t.Fatalf("migrateUpTo(007_token_akses): %v", err)
	}
	isiDataRilis005(t, s)
	exec(t, s, `INSERT INTO item (pengguna_id, nama, jenis, kedaluwarsa, sumber, biaya, mata_uang)
		VALUES ($1, 'Claude Pro', 'tagihan', '2027-01-01', 'manual', 2000, 'USD')`, fxRana)
	exec(t, s, `INSERT INTO tugas (pengguna_id, judul, jatuh_tempo, biaya, mata_uang)
		VALUES ($1, 'Audit USD', '2027-01-01', 5000, 'USD')`, fxRana)
	exec(t, s, `INSERT INTO aktivitas (pengguna_id, jenis, teks, tanggal, jumlah, mata_uang)
		VALUES ($1, 'tagihan_dibayar', 'Bayar USD', '2026-11-19', 2000, 'USD')`, fxRana)

	// Before 008_sgd, SGD is still rejected.
	if _, err := s.pool.Exec(ctx, `INSERT INTO item (pengguna_id, nama, jenis, kedaluwarsa, sumber, biaya, mata_uang)
		VALUES ($1, 'SG VPS', 'tagihan', '2027-01-01', 'manual', 1500, 'SGD')`, fxRana); err == nil {
		t.Fatal("SGD accepted before 008_sgd")
	}

	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("Migrate to 008_sgd: %v", err)
	}

	// After 008_sgd, SGD is accepted in all three tables while EUR remains rejected.
	exec(t, s, `INSERT INTO item (pengguna_id, nama, jenis, kedaluwarsa, sumber, biaya, mata_uang)
		VALUES ($1, 'SG VPS', 'tagihan', '2027-01-01', 'manual', 1500, 'SGD')`, fxRana)
	exec(t, s, `INSERT INTO tugas (pengguna_id, judul, jatuh_tempo, biaya, mata_uang)
		VALUES ($1, 'Audit SGD', '2027-01-01', 2500, 'SGD')`, fxRana)
	exec(t, s, `INSERT INTO aktivitas (pengguna_id, jenis, teks, tanggal, jumlah, mata_uang)
		VALUES ($1, 'tagihan_dibayar', 'Bayar SGD', '2026-11-19', 1500, 'SGD')`, fxRana)

	if n := hitung(t, s, `SELECT count(*) FROM item WHERE mata_uang IN ('IDR','USD','SGD')`); n != fxJumlah["item"]+2 {
		t.Errorf("item count after 008_sgd = %d, want %d", n, fxJumlah["item"]+2)
	}
	if n := hitung(t, s, `SELECT count(*) FROM tugas WHERE mata_uang = 'SGD'`); n != 1 {
		t.Errorf("SGD tugas count = %d, want 1", n)
	}
	if n := hitung(t, s, `SELECT count(*) FROM aktivitas WHERE mata_uang = 'SGD'`); n != 1 {
		t.Errorf("SGD aktivitas count = %d, want 1", n)
	}
}
