package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/3gpp-mcp/3gpp-mcp/internal/core"
	"github.com/3gpp-mcp/3gpp-mcp/internal/ingest"
	"github.com/3gpp-mcp/3gpp-mcp/internal/model"
	"github.com/3gpp-mcp/3gpp-mcp/internal/store"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func setupMCPServer(t *testing.T) (*server.MCPServer, *store.DB, func()) {
	t.Helper()

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	catStore := store.NewCatalogStore(db, "", "")
	specStore := store.NewSpecStore(db)
	searchStore := store.NewSearchStore(db)

	pipeline := ingest.NewPipeline(specStore, ingest.PipelineConfig{
		DataDir: dir,
		DownloaderCfg: ingest.DownloaderConfig{
			UserAgent: "3gpp-mcp-test",
		},
	})

	c := core.New(catStore, specStore, searchStore, pipeline)

	tx, _ := db.Driver().Begin()
	tx.Exec("INSERT INTO spec_catalog (id, title, series, wg, version) VALUES ('38.331', 'NR; Radio Resource Control (RRC)', '38', 'R2', '19.3.0')")
	tx.Exec("INSERT INTO spec_catalog (id, title, series, wg, version) VALUES ('24.501', 'Non-Access-Stratum (NAS) for 5GS', '24', 'C1', '19.1.0')")
	tx.Commit()

	sections := []model.Section{
		{SectionNumber: "4", ParentNumber: "", Title: "General", Content: ""},
		{SectionNumber: "4.1", ParentNumber: "4", Title: "Overview", Content: ""},
		{SectionNumber: "4.1.1", ParentNumber: "4.1", Title: "Scope", Content: "The present document specifies the Radio Resource Control protocol for the NR radio interface."},
		{SectionNumber: "4.1.2", ParentNumber: "4.1", Title: "Architecture", Content: "The RRC protocol operates between the UE and the gNB."},
		{SectionNumber: "5", ParentNumber: "", Title: "Procedures", Content: ""},
		{SectionNumber: "5.1", ParentNumber: "5", Title: "RRC Connection Control", Content: "This procedure governs the establishment and release of RRC connections."},
		{SectionNumber: "5.3", ParentNumber: "5", Title: "RRC Reestablishment", Content: "The AMF is involved in the RRC reestablishment procedure through the NG interface."},
	}
	specStore.InsertSections("38.331", "Rel-19", sections)

	srv := NewServer(c)

	return srv, db, func() {
		db.Close()
	}
}

func callTool(t *testing.T, srv *server.MCPServer, toolName string, args map[string]any) (*mcpgo.CallToolResult, error) {
	t.Helper()

	msg := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      toolName,
			"arguments": args,
		},
	}

	raw, _ := json.Marshal(msg)
	resp := srv.HandleMessage(context.Background(), raw)

	switch r := resp.(type) {
	case mcpgo.JSONRPCResponse:
		if r.Result == nil {
			return nil, fmt.Errorf("nil result")
		}
		resultBytes, _ := json.Marshal(r.Result)
		var cr mcpgo.CallToolResult
		if err := json.Unmarshal(resultBytes, &cr); err != nil {
			return nil, fmt.Errorf("unmarshal result: %w", err)
		}
		if cr.IsError {
			if len(cr.Content) > 0 {
				if tc, ok := cr.Content[0].(mcpgo.TextContent); ok {
					return nil, fmt.Errorf("%s", tc.Text)
				}
			}
			return nil, fmt.Errorf("tool error")
		}
		return &cr, nil
	case mcpgo.JSONRPCError:
		return nil, fmt.Errorf("rpc error: %s", r.Error.Message)
	}
	return nil, fmt.Errorf("unexpected response type: %T", resp)
}

func readResource(t *testing.T, srv *server.MCPServer, uri string) (string, error) {
	t.Helper()

	msg := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "resources/read",
		"params": map[string]any{
			"uri": uri,
		},
	}

	raw, _ := json.Marshal(msg)
	resp := srv.HandleMessage(context.Background(), raw)

	switch r := resp.(type) {
	case mcpgo.JSONRPCResponse:
		resultBytes, _ := json.Marshal(r.Result)
		var wrapper struct {
			Contents []mcpgo.TextResourceContents `json:"contents"`
		}
		if err := json.Unmarshal(resultBytes, &wrapper); err != nil {
			return "", fmt.Errorf("unmarshal: %w", err)
		}
		if len(wrapper.Contents) == 0 {
			return "", fmt.Errorf("no contents in response")
		}
		return wrapper.Contents[0].Text, nil
	case mcpgo.JSONRPCError:
		return "", fmt.Errorf("rpc error: %s", r.Error.Message)
	}
	return "", fmt.Errorf("unexpected response type: %T", resp)
}

func listItems(t *testing.T, srv *server.MCPServer, method string) ([]byte, error) {
	t.Helper()

	msg := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  method,
	}

	raw, _ := json.Marshal(msg)
	resp := srv.HandleMessage(context.Background(), raw)

	switch r := resp.(type) {
	case mcpgo.JSONRPCResponse:
		return json.Marshal(r.Result)
	case mcpgo.JSONRPCError:
		return nil, fmt.Errorf("rpc error: %s", r.Error.Message)
	}
	return nil, fmt.Errorf("unexpected response type: %T", resp)
}

func TestTool_list_specs_all(t *testing.T) {
	srv, _, cleanup := setupMCPServer(t)
	defer cleanup()

	cr, err := callTool(t, srv, "list_specs", map[string]any{})
	if err != nil {
		t.Fatalf("list_specs: %v", err)
	}
	if len(cr.Content) == 0 {
		t.Fatal("no content returned")
	}
	text := cr.Content[0].(mcpgo.TextContent).Text
	if text == "" {
		t.Error("empty text")
	}
	if !json.Valid([]byte(text)) {
		t.Error("not valid JSON")
	}
}

func TestTool_list_specs_by_series(t *testing.T) {
	srv, _, cleanup := setupMCPServer(t)
	defer cleanup()

	cr, err := callTool(t, srv, "list_specs", map[string]any{"series": "38"})
	if err != nil {
		t.Fatalf("list_specs series=38: %v", err)
	}
	text := cr.Content[0].(mcpgo.TextContent).Text
	if !json.Valid([]byte(text)) {
		t.Error("not valid JSON")
	}
}

func TestTool_list_specs_by_keyword(t *testing.T) {
	srv, _, cleanup := setupMCPServer(t)
	defer cleanup()

	cr, err := callTool(t, srv, "list_specs", map[string]any{"keyword": "RRC"})
	if err != nil {
		t.Fatalf("list_specs keyword=RRC: %v", err)
	}
	text := cr.Content[0].(mcpgo.TextContent).Text
	if !json.Valid([]byte(text)) {
		t.Error("not valid JSON")
	}
}

func TestTool_list_specs_no_results(t *testing.T) {
	srv, _, cleanup := setupMCPServer(t)
	defer cleanup()

	cr, err := callTool(t, srv, "list_specs", map[string]any{"keyword": "NONEXISTENT_XYZ_123"})
	if err != nil {
		t.Fatalf("list_specs: %v", err)
	}
	text := cr.Content[0].(mcpgo.TextContent).Text
	if text != "[]" {
		t.Errorf("expected empty JSON array for no results, got %s", text)
	}
}

func TestTool_get_section_overview(t *testing.T) {
	srv, _, cleanup := setupMCPServer(t)
	defer cleanup()

	cr, err := callTool(t, srv, "get_section", map[string]any{
		"spec_id": "38.331",
	})
	if err != nil {
		t.Fatalf("get_section overview: %v", err)
	}
	text := cr.Content[0].(mcpgo.TextContent).Text
	if text == "" {
		t.Error("empty text")
	}
}

func TestTool_get_section_specific(t *testing.T) {
	srv, _, cleanup := setupMCPServer(t)
	defer cleanup()

	cr, err := callTool(t, srv, "get_section", map[string]any{
		"spec_id": "38.331",
		"release": "Rel-19",
		"section": "4.1.1",
	})
	if err != nil {
		t.Fatalf("get_section 4.1.1: %v", err)
	}
	text := cr.Content[0].(mcpgo.TextContent).Text
	if text == "" {
		t.Error("empty text for section 4.1.1")
	}
}

func TestTool_get_section_with_children(t *testing.T) {
	srv, _, cleanup := setupMCPServer(t)
	defer cleanup()

	cr, err := callTool(t, srv, "get_section", map[string]any{
		"spec_id": "38.331",
		"release": "Rel-19",
		"section": "4.1",
	})
	if err != nil {
		t.Fatalf("get_section 4.1: %v", err)
	}
	text := cr.Content[0].(mcpgo.TextContent).Text
	if text == "" {
		t.Error("empty text for section 4.1")
	}
}

func TestTool_get_section_not_found(t *testing.T) {
	srv, _, cleanup := setupMCPServer(t)
	defer cleanup()

	_, err := callTool(t, srv, "get_section", map[string]any{
		"spec_id": "99.999",
	})
	if err == nil {
		t.Error("expected error for unknown spec")
	}
}

func TestTool_search_spec_match(t *testing.T) {
	srv, _, cleanup := setupMCPServer(t)
	defer cleanup()

	cr, err := callTool(t, srv, "search_spec", map[string]any{
		"spec_id": "38.331",
		"release": "Rel-19",
		"query":   "AMF",
	})
	if err != nil {
		t.Fatalf("search_spec AMF: %v", err)
	}
	text := cr.Content[0].(mcpgo.TextContent).Text
	if text == "" || text == "[]" {
		t.Error("expected results for 'AMF' search")
	}
}

func TestTool_search_spec_no_match(t *testing.T) {
	srv, _, cleanup := setupMCPServer(t)
	defer cleanup()

	cr, err := callTool(t, srv, "search_spec", map[string]any{
		"spec_id": "38.331",
		"release": "Rel-19",
		"query":   "NONEXISTENTFOOBARXYZ",
	})
	if err != nil {
		t.Fatalf("search_spec: %v", err)
	}
	text := cr.Content[0].(mcpgo.TextContent).Text
	if text != "[]" {
		t.Errorf("expected empty array, got %s", text)
	}
}

func TestTool_search_spec_default_release(t *testing.T) {
	srv, _, cleanup := setupMCPServer(t)
	defer cleanup()

	cr, err := callTool(t, srv, "search_spec", map[string]any{
		"spec_id": "38.331",
		"query":   "RRC",
	})
	if err != nil {
		t.Fatalf("search_spec default release: %v", err)
	}
	text := cr.Content[0].(mcpgo.TextContent).Text
	if text == "" || text == "[]" {
		t.Error("expected results with default release")
	}
}

func TestResource_catalog(t *testing.T) {
	srv, _, cleanup := setupMCPServer(t)
	defer cleanup()

	text, err := readResource(t, srv, "3gpp://catalog")
	if err != nil {
		t.Fatalf("read catalog: %v", err)
	}
	if text == "" || !json.Valid([]byte(text)) {
		t.Error("invalid catalog response")
	}
}

func TestResource_spec_overview(t *testing.T) {
	srv, _, cleanup := setupMCPServer(t)
	defer cleanup()

	text, err := readResource(t, srv, "3gpp://specs/38.331")
	if err != nil {
		t.Fatalf("read spec overview: %v", err)
	}
	if text == "" || !json.Valid([]byte(text)) {
		t.Error("invalid spec overview response")
	}
}

func TestResource_spec_not_found(t *testing.T) {
	srv, _, cleanup := setupMCPServer(t)
	defer cleanup()

	_, err := readResource(t, srv, "3gpp://specs/99.999")
	if err == nil {
		t.Error("expected error for unknown spec")
	}
}

func TestResource_section(t *testing.T) {
	srv, _, cleanup := setupMCPServer(t)
	defer cleanup()

	text, err := readResource(t, srv, "3gpp://specs/38.331/Rel-19/4.1.1")
	if err != nil {
		t.Fatalf("read section: %v", err)
	}
	if text == "" {
		t.Error("empty section response")
	}
}

func TestListTools(t *testing.T) {
	srv, _, cleanup := setupMCPServer(t)
	defer cleanup()

	data, err := listItems(t, srv, "tools/list")
	if err != nil {
		t.Fatalf("tools/list: %v", err)
	}
	var result struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("unmarshal tools/list: %v", err)
	}
	names := map[string]bool{}
	for _, tool := range result.Tools {
		names[tool.Name] = true
	}
	for _, want := range []string{"list_specs", "search_spec", "get_section"} {
		if !names[want] {
			t.Errorf("missing tool: %s", want)
		}
	}
}

func TestListResources(t *testing.T) {
	srv, _, cleanup := setupMCPServer(t)
	defer cleanup()

	data, err := listItems(t, srv, "resources/list")
	if err != nil {
		t.Fatalf("resources/list: %v", err)
	}
	var result struct {
		Resources []struct {
			URI string `json:"uri"`
		} `json:"resources"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("unmarshal resources/list: %v", err)
	}
	uris := map[string]bool{}
	for _, r := range result.Resources {
		uris[r.URI] = true
	}
	if !uris["3gpp://catalog"] {
		t.Error("missing static resource: 3gpp://catalog")
	}
}

func TestListResourceTemplates(t *testing.T) {
	srv, _, cleanup := setupMCPServer(t)
	defer cleanup()

	data, err := listItems(t, srv, "resources/templates/list")
	if err != nil {
		t.Fatalf("resources/templates/list: %v", err)
	}
	var result struct {
		ResourceTemplates []struct {
			URITemplate string `json:"uriTemplate"`
		} `json:"resourceTemplates"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("unmarshal templates: %v", err)
	}
	templates := map[string]bool{}
	for _, rt := range result.ResourceTemplates {
		templates[rt.URITemplate] = true
	}
	for _, want := range []string{
		"3gpp://specs/{spec_id}",
		"3gpp://specs/{spec_id}/{release}/{section}",
	} {
		if !templates[want] {
			t.Errorf("missing resource template: %s", want)
		}
	}
}
