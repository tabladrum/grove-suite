package api

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/tabladrum/grove-suite/relay/internal/config"
	"github.com/tabladrum/grove-suite/relay/internal/db"
	groveClient "github.com/tabladrum/grove-suite/relay/internal/grove"
	"github.com/tabladrum/grove-suite/relay/internal/ingestion"
	"github.com/tabladrum/grove-suite/relay/internal/integration"
	"github.com/tabladrum/grove-suite/relay/internal/lifecycle"
	"github.com/tabladrum/grove-suite/relay/internal/routing"
)

type Server struct {
	cfg          *config.Config
	store        *db.IntentStore
	repos        *db.RepoStore
	projects     *db.ProjectStore
	integrations *db.IntegrationStore
	pipeline     *ingestion.Pipeline
	lc           *lifecycle.Lifecycle
	registry     *integration.Registry
	router       *routing.Router
	grove        *groveClient.Client
	token        string
	srv          *http.Server
}

func NewServer(
	cfg *config.Config,
	store *db.IntentStore,
	repos *db.RepoStore,
	projects *db.ProjectStore,
	integrations *db.IntegrationStore,
	pipeline *ingestion.Pipeline,
	lc *lifecycle.Lifecycle,
	registry *integration.Registry,
	router *routing.Router,
	grove *groveClient.Client,
	token string,
) *Server {
	return &Server{
		cfg:          cfg,
		store:        store,
		repos:        repos,
		projects:     projects,
		integrations: integrations,
		pipeline:     pipeline,
		lc:           lc,
		registry:     registry,
		router:       router,
		grove:        grove,
		token:        token,
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	ih := &intentHandlers{store: s.store, pipeline: s.pipeline, lc: s.lc, projects: s.projects}
	wh := &webhookHandlers{
		store:               s.store,
		pipeline:            s.pipeline,
		lc:                  s.lc,
		router:              s.router,
		projects:            s.projects,
		jiraWebhookSecret:   s.cfg.Jira.WebhookSecret,
		githubWebhookSecret: s.cfg.GitHub.WebhookSecret,
		githubLabelTrigger:  s.cfg.GitHub.LabelTrigger,
		jiraRelayField:      s.cfg.Jira.RelayProjectField,
	}
	ph := &projectHandlers{
		repos:        s.repos,
		projects:     s.projects,
		integrations: s.integrations,
		store:        s.store,
		pipeline:     s.pipeline,
		lc:           s.lc,
	}

	mux.HandleFunc("GET /health", s.health)
	mux.HandleFunc("GET /", dashboardHandler(s.store, s.repos, s.projects, s.registry, s.grove))

	// Intent routes
	mux.HandleFunc("GET /api/intents", ih.list)
	mux.HandleFunc("POST /api/intents", ih.create)
	mux.HandleFunc("GET /api/intents/unrouted", ph.listUnrouted)
	mux.HandleFunc("GET /api/intents/{id}", ih.get)
	mux.HandleFunc("POST /api/intents/{id}/approve", ih.approve)
	mux.HandleFunc("POST /api/intents/{id}/reject", ih.reject)
	mux.HandleFunc("POST /api/intents/{id}/assign", ph.assignIntent)
	mux.HandleFunc("GET /api/intents/{id}/events", ih.events)

	// Repo routes
	mux.HandleFunc("GET /api/repos", ph.listRepos)
	mux.HandleFunc("POST /api/repos", ph.createRepo)
	mux.HandleFunc("GET /api/repos/{id}", ph.getRepo)
	mux.HandleFunc("DELETE /api/repos/{id}", ph.deleteRepo)

	// Project routes
	mux.HandleFunc("GET /api/projects", ph.listProjects)
	mux.HandleFunc("POST /api/projects", ph.createProject)
	mux.HandleFunc("GET /api/projects/{id}", ph.getProject)
	mux.HandleFunc("DELETE /api/projects/{id}", ph.deleteProject)
	mux.HandleFunc("GET /api/projects/{id}/integrations", ph.listIntegrations)
	mux.HandleFunc("POST /api/projects/{id}/integrations", ph.addIntegration)
	mux.HandleFunc("DELETE /api/projects/{id}/integrations/{integration_id}", ph.deleteIntegration)

	// Webhook routes (unauthenticated — validated by HMAC)
	mux.HandleFunc("POST /integrations/jira/webhook", wh.jira)
	mux.HandleFunc("POST /integrations/github/webhook", wh.github)

	return TokenMiddleware(s.token, mux)
}

func (s *Server) Listen() error {
	addr := fmt.Sprintf("127.0.0.1:%d", s.cfg.Port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", addr, err)
	}
	s.srv = &http.Server{
		Handler:      s.Handler(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
	return s.srv.Serve(ln)
}

func (s *Server) Shutdown(ctx context.Context) error {
	if s.srv == nil {
		return nil
	}
	return s.srv.Shutdown(ctx)
}

func (s *Server) Address() string {
	return fmt.Sprintf("127.0.0.1:%d", s.cfg.Port)
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
