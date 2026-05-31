// Package cli — hook + outbox subcommands (MVP-L8).
//
// `relay hook install [--force]` writes the pre-push hook.
// `relay hook uninstall` removes the relay-managed hook.
// `relay outbox push --intent-store=<path> [--batch=N]` replays new
// certificates from the engine store into a local intent-store git repo.
package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/tabladrum/grove-suite/relay/internal/enginestore"
	"github.com/tabladrum/grove-suite/relay/internal/githook"
	"github.com/tabladrum/grove-suite/relay/internal/outbox"
)

// RunHook dispatches `relay hook <install|uninstall>`.
func RunHook(args []string) int {
	return runHookWith(args, os.Stdout, os.Stderr)
}

func runHookWith(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: relay hook <install|uninstall> [--repo <path>] [--force]")
		return 1
	}
	sub := args[0]
	fs := flag.NewFlagSet("hook "+sub, flag.ContinueOnError)
	repo := fs.String("repo", ".", "path to git repository")
	force := fs.Bool("force", false, "overwrite a non-managed pre-push hook (install only)")
	fs.SetOutput(stderr)
	if err := fs.Parse(args[1:]); err != nil {
		return 1
	}
	abs, err := filepath.Abs(*repo)
	if err != nil {
		fmt.Fprintln(stderr, "hook:", err)
		return 1
	}
	switch sub {
	case "install":
		path, err := githook.Install(abs, *force)
		if err != nil {
			fmt.Fprintln(stderr, "hook install:", err)
			return 1
		}
		fmt.Fprintln(stdout, "installed:", path)
		return 0
	case "uninstall":
		removed, err := githook.Uninstall(abs)
		if err != nil {
			fmt.Fprintln(stderr, "hook uninstall:", err)
			return 1
		}
		if removed {
			fmt.Fprintln(stdout, "removed managed hook")
		} else {
			fmt.Fprintln(stdout, "no managed hook present")
		}
		return 0
	default:
		fmt.Fprintf(stderr, "unknown hook subcommand: %s\n", sub)
		return 1
	}
}

// RunOutbox dispatches `relay outbox <push>`.
func RunOutbox(args []string) int {
	return runOutboxWith(args, os.Stdout, os.Stderr)
}

func runOutboxWith(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "push" {
		fmt.Fprintln(stderr, "usage: relay outbox push --intent-store <path> [--repo <path>] [--batch N]")
		return 1
	}
	fs := flag.NewFlagSet("outbox push", flag.ContinueOnError)
	repo := fs.String("repo", ".", "path to relay-managed repo (locates .relay/relay.db)")
	intentStore := fs.String("intent-store", "", "path to the intent-store git repository (required)")
	batch := fs.Int("batch", 0, "max certs per push (0 = unlimited)")
	fs.SetOutput(stderr)
	if err := fs.Parse(args[1:]); err != nil {
		return 1
	}
	if *intentStore == "" {
		fmt.Fprintln(stderr, "outbox push: --intent-store is required")
		return 1
	}

	repoAbs, _ := filepath.Abs(*repo)
	storeAbs, _ := filepath.Abs(*intentStore)

	dbPath := filepath.Join(repoAbs, ".relay", "engine.db")
	store, err := enginestore.Open(dbPath)
	if err != nil {
		fmt.Fprintln(stderr, "outbox push: open engine store:", err)
		return 1
	}
	defer store.Close()

	p := outbox.New(store, storeAbs)
	p.BatchLimit = *batch
	res, err := p.Push(context.Background())
	if err != nil {
		fmt.Fprintln(stderr, "outbox push:", err)
		return 1
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(res)
	return 0
}
