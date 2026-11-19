package store

// The tests in the *_pg_test.go files run against a real PostgreSQL. They are
// skipped unless TENGGAT_UJI_PG=1; the connection comes from the standard PG*
// variables (PGHOST, PGPORT, PGUSER, PGPASSWORD, PGDATABASE, PGSSLMODE). Every
// test gets a schema of its own, dropped when the test ends, so the tests can
// run in parallel and leave the database as they found it.

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// storeUji returns a Store on an empty schema of its own, without migrations.
func storeUji(t *testing.T) *Store {
	t.Helper()
	if os.Getenv("TENGGAT_UJI_PG") != "1" {
		t.Skip("set TENGGAT_UJI_PG=1 and the PG* variables to run against PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatal(err)
	}
	schema := "uji_" + hex.EncodeToString(b[:])

	admin, err := pgx.Connect(ctx, "")
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	_, err = admin.Exec(ctx, "CREATE SCHEMA "+schema)
	admin.Close(ctx)
	if err != nil {
		t.Fatalf("create schema %s: %v", schema, err)
	}

	cfg, err := pgxpool.ParseConfig("")
	if err != nil {
		t.Fatal(err)
	}
	// Only the test's schema: nothing a test does can reach public or another test.
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	cfg.MaxConns = 4
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		admin, err := pgx.Connect(ctx, "")
		if err != nil {
			t.Errorf("drop schema %s: %v", schema, err)
			return
		}
		defer admin.Close(ctx)
		if _, err := admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Errorf("drop schema %s: %v", schema, err)
		}
	})
	return &Store{pool: pool}
}

// storeSiap returns a Store on a schema of its own with every migration applied.
func storeSiap(t *testing.T) *Store {
	t.Helper()
	s := storeUji(t)
	if err := s.Migrate(t.Context()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return s
}

// penggunaUji creates an account with a fresh email.
func penggunaUji(t *testing.T, s *Store, nama string) Pengguna {
	t.Helper()
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatal(err)
	}
	u, err := s.CreateUser(t.Context(), nama, "u"+hex.EncodeToString(b[:])+"@contoh.id", "$argon2id$uji")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	return u
}

// klienUji creates an active client.
func klienUji(t *testing.T, s *Store, userID, nama string) Klien {
	t.Helper()
	k, err := s.CreateKlien(t.Context(), userID, nama, "", "")
	if err != nil {
		t.Fatalf("CreateKlien(%q): %v", nama, err)
	}
	return k
}

// itemUji creates a manual item of the given kind due on a date.
func itemUji(t *testing.T, s *Store, userID, nama, jenis string, due time.Time) Item {
	t.Helper()
	it, err := s.Create(t.Context(), userID, Input{Nama: nama, Jenis: jenis, Kedaluwarsa: due, Sumber: "manual"})
	if err != nil {
		t.Fatalf("Create(%q): %v", nama, err)
	}
	return it
}

// hitung runs a count query.
func hitung(t *testing.T, s *Store, query string, args ...any) int {
	t.Helper()
	var n int
	if err := s.pool.QueryRow(t.Context(), query, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}

// exec runs raw SQL, failing the test on error.
func exec(t *testing.T, s *Store, query string, args ...any) {
	t.Helper()
	if _, err := s.pool.Exec(t.Context(), query, args...); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
}

// kodeSQL returns the SQLSTATE and constraint of a PostgreSQL error, "" otherwise.
func kodeSQL(err error) (code, constraint string) {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code, pgErr.ConstraintName
	}
	return "", ""
}

// mustCheckViolation fails unless err is a check_violation (23514) of the constraint.
func mustCheckViolation(t *testing.T, err error, constraint string) {
	t.Helper()
	code, got := kodeSQL(err)
	if code != "23514" || got != constraint {
		t.Errorf("err = %v (SQLSTATE %q, constraint %q), want check_violation of %s", err, code, got, constraint)
	}
}

// tanggal is a calendar date at midnight UTC, the form dates come back from PostgreSQL in.
func tanggal(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func ptr[T any](v T) *T { return &v }

// sama reports whether two dates or instants are the same moment, nil included.
func sama(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(*b)
}

func str(p *string) string {
	if p == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%q", *p)
}
