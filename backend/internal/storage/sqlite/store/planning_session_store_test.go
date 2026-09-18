package store_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
)

func planningSessionFixture(revision domain.ContractRevision, now time.Time) domain.PlanningSession {
	contextJSON := []byte(`{"root":"/tmp/provider-project","files":[],"digest":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`)
	return domain.PlanningSession{
		ID: "planning-store", OutcomeID: revision.OutcomeID, ProjectID: "provider-project",
		ContractRevisionID: revision.ID, ContractRevisionNumber: revision.Number, Revision: 1,
		Status: domain.PlanningSessionActive, WaitingOn: domain.PlanningWaitingOwner,
		Binding:             domain.PlanningBinding{Mode: domain.PlanningModeDirectAPI, Provider: "openai", ModelSelection: domain.PlanningModelExplicit, Model: "planner-test"},
		ContextMode:         domain.PlanningContextRepositoryRead,
		PlanningGrantDigest: domain.DigestSHA256([]byte("repository-read-only")), ContextDigest: domain.DigestSHA256(contextJSON), ContextSnapshotJSON: contextJSON,
		RequestKey: "planning-start-key", RequestFingerprint: domain.DigestSHA256([]byte("planning-start")), CreatedAt: now, UpdatedAt: now,
	}
}

func TestPlanningSessionStore_IdempotentTurnsAndCanonicalPlanLink(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	revision := seedProviderPlanOutcome(t, s)
	now := time.Date(2026, 9, 11, 10, 0, 0, 0, time.UTC)
	session := planningSessionFixture(revision, now)

	created, replay, err := s.CreatePlanningSession(ctx, session)
	if err != nil || replay {
		t.Fatalf("create planning session replay=%v err=%v", replay, err)
	}
	replayed, replay, err := s.CreatePlanningSession(ctx, session)
	if err != nil || !replay || replayed.ID != created.ID {
		t.Fatalf("replay planning session = %+v replay=%v err=%v", replayed, replay, err)
	}

	owner := domain.PlanningTurn{
		ID: "planning-turn-owner", Role: domain.PlanningTurnOwner, Kind: domain.PlanningTurnFinalizeRequest,
		Text: "Propose the plan now.", RequestKey: "planning-finalize-key",
		RequestFingerprint: domain.DigestSHA256([]byte("planning-finalize")), CreatedAt: now.Add(time.Second),
	}
	current, storedOwner, replay, err := s.AppendPlanningOwnerTurn(ctx, session.ID, 1, owner)
	if err != nil || replay || current.Revision != 2 || current.WaitingOn != domain.PlanningWaitingProvider || storedOwner.Sequence != 1 {
		t.Fatalf("append owner turn session=%+v turn=%+v replay=%v err=%v", current, storedOwner, replay, err)
	}
	current, replayedOwner, replay, err := s.AppendPlanningOwnerTurn(ctx, session.ID, 1, owner)
	if err != nil || !replay || replayedOwner.ID != storedOwner.ID || current.Revision != 2 {
		t.Fatalf("replay owner turn session=%+v turn=%+v replay=%v err=%v", current, replayedOwner, replay, err)
	}

	run := domain.IntelligenceRun{
		ID: "intel-planning-store", Kind: domain.IntelligenceRunPlanDraft, ProjectID: session.ProjectID,
		OutcomeID: revision.OutcomeID, ContractRevisionID: revision.ID, SourceRevision: revision.Number,
		RequestedProvider: session.Binding.Provider, RequestedModel: session.Binding.Model,
		InputDigest: domain.DigestSHA256([]byte("planning turn input")), Status: domain.IntelligenceRunRunning, CreatedAt: now.Add(2 * time.Second),
	}
	if err := s.CreateIntelligenceRun(ctx, run); err != nil {
		t.Fatalf("create planning intelligence run: %v", err)
	}
	planner := domain.PlanningTurn{
		ID: "planning-turn-planner", ReplyToTurnID: storedOwner.ID, Role: domain.PlanningTurnPlanner,
		Kind: domain.PlanningTurnPlanProposal, Text: "A two-step Plan is ready for review.",
		StructuredPayload: []byte(`{"Kind":"plan_proposal","Message":"A two-step Plan is ready for review."}`),
		IntelligenceRunID: run.ID, CreatedAt: now.Add(3 * time.Second),
	}
	current, err = s.AppendPlanningProviderTurn(ctx, session.ID, 2, planner, domain.PlanningWaitingOwner, "openai", "planner-test", "")
	if err != nil || current.Revision != 3 || current.WaitingOn != domain.PlanningWaitingOwner {
		t.Fatalf("append planner turn session=%+v err=%v", current, err)
	}

	plan := canonicalGraphPlan(t, revision)
	plan.ID = "plan-from-planning-store"
	plan.PlanningSessionID = session.ID
	plan.SourceIntelligenceRunID = run.ID
	saved, err := s.AppendPlanRevision(ctx, revision.OutcomeID, plan)
	if err != nil {
		t.Fatalf("append planning-sourced Plan: %v", err)
	}
	current, found, err := s.GetPlanningSession(ctx, revision.OutcomeID, session.ID)
	if err != nil || !found {
		t.Fatalf("read atomically linked planning session found=%v err=%v", found, err)
	}
	if current.Status != domain.PlanningSessionProposalReady || current.ProposedPlanRevisionID != saved.ID || current.WaitingOn != domain.PlanningWaitingNone {
		t.Fatalf("proposal-ready session = %+v", current)
	}
	got, found, err := s.GetPlanRevisionByPlanningSession(ctx, revision.OutcomeID, session.ID)
	if err != nil || !found || got.PlanningSessionID != session.ID || got.SourceIntelligenceRunID != run.ID {
		t.Fatalf("planning Plan found=%v plan=%+v err=%v", found, got, err)
	}
}

func TestPlanningSessionStore_NativePacketTurnsDoNotClaimSingleConversationRef(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	revision := seedProviderPlanOutcome(t, s)
	now := time.Date(2026, 9, 11, 10, 15, 0, 0, time.UTC)
	session := planningSessionFixture(revision, now)
	session.Binding = domain.PlanningBinding{Mode: domain.PlanningModeNativeHarness, Provider: "codex-app-server", ModelSelection: domain.PlanningModelProviderDefault}
	if _, _, err := s.CreatePlanningSession(ctx, session); err != nil {
		t.Fatal(err)
	}
	for index, nativeRef := range []string{"native-turn-1", "native-turn-2"} {
		owner := domain.PlanningTurn{
			ID: domain.PlanningTurnID(fmt.Sprintf("native-owner-%d", index+1)), Role: domain.PlanningTurnOwner,
			Kind: domain.PlanningTurnMessage, Text: "Continue from Kennel history.",
			RequestKey: fmt.Sprintf("native-owner-key-%d", index+1), RequestFingerprint: domain.DigestSHA256([]byte(fmt.Sprintf("native-owner-%d", index+1))),
			CreatedAt: now.Add(time.Duration(index*2+1) * time.Second),
		}
		waiting, storedOwner, _, err := s.AppendPlanningOwnerTurn(ctx, session.ID, int64(index*2+1), owner)
		if err != nil {
			t.Fatal(err)
		}
		run := domain.IntelligenceRun{
			ID: domain.IntelligenceRunID(fmt.Sprintf("native-run-%d", index+1)), Kind: domain.IntelligenceRunPlanDraft,
			ProjectID: session.ProjectID, OutcomeID: revision.OutcomeID, ContractRevisionID: revision.ID, SourceRevision: revision.Number,
			RequestedProvider: session.Binding.Provider, InputDigest: domain.DigestSHA256([]byte(fmt.Sprintf("native-run-%d", index+1))),
			Status: domain.IntelligenceRunRunning, CreatedAt: now.Add(time.Duration(index*2+2) * time.Second),
		}
		if err := s.CreateIntelligenceRun(ctx, run); err != nil {
			t.Fatal(err)
		}
		planner := domain.PlanningTurn{
			ID: domain.PlanningTurnID(fmt.Sprintf("native-planner-%d", index+1)), ReplyToTurnID: storedOwner.ID,
			Role: domain.PlanningTurnPlanner, Kind: domain.PlanningTurnClarification, Text: "A normalized reply.",
			StructuredPayload: []byte(`{"Kind":"clarification"}`), IntelligenceRunID: run.ID,
			CreatedAt: now.Add(time.Duration(index*2+2) * time.Second),
		}
		current, err := s.AppendPlanningProviderTurn(ctx, session.ID, waiting.Revision, planner, domain.PlanningWaitingOwner, session.Binding.Provider, "gpt-live", nativeRef)
		if err != nil {
			t.Fatalf("append native reply %d: %v", index+1, err)
		}
		if current.NativeConversationRef != "" {
			t.Fatalf("planning session claimed one native thread: %q", current.NativeConversationRef)
		}
	}
}

func TestPlanningSessionStore_RejectsPlanWhenContractChangesBeforeAtomicFinalize(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	revision := seedProviderPlanOutcome(t, s)
	now := time.Date(2026, 9, 11, 10, 30, 0, 0, time.UTC)
	session := planningSessionFixture(revision, now)
	if _, _, err := s.CreatePlanningSession(ctx, session); err != nil {
		t.Fatal(err)
	}
	owner := domain.PlanningTurn{
		ID: "planning-race-owner", Role: domain.PlanningTurnOwner, Kind: domain.PlanningTurnFinalizeRequest,
		Text: "Propose the plan now.", RequestKey: "planning-race-finalize",
		RequestFingerprint: domain.DigestSHA256([]byte("planning-race-finalize")), CreatedAt: now.Add(time.Second),
	}
	waiting, storedOwner, _, err := s.AppendPlanningOwnerTurn(ctx, session.ID, 1, owner)
	if err != nil {
		t.Fatal(err)
	}
	run := domain.IntelligenceRun{
		ID: "intel-planning-race", Kind: domain.IntelligenceRunPlanDraft, ProjectID: session.ProjectID,
		OutcomeID: revision.OutcomeID, ContractRevisionID: revision.ID, SourceRevision: revision.Number,
		RequestedProvider: session.Binding.Provider, RequestedModel: session.Binding.Model,
		InputDigest: domain.DigestSHA256([]byte("planning race input")), Status: domain.IntelligenceRunRunning, CreatedAt: now.Add(2 * time.Second),
	}
	if err := s.CreateIntelligenceRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	planner := domain.PlanningTurn{
		ID: "planning-race-planner", ReplyToTurnID: storedOwner.ID, Role: domain.PlanningTurnPlanner,
		Kind: domain.PlanningTurnPlanProposal, Text: "A Plan is ready.", StructuredPayload: []byte(`{"Kind":"plan_proposal"}`),
		IntelligenceRunID: run.ID, CreatedAt: now.Add(3 * time.Second),
	}
	if _, err := s.AppendPlanningProviderTurn(ctx, session.ID, waiting.Revision, planner, domain.PlanningWaitingOwner, "openai", "planner-test", ""); err != nil {
		t.Fatal(err)
	}
	next := domain.ContractRevision{
		ID: "planning-race-contract-2", OutcomeID: revision.OutcomeID,
		Goal: "Use the new Contract.", SuccessCriteria: []string{"Only current-Contract Plans persist."},
		Review: "Inspect lineage.", AuthorityCeiling: revision.AuthorityCeiling, CreatedAt: now.Add(4 * time.Second),
	}
	if _, err := s.AppendContractRevision(ctx, revision.OutcomeID, revision.Number, next); err != nil {
		t.Fatal(err)
	}
	plan := canonicalGraphPlan(t, revision)
	plan.ID = "stale-plan-from-planning-race"
	plan.PlanningSessionID = session.ID
	plan.SourceIntelligenceRunID = run.ID
	if _, err := s.AppendPlanRevision(ctx, revision.OutcomeID, plan); err == nil {
		t.Fatal("saved a planning Plan after its Contract stopped being current")
	}
	if _, found, err := s.GetLatestPlanRevision(ctx, revision.OutcomeID); err != nil || found {
		t.Fatalf("stale planning Plan survived rollback found=%v err=%v", found, err)
	}
}

func TestPlanningSessionStore_RejectsChangedRequestAndStaleRevision(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	revision := seedProviderPlanOutcome(t, s)
	now := time.Date(2026, 9, 11, 11, 0, 0, 0, time.UTC)
	session := planningSessionFixture(revision, now)
	if _, _, err := s.CreatePlanningSession(ctx, session); err != nil {
		t.Fatal(err)
	}
	changed := session
	changed.RequestFingerprint = domain.DigestSHA256([]byte("changed semantics"))
	_, _, err := s.CreatePlanningSession(ctx, changed)
	var requestConflict *ports.PlanningRequestConflictError
	if !errors.As(err, &requestConflict) {
		t.Fatalf("changed request error = %v", err)
	}

	owner := domain.PlanningTurn{ID: "stale-owner", Role: domain.PlanningTurnOwner, Kind: domain.PlanningTurnMessage, Text: "Inspect first.", RequestKey: "stale-owner-key", RequestFingerprint: domain.DigestSHA256([]byte("stale-owner")), CreatedAt: now.Add(time.Second)}
	_, _, _, err = s.AppendPlanningOwnerTurn(ctx, session.ID, 99, owner)
	var revisionConflict *ports.PlanningSessionRevisionConflictError
	if !errors.As(err, &revisionConflict) || revisionConflict.Current != 1 {
		t.Fatalf("stale revision error = %v", err)
	}
}

func TestPlanningSessionStore_RecoversCrashAfterOwnerTurnBeforeRunCreation(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	revision := seedProviderPlanOutcome(t, s)
	now := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	session := planningSessionFixture(revision, now)
	if _, _, err := s.CreatePlanningSession(ctx, session); err != nil {
		t.Fatal(err)
	}
	owner := domain.PlanningTurn{
		ID: "crash-window-owner", Role: domain.PlanningTurnOwner, Kind: domain.PlanningTurnMessage,
		Text: "Inspect the bounded context.", RequestKey: "crash-window-key",
		RequestFingerprint: domain.DigestSHA256([]byte("crash-window")), CreatedAt: now.Add(time.Second),
	}
	waiting, stored, replay, err := s.AppendPlanningOwnerTurn(ctx, session.ID, 1, owner)
	if err != nil || replay || waiting.WaitingOn != domain.PlanningWaitingProvider {
		t.Fatalf("persist crash-window turn session=%+v replay=%v err=%v", waiting, replay, err)
	}
	// Simulate process death at the exact boundary before CreateIntelligenceRun.
	recovered, err := s.RecoverInterruptedPlanningSessions(ctx, now.Add(2*time.Second))
	if err != nil || recovered != 1 {
		t.Fatalf("recover interrupted planning sessions=%d err=%v", recovered, err)
	}
	got, found, err := s.GetPlanningSession(ctx, revision.OutcomeID, session.ID)
	if err != nil || !found {
		t.Fatalf("read recovered session found=%v err=%v", found, err)
	}
	if got.WaitingOn != domain.PlanningWaitingOwner || got.Revision != 3 || got.LastFailureCode != "PLANNING_REPLY_AMBIGUOUS" {
		t.Fatalf("recovered session=%+v", got)
	}
	// The original key is a read-only replay. It must not create another owner
	// turn or cause the provider call to be retried implicitly.
	got, replayed, replay, err := s.AppendPlanningOwnerTurn(ctx, session.ID, 1, owner)
	if err != nil || !replay || replayed.ID != stored.ID || got.Revision != 3 {
		t.Fatalf("replay after recovery session=%+v turn=%+v replay=%v err=%v", got, replayed, replay, err)
	}
	turns, err := s.ListPlanningTurns(ctx, session.ID)
	if err != nil || len(turns) != 1 {
		t.Fatalf("turns after recovery=%+v err=%v", turns, err)
	}
}

func TestPlanStore_CurrentContractGuardRejectsSessionlessStaleBinding(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	revision := seedProviderPlanOutcome(t, s)
	now := time.Date(2026, 9, 11, 12, 30, 0, 0, time.UTC)

	// Control: a sessionless Plan bound to the current Contract inserts.
	current := canonicalGraphPlan(t, revision)
	current.ID = "plan-guard-current"
	if _, err := s.AppendPlanRevision(ctx, revision.OutcomeID, current); err != nil {
		t.Fatalf("append current-bound Plan: %v", err)
	}

	next := domain.ContractRevision{
		ID: "plan-guard-contract-2", OutcomeID: revision.OutcomeID,
		Goal: "Use the new Contract.", SuccessCriteria: []string{"Only current-Contract Plans persist."},
		Review: "Inspect lineage.", AuthorityCeiling: revision.AuthorityCeiling, CreatedAt: now.Add(time.Second),
	}
	if _, err := s.AppendContractRevision(ctx, revision.OutcomeID, revision.Number, next); err != nil {
		t.Fatal(err)
	}

	// A sessionless insert bound to the superseded revision must abort in the
	// same transaction - the one-shot lane has no planning-source trigger.
	stale := canonicalGraphPlan(t, revision)
	stale.ID = "plan-guard-stale"
	_, err := s.AppendPlanRevision(ctx, revision.OutcomeID, stale)
	var staleErr *ports.PlanContractStaleError
	if !errors.As(err, &staleErr) || staleErr.ContractRevision != revision.Number {
		t.Fatalf("stale sessionless Plan error = %v, want PlanContractStaleError for revision %d", err, revision.Number)
	}
	latest, found, err := s.GetLatestPlanRevision(ctx, revision.OutcomeID)
	if err != nil || !found || latest.ID != current.ID {
		t.Fatalf("stale Plan survived: latest=%+v found=%v err=%v", latest, found, err)
	}
}
