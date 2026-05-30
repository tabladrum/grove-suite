package github

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/tabladrum/grove-suite/relay/internal/core"
)

type Config struct {
	Token         string
	WebhookSecret string
	LabelTrigger  string
	AutoApprove   bool
}

type Connector struct {
	cfg Config
}

func New(cfg Config) *Connector {
	return &Connector{cfg: cfg}
}

func (c *Connector) Name() string { return "github" }

// FetchTicket expects ref in format "owner/repo#123".
func (c *Connector) FetchTicket(ref string) (*core.TicketData, error) {
	owner, repo, number, err := parseRef(ref)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues/%d", owner, repo, number)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.Token)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github fetch %s: %d", ref, resp.StatusCode)
	}

	var issue struct {
		Title  string `json:"title"`
		Body   string `json:"body"`
		User   struct{ Login string } `json:"user"`
		Labels []struct{ Name string } `json:"labels"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&issue); err != nil {
		return nil, err
	}

	labels := make([]string, 0, len(issue.Labels))
	for _, l := range issue.Labels {
		labels = append(labels, l.Name)
	}
	return &core.TicketData{
		Ref:         ref,
		Title:       issue.Title,
		Description: issue.Body,
		Author:      issue.User.Login,
		Labels:      labels,
	}, nil
}

func (c *Connector) PostComment(ref, message string) error {
	owner, repo, number, err := parseRef(ref)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues/%d/comments", owner, repo, number)
	payload := map[string]string{"body": message}
	data, _ := json.Marshal(payload)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("github comment %s: %d", ref, resp.StatusCode)
	}
	return nil
}

func (c *Connector) TransitionTicket(ref, toStatus string) error {
	if strings.ToLower(toStatus) != "closed" && strings.ToLower(toStatus) != "open" {
		return nil
	}
	owner, repo, number, err := parseRef(ref)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues/%d", owner, repo, number)
	payload := map[string]string{"state": strings.ToLower(toStatus)}
	data, _ := json.Marshal(payload)
	req, err := http.NewRequest(http.MethodPatch, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("github transition %s: %d", ref, resp.StatusCode)
	}
	return nil
}

// ValidateWebhook checks X-Hub-Signature-256.
func ValidateWebhook(secret string, body []byte, r *http.Request) bool {
	if secret == "" {
		return true
	}
	sig := r.Header.Get("X-Hub-Signature-256")
	if sig == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(sig), []byte(expected))
}

type WebhookEvent struct {
	Action           string
	IssueRef         string // "owner/repo#number"
	Title            string
	Body             string
	Author           string
	Labels           []string
	HasLabel         bool   // true if the trigger label is present
	RelayProjectHint string // extracted from relay-project: label prefix
}

func ParseWebhook(body io.Reader, labelTrigger string) (*WebhookEvent, error) {
	var payload struct {
		Action string `json:"action"`
		Issue  struct {
			Number int    `json:"number"`
			Title  string `json:"title"`
			Body   string `json:"body"`
			User   struct{ Login string } `json:"user"`
			Labels []struct{ Name string } `json:"labels"`
		} `json:"issue"`
		Repository struct {
			FullName string `json:"full_name"`
		} `json:"repository"`
	}
	if err := json.NewDecoder(body).Decode(&payload); err != nil {
		return nil, err
	}

	labels := make([]string, 0, len(payload.Issue.Labels))
	hasLabel := false
	relayHint := ""
	for _, l := range payload.Issue.Labels {
		labels = append(labels, l.Name)
		if l.Name == labelTrigger {
			hasLabel = true
		}
		// extract relay-project:<name> hint
		const prefix = "relay-project:"
		if strings.HasPrefix(strings.ToLower(l.Name), prefix) {
			relayHint = l.Name[len(prefix):]
		}
	}

	ref := fmt.Sprintf("%s#%d", payload.Repository.FullName, payload.Issue.Number)
	return &WebhookEvent{
		Action:           payload.Action,
		IssueRef:         ref,
		Title:            payload.Issue.Title,
		Body:             payload.Issue.Body,
		Author:           payload.Issue.User.Login,
		Labels:           labels,
		HasLabel:         hasLabel,
		RelayProjectHint: relayHint,
	}, nil
}

func parseRef(ref string) (owner, repo string, number int, err error) {
	parts := strings.SplitN(ref, "#", 2)
	if len(parts) != 2 {
		return "", "", 0, fmt.Errorf("invalid github ref %q (expected owner/repo#number)", ref)
	}
	repoparts := strings.SplitN(parts[0], "/", 2)
	if len(repoparts) != 2 {
		return "", "", 0, fmt.Errorf("invalid github ref %q", ref)
	}
	n, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", "", 0, fmt.Errorf("invalid issue number in %q", ref)
	}
	return repoparts[0], repoparts[1], n, nil
}
