package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// CatatanLaporan returns the owner's words for one client's report of one
// month (bulan is the first day), "" when there are none.
func (s *Store) CatatanLaporan(ctx context.Context, klienID string, bulan time.Time) (string, error) {
	var teks string
	err := s.pool.QueryRow(ctx, `SELECT teks FROM catatan_laporan WHERE klien_id = $1 AND bulan = $2`,
		klienID, bulan).Scan(&teks)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return teks, err
}

// SetCatatanLaporan stores the owner's words for one month of one of the
// user's clients. Another user's client is reported as not found.
func (s *Store) SetCatatanLaporan(ctx context.Context, userID, klienID string, bulan time.Time, teks string) error {
	tag, err := s.pool.Exec(ctx, `INSERT INTO catatan_laporan (klien_id, bulan, teks)
		SELECT id, $3, $4 FROM klien WHERE id = $1 AND pengguna_id = $2
		ON CONFLICT (klien_id, bulan) DO UPDATE SET teks = excluded.teks, diubah = now()`, klienID, userID, bulan, teks)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// SetReportToken stores the hash of a client's report link token, replacing the
// previous one, so the old link stops working.
func (s *Store) SetReportToken(ctx context.Context, userID, klienID, tokenHash string) error {
	tag, err := s.pool.Exec(ctx, `UPDATE klien SET hash_token_laporan = $3 WHERE id = $1 AND pengguna_id = $2`,
		klienID, userID, tokenHash)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// DeleteReportToken turns a client's report link off.
func (s *Store) DeleteReportToken(ctx context.Context, userID, klienID string) error {
	tag, err := s.pool.Exec(ctx, `UPDATE klien SET hash_token_laporan = NULL WHERE id = $1 AND pengguna_id = $2`,
		klienID, userID)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// KlienByReportToken finds the client a report link belongs to, and its owner.
func (s *Store) KlienByReportToken(ctx context.Context, tokenHash string) (Klien, Pengguna, error) {
	var k Klien
	var u Pengguna
	err := s.pool.QueryRow(ctx, `SELECT k.id, k.pengguna_id, k.nama, k.catatan, k.status, k.dibuat,
		p.id, p.nama, p.email, p.dibuat
		FROM klien AS k JOIN pengguna AS p ON p.id = k.pengguna_id
		WHERE k.hash_token_laporan = $1`, tokenHash).Scan(&k.ID, &k.PenggunaID, &k.Nama, &k.Catatan, &k.Status, &k.Dibuat,
		&u.ID, &u.Nama, &u.Email, &u.Dibuat)
	if errors.Is(err, pgx.ErrNoRows) {
		return k, u, ErrNotFound
	}
	k.LaporanAktif = true
	return k, u, err
}
