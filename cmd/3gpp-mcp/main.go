package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/3gpp-mcp/3gpp-mcp/internal/config"
	"github.com/3gpp-mcp/3gpp-mcp/internal/core"
	"github.com/3gpp-mcp/3gpp-mcp/internal/ingest"
	"github.com/3gpp-mcp/3gpp-mcp/internal/mcp"
	"github.com/3gpp-mcp/3gpp-mcp/internal/model"
	"github.com/3gpp-mcp/3gpp-mcp/internal/store"
	"github.com/mark3labs/mcp-go/server"

	"github.com/spf13/cobra"
)

var (
	cfg     = config.DefaultConfig()
	jsonOut bool
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "3gpp-mcp",
		Short: "3GPP protocol specification knowledge tool",
		Long:  "3GPP MCP Server — query 3GPP protocol specifications via CLI or MCP protocol.",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return initCore()
		},
	}

	rootCmd.PersistentFlags().BoolVar(&jsonOut, "json", false, "output as JSON")

	rootCmd.AddCommand(
		catalogCmd(),
		specCmd(),
		searchCmd(),
		serverCmd(),
		syncCmd(),
		cacheStatusCmd(),
		cacheClearCmd(),
		repairCmd(),
	)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

var c *core.Core

func initCore() error {
	os.MkdirAll(cfg.DataDir, 0755)

	db, err := store.Open(cfg.DBPath())
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}

	catStore := store.NewCatalogStore(db, cfg.HTTPUserAgent, cfg.DynareportURL)
	specStore := store.NewSpecStore(db)
	searchStore := store.NewSearchStore(db)

	pipeline := ingest.NewPipeline(specStore, ingest.PipelineConfig{
		DataDir: filepath.Join(cfg.DataDir, "cache"),
		DownloaderCfg: ingest.DownloaderConfig{
			UserAgent: cfg.HTTPUserAgent,
		},
	})

	c = core.New(catStore, specStore, searchStore, pipeline)

	// Auto-sync catalog if empty
	n, err := catStore.Count()
	if err != nil {
		slog.Warn("failed to check catalog count", "err", err)
	} else if n == 0 {
		slog.Info("catalog empty, syncing from dynareport...")
		count, syncErr := catStore.Sync()
		if syncErr != nil {
			slog.Warn("failed to sync catalog", "err", syncErr)
		} else {
			slog.Info("catalog synced", "specs", count)
		}
	}

	return nil
}

func output(v any) {
	if jsonOut {
		b, _ := json.MarshalIndent(v, "", "  ")
		fmt.Println(string(b))
		return
	}
	switch val := v.(type) {
	case []model.Spec:
		w := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tTitle\tSeries\tWG\tVersion")
		for _, sp := range val {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", sp.ID, sp.Title, sp.Series, sp.WG, sp.Version)
		}
		w.Flush()
	case []model.Section:
		for _, sec := range val {
			fmt.Printf("  %-12s  %s\n", sec.SectionNumber, sec.Title)
		}
	case []model.SearchResult:
		for _, r := range val {
			fmt.Printf("--- %s §%s [%s] ---\n%s\n\n", r.SpecID, r.SectionNumber, r.SectionTitle, r.Content)
		}
	case int:
		fmt.Println(v)
	default:
		fmt.Printf("%+v\n", v)
	}
}

func catalogCmd() *cobra.Command {
	var series string
	var keyword string

	cmd := &cobra.Command{
		Use:   "catalog",
		Short: "List 3GPP specifications",
		RunE: func(cmd *cobra.Command, args []string) error {
			specs, err := c.ListSpecs(series, keyword)
			if err != nil {
				return err
			}
			output(specs)
			return nil
		},
	}

	cmd.Flags().StringVar(&series, "series", "", "filter by series (e.g. 38)")
	cmd.Flags().StringVar(&keyword, "keyword", "", "filter by title keyword")

	return cmd
}

func specCmd() *cobra.Command {
	var release string
	var section string

	cmd := &cobra.Command{
		Use:   "spec <spec_id>",
		Short: "Read a 3GPP specification",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			specID := args[0]

			if section == "" {
				// Overview
				sp, children, err := c.GetSpecOverview(specID)
				if err != nil {
					return err
				}
				if children != nil {
					if !jsonOut {
						fmt.Printf("%s  %s  (series %s, %s)\n\n", sp.ID, sp.Title, sp.Series, sp.WG)
					}
					output(children)
				} else {
					if !jsonOut {
						fmt.Printf("%s  %s  (series %s, %s)\n[not cached]\n", sp.ID, sp.Title, sp.Series, sp.WG)
					} else {
						output(sp)
					}
				}
				return nil
			}

			// Specific section
			if release == "" {
				sp, _ := c.Catalog().Get(specID)
				if sp != nil {
					release = releaseFromVersion(sp.Version)
				}
				if release == "" {
					release = "Rel-18"
				}
			}

			sec, children, err := c.GetSection(specID, release, section)
			if err != nil {
				return err
			}

			if sec.Content != "" {
				if !jsonOut {
					fmt.Printf("%s §%s  %s\n", specID, sec.SectionNumber, sec.Title)
					fmt.Printf("───\n%s\n", sec.Content)
				}
			}
			if len(children) > 0 {
				if !jsonOut {
					fmt.Printf("\nSubsections:\n")
				}
				output(children)
			}
			if jsonOut {
				output(sec)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&release, "release", "", "release label (e.g. Rel-18)")
	cmd.Flags().StringVarP(&section, "section", "s", "", "section number (e.g. 5.3.7)")

	return cmd
}

func searchCmd() *cobra.Command {
	var release string

	cmd := &cobra.Command{
		Use:   "search <spec_id> <query>",
		Short: "Search within a 3GPP specification",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			specID := args[0]
			query := args[1]

			if release == "" {
				sp, _ := c.Catalog().Get(specID)
				if sp != nil {
					release = releaseFromVersion(sp.Version)
				}
				if release == "" {
					release = "Rel-18"
				}
			}

			results, err := c.SearchInSpec(specID, release, query, 20)
			if err != nil {
				return err
			}
			if len(results) == 0 && !jsonOut {
				fmt.Println("no results found")
				return nil
			}
			output(results)
			return nil
		},
	}

	cmd.Flags().StringVar(&release, "release", "", "release label")

	return cmd
}

func serverCmd() *cobra.Command {
	var transport string
	var addr string

	cmd := &cobra.Command{
		Use:   "server",
		Short: "Start MCP server",
		RunE: func(cmd *cobra.Command, args []string) error {
			srv := mcp.NewServer(c)

			switch transport {
			case "stdio":
				return server.ServeStdio(srv)
			case "sse":
				sse := server.NewSSEServer(srv, server.WithBaseURL("http://"+addr))
				return sse.Start(addr)
			case "both":
				sse := server.NewSSEServer(srv, server.WithBaseURL("http://"+addr))
				go func() {
					if err := sse.Start(addr); err != nil {
						slog.Error("SSE server", "err", err)
					}
				}()
				return server.ServeStdio(srv)
			default:
				return fmt.Errorf("unknown transport: %s", transport)
			}
		},
	}

	cmd.Flags().StringVar(&transport, "transport", "stdio", "transport: stdio, sse, or both")
	cmd.Flags().StringVar(&addr, "addr", ":8080", "SSE listen address")

	return cmd
}

func syncCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sync",
		Short: "Refresh the catalog from 3GPP dynareport",
		RunE: func(cmd *cobra.Command, args []string) error {
			n, err := c.Catalog().Sync()
			if err != nil {
				return err
			}
			fmt.Printf("synced %d specs\n", n)
			return nil
		},
	}
}

func cacheStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "cache-status",
		Short: "Show which specs are locally cached",
		RunE: func(cmd *cobra.Command, args []string) error {
			specs, err := c.CachedSpecs()
			if err != nil {
				return err
			}
			if len(specs) == 0 {
				fmt.Println("no specs cached")
				return nil
			}
			for _, s := range specs {
				fmt.Printf("%s  %s\n", s.SpecID, s.Release)
			}
			return nil
		},
	}
}

func cacheClearCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "cache-clear <spec_id>",
		Short: "Remove cached content for a specification",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			specID := args[0]
			if err := c.DeleteCachedSpec(specID); err != nil {
				return err
			}
			fmt.Printf("cleared cache for %s\n", specID)
			return nil
		},
	}
}

func repairCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "repair",
		Short: "Verify and repair database and indexes",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("repair not yet implemented")
			return nil
		},
	}
}

// releaseFromVersion extracts release label from a version string like "19.3.0" → "Rel-19".
func releaseFromVersion(version string) string {
	if len(version) >= 2 {
		return "Rel-" + version[:2]
	}
	return ""
}
