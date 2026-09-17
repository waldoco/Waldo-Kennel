package outcome

import (
	"strings"
	"testing"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
)

func TestMissionTopologyFingerprintIsStableForStateAndChangesForTopology(t *testing.T) {
	plan := schedulerPlanFixture()
	first := missionTopologyFingerprint(plan)
	if len(first) != 64 {
		t.Fatalf("fingerprint=%q", first)
	}
	copyPlan := plan
	copyPlan.Summary = "state-only metadata"
	if got := missionTopologyFingerprint(copyPlan); got != first {
		t.Fatalf("non-topology field changed fingerprint: %s -> %s", first, got)
	}
	copyPlan.WorkUnits = append([]domain.WorkUnit(nil), plan.WorkUnits...)
	copyPlan.WorkUnits[1].DependsOn = nil
	if got := missionTopologyFingerprint(copyPlan); got == first {
		t.Fatal("dependency change retained topology fingerprint")
	}
	copyPlan = plan
	copyPlan.ID = "another-plan"
	if got := missionTopologyFingerprint(copyPlan); got == first {
		t.Fatal("authorized plan swap retained topology fingerprint")
	}
}

func TestMissionProjectionShapeForkJoinSerialCustody(t *testing.T) {
	plan := schedulerPlanFixture()
	plan.WorkUnits = []domain.WorkUnit{
		plan.WorkUnits[0],
		{ID: "wu-b", Kind: domain.WorkUnitDirect, Role: domain.WorkUnitRoleImplement, Title: "B", ContractRevisionNumber: 1, DependsOn: []domain.WorkUnitID{"wu-a"}, Inputs: []domain.WorkUnitInput{{FromWorkUnitID: "wu-a", Required: "findings", Position: 1}}, CriterionIDs: []domain.CriterionID{"crit-b"}},
		{ID: "wu-c", Kind: domain.WorkUnitDirect, Title: "C", ContractRevisionNumber: 1, DependsOn: []domain.WorkUnitID{"wu-a"}, CriterionIDs: []domain.CriterionID{"crit-c"}},
		{ID: "wu-d", Kind: domain.WorkUnitDirect, Title: "Consolidate", ContractRevisionNumber: 1, DependsOn: []domain.WorkUnitID{"wu-b", "wu-c"}, CriterionIDs: []domain.CriterionID{"crit-d"}},
	}
	now := time.Unix(100, 0).UTC()
	schedule := ScheduleView{Plan: plan, NextRunnableID: "", CustodyHeldBy: "wu-b", WorkUnits: []WorkUnitScheduleView{
		{WorkUnit: plan.WorkUnits[0], State: WorkUnitScheduleProven, CriterionReady: map[domain.CriterionID]bool{"crit-a": true}},
		{WorkUnit: plan.WorkUnits[1], State: WorkUnitScheduleExecuting, Attempts: []domain.Attempt{{ID: "att-b", OutcomeID: plan.OutcomeID, PlanRevisionID: plan.ID, WorkUnitID: "wu-b", Number: 2, ContractRevisionNumber: 1, Status: domain.AttemptRunning, CreatedAt: now, UpdatedAt: now}}, CriterionReady: map[domain.CriterionID]bool{"crit-b": false}},
		{WorkUnit: plan.WorkUnits[2], State: WorkUnitScheduleBlocked, BlockedReason: BlockedCustodyHeld, CriterionReady: map[domain.CriterionID]bool{"crit-c": false}},
		{WorkUnit: plan.WorkUnits[3], State: WorkUnitScheduleBlocked, BlockedReason: BlockedAwaitingDependencyProof, BlockingDependencies: []domain.WorkUnitID{"wu-b", "wu-c"}, CriterionReady: map[domain.CriterionID]bool{"crit-d": false}},
	}}
	view, err := composeMissionProjection(domain.Outcome{ID: plan.OutcomeID, SpaceID: "rsp-project", CurrentRevisionNumber: 1}, schedule, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Nodes) != 4 || len(view.Edges) != 4 {
		t.Fatalf("nodes/edges=%d/%d", len(view.Nodes), len(view.Edges))
	}
	if view.MissionID != "rsp-project" || view.CustodyHeldBy != "wu-b" {
		t.Fatalf("scope/custody=%s/%s", view.MissionID, view.CustodyHeldBy)
	}
	if view.Nodes[1].Role != string(domain.WorkUnitRoleImplement) || len(view.Nodes[1].Inputs) != 1 || view.Nodes[1].Inputs[0].Required != "findings" {
		t.Fatalf("role/input projection=%+v", view.Nodes[1])
	}
	if view.Nodes[2].NextAction != "" || view.Nodes[2].BlockedReason != string(BlockedCustodyHeld) {
		t.Fatalf("parallel branch falsely actionable: %+v", view.Nodes[2])
	}
	if view.Nodes[3].NextAction != "" || strings.Join(stringWorkUnitIDsForTest(view.Nodes[3].BlockingDependencies), ",") != "wu-b,wu-c" {
		t.Fatalf("join=%+v", view.Nodes[3])
	}
}

func stringWorkUnitIDsForTest(in []domain.WorkUnitID) []string {
	out := make([]string, len(in))
	for i, v := range in {
		out[i] = string(v)
	}
	return out
}

func TestMissionGenerationsCoverEveryProjectedStateAndAvoidMillisecondCollision(t *testing.T) {
	plan := schedulerPlanFixture()
	now := time.Unix(100, 123456).UTC()
	base := ScheduleView{Plan: plan, NextRunnableID: "wu-a", WorkUnits: []WorkUnitScheduleView{{WorkUnit: plan.WorkUnits[0], State: WorkUnitScheduleRunnable, CriterionReady: map[domain.CriterionID]bool{"crit-a": false}}, {WorkUnit: plan.WorkUnits[1], State: WorkUnitScheduleBlocked, BlockedReason: BlockedAwaitingDependencyProof, CriterionReady: map[domain.CriterionID]bool{"crit-b": false}}}}
	record := domain.Outcome{ID: plan.OutcomeID, SpaceID: "rsp", CurrentRevisionNumber: 1}
	first, err := composeMissionProjection(record, base, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	changed := base
	changed.WorkUnits = append([]WorkUnitScheduleView(nil), base.WorkUnits...)
	changed.WorkUnits[0] = base.WorkUnits[0]
	changed.WorkUnits[0].CriterionReady = map[domain.CriterionID]bool{"crit-a": true}
	changed.WorkUnits[0].State = WorkUnitScheduleProven
	changed.NextRunnableID = "wu-b"
	second, err := composeMissionProjection(record, changed, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if first.Generation == second.Generation || first.Nodes[0].Generation == second.Nodes[0].Generation {
		t.Fatal("proof/schedule change retained generation")
	}
	withRef, err := composeMissionProjection(record, base, nil, func(id domain.AttemptID) (domain.AttemptSessionRef, bool, error) {
		return domain.AttemptSessionRef{ID: "ref", AttemptID: id, Seq: 1, SessionID: "s", Harness: domain.HarnessCodex, Mode: domain.SessionModeChat, BoundAt: now}, true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// Add an Attempt without changing the millisecond bucket, then attach a ref.
	base.WorkUnits[0].Attempts = []domain.Attempt{{ID: "a", OutcomeID: plan.OutcomeID, PlanRevisionID: plan.ID, WorkUnitID: "wu-a", Number: 1, ContractRevisionNumber: 1, Status: domain.AttemptRunning, CreatedAt: now, UpdatedAt: now}}
	withoutRef, err := composeMissionProjection(record, base, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	withRef, err = composeMissionProjection(record, base, nil, func(id domain.AttemptID) (domain.AttemptSessionRef, bool, error) {
		return domain.AttemptSessionRef{ID: "ref", AttemptID: id, Seq: 1, SessionID: "s", Harness: domain.HarnessCodex, Mode: domain.SessionModeChat, BoundAt: now.Add(time.Nanosecond)}, true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if withoutRef.Generation == withRef.Generation || withoutRef.Nodes[0].Generation == withRef.Nodes[0].Generation {
		t.Fatal("session ref change inside one millisecond retained generation")
	}
}

func TestMissionRetryableAndApprovalAttentionAreActionable(t *testing.T) {
	plan := schedulerPlanFixture()
	now := time.Unix(100, 0).UTC()
	schedule := ScheduleView{Plan: plan, NextRunnableID: "wu-a", WorkUnits: []WorkUnitScheduleView{{WorkUnit: plan.WorkUnits[0], State: WorkUnitScheduleRetryable, CriterionReady: map[domain.CriterionID]bool{}}}}
	q := domain.NeedsYouQuestion{ID: "q", OutcomeID: plan.OutcomeID, PlanRevisionID: plan.ID, WorkUnitID: "wu-a", Kind: domain.NeedsYouApproval, Reason: "Approve elevated capability", Generation: "g", UpdatedAt: now}
	view, err := composeMissionProjection(domain.Outcome{ID: plan.OutcomeID, SpaceID: "rsp", CurrentRevisionNumber: 1}, schedule, map[domain.WorkUnitID]domain.NeedsYouQuestion{"wu-a": q}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if view.Nodes[0].NextAction != "start" {
		t.Fatalf("retry action=%q", view.Nodes[0].NextAction)
	}
	if view.Nodes[0].Attention == nil || view.Nodes[0].Attention.Kind != "needs_approval" || view.Nodes[0].Attention.Summary != q.Reason {
		t.Fatalf("attention=%+v", view.Nodes[0].Attention)
	}
}

func TestMissionProjectionExecutionBindingIsOnlyCanonicalApprovedSemantics(t *testing.T) {
	plan := schedulerPlanFixture()
	plan.WorkUnits[0].Provider = domain.HarnessCodex
	plan.WorkUnits[0].ModelSelection = domain.ExecutionBindingModelExplicit
	plan.WorkUnits[0].Model = "gpt-test"
	plan.WorkUnits[1].Provider = domain.HarnessClaudeCode
	plan.WorkUnits[1].ModelSelection = domain.ExecutionBindingModelHistoricalUnbound
	schedule := ScheduleView{Plan: plan, WorkUnits: []WorkUnitScheduleView{
		{WorkUnit: plan.WorkUnits[0], State: WorkUnitScheduleRunnable},
		{WorkUnit: plan.WorkUnits[1], State: WorkUnitScheduleBlocked},
	}}
	view, err := composeMissionProjection(domain.Outcome{ID: plan.OutcomeID, SpaceID: "rsp", CurrentRevisionNumber: 1}, schedule, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := view.Nodes[0].ExecutionBinding; got == nil || got.Provider != "codex" || got.ModelSelection != "explicit" || got.Model != "gpt-test" {
		t.Fatalf("binding=%+v", got)
	}
	if view.Nodes[1].ExecutionBinding != nil {
		t.Fatalf("historical unbound execution projected as planned authority: %+v", view.Nodes[1].ExecutionBinding)
	}
	if view.Nodes[0].Links == nil || len(view.Nodes[0].Links) != 0 {
		t.Fatalf("safe links must be an empty array, got %#v", view.Nodes[0].Links)
	}
}

func TestMissionGenerationIncludesDisplayEnrichmentsButTopologyDoesNot(t *testing.T) {
	plan := schedulerPlanFixture()
	schedule := ScheduleView{Plan: plan, WorkUnits: []WorkUnitScheduleView{{WorkUnit: plan.WorkUnits[0], State: WorkUnitScheduleRunnable}}}
	view, err := composeMissionProjection(domain.Outcome{ID: plan.OutcomeID, SpaceID: "rsp", CurrentRevisionNumber: 1}, schedule, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	baseTopology, baseGeneration := view.TopologyFingerprint, missionProjectionGeneration(view)
	view.MissionLabel = "Friendly project"
	view.Nodes[0].Links = append(view.Nodes[0].Links, MissionLink{Kind: "retained_result", ID: "att-1", Label: "Retained result", State: "available"})
	if got := missionProjectionGeneration(view); got == baseGeneration {
		t.Fatal("display enrichment retained projection generation")
	}
	if view.TopologyFingerprint != baseTopology {
		t.Fatal("display enrichment changed topology")
	}
}

func TestMeasuredMissionChangesRequiresFrozenCompleteFullyMeasuredReceipt(t *testing.T) {
	zero, two, frozen := int64(0), int64(2), time.Unix(200, 0).UTC()
	base := domain.AttemptReceipt{AttemptID: "att", ArtifactVersion: "artifact", RetentionState: domain.RetentionRetained, FrozenAt: &frozen, Files: []domain.ArtifactFile{{Additions: &two, Deletions: &zero}}}
	if got := measuredMissionChanges(base); got == nil || got.Additions != 2 || got.FilesChanged != 1 || got.SourceAttemptID != "att" || got.ArtifactVersion != "artifact" {
		t.Fatalf("summary=%+v", got)
	}
	unfrozen := base
	unfrozen.FrozenAt = nil
	if measuredMissionChanges(unfrozen) != nil {
		t.Fatal("unfrozen receipt projected measurements")
	}
	partial := base
	partial.RetentionState = domain.RetentionIncomplete
	if measuredMissionChanges(partial) != nil {
		t.Fatal("partial receipt projected measurements")
	}
	ambiguous := base
	ambiguous.Files[0].Deletions = nil
	if measuredMissionChanges(ambiguous) != nil {
		t.Fatal("partly measured receipt projected summary")
	}
}
