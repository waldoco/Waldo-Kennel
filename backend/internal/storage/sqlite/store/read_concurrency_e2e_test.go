package store_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/httpd/apierr"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/service/outcome"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/storage/sqlite"
)

// e2eSpawner is a minimal ports.AttemptSessionSpawner: it never launches a
// real provider process, but it DOES allocate a distinct real temp directory
// per spawn, so concurrently admitted Attempts can be proven to have gotten
// genuinely separate workspaces rather than sharing one.
type e2eSpawner struct {
	t *testing.T

	mu         sync.Mutex
	spawnN     int
	spawned    []ports.AttemptSpawnRequest
	workspaces []string
}

func (sp *e2eSpawner) ProfileReadiness(context.Context, domain.ProjectID, domain.ExecutionBinding, *domain.AttemptExecutionPolicy) (ports.AgentProfileReadiness, error) {
	return ports.AgentProfileReadiness{Ready: true, Detail: "fake profile ready"}, nil
}

func (sp *e2eSpawner) Spawn(_ context.Context, req ports.AttemptSpawnRequest) (ports.AttemptSpawnResult, error) {
	sp.mu.Lock()
	sp.spawnN++
	n := sp.spawnN
	sp.spawned = append(sp.spawned, req)
	sp.mu.Unlock()

	workspace := sp.t.TempDir()
	sp.mu.Lock()
	sp.workspaces = append(sp.workspaces, workspace)
	sp.mu.Unlock()
	rec := domain.SessionRecord{
		ID:      domain.SessionID("sess-e2e-" + itoa(n)),
		Mode:    domain.SessionModeTUI,
		Harness: req.Harness,
		Metadata: domain.SessionMetadata{
			WorkspacePath: workspace,
		},
		Activity: domain.Activity{State: domain.ActivityActive, LastActivityAt: time.Now().UTC()},
	}
	bound, err := req.ExecutionPolicy.BindWorkspaceRoot(workspace)
	if err != nil {
		return ports.AttemptSpawnResult{}, err
	}
	return ports.AttemptSpawnResult{Session: domain.Session{SessionRecord: rec}, ExecutionPolicy: &bound}, nil
}

func (sp *e2eSpawner) Terminate(context.Context, domain.ProjectID, string) (ports.TerminationResult, error) {
	return ports.TerminationResult{ProviderStopped: true, WorkspaceFreed: true}, nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := ""
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	return digits
}

// e2eHeartbeats is an empty heartbeat source: this test never needs a
// recorded heartbeat fact for its assertions.
type e2eHeartbeats struct{}

func (e2eHeartbeats) GetSession(context.Context, domain.SessionID) (domain.SessionRecord, bool, error) {
	return domain.SessionRecord{}, false, nil
}

// seedReadConcurrencyPlan directly persists (bypassing the intelligence
// layer, exactly like seedApprovedPlan above) an approved Plan with two
// independent read-only WorkUnits and one independent write-capable
// WorkUnit, so the real StartAttempt admission/fence path can be exercised
// against real SQLite without needing a live LLM provider or a live
// worktree-writing Git operation, which this slice does not change.
func seedReadConcurrencyPlan(t *testing.T, s *sqlite.Store, projectID string) (domain.PlanRevision, domain.OutcomeID) {
	t.Helper()
	ctx := context.Background()
	seedProject(t, s, projectID)
	space, err := s.EnsureWorkResponsibilitySpace(ctx, domain.ProjectID(projectID))
	if err != nil {
		t.Fatalf("ensure space: %v", err)
	}
	outcomeRecord := domain.Outcome{ID: domain.OutcomeID("out-" + projectID), SpaceID: space.ID, Title: "Read Concurrency E2E"}
	contract := domain.ContractRevision{
		ID: domain.ContractRevisionID("cr-" + projectID), OutcomeID: outcomeRecord.ID, Number: 1,
		Goal: "Prove concurrent reads and serialized writes.", SuccessCriteria: []string{"reads run together and writes stay exclusive"},
		Review: "Deterministic checks.",
		Criteria: []domain.ContractCriterion{
			{ID: "crit-main", ContractRevisionID: domain.ContractRevisionID("cr-" + projectID), Position: 1, Text: "reads run together and writes stay exclusive"},
		},
		AuthorityCeiling: domain.ProposedAuthority{ReadWorkspace: true, WriteWorkspace: true, ExecuteLocal: true},
	}
	if err := s.CreateOutcomeWithContract(ctx, outcomeRecord, contract, "rk-create-"+projectID); err != nil {
		t.Fatalf("create outcome: %v", err)
	}

	// Every unit redundantly covers the same single contract criterion, so
	// ValidateAgainstContract's coverage check is satisfied without implying
	// any of them are independently provable — this test's subject is
	// admission/fence concurrency, not proof/verification.
	readOnly := func(id string) domain.WorkUnit {
		return domain.WorkUnit{
			ID: domain.WorkUnitID(id), Kind: domain.WorkUnitDirect, Title: "Read " + id,
			ContractRevisionNumber: 1, OutputSummary: id + " output", EvidenceChecks: []string{id + " evidence"},
			VerificationRequirement: "verify " + id, StopConditions: []string{"stop"}, CriterionIDs: []domain.CriterionID{"crit-main"},
			Provider: domain.HarnessCodex, ModelSelection: domain.ExecutionBindingModelProviderDefault,
			RequiredCapabilities: []string{domain.CapabilityWorktreeRead},
		}
	}
	writeUnit := domain.WorkUnit{
		ID: "wu-write", Kind: domain.WorkUnitDirect, Title: "Write", ContractRevisionNumber: 1,
		OutputSummary: "write output", EvidenceChecks: []string{"write evidence"},
		VerificationRequirement: "verify write", StopConditions: []string{"stop"}, CriterionIDs: []domain.CriterionID{"crit-main"},
		Provider: domain.HarnessCodex, ModelSelection: domain.ExecutionBindingModelProviderDefault,
		RequiredCapabilities: []string{domain.CapabilityWorktreeRead, domain.CapabilityWorktreeWrite},
	}
	units := []domain.WorkUnit{readOnly("wu-read-1"), readOnly("wu-read-2"), writeUnit}
	grants := []domain.CapabilityGrant{
		{ID: domain.CapabilityGrantID("cg-read-" + projectID), Name: domain.CapabilityWorktreeRead, Scope: "worktree/*"},
		{ID: domain.CapabilityGrantID("cg-write-" + projectID), Name: domain.CapabilityWorktreeWrite, Scope: "worktree/*"},
	}
	digest, err := domain.ComputePlanRunBriefCoreDigest(contract, units, grants)
	if err != nil {
		t.Fatalf("compute digest: %v", err)
	}
	routing := make([]domain.WorkUnitRoutingDecision, 0, len(units))
	for _, unit := range units {
		routing = append(routing, domain.WorkUnitRoutingDecision{
			WorkUnitID: unit.ID,
			Decision: domain.RoutingDecision{
				Status: domain.RoutingDecisionRecommended, PolicyVersion: domain.RoutingPolicyVersion,
				Role: domain.RoutingRoleWorker, RecommendedCandidateID: string(domain.HarnessCodex),
				RecommendedProvider: string(domain.HarnessCodex), RecommendedModelSelection: domain.ExecutionBindingModelProviderDefault,
			},
		})
	}
	plan, err := s.AppendPlanRevision(ctx, outcomeRecord.ID, domain.PlanRevision{
		ID: domain.PlanRevisionID("plan-" + projectID), OutcomeID: outcomeRecord.ID, ContractRevisionNumber: 1,
		Status: domain.PlanStatusProposed, Summary: "Two independent reads plus one independent write",
		WorkUnits: units, Grants: grants, RoutingDecisions: routing, RunBriefCoreDigest: digest,
	})
	if err != nil {
		t.Fatalf("append plan: %v", err)
	}
	approved, found, err := s.ApprovePlanRevision(ctx, outcomeRecord.ID, plan.ID)
	if err != nil || !found {
		t.Fatalf("approve plan: found=%v err=%v", found, err)
	}
	return approved, outcomeRecord.ID
}

// TestReadConcurrencyEndToEnd drives the REAL production entrypoint
// (Service.StartAttempt, the same method the HTTP controller calls) against
// REAL SQLite, proving the #122 first slice through the actual code path a
// live request takes rather than only through scheduler-internal unit tests:
//
//  1. Two independent read-only WorkUnits are both admitted (Queued) with
//     distinct Attempts and distinct real workspace directories, while
//     neither is finished — the ADR 0009 §6 guarantee.
//  2. The independent write-capable WorkUnit is refused while either read
//     Attempt is still open, exactly preserving today's exclusive-write
//     guarantee.
//  3. Once both reads are ended, the write-capable WorkUnit is admitted.
//  4. A second concurrent write-capable admission attempt is refused, exactly
//     as before this slice.
func TestReadConcurrencyEndToEnd(t *testing.T) {
	s := newTestStore(t)
	plan, outcomeID := seedReadConcurrencyPlan(t, s, "read-e2e")
	spawner := &e2eSpawner{t: t}
	svc := outcome.New(s, nil).WithExecution(spawner, e2eHeartbeats{})
	ctx := context.Background()

	read1, err := svc.StartAttempt(ctx, outcomeID, outcome.StartAttemptInput{
		PlanRevisionID: plan.ID, WorkUnitID: "wu-read-1", RequestKey: "rk-read-1",
	})
	if err != nil {
		t.Fatalf("start read-only wu-read-1: %v", err)
	}
	if read1.Attempt.Status != domain.AttemptRunning {
		t.Fatalf("wu-read-1 attempt status = %s, want running", read1.Attempt.Status)
	}

	// The second independent read-only WorkUnit must be admissible WHILE the
	// first is still open — this is the actual behavior this slice adds.
	read2, err := svc.StartAttempt(ctx, outcomeID, outcome.StartAttemptInput{
		PlanRevisionID: plan.ID, WorkUnitID: "wu-read-2", RequestKey: "rk-read-2",
	})
	if err != nil {
		t.Fatalf("start read-only wu-read-2 while wu-read-1 is active: %v", err)
	}
	if read2.Attempt.Status != domain.AttemptRunning {
		t.Fatalf("wu-read-2 attempt status = %s, want running", read2.Attempt.Status)
	}
	if read1.Attempt.ID == read2.Attempt.ID {
		t.Fatalf("expected two distinct Attempts, got the same id twice: %s", read1.Attempt.ID)
	}
	if len(read1.Sessions) != 1 || len(read2.Sessions) != 1 {
		t.Fatalf("expected exactly one session ref per attempt, got %d and %d", len(read1.Sessions), len(read2.Sessions))
	}
	spawner.mu.Lock()
	workspaces := append([]string(nil), spawner.workspaces...)
	spawner.mu.Unlock()
	if len(workspaces) != 2 || workspaces[0] == "" || workspaces[1] == "" || workspaces[0] == workspaces[1] {
		t.Fatalf("expected two distinct real workspace directories, got %v", workspaces)
	}

	// The independent write-capable WorkUnit must NOT be admissible while
	// either read-only Attempt is still open.
	_, err = svc.StartAttempt(ctx, outcomeID, outcome.StartAttemptInput{
		PlanRevisionID: plan.ID, WorkUnitID: "wu-write", RequestKey: "rk-write-blocked",
	})
	var apiErr *apierr.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("start wu-write while reads are active: err = %v, want an apierr.Error refusal", err)
	}
	if apiErr.Code != outcome.CodeNoRunnableWorkUnit && apiErr.Code != "WORK_UNIT_NOT_RUNNABLE" {
		t.Fatalf("start wu-write while reads are active: code = %s, want a not-runnable refusal", apiErr.Code)
	}

	// End both reads (as a real Attempt lifecycle would once terminated):
	// transition each to a terminal status and release its custody fence.
	// Cancelled is a legal terminal transition directly from Running; this
	// test's subject is admission/fence concurrency, not the
	// Verification/Acceptance path a Succeeded status would imply.
	if _, err := s.TransitionAttemptStatus(ctx, outcomeID, read1.Attempt.ID, domain.AttemptRunning, domain.AttemptCancelled, time.Now().UTC()); err != nil {
		t.Fatalf("end wu-read-1: %v", err)
	}
	if _, err := s.TransitionAttemptStatus(ctx, outcomeID, read2.Attempt.ID, domain.AttemptRunning, domain.AttemptCancelled, time.Now().UTC()); err != nil {
		t.Fatalf("end wu-read-2: %v", err)
	}
	if _, err := s.ReleaseFenceForAttempt(ctx, read1.Attempt.ID, "reads-done", time.Now().UTC()); err != nil {
		t.Fatalf("release fence read-1: %v", err)
	}
	if _, err := s.ReleaseFenceForAttempt(ctx, read2.Attempt.ID, "reads-done", time.Now().UTC()); err != nil {
		t.Fatalf("release fence read-2: %v", err)
	}

	write, err := svc.StartAttempt(ctx, outcomeID, outcome.StartAttemptInput{
		PlanRevisionID: plan.ID, WorkUnitID: "wu-write", RequestKey: "rk-write-1",
	})
	if err != nil {
		t.Fatalf("start wu-write after reads finished: %v", err)
	}
	if write.Attempt.Status != domain.AttemptRunning {
		t.Fatalf("wu-write attempt status = %s, want running", write.Attempt.Status)
	}

	// A second concurrent write-capable admission is still refused, exactly
	// as it was before this slice — the exclusive write guarantee.
	_, err = svc.StartAttempt(ctx, outcomeID, outcome.StartAttemptInput{
		PlanRevisionID: plan.ID, WorkUnitID: "wu-write", RequestKey: "rk-write-2",
	})
	if !errors.As(err, &apiErr) {
		t.Fatalf("second write while wu-write is active: err = %v, want an apierr.Error refusal", err)
	}
}
