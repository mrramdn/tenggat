-- Accounts: every asset now belongs to one user.
CREATE TABLE pengguna (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    nama          text NOT NULL CHECK (char_length(nama) BETWEEN 2 AND 80),
    email         text NOT NULL CHECK (char_length(email) BETWEEN 3 AND 254 AND email = lower(email)),
    hash_sandi    text NOT NULL,
    dibuat        timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX pengguna_email_idx ON pengguna (email);

-- A session is stored as the SHA-256 of its token, so a database dump cannot be replayed as cookies.
CREATE TABLE sesi (
    hash_token    text PRIMARY KEY,
    pengguna_id   uuid NOT NULL REFERENCES pengguna (id) ON DELETE CASCADE,
    dibuat        timestamptz NOT NULL DEFAULT now(),
    kedaluwarsa   timestamptz NOT NULL
);
CREATE INDEX sesi_pengguna_idx ON sesi (pengguna_id);

-- Assets from the version without accounts have no owner and cannot be shown to anyone, so they are removed.
ALTER TABLE item ADD COLUMN pengguna_id uuid REFERENCES pengguna (id) ON DELETE CASCADE;
DELETE FROM item WHERE pengguna_id IS NULL;
ALTER TABLE item ALTER COLUMN pengguna_id SET NOT NULL;
DROP INDEX item_kedaluwarsa_idx;
CREATE INDEX item_pengguna_kedaluwarsa_idx ON item (pengguna_id, kedaluwarsa);
