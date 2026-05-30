package cli

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/google/uuid"
	"github.com/tabladrum/grove-suite/relay/internal/api"
	"github.com/tabladrum/grove-suite/relay/internal/config"
	"github.com/tabladrum/grove-suite/relay/internal/core"
	relayDB "github.com/tabladrum/grove-suite/relay/internal/db"
	groveClient "github.com/tabladrum/grove-suite/relay/internal/grove"
	"github.com/tabladrum/grove-suite/relay/internal/ingestion"
	"github.com/tabladrum/grove-suite/relay/internal/integration"
	ghConn "github.com/tabladrum/grove-suite/relay/internal/integration/github"
	jiraConn "github.com/tabladrum/grove-suite/relay/internal/integration/jira"
	"github.com/tabladrum/grove-suite/relay/internal/lifecycle"
	"github.com/tabladrum/grove-suite/relay/internal/routing"
	"github.com/tabladrum/grove-suite/relay/internal/version"
)

func Run(args []string) int {
	if len(args) == 0 {
		printUsage()
		return 1
	}
	switch args[0] {
	case "serve":
		return cmdServe(args[1:])
	case "repo":
		return cmdRepo(args[1:])
	case "project":
		return cmdProject(args[1:])
	case "intent":
		return cmdIntent(args[1:])
	case "version", "--version", "-v":
		fmt.Println("relay", version.Version)
		return 0
	case "help", "--help", "-h":
		printUsage()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", args[0])
		printUsage()
		return 1
	}
}

// ── serve ─────────────────────────────────────────────────────────────────────

func cmdServe(args []string) int {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	port := fs.Int("port", 0, "port (overrides config)")
	dbURL := fs.String("db", "", "postgres DSN (overrides config)")
	cfgPath := fs.String("config", "relay.yaml", "config file path")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		return 1
	}
	if *port != 0 {
		cfg.Port = *port
	}
	if *dbURL != "" {
		cfg.DBURL = *dbURL
	}

	sqlDB, err := relayDB.Open(cfg.DBURL)
	if err != nil {
		fmt.Fprintln(os.Stderr, "db:", err)
		return 1
	}
	defer sqlDB.Close()

	store := relayDB.NewIntentStore(sqlDB)
	repos := relayDB.NewRepoStore(sqlDB)
	projects := relayDB.NewProjectStore(sqlDB)
	integrations := relayDB.NewIntegrationStore(sqlDB)

	registry := buildRegistry(cfg)
	grove := groveClient.NewClient(cfg.GroveURL)
	scorer := ingestion.NewGranularityScorer(grove, cfg.GSThreshold)
	pipeline := ingestion.NewPipeline(store, scorer, cfg.GSThreshold, projects)
	lc := lifecycle.New(store, registry)
	router := routing.New(integrations, projects)

	token, err := api.LoadOrCreateToken(cfg.TokenFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "token:", err)
		return 1
	}

	if err := grove.EnsureRunning("."); err != nil {
		fmt.Fprintln(os.Stderr, "warning: grove not reachable:", err)
	}

	srv := api.NewServer(cfg, store, repos, projects, integrations, pipeline, lc, registry, router, grove, token)
	fmt.Printf("relay serving on http://%s\n", srv.Address())
	if err := srv.Listen(); err != nil {
		fmt.Fprintln(os.Stderr, "serve:", err)
		return 1
	}
	return 0
}

// ── repo ──────────────────────────────────────────────────────────────────────

func cmdRepo(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: relay repo <add|list|remove>")
		return 1
	}
	_, store, repos, _, _, err := openStores()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer store.DB().Close()

	switch args[0] {
	case "add":
		return cmdRepoAdd(args[1:], repos)
	case "list":
		return cmdRepoList(repos)
	case "remove", "rm":
		return cmdRepoRemove(args[1:], repos)
	default:
		fmt.Fprintf(os.Stderr, "unknown repo subcommand: %s\n", args[0])
		return 1
	}
}

func cmdRepoAdd(args []string, repos *relayDB.RepoStore) int {
	fs := flag.NewFlagSet("repo add", flag.ContinueOnError)
	name := fs.String("name", "", "short name for the repo")
	branch := fs.String("branch", "main", "default branch")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if fs.NArg() == 0 || *name == "" {
		fmt.Fprintln(os.Stderr, "usage: relay repo add --name <name> [--branch main] <url>")
		return 1
	}
	repo := &core.Repo{
		ID:            uuid.NewString(),
		Name:          *name,
		URL:           fs.Arg(0),
		DefaultBranch: *branch,
		CreatedAt:     time.Now(),
	}
	if err := repos.Create(repo); err != nil {
		fmt.Fprintln(os.Stderr, "create repo:", err)
		return 1
	}
	fmt.Printf("repo %s added (%s)\n", repo.Name, repo.ID[:8])
	return 0
}

func cmdRepoList(repos *relayDB.RepoStore) int {
	list, err := repos.List()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tNAME\tURL\tBRANCH")
	for _, r := range list {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", r.ID[:8], r.Name, r.URL, r.DefaultBranch)
	}
	tw.Flush()
	return 0
}

func cmdRepoRemove(args []string, repos *relayDB.RepoStore) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: relay repo remove <id>")
		return 1
	}
	if err := repos.Delete(args[0]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println("removed")
	return 0
}

// ── project ───────────────────────────────────────────────────────────────────

func cmdProject(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: relay project <add|list|show|link|unlink>")
		return 1
	}
	_, store, repos, projects, integrations, err := openStores()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer store.DB().Close()

	switch args[0] {
	case "add":
		return cmdProjectAdd(args[1:], repos, projects)
	case "list":
		return cmdProjectList(args[1:], projects)
	case "show":
		return cmdProjectShow(args[1:], projects)
	case "link":
		return cmdProjectLink(args[1:], projects, integrations)
	case "unlink":
		return cmdProjectUnlink(args[1:], integrations)
	default:
		fmt.Fprintf(os.Stderr, "unknown project subcommand: %s\n", args[0])
		return 1
	}
}

func cmdProjectAdd(args []string, repos *relayDB.RepoStore, projects *relayDB.ProjectStore) int {
	fs := flag.NewFlagSet("project add", flag.ContinueOnError)
	repo := fs.String("repo", "", "repo name (required)")
	path := fs.String("path", "/", "source path within the repo (for monorepos)")
	threshold := fs.Float64("gs-threshold", 0.70, "GS threshold for this project")
	autoApprove := fs.Bool("auto-approve", false, "auto-approve passing intents")
	owner := fs.String("owner", "", "owner email or team")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if fs.NArg() == 0 || *repo == "" {
		fmt.Fprintln(os.Stderr, "usage: relay project add <name> --repo <repo-name> [--path /] [--owner eng@example.com]")
		return 1
	}

	r, err := repos.GetByName(*repo)
	if err != nil {
		fmt.Fprintln(os.Stderr, "repo not found:", *repo)
		return 1
	}

	now := time.Now()
	proj := &core.Project{
		ID:          uuid.NewString(),
		Name:        fs.Arg(0),
		RepoID:      r.ID,
		SourcePath:  *path,
		GSThreshold: *threshold,
		AutoApprove: *autoApprove,
		Owner:       *owner,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := projects.Create(proj); err != nil {
		fmt.Fprintln(os.Stderr, "create project:", err)
		return 1
	}
	fmt.Printf("project %s added (id: %s, repo: %s, path: %s)\n",
		proj.Name, proj.ID[:8], r.Name, proj.SourcePath)
	return 0
}

func cmdProjectList(args []string, projects *relayDB.ProjectStore) int {
	list, err := projects.List("")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tNAME\tPATH\tGS\tAUTO")
	for _, p := range list {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%.2f\t%v\n",
			p.ID[:8], p.Name, p.SourcePath, p.GSThreshold, p.AutoApprove)
	}
	tw.Flush()
	return 0
}

func cmdProjectShow(args []string, projects *relayDB.ProjectStore) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: relay project show <id-or-name>")
		return 1
	}
	p, err := projects.GetWithRepo(args[0])
	if err != nil {
		p2, err2 := projects.GetByName(args[0])
		if err2 != nil {
			fmt.Fprintln(os.Stderr, "project not found:", args[0])
			return 1
		}
		fmt.Printf("ID:          %s\nName:        %s\nRepo ID:     %s\nPath:        %s\nGS:          %.2f\nAuto:        %v\nOwner:       %s\n",
			p2.ID, p2.Name, p2.RepoID, p2.SourcePath, p2.GSThreshold, p2.AutoApprove, p2.Owner)
		return 0
	}
	fmt.Printf("ID:          %s\nName:        %s\nRepo:        %s (%s)\nPath:        %s\nBranch:      %s\nGS:          %.2f\nAuto:        %v\nOwner:       %s\n",
		p.ID, p.Name, p.RepoName, p.RepoURL, p.SourcePath, p.RepoDefaultBranch,
		p.GSThreshold, p.AutoApprove, p.Owner)
	return 0
}

func cmdProjectLink(args []string, projects *relayDB.ProjectStore, integrations *relayDB.IntegrationStore) int {
	if len(args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: relay project link <project-name> <type> <external-id> [--trigger <status>] [--label <label>] [--field <field>] [--component <comp>] [--auto-approve]")
		return 1
	}
	fs := flag.NewFlagSet("project link", flag.ContinueOnError)
	trigger := fs.String("trigger", "", "trigger status (Jira) or label (GitHub)")
	label := fs.String("label", "", "label trigger (GitHub Issues)")
	field := fs.String("field", "", "relay-project custom field name (Jira)")
	component := fs.String("component", "", "component filter for A-routing")
	labelPrefix := fs.String("label-prefix", "relay-project:", "relay-project label prefix")
	autoApprove := fs.Bool("auto-approve", false, "auto-approve passing intents from this integration")

	// Reparse from args[3:] to allow flags after positional args
	projectName, intType, externalID := args[0], args[1], args[2]
	if err := fs.Parse(args[3:]); err != nil {
		return 1
	}

	proj, err := projects.GetByName(projectName)
	if err != nil {
		fmt.Fprintln(os.Stderr, "project not found:", projectName)
		return 1
	}

	cfg := core.IntegrationConfig{
		TriggerStatus:           *trigger,
		RelayProjectField:       *field,
		ComponentFilter:         *component,
		LabelTrigger:            *label,
		RelayProjectLabelPrefix: *labelPrefix,
		AutoApprove:             *autoApprove,
	}

	pi := &core.ProjectIntegration{
		ID:         uuid.NewString(),
		ProjectID:  proj.ID,
		Type:       intType,
		ExternalID: externalID,
		Config:     cfg,
		CreatedAt:  time.Now(),
	}
	if err := integrations.Add(pi); err != nil {
		fmt.Fprintln(os.Stderr, "link integration:", err)
		return 1
	}
	fmt.Printf("linked %s (%s:%s) to project %s\n", externalID, intType, externalID, projectName)
	return 0
}

func cmdProjectUnlink(args []string, integrations *relayDB.IntegrationStore) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: relay project unlink <integration-id>")
		return 1
	}
	if err := integrations.Delete(args[0]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println("unlinked")
	return 0
}

// ── intent ────────────────────────────────────────────────────────────────────

func cmdIntent(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: relay intent <create|list|show|approve|reject|assign|from-jira|from-github>")
		return 1
	}

	cfg, store, repos, projects, integrations, err := openStores()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer store.DB().Close()
	_ = repos

	registry := buildRegistry(cfg)
	lc := lifecycle.New(store, registry)
	grove := groveClient.NewClient(cfg.GroveURL)
	scorer := ingestion.NewGranularityScorer(grove, cfg.GSThreshold)
	pipeline := ingestion.NewPipeline(store, scorer, cfg.GSThreshold, projects)
	router := routing.New(integrations, projects)

	switch args[0] {
	case "create":
		return cmdIntentCreate(args[1:], store, projects, pipeline, lc)
	case "list":
		return cmdIntentList(args[1:], store)
	case "show":
		return cmdIntentShow(args[1:], store)
	case "approve":
		return cmdIntentApprove(args[1:], store, lc)
	case "reject":
		return cmdIntentReject(args[1:], store, lc)
	case "assign":
		return cmdIntentAssign(args[1:], store, projects, pipeline, lc)
	case "from-jira":
		return cmdFromJira(args[1:], cfg, store, projects, pipeline, lc, router)
	case "from-github":
		return cmdFromGitHub(args[1:], cfg, store, projects, pipeline, lc, router)
	default:
		fmt.Fprintf(os.Stderr, "unknown intent subcommand: %s\n", args[0])
		return 1
	}
}

func cmdIntentCreate(args []string, store *relayDB.IntentStore, projects *relayDB.ProjectStore, pipeline *ingestion.Pipeline, lc *lifecycle.Lifecycle) int {
	fs := flag.NewFlagSet("intent create", flag.ContinueOnError)
	project := fs.String("project", "", "project name (required)")
	domain := fs.String("domain", "", "optional domain tag")
	author := fs.String("author", os.Getenv("USER"), "author identifier")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if fs.NArg() == 0 || *project == "" {
		fmt.Fprintln(os.Stderr, "usage: relay intent create <description> --project <name> [--domain <d>] [--author <a>]")
		return 1
	}

	proj, err := projects.GetByName(*project)
	if err != nil {
		fmt.Fprintln(os.Stderr, "project not found:", *project)
		return 1
	}

	intent, err := createAndValidate(store, pipeline, lc, fs.Arg(0), proj.ID, *domain, *author, core.SourceNative, "")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	printIntent(intent)
	return 0
}

func cmdIntentList(args []string, store *relayDB.IntentStore) int {
	fs := flag.NewFlagSet("intent list", flag.ContinueOnError)
	status := fs.String("status", "", "filter by status")
	project := fs.String("project", "", "filter by project id")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	intents, err := store.List(relayDB.ListFilter{
		Status:    *status,
		ProjectID: *project,
		Limit:     50,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tPROJECT\tSTATUS\tGS\tDESCRIPTION")
	for _, i := range intents {
		desc := i.Description
		if len(desc) > 55 {
			desc = desc[:52] + "..."
		}
		proj := i.ProjectID
		if len(proj) > 8 {
			proj = proj[:8]
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%.2f\t%s\n", i.ID[:8], proj, i.Status, i.GSScore, desc)
	}
	tw.Flush()
	return 0
}

func cmdIntentShow(args []string, store *relayDB.IntentStore) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: relay intent show <id>")
		return 1
	}
	intent, err := store.Get(args[0])
	if err != nil {
		if err == sql.ErrNoRows {
			fmt.Fprintln(os.Stderr, "not found")
		} else {
			fmt.Fprintln(os.Stderr, err)
		}
		return 1
	}
	printIntent(intent)
	return 0
}

func cmdIntentApprove(args []string, store *relayDB.IntentStore, lc *lifecycle.Lifecycle) int {
	fs := flag.NewFlagSet("intent approve", flag.ContinueOnError)
	by := fs.String("by", os.Getenv("USER"), "approver identifier")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if fs.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "usage: relay intent approve <id> [--by <email>]")
		return 1
	}
	if err := lc.Transition(context.Background(), fs.Arg(0), core.StatusQueued, *by, "manually approved"); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	intent, _ := store.Get(fs.Arg(0))
	printIntent(intent)
	return 0
}

func cmdIntentReject(args []string, store *relayDB.IntentStore, lc *lifecycle.Lifecycle) int {
	fs := flag.NewFlagSet("intent reject", flag.ContinueOnError)
	by := fs.String("by", os.Getenv("USER"), "rejecter identifier")
	reason := fs.String("reason", "", "rejection reason")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if fs.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "usage: relay intent reject <id> [--by <email>] [--reason <text>]")
		return 1
	}
	if err := lc.Transition(context.Background(), fs.Arg(0), core.StatusRejected, *by, *reason); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	intent, _ := store.Get(fs.Arg(0))
	printIntent(intent)
	return 0
}

func cmdIntentAssign(args []string, store *relayDB.IntentStore, projects *relayDB.ProjectStore, pipeline *ingestion.Pipeline, lc *lifecycle.Lifecycle) int {
	fs := flag.NewFlagSet("intent assign", flag.ContinueOnError)
	project := fs.String("project", "", "project name to assign to")
	by := fs.String("by", os.Getenv("USER"), "assignee identifier")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if fs.NArg() == 0 || *project == "" {
		fmt.Fprintln(os.Stderr, "usage: relay intent assign <id> --project <name> [--by <email>]")
		return 1
	}

	proj, err := projects.GetByName(*project)
	if err != nil {
		fmt.Fprintln(os.Stderr, "project not found:", *project)
		return 1
	}

	ctx := context.Background()
	if err := lc.Assign(ctx, fs.Arg(0), proj.ID, *by); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	intent, err := store.Get(fs.Arg(0))
	if err == nil {
		result, err := pipeline.ValidateForProject(ctx, intent)
		if err == nil {
			if result.Passed {
				_ = lc.Transition(ctx, intent.ID, core.StatusQueued, "system", "GS passed after assignment")
			} else {
				_ = lc.MarkNeedsInfo(ctx, intent.ID, result.Feedback)
			}
		}
	}

	fresh, _ := store.Get(fs.Arg(0))
	printIntent(fresh)
	return 0
}

func cmdFromJira(args []string, cfg *config.Config, store *relayDB.IntentStore, projects *relayDB.ProjectStore, pipeline *ingestion.Pipeline, lc *lifecycle.Lifecycle, router *routing.Router) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: relay intent from-jira <ticket-id>")
		return 1
	}
	conn := jiraConn.New(jiraConn.Config{
		URL:   cfg.Jira.URL,
		Token: cfg.Jira.Token,
	})
	ticket, err := conn.FetchTicket(args[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, "fetch ticket:", err)
		return 1
	}

	boardKey := args[0]
	for i, c := range args[0] {
		if c == '-' {
			boardKey = args[0][:i]
			break
		}
	}

	result, err := router.Route("jira", boardKey, ticket)
	if err != nil {
		fmt.Fprintln(os.Stderr, "route:", err)
		return 1
	}
	if result == nil {
		fmt.Fprintln(os.Stderr, "no project integration registered for Jira board:", boardKey)
		return 1
	}

	projectID := result.ProjectID
	if result.Unrouted {
		fmt.Fprintln(os.Stderr, "could not determine project; use relay intent assign <id> --project <name>")
		projectID = ""
	}

	desc := ticket.Title
	if ticket.Description != "" && ticket.Description != ticket.Title {
		desc += " — " + ticket.Description
	}
	intent, err := createAndValidate(store, pipeline, lc, desc, projectID, "", ticket.Author, core.SourceJira, ticket.Ref)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	printIntent(intent)
	return 0
}

func cmdFromGitHub(args []string, cfg *config.Config, store *relayDB.IntentStore, projects *relayDB.ProjectStore, pipeline *ingestion.Pipeline, lc *lifecycle.Lifecycle, router *routing.Router) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: relay intent from-github <owner/repo#number>")
		return 1
	}
	conn := ghConn.New(ghConn.Config{Token: cfg.GitHub.Token})
	ticket, err := conn.FetchTicket(args[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, "fetch issue:", err)
		return 1
	}

	repoID := args[0]
	for i, c := range args[0] {
		if c == '#' {
			repoID = args[0][:i]
			break
		}
	}

	result, err := router.Route("github_issues", repoID, ticket)
	if err != nil {
		fmt.Fprintln(os.Stderr, "route:", err)
		return 1
	}

	projectID := ""
	if result != nil && !result.Unrouted {
		projectID = result.ProjectID
	}

	desc := ticket.Title
	if ticket.Description != "" && ticket.Description != ticket.Title {
		desc += " — " + ticket.Description
	}
	intent, err := createAndValidate(store, pipeline, lc, desc, projectID, "", ticket.Author, core.SourceGitHubIssue, ticket.Ref)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	printIntent(intent)
	return 0
}

// ── helpers ───────────────────────────────────────────────────────────────────

func openStores() (*config.Config, *relayDB.IntentStore, *relayDB.RepoStore, *relayDB.ProjectStore, *relayDB.IntegrationStore, error) {
	cfg, err := config.Load("relay.yaml")
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("config: %w", err)
	}
	sqlDB, err := relayDB.Open(cfg.DBURL)
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("db: %w", err)
	}
	return cfg,
		relayDB.NewIntentStore(sqlDB),
		relayDB.NewRepoStore(sqlDB),
		relayDB.NewProjectStore(sqlDB),
		relayDB.NewIntegrationStore(sqlDB),
		nil
}

func buildRegistry(cfg *config.Config) *integration.Registry {
	reg := integration.NewRegistry()
	if cfg.Jira.URL != "" && cfg.Jira.Token != "" {
		reg.Register(jiraConn.New(jiraConn.Config{
			URL:           cfg.Jira.URL,
			Token:         cfg.Jira.Token,
			WebhookSecret: cfg.Jira.WebhookSecret,
		}))
	}
	if cfg.GitHub.Token != "" {
		reg.Register(ghConn.New(ghConn.Config{
			Token:         cfg.GitHub.Token,
			WebhookSecret: cfg.GitHub.WebhookSecret,
			LabelTrigger:  cfg.GitHub.LabelTrigger,
			AutoApprove:   cfg.GitHub.AutoApprove,
		}))
	}
	return reg
}

func createAndValidate(
	store *relayDB.IntentStore,
	pipeline *ingestion.Pipeline,
	lc *lifecycle.Lifecycle,
	description, projectID, domain, author string,
	source core.IntentSource,
	sourceRef string,
) (*core.Intent, error) {
	ctx := context.Background()
	now := time.Now()

	status := core.StatusDraft
	if projectID == "" {
		status = core.StatusUnrouted
	}

	intent := &core.Intent{
		ID:          uuid.NewString(),
		ProjectID:   projectID,
		Description: description,
		Domain:      domain,
		Source:      source,
		SourceRef:   sourceRef,
		Status:      status,
		Author:      author,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := store.Create(intent); err != nil {
		return nil, fmt.Errorf("create: %w", err)
	}
	_ = store.AppendEvent(&core.IntentEvent{
		IntentID:  intent.ID,
		Type:      "created",
		Actor:     author,
		CreatedAt: now,
	})

	if projectID == "" {
		_ = lc.Unroute(ctx, intent.ID, "system")
		return store.Get(intent.ID)
	}

	_ = lc.Submit(ctx, intent.ID, author)
	result, err := pipeline.ValidateForProject(ctx, intent)
	if err == nil {
		if result.Passed {
			_ = lc.Transition(ctx, intent.ID, core.StatusQueued, "system", "GS check passed")
		} else {
			_ = lc.MarkNeedsInfo(ctx, intent.ID, result.Feedback)
		}
	}
	return store.Get(intent.ID)
}

func printIntent(i *core.Intent) {
	if i == nil {
		return
	}
	fmt.Printf("ID:          %s\n", i.ID)
	fmt.Printf("Project:     %s\n", i.ProjectID)
	fmt.Printf("Status:      %s\n", i.Status)
	fmt.Printf("GS Score:    %.2f\n", i.GSScore)
	fmt.Printf("ICR Symbols: %d\n", i.ICRSymbols)
	fmt.Printf("Source:      %s\n", i.Source)
	fmt.Printf("Description: %s\n", i.Description)
	fmt.Printf("Author:      %s\n", i.Author)
	fmt.Printf("Created:     %s\n", i.CreatedAt.Format("2006-01-02 15:04:05"))
}

func printUsage() {
	fmt.Println(`usage: relay <command> [options]

Commands:
  serve            start the HTTP server
  repo             manage repositories
  project          manage projects
  intent           manage intents
  version          print version

Repo subcommands:
  relay repo add --name <name> [--branch main] <url>
  relay repo list
  relay repo remove <id>

Project subcommands:
  relay project add <name> --repo <repo-name> [--path /] [--owner eng@example.com]
  relay project list
  relay project show <id-or-name>
  relay project link <project> <type> <external-id> [--trigger <status>] [--label <label>] [--field <field>]
  relay project unlink <integration-id>

Intent subcommands:
  relay intent create <desc> --project <name> [--domain <d>] [--author <a>]
  relay intent list [--status <s>] [--project <id>]
  relay intent show <id>
  relay intent approve <id> [--by <email>]
  relay intent reject <id> [--by <email>] [--reason <text>]
  relay intent assign <id> --project <name> [--by <email>]
  relay intent from-jira <ticket-id>
  relay intent from-github <owner/repo#number>`)
}
