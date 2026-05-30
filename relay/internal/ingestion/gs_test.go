package ingestion

import (
	"testing"
)

func TestHeuristicScore(t *testing.T) {
	tests := []struct {
		desc     string
		minScore float64
		maxScore float64
	}{
		{
			desc:     "Add rate limiting to the /api/auth/login endpoint — 100 req/min per IP",
			minScore: 0.60,
			maxScore: 1.0,
		},
		{
			desc:     "refactor all",
			minScore: 0.0,
			maxScore: 0.55,
		},
		{
			desc:     "improve",
			minScore: 0.0,
			maxScore: 0.55,
		},
		{
			desc:     "x",
			minScore: 0.0,
			maxScore: 0.55,
		},
		{
			desc:     "Update the UserService.GetByEmail method to return a not-found error instead of nil when the user does not exist in the auth domain",
			minScore: 0.60,
			maxScore: 1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc[:min(30, len(tt.desc))], func(t *testing.T) {
			score := heuristicScore(tt.desc)
			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("heuristicScore(%q) = %.2f, want [%.2f, %.2f]",
					tt.desc, score, tt.minScore, tt.maxScore)
			}
		})
	}
}

func TestGSResultWithoutGrove(t *testing.T) {
	scorer := NewGranularityScorer(nil, 0.70)

	result := scorer.Score("Add rate limiting to /api/auth/login endpoint")
	if result.Score < 0.60 {
		t.Errorf("specific intent should score >= 0.60, got %.2f", result.Score)
	}

	result = scorer.Score("refactor all the things")
	if result.Score >= 0.70 {
		t.Errorf("vague intent should score < 0.70, got %.2f", result.Score)
	}
	if result.Feedback == "" {
		t.Error("vague intent should have feedback")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
