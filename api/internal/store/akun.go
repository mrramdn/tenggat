package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ErrEmailTaken is returned when an account already uses the email.
var ErrEmailTaken = errors.New("email sudah terdaftar")

// Pengguna is one account.
type Pengguna struct {
	ID        string    `json:"id"`
	Nama      string    `json:"nama"`
	Email     string    `json:"email"`
	HashSandi string    `json:"-"`
	Dibuat    time.Time `json:"dibuat"`
}

func scanUser(row pgx.Row) (Pengguna, error) {
	var u Pengguna
	err := row.Scan(&u.ID, &u.Nama, &u.Email, &u.HashSandi, &u.Dibuat)
	if errors.Is(err, pgx.ErrNoRows) {
		return u, ErrNotFound
	}
	return u, err
}

// CreateUser stores a new account; the email must already be lower case.
func (s *Store) CreateUser(ctx context.Context, nama, email, hash string) (Pengguna, error) {
	u, err := scanUser(s.pool.QueryRow(ctx, `INSERT INTO pengguna (nama, email, hash_sandi) VALUES ($1, $2, $3)
		RETURNING id, nama, email, hash_sandi, dibuat`, nama, email, hash))
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation on pengguna_email_idx
		return u, ErrEmailTaken
	}
	return u, err
}

// UserByEmail finds an account by its lower-case email.
func (s *Store) UserByEmail(ctx context.Context, email string) (Pengguna, error) {
	return scanUser(s.pool.QueryRow(ctx, `SELECT id, nama, email, hash_sandi, dibuat FROM pengguna WHERE email = $1`, email))
}

// CreateSession stores the hash of a new session token.
func (s *Store) CreateSession(ctx context.Context, tokenHash, userID string, expires time.Time) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO sesi (hash_token, pengguna_id, kedaluwarsa) VALUES ($1, $2, $3)`,
		tokenHash, userID, expires)
	return err
}

// UserBySession returns the account of a session that has not expired.
func (s *Store) UserBySession(ctx context.Context, tokenHash string, now time.Time) (Pengguna, error) {
	return scanUser(s.pool.QueryRow(ctx, `SELECT p.id, p.nama, p.email, p.hash_sandi, p.dibuat
		FROM sesi s JOIN pengguna p ON p.id = s.pengguna_id
		WHERE s.hash_token = $1 AND s.kedaluwarsa > $2`, tokenHash, now))
}

// DeleteSession ends one session.
func (s *Store) DeleteSession(ctx context.Context, tokenHash string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM sesi WHERE hash_token = $1`, tokenHash)
	return err
}

// DeleteUser removes an account; its sessions and items go with it (ON DELETE CASCADE).
func (s *Store) DeleteUser(ctx context.Context, userID string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM pengguna WHERE id = $1`, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteExpiredSessions removes sessions past their end.
func (s *Store) DeleteExpiredSessions(ctx context.Context, now time.Time) (int64, error) {
	tag, err := s.pool.Exec(ctx, `DELETE FROM sesi WHERE kedaluwarsa <= $1`, now)
	return tag.RowsAffected(), err
}
