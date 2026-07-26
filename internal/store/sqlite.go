package store

import (
	"database/sql"
	"fmt"
	"os"
	"strings"

	_ "github.com/glebarez/go-sqlite"
)

const (
	schemaVersion = 1
)

type DB struct {
	db     *sql.DB
	dbPath string
}

// Open opens the SQLite database at the given path, runs migrations,
// and verifies integrity. If the database is corrupt, it is deleted
// and recreated from scratch.
func Open(dbPath string) (*DB, error) {
	db, err := openDB(dbPath)
	if err != nil {
		return nil, err
	}

	store := &DB{db: db, dbPath: dbPath}

	if err := store.ensureIntegrity(); err != nil {
		db.Close()
		if removeErr := store.rebuild(); removeErr != nil {
			return nil, fmt.Errorf("rebuild after integrity failure: %w (%w)", removeErr, err)
		}
		return Open(dbPath)
	}

	if err := store.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return store, nil
}

func openDB(dbPath string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	return db, nil
}

// Close closes the database connection.
func (d *DB) Close() error {
	return d.db.Close()
}

// Driver exposes the underlying *sql.DB for use by other store packages.
func (d *DB) Driver() *sql.DB {
	return d.db
}

// Ping verifies the database connection is alive.
func (d *DB) Ping() error {
	return d.db.Ping()
}

func (d *DB) rebuild() error {
	d.db.Close()
	if err := os.Remove(d.dbPath); err != nil {
		return fmt.Errorf("remove corrupt db: %w", err)
	}
	var err error
	d.db, err = openDB(d.dbPath)
	return err
}

func (d *DB) ensureIntegrity() error {
	var ok string
	err := d.db.QueryRow("PRAGMA integrity_check").Scan(&ok)
	if err != nil {
		if strings.Contains(err.Error(), "not a database") ||
			strings.Contains(err.Error(), "malformed") ||
			strings.Contains(err.Error(), "file is not a database") {
			return fmt.Errorf("corrupt database: %w", err)
		}
		return fmt.Errorf("integrity check: %w", err)
	}
	if ok != "ok" {
		return fmt.Errorf("integrity check returned: %s", ok)
	}
	return nil
}

func (d *DB) migrate() error {
	if _, err := d.db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_version (
			version INTEGER NOT NULL
		)
	`); err != nil {
		return err
	}

	var currentVersion int
	err := d.db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_version").Scan(&currentVersion)
	if err != nil {
		return err
	}

	if currentVersion >= schemaVersion {
		return nil
	}

	return d.runMigrations(currentVersion)
}

func (d *DB) runMigrations(from int) error {
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for v := from + 1; v <= schemaVersion; v++ {
		if err := d.applyMigration(tx, v); err != nil {
			return fmt.Errorf("migration %d: %w", v, err)
		}
	}

	return tx.Commit()
}

func (d *DB) applyMigration(tx *sql.Tx, version int) error {
	switch version {
	case 1:
		return d.migrationV1(tx)
	default:
		return fmt.Errorf("unknown migration version %d", version)
	}
}

func (d *DB) migrationV1(tx *sql.Tx) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS spec_catalog (
			id          TEXT PRIMARY KEY,
			title       TEXT NOT NULL,
			series      TEXT NOT NULL,
			wg          TEXT NOT NULL,
			version     TEXT NOT NULL
		)`,

		`CREATE TABLE IF NOT EXISTS sections (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			spec_id         TEXT NOT NULL,
			release         TEXT NOT NULL,
			section_number  TEXT NOT NULL,
			parent_number   TEXT,
			title           TEXT NOT NULL,
			content         TEXT NOT NULL,
			UNIQUE(spec_id, release, section_number)
		)`,

		`CREATE INDEX IF NOT EXISTS idx_sections_parent
		 ON sections(spec_id, release, parent_number)`,

		`CREATE INDEX IF NOT EXISTS idx_sections_spec_release
		 ON sections(spec_id, release)`,

		`INSERT INTO schema_version (version) VALUES (1)`,
	}

	for _, stmt := range statements {
		if _, err := tx.Exec(stmt); err != nil {
			return err
		}
	}

	return nil
}
