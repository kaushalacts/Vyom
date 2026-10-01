package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/kaushalacts/vyom/internal/repository"
	"github.com/kaushalacts/vyom/migrations"
	"github.com/kaushalacts/vyom/pkg/models"
	"github.com/kaushalacts/vyom/internal/providererr"
)

// Querier is the common interface for *sql.DB and *sql.Tx.
type Querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// DB is the SQLite implementation of repository.DB.
type DB struct {
	q  Querier
	db *sql.DB // underlying pool for Close()
}

var _ repository.DB = (*DB)(nil)
var _ Querier = (*sql.DB)(nil)
var _ Querier = (*sql.Tx)(nil)

// New opens a SQLite database at path and runs migrations.
func New(path string) (*DB, error) {
	if path == "" {
		path = ":memory:"
	}
	dsn := fmt.Sprintf("file:%s?_fk=1&_busy_timeout=5000&_journal_mode=WAL", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, providererr.Wrap(err, "sqlite", "Open", providererr.KindConfiguration)
	}
	d := &DB{db: db, q: db}
	if err := d.Migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return d, nil
}

func (d *DB) Migrate(ctx context.Context) error {
	if _, err := d.q.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version     INTEGER PRIMARY KEY,
			checksum    TEXT NOT NULL,
			applied_at  TEXT NOT NULL DEFAULT (datetime('now'))
		)
	`); err != nil {
		return providererr.Wrap(err, "sqlite", "Migrate:create_migrations_table", providererr.KindInternal)
	}

	entries, err := fs.ReadDir(migrations.FS, ".")
	if err != nil {
		return providererr.Wrap(err, "sqlite", "Migrate:read_dir", providererr.KindInternal)
	}

	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".up.sql") {
			continue
		}
		version := 0
		fmt.Sscanf(name, "%d_", &version)
		if version == 0 {
			continue
		}

		var exists int
		if err := d.q.QueryRowContext(ctx,
			"SELECT 1 FROM schema_migrations WHERE version = ?", version).Scan(&exists); err == nil {
			continue
		}

		content, err := fs.ReadFile(migrations.FS, name)
		if err != nil {
			return providererr.Wrap(err, "sqlite", "Migrate:read_file", providererr.KindInternal)
		}

		tx, err := d.db.BeginTx(ctx, nil)
		if err != nil {
			return providererr.Wrap(err, "sqlite", "Migrate:begin", providererr.KindInternal)
		}

		if _, err := tx.ExecContext(ctx, string(content)); err != nil {
			_ = tx.Rollback()
			return providererr.Wrap(err, "sqlite", "Migrate:exec", providererr.KindMalformedResponse)
		}

		checksum := fmt.Sprintf("%x", hashBytes(content))
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO schema_migrations (version, checksum) VALUES (?, ?)",
			version, checksum); err != nil {
			_ = tx.Rollback()
			return providererr.Wrap(err, "sqlite", "Migrate:record", providererr.KindInternal)
		}

		if err := tx.Commit(); err != nil {
			return providererr.Wrap(err, "sqlite", "Migrate:commit", providererr.KindInternal)
		}
	}
	return nil
}

func hashBytes(b []byte) uint64 {
	const fnvOffset uint64 = 14695981039346656037
	const fnvPrime uint64 = 1099511628211
	h := fnvOffset
	for _, c := range b {
		h ^= uint64(c)
		h *= fnvPrime
	}
	return h
}

func (d *DB) Close() error {
	if d.db != nil {
		err := d.db.Close()
		d.db = nil
		d.q = nil
		return err
	}
	return nil
}

func (d *DB) Tx(ctx context.Context, fn func(repository.DB) error) error {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return providererr.Wrap(err, "sqlite", "Tx:begin", providererr.KindInternal)
	}
	txDB := &DB{q: tx}
	if err := fn(txDB); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// Delegation to Querier
func (d *DB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return d.q.ExecContext(ctx, query, args...)
}

func (d *DB) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return d.q.QueryContext(ctx, query, args...)
}

func (d *DB) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return d.q.QueryRowContext(ctx, query, args...)
}
