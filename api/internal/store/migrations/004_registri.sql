-- Domains whose expiry date is read from the registry over RDAP.
ALTER TABLE item DROP CONSTRAINT item_sumber_check;
ALTER TABLE item ADD CONSTRAINT item_sumber_check CHECK (sumber IN ('manual', 'server', 'registri'));
-- What the last registry lookup said: {"domain", "registrar", "server"}.
ALTER TABLE item ADD COLUMN info_registri jsonb;
-- Only a domain with a host is read from the registry.
ALTER TABLE item ADD CONSTRAINT registri_punya_domain
    CHECK (sumber <> 'registri' OR (jenis = 'domain' AND host IS NOT NULL));

-- A calendar subscription link: the SHA-256 of its token, like sessions, so a
-- database dump cannot be turned into working links. NULL means no link.
ALTER TABLE pengguna ADD COLUMN hash_token_kalender text UNIQUE;
