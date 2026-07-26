package ingest

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/3gpp-mcp/3gpp-mcp/internal/store"
)

// Pipeline orchestrates the spec ingestion flow: download → extract → parse → store.
type Pipeline struct {
	downloader *Downloader
	parser     *DocParser
	specStore  *store.SpecStore
	dataDir    string
}

// PipelineConfig holds configuration for the ingestion pipeline.
type PipelineConfig struct {
	DownloaderCfg DownloaderConfig
	DataDir       string
}

// NewPipeline creates a new ingestion Pipeline.
func NewPipeline(specStore *store.SpecStore, cfg PipelineConfig) *Pipeline {
	return &Pipeline{
		downloader: NewDownloader(cfg.DownloaderCfg),
		parser:     NewDocParser(),
		specStore:  specStore,
		dataDir:    cfg.DataDir,
	}
}

// Run downloads, parses, and stores a 3GPP specification.
func (p *Pipeline) Run(specID, release string) error {
	tempDir, err := os.MkdirTemp("", "3gpp-ingest-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	zipPath, err := p.downloader.Download(specID, release, tempDir)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}

	docxPath, err := p.extractDocx(zipPath, tempDir)
	if err != nil {
		return fmt.Errorf("extract: %w", err)
	}

	result, err := p.parser.Parse(docxPath, specID, release)
	if err != nil {
		return fmt.Errorf("parse: %w", err)
	}

	if len(result.Sections) == 0 {
		return fmt.Errorf("no sections extracted from %s", specID)
	}

	if err := p.specStore.InsertSections(specID, release, result.Sections); err != nil {
		return fmt.Errorf("store sections: %w", err)
	}

	return nil
}

// extractDocx finds and extracts the first .docx file from a ZIP archive.
func (p *Pipeline) extractDocx(zipPath, destDir string) (string, error) {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", fmt.Errorf("open zip: %w", err)
	}
	defer r.Close()

	for _, f := range r.File {
		if filepath.Ext(f.Name) == ".docx" {
			dest := filepath.Join(destDir, filepath.Base(f.Name))
			if err := p.extractFile(f, dest); err != nil {
				return "", err
			}
			return dest, nil
		}
	}

	return "", fmt.Errorf("no .docx file found in %s", filepath.Base(zipPath))
}

func (p *Pipeline) extractFile(f *zip.File, dest string) error {
	r, err := f.Open()
	if err != nil {
		return fmt.Errorf("open %s: %w", f.Name, err)
	}
	defer r.Close()

	w, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("create %s: %w", dest, err)
	}
	defer w.Close()

	if _, err := w.ReadFrom(r); err != nil {
		os.Remove(dest)
		return fmt.Errorf("write %s: %w", dest, err)
	}

	return nil
}

// DownloadDocx downloads the spec ZIP, extracts the .docx, and saves it to destPath.
func (p *Pipeline) DownloadDocx(specID, release, destPath string) (string, error) {
	tempDir, err := os.MkdirTemp("", "3gpp-download-*")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	zipPath, err := p.downloader.Download(specID, release, tempDir)
	if err != nil {
		return "", fmt.Errorf("download: %w", err)
	}

	docxPath, err := p.extractDocx(zipPath, tempDir)
	if err != nil {
		return "", fmt.Errorf("extract: %w", err)
	}

	src, err := os.Open(docxPath)
	if err != nil {
		return "", fmt.Errorf("open source: %w", err)
	}
	defer src.Close()

	dst, err := os.Create(destPath)
	if err != nil {
		return "", fmt.Errorf("create destination: %w", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		os.Remove(destPath)
		return "", fmt.Errorf("copy: %w", err)
	}

	return destPath, nil
}
