package cli

// `relay tools <install|list|which>` — manage Stage-2 binaries.

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/tabladrum/grove-suite/relay/internal/tools"
)

// RunTools dispatches `relay tools <sub>`.
func RunTools(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: relay tools <install|list|which> [args]")
		return 1
	}
	switch args[0] {
	case "install":
		return cmdToolsInstall(args[1:])
	case "list":
		return cmdToolsList(args[1:])
	case "which":
		return cmdToolsWhich(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown tools subcommand: %s\n", args[0])
		return 1
	}
}

func cmdToolsInstall(args []string) int {
	fs := flag.NewFlagSet("tools install", flag.ContinueOnError)
	only := fs.String("only", "", "comma-separated subset of tools to install (default: all known)")
	root := fs.String("root", "", "override install root (default: $RELAY_TOOLS_ROOT or ~/.relay/tools)")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	inst, err := newInstaller(*root)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	names := selectTools(*only)
	if len(names) == 0 {
		fmt.Fprintln(os.Stderr, "no tools selected")
		return 1
	}

	var failed int
	for _, name := range names {
		fmt.Printf("→ installing %s ... ", name)
		path, err := inst.Install(name)
		if err != nil {
			fmt.Printf("FAILED\n  %v\n", err)
			failed++
			continue
		}
		fmt.Printf("ok\n  binary: %s\n  shim:   %s\n", path, shimPath(inst, name))
	}
	if failed > 0 {
		fmt.Fprintf(os.Stderr, "%d/%d installations failed\n", failed, len(names))
		return 1
	}
	return 0
}

func cmdToolsList(args []string) int {
	fs := flag.NewFlagSet("tools list", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return 1
	}
	printToolsList(os.Stdout)
	return 0
}

func cmdToolsWhich(args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: relay tools which <name>")
		return 1
	}
	name := args[0]
	path := tools.Locate(name)
	if path == "" {
		fmt.Fprintf(os.Stderr, "%s: not found (run `relay tools install`)\n", name)
		return 1
	}
	fmt.Println(path)
	return 0
}

func newInstaller(rootOverride string) (*tools.Installer, error) {
	if rootOverride != "" {
		return &tools.Installer{Root: rootOverride}, nil
	}
	return tools.NewInstaller()
}

func shimPath(inst *tools.Installer, name string) string {
	bn := name
	// `tools.Available`-style platform conditional is internal; the shim file
	// name is "<name>" on POSIX and "<name>.exe" on Windows — keep parity
	// with locate.binaryFileName by deferring to Locate after install.
	if got := tools.Locate(name); got != "" {
		return got
	}
	return fmt.Sprintf("%s/%s", inst.BinDir(), bn)
}

func selectTools(only string) []string {
	if only == "" {
		out := make([]string, 0, len(tools.KnownTools))
		for _, t := range tools.KnownTools {
			out = append(out, t.Name)
		}
		return out
	}
	parts := strings.Split(only, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if !tools.IsKnown(p) {
			fmt.Fprintf(os.Stderr, "warning: %q is not a known tool, skipping\n", p)
			continue
		}
		out = append(out, p)
	}
	return out
}

func printToolsList(w io.Writer) {
	for _, t := range tools.KnownTools {
		status := "missing"
		if tools.Available(t.Name) {
			status = "ok"
		}
		fmt.Fprintf(w, "  %-16s [%s]  %s\n", t.Name, status, t.Description)
	}
}
