package cli

// CLI mirrors of the Auto-Intent Capture MCP tools. Each subcommand wraps an
// intent.Service method against the current working directory, so humans and
// scripts can drive the same draft → promote lifecycle the agent uses via MCP.

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/tabladrum/grove-suite/relay/internal/intent"
)

// RunIntentCapture dispatches `relay intent open|update|close|list|get`. It is
// invoked from the top-level command router when the first non-flag argument
// after `relay intent` is one of the Auto-Intent Capture verbs. Falls through
// to the Phase 1 intent commands (create/list/show/approve/...) when the verb
// is not recognized here.
func RunIntentCapture(args []string) (handled bool, exit int) {
	if len(args) == 0 {
		return false, 0
	}
	switch args[0] {
	case "open":
		return true, cmdIntentOpen(args[1:])
	case "update":
		return true, cmdIntentUpdate(args[1:])
	case "close":
		return true, cmdIntentClose(args[1:])
	case "list-captured":
		// Disambiguated name so we don't clash with the Phase 1 `intent list`.
		return true, cmdIntentCaptureList(args[1:])
	case "get-captured":
		return true, cmdIntentCaptureGet(args[1:])
	}
	return false, 0
}

func cmdIntentOpen(args []string) int {
	fs := flag.NewFlagSet("intent open", flag.ContinueOnError)
	title := fs.String("title", "", "title (≤80 chars summary of the user's request)")
	description := fs.String("description", "", "description (verbatim user prompt; required)")
	agent := fs.String("agent", "", "agent identifier (e.g. claude-code:1.4.2)")
	model := fs.String("model", "", "model identifier (e.g. claude-sonnet-4-6:2026-04-15)")
	ts := fs.String("conversation-ts", "", "ISO8601 timestamp of the originating conversation")
	repo := fs.String("repo", ".", "repo root (defaults to current dir)")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if *title == "" || *description == "" {
		fmt.Fprintln(os.Stderr, "usage: relay intent open --title <t> --description <d> [--agent <a>] [--model <m>] [--conversation-ts <iso>]")
		return 1
	}
	from := intent.OriginatedFrom{Agent: *agent, Model: *model}
	if *ts != "" {
		if t, err := time.Parse(time.RFC3339, *ts); err == nil {
			from.ConversationTS = t
		}
	}
	svc := intent.New(*repo)
	i, path, err := svc.Open(*title, *description, from)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Printf("intent_id:  %s\n", i.ID)
	fmt.Printf("draft_path: %s\n", path)
	fmt.Printf("status:     %s\n", i.Status)
	fmt.Println("next:       edit the draft if needed, then `relay intent close --id " + i.ID + "`")
	return 0
}

func cmdIntentUpdate(args []string) int {
	fs := flag.NewFlagSet("intent update", flag.ContinueOnError)
	id := fs.String("id", "", "intent ID (required)")
	patchFile := fs.String("patch-file", "", "path to YAML file containing the patch")
	patchInline := fs.String("patch", "", "inline YAML patch (e.g. 'risk_level: low')")
	repo := fs.String("repo", ".", "repo root")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if *id == "" {
		fmt.Fprintln(os.Stderr, "usage: relay intent update --id <id> [--patch <yaml>|--patch-file <path>]")
		return 1
	}
	var raw []byte
	switch {
	case *patchFile != "":
		b, err := os.ReadFile(*patchFile)
		if err != nil {
			fmt.Fprintln(os.Stderr, "read patch-file:", err)
			return 1
		}
		raw = b
	case *patchInline != "":
		raw = []byte(*patchInline)
	default:
		fmt.Fprintln(os.Stderr, "either --patch or --patch-file is required")
		return 1
	}
	var patch map[string]any
	if err := yaml.Unmarshal(raw, &patch); err != nil {
		fmt.Fprintln(os.Stderr, "parse patch:", err)
		return 1
	}
	svc := intent.New(*repo)
	i, err := svc.Update(*id, patch)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Printf("updated %s (title=%q)\n", i.ID, i.Title)
	return 0
}

func cmdIntentClose(args []string) int {
	fs := flag.NewFlagSet("intent close", flag.ContinueOnError)
	id := fs.String("id", "", "intent ID (required)")
	agent := fs.String("agent", "", "agent identifier")
	repo := fs.String("repo", ".", "repo root")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if *id == "" {
		fmt.Fprintln(os.Stderr, "usage: relay intent close --id <id> [--agent <a>]")
		return 1
	}
	svc := intent.New(*repo)
	committedPath, hash, trailer, err := svc.Close(*id, *agent)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Printf("committed_path: %s\n", committedPath)
	fmt.Printf("intent_hash:    sha256:%s\n", hash)
	fmt.Println("trailer_block:")
	for _, line := range strings.Split(strings.TrimRight(trailer, "\n"), "\n") {
		fmt.Println("  " + line)
	}
	return 0
}

func cmdIntentCaptureList(args []string) int {
	fs := flag.NewFlagSet("intent list-captured", flag.ContinueOnError)
	status := fs.String("status", "all", "draft|committed|all")
	repo := fs.String("repo", ".", "repo root")
	jsonOut := fs.Bool("json", false, "emit JSON instead of a table")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	svc := intent.New(*repo)
	entries, err := svc.List(*status)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if *jsonOut {
		_ = json.NewEncoder(os.Stdout).Encode(entries)
		return 0
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tSTATUS\tCREATED\tTITLE")
	for _, e := range entries {
		title := e.Title
		if len(title) > 60 {
			title = title[:57] + "..."
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n",
			e.ID, e.Status, e.CreatedAt.Format("2006-01-02 15:04"), title)
	}
	tw.Flush()
	return 0
}

func cmdIntentCaptureGet(args []string) int {
	fs := flag.NewFlagSet("intent get-captured", flag.ContinueOnError)
	id := fs.String("id", "", "intent ID (required)")
	repo := fs.String("repo", ".", "repo root")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if *id == "" {
		fmt.Fprintln(os.Stderr, "usage: relay intent get-captured --id <id>")
		return 1
	}
	svc := intent.New(*repo)
	// Try committed first, then draft.
	if i, err := svc.LoadCommitted(*id); err == nil {
		return printIntentYAML(i)
	}
	if i, err := svc.LoadDraft(*id); err == nil {
		return printIntentYAML(i)
	}
	fmt.Fprintf(os.Stderr, "intent %s not found\n", *id)
	return 1
}

func printIntentYAML(i *intent.Intent) int {
	data, err := i.Marshal()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	os.Stdout.Write(data)
	return 0
}
