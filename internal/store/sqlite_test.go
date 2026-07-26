package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpen_NewDB(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open(): %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		t.Fatalf("ping after open: %v", err)
	}
}

func TestOpen_ReopenExisting(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	db1, err := Open(dbPath)
	if err != nil {
		t.Fatalf("first Open(): %v", err)
	}
	defer db1.Close()

	db2, err := Open(dbPath)
	if err != nil {
		t.Fatalf("second Open(): %v", err)
	}
	defer db2.Close()
}

func TestOpen_CorruptDB(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	// Write garbage data
	if err := os.WriteFile(dbPath, []byte("not a valid sqlite database"), 0644); err != nil {
		t.Fatalf("write garbage: %v", err)
	}

	// Should detect corruption, delete, and recreate
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open() on garbage file: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		t.Fatalf("ping after corruption recovery: %v", err)
	}
}

func TestMigrations_CreatesTables(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open(): %v", err)
	}
	defer db.Close()

	sqlDB := db.Driver()

	// Verify catalog table exists and has expected columns
	rows, err := sqlDB.Query("SELECT id, title, series, wg, version FROM spec_catalog LIMIT 0")
	if err != nil {
		t.Errorf("spec_catalog table not found: %v", err)
	}
	rows.Close()

	// Verify sections table exists
	rows2, err := sqlDB.Query("SELECT spec_id, release, section_number, parent_number, title, content FROM sections LIMIT 0")
	if err != nil {
		t.Errorf("sections table not found: %v", err)
	}
	rows2.Close()

	// Verify schema version
	var v int
	if err := sqlDB.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_version").Scan(&v); err != nil {
		t.Fatalf("schema_version query: %v", err)
	}
	if v != 1 {
		t.Errorf("schema version: want 1, got %d", v)
	}
}

func TestMigrations_Idempotent(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	// Open twice — second open should skip migrations
	db1, err := Open(dbPath)
	if err != nil {
		t.Fatalf("first Open(): %v", err)
	}
	db1.Close()

	db2, err := Open(dbPath)
	if err != nil {
		t.Fatalf("second Open(): %v", err)
	}
	defer db2.Close()
}
