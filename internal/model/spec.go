package model

// Spec represents a 3GPP specification from the catalog.
type Spec struct {
	ID      string `json:"id"`      // "38.331"
	Title   string `json:"title"`   // "NR; Radio Resource Control (RRC); Protocol specification"
	Series  string `json:"series"`  // "38"
	WG      string `json:"wg"`      // "R2"
	Version string `json:"version"` // "19.3.0" (latest version from dynareport)
}

// Section represents a section within a 3GPP specification document.
type Section struct {
	SpecID        string `json:"spec_id"`
	Release       string `json:"release"`
	SectionNumber string `json:"section_number"` // "5.3.7"
	ParentNumber  string `json:"parent_number"`  // "5.3" (empty for top-level)
	Title         string `json:"title"`            // "RRC Reestablishment"
	Content       string `json:"content"`          // full text including intro paragraphs
}

// SearchResult represents a single match from a full-text search within a spec.
type SearchResult struct {
	SpecID        string  `json:"spec_id"`
	Release       string  `json:"release"`
	SectionNumber string  `json:"section_number"`
	SectionTitle  string  `json:"section_title"`
	Content       string  `json:"content"` // matching paragraph text
	Score         float64 `json:"score,omitempty"`
}
