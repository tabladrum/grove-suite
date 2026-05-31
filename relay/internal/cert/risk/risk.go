// Package risk computes a per-file risk heatmap from the inputs
// available at certificate-signing time: ICR confidence, Stage 1
// coverage delta, Stage 2 finding counts, and Grove blast-radius edges.
//
// The model is intentionally simple and explicit — every coefficient
// lives in this file so reviewers can challenge the math. Bump
// ModelVersion whenever a coefficient changes; the previous version
// remains pinned to certificates already on disk.
package risk

import (
	"sort"
	"strings"

	"github.com/tabladrum/grove-suite/relay/internal/cert"
	"github.com/tabladrum/grove-suite/relay/internal/core"
)

// ModelVersion identifies the scoring formula. Stored alongside each
// heatmap so historical certificates remain interpretable.
const ModelVersion = "v1"

// Cell is a single file's risk score plus the components that produced it.
type Cell struct {
	File       string             `json:"file"`
	Score      float64            `json:"score"`
	Components map[string]float64 `json:"components"`
}

// Heatmap is the structured form of the per-file risk view stored in
// certificate.payload["risk_heatmap"].
type Heatmap struct {
	ModelVersion string  `json:"model_version"`
	Overall      float64 `json:"overall"`
	Cells        []Cell  `json:"cells"`
}

// Inputs captures every signal the model consumes. Optional fields may
// be left zero/nil; the model degrades gracefully.
type Inputs struct {
	ICR    core.ICR
	Build *cert.BuildResult // nil if stage 1 was skipped
	Analysis *cert.AnalysisResult // nil if stage 2 was skipped
}

// Compute returns a Heatmap for the changeset. The scoring is bounded
// to [0, 1] per file with the following composition:
//
//	score = 0.30 * (1 - icr_confidence)          // unknown blast radius
//	      + 0.30 * stage2_severity_for_file
//	      + 0.25 * coverage_deficit_for_file
//	      + 0.15 * touch_intensity_for_file
//
// Each component is clamped to [0, 1] before weighting. The overall
// score is the max across cells (worst-file heuristic — risk is not
// averaged out by uninvolved files).
func Compute(in Inputs) Heatmap {
	files := uniqueFiles(in.ICR.Files)
	if len(files) == 0 {
		return Heatmap{ModelVersion: ModelVersion, Cells: []Cell{}}
	}

	findingsByFile := analysisFindingsByFile(in.Analysis)
	touch := touchIntensity(files)

	covDef := 0.0
	if in.Build != nil && in.Build.Tests.CoveragePct > 0 {
		covDef = clamp01(1.0 - in.Build.Tests.CoveragePct/100.0)
	}

	icrUnknown := clamp01(1.0 - in.ICR.Confidence)

	cells := make([]Cell, 0, len(files))
	overall := 0.0
	for _, f := range files {
		sev := analysisSeverityForFile(findingsByFile[f])
		ti := touch[f]
		comp := map[string]float64{
			"icr_unknown":      icrUnknown,
			"analysis_severity":  sev,
			"coverage_deficit": covDef,
			"touch_intensity":  ti,
		}
		score := 0.30*icrUnknown + 0.30*sev + 0.25*covDef + 0.15*ti
		score = clamp01(score)
		if score > overall {
			overall = score
		}
		cells = append(cells, Cell{File: f, Score: score, Components: comp})
	}
	// Hottest first for human consumption.
	sort.SliceStable(cells, func(i, j int) bool { return cells[i].Score > cells[j].Score })

	return Heatmap{ModelVersion: ModelVersion, Overall: overall, Cells: cells}
}

func uniqueFiles(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, f := range in {
		f = strings.TrimSpace(f)
		if f == "" || seen[f] {
			continue
		}
		seen[f] = true
		out = append(out, f)
	}
	sort.Strings(out)
	return out
}

func analysisFindingsByFile(s2 *cert.AnalysisResult) map[string][]cert.Finding {
	out := map[string][]cert.Finding{}
	if s2 == nil {
		return out
	}
	for _, f := range s2.Findings {
		if f.Path == "" {
			continue
		}
		out[f.Path] = append(out[f.Path], f)
	}
	return out
}

// analysisSeverityForFile collapses a slice of findings into a [0,1]
// severity for the file. critical = 1.0, high = 0.75, medium = 0.5,
// low = 0.25. Worst finding wins.
func analysisSeverityForFile(fs []cert.Finding) float64 {
	worst := 0.0
	for _, f := range fs {
		var v float64
		switch strings.ToLower(string(f.Severity)) {
		case "critical":
			v = 1.0
		case "high":
			v = 0.75
		case "medium":
			v = 0.5
		case "low":
			v = 0.25
		}
		if v > worst {
			worst = v
		}
	}
	return worst
}

// touchIntensity favours wide diffs: a single touched file is low
// intensity, a 20-file diff caps at 1.0. Linear scaling keeps the model
// debuggable.
func touchIntensity(files []string) map[string]float64 {
	out := map[string]float64{}
	n := float64(len(files))
	if n == 0 {
		return out
	}
	ti := clamp01(n / 20.0)
	for _, f := range files {
		out[f] = ti
	}
	return out
}

func clamp01(x float64) float64 {
	if x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}
