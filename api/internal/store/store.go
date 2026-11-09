// Package store keeps items in PostgreSQL with hand-written SQL.
package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mrramdn/tenggat/api/internal/expiry"
)

// ErrNotFound is returned when no item has the requested id.
var ErrNotFound = errors.New("item tidak ditemukan")

// TLSInfo is what the last certificate check read from the server.
type TLSInfo struct {
	Penerbit     string    `json:"penerbit"`
	Subjek       string    `json:"subjek"`
	Alamat       string    `json:"alamat"`
	BerlakuSejak time.Time `json:"berlaku_sejak"`
	Berakhir     time.Time `json:"berakhir"`
	RantaiValid  bool      `json:"rantai_valid"`
	GalatRantai  string    `json:"galat_rantai,omitempty"`
}

// Item is one row of the item table.
type Item struct {
	ID            string
	PenggunaID    string
	Nama          string
	Jenis         string
	Klien         string
	Host          *string
	Kedaluwarsa   time.Time // a calendar date at midnight UTC
	Sumber        string
	Biaya         *int64
	Catatan       string
	InfoTLS       *TLSInfo
	TerakhirDicek *time.Time
	GalatCek      *string
	Dibuat        time.Time
	Diubah        time.Time
}

// Input holds the fields a client may set.
type Input struct {
	Nama        string
	Jenis       string
	Klien       string
	Host        *string
	Kedaluwarsa time.Time
	Sumber      string
	Biaya       *int64
	Catatan     string
	InfoTLS     *TLSInfo
	Dicek       *time.Time // set when the date was just read from the server
}

// Filter narrows a listing. Empty fields mean no filter.
type Filter struct {
	Status expiry.Status
	Klien  string
}

// Summary counts items per status and lists the known clients.
type Summary struct {
	Lewat  int      `json:"lewat"`
	Kritis int      `json:"kritis"`
	Segera int      `json:"segera"`
	Aman   int      `json:"aman"`
	Total  int      `json:"total"`
	Klien  []string `json:"klien"`
}

// Store wraps a connection pool.
type Store struct {
	pool *pgxpool.Pool
}

// Open connects using the standard PG* environment variables (PGHOST, PGUSER,
// PGPASSWORD, PGDATABASE, ...), so the password never has to be URL-encoded.
func Open(ctx context.Context) (*Store, error) {
	cfg, err := pgxpool.ParseConfig("")
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = 8
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return &Store{pool: pool}, nil
}

// Close releases the pool.
func (s *Store) Close() { s.pool.Close() }

// Ping checks that the database answers.
func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

const columns = `id, pengguna_id, nama, jenis, klien, host, kedaluwarsa, sumber, biaya, catatan,
	info_tls, terakhir_dicek, galat_cek, dibuat, diubah`

func scanItem(row pgx.Row) (Item, error) {
	var it Item
	err := row.Scan(&it.ID, &it.PenggunaID, &it.Nama, &it.Jenis, &it.Klien, &it.Host, &it.Kedaluwarsa, &it.Sumber,
		&it.Biaya, &it.Catatan, &it.InfoTLS, &it.TerakhirDicek, &it.GalatCek, &it.Dibuat, &it.Diubah)
	if errors.Is(err, pgx.ErrNoRows) {
		return it, ErrNotFound
	}
	return it, err
}

// statusRange returns the inclusive date bounds of a status relative to today.
func statusRange(st expiry.Status, today time.Time) (from, to *time.Time) {
	day := func(n int) *time.Time { d := today.AddDate(0, 0, n); return &d }
	switch st {
	case expiry.Lewat:
		return nil, day(-1)
	case expiry.Kritis:
		return day(0), day(7)
	case expiry.Segera:
		return day(8), day(30)
	case expiry.Aman:
		return day(31), nil
	}
	return nil, nil
}

// List returns one user's items ordered by the nearest expiry date.
func (s *Store) List(ctx context.Context, userID string, f Filter, now time.Time) ([]Item, error) {
	from, to := statusRange(f.Status, expiry.DateOf(now))
	var klien *string
	if f.Klien != "" {
		klien = &f.Klien
	}
	rows, err := s.pool.Query(ctx, `SELECT `+columns+` FROM item
		WHERE pengguna_id = $4
		  AND ($1::date IS NULL OR kedaluwarsa >= $1)
		  AND ($2::date IS NULL OR kedaluwarsa <= $2)
		  AND ($3::text IS NULL OR klien = $3)
		ORDER BY kedaluwarsa, nama`, from, to, klien, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Item{}
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

// Get returns one of a user's items. Another user's item is reported as not found.
func (s *Store) Get(ctx context.Context, userID, id string) (Item, error) {
	return scanItem(s.pool.QueryRow(ctx, `SELECT `+columns+` FROM item WHERE id = $1 AND pengguna_id = $2`, id, userID))
}

// Create inserts an item for a user and returns it as stored.
func (s *Store) Create(ctx context.Context, userID string, in Input) (Item, error) {
	return scanItem(s.pool.QueryRow(ctx, `INSERT INTO item
		(pengguna_id, nama, jenis, klien, host, kedaluwarsa, sumber, biaya, catatan, info_tls, terakhir_dicek)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING `+columns,
		userID, in.Nama, in.Jenis, in.Klien, in.Host, in.Kedaluwarsa, in.Sumber, in.Biaya, in.Catatan, in.InfoTLS, in.Dicek))
}

// Update replaces the editable fields of one of a user's items.
func (s *Store) Update(ctx context.Context, userID, id string, in Input) (Item, error) {
	return scanItem(s.pool.QueryRow(ctx, `UPDATE item SET
		nama = $2, jenis = $3, klien = $4, host = $5, kedaluwarsa = $6, sumber = $7, biaya = $8,
		catatan = $9, info_tls = $10,
		terakhir_dicek = COALESCE($11, terakhir_dicek),
		galat_cek = CASE WHEN $11::timestamptz IS NULL THEN galat_cek END,
		diubah = now()
		WHERE id = $1 AND pengguna_id = $12
		RETURNING `+columns,
		id, in.Nama, in.Jenis, in.Klien, in.Host, in.Kedaluwarsa, in.Sumber, in.Biaya, in.Catatan, in.InfoTLS, in.Dicek, userID))
}

// Delete removes one of a user's items.
func (s *Store) Delete(ctx context.Context, userID, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM item WHERE id = $1 AND pengguna_id = $2`, id, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// SaveCheck stores a successful certificate check.
func (s *Store) SaveCheck(ctx context.Context, id string, date time.Time, info TLSInfo, at time.Time) (Item, error) {
	return scanItem(s.pool.QueryRow(ctx, `UPDATE item SET
		kedaluwarsa = $2, info_tls = $3, terakhir_dicek = $4, galat_cek = NULL, diubah = now()
		WHERE id = $1 AND jenis = 'sertifikat'
		RETURNING `+columns, id, date, info, at))
}

// SaveCheckError records a failed certificate check and keeps the last known date.
func (s *Store) SaveCheckError(ctx context.Context, id, message string, at time.Time) (Item, error) {
	return scanItem(s.pool.QueryRow(ctx, `UPDATE item SET terakhir_dicek = $2, galat_cek = $3
		WHERE id = $1 AND jenis = 'sertifikat'
		RETURNING `+columns, id, at, message))
}

// DueForCheck lists certificates not checked since the given time.
func (s *Store) DueForCheck(ctx context.Context, before time.Time) ([]Item, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+columns+` FROM item
		WHERE jenis = 'sertifikat' AND (terakhir_dicek IS NULL OR terakhir_dicek < $1)
		ORDER BY terakhir_dicek NULLS FIRST`, before)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []Item
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

// Summary counts a user's items per status for today and lists the clients in use.
func (s *Store) Summary(ctx context.Context, userID string, now time.Time) (Summary, error) {
	var sum Summary
	today := expiry.DateOf(now)
	err := s.pool.QueryRow(ctx, `SELECT
		count(*) FILTER (WHERE kedaluwarsa < $1::date),
		count(*) FILTER (WHERE kedaluwarsa BETWEEN $1::date AND $1::date + 7),
		count(*) FILTER (WHERE kedaluwarsa BETWEEN $1::date + 8 AND $1::date + 30),
		count(*) FILTER (WHERE kedaluwarsa > $1::date + 30),
		count(*)
		FROM item WHERE pengguna_id = $2`, today, userID).Scan(&sum.Lewat, &sum.Kritis, &sum.Segera, &sum.Aman, &sum.Total)
	if err != nil {
		return sum, fmt.Errorf("count: %w", err)
	}
	rows, err := s.pool.Query(ctx, `SELECT DISTINCT klien FROM item WHERE pengguna_id = $1 AND klien <> '' ORDER BY klien`, userID)
	if err != nil {
		return sum, err
	}
	sum.Klien, err = pgx.CollectRows(rows, pgx.RowTo[string])
	if sum.Klien == nil {
		sum.Klien = []string{}
	}
	return sum, err
}
