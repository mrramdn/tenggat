CREATE TABLE token_akses (
  id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  pengguna_id      uuid NOT NULL REFERENCES pengguna (id) ON DELETE CASCADE,
  nama             text NOT NULL CONSTRAINT token_akses_nama_check CHECK (char_length(nama) BETWEEN 1 AND 60),
  hash_token       text NOT NULL CONSTRAINT token_akses_hash_unik UNIQUE,
  awalan           text NOT NULL,
  cakupan          text NOT NULL CONSTRAINT token_akses_cakupan_check CHECK (cakupan IN ('baca','tulis')),
  dibuat           timestamptz NOT NULL DEFAULT now(),
  terakhir_dipakai timestamptz,
  kedaluwarsa      timestamptz
);
CREATE INDEX token_akses_pengguna_idx ON token_akses (pengguna_id);
