package cli

import (
	"flag"
	"fmt"
	"os"

	"github.com/tabladrum/grove-suite/relay/internal/api/mcp"
)

// RunMCP dispatches `relay mcp <subcommand>`.
func RunMCP(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: relay mcp <serve>")
		return 1
	}
	switch args[0] {
	case "serve":
		return cmdMCPServe(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown mcp subcommand: %s\n", args[0])
		return 1
	}
}

// cmdMCPServe runs the stdio MCP server until EOF on stdin.
func cmdMCPServe(args []string) int {
	fs := flag.NewFlagSet("mcp serve", flag.ContinueOnError)
	repo := fs.String("repo", "", "default repository root for tool calls (overridable per call)")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	root := *repo
	if root == "" {
		if wd, err := os.Getwd(); err == nil {
			root = wd
		}
	}
	srv := mcp.NewServer(root, BuildEngine)
	if err := srv.Serve(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "mcp serve:", err)
		return 1
	}
	return 0
}
