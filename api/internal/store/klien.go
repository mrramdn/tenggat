package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/mrramdn/tenggat/api/internal/expiry"
)

// ErrKlienTaken is returned when the user already has a client with that name,
// compared without regard to case.
var ErrKlienTaken = errors.New("nama klien sudah dipakai")

// Klien is one of a user's clients.
type Klien struct {
	ID         string
	PenggunaID string
	Nama       string
	Catatan    string
	Dibuat     time.Time
}

// KlienRingkas is a client with figures over its assets.
type KlienRingkas struct {
	Klien
	JumlahAset  int
	Terdekat    *time.Time // the earliest expiry date among its assets, nil without assets
	Biaya30Hari int64      // the summed cost of assets that expire from today to 30 days ahead
}

const klienColumns = `id, pengguna_id, nama, catatan, dibuat`

func scanKlien(row pgx.Row) (Klien, error) {
	var k Klien
	err := row.Scan(&k.ID, &k.PenggunaID, &k.Nama, &k.Catatan, &k.Dibuat)
	if errors.Is(err, pgx.ErrNoRows) {
		return k, ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation on klien_pengguna_nama_idx
		return k, ErrKlienTaken
	}
	return k, err
}

// ListKlien returns a user's clients sorted by name, each with its asset count,
// nearest expiry date and the cost due in the next 30 days. "Today" is the
// date in Jakarta, as in Summary.
func (s *Store) ListKlien(ctx context.Context, userID string, now time.Time) ([]KlienRingkas, error) {
	rows, err := s.pool.Query(ctx, `SELECT k.id, k.pengguna_id, k.nama, k.catatan, k.dibuat,
		count(i.id), min(i.kedaluwarsa),
		COALESCE(sum(i.biaya) FILTER (WHERE i.kedaluwarsa BETWEEN $2::date AND $2::date + 30), 0)
		FROM klien AS k LEFT JOIN item AS i ON i.klien_id = k.id
		WHERE k.pengguna_id = $1
		GROUP BY k.id
		ORDER BY lower(k.nama), k.nama`, userID, expiry.DateOf(now))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []KlienRingkas{}
	for rows.Next() {
		var k KlienRingkas
		if err := rows.Scan(&k.ID, &k.PenggunaID, &k.Nama, &k.Catatan, &k.Dibuat,
			&k.JumlahAset, &k.Terdekat, &k.Biaya30Hari); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// GetKlien returns one of a user's clients. Another user's client is reported as not found.
func (s *Store) GetKlien(ctx context.Context, userID, id string) (Klien, error) {
	return scanKlien(s.pool.QueryRow(ctx, `SELECT `+klienColumns+` FROM klien WHERE id = $1 AND pengguna_id = $2`,
		id, userID))
}

// CreateKlien adds a client. It returns ErrKlienTaken when the name is in use.
func (s *Store) CreateKlien(ctx context.Context, userID, nama, catatan string) (Klien, error) {
	return scanKlien(s.pool.QueryRow(ctx, `INSERT INTO klien (pengguna_id, nama, catatan) VALUES ($1, $2, $3)
		RETURNING `+klienColumns, userID, nama, catatan))
}

// UpdateKlien renames a client or changes its note.
func (s *Store) UpdateKlien(ctx context.Context, userID, id, nama, catatan string) (Klien, error) {
	return scanKlien(s.pool.QueryRow(ctx, `UPDATE klien SET nama = $3, catatan = $4
		WHERE id = $1 AND pengguna_id = $2
		RETURNING `+klienColumns, id, userID, nama, catatan))
}

// DeleteKlien removes a client; its assets stay, without a client.
func (s *Store) DeleteKlien(ctx context.Context, userID, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM klien WHERE id = $1 AND pengguna_id = $2`, id, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// EnsureKlien returns the user's client with this name, ignoring case, and
// creates it when there is none. Two requests racing on a new name get the same row.
func (s *Store) EnsureKlien(ctx context.Context, userID, nama string) (Klien, error) {
	k, err := scanKlien(s.pool.QueryRow(ctx, `INSERT INTO klien (pengguna_id, nama) VALUES ($1, $2)
		ON CONFLICT (pengguna_id, lower(nama)) DO NOTHING
		RETURNING `+klienColumns, userID, nama))
	if !errors.Is(err, ErrNotFound) {
		return k, err
	}
	return scanKlien(s.pool.QueryRow(ctx, `SELECT `+klienColumns+` FROM klien
		WHERE pengguna_id = $1 AND lower(nama) = lower($2)`, userID, nama))
}
