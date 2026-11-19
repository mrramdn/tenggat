package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// Tugas is a recurring or one-off task. Its due date is a deadline like an
// asset's expiry date.
type Tugas struct {
	ID              string
	PenggunaID      string
	KlienID         *string
	Klien           string // the client's name, "" without a client
	Judul           string
	Catatan         string
	UlangJumlah     int
	UlangSatuan     string
	JatuhTempo      time.Time // a calendar date at midnight UTC
	Biaya           *int64
	MataUang        string
	TerakhirSelesai *time.Time
	SelesaiPada     *time.Time
	Dibuat          time.Time
	Diubah          time.Time
}

// TugasInput holds the fields a client may set on a task.
type TugasInput struct {
	KlienID     *string
	Judul       string
	Catatan     string
	UlangJumlah int
	UlangSatuan string
	JatuhTempo  time.Time
	Biaya       *int64
	MataUang    string
}

// TugasFilter narrows a task listing. Empty fields mean no filter.
type TugasFilter struct {
	KlienID string
	Sampai  *time.Time // only tasks due on or before this date, overdue ones included
	Selesai bool       // false lists active tasks, true lists closed ones
}

const tugasColumns = `t.id, t.pengguna_id, t.klien_id, COALESCE(k.nama, ''), t.judul, t.catatan,
	COALESCE(t.ulang_jumlah, 0), COALESCE(t.ulang_satuan, ''), t.jatuh_tempo, t.biaya, t.mata_uang,
	t.terakhir_selesai, t.selesai_pada, t.dibuat, t.diubah`

// tugasReturning ends "WITH t AS (INSERT/UPDATE ... RETURNING *" like returning does for items.
const tugasReturning = `RETURNING *) SELECT ` + tugasColumns + ` FROM t LEFT JOIN klien AS k ON k.id = t.klien_id`

func scanTugas(row pgx.Row) (Tugas, error) {
	var t Tugas
	err := row.Scan(&t.ID, &t.PenggunaID, &t.KlienID, &t.Klien, &t.Judul, &t.Catatan, &t.UlangJumlah, &t.UlangSatuan,
		&t.JatuhTempo, &t.Biaya, &t.MataUang, &t.TerakhirSelesai, &t.SelesaiPada, &t.Dibuat, &t.Diubah)
	if errors.Is(err, pgx.ErrNoRows) {
		return t, ErrNotFound
	}
	return t, err
}

// ListTugas returns a user's tasks ordered by due date (or by completion date
// newest first when listing closed tasks).
func (s *Store) ListTugas(ctx context.Context, userID string, f TugasFilter) ([]Tugas, error) {
	var klienID *string
	if f.KlienID != "" {
		klienID = &f.KlienID
	}
	rows, err := s.pool.Query(ctx, `SELECT `+tugasColumns+` FROM tugas AS t LEFT JOIN klien AS k ON k.id = t.klien_id
		WHERE t.pengguna_id = $1
		  AND ($2::uuid IS NULL OR t.klien_id = $2)
		  AND ($3::date IS NULL OR t.jatuh_tempo <= $3)
		  AND (t.selesai_pada IS NOT NULL) = $4
		ORDER BY t.selesai_pada DESC, t.jatuh_tempo, t.judul`, userID, klienID, f.Sampai, f.Selesai)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Tugas{}
	for rows.Next() {
		t, err := scanTugas(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// GetTugas returns one of a user's tasks. Another user's task is reported as not found.
func (s *Store) GetTugas(ctx context.Context, userID, id string) (Tugas, error) {
	return scanTugas(s.pool.QueryRow(ctx, `SELECT `+tugasColumns+` FROM tugas AS t LEFT JOIN klien AS k ON k.id = t.klien_id
		WHERE t.id = $1 AND t.pengguna_id = $2`, id, userID))
}

// CreateTugas adds a task.
func (s *Store) CreateTugas(ctx context.Context, userID string, in TugasInput) (Tugas, error) {
	return scanTugas(s.pool.QueryRow(ctx, `WITH t AS (INSERT INTO tugas
		(pengguna_id, klien_id, judul, catatan, ulang_jumlah, ulang_satuan, jatuh_tempo, biaya, mata_uang)
		VALUES ($1, $2, $3, $4, NULLIF($5, 0), NULLIF($6, ''), $7, $8, COALESCE(NULLIF($9, ''), 'IDR'))
		`+tugasReturning, userID, in.KlienID, in.Judul, in.Catatan, in.UlangJumlah, in.UlangSatuan, in.JatuhTempo, in.Biaya, in.MataUang))
}

// UpdateTugas replaces the editable fields of one of a user's active tasks.
func (s *Store) UpdateTugas(ctx context.Context, userID, id string, in TugasInput) (Tugas, error) {
	return scanTugas(s.pool.QueryRow(ctx, `WITH t AS (UPDATE tugas SET
		klien_id = $3, judul = $4, catatan = $5, ulang_jumlah = NULLIF($6, 0), ulang_satuan = NULLIF($7, ''),
		jatuh_tempo = $8, biaya = $9, mata_uang = COALESCE(NULLIF($10, ''), 'IDR'),
		diubah = now()
		WHERE id = $1 AND pengguna_id = $2 AND selesai_pada IS NULL
		`+tugasReturning, id, userID, in.KlienID, in.Judul, in.Catatan, in.UlangJumlah, in.UlangSatuan, in.JatuhTempo, in.Biaya, in.MataUang))
}

// CompleteTugas marks an active task done at a moment and moves its due date
// from one date to the next. A task whose due date is no longer from (a second
// click, or an edit in between) or that is already closed is reported as not
// found, so it never moves twice.
func (s *Store) CompleteTugas(ctx context.Context, userID, id string, from, next, at time.Time) (Tugas, error) {
	return scanTugas(s.pool.QueryRow(ctx, `WITH t AS (UPDATE tugas SET
		jatuh_tempo = $4, terakhir_selesai = $5, diubah = now()
		WHERE id = $1 AND pengguna_id = $2 AND jatuh_tempo = $3 AND selesai_pada IS NULL
		`+tugasReturning, id, userID, from, next, at))
}

// CloseTugas marks a one-off task done at a moment, closing it. A recurring
// task, a task already closed, or one whose due date is no longer from is
// reported as not found.
func (s *Store) CloseTugas(ctx context.Context, userID, id string, from, at time.Time) (Tugas, error) {
	return scanTugas(s.pool.QueryRow(ctx, `WITH t AS (UPDATE tugas SET
		selesai_pada = $4, terakhir_selesai = $4, diubah = now()
		WHERE id = $1 AND pengguna_id = $2 AND jatuh_tempo = $3 AND ulang_jumlah IS NULL AND selesai_pada IS NULL
		`+tugasReturning, id, userID, from, at))
}

// DeleteTugas removes one of a user's tasks.
func (s *Store) DeleteTugas(ctx context.Context, userID, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM tugas WHERE id = $1 AND pengguna_id = $2`, id, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
