-- One row per asset whose expiry date Tenggat watches.
CREATE TABLE item (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    nama           text NOT NULL CHECK (char_length(nama) BETWEEN 2 AND 120),
    jenis          text NOT NULL CHECK (jenis IN ('sertifikat', 'domain', 'token', 'langganan', 'lainnya')),
    klien          text NOT NULL DEFAULT '' CHECK (char_length(klien) <= 80),
    host           text CHECK (host ~ '^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)+$'),
    kedaluwarsa    date NOT NULL,
    sumber         text NOT NULL DEFAULT 'manual' CHECK (sumber IN ('manual', 'server')),
    biaya          bigint CHECK (biaya >= 0),
    catatan        text NOT NULL DEFAULT '' CHECK (char_length(catatan) <= 500),
    info_tls       jsonb,
    terakhir_dicek timestamptz,
    galat_cek      text,
    dibuat         timestamptz NOT NULL DEFAULT now(),
    diubah         timestamptz NOT NULL DEFAULT now(),
    -- A certificate is always read from a host.
    CONSTRAINT sertifikat_punya_host CHECK (jenis <> 'sertifikat' OR (host IS NOT NULL AND sumber = 'server'))
);

CREATE INDEX item_kedaluwarsa_idx ON item (kedaluwarsa);
