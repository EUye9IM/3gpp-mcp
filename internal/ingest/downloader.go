package ingest

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/3gpp-mcp/3gpp-mcp/internal/proxy"
)

// Downloader fetches 3GPP spec ZIP files from the 3GPP server.
type Downloader struct {
	httpClient *http.Client
	userAgent  string
	baseURL    string
}

// DownloaderConfig holds configuration for the downloader.
type DownloaderConfig struct {
	UserAgent  string
	Timeout    time.Duration
	HTTPProxy  string
	HTTPSProxy string
	NoProxy    string
}

// NewDownloader creates a new Downloader.
func NewDownloader(cfg DownloaderConfig) *Downloader {
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	return &Downloader{
		httpClient: proxy.NewHTTPClient(cfg.Timeout, cfg.HTTPProxy, cfg.HTTPSProxy, cfg.NoProxy),
		userAgent:  cfg.UserAgent,
		baseURL:    "https://www.3gpp.org/ftp/Specs",
	}
}

// Download fetches a 3GPP spec ZIP file to the given destination directory.
func (d *Downloader) Download(specID, release, destDir string) (string, error) {
	series := strings.SplitN(specID, ".", 2)[0]

	// Try /latest/<release>/<series>_series/ first
	dirURL := fmt.Sprintf("%s/latest/%s/%s_series/", d.baseURL, release, series)
	filename, err := d.findSpecFile(dirURL, specID)
	if err != nil {
		// Try /archive/<series>_series/<specID>/
		dirURL = fmt.Sprintf("%s/archive/%s_series/%s/", d.baseURL, series, specID)
		filename, err = d.findSpecFile(dirURL, specID)
		if err != nil {
			return "", fmt.Errorf("spec %s not found for release %s: %w", specID, release, err)
		}
	}

	fileURL := dirURL + filename
	localPath := filepath.Join(destDir, filename)

	if err := d.downloadFile(fileURL, dirURL, localPath); err != nil {
		return "", err
	}

	return localPath, nil
}

func (d *Downloader) findSpecFile(dirURL, specID string) (string, error) {
	req, err := http.NewRequest("GET", dirURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", d.userAgent)

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("list dir %s: %w", dirURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("list dir %s: HTTP %d", dirURL, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read dir %s: %w", dirURL, err)
	}

	return extractZipFilename(string(body), specID)
}

func extractZipFilename(html, specID string) (string, error) {
	// Build a filename prefix: strip dots, handle hyphens
	// "38.331" → "38331", "38.101-1" → "38101-1"
	prefix := strings.ReplaceAll(specID, ".", "")

	// Match href where the path contains /<prefix>...zip
	re := regexp.MustCompile(`href="[^"]*/` + regexp.QuoteMeta(prefix) + `[^"]*\.zip"`)
	match := re.FindString(html)
	if match == "" {
		return "", fmt.Errorf("no ZIP found for spec %s", specID)
	}

	// Extract filename from href value (may be absolute URL or relative path)
	filename := strings.TrimPrefix(match, `href="`)
	filename = strings.TrimSuffix(filename, `"`)
	// Strip leading path to get just the filename
	if idx := strings.LastIndexByte(filename, '/'); idx >= 0 {
		filename = filename[idx+1:]
	}

	return filename, nil
}

func (d *Downloader) downloadFile(fileURL, refererURL, localPath string) error {
	req, err := http.NewRequest("GET", fileURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", d.userAgent)
	req.Header.Set("Referer", refererURL)

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("download %s: %w", fileURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("download %s: HTTP %d", fileURL, resp.StatusCode)
	}

	f, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("create %s: %w", localPath, err)
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		os.Remove(localPath)
		return fmt.Errorf("write %s: %w", localPath, err)
	}

	return nil
}
