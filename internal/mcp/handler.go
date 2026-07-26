package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/3gpp-mcp/3gpp-mcp/internal/core"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// NewServer creates an MCP server backed by the given Core.
func NewServer(c *core.Core) *server.MCPServer {
	s := server.NewMCPServer("3gpp-mcp", "1.0.0",
		server.WithResourceCapabilities(false, true),
	)

	registerTools(s, c)
	registerResources(s, c)

	return s
}

func registerTools(s *server.MCPServer, c *core.Core) {
	s.AddTool(mcpgo.NewTool("list_specs",
		mcpgo.WithDescription("List available 3GPP specifications by series number and/or title keyword. When you don't know which specification defines a concept, use this tool first to find candidate specs. Confirm the spec_id with the user before querying further."),
		mcpgo.WithString("series", mcpgo.Description("Filter by series number (e.g. 38 for TS 38.xxx)")),
		mcpgo.WithString("keyword", mcpgo.Description("Filter specs whose title contains this text")),
	), func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		args := req.GetArguments()
		series := getString(args, "series")
		keyword := getString(args, "keyword")

		specs, err := c.ListSpecs(series, keyword)
		if err != nil {
			return mcpgo.NewToolResultError(err.Error()), nil
		}
		return marshalResult(specs)
	})

	s.AddTool(mcpgo.NewTool("search_spec",
		mcpgo.WithDescription("Full-text search within a 3GPP specification. Uses FTS5 syntax. IMPORTANT: Always confirm the spec_id with the user before calling this tool. Use list_specs first if you are unsure which spec to search."),
		mcpgo.WithString("spec_id", mcpgo.Required(), mcpgo.Description("Specification ID (e.g. 38.331)")),
		mcpgo.WithString("query", mcpgo.Required(), mcpgo.Description("FTS5 search query")),
		mcpgo.WithString("release", mcpgo.Description("Release label (e.g. Rel-18). Defaults to latest.")),
		mcpgo.WithNumber("limit", mcpgo.Description("Maximum results (default: 10)")),
	), func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		args := req.GetArguments()
		specID := getString(args, "spec_id")
		query := getString(args, "query")
		release := getString(args, "release")
		if release == "" {
			sp, _ := c.Catalog().Get(specID)
			if sp != nil {
				release = releaseFromVersion(sp.Version)
			}
			if release == "" {
				release = "Rel-18"
			}
		}

		limit := 10
		if l, ok := args["limit"].(float64); ok {
			limit = int(l)
		}

		results, err := c.SearchInSpec(specID, release, query, limit)
		if err != nil {
			return mcpgo.NewToolResultError(err.Error()), nil
		}
		return marshalResult(results)
	})

	s.AddTool(mcpgo.NewTool("get_section",
		mcpgo.WithDescription("Read a specific section of a 3GPP specification. For non-leaf sections returns the section content plus its immediate child section numbers and titles. Use to navigate the spec table of contents. IMPORTANT: Use search_spec first to discover relevant section numbers, then read the specific section with this tool."),
		mcpgo.WithString("spec_id", mcpgo.Required(), mcpgo.Description("Specification ID (e.g. 38.331)")),
		mcpgo.WithString("section", mcpgo.Description("Section number (e.g. 5.3.7). Omit for top-level table of contents.")),
		mcpgo.WithString("release", mcpgo.Description("Release label (e.g. Rel-18). Defaults to latest.")),
	), func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		args := req.GetArguments()
		specID := getString(args, "spec_id")
		section := getString(args, "section")
		release := getString(args, "release")
		if release == "" {
			sp, _ := c.Catalog().Get(specID)
			if sp != nil {
				release = releaseFromVersion(sp.Version)
			}
			if release == "" {
				release = "Rel-18"
			}
		}

		if section == "" {
			// Return overview / table of contents
			sp, children, err := c.GetSpecOverview(specID)
			if err != nil {
				return mcpgo.NewToolResultError(err.Error()), nil
			}
			return marshalResult(map[string]any{
				"spec":     sp,
				"sections": children,
			})
		}

		sec, children, err := c.GetSection(specID, release, section)
		if err != nil {
			return mcpgo.NewToolResultError(err.Error()), nil
		}
		return marshalResult(map[string]any{
			"section":  sec,
			"children": children,
		})
	})
}

func registerResources(s *server.MCPServer, c *core.Core) {
	s.AddResource(mcpgo.NewResource("3gpp://catalog", "3GPP Specification Catalog",
		mcpgo.WithMIMEType("application/json"),
		mcpgo.WithResourceDescription("All known 3GPP specifications with metadata"),
	), func(ctx context.Context, req mcpgo.ReadResourceRequest) ([]mcpgo.ResourceContents, error) {
		specs, err := c.ListSpecs("", "")
		if err != nil {
			return nil, err
		}
		data, _ := json.Marshal(specs)
		return []mcpgo.ResourceContents{
			mcpgo.TextResourceContents{URI: "3gpp://catalog", MIMEType: "application/json", Text: string(data)},
		}, nil
	})

	s.AddResourceTemplate(mcpgo.NewResourceTemplate("3gpp://specs/{spec_id}", "Specification Overview",
		mcpgo.WithTemplateMIMEType("application/json"),
		mcpgo.WithTemplateDescription("Overview of a 3GPP specification: title, version, and top-level sections"),
	), func(ctx context.Context, req mcpgo.ReadResourceRequest) ([]mcpgo.ResourceContents, error) {
		specID := getResourceArg(req.Params.Arguments, "spec_id")
		sp, children, err := c.GetSpecOverview(specID)
		if err != nil {
			return nil, err
		}
		if sp == nil {
			return nil, fmt.Errorf("spec %s not found", specID)
		}
		data, _ := json.Marshal(map[string]any{
			"spec":     sp,
			"sections": children,
		})
		return []mcpgo.ResourceContents{
			mcpgo.TextResourceContents{URI: req.Params.URI, MIMEType: "application/json", Text: string(data)},
		}, nil
	})

	s.AddResourceTemplate(mcpgo.NewResourceTemplate("3gpp://specs/{spec_id}/{release}/{section}", "Specification Section",
		mcpgo.WithTemplateMIMEType("application/json"),
		mcpgo.WithTemplateDescription("Content of a specific section within a 3GPP specification"),
	), func(ctx context.Context, req mcpgo.ReadResourceRequest) ([]mcpgo.ResourceContents, error) {
		params := req.Params.Arguments
		specID := getResourceArg(params, "spec_id")
		release := getResourceArg(params, "release")
		section := getResourceArg(params, "section")

		sec, children, err := c.GetSection(specID, release, section)
		if err != nil {
			return nil, err
		}
		data, _ := json.Marshal(map[string]any{
			"section":  sec,
			"children": children,
		})
		return []mcpgo.ResourceContents{
			mcpgo.TextResourceContents{URI: req.Params.URI, MIMEType: "application/json", Text: string(data)},
		}, nil
	})
}

func getString(args map[string]any, name string) string {
	if v, ok := args[name]; ok {
		if s, ok := v.(string); ok {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

// getResourceArg safely extracts a string argument from resource template params.
// mcp-go may store template args as []string. Returns the first value if a slice.
func getResourceArg(args map[string]any, name string) string {
	v, ok := args[name]
	if !ok {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case []string:
		if len(val) > 0 {
			return val[0]
		}
	}
	return fmt.Sprint(v)
}

func marshalResult(v any) (*mcpgo.CallToolResult, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return mcpgo.NewToolResultError(err.Error()), nil
	}
	// nil slices marshal to "null"; replace with "[]" for consistent empty results.
	text := string(data)
	if text == "null" {
		text = "[]"
	}
	return &mcpgo.CallToolResult{
		Content: []mcpgo.Content{
			mcpgo.TextContent{Type: "text", Text: text},
		},
	}, nil
}

func releaseFromVersion(version string) string {
	if len(version) >= 2 {
		return "Rel-" + version[:2]
	}
	return ""
}
