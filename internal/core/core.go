package core

import (
	"fmt"

	"github.com/3gpp-mcp/3gpp-mcp/internal/ingest"
	"github.com/3gpp-mcp/3gpp-mcp/internal/model"
	"github.com/3gpp-mcp/3gpp-mcp/internal/store"
)

// Core provides the business logic shared by CLI and MCP layers.
type Core struct {
	catalogStore *store.CatalogStore
	specStore    *store.SpecStore
	searchStore  *store.SearchStore
	pipeline     *ingest.Pipeline
}

// New creates a new Core.
func New(catalogStore *store.CatalogStore, specStore *store.SpecStore, searchStore *store.SearchStore, pipeline *ingest.Pipeline) *Core {
	return &Core{
		catalogStore: catalogStore,
		specStore:    specStore,
		searchStore:  searchStore,
		pipeline:     pipeline,
	}
}

// ListSpecs returns specs from the catalog, optionally filtered by series and keyword.
func (c *Core) ListSpecs(series, keyword string) ([]model.Spec, error) {
	return c.catalogStore.List(series, keyword)
}

// GetSpecOverview returns a spec's metadata plus its top-level section directory.
func (c *Core) GetSpecOverview(specID string) (*model.Spec, []model.Section, error) {
	sp, err := c.catalogStore.Get(specID)
	if err != nil {
		return nil, nil, err
	}
	if sp == nil {
		return nil, nil, fmt.Errorf("spec %s not found in catalog", specID)
	}

	children, err := c.specStore.ListChildSections(specID, "", "")
	if err != nil {
		if err == store.ErrNotCached {
			// Content not cached yet — return metadata only
			return sp, nil, nil
		}
		return nil, nil, err
	}

	return sp, children, nil
}

// GetSection returns a single section by spec, release, and section number.
// If the spec has not been cached, it transparently triggers download and parsing.
func (c *Core) GetSection(specID, release, sectionNumber string) (*model.Section, []model.Section, error) {
	sp, err := c.catalogStore.Get(specID)
	if err != nil {
		return nil, nil, err
	}
	if sp == nil {
		return nil, nil, fmt.Errorf("spec %s not found in catalog", specID)
	}

	sec, children, err := c.tryGetSection(specID, release, sectionNumber)
	if err == store.ErrNotCached {
		if err := c.pipeline.Run(specID, release); err != nil {
			return nil, nil, fmt.Errorf("ingest %s/%s: %w", specID, release, err)
		}
		sec, children, err = c.tryGetSection(specID, release, sectionNumber)
	}
	if err != nil {
		return nil, nil, err
	}

	return sec, children, nil
}

func (c *Core) tryGetSection(specID, release, sectionNumber string) (*model.Section, []model.Section, error) {
	sec, err := c.specStore.GetSection(specID, release, sectionNumber)
	if err != nil {
		return nil, nil, err
	}

	children, err := c.specStore.ListChildSections(specID, release, sectionNumber)
	if err != nil {
		return nil, nil, err
	}

	return sec, children, nil
}

// SearchInSpec performs a full-text search within a spec.
// If the spec has not been cached, it transparently triggers download and parsing,
// then retries the search.
func (c *Core) SearchInSpec(specID, release, query string, limit int) ([]model.SearchResult, error) {
	results, err := c.searchStore.SearchInSpec(specID, release, query, limit)
	if err != nil {
		return nil, err
	}

	if len(results) == 0 {
		has, checkErr := c.specStore.HasContent(specID, release)
		if checkErr != nil {
			return nil, checkErr
		}
		if !has {
			// Trigger download and retry search
			if err := c.pipeline.Run(specID, release); err != nil {
				return nil, fmt.Errorf("ingest %s/%s: %w", specID, release, err)
			}
			return c.searchStore.SearchInSpec(specID, release, query, limit)
		}
	}

	return results, nil
}
