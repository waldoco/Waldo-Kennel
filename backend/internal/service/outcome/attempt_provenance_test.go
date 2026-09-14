package outcome_test

import (
	"context"
	"testing"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
)

// The Attempt read model carries the persisted protocol-negotiation episode
// for each session binding, so Mission Control answers "which provider build
// and negotiated surface ran this work" from durable evidence (C1.6).
func TestGetAttemptSurfacesRecordedProtocolProvenance(t *testing.T) {
	svc, store, _, _, outcomeID, planID := newAttemptHarness(t)
	ctx := context.Background()

	view, err := svc.StartAttempt(ctx, outcomeID, startInput(planID))
	if err != nil {
		t.Fatalf("start attempt: %v", err)
	}
	if len(view.Sessions) != 1 {
		t.Fatalf("sessions = %d, want the one admission binding", len(view.Sessions))
	}
	ref := view.Sessions[0]

	// Before anything is recorded the binding has no provenance, and that is
	// absence, not an error.
	if len(view.ProtocolProvenance) != 0 {
		t.Fatalf("provenance = %+v, want none recorded yet", view.ProtocolProvenance)
	}

	episode := domain.ChatProtocolProvenance{
		SessionID:        ref.SessionID,
		Seq:              1,
		Harness:          domain.HarnessCodex,
		Provider:         "codex app-server",
		InstalledVersion: "0.154.0",
		GeneratedFrom:    "0.153.4",
		ProtocolDigest:   "821a34c2aebae893",
		GeneratedDigest:  "821a34c2aebae893",
		MatchesGenerated: true,
		NegotiatedAt:     ref.BoundAt.Add(-time.Second),
	}
	store.mu.Lock()
	store.provenance = map[string][]domain.ChatProtocolProvenance{ref.SessionID: {episode}}
	store.mu.Unlock()

	reread, err := svc.GetAttempt(ctx, outcomeID, view.Attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := reread.ProtocolProvenance[ref.ID]
	if !ok {
		t.Fatalf("provenance missing for binding %s", ref.ID)
	}
	if got.ProtocolDigest != episode.ProtocolDigest || got.InstalledVersion != episode.InstalledVersion || !got.MatchesGenerated {
		t.Fatalf("provenance = %+v, want the recorded episode", got)
	}
}
