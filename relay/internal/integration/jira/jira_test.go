package jira

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
	secret := "test-secret"
	body := []byte(`{"webhookEvent":"jira:issue_updated"}`)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(string(body)))
	req.Header.Set("X-Hub-Signature", sig)

	if !ValidateWebhook(secret, body, req) {
		t.Error("expected valid signature to pass")
	}
}

func TestValidateWebhook_Invalid(t *testing.T) {
	secret := "test-secret"
	body := []byte(`{"webhookEvent":"jira:issue_updated"}`)

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(string(body)))
	req.Header.Set("X-Hub-Signature", "sha256=deadbeef")

	if ValidateWebhook(secret, body, req) {
		t.Error("expected invalid signature to fail")
	}
}

func TestValidateWebhook_EmptySecret(t *testing.T) {
	body := []byte(`{}`)
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(string(body)))
	if !ValidateWebhook("", body, req) {
		t.Error("empty secret should always pass")
	}
}

func TestParseWebhook(t *testing.T) {
	payload := `{
		"webhookEvent": "jira:issue_updated",
		"issue": {
			"key": "AUTH-42",
			"fields": {
				"summary": "Add rate limiting",
				"status": {"name": "Ready for Relay"}
			}
		}
	}`
	ev, err := ParseWebhook(strings.NewReader(payload), "Relay Project")
	if err != nil {
		t.Fatal(err)
	}
	if ev.IssueKey != "AUTH-42" {
		t.Errorf("IssueKey = %q, want AUTH-42", ev.IssueKey)
	}
	if ev.Summary != "Add rate limiting" {
		t.Errorf("Summary = %q, want 'Add rate limiting'", ev.Summary)
	}
	if ev.Status != "Ready for Relay" {
		t.Errorf("Status = %q", ev.Status)
	}
}
