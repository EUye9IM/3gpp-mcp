package ingest

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDownloader_Integration(t *testing.T) {
	t.Skip("integration test: requires network access to 3gpp.org")

	d := NewDownloader(DownloaderConfig{
		UserAgent: "Mozilla/5.0 (compatible; 3gpp-mcp-test)",
		Timeout:   30 * time.Second,
	})

	dir := t.TempDir()
	path, err := d.Download("38.331", "Rel-18", dir)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat downloaded file: %v", err)
	}
	if info.Size() == 0 {
		t.Error("downloaded file is empty")
	}
	t.Logf("downloaded %s (%d bytes)", filepath.Base(path), info.Size())

	// Verify it's a valid ZIP
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	buf := make([]byte, 4)
	n, err := f.Read(buf)
	if err != nil || n < 4 {
		t.Fatal("failed to read ZIP header")
	}
	if buf[0] != 'P' || buf[1] != 'K' {
		t.Errorf("not a ZIP file: magic bytes %x", buf[:4])
	}
}

func TestExtractZipFilename(t *testing.T) {
	tests := []struct {
		name   string
		html   string
		specID string
		want   string
	}{
		{
			name:   "matches 38.331",
			html:   `<a href="https://www.3gpp.org/ftp/Specs/latest/Rel-18/38_series/38331-ia0.zip">38331-ia0.zip</a>`,
			specID: "38.331",
			want:   "38331-ia0.zip",
		},
		{
			name:   "matches 23.501",
			html:   `<a href="https://www.3gpp.org/ftp/Specs/latest/Rel-18/23_series/23501-ic0.zip">`,
			specID: "23.501",
			want:   "23501-ic0.zip",
		},
		{
			name:   "matches hyphenated spec 38.101-1",
			html:   `<a href="/ftp/Specs/latest/Rel-18/38_series/38101-1-ie0.zip">`,
			specID: "38.101-1",
			want:   "38101-1-ie0.zip",
		},
		{
			name:   "no match",
			html:   `<a href="other.zip">`,
			specID: "99.999",
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := extractZipFilename(tt.html, tt.specID)
			if tt.want == "" {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tt.want {
				t.Errorf("want %q, got %q", tt.want, result)
			}
		})
	}
}
