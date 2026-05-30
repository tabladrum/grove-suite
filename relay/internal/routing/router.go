package routing

import (
	"strings"

	"github.com/tabladrum/grove-suite/relay/internal/core"
	"github.com/tabladrum/grove-suite/relay/internal/db"
)

// Result is returned by Route.
// ProjectID is empty and Unrouted is true when no project could be resolved.
type Result struct {
	ProjectID   string
	AutoApprove bool
	Unrouted    bool
}

type Router struct {
	integrations *db.IntegrationStore
	projects     *db.ProjectStore
}

func New(integrations *db.IntegrationStore, projects *db.ProjectStore) *Router {
	return &Router{integrations: integrations, projects: projects}
}

// Route applies the B → C → A algorithm.
//
//   B: explicit relay-project field/label on the ticket  → direct route
//   C: no unique match                                   → unrouted (human assigns)
//   A: component/label filter on the integration config  → route if exactly one matches
func (r *Router) Route(intType, externalID string, ticket *core.TicketData) (*Result, error) {
	pis, err := r.integrations.FindByTypeAndExternalID(intType, externalID)
	if err != nil {
		return nil, err
	}
	if len(pis) == 0 {
		return nil, nil // board not registered; ignore entirely
	}

	// ── B: explicit relay-project hint ────────────────────────────────────────
	if hint := relayProjectHint(intType, ticket, pis); hint != "" {
		for _, pi := range pis {
			proj, err := r.projects.Get(pi.ProjectID)
			if err != nil || proj == nil {
				continue
			}
			if strings.EqualFold(proj.Name, hint) {
				return &Result{
					ProjectID:   proj.ID,
					AutoApprove: pi.Config.AutoApprove || proj.AutoApprove,
				}, nil
			}
		}
		// Hint was set but didn't match any registered project → unrouted
		return &Result{Unrouted: true}, nil
	}

	// ── A: component/label filter ─────────────────────────────────────────────
	var matched []*core.ProjectIntegration
	for i := range pis {
		if matchesComponentFilter(intType, ticket, &pis[i].Config) {
			matched = append(matched, pis[i])
		}
	}
	if len(matched) == 1 {
		proj, err := r.projects.Get(matched[0].ProjectID)
		if err != nil || proj == nil {
			return &Result{Unrouted: true}, nil
		}
		return &Result{
			ProjectID:   proj.ID,
			AutoApprove: matched[0].Config.AutoApprove || proj.AutoApprove,
		}, nil
	}

	// ── C: unrouted ───────────────────────────────────────────────────────────
	return &Result{Unrouted: true}, nil
}

// relayProjectHint extracts the explicit relay-project value from the ticket,
// using the first matching integration's config to know where to look.
func relayProjectHint(intType string, ticket *core.TicketData, pis []*core.ProjectIntegration) string {
	// The connector already extracted it into TicketData.RelayProjectHint.
	if ticket.RelayProjectHint != "" {
		return ticket.RelayProjectHint
	}

	// Fallback: scan labels for the configured prefix (GitHub Issues / Linear).
	prefix := ""
	for _, pi := range pis {
		if pi.Config.RelayProjectLabelPrefix != "" {
			prefix = pi.Config.RelayProjectLabelPrefix
			break
		}
	}
	if prefix == "" {
		prefix = "relay-project:"
	}
	for _, label := range ticket.Labels {
		if strings.HasPrefix(strings.ToLower(label), strings.ToLower(prefix)) {
			return strings.TrimPrefix(label, prefix)
		}
	}
	return ""
}

// matchesComponentFilter returns true if the ticket matches the A-route filter
// configured on a specific integration.
func matchesComponentFilter(intType string, ticket *core.TicketData, cfg *core.IntegrationConfig) bool {
	switch intType {
	case "jira":
		if cfg.ComponentFilter == "" {
			return false
		}
		return strings.EqualFold(ticket.Component, cfg.ComponentFilter)
	case "github_issues", "github_projects":
		if cfg.ComponentLabel == "" {
			return false
		}
		for _, l := range ticket.Labels {
			if strings.EqualFold(l, cfg.ComponentLabel) {
				return true
			}
		}
		return false
	}
	return false
}
