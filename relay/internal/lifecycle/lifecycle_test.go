package lifecycle

import (
	"testing"

	"github.com/tabladrum/grove-suite/relay/internal/core"
)

func TestValidTransitions(t *testing.T) {
	valid := [][2]core.IntentStatus{
		{core.StatusDraft, core.StatusValidating},
		{core.StatusValidating, core.StatusNeedsInfo},
		{core.StatusValidating, core.StatusQueued},
		{core.StatusValidating, core.StatusRejected},
		{core.StatusNeedsInfo, core.StatusValidating},
	}
	for _, pair := range valid {
		if !isValidTransition(pair[0], pair[1]) {
			t.Errorf("expected %s → %s to be valid", pair[0], pair[1])
		}
	}
}

func TestInvalidTransitions(t *testing.T) {
	invalid := [][2]core.IntentStatus{
		{core.StatusDraft, core.StatusQueued},
		{core.StatusDraft, core.StatusRejected},
		{core.StatusQueued, core.StatusDraft},
		{core.StatusRejected, core.StatusQueued},
		{core.StatusNeedsInfo, core.StatusQueued},
	}
	for _, pair := range invalid {
		if isValidTransition(pair[0], pair[1]) {
			t.Errorf("expected %s → %s to be invalid", pair[0], pair[1])
		}
	}
}

func TestStatusMessage(t *testing.T) {
	if msg := statusMessage(core.StatusQueued, ""); msg == "" {
		t.Error("queued status should produce a message")
	}
	if msg := statusMessage(core.StatusRejected, "too broad"); msg == "" {
		t.Error("rejected status should produce a message")
	}
	if msg := statusMessage(core.StatusNeedsInfo, "scope is too broad"); msg == "" {
		t.Error("needs_info status should produce a message")
	}
}
