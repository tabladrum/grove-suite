package lifecycle

import (
	"context"
	"fmt"
	"time"

	"github.com/provasign/provasign/internal/core"
	"github.com/provasign/provasign/internal/db"
	"github.com/provasign/provasign/internal/integration"
)

var validTransitions = map[core.IntentStatus][]core.IntentStatus{
	core.StatusDraft:      {core.StatusValidating, core.StatusUnrouted},
	core.StatusUnrouted:   {core.StatusValidating},
	core.StatusValidating: {core.StatusNeedsInfo, core.StatusQueued, core.StatusRejected},
	core.StatusNeedsInfo:  {core.StatusValidating},
}

type Lifecycle struct {
	store    *db.IntentStore
	registry *integration.Registry
}

func New(store *db.IntentStore, registry *integration.Registry) *Lifecycle {
	return &Lifecycle{store: store, registry: registry}
}

func (lc *Lifecycle) Transition(ctx context.Context, id string, to core.IntentStatus, actor, detail string) error {
	intent, err := lc.store.Get(id)
	if err != nil {
		return fmt.Errorf("get intent: %w", err)
	}

	if !isValidTransition(intent.Status, to) {
		return fmt.Errorf("invalid transition %s → %s", intent.Status, to)
	}

	now := time.Now()
	switch to {
	case core.StatusQueued:
		if err := lc.store.Approve(id, actor, now); err != nil {
			return fmt.Errorf("approve: %w", err)
		}
	case core.StatusRejected:
		if err := lc.store.Reject(id, actor, detail, now); err != nil {
			return fmt.Errorf("reject: %w", err)
		}
	default:
		if err := lc.store.UpdateStatus(id, to, now); err != nil {
			return fmt.Errorf("update status: %w", err)
		}
	}

	ev := &core.IntentEvent{
		IntentID:  id,
		Type:      string(to),
		Detail:    detail,
		Actor:     actor,
		CreatedAt: now,
	}
	if err := lc.store.AppendEvent(ev); err != nil {
		return fmt.Errorf("append event: %w", err)
	}

	if intent.SourceRef != "" && intent.Source != core.SourceNative {
		lc.postFeedback(intent, to, detail)
	}

	return nil
}

func (lc *Lifecycle) Submit(ctx context.Context, id, actor string) error {
	return lc.Transition(ctx, id, core.StatusValidating, actor, "submitted for validation")
}

func (lc *Lifecycle) Unroute(ctx context.Context, id, actor string) error {
	return lc.Transition(ctx, id, core.StatusUnrouted, actor, "no project resolved; awaiting assignment")
}

func (lc *Lifecycle) Assign(ctx context.Context, id, projectID, actor string) error {
	now := time.Now()
	if err := lc.store.AssignProject(id, projectID, now); err != nil {
		return fmt.Errorf("assign project: %w", err)
	}
	return lc.Transition(ctx, id, core.StatusValidating, actor, "project assigned: "+projectID)
}

func (lc *Lifecycle) MarkNeedsInfo(ctx context.Context, id, feedback string) error {
	intent, err := lc.store.Get(id)
	if err != nil {
		return err
	}
	if err := lc.Transition(ctx, id, core.StatusNeedsInfo, "system", feedback); err != nil {
		return err
	}
	if intent.SourceRef != "" && intent.Source != core.SourceNative {
		if conn, err := lc.registry.Get(string(intent.Source)); err == nil {
			_ = conn.PostComment(intent.SourceRef, "Relay: "+feedback)
		}
	}
	return nil
}

func (lc *Lifecycle) postFeedback(intent *core.Intent, to core.IntentStatus, detail string) {
	conn, err := lc.registry.Get(string(intent.Source))
	if err != nil {
		return
	}
	msg := statusMessage(to, detail)
	if msg != "" {
		_ = conn.PostComment(intent.SourceRef, msg)
	}
}

func statusMessage(status core.IntentStatus, detail string) string {
	switch status {
	case core.StatusValidating:
		return "Relay: intent received and is being validated."
	case core.StatusNeedsInfo:
		return "Relay: " + detail
	case core.StatusQueued:
		return "Relay: intent approved and queued for execution."
	case core.StatusRejected:
		if detail != "" {
			return "Relay: intent rejected — " + detail
		}
		return "Relay: intent rejected."
	}
	return ""
}

func isValidTransition(from, to core.IntentStatus) bool {
	allowed, ok := validTransitions[from]
	if !ok {
		return false
	}
	for _, a := range allowed {
		if a == to {
			return true
		}
	}
	return false
}
