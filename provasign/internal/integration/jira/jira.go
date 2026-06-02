package jira

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/provasign/provasign/internal/core"
)

type Config struct {
	URL           string
	Token         string
	WebhookSecret string
}

type Connector struct {
	cfg Config
}

func New(cfg Config) *Connector {
	return &Connector{cfg: cfg}
}

func (c *Connector) Name() string { return "jira" }

func (c *Connector) FetchTicket(ref string) (*core.TicketData, error) {
	url := fmt.Sprintf("%s/rest/api/3/issue/%s", strings.TrimRight(c.cfg.URL, "/"), ref)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.Token)
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jira fetch %s: %d", ref, resp.StatusCode)
	}

	var body struct {
		Key    string `json:"key"`
		Fields struct {
			Summary     string `json:"summary"`
			Description any    `json:"description"`
			Priority    struct{ Name string } `json:"priority"`
			Labels      []string `json:"labels"`
			Reporter    struct{ DisplayName string } `json:"reporter"`
		} `json:"fields"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}

	desc := extractJiraDescription(body.Fields.Description)
	return &core.TicketData{
		Ref:         body.Key,
		Title:       body.Fields.Summary,
		Description: desc,
		Priority:    body.Fields.Priority.Name,
		Labels:      body.Fields.Labels,
		Author:      body.Fields.Reporter.DisplayName,
	}, nil
}

func (c *Connector) PostComment(ref, message string) error {
	url := fmt.Sprintf("%s/rest/api/3/issue/%s/comment", strings.TrimRight(c.cfg.URL, "/"), ref)
	payload := map[string]any{
		"body": map[string]any{
			"type":    "doc",
			"version": 1,
			"content": []map[string]any{
				{
					"type": "paragraph",
					"content": []map[string]any{
						{"type": "text", "text": message},
					},
				},
			},
		},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("jira comment %s: %d", ref, resp.StatusCode)
	}
	return nil
}

func (c *Connector) TransitionTicket(ref, toStatus string) error {
	transID, err := c.findTransitionID(ref, toStatus)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/rest/api/3/issue/%s/transitions", strings.TrimRight(c.cfg.URL, "/"), ref)
	payload := map[string]any{"transition": map[string]string{"id": transID}}
	data, _ := json.Marshal(payload)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("jira transition %s → %s: %d", ref, toStatus, resp.StatusCode)
	}
	return nil
}

func (c *Connector) findTransitionID(ref, toStatus string) (string, error) {
	url := fmt.Sprintf("%s/rest/api/3/issue/%s/transitions", strings.TrimRight(c.cfg.URL, "/"), ref)
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("Authorization", "Bearer "+c.cfg.Token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var body struct {
		Transitions []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"transitions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", err
	}
	lower := strings.ToLower(toStatus)
	for _, t := range body.Transitions {
		if strings.ToLower(t.Name) == lower {
			return t.ID, nil
		}
	}
	return "", fmt.Errorf("transition %q not found for %s", toStatus, ref)
}

// ValidateWebhook checks the HMAC-SHA256 signature from Jira.
func ValidateWebhook(secret string, body []byte, r *http.Request) bool {
	sig := r.Header.Get("X-Hub-Signature")
	if sig == "" {
		sig = r.Header.Get("X-Atlassian-Token")
	}
	if secret == "" {
		return true
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(sig), []byte(expected))
}

// ParseWebhook parses a Jira webhook payload into the relevant fields.
type WebhookEvent struct {
	EventType        string
	IssueKey         string
	Summary          string
	Status           string
	Component        string
	RelayProjectHint string
}

// ParseWebhook parses a Jira webhook payload. relayProjectField is the Jira
// custom field name to look up for explicit routing (e.g. "Relay Project").
func ParseWebhook(body io.Reader, relayProjectField string) (*WebhookEvent, error) {
	var payload struct {
		WebhookEvent string `json:"webhookEvent"`
		Issue        struct {
			Key    string `json:"key"`
			Fields map[string]json.RawMessage `json:"fields"`
		} `json:"issue"`
	}
	if err := json.NewDecoder(body).Decode(&payload); err != nil {
		return nil, err
	}

	fields := payload.Issue.Fields
	ev := &WebhookEvent{
		EventType: payload.WebhookEvent,
		IssueKey:  payload.Issue.Key,
	}

	if raw, ok := fields["summary"]; ok {
		_ = json.Unmarshal(raw, &ev.Summary)
	}
	if raw, ok := fields["status"]; ok {
		var s struct{ Name string `json:"name"` }
		if json.Unmarshal(raw, &s) == nil {
			ev.Status = s.Name
		}
	}
	if raw, ok := fields["components"]; ok {
		var comps []struct{ Name string `json:"name"` }
		if json.Unmarshal(raw, &comps) == nil && len(comps) > 0 {
			ev.Component = comps[0].Name
		}
	}
	if relayProjectField != "" {
		if raw, ok := fields[relayProjectField]; ok {
			// custom fields can be string or object with "value"
			var strVal string
			if json.Unmarshal(raw, &strVal) == nil {
				ev.RelayProjectHint = strVal
			} else {
				var obj struct{ Value string `json:"value"` }
				if json.Unmarshal(raw, &obj) == nil {
					ev.RelayProjectHint = obj.Value
				}
			}
		}
	}
	return ev, nil
}

func extractJiraDescription(v any) string {
	if v == nil {
		return ""
	}
	switch d := v.(type) {
	case string:
		return d
	case map[string]any:
		return extractAtlassianDoc(d)
	}
	return fmt.Sprintf("%v", v)
}

func extractAtlassianDoc(doc map[string]any) string {
	content, ok := doc["content"].([]any)
	if !ok {
		return ""
	}
	var parts []string
	for _, block := range content {
		bMap, ok := block.(map[string]any)
		if !ok {
			continue
		}
		inner, ok := bMap["content"].([]any)
		if !ok {
			continue
		}
		for _, item := range inner {
			iMap, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if text, ok := iMap["text"].(string); ok {
				parts = append(parts, text)
			}
		}
	}
	return strings.Join(parts, " ")
}
