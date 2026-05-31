package cli

// Engine-mode CLI commands (MVP-L1): submit, check, init, cert.
// Implemented in a separate file to keep Phase-1 CLI untouched.

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/tabladrum/grove-suite/relay/internal/admission"
	"github.com/tabladrum/grove-suite/relay/internal/config"
	"github.com/tabladrum/grove-suite/relay/internal/core"
	"github.com/tabladrum/grove-suite/relay/internal/engine"
	"github.com/tabladrum/grove-suite/relay/internal/enginestore"
	"github.com/tabladrum/grove-suite/relay/internal/policy"
	"github.com/tabladrum/grove-suite/relay/internal/signer"
)

// RunEngine dispatches the engine-mode subcommands. Returns an exit code.
// Wire this into Run() once Phase-1 dispatch is updated.
func RunEngine(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: relay <submit|check|init|cert>")
		return 1
	}
	switch args[0] {
	case "submit":
		return cmdSubmit(args[1:])
	case "check":
		return cmdCheck(args[1:])
	case "init":
		return cmdInit(args[1:])
	case "cert":
		return cmdCert(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown engine command: %s\n", args[0])
		return 1
	}
}

// cmdInit scaffolds a minimal `.relay/` in the current directory.
func cmdInit(args []string) int {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	dir := fs.String("dir", ".", "directory in which to create .relay/")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	root, err := filepath.Abs(*dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	relayDir := filepath.Join(root, ".relay")
	if err := os.MkdirAll(relayDir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "mkdir:", err)
		return 1
	}
	cfgPath := filepath.Join(relayDir, "relay.yaml")
	if _, err := os.Stat(cfgPath); err == nil {
		fmt.Fprintf(os.Stderr, "%s already exists, leaving untouched\n", cfgPath)
	} else {
		const tmpl = `relay_version: "0.1"
# Uncomment and set when using a stack template.
# stack: go-microservice
policies:
  path:
    enabled: true
    options:
      deny:
        - ".git/"
        - "vendor/"
        - "node_modules/"
  size:
    enabled: true
    options:
      max_files: 100
      max_added_lines: 5000
`
		if err := os.WriteFile(cfgPath, []byte(tmpl), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "write:", err)
			return 1
		}
		fmt.Printf("wrote %s\n", cfgPath)
	}
	// Touch keys directory + engine.db parent.
	if err := os.MkdirAll(filepath.Join(relayDir, "keys"), 0o700); err != nil {
		fmt.Fprintln(os.Stderr, "mkdir keys:", err)
		return 1
	}
	fmt.Printf("relay initialized at %s\n", relayDir)
	return 0
}

// cmdCheck loads config, builds the engine, runs Check on the diff, and prints results.
// Flags:
//
//	--diff <path>    Path to unified diff file. Use "-" for stdin.
//	--intent <id>    Intent identifier (optional in laptop mode).
//	--repo <path>    Repo root (defaults to cwd).
//	--json           Emit JSON instead of human-readable output.
func cmdCheck(args []string) int {
	cs, repoRoot, asJSON, err := parseChangeSetFlags("check", args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	cs.RepoRoot = repoRoot
	e, cleanup, err := buildEngine(repoRoot)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer cleanup()

	res, err := e.Check(context.Background(), cs)
	if err != nil {
		fmt.Fprintln(os.Stderr, "check:", err)
		return 1
	}
	if asJSON {
		_ = json.NewEncoder(os.Stdout).Encode(res)
	} else {
		printCheckResult(os.Stdout, res)
	}
	if !res.Allowed {
		return 2
	}
	return 0
}

// cmdSubmit runs the full pipeline. Same flags as `check` plus the diff is admitted.
func cmdSubmit(args []string) int {
	cs, repoRoot, asJSON, err := parseChangeSetFlags("submit", args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	cs.RepoRoot = repoRoot
	e, cleanup, err := buildEngine(repoRoot)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer cleanup()
	e.Admit = admission.NewGitAdmitter()

	res, err := e.Submit(context.Background(), cs)
	if err != nil {
		fmt.Fprintln(os.Stderr, "submit:", err)
		return 1
	}
	if asJSON {
		_ = json.NewEncoder(os.Stdout).Encode(res)
	} else {
		printCheckResult(os.Stdout, res)
		if res.Allowed {
			fmt.Printf("admitted: %s\n", res.CommitSHA)
			fmt.Printf("certificate: %s\n", res.Certificate.ID)
		}
	}
	if !res.Allowed {
		return 2
	}
	return 0
}

// cmdCert handles `relay cert verify <commit-sha>` and `relay cert show <commit-sha>`.
func cmdCert(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: relay cert <verify|show> <commit-sha>")
		return 1
	}
	sub := args[0]
	if len(args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: relay cert %s <commit-sha>\n", sub)
		return 1
	}
	sha := args[1]

	repoRoot, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	root, err := config.DiscoverRelayRoot(repoRoot)
	if err != nil {
		fmt.Fprintln(os.Stderr, "discover .relay/:", err)
		return 1
	}
	store, err := enginestore.Open(filepath.Join(root, ".relay", "engine.db"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "open store:", err)
		return 1
	}
	defer store.Close()
	cert, err := store.GetCertificateByCommit(context.Background(), sha)
	if err != nil {
		fmt.Fprintln(os.Stderr, "lookup cert:", err)
		return 1
	}

	switch sub {
	case "show":
		_ = json.NewEncoder(os.Stdout).Encode(cert)
		return 0
	case "verify":
		pubPath := filepath.Join(root, ".relay", "keys", "signing.ed25519.pub")
		pub, err := os.ReadFile(pubPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "read pubkey:", err)
			return 1
		}
		if err := signer.Verify(pub, cert, cert.Signature); err != nil {
			fmt.Fprintln(os.Stderr, "verify FAILED:", err)
			return 1
		}
		fmt.Printf("verify OK  cert=%s  commit=%s  signed_by=%s\n", cert.ID, sha, cert.SignedBy)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown cert subcommand: %s\n", sub)
		return 1
	}
}

// parseChangeSetFlags is shared by submit/check.
func parseChangeSetFlags(name string, args []string) (cs *core.ChangeSet, repoRoot string, asJSON bool, err error) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	diffPath := fs.String("diff", "", "path to unified diff (use - for stdin)")
	intentID := fs.String("intent", "", "intent identifier")
	brief := fs.String("brief", "", "short description of the change")
	author := fs.String("author", "", "author/agent name")
	model := fs.String("model", "", "agent model identifier")
	repo := fs.String("repo", "", "repo root (defaults to cwd)")
	js := fs.Bool("json", false, "emit JSON output")
	if perr := fs.Parse(args); perr != nil {
		return nil, "", false, perr
	}
	if *diffPath == "" {
		return nil, "", false, fmt.Errorf("--diff is required")
	}
	diff, derr := readDiff(*diffPath)
	if derr != nil {
		return nil, "", false, fmt.Errorf("read diff: %w", derr)
	}
	root := *repo
	if root == "" {
		wd, werr := os.Getwd()
		if werr != nil {
			return nil, "", false, werr
		}
		root = wd
	}
	cs = &core.ChangeSet{
		IntentID:    *intentID,
		IntentBrief: *brief,
		Diff:        diff,
		Author:      *author,
		AgentModel:  *model,
	}
	return cs, root, *js, nil
}

func readDiff(path string) (string, error) {
	if path == "-" {
		data, err := io.ReadAll(os.Stdin)
		return string(data), err
	}
	data, err := os.ReadFile(path)
	return string(data), err
}

// buildEngine wires an Engine for use by the engine-mode CLI commands.
// Returns a cleanup function the caller must defer.
func buildEngine(start string) (*engine.Engine, func(), error) {
	root, err := config.DiscoverRelayRoot(start)
	if err != nil {
		return nil, nil, fmt.Errorf("discover .relay/: %w", err)
	}
	cfg, err := config.LoadRelayConfig(start)
	if err != nil {
		return nil, nil, fmt.Errorf("load config: %w", err)
	}
	store, err := enginestore.Open(filepath.Join(root, ".relay", "engine.db"))
	if err != nil {
		return nil, nil, fmt.Errorf("open store: %w", err)
	}
	keyDir := filepath.Join(root, ".relay", "keys")
	sgn, err := signer.LoadOrCreateLocal(keyDir)
	if err != nil {
		_ = store.Close()
		return nil, nil, fmt.Errorf("signer: %w", err)
	}
	reg := policy.NewRegistry()
	reg.Register(policy.PathGate{})
	reg.Register(policy.SizeGate{})
	e := &engine.Engine{
		Store:    store,
		Policies: reg,
		ICR:      engine.NoopICRProvider{},
		Signer:   sgn,
		Config:   cfg,
	}
	return e, func() { _ = store.Close() }, nil
}

func printCheckResult(w io.Writer, res *engine.Result) {
	status := "ALLOW"
	if !res.Allowed {
		status = "DENY"
	}
	fmt.Fprintf(w, "verdict: %s\n", status)
	for _, p := range res.Policies {
		marker := "  ✓"
		if !p.Allowed() {
			marker = "  ✗"
		}
		fmt.Fprintf(w, "%s [%s] %s — %s\n", marker, p.Gate, p.Verdict, p.Message)
		if p.NextAction != "" {
			fmt.Fprintf(w, "      next: %s\n", p.NextAction)
		}
	}
	if res.Certificate != nil {
		fmt.Fprintf(w, "certificate: %s (signed_by=%s sig=%s)\n",
			res.Certificate.ID, res.Certificate.SignedBy,
			shortHex(res.Certificate.Signature))
	}
}

func shortHex(b []byte) string {
	h := hex.EncodeToString(b)
	if len(h) > 16 {
		return h[:16] + "..."
	}
	return h
}
