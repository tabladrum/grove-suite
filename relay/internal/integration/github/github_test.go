package github

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestValidateWebhook_Valid(t *testing.T) {
	secret := "gh-secret"
	body := []byte(`{"action":"labeled"}`)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(string(body)))
	req.Header.Set("X-Hub-Signature-256", sig)

	if !ValidateWebhook(secret, body, req) {
		t.Error("expected valid signature to pass")
	}
}

func TestValidateWebhook_Invalid(t *testing.T) {
	secret := "gh-secret"
	body := []byte(`{"action":"labeled"}`)
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(string(body)))
	req.Header.Set("X-Hub-Signature-256", "sha256=badvalue")

	if ValidateWebhook(secret, body, req) {
		t.Error("expected invalid signature to fail")
	}
}

func TestParseWebhook_LabelTrigger(t *testing.T) {
	payload := `{
		"action": "labeled",
		"issue": {
			"number": 99,
			"title": "Fix login timeout",
			"body": "Users are getting logged out prematurely",
			"user": {"login": "alice"},
			"labels": [{"name": "relay"}, {"name": "bug"}]
		},
		"repository": {"full_name": "acme/backend"}
	}`
	ev, err := ParseWebhook(strings.NewReader(payload), "relay")
	if err != nil {
		t.Fatal(err)
	}
	if ev.IssueRef != "acme/backend#99" {
		t.Errorf("IssueRef = %q, want acme/backend#99", ev.IssueRef)
	}
	if !ev.HasLabel {
		t.Error("expected HasLabel=true")
	}
	if ev.Author != "alice" {
		t.Errorf("Author = %q, want alice", ev.Author)
	}
}

func TestParseWebhook_NoTriggerLabel(t *testing.T) {
	payload := `{
		"action": "labeled",
		"issue": {
			"number": 5,
			"title": "Bug",
			"body": "",
			"user": {"login": "bob"},
			"labels": [{"name": "bug"}]
		},
		"repository": {"full_name": "acme/frontend"}
	}`
	ev, err := ParseWebhook(strings.NewReader(payload), "relay")
	if err != nil {
		t.Fatal(err)
	}
	if ev.HasLabel {
		t.Error("expected HasLabel=false")
	}
}

func TestParseRef(t *testing.T) {
	owner, repo, number, err := parseRef("acme/backend#42")
	if err != nil {
		t.Fatal(err)
	}
	if owner != "acme" || repo != "backend" || number != 42 {
		t.Errorf("got %s/%s#%d", owner, repo, number)
	}
}

func TestParseRef_Invalid(t *testing.T) {
	if _, _, _, err := parseRef("invalid"); err == nil {
		t.Error("expected error for invalid ref")
	}
}
