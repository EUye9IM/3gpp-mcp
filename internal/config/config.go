package config

import "path/filepath"

// Config holds all configuration for the 3GPP MCP server.
type Config struct {
	DataDir       string // root directory for all data files (default: ./data)
	DynareportURL string // URL for spec catalog scraping
	HTTPUserAgent string // User-Agent for HTTP requests
	ServerAddr    string // SSE server listen address
	Transport     string // MCP transport: "stdio", "sse", or "both"
	HTTPProxy     string // HTTP proxy address (e.g. "http://proxy:8080")
	HTTPSProxy    string // HTTPS proxy address (e.g. "http://proxy:8080")
	NoProxy       string // Comma-separated addresses to bypass proxy
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		DataDir:       "./data",
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

// CacheDir returns the root directory for cached ZIP files.
func (c *Config) CacheDir() string {
	return filepath.Join(c.DataDir, "cache")
}
