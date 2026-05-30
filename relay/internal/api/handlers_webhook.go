package api

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/tabladrum/grove-suite/relay/internal/core"
	"github.com/tabladrum/grove-suite/relay/internal/db"
	"github.com/tabladrum/grove-suite/relay/internal/ingestion"
	ghConn "github.com/tabladrum/grove-suite/relay/internal/integration/github"
	jiraConn "github.com/tabladrum/grove-suite/relay/internal/integration/jira"
	"github.com/tabladrum/grove-suite/relay/internal/lifecycle"
	"github.com/tabladrum/grove-suite/relay/internal/routing"
)

type webhookHandlers struct {
	store    *db.IntentStore
	pipeline *ingestion.Pipeline
	lc       *lifecycle.Lifecycle
	router   *routing.Router
	projects *db.ProjectStore
	jiraWebhookSecret   string
	githubWebhookSecret string
	githubLabelTrigger  string
	jiraRelayField      string // Jira custom field name for relay-project
}

func (h *webhookHandlers) jira(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "read body")
		return
	}
	if !jiraConn.ValidateWebhook(h.jiraWebhookSecret, body, r) {
		writeError(w, http.StatusUnauthorized, "invalid signature")
		return
	}

	ev, err := jiraConn.ParseWebhook(bytes.NewReader(body), h.jiraRelayField)
	if err != nil {
		writeError(w, http.StatusBadRequest, "parse webhook: "+err.Error())
		return
	}
	if ev.EventType != "jira:issue_updated" && ev.EventType != "jira:issue_created" {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// board key = prefix before "-" in the issue key (e.g. "AUTH" from "AUTH-42")
	boardKey := jiraBoardKey(ev.IssueKey)
	ticket := &core.TicketData{
		Ref:              ev.IssueKey,
		Title:            ev.Summary,
		Description:      ev.Summary,
		Author:           "jira-webhook",
		Component:        ev.Component,
		RelayProjectHint: ev.RelayProjectHint,
	}

	h.ingestViaRouter(w, r, "jira", boardKey, ev.Status, ticket, core.SourceJira)
}

func (h *webhookHandlers) github(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "read body")
		return
	}
	if !ghConn.ValidateWebhook(h.githubWebhookSecret, body, r) {
		writeError(w, http.StatusUnauthorized, "invalid signature")
		return
	}
	if r.Header.Get("X-GitHub-Event") != "issues" {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	ev, err := ghConn.ParseWebhook(bytes.NewReader(body), h.githubLabelTrigger)
	if err != nil {
		writeError(w, http.StatusBadRequest, "parse webhook: "+err.Error())
		return
	}
	if ev.Action != "labeled" && ev.Action != "opened" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if !ev.HasLabel {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// external_id for github_issues integrations is "owner/repo"
	repoID := githubRepoID(ev.IssueRef)
	ticket := &core.TicketData{
		Ref:              ev.IssueRef,
		Title:            ev.Title,
		Description:      ev.Body,
		Author:           ev.Author,
		Labels:           ev.Labels,
		RelayProjectHint: ev.RelayProjectHint,
	}

	h.ingestViaRouter(w, r, "github_issues", repoID, "", ticket, core.SourceGitHubIssue)
}

func (h *webhookHandlers) ingestViaRouter(
	w http.ResponseWriter, r *http.Request,
	intType, externalID, triggerStatus string,
	ticket *core.TicketData,
	source core.IntentSource,
) {
	ctx := r.Context()

	routeResult, err := h.router.Route(intType, externalID, ticket)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "routing: "+err.Error())
		return
	}
	if routeResult == nil {
		// board not registered with any project — ignore silently
		w.WriteHeader(http.StatusNoContent)
		return
	}

	now := time.Now()
	status := core.StatusDraft
	if routeResult.Unrouted {
		status = core.StatusUnrouted
	}

	desc := ticket.Title
	if ticket.Description != "" && ticket.Description != ticket.Title {
		desc += " — " + ticket.Description
	}

	intent := &core.Intent{
		ID:          uuid.NewString(),
		ProjectID:   routeResult.ProjectID,
		Description: desc,
		Source:      source,
		SourceRef:   ticket.Ref,
		Status:      status,
		Author:      ticket.Author,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := h.store.Create(intent); err != nil {
		writeError(w, http.StatusInternalServerError, "create intent: "+err.Error())
		return
	}
	_ = h.store.AppendEvent(&core.IntentEvent{
		IntentID:  intent.ID,
		Type:      "created",
		Detail:    "from " + string(source),
		Actor:     ticket.Author,
		CreatedAt: now,
	})

	if routeResult.Unrouted {
		_ = h.lc.Unroute(ctx, intent.ID, "system")
		writeJSON(w, http.StatusCreated, map[string]string{
			"id":     intent.ID,
			"status": "unrouted",
			"note":   "no project resolved; assign via POST /api/intents/" + intent.ID + "/assign",
		})
		return
	}

	h.runValidation(ctx, intent, routeResult.AutoApprove)
	writeJSON(w, http.StatusCreated, map[string]string{"id": intent.ID})
}

func (h *webhookHandlers) runValidation(ctx context.Context, intent *core.Intent, autoApprove bool) {
	if err := h.lc.Submit(ctx, intent.ID, "system"); err != nil {
		return
	}
	result, err := h.pipeline.ValidateForProject(ctx, intent)
	if err != nil {
		return
	}
	if result.Passed {
		if autoApprove {
			_ = h.lc.Transition(ctx, intent.ID, core.StatusQueued, "system", "GS passed; auto-approved")
		} else {
			_ = h.lc.Transition(ctx, intent.ID, core.StatusQueued, "system", "GS passed")
		}
	} else {
		_ = h.lc.MarkNeedsInfo(ctx, intent.ID, result.Feedback)
	}
}

func jiraBoardKey(issueKey string) string {
	for i, c := range issueKey {
		if c == '-' {
			return issueKey[:i]
		}
	}
	return issueKey
}

func githubRepoID(issueRef string) string {
	// "owner/repo#123" → "owner/repo"
	for i, c := range issueRef {
		if c == '#' {
			return issueRef[:i]
		}
	}
	return issueRef
}
