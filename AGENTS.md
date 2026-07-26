# AGENTS.md

AI agent instructions for working on the `3gpp-mcp` project.

## Project Overview

3GPP MCP Server — a Go-based tool that provides 3GPP protocol specification knowledge to AI agents via:
- **MCP protocol** (Model Context Protocol, JSON-RPC over stdio/SSE)
- **CLI** (human and AI-scriptable via `--json` flag)

Full design decisions are documented in [DESIGN.md](./DESIGN.md).

## Build & Run

```bash
# Build
go build -o bin/3gpp-mcp ./cmd/3gpp-mcp/

# Run CLI
go run ./cmd/3gpp-mcp/ catalog
go run ./cmd/3gpp-mcp/ spec 38.331 --release Rel-18
go run ./cmd/3gpp-mcp/ download 38.331
go run ./cmd/3gpp-mcp/ download 38.331 -o /path/to/output.docx

# Run MCP server
go run ./cmd/3gpp-mcp/ server --transport stdio
go run ./cmd/3gpp-mcp/ server --transport sse --addr :8080

# Test
go test ./...
```

## Architecture

```
cmd/3gpp-mcp/main.go          # cobra entry point
internal/
  cli/                        # CLI subcommands (catalog, spec, search, server, mgmt)
  mcp/                        # MCP protocol: server, handler, transports (stdio/SSE)
  core/                       # Shared business logic (catalog, spec retrieval, search)
  model/                      # Domain types: Spec, Section, Release, SearchResult
  store/                      # SQLite (metadata + parsed text + FTS5 search)
  ingest/                     # On-demand spec download (FTP) + .docx parsing pipeline
  config/                     # Config struct and defaults
```

## Key Design Decisions

- **Transparent lazy loading**: Agent never sees cache status. Reading a spec auto-triggers download+parse if not cached.
- **Catalog from dynareport**: Catalog scraped from `3gpp.org/dynareport?code=status-report.htm` on first run. No embedded JSON.
- **Pure Go .docx parsing**: 3GPP specs are .docx (Office Open XML). Parse via `archive/zip` + `encoding/xml`. No external binary dependencies.
- **Section storage**: Flat SQLite table with `section_number` + `parent_number` (materialized path).
- **FTS5 full-text search**: SQLite FTS5 virtual table, auto-synced with sections table via triggers.
- **CLI + MCP share the `core/` layer**: `cli/` handles arg parsing and output formatting; `mcp/` handles MCP protocol. Both call into `core/`.
- **Dual output**: `--json` flag on all CLI commands produces structured JSON for AI consumption; default is human-readable ANSI text.
- **Data sources**: HTTPS for directory listing + dynareport (public, with User-Agent). FTP for spec ZIP download (anonymous).

## Code Conventions

- Go standard library preferred; minimize external dependencies.
- Use `log/slog` for logging.
- SQLite via `modernc.org/sqlite` (CGO-free, includes FTS5).
- Search via SQLite FTS5 (built into go-sqlite, no separate dependency).
- CLI via `github.com/spf13/cobra`.
- MCP via `github.com/mark3labs/mcp-go`.
- No CGO. No external binary dependencies (no libreoffice).
- Spec download via HTTPS (not FTP; 3GPP server requires Referer header).
- Error handling: return wrapped errors with context; never panic in library code.
- Tests: table-driven tests in `*_test.go` files alongside source. Use `go test -race` in CI.

## Adding Features

1. Define domain types in `internal/model/`
2. Implement logic in `internal/core/`
3. Add CLI command in `internal/cli/`
4. Wire MCP handler in `internal/mcp/handler.go`
5. Write tests next to the implementation

## Data Model

```
Spec {
    ID       string   // "38.331"
    Title    string   // "NR; Radio Resource Control (RRC); Protocol specification"
    Series   string   // "38"
    WG       string   // "R2"
    Version  string   // "19.3.0"
}

Section {
    SpecID         string   // "38.331"
    Release        string   // "Rel-18"
    SectionNumber  string   // "5.3.7"
    ParentNumber   string   // "5.3" (NULL for top-level)
    Title          string
    Content        string
}

SearchResult {
    SpecID         string
    Release        string
    SectionNumber  string
    SectionTitle   string
    Content        string
}
```
