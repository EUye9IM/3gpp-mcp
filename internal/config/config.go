package config

import "path/filepath"

// Config holds all configuration for the 3GPP MCP server.
type Config struct {
	DataDir       string // root directory for all data files (default: ./data)
	FTPHost       string // 3GPP FTP server host
	DynareportURL string // URL for spec catalog scraping
	HTTPUserAgent string // User-Agent for HTTP requests
	ServerAddr    string // SSE server listen address
	Transport     string // MCP transport: "stdio", "sse", or "both"
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		DataDir:       "./data",
		FTPHost:       "www.3gpp.org:21",
		DynareportURL: "https://www.3gpp.org/dynareport?code=status-report.htm",
		HTTPUserAgent: "Mozilla/5.0 (compatible; 3gpp-mcp/1.0)",
		ServerAddr:    ":8080",
		Transport:     "stdio",
	}
}

// DBPath returns the full path to the SQLite database file.
func (c *Config) DBPath() string {
	return filepath.Join(c.DataDir, "specs.db")
}

// IndexDir returns the root directory for bleve search indexes.
func (c *Config) IndexDir() string {
	return filepath.Join(c.DataDir, "index")
}

// CacheDir returns the root directory for cached ZIP files.
func (c *Config) CacheDir() string {
	return filepath.Join(c.DataDir, "cache")
}
