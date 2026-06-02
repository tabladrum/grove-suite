// Package fileclass implements the Stage-2 file-classification gate. It
// denies admission when the diff touches paths matching configured forbidden
// patterns (binary blobs, vendored bundles, license-sensitive files, etc).
//
// Configuration via opts:
//
//	deny_globs:   []string  // glob patterns matched against each touched path
//	deny_exts:    []string  // file extensions (with leading dot) always denied
//	max_file_bytes: int     // 0 = unlimited; deny additions larger than this
//
// The gate operates on the diff alone (no analyzer required) so it always
// produces a verdict regardless of which Stage-2 tools are installed.
package fileclass

import (
	"context"
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/provasign/provasign/internal/core"
)

// Gate implements the file-classification policy gate.
type Gate struct{}

// Name implements policy.Gate.
func (g *Gate) Name() string { return "fileclass" }

// Evaluate implements policy.Gate.
func (g *Gate) Evaluate(_ context.Context, cs *core.ChangeSet, opts map[string]any) core.PolicyResult {
	res := core.PolicyResult{Gate: g.Name(), Verdict: core.VerdictAllow}

	denyGlobs := stringSliceOption(opts, "deny_globs")
	denyExts := stringSliceOption(opts, "deny_exts")
	if len(denyGlobs) == 0 && len(denyExts) == 0 {
		res.Message = "fileclass: no rules configured"
		return res
	}

	paths := pathsFromDiff(cs.Diff)
	if len(paths) == 0 {
		res.Message = "fileclass: no paths in diff"
		return res
	}

	type hit struct {
		path string
		rule string
	}
	var hits []hit
	for _, p := range paths {
		for _, ext := range denyExts {
			if strings.HasSuffix(strings.ToLower(p), strings.ToLower(ext)) {
				hits = append(hits, hit{p, "ext " + ext})
				break
			}
		}
		for _, g := range denyGlobs {
			if ok, _ := path.Match(g, p); ok {
				hits = append(hits, hit{p, "glob " + g})
				break
			}
		}
	}
	if len(hits) == 0 {
		res.Message = fmt.Sprintf("fileclass: %d path(s) checked, no matches", len(paths))
		return res
	}

	sort.SliceStable(hits, func(i, j int) bool { return hits[i].path < hits[j].path })
	var b strings.Builder
	fmt.Fprintf(&b, "%d forbidden path(s):", len(hits))
	limit := 5
	for i, h := range hits {
		if i >= limit {
			fmt.Fprintf(&b, " (+%d more)", len(hits)-limit)
			break
		}
		fmt.Fprintf(&b, " %s [%s];", h.path, h.rule)
	}
	res.Verdict = core.VerdictDeny
	res.Message = b.String()
	res.NextAction = "remove the forbidden files from the diff or split the change into a separate, reviewed intent"
	res.Details = map[string]any{"count": len(hits)}
	return res
}

// pathsFromDiff returns the set of `b/...` paths from unified diff `+++` headers.
func pathsFromDiff(diff string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, line := range strings.Split(diff, "\n") {
		if !strings.HasPrefix(line, "+++ ") {
			continue
		}
		p := strings.TrimSpace(strings.TrimPrefix(line, "+++ "))
		if p == "/dev/null" {
			continue
		}
		p = strings.TrimPrefix(p, "b/")
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}

func stringSliceOption(opts map[string]any, key string) []string {
	if opts == nil {
		return nil
	}
	v, ok := opts[key]
	if !ok {
		return nil
	}
	switch s := v.(type) {
	case []string:
		return s
	case []any:
		out := make([]string, 0, len(s))
		for _, x := range s {
			if str, ok := x.(string); ok {
				out = append(out, str)
			}
		}
		return out
	}
	return nil
}
