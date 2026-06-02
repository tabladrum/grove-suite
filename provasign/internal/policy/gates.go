package policy

import (
	"context"
	"fmt"
	"strings"

	"github.com/provasign/provasign/internal/core"
)

// PathGate denies changesets that touch any path with a forbidden prefix.
// Options:
//
//	deny: ["vendor/", ".git/", "secrets/"]
//
// Path matching is prefix-based against the file path as it appears in the
// unified diff header (`+++ b/<path>`), case-sensitive.
type PathGate struct{}

func (PathGate) Name() string { return "path" }

func (PathGate) Evaluate(_ context.Context, cs *core.ChangeSet, opts map[string]any) core.PolicyResult {
	deny := stringSlice(opts["deny"])
	files := touchedPaths(cs.Diff)
	var hits []string
	for _, f := range files {
		for _, d := range deny {
			if strings.HasPrefix(f, d) {
				hits = append(hits, fmt.Sprintf("%s (matched %q)", f, d))
				break
			}
		}
	}
	if len(hits) > 0 {
		return core.PolicyResult{
			Gate:       "path",
			Verdict:    core.VerdictDeny,
			Message:    fmt.Sprintf("%d path(s) match deny list", len(hits)),
			NextAction: "remove the changes to denied paths and resubmit",
			Details:    map[string]any{"hits": hits},
		}
	}
	return core.PolicyResult{
		Gate:    "path",
		Verdict: core.VerdictAllow,
		Message: fmt.Sprintf("%d file(s) inspected, no deny-list matches", len(files)),
		Details: map[string]any{"files": files},
	}
}

// SizeGate denies changesets that exceed configured file/line caps.
// Options:
//
//	max_files: 100
//	max_added_lines: 5000
type SizeGate struct{}

func (SizeGate) Name() string { return "size" }

func (SizeGate) Evaluate(_ context.Context, cs *core.ChangeSet, opts map[string]any) core.PolicyResult {
	maxFiles := intOption(opts, "max_files", 100)
	maxAdded := intOption(opts, "max_added_lines", 5000)
	files := touchedPaths(cs.Diff)
	added := countAddedLines(cs.Diff)

	if len(files) > maxFiles {
		return core.PolicyResult{
			Gate:       "size",
			Verdict:    core.VerdictDeny,
			Message:    fmt.Sprintf("%d files changed exceeds max_files=%d", len(files), maxFiles),
			NextAction: "split the change into smaller intent-scoped commits",
			Details:    map[string]any{"files": len(files), "limit": maxFiles},
		}
	}
	if added > maxAdded {
		return core.PolicyResult{
			Gate:       "size",
			Verdict:    core.VerdictDeny,
			Message:    fmt.Sprintf("%d lines added exceeds max_added_lines=%d", added, maxAdded),
			NextAction: "split the change into smaller intent-scoped commits",
			Details:    map[string]any{"added": added, "limit": maxAdded},
		}
	}
	verdict := core.VerdictAllow
	msg := fmt.Sprintf("%d file(s), %d line(s) added", len(files), added)
	if len(files) > maxFiles*8/10 || added > maxAdded*8/10 {
		verdict = core.VerdictWarn
		msg += " (approaching size limit)"
	}
	return core.PolicyResult{
		Gate:    "size",
		Verdict: verdict,
		Message: msg,
		Details: map[string]any{"files": len(files), "added": added},
	}
}

// touchedPaths extracts file paths from `+++ b/<path>` headers in a unified diff.
// `/dev/null` markers (file deletion target) are skipped.
func touchedPaths(diff string) []string {
	var out []string
	for _, line := range strings.Split(diff, "\n") {
		if !strings.HasPrefix(line, "+++ ") {
			continue
		}
		path := strings.TrimSpace(strings.TrimPrefix(line, "+++ "))
		if path == "/dev/null" || path == "" {
			continue
		}
		// Strip optional `b/` git prefix.
		path = strings.TrimPrefix(path, "b/")
		// Strip trailing tab-prefixed metadata (git diffs include timestamps).
		if i := strings.Index(path, "\t"); i >= 0 {
			path = path[:i]
		}
		out = append(out, path)
	}
	return out
}

// countAddedLines counts diff lines starting with '+' that are NOT the
// `+++ ` file header marker.
func countAddedLines(diff string) int {
	n := 0
	for _, line := range strings.Split(diff, "\n") {
		if strings.HasPrefix(line, "+++ ") {
			continue
		}
		if strings.HasPrefix(line, "+") {
			n++
		}
	}
	return n
}

func stringSlice(v any) []string {
	raw, ok := v.([]any)
	if !ok {
		// Allow already-typed []string for programmatic construction.
		if s, ok := v.([]string); ok {
			return s
		}
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, x := range raw {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func intOption(opts map[string]any, key string, def int) int {
	v, ok := opts[key]
	if !ok {
		return def
	}
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64: // JSON numbers
		return int(n)
	}
	return def
}
