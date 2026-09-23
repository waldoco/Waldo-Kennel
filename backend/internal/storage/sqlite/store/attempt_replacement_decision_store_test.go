package store_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/storage/sqlite"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/storage/sqlite/sqlitetest"
)

func TestAttemptReplacementDecisionIsBoundCurrentAndIdempotent(t *testing.T) {
	dataDir := t.TempDir()
	s := sqlitetest.MustOpenAt(t, dataDir)
	ctx := context.Background()
	seedProject(t, s, "replacement")
	space, err := s.EnsureWorkResponsibilitySpace(ctx, "replacement")
	if err != nil {
		t.Fatal(err)
	}
	outcomeID := domain.OutcomeID("out-replacement")
	contract := domain.ContractRevision{ID: "contract-replacement", OutcomeID: outcomeID, Number: 1, Goal: "g", SuccessCriteria: []string{"done"}, Review: "review", Criteria: []domain.ContractCriterion{{ID: "criterion", ContractRevisionID: "contract-replacement", Position: 1, Text: "done"}}}
	if err := s.CreateOutcomeWithContract(ctx, domain.Outcome{ID: outcomeID, SpaceID: space.ID, Title: "o"}, contract, "create-key"); err != nil {
		t.Fatal(err)
	}
	unit := domain.WorkUnit{ID: "unit-replacement", Kind: domain.WorkUnitDirect, Title: "w", ContractRevisionNumber: 1, OutputSummary: "done", EvidenceChecks: []string{"check"}, VerificationRequirement: "review", StopConditions: []string{"stop"}, Provider: domain.HarnessCodex, ModelSelection: domain.ExecutionBindingModelProviderDefault, CriterionIDs: []domain.CriterionID{"criterion"}}
	digest, err := domain.ComputeRunBriefCoreDigest(contract, unit, nil)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := s.AppendPlanRevision(ctx, outcomeID, domain.PlanRevision{ID: "plan-replacement", OutcomeID: outcomeID, ContractRevisionNumber: 1, Status: domain.PlanStatusProposed, Summary: "p", WorkUnits: []domain.WorkUnit{unit}, RoutingDecisions: []domain.WorkUnitRoutingDecision{{WorkUnitID: unit.ID, Decision: domain.RoutingDecision{Status: domain.RoutingDecisionRecommended, PolicyVersion: domain.RoutingPolicyVersion, Role: domain.RoutingRoleWorker, RecommendedCandidateID: string(domain.HarnessCodex), RecommendedProvider: string(domain.HarnessCodex), RecommendedModelSelection: domain.ExecutionBindingModelProviderDefault}}}, RunBriefCoreDigest: digest})
	if err != nil {
		t.Fatal(err)
	}
	if _, found, err := s.ApprovePlanRevision(ctx, outcomeID, plan.ID); err != nil || !found {
		t.Fatalf("approve found=%v err=%v", found, err)
	}
	intent, err := s.AppendRunIntent(ctx, domain.OutcomeRunIntent{ID: "intent-replacement", OutcomeID: outcomeID, Desired: domain.RunIntentRunning, PlanRevisionID: plan.ID, ContractRevisionNumber: 1, RequestKey: "run-key", RequestFingerprint: strings.Repeat("f", 64), RequestedAt: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := s.CreateAttemptWithFence(ctx, ports.AttemptAdmission{OutcomeID: outcomeID, PlanRevisionID: plan.ID, WorkUnitID: unit.ID, ContractRevisionNumber: 1, RunIntentGeneration: intent.Generation, RequestKey: "attempt-key", FenceSubject: "replacement", At: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := sql.Open("sqlite", "file:"+filepath.Join(dataDir, "kennel.db")+"?_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = raw.Close() })
	activeInsert := `INSERT INTO attempt_replacement_decisions(id,outcome_id,predecessor_attempt_id,plan_revision_id,work_unit_id,run_intent_generation,contract_revision_number,action,request_key,request_fingerprint,owner_principal,created_at) VALUES(?,?,?,?,?,?,?,'replace',?,?,'local-owner:apprun-raw',?)`
	if _, err := raw.ExecContext(ctx, activeInsert, "raw-active", outcomeID, attempt.ID, plan.ID, unit.ID, intent.Generation, 1, "raw-active", strings.Repeat("8", 64), time.Now().UTC()); err == nil || !strings.Contains(err.Error(), "authority is not current") {
		t.Fatalf("direct insert active predecessor error=%v", err)
	}
	if _, _, err := s.CreateAttemptReplacementDecision(ctx, domain.AttemptReplacementDecision{ID: "decision-active", OutcomeID: outcomeID, PredecessorAttemptID: attempt.ID, PlanRevisionID: plan.ID, WorkUnitID: unit.ID, RunIntentGeneration: intent.Generation, ContractRevisionNumber: 1, Action: "replace", RequestKey: "decision-active-key", RequestFingerprint: strings.Repeat("9", 64), OwnerPrincipal: "local-owner:apprun-test", CreatedAt: time.Now().UTC()}); err == nil {
		t.Fatal("accepted replacement decision for active predecessor")
	}
	if _, err := s.TransitionAttemptStatus(ctx, outcomeID, attempt.ID, domain.AttemptQueued, domain.AttemptFailed, time.Now().UTC()); err != nil {
		t.Fatalf("terminalize predecessor: %v", err)
	}
	d := domain.AttemptReplacementDecision{ID: "decision-1", OutcomeID: outcomeID, PredecessorAttemptID: attempt.ID, PlanRevisionID: plan.ID, WorkUnitID: unit.ID, RunIntentGeneration: intent.Generation, ContractRevisionNumber: 1, Action: "replace", RequestKey: "decision-key", RequestFingerprint: strings.Repeat("a", 64), OwnerPrincipal: "local-owner:apprun-test", CreatedAt: time.Now().UTC()}
	got, created, err := s.CreateAttemptReplacementDecision(ctx, d)
	if err != nil || !created || got.ID != d.ID {
		t.Fatalf("create got=%+v created=%v err=%v", got, created, err)
	}
	baseInsert := `INSERT INTO attempt_replacement_decisions(id,outcome_id,predecessor_attempt_id,plan_revision_id,work_unit_id,run_intent_generation,contract_revision_number,action,request_key,request_fingerprint,owner_principal,created_at)
		SELECT ?,?,?,?,?,?,?,'replace',?,?,'local-owner:apprun-raw',?`
	if _, err := raw.ExecContext(ctx, baseInsert, "raw-valid", outcomeID, attempt.ID, plan.ID, unit.ID, intent.Generation, 1, "raw-valid", strings.Repeat("0", 64), time.Now().UTC()); err != nil {
		t.Fatalf("direct valid insert: %v", err)
	}
	for name, args := range map[string][]any{
		"wrong outcome":    {"raw-out", "out-missing", attempt.ID, plan.ID, unit.ID, intent.Generation, 1, "raw-out", strings.Repeat("1", 64), time.Now().UTC()},
		"wrong plan":       {"raw-plan", outcomeID, attempt.ID, "plan-missing", unit.ID, intent.Generation, 1, "raw-plan", strings.Repeat("2", 64), time.Now().UTC()},
		"wrong unit":       {"raw-unit", outcomeID, attempt.ID, plan.ID, "unit-missing", intent.Generation, 1, "raw-unit", strings.Repeat("3", 64), time.Now().UTC()},
		"wrong generation": {"raw-generation", outcomeID, attempt.ID, plan.ID, unit.ID, intent.Generation + 1, 1, "raw-generation", strings.Repeat("4", 64), time.Now().UTC()},
		"wrong contract":   {"raw-contract", outcomeID, attempt.ID, plan.ID, unit.ID, intent.Generation, 2, "raw-contract", strings.Repeat("5", 64), time.Now().UTC()},
	} {
		if _, err := raw.ExecContext(ctx, baseInsert, args...); err == nil || !strings.Contains(err.Error(), "authority is not current") {
			t.Fatalf("direct insert %s error=%v", name, err)
		}
	}
	for name, statement := range map[string]string{
		"update": `UPDATE attempt_replacement_decisions SET action='replace' WHERE id='decision-1'`,
		"delete": `DELETE FROM attempt_replacement_decisions WHERE id='decision-1'`,
	} {
		if _, err := raw.ExecContext(ctx, statement); err == nil || !strings.Contains(err.Error(), "immutable") {
			t.Fatalf("%s immutable guard error=%v", name, err)
		}
	}
	replay := d
	replay.ID = "decision-other"
	got, created, err = s.CreateAttemptReplacementDecision(ctx, replay)
	if err != nil || created || got.ID != d.ID {
		t.Fatalf("replay got=%+v created=%v err=%v", got, created, err)
	}
	conflict := d
	conflict.ID = "decision-conflict"
	conflict.RequestFingerprint = strings.Repeat("b", 64)
	_, _, err = s.CreateAttemptReplacementDecision(ctx, conflict)
	var target *ports.AttemptReplacementDecisionConflictError
	if !errors.As(err, &target) {
		t.Fatalf("conflict err=%v", err)
	}
	stale := d
	stale.ID = "decision-stale"
	stale.RequestKey = "stale-key"
	stale.RequestFingerprint = strings.Repeat("c", 64)
	stale.RunIntentGeneration++
	if _, _, err := s.CreateAttemptReplacementDecision(ctx, stale); err == nil {
		t.Fatal("accepted stale generation")
	}
	pausedIntent, err := s.AppendRunIntent(ctx, domain.OutcomeRunIntent{ID: "intent-paused", OutcomeID: outcomeID, Desired: domain.RunIntentPaused, PlanRevisionID: plan.ID, ContractRevisionNumber: 1, Command: domain.RunCommandPause, RequestKey: "pause-key", RequestFingerprint: strings.Repeat("d", 64), ExpectedGenerationSet: true, ExpectedGeneration: intent.Generation, RequestedAt: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.ExecContext(ctx, baseInsert, "raw-paused", outcomeID, attempt.ID, plan.ID, unit.ID, intent.Generation, 1, "raw-paused", strings.Repeat("6", 64), time.Now().UTC()); err == nil || !strings.Contains(err.Error(), "authority is not current") {
		t.Fatalf("direct insert against non-running latest intent error=%v", err)
	}
	notRunning := d
	notRunning.ID = "decision-not-running"
	notRunning.RequestKey = "not-running-key"
	notRunning.RequestFingerprint = strings.Repeat("e", 64)
	notRunning.RunIntentGeneration = pausedIntent.Generation
	if _, _, err := s.CreateAttemptReplacementDecision(ctx, notRunning); err == nil {
		t.Fatal("accepted replacement while run intent paused")
	}
}

func TestAttemptReplacementDecisionConcurrentPeerReplayAndConflict(t *testing.T) {
	// The primary test above proves full authority bindings. This race opens an
	// independent writer pool against its committed fixture, as a second daemon.
	dataDir := t.TempDir()
	s1 := sqlitetest.MustOpenAt(t, dataDir)
	ctx := context.Background()
	seedProject(t, s1, "replacement-peer")
	space, err := s1.EnsureWorkResponsibilitySpace(ctx, "replacement-peer")
	if err != nil {
		t.Fatal(err)
	}
	outcomeID := domain.OutcomeID("out-replacement-peer")
	contract := domain.ContractRevision{ID: "contract-replacement-peer", OutcomeID: outcomeID, Number: 1, Goal: "g", SuccessCriteria: []string{"done"}, Review: "review", Criteria: []domain.ContractCriterion{{ID: "criterion-peer", ContractRevisionID: "contract-replacement-peer", Position: 1, Text: "done"}}}
	if err := s1.CreateOutcomeWithContract(ctx, domain.Outcome{ID: outcomeID, SpaceID: space.ID, Title: "o"}, contract, "create-peer"); err != nil {
		t.Fatal(err)
	}
	unit := domain.WorkUnit{ID: "unit-replacement-peer", Kind: domain.WorkUnitDirect, Title: "w", ContractRevisionNumber: 1, OutputSummary: "done", EvidenceChecks: []string{"check"}, VerificationRequirement: "review", StopConditions: []string{"stop"}, Provider: domain.HarnessCodex, ModelSelection: domain.ExecutionBindingModelProviderDefault, CriterionIDs: []domain.CriterionID{"criterion-peer"}}
	digest, _ := domain.ComputeRunBriefCoreDigest(contract, unit, nil)
	plan, err := s1.AppendPlanRevision(ctx, outcomeID, domain.PlanRevision{ID: "plan-replacement-peer", OutcomeID: outcomeID, ContractRevisionNumber: 1, Status: domain.PlanStatusProposed, Summary: "p", WorkUnits: []domain.WorkUnit{unit}, RoutingDecisions: []domain.WorkUnitRoutingDecision{{WorkUnitID: unit.ID, Decision: domain.RoutingDecision{Status: domain.RoutingDecisionRecommended, PolicyVersion: domain.RoutingPolicyVersion, Role: domain.RoutingRoleWorker, RecommendedCandidateID: string(domain.HarnessCodex), RecommendedProvider: string(domain.HarnessCodex), RecommendedModelSelection: domain.ExecutionBindingModelProviderDefault}}}, RunBriefCoreDigest: digest})
	if err != nil {
		t.Fatal(err)
	}
	if _, found, err := s1.ApprovePlanRevision(ctx, outcomeID, plan.ID); err != nil || !found {
		t.Fatal(err)
	}
	intent, err := s1.AppendRunIntent(ctx, domain.OutcomeRunIntent{ID: "intent-replacement-peer", OutcomeID: outcomeID, Desired: domain.RunIntentRunning, PlanRevisionID: plan.ID, ContractRevisionNumber: 1, RequestKey: "run-peer", RequestFingerprint: strings.Repeat("f", 64), RequestedAt: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := s1.CreateAttemptWithFence(ctx, ports.AttemptAdmission{OutcomeID: outcomeID, PlanRevisionID: plan.ID, WorkUnitID: unit.ID, ContractRevisionNumber: 1, RunIntentGeneration: intent.Generation, RequestKey: "attempt-peer", FenceSubject: "replacement-peer", At: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s1.TransitionAttemptStatus(ctx, outcomeID, attempt.ID, domain.AttemptQueued, domain.AttemptFailed, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	s2 := openPeerStore(t, dataDir)
	d := domain.AttemptReplacementDecision{ID: "decision-peer-1", OutcomeID: outcomeID, PredecessorAttemptID: attempt.ID, PlanRevisionID: plan.ID, WorkUnitID: unit.ID, RunIntentGeneration: intent.Generation, ContractRevisionNumber: 1, Action: "replace", RequestKey: "decision-peer", RequestFingerprint: strings.Repeat("a", 64), OwnerPrincipal: "local-owner:apprun-peer", CreatedAt: time.Now().UTC()}
	results := make(chan error, 2)
	create := func(store *sqlite.Store, value domain.AttemptReplacementDecision) error {
		_, _, err := store.CreateAttemptReplacementDecision(ctx, value)
		return err
	}
	go func() { results <- create(s1, d) }()
	go func() { x := d; x.ID = "decision-peer-2"; results <- create(s2, x) }()
	for i := 0; i < 2; i++ {
		if err := <-results; err != nil {
			t.Fatalf("same semantic peer replay: %v", err)
		}
	}
	x := d
	x.ID = "decision-peer-conflict"
	x.RequestFingerprint = strings.Repeat("b", 64)
	if _, _, err := s2.CreateAttemptReplacementDecision(ctx, x); err == nil {
		t.Fatal("peer conflict accepted")
	}
}
