package core

import (
	"regexp"
	"strconv"
	"strings"
)

// LineRange is a half-open [Start, End) range of 1-indexed line numbers.
type LineRange struct {
	Start int
	End   int
}

// Contains reports whether line is within the range.
func (r LineRange) Contains(line int) bool { return line >= r.Start && line < r.End }

// AddedLineMap maps file path → sorted list of LineRanges added/modified by
// a unified diff. A file present with an empty slice means the file itself
// was added or renamed (caller may treat as "all lines new"). Files absent
// from the map were not touched by the diff.
type AddedLineMap map[string][]LineRange

// InScope reports whether (path, line) falls in any added range. Lines == 0
// (analyzer didn't report a line) are treated as in-scope only when the
// file appears in the map at all.
func (m AddedLineMap) InScope(path string, line int) bool {
	ranges, ok := m[normalizePath(path)]
	if !ok {
		return false
	}
	if line <= 0 || len(ranges) == 0 {
		return true
	}
	for _, r := range ranges {
		if r.Contains(line) {
			return true
		}
	}
	return false
}

// FileTouched reports whether the diff touched path at all.
func (m AddedLineMap) FileTouched(path string) bool {
	_, ok := m[normalizePath(path)]
	return ok
}

var (
	hunkHeaderRE = regexp.MustCompile(`^@@\s+-\d+(?:,\d+)?\s+\+(\d+)(?:,(\d+))?\s+@@`)
	plusFileRE   = regexp.MustCompile(`^\+\+\+\s+(?:b/)?(.+?)\s*$`)
)

// ParseAddedLines extracts the added line ranges from a unified diff. New
// files (whose `+++` header is followed by `+` lines with no preceding
// `---` body) are represented with an empty range slice meaning
// "everything is new".
func ParseAddedLines(diff string) AddedLineMap {
	out := AddedLineMap{}
	if strings.TrimSpace(diff) == "" {
		return out
	}
	var (
		curPath  string
		curLine  int
		ranges   []LineRange
		runStart int
		runLen   int
	)
	flush := func() {
		if curPath == "" {
			return
		}
		if runLen > 0 {
			ranges = append(ranges, LineRange{Start: runStart, End: runStart + runLen})
			runStart, runLen = 0, 0
		}
		// Merge contiguous/duplicate entries (idempotent for callers that
		// reuse the map across multiple parses).
		if existing, ok := out[curPath]; ok && len(existing) > 0 {
			out[curPath] = mergeRanges(append(existing, ranges...))
		} else {
			out[curPath] = mergeRanges(ranges)
		}
		ranges = nil
	}
	for _, raw := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(raw, "+++ "):
			flush()
			m := plusFileRE.FindStringSubmatch(raw)
			if m == nil {
				curPath = ""
				continue
			}
			curPath = normalizePath(strings.TrimSpace(m[1]))
			if curPath == "/dev/null" {
				curPath = ""
			}
		case strings.HasPrefix(raw, "@@"):
			if runLen > 0 {
				ranges = append(ranges, LineRange{Start: runStart, End: runStart + runLen})
				runStart, runLen = 0, 0
			}
			m := hunkHeaderRE.FindStringSubmatch(raw)
			if m == nil {
				continue
			}
			start, _ := strconv.Atoi(m[1])
			curLine = start
		case strings.HasPrefix(raw, "+") && !strings.HasPrefix(raw, "+++"):
			if curPath == "" {
				continue
			}
			if runLen == 0 {
				runStart = curLine
			}
			runLen++
			curLine++
		case strings.HasPrefix(raw, "-") && !strings.HasPrefix(raw, "---"):
			// removed line — does not advance new-file line counter
			if runLen > 0 {
				ranges = append(ranges, LineRange{Start: runStart, End: runStart + runLen})
				runStart, runLen = 0, 0
			}
		case strings.HasPrefix(raw, " "):
			// context line — advances new-file counter, breaks run
			if runLen > 0 {
				ranges = append(ranges, LineRange{Start: runStart, End: runStart + runLen})
				runStart, runLen = 0, 0
			}
			curLine++
		}
	}
	flush()
	return out
}

func mergeRanges(in []LineRange) []LineRange {
	if len(in) <= 1 {
		return in
	}
	// sort by start
	for i := 1; i < len(in); i++ {
		j := i
		for j > 0 && in[j-1].Start > in[j].Start {
			in[j-1], in[j] = in[j], in[j-1]
			j--
		}
	}
	out := in[:1]
	for _, r := range in[1:] {
		last := &out[len(out)-1]
		if r.Start <= last.End {
			if r.End > last.End {
				last.End = r.End
			}
			continue
		}
		out = append(out, r)
	}
	return out
}

func normalizePath(p string) string {
	p = strings.TrimSpace(p)
	if strings.HasPrefix(p, "./") {
		p = p[2:]
	}
	return p
}
