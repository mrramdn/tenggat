package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// RegistryInfo is what the last RDAP lookup of a domain item read.
type RegistryInfo struct {
	Domain    string `json:"domain"`
	Registrar string `json:"registrar"`
	Server    string `json:"server"`
}

// SaveRegistryCheck stores a successful registry lookup of a domain item.
func (s *Store) SaveRegistryCheck(ctx context.Context, id string, date time.Time, info RegistryInfo, at time.Time) (Item, error) {
	return scanItem(s.pool.QueryRow(ctx, `UPDATE item SET
		kedaluwarsa = $2, info_registri = $3, terakhir_dicek = $4, galat_cek = NULL, diubah = now()
		WHERE id = $1 AND sumber = 'registri'
		RETURNING `+columns, id, date, info, at))
}

// SetCalendarToken stores the hash of a user's calendar link token, replacing
// the previous one, so the old link stops working.
func (s *Store) SetCalendarToken(ctx context.Context, userID, tokenHash string) error {
	tag, err := s.pool.Exec(ctx, `UPDATE pengguna SET hash_token_kalender = $2 WHERE id = $1`, userID, tokenHash)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// DeleteCalendarToken turns a user's calendar link off.
func (s *Store) DeleteCalendarToken(ctx context.Context, userID string) error {
	_, err := s.pool.Exec(ctx, `UPDATE pengguna SET hash_token_kalender = NULL WHERE id = $1`, userID)
	return err
}

// UserByCalendarToken finds the account a calendar link belongs to.
func (s *Store) UserByCalendarToken(ctx context.Context, tokenHash string) (Pengguna, error) {
	return scanUser(s.pool.QueryRow(ctx, `SELECT id, nama, email, hash_sandi, dibuat
		FROM pengguna WHERE hash_token_kalender = $1`, tokenHash))
}

// CalendarActive reports whether a user has a calendar link.
func (s *Store) CalendarActive(ctx context.Context, userID string) (bool, error) {
	var active bool
	err := s.pool.QueryRow(ctx, `SELECT hash_token_kalender IS NOT NULL FROM pengguna WHERE id = $1`, userID).Scan(&active)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrNotFound
	}
	return active, err
}
