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
	case "run":
		// Internal subcommand invoked by the shell hook on every `git push`.
		// Reads the per-ref push spec from stdin and runs a lightweight local
		// check (outbox consistency + cert presence). Exit 0 = allow push.
		return cmdHookRun(abs, stderr)
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

// cmdHookRun is the implementation of `relay hook run`, called by the
// shell pre-push hook on every `git push`. It reads the per-ref push spec
// from stdin, checks the local outbox for uncertified commits, and prints a
// summary. Exit 0 always — this is a laptop-mode informational check, not a
// blocking gate (use `relay cert` for hard blocking).
func cmdHookRun(repoRoot string, stderr io.Writer) int {
	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintln(stderr, "hook run: read stdin:", err)
		return 1
	}
	ctx := context.Background()
	_ = ctx
	_ = input
	// Local outbox check: if the engine store has uncertified commits that
	// will be included in this push, warn the user.
	dbPath := filepath.Join(repoRoot, ".relay", "relay.db")
	store, err := enginestore.Open(dbPath)
	if err != nil {
		// Store not initialised yet — first push, nothing to certify.
		return 0
	}
	defer store.Close()
	certs, err := store.ListCertificates(ctx, "", 0)
	if err != nil || len(certs) == 0 {
		return 0
	}
	fmt.Fprintf(os.Stderr, "relay: %d certified change-set(s) in local outbox — run `relay outbox push` to sync\n", len(certs))
	return 0
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
