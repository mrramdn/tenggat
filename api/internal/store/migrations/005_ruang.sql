-- Each client gets its own space: a status, recurring maintenance tasks, an activity feed that also holds the work
-- log, a monthly report that can be shared as a read-only link, and the owner's own notification webhook.

ALTER TABLE klien ADD COLUMN status text NOT NULL DEFAULT 'aktif' CHECK (status IN ('aktif', 'jeda', 'selesai'));

-- A recurring maintenance task (backups, plugin updates, a monthly check...). Its next due date is a deadline like an
-- asset's expiry date, so it gets the same statuses.
CREATE TABLE tugas (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    pengguna_id      uuid NOT NULL REFERENCES pengguna (id) ON DELETE CASCADE,
    klien_id         uuid REFERENCES klien (id) ON DELETE SET NULL,
    judul            text NOT NULL CHECK (char_length(judul) BETWEEN 2 AND 120),
    catatan          text NOT NULL DEFAULT '' CHECK (char_length(catatan) <= 500),
    ulang_jumlah     integer NOT NULL DEFAULT 1 CHECK (ulang_jumlah BETWEEN 1 AND 365),
    ulang_satuan     text NOT NULL DEFAULT 'bulan' CHECK (ulang_satuan IN ('hari', 'minggu', 'bulan', 'tahun')),
    jatuh_tempo      date NOT NULL,
    biaya            bigint CHECK (biaya >= 0),
    terakhir_selesai timestamptz,
    dibuat           timestamptz NOT NULL DEFAULT now(),
    diubah           timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX tugas_pengguna_jatuh_tempo_idx ON tugas (pengguna_id, jatuh_tempo);
CREATE INDEX tugas_klien_idx ON tugas (klien_id);

-- What happened, per client: notes and work entries the user writes, and events the app records itself.
-- 'catatan' is internal; 'pekerjaan' is work done for the client and goes into the monthly report.
CREATE TABLE aktivitas (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    pengguna_id uuid NOT NULL REFERENCES pengguna (id) ON DELETE CASCADE,
    klien_id    uuid REFERENCES klien (id) ON DELETE SET NULL,
    item_id     uuid REFERENCES item (id) ON DELETE SET NULL,
    tugas_id    uuid REFERENCES tugas (id) ON DELETE SET NULL,
    jenis       text NOT NULL CHECK (jenis IN ('catatan', 'pekerjaan', 'aset_baru', 'aset_diperpanjang',
                                               'aset_dihapus', 'cek_diperbarui', 'cek_gagal', 'tugas_baru',
                                               'tugas_selesai', 'tugas_dihapus', 'laporan_dibagikan')),
    teks        text NOT NULL CHECK (char_length(teks) BETWEEN 1 AND 2000),
    -- The Jakarta date the work happened (a work entry may be logged later); the creation date otherwise.
    tanggal     date NOT NULL,
    durasi_menit integer CHECK (durasi_menit BETWEEN 1 AND 1440),
    data        jsonb,
    dibuat      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX aktivitas_pengguna_klien_dibuat_idx ON aktivitas (pengguna_id, klien_id, dibuat DESC);
CREATE INDEX aktivitas_klien_tanggal_idx ON aktivitas (klien_id, tanggal);

-- A client report shared as a read-only link. Like sessions, only the SHA-256 of the token is stored.
ALTER TABLE klien ADD COLUMN hash_token_laporan text UNIQUE;

-- The owner's own words for one client's report of one month (a summary or recommendations).
CREATE TABLE catatan_laporan (
    klien_id uuid NOT NULL REFERENCES klien (id) ON DELETE CASCADE,
    bulan    date NOT NULL CHECK (extract(day FROM bulan) = 1),
    teks     text NOT NULL DEFAULT '' CHECK (char_length(teks) <= 2000),
    diubah   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (klien_id, bulan)
);

-- The owner's own webhook (Slack, Discord or Telegram) for reminders; NULL means off. webhook_terakhir is the Jakarta
-- date of the last daily digest, so a restart never sends the same day twice.
ALTER TABLE pengguna ADD COLUMN webhook_url text;
ALTER TABLE pengguna ADD COLUMN webhook_terakhir date;
