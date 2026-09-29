// Package sqlite implements catalog.Catalog on a SQLite database using the
// pure-Go modernc.org/sqlite driver.
package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite" // registers the "sqlite" driver

	"github.com/bjarneo/cliamp/catalog"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// ErrSchemaTooNew means the database was migrated by a newer omatunes than
// this one, so this binary must not write to it.
var ErrSchemaTooNew = errors.New("catalog database schema is newer than this omatunes")

// dsnParams apply to every pooled connection. WAL lets the UI read while a
// sync writes; _txlock=immediate takes the write lock when a transaction
// begins, so a sync never fails upgrading a read lock mid-transaction.
const dsnParams = "_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=on&_synchronous=NORMAL&_txlock=immediate"

// Store is a catalog.Catalog backed by SQLite.
type Store struct {
	db *sql.DB
}

var _ catalog.Catalog = (*Store)(nil)

// Open opens (creating if needed) the catalog at dbPath and applies pending
// migrations. Several processes may open the same new database at once.
func Open(ctx context.Context, dbPath string) (*Store, error) {
	if strings.ContainsAny(dbPath, "?#") {
		return nil, fmt.Errorf("catalog path %q: must not contain ? or #", dbPath)
	}
	// Track URIs can carry stream tokens, so the catalog is private to the
	// user even if its directory already existed with looser permissions.
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create catalog directory: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, fmt.Errorf("secure catalog directory: %w", err)
	}
	db, err := sql.Open("sqlite", dbPath+"?"+dsnParams)
	if err != nil {
		return nil, fmt.Errorf("open catalog %s: %w", dbPath, err)
	}
	if err := migrate(ctx, db, migrationFS); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

type migration struct {
	version int
	name    string
	sql     string
}

// loadMigrations reads migrations/NNN_name.sql from fsys, ordered by NNN.
// Versions must run 1, 2, 3… with no gaps or repeats.
func loadMigrations(fsys fs.FS) ([]migration, error) {
	paths, err := fs.Glob(fsys, "migrations/*.sql")
	if err != nil {
		return nil, fmt.Errorf("list migrations: %w", err)
	}
	var out []migration
	for _, p := range paths {
		name := path.Base(p)
		num, _, ok := strings.Cut(name, "_")
		version, err := strconv.Atoi(num)
		if !ok || err != nil || version < 1 {
			return nil, fmt.Errorf("migration %q: name must be NNN_description.sql", name)
		}
		body, err := fs.ReadFile(fsys, p)
		if err != nil {
			return nil, fmt.Errorf("read migration %s: %w", name, err)
		}
		out = append(out, migration{version: version, name: name, sql: string(body)})
	}
	slices.SortFunc(out, func(a, b migration) int { return a.version - b.version })
	for i, m := range out {
		if m.version != i+1 {
			return nil, fmt.Errorf("migration %s: expected version %d", m.name, i+1)
		}
	}
	return out, nil
}

// migrate applies every migration newer than the database's version, each in
// its own transaction together with its schema_migrations row, so a failing
// migration leaves the database at the previous version. applyMigration
// re-checks the version under the write lock, so a process that loses a race
// to open a new database skips what the winner already applied.
func migrate(ctx context.Context, db *sql.DB, fsys fs.FS) error {
	migrations, err := loadMigrations(fsys)
	if err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version    INTEGER PRIMARY KEY,
		applied_at INTEGER NOT NULL
	)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}
	var current int
	if err := db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&current); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	if current > len(migrations) {
		return fmt.Errorf("%w (database v%d, supported v%d)", ErrSchemaTooNew, current, len(migrations))
	}
	for _, m := range migrations[current:] {
		if err := applyMigration(ctx, db, m); err != nil {
			return err
		}
	}
	return nil
}

func applyMigration(ctx context.Context, db *sql.DB, m migration) error {
	tx, err := db.BeginTx(ctx, nil) // BEGIN IMMEDIATE via _txlock
	if err != nil {
		return fmt.Errorf("migration %s: begin: %w", m.name, err)
	}
	defer tx.Rollback() // no-op after Commit
	var applied bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = ?)`, m.version).Scan(&applied); err != nil {
		return fmt.Errorf("migration %s: check version: %w", m.name, err)
	}
	if applied {
		return nil
	}
	if _, err := tx.ExecContext(ctx, m.sql); err != nil {
		return fmt.Errorf("migration %s: %w", m.name, err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)`,
		m.version, time.Now().UnixMilli()); err != nil {
		return fmt.Errorf("migration %s: record version: %w", m.name, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("migration %s: commit: %w", m.name, err)
	}
	return nil
}
