package ingest

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"

	"github.com/3gpp-mcp/3gpp-mcp/internal/store"
)

func TestPipeline_Run(t *testing.T) {
	dir := t.TempDir()

	db, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	specStore := store.NewSpecStore(db)

	// Use the already-downloaded 29.508 spec from the project root
	srcZip := "/root/proj/3gpp-mcp/29508-k00.zip"
	if _, err := os.Stat(srcZip); os.IsNotExist(err) {
		t.Skip("test fixture 29508-k00.zip not found in project root")
	}

	// Copy to temp dir to simulate a download
	dstZip := filepath.Join(dir, "29508-k00.zip")
	if err := copyFile(srcZip, dstZip); err != nil {
		t.Fatalf("copy fixture: %v", err)
	}

	// Hack: override the downloader to just return the local file
	// For now, manually run the extract→parse→store steps
	r, err := zip.OpenReader(dstZip)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	// Find the .docx
	var docxPath string
	for _, f := range r.File {
		if filepath.Ext(f.Name) == ".docx" {
			dest := filepath.Join(dir, filepath.Base(f.Name))
			rc, _ := f.Open()
			w, _ := os.Create(dest)
			if _, err := w.ReadFrom(rc); err != nil {
				t.Fatal(err)
			}
			rc.Close()
			w.Close()
			docxPath = dest
			break
		}
	}
	if docxPath == "" {
		t.Fatal("no .docx found in ZIP")
	}

	// Parse
	parser := NewDocParser()
	result, err := parser.Parse(docxPath, "29.508", "Rel-20")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if len(result.Sections) < 50 {
		t.Fatalf("too few sections: %d", len(result.Sections))
	}

	// Store
	if err := specStore.InsertSections(result.SpecID, result.Release, result.Sections); err != nil {
		t.Fatalf("store: %v", err)
	}

	// Verify
	has, _ := specStore.HasContent("29.508", "Rel-20")
	if !has {
		t.Error("content not stored")
	}

	sec, err := specStore.GetSection("29.508", "Rel-20", "4.1.1")
	if err != nil {
		t.Fatalf("get section: %v", err)
	}
	if len(sec.Content) < 100 {
		t.Errorf("section 4.1.1 content too short: %d chars", len(sec.Content))
	}
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}
