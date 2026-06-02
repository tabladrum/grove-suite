package ingestion

import (
	"context"
	"time"

	"github.com/provasign/provasign/internal/core"
	"github.com/provasign/provasign/internal/db"
)

// projectGetter is a minimal interface so Pipeline doesn't import the full ProjectStore.
type projectGetter interface {
	Get(id string) (*core.Project, error)
}

type Pipeline struct {
	store     *db.IntentStore
	scorer    *GranularityScorer
	threshold float64
	projects  projectGetter
}

func NewPipeline(store *db.IntentStore, scorer *GranularityScorer, threshold float64, projects projectGetter) *Pipeline {
	return &Pipeline{store: store, scorer: scorer, threshold: threshold, projects: projects}
}

type ValidationResult struct {
	Passed     bool
	GSScore    float64
	ICRSymbols int
	Feedback   string
}

// Validate runs the GS pipeline using the global threshold.
func (p *Pipeline) Validate(ctx context.Context, intent *core.Intent) (*ValidationResult, error) {
	return p.validate(ctx, intent, p.threshold)
}

// ValidateForProject uses the project's configured GS threshold if available.
func (p *Pipeline) ValidateForProject(ctx context.Context, intent *core.Intent) (*ValidationResult, error) {
	threshold := p.threshold
	if intent.ProjectID != "" && p.projects != nil {
		proj, err := p.projects.Get(intent.ProjectID)
		if err == nil && proj != nil && proj.GSThreshold > 0 {
			threshold = proj.GSThreshold
		}
	}
	return p.validate(ctx, intent, threshold)
}

func (p *Pipeline) validate(ctx context.Context, intent *core.Intent, threshold float64) (*ValidationResult, error) {
	if err := p.store.UpdateStatus(intent.ID, core.StatusValidating, time.Now()); err != nil {
		return nil, err
	}
	result := p.scorer.Score(intent.Description)
	if err := p.store.UpdateGS(intent.ID, result.Score, result.ICRSymbols); err != nil {
		return nil, err
	}
	return &ValidationResult{
		Passed:     result.Score >= threshold,
		GSScore:    result.Score,
		ICRSymbols: result.ICRSymbols,
		Feedback:   result.Feedback,
	}, nil
}
