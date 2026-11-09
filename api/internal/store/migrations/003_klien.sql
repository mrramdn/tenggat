-- Clients become rows of their own instead of free text on each asset.
CREATE TABLE klien (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    pengguna_id   uuid NOT NULL REFERENCES pengguna (id) ON DELETE CASCADE,
    nama          text NOT NULL CHECK (char_length(nama) BETWEEN 1 AND 80),
    catatan       text NOT NULL DEFAULT '' CHECK (char_length(catatan) <= 500),
    dibuat        timestamptz NOT NULL DEFAULT now()
);
-- "Toko Budi" and "toko budi" are the same client.
CREATE UNIQUE INDEX klien_pengguna_nama_idx ON klien (pengguna_id, lower(nama));

-- One client per name a user already typed. Spellings that differ only in case
-- collapse into one, named after the spelling used on the most assets.
INSERT INTO klien (pengguna_id, nama, dibuat)
SELECT DISTINCT ON (pengguna_id, lower(nama)) pengguna_id, nama, pertama
FROM (
    SELECT pengguna_id, btrim(klien) AS nama, count(*) AS jumlah, min(dibuat) AS pertama
    FROM item
    WHERE btrim(klien) <> ''
    GROUP BY pengguna_id, btrim(klien)
) AS ejaan
ORDER BY pengguna_id, lower(nama), jumlah DESC, pertama;

ALTER TABLE item ADD COLUMN klien_id uuid REFERENCES klien (id) ON DELETE SET NULL;
UPDATE item AS i SET klien_id = k.id
FROM klien AS k
WHERE k.pengguna_id = i.pengguna_id AND lower(k.nama) = lower(btrim(i.klien)) AND btrim(i.klien) <> '';
ALTER TABLE item DROP COLUMN klien;
CREATE INDEX item_pengguna_klien_idx ON item (pengguna_id, klien_id);
