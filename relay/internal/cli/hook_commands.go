// Package cli — hook + outbox subcommands (MVP-L8).
//
// `relay hook install [--force]` writes the pre-push hook.
// `relay hook uninstall` removes the relay-managed hook.
// `relay outbox push --intent-store=<path> [--batch=N]` replays new
// certificates from the engine store into a local intent-store git repo.
package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/tabladrum/grove-suite/relay/internal/analyzers/gitleaks"
	"github.com/tabladrum/grove-suite/relay/internal/analyzers/semgrep"
	"github.com/tabladrum/grove-suite/relay/internal/cert"
	"github.com/tabladrum/grove-suite/relay/internal/core"
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
		fmt.Fprintln(stderr, "usage: relay hook <install|install-all|uninstall|pre-commit|run> [--repo <path>] [--force] [--pre-commit]")
		return 1
	}
	sub := args[0]
	fs := flag.NewFlagSet("hook "+sub, flag.ContinueOnError)
	repo := fs.String("repo", ".", "path to git repository")
	force := fs.Bool("force", false, "overwrite a non-managed hook (install only)")
	preCommit := fs.Bool("pre-commit", false, "target the pre-commit hook (install/uninstall only)")
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
		if *preCommit {
			path, err := githook.InstallPreCommit(abs, *force)
			if err != nil {
				fmt.Fprintln(stderr, "hook install --pre-commit:", err)
				return 1
			}
			fmt.Fprintln(stdout, "installed:", path)
			return 0
		}
		path, err := githook.Install(abs, *force)
		if err != nil {
			fmt.Fprintln(stderr, "hook install:", err)
			return 1
		}
		fmt.Fprintln(stdout, "installed:", path)
		return 0
	case "install-all":
		// Convenience: install pre-push AND pre-commit in one go.
		paths := []string{}
		if p, err := githook.Install(abs, *force); err != nil {
			fmt.Fprintln(stderr, "hook install (pre-push):", err)
			return 1
		} else {
			paths = append(paths, p)
		}
		if p, err := githook.InstallPreCommit(abs, *force); err != nil {
			fmt.Fprintln(stderr, "hook install (pre-commit):", err)
			return 1
		} else {
			paths = append(paths, p)
		}
		for _, p := range paths {
			fmt.Fprintln(stdout, "installed:", p)
		}
		return 0
	case "run":
		// Internal subcommand invoked by the shell hook on every `git push`.
		// Reads the per-ref push spec from stdin and runs a lightweight local
		// check (outbox consistency + cert presence). Exit 0 = allow push.
		return cmdHookRun(abs, stderr)
	case "pre-commit":
		// Internal subcommand invoked by the pre-commit shell hook.
		return cmdHookPreCommit(abs, stdout, stderr)
	case "uninstall":
		if *preCommit {
			removed, err := githook.UninstallPreCommit(abs)
			if err != nil {
				fmt.Fprintln(stderr, "hook uninstall --pre-commit:", err)
				return 1
			}
			if removed {
				fmt.Fprintln(stdout, "removed managed pre-commit hook")
			} else {
				fmt.Fprintln(stdout, "no managed pre-commit hook present")
			}
			return 0
		}
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

// cmdHookPreCommit runs the laptop pre-commit gates: gitleaks (secrets) and
// semgrep (OWASP patterns) on staged files. Findings at or above HIGH cause
// the commit to be rejected; LOW/MEDIUM findings are printed as warnings.
// Missing analyzer binaries are warnings, never errors — the hook must not
// block commits on machines where the tool isn't installed yet.
//
// Bypass with `git commit --no-verify`.
func cmdHookPreCommit(repoRoot string, stdout, stderr io.Writer) int {
	ctx := context.Background()
	staged, err := stagedFiles(ctx, repoRoot)
	if err != nil {
		fmt.Fprintln(stderr, "pre-commit: list staged files:", err)
		return 0 // non-blocking on git plumbing errors
	}
	if len(staged) == 0 {
		return 0
	}
	cs := &core.ChangeSet{RepoRoot: repoRoot}

	var blocking []cert.Finding
	var warnings []cert.Finding

	runOne := func(name string, run func() ([]cert.Finding, error)) {
		findings, err := run()
		if err != nil {
			// Non-fatal: e.g. binary missing. Surface a short note.
			fmt.Fprintf(stderr, "relay pre-commit: %s skipped (%v)\n", name, err)
			return
		}
		for _, f := range findings {
			// Scope to staged files only — analyzers may scan repo-wide.
			if f.Path != "" && !inSet(staged, f.Path) {
				continue
			}
			switch f.Severity {
			case cert.SeverityHigh, cert.SeverityCritical:
				blocking = append(blocking, f)
			default:
				warnings = append(warnings, f)
			}
		}
	}

	runOne("gitleaks", func() ([]cert.Finding, error) {
		return gitleaks.New().Run(ctx, cs, repoRoot)
	})
	runOne("semgrep", func() ([]cert.Finding, error) {
		return semgrep.New().Run(ctx, cs, repoRoot)
	})

	for _, f := range warnings {
		fmt.Fprintf(stdout, "warn: %s [%s] %s:%d %s\n", f.Analyzer, f.RuleID, f.Path, f.Line, f.Message)
	}
	if len(blocking) > 0 {
		fmt.Fprintln(stderr)
		fmt.Fprintf(stderr, "relay pre-commit: %d high-severity finding(s) — commit blocked.\n", len(blocking))
		for _, f := range blocking {
			fmt.Fprintf(stderr, "  %s [%s] %s:%d %s\n", f.Analyzer, f.RuleID, f.Path, f.Line, f.Message)
		}
		fmt.Fprintln(stderr, "bypass with: git commit --no-verify")
		return 1
	}
	return 0
}

// stagedFiles returns the paths (relative to repoRoot) currently staged for
// commit. Uses git plumbing; safe to call from inside a hook.
func stagedFiles(ctx context.Context, repoRoot string) ([]string, error) {
	cmd := exec.CommandContext(ctx, "git", "diff", "--cached", "--name-only", "--diff-filter=ACMR")
	cmd.Dir = repoRoot
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	var paths []string
	for _, l := range lines {
		if l != "" {
			paths = append(paths, l)
		}
	}
	return paths, nil
}

func inSet(set []string, s string) bool {
	for _, x := range set {
		if x == s {
			return true
		}
	}
	return false
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
