package store

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/3gpp-mcp/3gpp-mcp/internal/model"
)

// ErrNotCached is returned when a spec has not been downloaded and parsed yet.
var ErrNotCached = errors.New("spec not in local cache")

// SpecStore provides access to spec section content.
type SpecStore struct {
	db *sql.DB
}

// NewSpecStore creates a new SpecStore.
func NewSpecStore(d *DB) *SpecStore {
	return &SpecStore{db: d.Driver()}
}

// HasContent returns true if at least one section exists for the given spec/release.
func (s *SpecStore) HasContent(specID, release string) (bool, error) {
	var n int
	err := s.db.QueryRow(
		"SELECT COUNT(*) FROM sections WHERE spec_id = ? AND release = ? LIMIT 1",
		specID, release,
	).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("has content %s/%s: %w", specID, release, err)
	}
	return n > 0, nil
}

// InsertSections bulk-inserts parsed sections for a spec/release.
// Uses a transaction; existing sections for the same spec/release are replaced.
func (s *SpecStore) InsertSections(specID, release string, sections []model.Section) error {
	if len(sections) == 0 {
		return nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Remove old sections for this spec/release
	if _, err := tx.Exec(
		"DELETE FROM sections WHERE spec_id = ? AND release = ?",
		specID, release,
	); err != nil {
		return fmt.Errorf("delete old sections: %w", err)
	}

	stmt, err := tx.Prepare(`
		INSERT OR REPLACE INTO sections (spec_id, release, section_number, parent_number, title, content)
		VALUES (?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("prepare insert: %w", err)
	}
	defer stmt.Close()

	for i := range sections {
		sec := &sections[i]
		sec.SpecID = specID
		sec.Release = release

		var parentNumber any
		if sec.ParentNumber != "" {
			parentNumber = sec.ParentNumber
		}

		if _, err := stmt.Exec(
			sec.SpecID, sec.Release, sec.SectionNumber,
			parentNumber, sec.Title, sec.Content,
		); err != nil {
			return fmt.Errorf("insert section %s: %w", sec.SectionNumber, err)
		}
	}

	return tx.Commit()
}

// GetSection returns a single section by spec ID, release, and section number.
// Returns ErrNotCached if the spec has no content at all.
func (s *SpecStore) GetSection(specID, release, sectionNumber string) (*model.Section, error) {
	has, err := s.HasContent(specID, release)
	if err != nil {
		return nil, err
	}
	if !has {
		return nil, ErrNotCached
	}

	var parentNumber sql.NullString
	sec := &model.Section{}

	err = s.db.QueryRow(`
		SELECT spec_id, release, section_number, parent_number, title, content
		FROM sections
		WHERE spec_id = ? AND release = ? AND section_number = ?
	`, specID, release, sectionNumber).Scan(
		&sec.SpecID, &sec.Release, &sec.SectionNumber,
		&parentNumber, &sec.Title, &sec.Content,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("section %s not found in %s/%s", sectionNumber, specID, release)
		}
		return nil, fmt.Errorf("get section %s: %w", sectionNumber, err)
	}

	if parentNumber.Valid {
		sec.ParentNumber = parentNumber.String
	}

	return sec, nil
}

// ListChildSections returns immediate child sections under the given parent.
// If parentNumber is empty, returns top-level sections (TOC).
func (s *SpecStore) ListChildSections(specID, release, parentNumber string) ([]model.Section, error) {
	has, err := s.HasContent(specID, release)
	if err != nil {
		return nil, err
	}
	if !has {
		return nil, ErrNotCached
	}

	var rows *sql.Rows
	if parentNumber == "" {
		rows, err = s.db.Query(`
			SELECT spec_id, release, section_number, parent_number, title, content
			FROM sections
			WHERE spec_id = ? AND release = ? AND parent_number IS NULL
			ORDER BY section_number
		`, specID, release)
	} else {
		rows, err = s.db.Query(`
			SELECT spec_id, release, section_number, parent_number, title, content
			FROM sections
			WHERE spec_id = ? AND release = ? AND parent_number = ?
			ORDER BY section_number
		`, specID, release, parentNumber)
	}
	if err != nil {
		return nil, fmt.Errorf("list children: %w", err)
	}
	defer rows.Close()

	var sections []model.Section
	for rows.Next() {
		var sec model.Section
		var pn sql.NullString
		if err := rows.Scan(&sec.SpecID, &sec.Release, &sec.SectionNumber, &pn, &sec.Title, &sec.Content); err != nil {
			return nil, fmt.Errorf("scan section: %w", err)
		}
		if pn.Valid {
			sec.ParentNumber = pn.String
		}
		sections = append(sections, sec)
	}

	return sections, rows.Err()
}

// DeleteContent removes all sections for a spec (all releases).
func (s *SpecStore) DeleteContent(specID string) error {
	_, err := s.db.Exec("DELETE FROM sections WHERE spec_id = ?", specID)
	if err != nil {
		return fmt.Errorf("delete content %s: %w", specID, err)
	}
	return nil
}

// ListCachedSpecs returns all unique spec_id/release pairs that have cached content.
func (s *SpecStore) ListCachedSpecs() ([]struct {
	SpecID  string
	Release string
}, error) {
	rows, err := s.db.Query(`
		SELECT DISTINCT spec_id, release FROM sections ORDER BY spec_id, release
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []struct {
		SpecID  string
		Release string
	}
	for rows.Next() {
		var specID, release string
		if err := rows.Scan(&specID, &release); err != nil {
			return nil, err
		}
		results = append(results, struct {
			SpecID  string
			Release string
		}{specID, release})
	}
	return results, rows.Err()
}
