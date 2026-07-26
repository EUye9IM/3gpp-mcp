package store

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/3gpp-mcp/3gpp-mcp/internal/model"
)

// SearchStore provides full-text search over spec sections using SQLite FTS5.
type SearchStore struct {
	db *sql.DB
}

// NewSearchStore creates a new SearchStore.
func NewSearchStore(d *DB) *SearchStore {
	return &SearchStore{db: d.Driver()}
}

// SearchInSpec performs a full-text search within a specific specification.
func (s *SearchStore) SearchInSpec(specID, release, query string, limit int) ([]model.SearchResult, error) {
	if limit <= 0 {
		limit = 10
	}

	ftsQuery := sanitizeFTS5Query(query)

	var rows *sql.Rows
	var err error

	if specID != "" && release != "" {
		rows, err = s.db.Query(`
			SELECT spec_id, release, section_number, title,
			       snippet(sections_fts, 4, '**', '**', '...', 64) AS snippet
			FROM sections_fts
			WHERE sections_fts MATCH ?
			  AND spec_id = ? AND release = ?
			ORDER BY rank
			LIMIT ?
		`, ftsQuery, specID, release, limit)
	} else if specID != "" {
		rows, err = s.db.Query(`
			SELECT spec_id, release, section_number, title,
			       snippet(sections_fts, 4, '**', '**', '...', 64) AS snippet
			FROM sections_fts
			WHERE sections_fts MATCH ?
			  AND spec_id = ?
			ORDER BY rank
			LIMIT ?
		`, ftsQuery, specID, limit)
	} else {
		rows, err = s.db.Query(`
			SELECT spec_id, release, section_number, title,
			       snippet(sections_fts, 4, '**', '**', '...', 64) AS snippet
			FROM sections_fts
			WHERE sections_fts MATCH ?
			ORDER BY rank
			LIMIT ?
		`, ftsQuery, limit)
	}

	if err != nil {
		return nil, fmt.Errorf("fts5 search: %w", err)
	}
	defer rows.Close()

	var results []model.SearchResult
	for rows.Next() {
		var r model.SearchResult
		if err := rows.Scan(&r.SpecID, &r.Release, &r.SectionNumber, &r.SectionTitle, &r.Content); err != nil {
			return nil, fmt.Errorf("scan search result: %w", err)
		}
		results = append(results, r)
	}

	return results, rows.Err()
}

// sanitizeFTS5Query escapes special FTS5 characters and prepares
// the user input for safe MATCH usage.
func sanitizeFTS5Query(query string) string {
	// Strip leading/trailing whitespace
	q := strings.TrimSpace(query)
	if q == "" {
		return q
	}

	// FTS5 special characters that need quoting: " and '
	// Replace double quotes with space (disallow explicit phrase syntax from user)
	q = strings.ReplaceAll(q, `"`, ` `)

	// Ensure no trailing operator (NOR/NOT/AND/OR) which causes FTS5 errors
	tokens := strings.Fields(q)
	if len(tokens) == 0 {
		return q
	}

	last := strings.ToUpper(tokens[len(tokens)-1])
	if last == "AND" || last == "OR" || last == "NOT" || last == "NOR" {
		q = strings.Join(tokens[:len(tokens)-1], " ")
	}

	return q
}
