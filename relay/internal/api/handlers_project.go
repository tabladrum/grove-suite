package api

import (
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/tabladrum/grove-suite/relay/internal/core"
	"github.com/tabladrum/grove-suite/relay/internal/db"
	"github.com/tabladrum/grove-suite/relay/internal/ingestion"
	"github.com/tabladrum/grove-suite/relay/internal/lifecycle"
)

type projectHandlers struct {
	repos        *db.RepoStore
	projects     *db.ProjectStore
	integrations *db.IntegrationStore
	store        *db.IntentStore
	pipeline     *ingestion.Pipeline
	lc           *lifecycle.Lifecycle
}

// ── Repos ─────────────────────────────────────────────────────────────────────

func (h *projectHandlers) createRepo(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name          string `json:"name"`
		URL           string `json:"url"`
		DefaultBranch string `json:"default_branch"`
	}
	if err := decodeJSON(r.Body, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Name == "" || req.URL == "" {
		writeError(w, http.StatusBadRequest, "name and url are required")
		return
	}
	if req.DefaultBranch == "" {
		req.DefaultBranch = "main"
	}
	repo := &core.Repo{
		ID:            uuid.NewString(),
		Name:          req.Name,
		URL:           req.URL,
		DefaultBranch: req.DefaultBranch,
		CreatedAt:     time.Now(),
	}
	if err := h.repos.Create(repo); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, repo)
}

func (h *projectHandlers) listRepos(w http.ResponseWriter, r *http.Request) {
	repos, err := h.repos.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if repos == nil {
		repos = []*core.Repo{}
	}
	writeJSON(w, http.StatusOK, repos)
}

func (h *projectHandlers) getRepo(w http.ResponseWriter, r *http.Request) {
	repo, err := h.repos.Get(r.PathValue("id"))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "repo not found")
		} else {
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, repo)
}

func (h *projectHandlers) deleteRepo(w http.ResponseWriter, r *http.Request) {
	if err := h.repos.Delete(r.PathValue("id")); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── Projects ──────────────────────────────────────────────────────────────────

func (h *projectHandlers) createProject(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string  `json:"name"`
		RepoID      string  `json:"repo_id"`
		RepoName    string  `json:"repo_name"` // alternative to repo_id
		SourcePath  string  `json:"source_path"`
		GSThreshold float64 `json:"gs_threshold"`
		AutoApprove bool    `json:"auto_approve"`
		Owner       string  `json:"owner"`
	}
	if err := decodeJSON(r.Body, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	repoID := req.RepoID
	if repoID == "" && req.RepoName != "" {
		repo, err := h.repos.GetByName(req.RepoName)
		if err != nil {
			writeError(w, http.StatusBadRequest, "repo not found: "+req.RepoName)
			return
		}
		repoID = repo.ID
	}
	if repoID == "" {
		writeError(w, http.StatusBadRequest, "repo_id or repo_name is required")
		return
	}
	if req.SourcePath == "" {
		req.SourcePath = "/"
	}
	if req.GSThreshold == 0 {
		req.GSThreshold = 0.70
	}

	now := time.Now()
	proj := &core.Project{
		ID:          uuid.NewString(),
		Name:        req.Name,
		RepoID:      repoID,
		SourcePath:  req.SourcePath,
		GSThreshold: req.GSThreshold,
		AutoApprove: req.AutoApprove,
		Owner:       req.Owner,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := h.projects.Create(proj); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, proj)
}

func (h *projectHandlers) listProjects(w http.ResponseWriter, r *http.Request) {
	repoID := r.URL.Query().Get("repo_id")
	projects, err := h.projects.List(repoID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if projects == nil {
		projects = []*core.Project{}
	}
	writeJSON(w, http.StatusOK, projects)
}

func (h *projectHandlers) getProject(w http.ResponseWriter, r *http.Request) {
	p, err := h.projects.GetWithRepo(r.PathValue("id"))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "project not found")
		} else {
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (h *projectHandlers) deleteProject(w http.ResponseWriter, r *http.Request) {
	if err := h.projects.Delete(r.PathValue("id")); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── Integrations (project link/unlink) ───────────────────────────────────────

func (h *projectHandlers) addIntegration(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	var req struct {
		Type       string                 `json:"type"`
		ExternalID string                 `json:"external_id"`
		Config     core.IntegrationConfig `json:"config"`
	}
	if err := decodeJSON(r.Body, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Type == "" || req.ExternalID == "" {
		writeError(w, http.StatusBadRequest, "type and external_id are required")
		return
	}

	pi := &core.ProjectIntegration{
		ID:         uuid.NewString(),
		ProjectID:  projectID,
		Type:       req.Type,
		ExternalID: req.ExternalID,
		Config:     req.Config,
		CreatedAt:  time.Now(),
	}
	if err := h.integrations.Add(pi); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, pi)
}

func (h *projectHandlers) listIntegrations(w http.ResponseWriter, r *http.Request) {
	pis, err := h.integrations.FindByProject(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if pis == nil {
		pis = []*core.ProjectIntegration{}
	}
	writeJSON(w, http.StatusOK, pis)
}

func (h *projectHandlers) deleteIntegration(w http.ResponseWriter, r *http.Request) {
	if err := h.integrations.Delete(r.PathValue("integration_id")); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── Intent assign (unrouted → project) ───────────────────────────────────────

func (h *projectHandlers) assignIntent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		ProjectID   string `json:"project_id"`
		ProjectName string `json:"project_name"`
		AssignedBy  string `json:"assigned_by"`
	}
	if err := decodeJSON(r.Body, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	projectID := req.ProjectID
	if projectID == "" && req.ProjectName != "" {
		proj, err := h.projects.GetByName(req.ProjectName)
		if err != nil {
			writeError(w, http.StatusBadRequest, "project not found: "+req.ProjectName)
			return
		}
		projectID = proj.ID
	}
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project_id or project_name is required")
		return
	}
	if req.AssignedBy == "" {
		writeError(w, http.StatusBadRequest, "assigned_by is required")
		return
	}

	if err := h.lc.Assign(r.Context(), id, projectID, req.AssignedBy); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	// Run GS now that we have a project
	intent, err := h.store.Get(id)
	if err == nil {
		result, err := h.pipeline.ValidateForProject(r.Context(), intent)
		if err == nil {
			if result.Passed {
				_ = h.lc.Transition(r.Context(), id, core.StatusQueued, "system", "GS passed after assignment")
			} else {
				_ = h.lc.MarkNeedsInfo(r.Context(), id, result.Feedback)
			}
		}
	}

	fresh, _ := h.store.Get(id)
	writeJSON(w, http.StatusOK, fresh)
}

// ── Unrouted queue ────────────────────────────────────────────────────────────

func (h *projectHandlers) listUnrouted(w http.ResponseWriter, r *http.Request) {
	intents, err := h.store.ListUnrouted(50)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if intents == nil {
		intents = []*core.Intent{}
	}
	writeJSON(w, http.StatusOK, intents)
}
