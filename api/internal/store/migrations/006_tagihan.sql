ALTER TABLE item DROP CONSTRAINT item_jenis_check;
ALTER TABLE item ADD CONSTRAINT item_jenis_check
  CHECK (jenis IN ('sertifikat','domain','token','langganan','tagihan','lainnya'));
ALTER TABLE item
  ADD COLUMN mata_uang text NOT NULL DEFAULT 'IDR',
  ADD COLUMN ulang_jumlah integer,
  ADD COLUMN ulang_satuan text,
  ADD COLUMN selesai_pada timestamptz,
  ADD CONSTRAINT item_mata_uang_check    CHECK (mata_uang IN ('IDR','USD')),
  ADD CONSTRAINT item_ulang_jumlah_check CHECK (ulang_jumlah BETWEEN 1 AND 365),
  ADD CONSTRAINT item_ulang_satuan_check CHECK (ulang_satuan IN ('hari','minggu','bulan','tahun')),
  ADD CONSTRAINT item_ulang_lengkap      CHECK ((ulang_jumlah IS NULL) = (ulang_satuan IS NULL)),
  ADD CONSTRAINT item_ulang_manual       CHECK (ulang_jumlah IS NULL OR sumber = 'manual'),
  ADD CONSTRAINT tagihan_manual          CHECK (jenis <> 'tagihan' OR (sumber = 'manual' AND host IS NULL));

ALTER TABLE tugas
  ALTER COLUMN ulang_jumlah DROP NOT NULL, ALTER COLUMN ulang_jumlah DROP DEFAULT,
  ALTER COLUMN ulang_satuan DROP NOT NULL, ALTER COLUMN ulang_satuan DROP DEFAULT,
  ADD COLUMN mata_uang text NOT NULL DEFAULT 'IDR',
  ADD COLUMN selesai_pada timestamptz,
  ADD CONSTRAINT tugas_mata_uang_check CHECK (mata_uang IN ('IDR','USD')),
  ADD CONSTRAINT tugas_ulang_lengkap   CHECK ((ulang_jumlah IS NULL) = (ulang_satuan IS NULL)),
  ADD CONSTRAINT tugas_selesai_sekali  CHECK (selesai_pada IS NULL OR ulang_jumlah IS NULL);

ALTER TABLE aktivitas DROP CONSTRAINT aktivitas_jenis_check;
ALTER TABLE aktivitas ADD CONSTRAINT aktivitas_jenis_check CHECK (jenis IN ('catatan','pekerjaan','aset_baru',
  'aset_diperpanjang','aset_dihapus','cek_diperbarui','cek_gagal','tugas_baru','tugas_selesai','tugas_dihapus',
  'laporan_dibagikan','tagihan_dibayar'));
ALTER TABLE aktivitas
  ADD COLUMN jumlah bigint,
  ADD COLUMN mata_uang text,
  ADD CONSTRAINT aktivitas_jumlah_check CHECK (jumlah IS NULL OR jumlah >= 0),
  ADD CONSTRAINT aktivitas_uang_lengkap CHECK ((jumlah IS NULL) = (mata_uang IS NULL)
                                              AND (mata_uang IS NULL OR mata_uang IN ('IDR','USD')));
