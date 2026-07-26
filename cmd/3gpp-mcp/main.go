package main

import (
	"fmt"
	"os"

	"github.com/3gpp-mcp/3gpp-mcp/internal/config"

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

func catalogCmd() *cobra.Command {
	var series string
	var keyword string

	cmd := &cobra.Command{
		Use:   "catalog",
		Short: "List 3GPP specifications",
		RunE: func(cmd *cobra.Command, args []string) error {
			// TODO: call core.Catalog.List(series, keyword)
			fmt.Fprintln(os.Stderr, "not yet implemented")
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
			// TODO: call core.Spec.GetOverview / GetContent
			_ = specID
			_ = release
			_ = section
			fmt.Fprintln(os.Stderr, "not yet implemented")
			return nil
		},
	}

	cmd.Flags().StringVar(&release, "release", "", "release label (e.g. Rel-18)")
	cmd.Flags().StringVarP(&section, "section", "s", "", "section number (e.g. 5.3.7)")

	return cmd
}

func searchCmd() *cobra.Command {
	var release string
	var contextLines int

	cmd := &cobra.Command{
		Use:   "search <spec_id> <query>",
		Short: "Search within a 3GPP specification",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			specID := args[0]
			query := args[1]
			// TODO: call core.Search.SearchSpec(specID, query, release, contextLines)
			_ = specID
			_ = query
			_ = release
			_ = contextLines
			fmt.Fprintln(os.Stderr, "not yet implemented")
			return nil
		},
	}

	cmd.Flags().StringVar(&release, "release", "", "release label")
	cmd.Flags().IntVarP(&contextLines, "context", "c", 0, "context lines around match")

	return cmd
}

func serverCmd() *cobra.Command {
	var transport string
	var addr string

	cmd := &cobra.Command{
		Use:   "server",
		Short: "Start MCP server",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg.Transport = transport
			cfg.ServerAddr = addr
			// TODO: start MCP server
			_ = cfg
			fmt.Fprintln(os.Stderr, "not yet implemented")
			return nil
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
			fmt.Fprintln(os.Stderr, "not yet implemented")
			return nil
		},
	}
}

func cacheStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "cache-status",
		Short: "Show which specs are locally cached",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(os.Stderr, "not yet implemented")
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
			fmt.Fprintln(os.Stderr, "not yet implemented")
			return nil
		},
	}
}

func repairCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "repair",
		Short: "Verify and repair database and indexes",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(os.Stderr, "not yet implemented")
			return nil
		},
	}
}
