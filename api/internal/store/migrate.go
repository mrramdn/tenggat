package store

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// Migrate applies the embedded SQL migrations that have not run yet, in file name
// order, each in its own transaction. An advisory lock keeps two API containers
// starting at the same time from applying the same file twice.
func (s *Store) Migrate(ctx context.Context) error {
	return s.migrateUpTo(ctx, "")
}

// migrateUpTo applies the migrations like Migrate but stops after the one whose
// version (the file name without .sql) is last; "" applies them all. Tests use it
// to build a database as an older release left it.
func (s *Store) migrateUpTo(ctx context.Context, last string) error {
	names, err := fs.Glob(migrationFiles, "migrations/*.sql")
	if err != nil {
		return err
	}
	sort.Strings(names)
	if last != "" {
		end := -1
		for i, name := range names {
			if migrationVersion(name) == last {
				end = i
				break
			}
		}
		if end < 0 {
			return fmt.Errorf("migration %s does not exist", last)
		}
		names = names[:end+1]
	}

	if _, err := s.pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		versi text PRIMARY KEY,
		dijalankan timestamptz NOT NULL DEFAULT now()
	)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	for _, name := range names {
		version := migrationVersion(name)
		body, err := migrationFiles.ReadFile(name)
		if err != nil {
			return err
		}
		err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(84730021)`); err != nil {
				return err
			}
			var done bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE versi = $1)`, version).Scan(&done); err != nil {
				return err
			}
			if done {
				return nil
			}
			if _, err := tx.Exec(ctx, string(body)); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `INSERT INTO schema_migrations (versi) VALUES ($1)`, version)
			return err
		})
		if err != nil {
			return fmt.Errorf("migration %s: %w", version, err)
		}
	}
	return nil
}

// migrationVersion turns "migrations/001_item.sql" into "001_item".
func migrationVersion(name string) string {
	return strings.TrimSuffix(strings.TrimPrefix(name, "migrations/"), ".sql")
}
