package routing

import (
	"testing"

	"github.com/provasign/provasign/internal/core"
)

func TestRelayProjectHint_FromTicketField(t *testing.T) {
	ticket := &core.TicketData{RelayProjectHint: "auth-service"}
	pis := []*core.ProjectIntegration{{Config: core.IntegrationConfig{}}}
	if got := relayProjectHint("jira", ticket, pis); got != "auth-service" {
		t.Errorf("got %q, want auth-service", got)
	}
}

func TestRelayProjectHint_FromLabel(t *testing.T) {
	ticket := &core.TicketData{
		Labels: []string{"bug", "relay-project:payments", "priority:high"},
	}
	pis := []*core.ProjectIntegration{{
		Config: core.IntegrationConfig{RelayProjectLabelPrefix: "relay-project:"},
	}}
	if got := relayProjectHint("github_issues", ticket, pis); got != "payments" {
		t.Errorf("got %q, want payments", got)
	}
}

func TestRelayProjectHint_NoHint(t *testing.T) {
	ticket := &core.TicketData{Labels: []string{"bug"}}
	pis := []*core.ProjectIntegration{{Config: core.IntegrationConfig{}}}
	if got := relayProjectHint("jira", ticket, pis); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestMatchesComponentFilter_Jira(t *testing.T) {
	cfg := &core.IntegrationConfig{ComponentFilter: "auth-service"}
	ticket := &core.TicketData{Component: "auth-service"}
	if !matchesComponentFilter("jira", ticket, cfg) {
		t.Error("expected match")
	}
	ticket.Component = "payments"
	if matchesComponentFilter("jira", ticket, cfg) {
		t.Error("expected no match")
	}
}

func TestMatchesComponentFilter_GitHub(t *testing.T) {
	cfg := &core.IntegrationConfig{ComponentLabel: "component:auth"}
	ticket := &core.TicketData{Labels: []string{"relay", "component:auth"}}
	if !matchesComponentFilter("github_issues", ticket, cfg) {
		t.Error("expected match")
	}
	ticket.Labels = []string{"relay"}
	if matchesComponentFilter("github_issues", ticket, cfg) {
		t.Error("expected no match")
	}
}

func TestMatchesComponentFilter_NoFilter(t *testing.T) {
	cfg := &core.IntegrationConfig{}
	ticket := &core.TicketData{Component: "anything"}
	if matchesComponentFilter("jira", ticket, cfg) {
		t.Error("empty filter should not match")
	}
}
