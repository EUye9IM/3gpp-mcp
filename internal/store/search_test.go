package store

import (
	"path/filepath"
	"testing"
)

func TestFTS5_SearchInSpec(t *testing.T) {
	db, _ := openTestStoreDB(t)
	defer db.Close()

	sqlDB := db.Driver()

	// Insert test sections
	tx, err := sqlDB.Begin()
	if err != nil {
		t.Fatal(err)
	}

	insertSection := func(specID, release, number, parent, title, content string) {
		var parentVal interface{}
		if parent != "" {
			parentVal = parent
		}
		_, err := tx.Exec(
			"INSERT OR REPLACE INTO sections (spec_id, release, section_number, parent_number, title, content) VALUES (?, ?, ?, ?, ?, ?)",
			specID, release, number, parentVal, title, content,
		)
		if err != nil {
			t.Fatalf("insert section %s: %v", number, err)
		}
	}

	insertSection("38.331", "Rel-18", "5.3.7", "5.3", "RRC Reestablishment", "The purpose of the RRC reestablishment procedure is to re-establish the RRC connection. A UE in state RRC_CONNECTED may initiate this procedure. The network processes the request and may send an RRCReestablishment message.")
	insertSection("38.331", "Rel-18", "5.3.3", "5.3", "RRC Connection Establishment", "The RRC connection establishment procedure is used to establish an RRC connection. This involves the UE sending an RRCSetupRequest to the network and the network responding with RRCSetup.")
	insertSection("23.501", "Rel-18", "5.6", "", "Network Slicing", "The 5G System enables network slicing. A network slice is a logical network that provides specific network capabilities. The AMF selects the appropriate network slice for each UE.")
	insertSection("23.501", "Rel-18", "5.7", "", "Session Management", "The SMF is responsible for PDU session management. The SMF interacts with the UPF for user plane traffic handling.")

	tx.Commit()

	searchStore := NewSearchStore(db)

	t.Run("search within single spec", func(t *testing.T) {
		results, err := searchStore.SearchInSpec("38.331", "Rel-18", "RRC reestablishment", 10)
		if err != nil {
			t.Fatalf("SearchInSpec: %v", err)
		}
		if len(results) == 0 {
			t.Fatal("expected results for RRC reestablishment")
		}
		if results[0].SpecID != "38.331" {
			t.Errorf("expected 38.331, got %s", results[0].SpecID)
		}
		if results[0].SectionNumber != "5.3.7" {
			t.Errorf("expected 5.3.7, got %s", results[0].SectionNumber)
		}
	})

	t.Run("search across specs", func(t *testing.T) {
		results, err := searchStore.SearchInSpec("", "", "network slice", 10)
		if err != nil {
			t.Fatalf("SearchInSpec: %v", err)
		}
		if len(results) == 0 {
			t.Fatal("expected results for network slice")
		}
		if results[0].SpecID != "23.501" {
			t.Errorf("expected 23.501, got %s", results[0].SpecID)
		}
	})

	t.Run("search no match", func(t *testing.T) {
		results, err := searchStore.SearchInSpec("38.331", "Rel-18", "xyzmissing", 10)
		if err != nil {
			t.Fatalf("SearchInSpec: %v", err)
		}
		if len(results) != 0 {
			t.Errorf("expected 0 results, got %d", len(results))
		}
	})

	t.Run("title-only search", func(t *testing.T) {
		results, err := searchStore.SearchInSpec("", "", "title:RRC", 10)
		if err != nil {
			t.Fatalf("SearchInSpec: %v", err)
		}
		for _, r := range results {
			if r.SpecID != "38.331" {
				t.Errorf("title search should only match 38.331, got %s", r.SpecID)
			}
		}
	})
}

func TestFTS5_TriggerSync(t *testing.T) {
	db, _ := openTestStoreDB(t)
	defer db.Close()

	sqlDB := db.Driver()
	searchStore := NewSearchStore(db)

	// INSERT → FTS5 should sync automatically
	sqlDB.Exec(`
		INSERT OR REPLACE INTO sections (spec_id, release, section_number, parent_number, title, content)
		VALUES ('99.999', 'Rel-18', '1', NULL, 'Test', 'temporary test content')
	`)

	results, err := searchStore.SearchInSpec("99.999", "Rel-18", "temporary", 10)
	if err != nil {
		t.Fatalf("search after insert: %v", err)
	}
	if len(results) == 0 {
		t.Error("FTS5 trigger failed: INSERT not synced")
	}

	// DELETE → FTS5 should sync via trigger
	sqlDB.Exec("DELETE FROM sections WHERE spec_id = '99.999'")

	results, err = searchStore.SearchInSpec("99.999", "Rel-18", "temporary", 10)
	if err != nil {
		t.Fatalf("search after delete: %v", err)
	}
	if len(results) != 0 {
		t.Error("FTS5 trigger failed: DELETE not synced")
	}
}

func TestSanitizeFTS5Query(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"RRC reestablishment", "RRC reestablishment"},
		{"AMF AND SMF", "AMF AND SMF"},
		{"query OR", "query"},
		{"test AND", "test"},
		{"test NOT", "test"},
		{`"quoted phrase"`, " quoted phrase "},
		{"  whitespace  ", "whitespace"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := sanitizeFTS5Query(tt.input)
			if got != tt.want {
				t.Errorf("want %q, got %q", tt.want, got)
			}
		})
	}
}

func TestFTS5TableExists(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open(): %v", err)
	}
	defer db.Close()

	sqlDB := db.Driver()

	// Verify FTS5 virtual table exists
	var name string
	if err := sqlDB.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='sections_fts'").Scan(&name); err != nil {
		t.Fatalf("sections_fts not found: %v", err)
	}

	// Verify triggers exist
	for _, trigger := range []string{"sections_ai", "sections_ad", "sections_au"} {
		var n string
		if err := sqlDB.QueryRow("SELECT name FROM sqlite_master WHERE type='trigger' AND name=?", trigger).Scan(&n); err != nil {
			t.Errorf("trigger %s not found: %v", trigger, err)
		}
	}
}
