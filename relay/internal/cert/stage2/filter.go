package stage2

import (
	"github.com/tabladrum/grove-suite/relay/internal/cert"
	"github.com/tabladrum/grove-suite/relay/internal/core"
)

// severityRank orders severities for the >= comparison done by the
// per-analyzer SeverityFloors filter.
var severityRank = map[cert.Severity]int{
	cert.SeverityInfo:     1,
	cert.SeverityLow:      2,
	cert.SeverityMedium:   3,
	cert.SeverityHigh:     4,
	cert.SeverityCritical: 5,
}

// filterFloor drops findings strictly below floor. Unknown severities
// (e.g. user-supplied via external analyzer) sort as 0 and are dropped.
func filterFloor(in []cert.Finding, floor cert.Severity) []cert.Finding {
	min := severityRank[floor]
	if min == 0 {
		return in
	}
	out := in[:0]
	for _, f := range in {
		if severityRank[f.Severity] >= min {
			out = append(out, f)
		}
	}
	return out
}

// filterNewCode drops findings whose (Path, Line) is not within the
// diff's added line ranges. Findings with no Path (analyzer-level
// errors, infrastructure errors) always pass through.
func filterNewCode(in []cert.Finding, added core.AddedLineMap) []cert.Finding {
	out := in[:0]
	for _, f := range in {
		if f.Path == "" {
			out = append(out, f)
			continue
		}
		if added.InScope(f.Path, f.Line) {
			out = append(out, f)
		}
	}
	return out
}
