package api

import (
	"embed"
	"html/template"
	"net/http"
	"strconv"
	"time"

	"github.com/provasign/provasign/internal/core"
	"github.com/provasign/provasign/internal/db"
	groveClient "github.com/provasign/provasign/internal/grove"
	"github.com/provasign/provasign/internal/integration"
)

//go:embed templates/*.html
var templateFS embed.FS

var dashTmpl = template.Must(
	template.New("dashboard.html").
		Funcs(template.FuncMap{
			"age":   humanAge,
			"slice": func(s string, i, j int) string { return s[i:j] },
		}).
		ParseFS(templateFS, "templates/dashboard.html"),
)

type dashboardData struct {
	Counts       map[string]int
	Intents      []*core.Intent
	Unrouted     []*core.Intent
	Repos        []*core.Repo
	Projects     []*core.Project
	Integrations []integrationStatus
	GroveOK      bool
}

type integrationStatus struct {
	Name string
	OK   bool
}

func dashboardHandler(
	store *db.IntentStore,
	repos *db.RepoStore,
	projects *db.ProjectStore,
	registry *integration.Registry,
	grove *groveClient.Client,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		counts, _ := store.CountByStatus()
		if counts == nil {
			counts = map[string]int{}
		}
		for _, s := range []string{"draft", "unrouted", "validating", "needs_info", "queued", "rejected"} {
			if _, ok := counts[s]; !ok {
				counts[s] = 0
			}
		}

		intents, _ := store.List(db.ListFilter{Limit: 20})
		unrouted, _ := store.ListUnrouted(10)
		repoList, _ := repos.List()
		projectList, _ := projects.List("")

		var statuses []integrationStatus
		for _, name := range registry.Names() {
			statuses = append(statuses, integrationStatus{Name: name, OK: true})
		}

		groveOK := false
		if grove != nil {
			groveOK = grove.Health() == nil
		}

		data := dashboardData{
			Counts:       counts,
			Intents:      intents,
			Unrouted:     unrouted,
			Repos:        repoList,
			Projects:     projectList,
			Integrations: statuses,
			GroveOK:      groveOK,
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := dashTmpl.Execute(w, data); err != nil {
			http.Error(w, "template error: "+err.Error(), http.StatusInternalServerError)
		}
	}
}

func humanAge(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return pluralUnit(int(d.Minutes()), "minute")
	case d < 24*time.Hour:
		return pluralUnit(int(d.Hours()), "hour")
	default:
		return pluralUnit(int(d.Hours()/24), "day")
	}
}

func pluralUnit(n int, unit string) string {
	if n == 1 {
		return "1 " + unit + " ago"
	}
	return strconv.Itoa(n) + " " + unit + "s ago"
}
