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
	s := server.NewMCPServer("3gpp-mcp", "1.0.0")

	registerTools(s, c)
	registerResources(s, c)

	return s
}

func registerTools(s *server.MCPServer, c *core.Core) {
	s.AddTool(mcpgo.NewTool("list_specs",
		mcpgo.WithDescription("List 3GPP specifications. Filter by series number and/or title keyword."),
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
		mcpgo.WithDescription("Full-text search within a 3GPP specification. Uses FTS5 syntax."),
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
		specID := req.Params.Arguments["spec_id"].(string)
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
		specID := params["spec_id"].(string)
		release := params["release"].(string)
		section := params["section"].(string)

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

func marshalResult(v any) (*mcpgo.CallToolResult, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return mcpgo.NewToolResultError(err.Error()), nil
	}
	return &mcpgo.CallToolResult{
		Content: []mcpgo.Content{
			mcpgo.TextContent{Type: "text", Text: string(data)},
		},
	}, nil
}

func releaseFromVersion(version string) string {
	if len(version) >= 2 {
		return "Rel-" + version[:2]
	}
	return ""
}
