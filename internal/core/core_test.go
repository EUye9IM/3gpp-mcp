package core

import (
	"path/filepath"
	"testing"

	"github.com/3gpp-mcp/3gpp-mcp/internal/ingest"
	"github.com/3gpp-mcp/3gpp-mcp/internal/model"
	"github.com/3gpp-mcp/3gpp-mcp/internal/store"
)

func setupCore(t *testing.T) (*Core, *store.DB, func()) {
	t.Helper()

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	catStore := store.NewCatalogStore(db, "", "")
	specStore := store.NewSpecStore(db)
	searchStore := store.NewSearchStore(db)

	pipeline := ingest.NewPipeline(specStore, ingest.PipelineConfig{
		DataDir: dir,
		DownloaderCfg: ingest.DownloaderConfig{
			UserAgent: "3gpp-mcp-test",
		},
	})

	core := New(catStore, specStore, searchStore, pipeline)

	return core, db, func() {
		db.Close()
	}
}

func TestCore_ListSpecs(t *testing.T) {
	core, db, cleanup := setupCore(t)
	defer cleanup()

	// Insert test catalog data directly
	tx, _ := db.Driver().Begin()
	tx.Exec("INSERT INTO spec_catalog (id, title, series, wg, version) VALUES ('38.331', 'NR RRC', '38', 'R2', '19.3.0')")
	tx.Exec("INSERT INTO spec_catalog (id, title, series, wg, version) VALUES ('23.501', '5GS Architecture', '23', 'S2', '20.0.0')")
	tx.Commit()

	specs, err := core.ListSpecs("", "")
	if err != nil {
		t.Fatalf("ListSpecs: %v", err)
	}
	if len(specs) != 2 {
		t.Errorf("want 2, got %d", len(specs))
	}

	specs38, _ := core.ListSpecs("38", "")
	if len(specs38) != 1 {
		t.Errorf("series 38: want 1, got %d", len(specs38))
	}

	specsKW, _ := core.ListSpecs("", "RRC")
	if len(specsKW) != 1 {
		t.Errorf("keyword RRC: want 1, got %d", len(specsKW))
	}
}

func TestCore_GetSpecOverview(t *testing.T) {
	core, db, cleanup := setupCore(t)
	defer cleanup()

	// Insert catalog entry
	tx, _ := db.Driver().Begin()
	tx.Exec("INSERT INTO spec_catalog (id, title, series, wg, version) VALUES ('99.999', 'Test Spec', '99', 'X', '1.0.0')")
	tx.Commit()

	// No content cached — should return spec metadata only
	sp, children, err := core.GetSpecOverview("99.999")
	if err != nil {
		t.Fatalf("GetSpecOverview: %v", err)
	}
	if sp == nil {
		t.Fatal("spec not found")
	}
	if sp.ID != "99.999" {
		t.Errorf("spec ID: want 99.999, got %s", sp.ID)
	}
	if children != nil {
		t.Errorf("expected nil children for uncached spec, got %d", len(children))
	}
}

func TestCore_GetSection(t *testing.T) {
	core, db, cleanup := setupCore(t)
	defer cleanup()

	tx, _ := db.Driver().Begin()
	tx.Exec("INSERT INTO spec_catalog (id, title, series, wg, version) VALUES ('99.999', 'Test', '99', 'X', '1.0.0')")
	tx.Commit()

	specStore := store.NewSpecStore(db)
	sections := []model.Section{
		{SectionNumber: "5", ParentNumber: "", Title: "Root", Content: "root content"},
		{SectionNumber: "5.1", ParentNumber: "5", Title: "Child", Content: "child content"},
	}
	specStore.InsertSections("99.999", "Rel-18", sections)

	t.Run("get existing section", func(t *testing.T) {
		sec, children, err := core.GetSection("99.999", "Rel-18", "5")
		if err != nil {
			t.Fatalf("GetSection: %v", err)
		}
		if sec.Title != "Root" {
			t.Errorf("title: want Root, got %q", sec.Title)
		}
		if len(children) != 1 {
			t.Errorf("children: want 1, got %d", len(children))
		}
	})

	t.Run("get leaf section", func(t *testing.T) {
		sec, children, err := core.GetSection("99.999", "Rel-18", "5.1")
		if err != nil {
			t.Fatalf("GetSection: %v", err)
		}
		if sec.Title != "Child" {
			t.Errorf("title: want Child, got %q", sec.Title)
		}
		if len(children) != 0 {
			t.Errorf("leaf children: want 0, got %d", len(children))
		}
	})

	t.Run("spec not in catalog", func(t *testing.T) {
		_, _, err := core.GetSection("88.888", "Rel-18", "1")
		if err == nil {
			t.Error("expected error for unknown spec")
		}
	})
}

func TestCore_SearchInSpec(t *testing.T) {
	core, db, cleanup := setupCore(t)
	defer cleanup()

	tx, _ := db.Driver().Begin()
	tx.Exec("INSERT INTO spec_catalog (id, title, series, wg, version) VALUES ('99.999', 'Test', '99', 'X', '1.0.0')")
	tx.Commit()

	specStore := store.NewSpecStore(db)
	sections := []model.Section{
		{SectionNumber: "1", Title: "Intro", Content: "This is the introduction to the test spec. It describes the overall system architecture and the role of the AMF."},
	}
	specStore.InsertSections("99.999", "Rel-18", sections)

	results, err := core.SearchInSpec("99.999", "Rel-18", "AMF", 10)
	if err != nil {
		t.Fatalf("SearchInSpec: %v", err)
	}
	if len(results) == 0 {
		t.Error("expected results for 'AMF'")
	}

	results2, _ := core.SearchInSpec("99.999", "Rel-18", "missingxyz", 10)
	if len(results2) != 0 {
		t.Error("expected 0 results for 'missingxyz'")
	}
}
