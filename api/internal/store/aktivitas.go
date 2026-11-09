package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// ErrAktivitasOtomatis is returned when a user tries to delete an entry the app
// recorded itself; only notes and work entries can be deleted.
var ErrAktivitasOtomatis = errors.New("aktivitas otomatis tidak dapat dihapus")

// Aktivitas is one entry of a client's activity feed: a note or work entry the
// user wrote, or an event the app recorded.
type Aktivitas struct {
	ID          string
	PenggunaID  string
	KlienID     *string
	Klien       string // the client's name, "" without a client
	ItemID      *string
	TugasID     *string
	Jenis       string
	Teks        string
	Tanggal     time.Time // a calendar date at midnight UTC
	DurasiMenit *int
	Data        json.RawMessage // nil when there is none
	Dibuat      time.Time
}

// AktivitasInput is a new entry.
type AktivitasInput struct {
	KlienID     *string
	ItemID      *string
	TugasID     *string
	Jenis       string
	Teks        string
	Tanggal     time.Time
	DurasiMenit *int
	Data        json.RawMessage
	// SekaliSehari skips the entry when the same item already has one of this
	// kind on this date; AddAktivitas then returns ErrNotFound.
	SekaliSehari bool
}

// AktivitasFilter narrows a listing. Empty fields mean no filter.
type AktivitasFilter struct {
	KlienID      string
	SebelumID    string     // only entries older than this one
	SebelumWaktu *time.Time // only entries created before this moment
	Jenis        []string
	Dari, Sampai *time.Time // tanggal bounds, inclusive
	Batas        int        // at most this many; 0 means 50
}

const aktivitasColumns = `a.id, a.pengguna_id, a.klien_id, COALESCE(k.nama, ''), a.item_id, a.tugas_id, a.jenis, a.teks,
	a.tanggal, a.durasi_menit, a.data, a.dibuat`

func scanAktivitas(row pgx.Row) (Aktivitas, error) {
	var a Aktivitas
	var data []byte
	err := row.Scan(&a.ID, &a.PenggunaID, &a.KlienID, &a.Klien, &a.ItemID, &a.TugasID, &a.Jenis, &a.Teks, &a.Tanggal,
		&a.DurasiMenit, &data, &a.Dibuat)
	if errors.Is(err, pgx.ErrNoRows) {
		return a, ErrNotFound
	}
	if data != nil {
		a.Data = json.RawMessage(data)
	}
	return a, err
}

// ListAktivitas returns a user's entries, newest first.
func (s *Store) ListAktivitas(ctx context.Context, userID string, f AktivitasFilter) ([]Aktivitas, error) {
	var klienID, sebelumID *string
	if f.KlienID != "" {
		klienID = &f.KlienID
	}
	if f.SebelumID != "" {
		sebelumID = &f.SebelumID
	}
	var jenis []string
	if len(f.Jenis) > 0 {
		jenis = f.Jenis
	}
	batas := f.Batas
	if batas <= 0 {
		batas = 50
	}
	rows, err := s.pool.Query(ctx, `SELECT `+aktivitasColumns+`
		FROM aktivitas AS a LEFT JOIN klien AS k ON k.id = a.klien_id
		WHERE a.pengguna_id = $1
		  AND ($2::uuid IS NULL OR a.klien_id = $2)
		  AND ($3::uuid IS NULL OR (a.dibuat, a.id) <
		       (SELECT b.dibuat, b.id FROM aktivitas AS b WHERE b.id = $3 AND b.pengguna_id = $1))
		  AND ($4::timestamptz IS NULL OR a.dibuat < $4)
		  AND ($5::text[] IS NULL OR a.jenis = ANY ($5))
		  AND ($6::date IS NULL OR a.tanggal >= $6)
		  AND ($7::date IS NULL OR a.tanggal <= $7)
		ORDER BY a.dibuat DESC, a.id DESC
		LIMIT $8`, userID, klienID, sebelumID, f.SebelumWaktu, jenis, f.Dari, f.Sampai, batas)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Aktivitas{}
	for rows.Next() {
		a, err := scanAktivitas(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// AddAktivitas stores an entry for a user and returns it as stored.
func (s *Store) AddAktivitas(ctx context.Context, userID string, in AktivitasInput) (Aktivitas, error) {
	var data []byte
	if in.Data != nil {
		data = in.Data
	}
	return scanAktivitas(s.pool.QueryRow(ctx, `WITH a AS (INSERT INTO aktivitas
		(pengguna_id, klien_id, item_id, tugas_id, jenis, teks, tanggal, durasi_menit, data)
		SELECT $1::uuid, $2::uuid, $3::uuid, $4::uuid, $5::text, $6::text, $7::date, $8::integer, $9::jsonb
		WHERE NOT ($10::boolean AND EXISTS (SELECT 1 FROM aktivitas
			WHERE item_id = $3 AND jenis = $5 AND tanggal = $7))
		RETURNING *) SELECT `+aktivitasColumns+` FROM a LEFT JOIN klien AS k ON k.id = a.klien_id`,
		userID, in.KlienID, in.ItemID, in.TugasID, in.Jenis, in.Teks, in.Tanggal, in.DurasiMenit, data, in.SekaliSehari))
}

// DeleteAktivitas removes one of a user's notes or work entries. An entry the
// app recorded itself gives ErrAktivitasOtomatis.
func (s *Store) DeleteAktivitas(ctx context.Context, userID, id string) error {
	var jenis string
	err := s.pool.QueryRow(ctx, `SELECT jenis FROM aktivitas WHERE id = $1 AND pengguna_id = $2`, id, userID).Scan(&jenis)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if jenis != "catatan" && jenis != "pekerjaan" {
		return ErrAktivitasOtomatis
	}
	tag, err := s.pool.Exec(ctx, `DELETE FROM aktivitas WHERE id = $1 AND pengguna_id = $2`, id, userID)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}
