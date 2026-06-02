package api

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/provasign/provasign/internal/core"
	"github.com/provasign/provasign/internal/db"
	"github.com/provasign/provasign/internal/ingestion"
	"github.com/provasign/provasign/internal/lifecycle"
)

type intentHandlers struct {
	store    *db.IntentStore
	pipeline *ingestion.Pipeline
	lc       *lifecycle.Lifecycle
	projects *db.ProjectStore
}

func (h *intentHandlers) create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Description string `json:"description"`
		ProjectID   string `json:"project_id"`
		ProjectName string `json:"project_name"`
		Domain      string `json:"domain"`
		Author      string `json:"author"`
	}
	if err := decodeJSON(r.Body, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Description == "" || req.Author == "" {
		writeError(w, http.StatusBadRequest, "description and author are required")
		return
	}

	// Resolve project
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

	now := time.Now()
	intent := &core.Intent{
		ID:          uuid.NewString(),
		ProjectID:   projectID,
		Description: req.Description,
		Domain:      req.Domain,
		Source:      core.SourceNative,
		Status:      core.StatusDraft,
		Author:      req.Author,
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
		Actor:     req.Author,
		CreatedAt: now,
	})

	if err := h.lc.Submit(r.Context(), intent.ID, req.Author); err == nil {
		result, err := h.pipeline.ValidateForProject(r.Context(), intent)
		if err == nil {
			if result.Passed {
				_ = h.lc.Transition(r.Context(), intent.ID, core.StatusQueued, "system", "GS check passed")
			} else {
				_ = h.lc.MarkNeedsInfo(r.Context(), intent.ID, result.Feedback)
			}
		}
	}

	fresh, _ := h.store.Get(intent.ID)
	if fresh == nil {
		fresh = intent
	}
	writeJSON(w, http.StatusCreated, fresh)
}

func (h *intentHandlers) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	if limit <= 0 {
		limit = 50
	}
	intents, err := h.store.List(db.ListFilter{
		Status: q.Get("status"),
		Domain: q.Get("domain"),
		Source: q.Get("source"),
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if intents == nil {
		intents = []*core.Intent{}
	}
	writeJSON(w, http.StatusOK, intents)
}

func (h *intentHandlers) get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	intent, err := h.store.Get(id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "intent not found")
		} else {
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, intent)
}

func (h *intentHandlers) approve(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		ApprovedBy string `json:"approved_by"`
	}
	if err := decodeJSON(r.Body, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.ApprovedBy == "" {
		writeError(w, http.StatusBadRequest, "approved_by is required")
		return
	}
	if err := h.lc.Transition(r.Context(), id, core.StatusQueued, req.ApprovedBy, "manually approved"); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	intent, _ := h.store.Get(id)
	writeJSON(w, http.StatusOK, intent)
}

func (h *intentHandlers) reject(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		RejectedBy string `json:"rejected_by"`
		Note       string `json:"note"`
	}
	if err := decodeJSON(r.Body, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.RejectedBy == "" {
		writeError(w, http.StatusBadRequest, "rejected_by is required")
		return
	}
	if err := h.lc.Transition(r.Context(), id, core.StatusRejected, req.RejectedBy, req.Note); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	intent, _ := h.store.Get(id)
	writeJSON(w, http.StatusOK, intent)
}

func (h *intentHandlers) events(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	evs, err := h.store.Events(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if evs == nil {
		evs = []*core.IntentEvent{}
	}
	writeJSON(w, http.StatusOK, evs)
}
