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
- Go version: **1.22** (must stay compatible).
- Use `log/slog` for logging.
- SQLite via `github.com/glebarez/go-sqlite` (CGO-free, includes FTS5).
- Search via SQLite FTS5 (built into go-sqlite, no separate dependency).
- CLI via `github.com/spf13/cobra`.
- MCP via `github.com/mark3labs/mcp-go`.
- No CGO. No external binary dependencies (no libreoffice).
- Spec download via HTTPS (not FTP; 3GPP server requires Referer header).

## Dependency Version Pinning (Go Module Lessons)

**Problem**: Go's module resolution picks the latest compatible version of each transitive dependency. If a newer transitive dep requires a higher Go version, `go mod tidy` upgrades the `go` directive beyond our target (1.22).

**Solution**: 
1. Use `github.com/glebarez/go-sqlite@v1.22.0` (fewer transitive deps, explicitly targets Go 1.22).
2. Pin ALL transitive `modernc.org/*` and `golang.org/x/*` deps to versions compatible with Go 1.22 by adding them as explicit indirect requires in `go.mod`.
3. Use `GOTOOLCHAIN=local go1.22.0 <cmd>` instead of `go <cmd>` to prevent Go from auto-upgrading the toolchain.
4. After any `go get` or new import, run `GOTOOLCHAIN=local go1.22.0 mod tidy` and verify `head -3 go.mod` still shows `go 1.22`.

**Known-good compatible versions** (Go 1.22):
- `github.com/glebarez/go-sqlite v1.22.0`
- `golang.org/x/sys v0.25.0`
- `modernc.org/libc v1.55.2`
- `modernc.org/mathutil v1.6.0`
- `modernc.org/memory v1.8.0`

**If a new dependency breaks the `go 1.22` requirement**:
1. Check `go list -m -json <module>@<version>` to find the minimum Go version.
2. Bisect to find a compatible version: try successively older versions until `GoVersion` is `<= 1.22`.
3. Pin it explicitly in `go.mod` under the `require` block (direct or indirect).
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
