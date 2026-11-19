package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// MaxTokensPerUser caps how many personal access tokens one account may hold.
const MaxTokensPerUser = 20

// ErrTokenLimit is returned when a user already has MaxTokensPerUser tokens.
var ErrTokenLimit = errors.New("batas jumlah token akses tercapai")

// TokenAkses is one personal access token, without its secret hash.
type TokenAkses struct {
	ID              string
	PenggunaID      string
	Nama            string
	Awalan          string
	Cakupan         string // baca or tulis
	Dibuat          time.Time
	TerakhirDipakai *time.Time
	Kedaluwarsa     *time.Time
}

// TokenInput holds the fields for a new personal access token.
type TokenInput struct {
	Nama        string
	Hash        string
	Awalan      string
	Cakupan     string
	Kedaluwarsa *time.Time
}

const tokenColumns = `id, pengguna_id, nama, awalan, cakupan, dibuat, terakhir_dipakai, kedaluwarsa`

func scanToken(row pgx.Row) (TokenAkses, error) {
	var t TokenAkses
	err := row.Scan(&t.ID, &t.PenggunaID, &t.Nama, &t.Awalan, &t.Cakupan, &t.Dibuat, &t.TerakhirDipakai, &t.Kedaluwarsa)
	if errors.Is(err, pgx.ErrNoRows) {
		return t, ErrNotFound
	}
	return t, err
}

// ListTokens returns a user's personal access tokens, newest first.
func (s *Store) ListTokens(ctx context.Context, userID string) ([]TokenAkses, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+tokenColumns+` FROM token_akses
		WHERE pengguna_id = $1
		ORDER BY dibuat DESC, id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TokenAkses{}
	for rows.Next() {
		t, err := scanToken(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// CreateToken stores a new personal access token for a user, up to
// MaxTokensPerUser tokens per account.
func (s *Store) CreateToken(ctx context.Context, userID string, in TokenInput) (TokenAkses, error) {
	t, err := scanToken(s.pool.QueryRow(ctx, `INSERT INTO token_akses
		(pengguna_id, nama, hash_token, awalan, cakupan, kedaluwarsa)
		SELECT $1, $2, $3, $4, $5, $6
		WHERE (SELECT count(*) FROM token_akses WHERE pengguna_id = $1) < $7
		RETURNING `+tokenColumns,
		userID, in.Nama, in.Hash, in.Awalan, in.Cakupan, in.Kedaluwarsa, MaxTokensPerUser))
	if errors.Is(err, ErrNotFound) {
		return t, ErrTokenLimit
	}
	return t, err
}

// DeleteToken removes one of a user's personal access tokens.
func (s *Store) DeleteToken(ctx context.Context, userID, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM token_akses WHERE id = $1 AND pengguna_id = $2`, id, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// UserByToken returns the account and token matching a token hash when the
// token has not expired. It stamps terakhir_dipakai at most once a minute; a
// failed stamp does not fail the lookup.
func (s *Store) UserByToken(ctx context.Context, hash string, now time.Time) (Pengguna, TokenAkses, error) {
	var u Pengguna
	var t TokenAkses
	err := s.pool.QueryRow(ctx, `SELECT p.id, p.nama, p.email, p.hash_sandi, p.dibuat,
		t.id, t.pengguna_id, t.nama, t.awalan, t.cakupan, t.dibuat, t.terakhir_dipakai, t.kedaluwarsa
		FROM token_akses AS t JOIN pengguna AS p ON p.id = t.pengguna_id
		WHERE t.hash_token = $1 AND (t.kedaluwarsa IS NULL OR t.kedaluwarsa > $2)`, hash, now).Scan(
		&u.ID, &u.Nama, &u.Email, &u.HashSandi, &u.Dibuat,
		&t.ID, &t.PenggunaID, &t.Nama, &t.Awalan, &t.Cakupan, &t.Dibuat, &t.TerakhirDipakai, &t.Kedaluwarsa)
	if errors.Is(err, pgx.ErrNoRows) {
		return u, t, ErrNotFound
	}
	if err != nil {
		return u, t, err
	}
	if t.TerakhirDipakai == nil || now.Sub(*t.TerakhirDipakai) >= time.Minute {
		if _, err := s.pool.Exec(ctx, `UPDATE token_akses SET terakhir_dipakai = $2
			WHERE id = $1 AND (terakhir_dipakai IS NULL OR terakhir_dipakai <= $2::timestamptz - interval '1 minute')`,
			t.ID, now); err == nil {
			t.TerakhirDipakai = &now
		}
	}
	return u, t, nil
}
