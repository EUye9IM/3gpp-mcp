package store

import (
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/3gpp-mcp/3gpp-mcp/internal/model"
)

// CatalogStore manages the spec_catalog table: syncing from dynareport
// and querying spec metadata.
type CatalogStore struct {
	db         *sql.DB
	httpUser   string
	dynaURL    string
	httpClient *http.Client
}

// NewCatalogStore creates a new CatalogStore backed by the given DB.
func NewCatalogStore(d *DB, userAgent, dynareportURL string, httpClient *http.Client) *CatalogStore {
	return &CatalogStore{
		db:         d.Driver(),
		httpUser:   userAgent,
		dynaURL:    dynareportURL,
		httpClient: httpClient,
	}
}

// List returns all specs in the catalog, optionally filtered by series
// and/or title keyword.
func (s *CatalogStore) List(series, keyword string) ([]model.Spec, error) {
	query := "SELECT id, title, series, wg, version FROM spec_catalog WHERE 1=1"
	var args []any

	if series != "" {
		query += " AND series = ?"
		args = append(args, series)
	}
	if keyword != "" {
		query += " AND title LIKE ?"
		args = append(args, "%"+keyword+"%")
	}
	query += " ORDER BY id"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query catalog: %w", err)
	}
	defer rows.Close()

	var specs []model.Spec
	for rows.Next() {
		var sp model.Spec
		if err := rows.Scan(&sp.ID, &sp.Title, &sp.Series, &sp.WG, &sp.Version); err != nil {
			return nil, fmt.Errorf("scan spec: %w", err)
		}
		specs = append(specs, sp)
	}
	return specs, rows.Err()
}

// Get returns a single spec by its ID (e.g. "38.331").
func (s *CatalogStore) Get(specID string) (*model.Spec, error) {
	row := s.db.QueryRow(
		"SELECT id, title, series, wg, version FROM spec_catalog WHERE id = ?",
		specID,
	)
	var sp model.Spec
	if err := row.Scan(&sp.ID, &sp.Title, &sp.Series, &sp.WG, &sp.Version); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get spec %s: %w", specID, err)
	}
	return &sp, nil
}

// Count returns the number of specs in the catalog.
func (s *CatalogStore) Count() (int, error) {
	var n int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM spec_catalog").Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// Sync fetches the dynareport HTML, parses spec entries, and replaces
// the entire catalog with the fresh data. A partial failure leaves the
// database unchanged (wrapped in a transaction).
func (s *CatalogStore) Sync() (int, error) {
	html, err := s.fetchDynareport()
	if err != nil {
		return 0, err
	}

	specs, err := parseDynareport(html)
	if err != nil {
		return 0, err
	}
	if len(specs) == 0 {
		return 0, fmt.Errorf("dynareport returned 0 spec entries")
	}

	if err := s.replaceAll(specs); err != nil {
		return 0, err
	}

	return len(specs), nil
}

func (s *CatalogStore) fetchDynareport() (string, error) {
	req, err := http.NewRequest("GET", s.dynaURL, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", s.httpUser)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch dynareport: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("dynareport: HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read dynareport: %w", err)
	}

	return string(body), nil
}

func (s *CatalogStore) replaceAll(specs []model.Spec) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec("DELETE FROM spec_catalog"); err != nil {
		return fmt.Errorf("clear catalog: %w", err)
	}

	stmt, err := tx.Prepare("INSERT INTO spec_catalog (id, title, series, wg, version) VALUES (?, ?, ?, ?, ?)")
	if err != nil {
		return fmt.Errorf("prepare insert: %w", err)
	}
	defer stmt.Close()

	for i := range specs {
		sp := &specs[i]
		if _, err := stmt.Exec(sp.ID, sp.Title, sp.Series, sp.WG, sp.Version); err != nil {
			return fmt.Errorf("insert %s: %w", sp.ID, err)
		}
	}

	return tx.Commit()
}

// parseDynareport extracts Spec entries from the dynareport HTML page.
//
// Expected HTML structure:
//
//	<tr class="even"> or <tr class="odd">
//	  <td>TS or TR</td>
//	  <td><a href="...">38.331</a></td>
//	  <td>NR; Radio Resource Control (RRC); Protocol specification</td>
//	  <td>19.3.0</td>
//	  <td>R2</td>
//	</tr>
func parseDynareport(html string) ([]model.Spec, error) {
	// Match either "even" or "odd" row classes
	re := regexp.MustCompile(
		`<tr\s+class="(?:even|odd)"[^>]*>\s*` +
			`<td>(TS|TR)</td>\s*` +
			`<td><a\s[^>]*>(\d+\.\d+(?:[-\w.]*)?)</a></td>\s*` +
			`<td>([^<]*)</td>\s*` +
			`<td>(\d+\.\d+\.\d+)</td>\s*` +
			`<td>([^<]+)</td>\s*` +
			`</tr>`,
	)

	matches := re.FindAllStringSubmatch(html, -1)
	if len(matches) == 0 {
		return nil, fmt.Errorf("no spec entries found in dynareport HTML")
	}

	seen := make(map[string]bool)
	var specs []model.Spec

	for _, m := range matches {
		if len(m) != 6 {
			continue
		}
		id := strings.TrimSpace(m[2])
		title := strings.TrimSpace(m[3])
		version := strings.TrimSpace(m[4])
		wg := strings.TrimSpace(m[5])
		if wg == "-" {
			wg = ""
		}
		series := strings.SplitN(id, ".", 2)[0]

		// Deduplicate by spec ID (page may list the same spec multiple times)
		if seen[id] {
			continue
		}
		seen[id] = true

		specs = append(specs, model.Spec{
			ID:      id,
			Title:   title,
			Series:  series,
			WG:      wg,
			Version: version,
		})
	}

	return specs, nil
}
